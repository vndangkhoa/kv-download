package com.kvdownload.app.storage

import android.content.Context
import android.graphics.Bitmap
import android.media.MediaMetadataRetriever
import android.net.Uri
import android.webkit.CookieManager
import android.webkit.MimeTypeMap
import com.google.gson.Gson
import com.google.gson.reflect.TypeToken
import com.kvdownload.app.api.AV01Resolver
import com.kvdownload.app.api.DownloadTask
import com.kvdownload.app.api.GalleryItem
import com.kvdownload.app.api.PhoneTransferState
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.SavedPhoneFile
import com.kvdownload.app.api.SniffedMedia
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
import java.io.FileInputStream
import java.io.FileOutputStream
import java.io.InputStream
import java.net.URI
import java.net.URLDecoder
import java.util.UUID

class PhoneDownloadManager(private val context: Context) {

    private val prefs = context.getSharedPreferences(PREFS_NAME, Context.MODE_PRIVATE)
    private val gson = Gson()
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
    private val vaultRoot: File by lazy {
        File(context.filesDir, "private_vault").apply {
            if (!exists()) mkdirs()
            val nomedia = File(this, ".nomedia")
            if (!nomedia.exists()) runCatching { nomedia.createNewFile() }
        }
    }

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
        val url = RetrofitClient.downloadTaskUrl(task.id, inline = false, mediaId = task.mediaId)
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
        channel: String? = null,
        referer: String? = null,
        formatPref: String = "highest",
        group: String? = null
    ) {
        if (AV01Resolver.isAV01Url(mediaUrl) || HlsSegmentDownloader.looksLikeHlsUrl(mediaUrl)) {
            saveHlsOrAV01ToPhone(
                url = mediaUrl,
                title = title.ifBlank { null },
                formatPref = formatPref,
                channel = channel,
                referer = referer
            )
            return
        }

        val ext = when (type) {
            "audio" -> "mp3"
            "image", "photo" -> "jpg"
            else -> "mp4"
        }
        val cleanTitle = sanitizeFileComponent(title, "kv_browser_${System.currentTimeMillis()}")
        val fileName = ensureExtension(cleanTitle, ext)
        val routedSource = if (isDirectMediaLink(mediaUrl)) {
            mediaUrl
        } else {
            RetrofitClient.browserStreamUrl(mediaUrl, referer)
        }
        val routedFallback = if (routedSource == mediaUrl) {
            RetrofitClient.browserStreamUrl(mediaUrl, referer)
        } else {
            null
        }
        enqueueDownload(
            transferKey = "direct:${mediaUrl.hashCode()}",
            sourceUrl = routedSource,
            fallbackSourceUrl = routedFallback,
            referer = referer,
            preferredFileName = fileName,
            title = title.ifBlank { fileName },
            channel = channel ?: "Browser Sniffer",
            group = group ?: "Browser Captures",
            category = type,
            serverTaskId = null,
            serverPath = null
        )
    }

    fun saveBatchToPhone(
        items: List<SniffedMedia>,
        formatPref: String = "highest",
        defaultReferer: String? = null,
        groupName: String? = null
    ) {
        items.forEach { item ->
            saveDirectMediaToPhone(
                mediaUrl = item.url,
                title = item.title,
                type = item.type,
                channel = item.channel ?: groupName,
                referer = item.referer ?: defaultReferer,
                formatPref = item.resolution ?: formatPref,
                group = groupName ?: "Browser Batch"
            )
        }
    }

    fun saveHlsOrAV01ToPhone(
        url: String,
        title: String? = null,
        formatPref: String = "highest",
        channel: String? = null,
        referer: String? = null
    ) {
        val transferKey = "hls:${url.hashCode()}"
        if (_transfers.value[transferKey]?.let { !it.isCompleted && it.error == null } == true) {
            return
        }

        val initialTitle = title?.takeIf { it.isNotBlank() } ?: "HLS Stream"
        scope.launch {
            updateTransfer(
                PhoneTransferState(
                    key = transferKey,
                    title = initialTitle,
                    progress = 0f,
                    statusText = if (AV01Resolver.isAV01Url(url)) "Resolving AV01 stream..." else "Preparing HLS segments..."
                )
            )

            var tempFile: File? = null
            try {
                var targetPlaylistUrl = url
                var resolvedTitle = title?.takeIf { it.isNotBlank() }
                var resolvedChannel = channel
                var resolvedReferer = referer
                var qualityLabel: String? = null

                if (AV01Resolver.isAV01Url(url)) {
                    val av = AV01Resolver.resolve(url)
                    resolvedTitle = resolvedTitle ?: av.title
                    resolvedChannel = resolvedChannel ?: "AV01"
                    resolvedReferer = resolvedReferer ?: av.referer
                    if (!url.contains("iw01.xyz", ignoreCase = true) || url.contains("master.m3u8", ignoreCase = true)) {
                        val picked = AV01Resolver.selectVariant(av.variants, formatPref)
                        targetPlaylistUrl = picked.url
                        qualityLabel = picked.label
                    } else {
                        // Already a specific variant URL: still refresh its access_token via AV01Resolver master if needed
                        targetPlaylistUrl = AV01Resolver.selectVariant(av.variants, formatPref).url
                        qualityLabel = AV01Resolver.selectVariant(av.variants, formatPref).label
                    }
                }

                val displayTitle = resolvedTitle ?: "HLS Video"
                val downloaded = HlsSegmentDownloader.downloadHlsToTempFile(
                    context = context,
                    playlistUrl = targetPlaylistUrl,
                    referer = resolvedReferer,
                    preferredFormat = formatPref,
                    qualityLabel = qualityLabel
                ) { progress, bytesNow, status ->
                    updateTransfer(
                        PhoneTransferState(
                            key = transferKey,
                            title = displayTitle,
                            progress = progress,
                            bytesDownloaded = bytesNow,
                            totalBytes = -1L,
                            statusText = status
                        )
                    )
                }
                tempFile = downloaded.file

                val baseName = sanitizeFileComponent(displayTitle, "kv_hls_${System.currentTimeMillis()}")
                val finalFileName = ensureExtension(baseName.substringBeforeLast('.', baseName), downloaded.extension)
                val finalChannel = resolvedChannel ?: "HLS Streams"
                val subfolder = buildRelativeSubfolder(finalChannel, "M3U8 Captures")
                val recordId = UUID.randomUUID().toString()

                val vaultResult = FileInputStream(downloaded.file).use { input ->
                    writeBytesToPrivateVault(
                        fileName = finalFileName,
                        category = "video",
                        subfolder = subfolder,
                        totalBytes = downloaded.totalBytes,
                        inputStream = input
                    ) { copied, total ->
                        val frac = if (total > 0L) 0.96f + 0.04f * (copied.toFloat() / total.toFloat()) else 0.98f
                        updateTransfer(
                            PhoneTransferState(
                                key = transferKey,
                                title = displayTitle,
                                progress = frac.coerceIn(0f, 1f),
                                bytesDownloaded = copied,
                                totalBytes = total,
                                statusText = "Saving to Private Vault..."
                            )
                        )
                    }
                }
                val thumbUri = extractPrivateVideoThumbnail(vaultResult.file, recordId)

                val record = SavedPhoneFile(
                    id = recordId,
                    serverTaskId = null,
                    serverPath = null,
                    fileName = finalFileName,
                    title = displayTitle,
                    channel = finalChannel,
                    group = "M3U8 Captures",
                    category = "video",
                    mimeType = downloaded.mimeType,
                    sizeBytes = vaultResult.bytesWritten,
                    savedAt = System.currentTimeMillis(),
                    contentUri = vaultResult.uri.toString(),
                    relativePath = vaultResult.displayPath,
                    thumbnailUri = thumbUri
                )
                addSavedRecord(record)

                updateTransfer(
                    PhoneTransferState(
                        key = transferKey,
                        title = displayTitle,
                        progress = 1f,
                        bytesDownloaded = vaultResult.bytesWritten,
                        totalBytes = vaultResult.bytesWritten,
                        isCompleted = true,
                        savedUri = vaultResult.uri.toString(),
                        relativePath = vaultResult.displayPath,
                        statusText = "Saved privately in app"
                    )
                )
            } catch (e: Exception) {
                updateTransfer(
                    PhoneTransferState(
                        key = transferKey,
                        title = initialTitle,
                        error = e.message ?: "Failed saving M3U8 stream to phone"
                    )
                )
            } finally {
                tempFile?.delete()
            }
        }
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
        fallbackSourceUrl: String? = null,
        referer: String? = null,
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
                    progress = 0f,
                    statusText = "Connecting..."
                )
            )

            try {
                fun buildReq(url: String): Request {
                    val b = Request.Builder().url(url)
                    if (!referer.isNullOrBlank()) {
                        b.header("Referer", referer)
                        try {
                            val u = URI.create(referer)
                            if (!u.scheme.isNullOrBlank() && !u.host.isNullOrBlank()) {
                                b.header("Origin", "${u.scheme}://${u.host}")
                            }
                        } catch (_: Exception) {}
                    }
                    try {
                        CookieManager.getInstance().getCookie(url)?.takeIf { it.isNotBlank() }?.let {
                            b.header("Cookie", it)
                        }
                    } catch (_: Exception) {}
                    return b.build()
                }

                var activeUrl = sourceUrl
                var response = try {
                    RetrofitClient.streamHttpClient.newCall(buildReq(activeUrl)).execute()
                } catch (e: Exception) {
                    if (!fallbackSourceUrl.isNullOrBlank() && fallbackSourceUrl != activeUrl) {
                        activeUrl = fallbackSourceUrl
                        RetrofitClient.streamHttpClient.newCall(buildReq(activeUrl)).execute()
                    } else {
                        throw e
                    }
                }

                if (!response.isSuccessful && !fallbackSourceUrl.isNullOrBlank() && fallbackSourceUrl != activeUrl) {
                    response.close()
                    activeUrl = fallbackSourceUrl
                    response = RetrofitClient.streamHttpClient.newCall(buildReq(activeUrl)).execute()
                }

                var initialCt = response.header("Content-Type")?.lowercase().orEmpty()
                if ((initialCt.contains("text/html") || initialCt.contains("application/xhtml")) &&
                    !fallbackSourceUrl.isNullOrBlank() &&
                    fallbackSourceUrl != activeUrl
                ) {
                    response.close()
                    activeUrl = fallbackSourceUrl
                    response = RetrofitClient.streamHttpClient.newCall(buildReq(activeUrl)).execute()
                    initialCt = response.header("Content-Type")?.lowercase().orEmpty()
                }

                response.use { resp ->
                    if (!resp.isSuccessful) {
                        throw IllegalStateException("HTTP ${resp.code}: ${resp.message}")
                    }

                    val rawCt = initialCt.ifBlank { resp.header("Content-Type")?.lowercase().orEmpty() }
                    if (rawCt.contains("text/html") || rawCt.contains("application/xhtml")) {
                        throw IllegalStateException("Target returned webpage instead of playable media")
                    }
                    if (rawCt.contains("mpegurl") || HlsSegmentDownloader.looksLikeHlsUrl(activeUrl)) {
                        resp.close()
                        val hlsOut = HlsSegmentDownloader.downloadHlsToTempFile(
                            context = context,
                            playlistUrl = activeUrl,
                            referer = referer,
                            preferredFormat = "highest",
                            qualityLabel = null
                        ) { progress, bytesNow, status ->
                            updateTransfer(
                                PhoneTransferState(
                                    key = transferKey,
                                    title = title,
                                    progress = progress,
                                    bytesDownloaded = bytesNow,
                                    totalBytes = -1L,
                                    statusText = status
                                )
                            )
                        }
                        try {
                            val finalFileName = ensureExtension(
                                preferredFileName.substringBeforeLast('.', preferredFileName),
                                hlsOut.extension
                            )
                            val subfolder = buildRelativeSubfolder(channel, group)
                            val recordId = UUID.randomUUID().toString()
                            val vaultResult = FileInputStream(hlsOut.file).use { input ->
                                writeBytesToPrivateVault(
                                    fileName = finalFileName,
                                    category = "video",
                                    subfolder = subfolder,
                                    totalBytes = hlsOut.totalBytes,
                                    inputStream = input
                                ) { _, _ -> }
                            }
                            val thumbUri = extractPrivateVideoThumbnail(vaultResult.file, recordId)
                            addSavedRecord(
                                SavedPhoneFile(
                                    id = recordId,
                                    serverTaskId = serverTaskId,
                                    serverPath = serverPath,
                                    fileName = finalFileName,
                                    title = title,
                                    channel = channel,
                                    group = group,
                                    category = "video",
                                    mimeType = hlsOut.mimeType,
                                    sizeBytes = vaultResult.bytesWritten,
                                    savedAt = System.currentTimeMillis(),
                                    contentUri = vaultResult.uri.toString(),
                                    relativePath = vaultResult.displayPath,
                                    thumbnailUri = thumbUri
                                )
                            )
                            updateTransfer(
                                PhoneTransferState(
                                    key = transferKey,
                                    title = title,
                                    progress = 1f,
                                    bytesDownloaded = vaultResult.bytesWritten,
                                    totalBytes = vaultResult.bytesWritten,
                                    isCompleted = true,
                                    savedUri = vaultResult.uri.toString(),
                                    relativePath = vaultResult.displayPath,
                                    statusText = "Saved privately in app"
                                )
                            )
                        } finally {
                            hlsOut.file.delete()
                        }
                        return@launch
                    }

                    val body = resp.body ?: throw IllegalStateException("Empty download body")
                    val totalBytes = body.contentLength()
                    val headerFileName = parseFileNameFromDisposition(
                        resp.header("Content-Disposition")
                    )
                    val finalFileName = sanitizeFileComponent(
                        headerFileName ?: preferredFileName,
                        preferredFileName
                    )
                    val mimeType = resp.header("Content-Type")
                        ?.substringBefore(';')
                        ?.trim()
                        ?.takeIf { it.contains('/') && it != "application/octet-stream" }
                        ?: resolveMimeType(finalFileName, category)

                    val resolvedCategory = resolveCategoryFromMime(mimeType, category)
                    val subfolder = buildRelativeSubfolder(channel, group)
                    val recordId = UUID.randomUUID().toString()

                    val vaultResult = writeBytesToPrivateVault(
                        fileName = finalFileName,
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
                                totalBytes = total,
                                statusText = if (total > 0L) "${(pct * 100).toInt()}% · ${formatFileSize(downloaded)}" else formatFileSize(downloaded)
                            )
                        )
                    }
                    val thumbUri = if (resolvedCategory == "video") {
                        extractPrivateVideoThumbnail(vaultResult.file, recordId)
                    } else null

                    val record = SavedPhoneFile(
                        id = recordId,
                        serverTaskId = serverTaskId,
                        serverPath = serverPath,
                        fileName = finalFileName,
                        title = title,
                        channel = channel,
                        group = group,
                        category = resolvedCategory,
                        mimeType = mimeType,
                        sizeBytes = vaultResult.bytesWritten,
                        savedAt = System.currentTimeMillis(),
                        contentUri = vaultResult.uri.toString(),
                        relativePath = vaultResult.displayPath,
                        thumbnailUri = thumbUri
                    )
                    addSavedRecord(record)

                    updateTransfer(
                        PhoneTransferState(
                            key = transferKey,
                            title = title,
                            progress = 1f,
                            bytesDownloaded = vaultResult.bytesWritten,
                            totalBytes = vaultResult.bytesWritten,
                            isCompleted = true,
                            savedUri = vaultResult.uri.toString(),
                            relativePath = vaultResult.displayPath,
                            statusText = "Saved privately in app"
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

    private data class PrivateVaultWriteResult(
        val file: File,
        val uri: Uri,
        val displayPath: String,
        val bytesWritten: Long
    )

    private fun writeBytesToPrivateVault(
        fileName: String,
        category: String,
        subfolder: String,
        totalBytes: Long,
        inputStream: InputStream,
        onProgress: (Long, Long) -> Unit
    ): PrivateVaultWriteResult {
        val baseDirName = when {
            category == "archive" -> "Archives"
            !_organizeByMediaType.value -> "Files"
            category == "audio" -> "Audio"
            category == "image" || category == "photo" -> "Photos"
            category == "video" -> "Videos"
            else -> "Files"
        }

        val root = vaultRoot
        val typeDir = File(root, baseDirName)
        val targetDir = if (subfolder.isNotBlank()) File(typeDir, subfolder) else typeDir
        if (!targetDir.exists()) {
            targetDir.mkdirs()
        }

        val destFile = uniqueTargetFile(targetDir, fileName)
        val partFile = File(targetDir, "${destFile.name}.${UUID.randomUUID()}.part")
        var written = 0L
        try {
            FileOutputStream(partFile).use { out ->
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
            if (written <= 0L) {
                partFile.delete()
                throw IllegalStateException("Downloaded file is empty")
            }
            if (!partFile.renameTo(destFile)) {
                partFile.copyTo(destFile, overwrite = true)
                partFile.delete()
            }
        } catch (e: Exception) {
            partFile.delete()
            if (destFile.exists() && destFile.length() == 0L) destFile.delete()
            throw e
        }

        val displayRel = if (subfolder.isNotBlank()) {
            "Private Vault/$baseDirName/$subfolder/${destFile.name}"
        } else {
            "Private Vault/$baseDirName/${destFile.name}"
        }
        return PrivateVaultWriteResult(destFile, Uri.fromFile(destFile), displayRel, written)
    }

    private fun extractPrivateVideoThumbnail(videoFile: File, recordId: String): String? {
        val retriever = MediaMetadataRetriever()
        return try {
            retriever.setDataSource(videoFile.absolutePath)
            val rawBmp = retriever.getFrameAtTime(1_000_000L, MediaMetadataRetriever.OPTION_CLOSEST_SYNC)
                ?: retriever.frameAtTime
                ?: return null
            val maxDim = 480
            val w = rawBmp.width
            val h = rawBmp.height
            val scaled = if (w > maxDim && h > 0) {
                val targetH = ((h.toFloat() / w.toFloat()) * maxDim).toInt().coerceAtLeast(1)
                Bitmap.createScaledBitmap(rawBmp, maxDim, targetH, true).also {
                    if (it !== rawBmp) rawBmp.recycle()
                }
            } else {
                rawBmp
            }
            val thumbDir = File(vaultRoot, ".thumbs")
            if (!thumbDir.exists()) thumbDir.mkdirs()
            val thumbFile = File(thumbDir, "$recordId.jpg")
            FileOutputStream(thumbFile).use { out ->
                scaled.compress(Bitmap.CompressFormat.JPEG, 82, out)
                out.flush()
            }
            scaled.recycle()
            Uri.fromFile(thumbFile).toString()
        } catch (_: Exception) {
            null
        } finally {
            try {
                retriever.release()
            } catch (_: Exception) {}
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
            record.thumbnailUri?.let { thumb ->
                runCatching {
                    val tUri = Uri.parse(thumb)
                    if (tUri.scheme == "file") tUri.path?.let { File(it).delete() }
                }
            }
            _savedFiles.update { list ->
                val updated = list.filterNot { it.id == record.id }
                persistCatalog(updated)
                updated
            }
        }
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
            val parsed = gson.fromJson<List<SavedPhoneFile>>(raw, type) ?: emptyList()
            parsed.filter { item ->
                val uri = Uri.parse(item.contentUri)
                if (uri.scheme == "file") {
                    uri.path?.let { File(it).exists() && File(it).length() > 0L } == true
                } else {
                    true
                }
            }
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

        fun isDirectMediaLink(url: String): Boolean {
            val trimmed = url.trim()
            if (trimmed.isBlank()) return false
            val lower = trimmed.lowercase()
            if (lower.contains("/download?id=") || lower.contains("/download?file=") ||
                lower.contains("/download/zip") || lower.contains("/api/browser/stream")
            ) {
                return true
            }
            val cleanPath = lower.substringBefore('?').substringBefore('#')
            val mediaExts = listOf(
                ".mp4", ".m4v", ".webm", ".mkv", ".mov", ".avi", ".flv",
                ".m3u8", ".mpd", ".mp3", ".m4a", ".aac", ".flac", ".ogg", ".wav",
                ".jpg", ".jpeg", ".png", ".webp", ".gif", ".zip"
            )
            if (mediaExts.any { cleanPath.endsWith(it) }) return true
            if (lower.contains("tiktokcdn") && lower.contains("mime_type=video")) return true
            if (lower.contains("googlevideo.com/videoplayback")) return true
            return false
        }

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
