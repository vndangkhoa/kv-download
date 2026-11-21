package com.kvdownload.app.update

import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Build
import android.provider.Settings
import androidx.core.content.FileProvider
import androidx.core.content.pm.PackageInfoCompat
import com.kvdownload.app.api.AppVersionResponse
import com.kvdownload.app.api.RetrofitClient
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.Request
import org.json.JSONArray
import java.io.File
import java.io.FileOutputStream

data class AppUpdateInfo(
    val currentVersion: String,
    val currentVersionCode: Long,
    val latestVersion: String,
    val downloadUrl: String,
    val fallbackUrls: List<String>,
    val releaseNotes: String,
    val releasePageUrl: String,
    val hasUpdate: Boolean
)

class AppUpdateManager(private val context: Context) {

    fun getInstalledVersion(): Pair<String, Long> {
        return try {
            val pkgInfo = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                context.packageManager.getPackageInfo(
                    context.packageName,
                    android.content.pm.PackageManager.PackageInfoFlags.of(0)
                )
            } else {
                @Suppress("DEPRECATION")
                context.packageManager.getPackageInfo(context.packageName, 0)
            }
            val verName = (pkgInfo.versionName ?: "1.0.43").removePrefix("v").removePrefix("V")
            val verCode = PackageInfoCompat.getLongVersionCode(pkgInfo)
            verName to verCode
        } catch (_: Exception) {
            "1.0.43" to 4L
        }
    }

    suspend fun checkForUpdate(): Result<AppUpdateInfo> = withContext(Dispatchers.IO) {
        val (installedVer, installedCode) = getInstalledVersion()

        var serverResp: AppVersionResponse? = null
        var serverErr: Exception? = null
        try {
            serverResp = RetrofitClient.getApi().getAppVersion()
        } catch (e: Exception) {
            serverErr = e
        }

        var ghRelease: GitHubApkRelease? = null
        var ghErr: Exception? = null
        try {
            ghRelease = fetchLatestGitHubApkRelease()
        } catch (e: Exception) {
            ghErr = e
        }

        val serverVer = serverResp?.version?.trim()?.removePrefix("v")?.removePrefix("V")
            ?.takeIf { isAppReleaseTag(it) }
        val ghVer = ghRelease?.version?.trim()?.removePrefix("v")?.removePrefix("V")
            ?.takeIf { isAppReleaseTag(it) }

        if (serverResp == null && ghRelease == null) {
            return@withContext Result.failure(
                serverErr ?: ghErr ?: IllegalStateException("Unable to reach update servers")
            )
        }

        val candidates = listOfNotNull(serverVer, ghVer)
        val bestVersion = candidates.maxWithOrNull { a, b -> compareVersions(a, b) } ?: installedVer
        val hasUpdate = isNewerVersion(bestVersion, installedVer)

        val versionedSynoUrl = "https://syno.vndns.net/kvdownload-v$bestVersion.apk"
        val orderedUrls = buildList {
            add(versionedSynoUrl)
            serverResp?.apkUrl?.takeIf { it.isNotBlank() && compareVersions(serverVer ?: "0", bestVersion) >= 0 }
                ?.let { add(it) }
            ghRelease?.apkUrl?.takeIf { it.isNotBlank() && compareVersions(ghVer ?: "0", bestVersion) >= 0 }
                ?.let { add(it) }
            serverResp?.latestApkUrl?.takeIf { it.isNotBlank() }?.let { add(it) }
            add(DEFAULT_SYNO_LATEST_APK)
        }.distinct()

        val notes = buildString {
            val ghNotes = ghRelease?.notes?.trim().orEmpty()
            val srvNotes = serverResp?.notes?.trim().orEmpty()
            when {
                ghNotes.isNotBlank() -> append(ghNotes)
                srvNotes.isNotBlank() -> append(srvNotes)
                else -> append("KV Download Android release v$bestVersion.")
            }
        }

        Result.success(
            AppUpdateInfo(
                currentVersion = installedVer,
                currentVersionCode = installedCode,
                latestVersion = bestVersion,
                downloadUrl = orderedUrls.first(),
                fallbackUrls = orderedUrls.drop(1),
                releaseNotes = notes,
                releasePageUrl = serverResp?.releasePageUrl?.takeIf { it.isNotBlank() }
                    ?: DEFAULT_SYNO_RELEASE_PAGE,
                hasUpdate = hasUpdate
            )
        )
    }

    suspend fun downloadApk(
        info: AppUpdateInfo,
        onProgress: (progress: Float, bytesRead: Long, totalBytes: Long) -> Unit
    ): Result<File> = withContext(Dispatchers.IO) {
        val updateDir = File(context.cacheDir, "updates").apply { mkdirs() }
        val finalFile = File(updateDir, "kvdownload-${info.latestVersion}.apk")
        val tmpFile = File(updateDir, "kvdownload-${info.latestVersion}.apk.tmp")

        val urlsToTry = (listOf(info.downloadUrl) + info.fallbackUrls).filter { it.isNotBlank() }.distinct()
        var lastError: Throwable? = null

        for (url in urlsToTry) {
            try {
                if (tmpFile.exists()) tmpFile.delete()
                val req = Request.Builder()
                    .url(url)
                    .header("User-Agent", "KVDownloadAndroid/${info.currentVersion}")
                    .build()

                RetrofitClient.streamHttpClient.newCall(req).execute().use { resp ->
                    if (!resp.isSuccessful) {
                        throw IllegalStateException("HTTP ${resp.code} from $url")
                    }
                    val body = resp.body ?: throw IllegalStateException("Empty body from $url")
                    val totalBytes = body.contentLength()
                    var bytesRead = 0L

                    body.byteStream().use { input ->
                        FileOutputStream(tmpFile).use { output ->
                            val buf = ByteArray(16 * 1024)
                            var read: Int
                            var lastEmitMs = 0L
                            while (input.read(buf).also { read = it } != -1) {
                                output.write(buf, 0, read)
                                bytesRead += read
                                val now = System.currentTimeMillis()
                                if (now - lastEmitMs > 120L || (totalBytes > 0 && bytesRead >= totalBytes)) {
                                    val prog = if (totalBytes > 0) (bytesRead.toFloat() / totalBytes).coerceIn(0f, 1f) else -1f
                                    onProgress(prog, bytesRead, totalBytes)
                                    lastEmitMs = now
                                }
                            }
                        }
                    }

                    if (tmpFile.length() < 1_000_000L) {
                        throw IllegalStateException("Downloaded APK too small (${tmpFile.length()} bytes)")
                    }
                    if (!hasApkZipHeader(tmpFile)) {
                        throw IllegalStateException("Downloaded file from $url is not a valid APK archive")
                    }
                    if (finalFile.exists()) finalFile.delete()
                    if (!tmpFile.renameTo(finalFile)) {
                        tmpFile.copyTo(finalFile, overwrite = true)
                        tmpFile.delete()
                    }
                    onProgress(1f, finalFile.length(), finalFile.length())
                    return@withContext Result.success(finalFile)
                }
            } catch (t: Throwable) {
                lastError = t
                if (tmpFile.exists()) tmpFile.delete()
            }
        }

        Result.failure(lastError ?: IllegalStateException("Failed to download APK update"))
    }

    fun canRequestPackageInstalls(): Boolean {
        return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            context.packageManager.canRequestPackageInstalls()
        } else {
            true
        }
    }

    fun buildUnknownSourcesSettingsIntent(): Intent {
        return Intent(
            Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
            Uri.parse("package:${context.packageName}")
        )
    }

    fun launchInstaller(apkFile: File): Result<Unit> {
        return try {
            val contentUri = FileProvider.getUriForFile(
                context,
                "${context.packageName}.fileprovider",
                apkFile
            )
            val intent = Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(contentUri, "application/vnd.android.package-archive")
                addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_GRANT_READ_URI_PERMISSION)
            }
            context.startActivity(intent)
            Result.success(Unit)
        } catch (t: Throwable) {
            Result.failure(t)
        }
    }

    private fun fetchLatestGitHubApkRelease(): GitHubApkRelease? {
        val req = Request.Builder()
            .url(GITHUB_RELEASES_API)
            .header("Accept", "application/vnd.github.v3+json")
            .header("User-Agent", "KVDownloadAndroid")
            .build()

        RetrofitClient.okHttpClient.newCall(req).execute().use { resp ->
            if (!resp.isSuccessful) {
                throw IllegalStateException("GitHub API HTTP ${resp.code}")
            }
            val raw = resp.body?.string().orEmpty()
            if (raw.isBlank()) return null

            val releases = JSONArray(raw)
            var best: GitHubApkRelease? = null
            for (i in 0 until releases.length()) {
                val rel = releases.optJSONObject(i) ?: continue
                if (rel.optBoolean("draft", false)) continue
                val tagName = rel.optString("tag_name", "").trim()
                val normalized = tagName.removePrefix("v").removePrefix("V").trim()
                if (!isAppReleaseTag(normalized)) continue

                val assets = rel.optJSONArray("assets") ?: continue
                var preferredApkUrl = ""
                var fallbackApkUrl = ""
                for (j in 0 until assets.length()) {
                    val asset = assets.optJSONObject(j) ?: continue
                    val name = asset.optString("name", "")
                    val dlUrl = asset.optString("browser_download_url", "")
                    if (name.endsWith(".apk", ignoreCase = true) && dlUrl.isNotBlank()) {
                        if (name.contains(normalized)) {
                            preferredApkUrl = dlUrl
                        } else if (fallbackApkUrl.isBlank()) {
                            fallbackApkUrl = dlUrl
                        }
                    }
                }
                val apkUrl = preferredApkUrl.ifBlank { fallbackApkUrl }
                if (apkUrl.isBlank()) continue

                val candidate = GitHubApkRelease(
                    version = normalized,
                    apkUrl = apkUrl,
                    notes = rel.optString("body", "")
                )
                if (best == null || compareVersions(candidate.version, best.version) > 0) {
                    best = candidate
                }
            }
            return best
        }
    }

    private fun hasApkZipHeader(file: File): Boolean {
        return try {
            file.inputStream().use { ins ->
                val header = ByteArray(4)
                ins.read(header) == 4 &&
                    header[0] == 0x50.toByte() &&
                    header[1] == 0x4B.toByte() &&
                    header[2] == 0x03.toByte() &&
                    header[3] == 0x04.toByte()
            }
        } catch (_: Exception) {
            false
        }
    }

    private data class GitHubApkRelease(
        val version: String,
        val apkUrl: String,
        val notes: String
    )

    companion object {
        const val DEFAULT_SYNO_RELEASE_PAGE = "https://syno.vndns.net/package/kvdownload"
        const val DEFAULT_SYNO_LATEST_APK = "https://syno.vndns.net/kvdownload-release.apk"
        private const val GITHUB_RELEASES_API =
            "https://api.github.com/repos/vndangkhoa/kv-download/releases?per_page=20"

        fun isAppReleaseTag(rawVersion: String): Boolean {
            val cleaned = rawVersion.trim().removePrefix("v").removePrefix("V")
            if (cleaned.isBlank() || cleaned.contains("-")) return false
            val parts = cleaned.split(".")
            if (parts.isEmpty()) return false
            return parts.all { part -> part.isNotEmpty() && part.all { it.isDigit() } }
        }

        fun compareVersions(a: String, b: String): Int {
            val partsA = a.trim().removePrefix("v").removePrefix("V").split(".")
                .map { it.takeWhile { c -> c.isDigit() }.toIntOrNull() ?: 0 }
            val partsB = b.trim().removePrefix("v").removePrefix("V").split(".")
                .map { it.takeWhile { c -> c.isDigit() }.toIntOrNull() ?: 0 }
            val maxLen = maxOf(partsA.size, partsB.size)
            for (i in 0 until maxLen) {
                val va = partsA.getOrElse(i) { 0 }
                val vb = partsB.getOrElse(i) { 0 }
                if (va != vb) return va.compareTo(vb)
            }
            return 0
        }

        fun isNewerVersion(candidate: String, current: String): Boolean {
            return isAppReleaseTag(candidate) && compareVersions(candidate, current) > 0
        }
    }
}
