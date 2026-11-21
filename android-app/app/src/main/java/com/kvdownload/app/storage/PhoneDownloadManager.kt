package com.kvdownload.app.storage

import android.content.ContentValues
import android.content.Context
import android.content.Intent
import android.media.MediaScannerConnection
import android.net.Uri
import android.os.Build
import android.os.Environment
import android.provider.MediaStore
import android.webkit.MimeTypeMap
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.kvdownload.app.api.DownloadTask
import com.kvdownload.app.api.GalleryItem
import com.kvdownload.app.api.PhoneTransferState
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.SavedPhoneFile
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.launch
import okhttp3.Request
import java.io.File
import java.io.FileOutputStream
import java.net.URLDecoder
import java.util.UUID

class PhoneDownloadManager(private val context: Context) {

    private val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
    private val gson = Gson()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    private val _transfers = MutableStateFlow<Map<String, PhoneTransferState>>(emptyMap())
    val transfers: StateFlow<Map<String, PhoneTransferState>> = _transfers.asStateFlow()

    private val _savedFiles = MutableStateFlow<List<SavedPhoneFile>>(loadSavedCatalog())
    val savedFiles: StateFlow<List<SavedPhoneFile>> = _savedFiles.asStateFlow()

    private val _autoSaveEnabled = MutableStateFlow(prefs.getBoolean(KEY_AUTO_SAVE, false))
    val autoSaveEnabled: StateFlow<Boolean> = _autoSaveEnabled.asStateFlow()

    private val _organizeByChannel = MutableStateFlow(prefs.getBoolean(KEY_ORG_CHANNEL, true))
    val organizeByChannel: StateFlow<Boolean> = _organizeByChannel.asStateFlow()

    private val _organizeByMediaType = MutableStateFlow(prefs.getBoolean(KEY_ORG_TYPE, true))
    val organizeByMediaType: StateFlow<Boolean> = _organizeByMediaType.asStateFlow()

    private val autoSavedTaskIds = CollectionsSafeSet(loadStringSet(KEY_AUTO_SAVED_IDS))

    fun setAutoSaveEnabled(enabled: Boolean) {
        _autoSaveEnabled.value = enabled
        prefs.edit().putBoolean(KEY_AUTO_SAVE, enabled).apply()
    }

    fun setOrganizeByChannel(enabled: Boolean) {
        _organizeByChannel.value = enabled
        prefs.edit().putBoolean(KEY_ORG_CHANNEL, enabled).apply()
    }

    fun setOrganizeByMediaType(enabled: Boolean) {
        _organizeByMediaType.value = enabled
        prefs.edit().putBoolean(KEY_ORG_TYPE, enabled).apply()
    }

    fun isTaskSavedOnPhone(taskId: String): Boolean =
        _savedFiles.value.any { it.serverTaskId == taskId }

    fun isGalleryItemSavedOnPhone(path: String): Boolean =
        _savedFiles.value.any { it.serverPath == path }

    fun findSavedForTask(taskId: String): SavedPhoneFile? =
        _savedFiles.value.firstOrNull { it.serverTaskId == taskId }

    fun findSavedForGallery(path: String): SavedPhoneFile? =
        _savedFiles.value.firstOrNull { it.serverPath == path }

    fun autoSaveIfNeeded(tasks: List<DownloadTask>) {
        if (!_autoSaveEnabled.value) return
        tasks.forEach { task ->
            if (task.status == "completed" &&
                !isTaskSavedOnPhone(task.id) &&
                !autoSavedTaskIds.contains(task.id) &&
                _transfers.value["task:${task.id}"] == null
            ) {
                autoSavedTaskIds.add(task.id)
                persistStringSet(KEY_AUTO_SAVED_IDS, autoSavedTaskIds.snapshot())
                saveTaskToPhone(task)
            }
        }
    }

    fun saveTaskToPhone(task: DownloadTask) {
        val url = RetrofitClient.downloadTaskUrl(task.id, inline = false)
        val fallbackExt = if (task.format == "audio") "mp3" else "mp4"
        val baseTitle = sanitizeFileComponent(task.displayTitle(), "media_${task.id}")
        val fileName = ensureExtension(baseTitle, fallbackExt)
        val category = if (task.format == "audio") "audio" else guessCategoryFromFileName(fileName)
        enqueueDownload(
            transferKey = "task:${task.id}",
            sourceUrl = url,
            preferredFileName = fileName,
            title = task.displayTitle(),
            channel = task.displayChannel(),
            group = task.groupTitle ?: task.playlistTitle,
            category = category,
            serverTaskId = task.id,
            serverPath = null
        )
    }

    fun saveGroupToPhone(tasks: List<DownloadTask>) {
        val completed = tasks.filter { it.status == "completed" }
        completed.forEach { task ->
            if (!isTaskSavedOnPhone(task.id)) {
                saveTaskToPhone(task)
            }
        }
    }

    fun saveGalleryItemToPhone(item: GalleryItem) {
        val url = RetrofitClient.downloadGalleryUrl(item.path, inline = false)
        val fileName = sanitizeFileComponent(item.name, "kv_${System.currentTimeMillis()}.mp4")
        enqueueDownload(
            transferKey = "gallery:${item.path}",
            sourceUrl = url,
            preferredFileName = fileName,
            title = item.displayTitle(),
            channel = item.channel,
            group = item.group,
            category = item.type,
            serverTaskId = null,
            serverPath = item.path
        )
    }

    fun saveDirectMediaToPhone(
        mediaUrl: String,
        title: String,
        type: String,
        channel: String? = null
    ) {
        val ext = when (type) {
            "audio" -> "mp3"
            "image" -> "jpg"
            else -> "mp4"
        }
        val cleanTitle = sanitizeFileComponent(title, "kv_browser_${System.currentTimeMillis()}")
        val fileName = ensureExtension(cleanTitle, ext)
        val fetchUrl = RetrofitClient.browserStreamUrl(mediaUrl)
        enqueueDownload(
            transferKey = "direct:${mediaUrl.hashCode()}",
            sourceUrl = fetchUrl,
            preferredFileName = fileName,
            title = title.ifBlank { fileName },
            channel = channel ?: "Browser Sniffer",
            group = "Browser Captures",
            category = type,
            serverTaskId = null,
            serverPath = null
        )
    }

    fun saveZipArchiveToPhone(zipUrl: String, archiveLabel: String, channelOrGroup: String? = null) {
        val safeName = sanitizeFileComponent(archiveLabel, "kv_archive") + ".zip"
        enqueueDownload(
            transferKey = "zip:${zipUrl.hashCode()}",
            sourceUrl = zipUrl,
            preferredFileName = safeName,
            title = "$archiveLabel (ZIP Archive)",
            channel = channelOrGroup,
            group = archiveLabel,
            category = "archive",
            serverTaskId = null,
            serverPath = null
        )
    }

    private fun enqueueDownload(
        transferKey: String,
        sourceUrl: String,
        preferredFileName: String,
        title: String,
        channel: String?,
        group: String?,
        category: String,
        serverTaskId: String?,
        serverPath: String?
    ) {
        if (_transfers.value[transferKey]?.let { !it.isCompleted && it.error == null } == true) {
            return
        }

        scope.launch {
            updateTransfer(
                PhoneTransferState(
                    key = transferKey,
                    title = title,
                    progress = 0f
                )
            )

            try {
                val request = Request.Builder().url(sourceUrl).build()
                RetrofitClient.streamHttpClient.newCall(request).execute().use { response ->
                    if (!response.isSuccessful) {
                        throw IllegalStateException("HTTP ${response.code}: ${response.message}")
                    }

                    val body = response.body ?: throw IllegalStateException("Empty download body")
                    val totalBytes = body.contentLength()
                    val headerFileName = parseFileNameFromDisposition(
                        response.header("Content-Disposition")
                    )
                    val finalFileName = sanitizeFileComponent(
                        headerFileName ?: preferredFileName,
                        preferredFileName
                    )
                    val mimeType = response.header("Content-Type")
                        ?.substringBefore(';')
                        ?.trim()
                        ?.takeIf { it.contains('/') && it != "application/octet-stream" }
                        ?: resolveMimeType(finalFileName, category)

                    val resolvedCategory = resolveCategoryFromMime(mimeType, category)
                    val subfolder = buildRelativeSubfolder(channel, group)

                    val (savedUri, displayRelPath, writtenBytes) = writeBytesToPublicStorage(
                        fileName = finalFileName,
                        mimeType = mimeType,
                        category = resolvedCategory,
                        subfolder = subfolder,
                        totalBytes = totalBytes,
                        inputStream = body.byteStream()
                    ) { downloaded, total ->
                        val pct = if (total > 0L) (downloaded.toFloat() / total.toFloat()).coerceIn(0f, 1f) else 0f
                        updateTransfer(
                            PhoneTransferState(
                                key = transferKey,
                                title = title,
                                progress = pct,
                                bytesDownloaded = downloaded,
                                totalBytes = total
                            )
                        )
                    }

                    val record = SavedPhoneFile(
                        id = UUID.randomUUID().toString(),
                        serverTaskId = serverTaskId,
                        serverPath = serverPath,
                        fileName = finalFileName,
                        title = title,
                        channel = channel,
                        group = group,
                        category = resolvedCategory,
                        mimeType = mimeType,
                        sizeBytes = writtenBytes,
                        savedAt = System.currentTimeMillis(),
                        contentUri = savedUri.toString(),
                        relativePath = displayRelPath
                    )
                    addSavedRecord(record)

                    updateTransfer(
                        PhoneTransferState(
                            key = transferKey,
                            title = title,
                            progress = 1f,
                            bytesDownloaded = writtenBytes,
                            totalBytes = writtenBytes,
                            isCompleted = true,
                            savedUri = savedUri.toString(),
                            relativePath = displayRelPath
                        )
                    )
                }
            } catch (e: Exception) {
                updateTransfer(
                    PhoneTransferState(
                        key = transferKey,
                        title = title,
                        error = e.message ?: "Failed saving file to phone"
                    )
                )
            }
        }
    }

    private fun writeBytesToPublicStorage(
        fileName: String,
        mimeType: String,
        category: String,
        subfolder: String,
        totalBytes: Long,
        inputStream: java.io.InputStream,
        onProgress: (Long, Long) -> Unit
    ): Triple<Uri, String, Long> {
        val baseDirName = when {
            category == "archive" || mimeType == "application/zip" -> Environment.DIRECTORY_DOWNLOADS
            !_organizeByMediaType.value -> Environment.DIRECTORY_DOWNLOADS
            category == "audio" -> Environment.DIRECTORY_MUSIC
            category == "image" || category == "photo" -> Environment.DIRECTORY_PICTURES
            category == "video" -> Environment.DIRECTORY_MOVIES
            else -> Environment.DIRECTORY_DOWNLOADS
        }

        val relativeFolder = if (subfolder.isNotBlank()) {
            "$baseDirName/KV Download/$subfolder"
        } else {
            "$baseDirName/KV Download"
        }

        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            val resolver = context.contentResolver
            val collectionUri = when (baseDirName) {
                Environment.DIRECTORY_MOVIES -> MediaStore.Video.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
                Environment.DIRECTORY_MUSIC -> MediaStore.Audio.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
                Environment.DIRECTORY_PICTURES -> MediaStore.Images.Media.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
                else -> MediaStore.Downloads.getContentUri(MediaStore.VOLUME_EXTERNAL_PRIMARY)
            }

            val values = ContentValues().apply {
                put(MediaStore.MediaColumns.DISPLAY_NAME, fileName)
                put(MediaStore.MediaColumns.MIME_TYPE, mimeType)
                put(MediaStore.MediaColumns.RELATIVE_PATH, relativeFolder)
                put(MediaStore.MediaColumns.IS_PENDING, 1)
            }

            val itemUri = resolver.insert(collectionUri, values)
                ?: throw IllegalStateException("Could not create MediaStore entry in $relativeFolder")

            var written = 0L
            try {
                resolver.openOutputStream(itemUri)?.use { out ->
                    val buffer = ByteArray(64 * 1024)
                    var read: Int
                    var lastEmit = 0L
                    while (inputStream.read(buffer).also { read = it } != -1) {
                        out.write(buffer, 0, read)
                        written += read
                        val now = System.currentTimeMillis()
                        if (now - lastEmit > 150) {
                            onProgress(written, totalBytes)
                            lastEmit = now
                        }
                    }
                    out.flush()
                } ?: throw IllegalStateException("Unable to open MediaStore output stream")

                val publishValues = ContentValues().apply {
                    put(MediaStore.MediaColumns.IS_PENDING, 0)
                }
                resolver.update(itemUri, publishValues, null, null)
                return Triple(itemUri, "$relativeFolder/$fileName", written)
            } catch (e: Exception) {
                try {
                    resolver.delete(itemUri, null, null)
                } catch (_: Exception) {}
                throw e
            }
        } else {
            @Suppress("DEPRECATION")
            val root = Environment.getExternalStoragePublicDirectory(baseDirName)
            val targetDir = if (subfolder.isNotBlank()) {
                File(File(root, "KV Download"), subfolder)
            } else {
                File(root, "KV Download")
            }
            if (!targetDir.exists()) {
                targetDir.mkdirs()
            }

            val destFile = uniqueTargetFile(targetDir, fileName)
            var written = 0L
            FileOutputStream(destFile).use { out ->
                val buffer = ByteArray(64 * 1024)
                var read: Int
                var lastEmit = 0L
                while (inputStream.read(buffer).also { read = it } != -1) {
                    out.write(buffer, 0, read)
                    written += read
                    val now = System.currentTimeMillis()
                    if (now - lastEmit > 150) {
                        onProgress(written, totalBytes)
                        lastEmit = now
                    }
                }
                out.flush()
            }

            MediaScannerConnection.scanFile(
                context,
                arrayOf(destFile.absolutePath),
                arrayOf(mimeType),
                null
            )
            return Triple(Uri.fromFile(destFile), "$relativeFolder/${destFile.name}", written)
        }
    }

    fun deleteSavedPhoneFile(record: SavedPhoneFile) {
        scope.launch {
            try {
                val uri = Uri.parse(record.contentUri)
                if (uri.scheme == "file") {
                    uri.path?.let { File(it).delete() }
                } else {
                    context.contentResolver.delete(uri, null, null)
                }
            } catch (_: Exception) {
                // Remove catalog entry even if user already deleted it externally.
            }
            _savedFiles.update { list ->
                val updated = list.filterNot { it.id == record.id }
                persistCatalog(updated)
                updated
            }
        }
    }

    fun openOnDevice(record: SavedPhoneFile) {
        try {
            val uri = Uri.parse(record.contentUri)
            val intent = Intent(Intent.ACTION_VIEW).apply {
                setDataAndType(uri, record.mimeType)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            context.startActivity(Intent.createChooser(intent, "Open ${record.title}").addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        } catch (_: Exception) {}
    }

    fun shareFile(record: SavedPhoneFile) {
        try {
            val uri = Uri.parse(record.contentUri)
            val intent = Intent(Intent.ACTION_SEND).apply {
                type = record.mimeType
                putExtra(Intent.EXTRA_STREAM, uri)
                addFlags(Intent.FLAG_GRANT_READ_URI_PERMISSION or Intent.FLAG_ACTIVITY_NEW_TASK)
            }
            context.startActivity(Intent.createChooser(intent, "Share ${record.title}").addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        } catch (_: Exception) {}
    }

    fun clearCompletedTransferBadges() {
        _transfers.update { map -> map.filterValues { !it.isCompleted && it.error == null } }
    }

    private fun buildRelativeSubfolder(channel: String?, group: String?): String {
        if (!_organizeByChannel.value) return ""
        val parts = mutableListOf<String>()
        val cleanChannel = sanitizeFolderComponent(channel)
        val cleanGroup = sanitizeFolderComponent(group)
        if (cleanChannel.isNotBlank()) parts.add(cleanChannel)
        if (cleanGroup.isNotBlank() && !cleanGroup.equals(cleanChannel, ignoreCase = true)) {
            parts.add(cleanGroup)
        }
        return parts.joinToString("/")
    }

    private fun updateTransfer(state: PhoneTransferState) {
        _transfers.update { current ->
            current.toMutableMap().apply { put(state.key, state) }
        }
    }

    private fun addSavedRecord(record: SavedPhoneFile) {
        _savedFiles.update { current ->
            val updated = listOf(record) + current.filterNot {
                (record.serverTaskId != null && it.serverTaskId == record.serverTaskId) ||
                    (record.serverPath != null && it.serverPath == record.serverPath)
            }
            persistCatalog(updated)
            updated
        }
    }

    private fun loadSavedCatalog(): List<SavedPhoneFile> {
        val raw = prefs.getString(KEY_SAVED_CATALOG, null) ?: return emptyList()
        return try {
            val type = object : TypeToken<List<SavedPhoneFile>>() {}.type
            gson.fromJson<List<SavedPhoneFile>>(raw, type) ?: emptyList()
        } catch (_: Exception) {
            emptyList()
        }
    }

    private fun persistCatalog(items: List<SavedPhoneFile>) {
        prefs.edit().putString(KEY_SAVED_CATALOG, gson.toJson(items)).apply()
    }

    private fun loadStringSet(key: String): Set<String> =
        prefs.getStringSet(key, emptySet()) ?: emptySet()

    private fun persistStringSet(key: String, values: Set<String>) {
        prefs.edit().putStringSet(key, values).apply()
    }

    private class CollectionsSafeSet(initial: Set<String>) {
        private val set = initial.toMutableSet()
        @Synchronized fun contains(v: String) = set.contains(v)
        @Synchronized fun add(v: String) = set.add(v)
        @Synchronized fun snapshot(): Set<String> = set.toSet()
    }

    companion object {
        private const val PREFS_NAME = "kv_download_phone_storage"
        private const val KEY_SAVED_CATALOG = "saved_catalog_v1"
        private const val KEY_AUTO_SAVE = "auto_save_completed"
        private const val KEY_ORG_CHANNEL = "organize_by_channel"
        private const val KEY_ORG_TYPE = "organize_by_type"
        private const val KEY_AUTO_SAVED_IDS = "auto_saved_task_ids"

        fun formatFileSize(bytes: Long): String {
            if (bytes <= 0L) return "0 B"
            val units = arrayOf("B", "KB", "MB", "GB", "TB")
            var v = bytes.toDouble()
            var idx = 0
            while (v >= 1024.0 && idx < units.lastIndex) {
                v /= 1024.0
                idx++
            }
            return if (idx == 0) "${bytes} B" else String.format("%.1f %s", v, units[idx])
        }

        private fun parseFileNameFromDisposition(header: String?): String? {
            if (header.isNullOrBlank()) return null
            val utf8Match = Regex("""filename\*\s*=\s*(?:UTF-8''|)([^;]+)""", RegexOption.IGNORE_CASE)
                .find(header)
            if (utf8Match != null) {
                val raw = utf8Match.groupValues[1].trim().trim('"')
                return try {
                    URLDecoder.decode(raw, "UTF-8")
                } catch (_: Exception) {
                    raw
                }
            }
            val stdMatch = Regex("""filename\s*=\s*"?([^";]+)"?""", RegexOption.IGNORE_CASE)
                .find(header)
            return stdMatch?.groupValues?.getOrNull(1)?.trim()
        }

        private fun sanitizeFileComponent(raw: String?, fallback: String): String {
            val cleaned = (raw ?: "")
                .replace(Regex("""[\\/:*?"<>|\u0000-\u001F]"""), "_")
                .trim()
                .take(110)
            return cleaned.ifBlank { fallback }
        }

        private fun sanitizeFolderComponent(raw: String?): String {
            if (raw.isNullOrBlank()) return ""
            val cleaned = raw
                .replace(Regex("""[\\/:*?"<>|\u0000-\u001F]"""), "_")
                .trim('.', ' ')
                .take(48)
            return cleaned
        }

        private fun ensureExtension(fileName: String, fallbackExt: String): String {
            val dot = fileName.lastIndexOf('.')
            return if (dot > 0 && fileName.length - dot in 2..6) fileName else "$fileName.$fallbackExt"
        }

        private fun guessCategoryFromFileName(name: String): String {
            val lower = name.lowercase()
            return when {
                lower.endsWith(".mp3") || lower.endsWith(".m4a") || lower.endsWith(".flac") ||
                    lower.endsWith(".wav") || lower.endsWith(".ogg") || lower.endsWith(".opus") -> "audio"
                lower.endsWith(".jpg") || lower.endsWith(".jpeg") || lower.endsWith(".png") ||
                    lower.endsWith(".webp") || lower.endsWith(".gif") -> "image"
                lower.endsWith(".zip") -> "archive"
                else -> "video"
            }
        }

        private fun resolveMimeType(fileName: String, category: String): String {
            val ext = fileName.substringAfterLast('.', "").lowercase()
            MimeTypeMap.getSingleton().getMimeTypeFromExtension(ext)?.let { return it }
            return when (category) {
                "audio" -> "audio/mpeg"
                "image", "photo" -> "image/jpeg"
                "archive" -> "application/zip"
                else -> "video/mp4"
            }
        }

        private fun resolveCategoryFromMime(mimeType: String, fallback: String): String =
            when {
                mimeType.startsWith("audio/") -> "audio"
                mimeType.startsWith("image/") -> "photo"
                mimeType == "application/zip" -> "archive"
                mimeType.startsWith("video/") -> "video"
                else -> fallback
            }

        private fun uniqueTargetFile(dir: File, fileName: String): File {
            var candidate = File(dir, fileName)
            if (!candidate.exists()) return candidate
            val stem = fileName.substringBeforeLast('.', fileName)
            val ext = fileName.substringAfterLast('.', "")
            var index = 1
            while (candidate.exists()) {
                val nextName = if (ext.isNotBlank()) "${stem}_$index.$ext" else "${stem}_$index"
                candidate = File(dir, nextName)
                index++
            }
            return candidate
        }
    }
}
