package com.kvdownload.app.api

import com.google.gson.annotations.SerializedName

data class DownloadTask(
    val id: String = "",
    val url: String = "",
    val status: String = "queued",
    val percent: Double = 0.0,
    val speed: String? = null,
    val eta: String? = null,
    val title: String? = null,
    val mediaName: String? = null,
    val channel: String? = null,
    val uploader: String? = null,
    val format: String? = null,
    val humanSize: String? = null,
    val totalBytes: Long? = null,
    val mediaId: String? = null,
    val thumbnail: String? = null,
    val errorMessage: String? = null,
    val createdAt: String = "",
    val playlistTitle: String? = null,
    val infoJsonPath: String? = null,
    val groupId: String? = null,
    val groupTitle: String? = null,
    val groupType: String? = null,
    val platform: String? = null,
    val duration: String? = null
) {
    fun displayTitle(): String =
        title?.takeIf { it.isNotBlank() }
            ?: mediaName?.takeIf { it.isNotBlank() }
            ?: url

    fun displayChannel(): String =
        channel?.takeIf { it.isNotBlank() }
            ?: uploader?.takeIf { it.isNotBlank() }
            ?: platform?.takeIf { it.isNotBlank() }
            ?: "Web"
}

data class TaskGroup(
    val groupId: String,
    val title: String,
    val groupType: String,
    val platform: String,
    val tasks: List<DownloadTask>,
    val completedCount: Int,
    val downloadingCount: Int,
    val queuedCount: Int,
    val failedCount: Int,
    val cancelledCount: Int,
    val totalBytes: Long,
    val overallPercent: Int,
    val latestCreatedAt: String
)

data class QueueResponse(
    val tasks: List<DownloadTask> = emptyList()
)

data class StreamVariant(
    val label: String,
    val url: String,
    val width: Int = 0,
    val height: Int = 0,
    val bandwidth: Long = 0L
)

data class EnqueueItemInput(
    val url: String,
    val title: String? = null,
    val thumbnail: String? = null,
    val uploader: String? = null,
    val channel: String? = null,
    val duration: String? = null,
    val infoJsonPath: String? = null,
    val referer: String? = null
)

data class QueueAddRequest(
    val urls: List<String>? = null,
    val items: List<EnqueueItemInput>? = null,
    val format: String = "highest",
    val cookies: String? = null,
    val rateLimit: String? = null,
    val playlistTitle: String? = null,
    val channel: String? = null,
    val groupId: String? = null,
    val groupTitle: String? = null,
    val groupType: String? = null,
    val referer: String? = null
)

data class QueueAddResponse(
    val success: Boolean = false,
    val tasks: List<DownloadTask>? = null,
    val error: String? = null
)

data class GalleryItem(
    val id: String = "",
    val name: String = "",
    val path: String = "",
    val title: String? = null,
    val channel: String? = null,
    val group: String? = null,
    val platform: String? = null,
    val duration: String? = null,
    val category: String? = null,
    val humanSize: String = "",
    val sizeInBytes: Long = 0L,
    val modTime: String = "",
    val type: String = "video",
    val thumbnail: String? = null,
    val url: String? = null
) {
    fun displayTitle(): String = title?.takeIf { it.isNotBlank() } ?: name
}

data class GalleryFolder(
    val name: String = "",
    val fileCount: Int = 0,
    val category: String? = null,
    val platform: String? = null,
    val subfolders: List<String>? = null,
    val totalBytes: Long? = null,
    val humanSize: String? = null,
    val thumbnail: String? = null,
    val modTime: String? = null
)

data class GalleryResponse(
    val items: List<GalleryItem> = emptyList(),
    val folders: List<GalleryFolder> = emptyList(),
    val totalItems: Int = 0,
    val totalSize: String = ""
)

data class GalleryOrganizeResponse(
    val success: Boolean = false,
    val movedFiles: Int = 0,
    val totalItems: Int = 0,
    val organizeMode: String? = null
)

data class DeleteMultipleRequest(
    val paths: List<String>
)

data class DeleteMultipleResponse(
    val deletedCount: Int = 0
)

data class StorageSettings(
    val downloadDir: String? = null,
    val path: String? = null,
    val organizeMode: String? = null
) {
    fun effectivePath(): String = downloadDir?.takeIf { it.isNotBlank() } ?: path ?: ""
}

data class StorageUpdateRequest(
    val downloadDir: String,
    val path: String,
    val organizeMode: String? = null
)

data class YtDlpVersionInfo(
    val version: String = "",
    val latest: String? = null,
    val channel: String? = null,
    val updateAvailable: Boolean? = null
)

data class UpdateYtdlpResponse(
    val message: String = ""
)

data class AppVersionResponse(
    val version: String = "",
    val apkUrl: String? = null,
    val latestApkUrl: String? = null,
    val releasePageUrl: String? = null,
    val notes: String? = null
)

data class WorkersResponse(
    val maxWorkers: Int = 3
)

data class ScanEntry(
    val id: String = "",
    val url: String = "",
    val webpage_url: String? = null,
    val title: String = "",
    val durationString: String? = null,
    @SerializedName("duration_string") val durationAlt: String? = null,
    val thumbnail: String? = null,
    val uploader: String? = null,
    val channel: String? = null,
    val infoJsonPath: String? = null
) {
    fun resolvedUrl(): String = url.ifBlank { webpage_url ?: "" }
    fun resolvedDuration(): String? = durationString ?: durationAlt
}

data class ScanResponse(
    val id: String? = null,
    val title: String? = null,
    val playlistTitle: String? = null,
    val channel: String? = null,
    val uploader: String? = null,
    val webpage_url: String? = null,
    val thumbnail: String? = null,
    val entries: List<ScanEntry>? = null,
    val playlistCount: Int? = null,
    val hasMore: Boolean? = null,
    val error: String? = null
)

data class SniffedMedia(
    val id: String = "",
    val url: String = "",
    val type: String = "video",
    val format: String? = null,
    val title: String = "",
    val size: String? = null,
    val thumbnail: String? = null,
    val source: String? = null,
    val duration: String? = null,
    val resolution: String? = null,
    val channel: String? = null,
    val isStream: Boolean? = null,
    val referer: String? = null,
    val variants: List<StreamVariant>? = null
)

data class BrowserSniffResponse(
    val url: String? = null,
    val pageTitle: String? = null,
    val items: List<SniffedMedia> = emptyList(),
    val error: String? = null
)

data class SavedPhoneFile(
    val id: String,
    val serverTaskId: String? = null,
    val serverPath: String? = null,
    val fileName: String,
    val title: String,
    val channel: String? = null,
    val group: String? = null,
    val category: String,
    val mimeType: String,
    val sizeBytes: Long,
    val savedAt: Long,
    val contentUri: String,
    val relativePath: String
)

data class PhoneTransferState(
    val key: String,
    val title: String,
    val progress: Float = 0f,
    val bytesDownloaded: Long = 0L,
    val totalBytes: Long = -1L,
    val isCompleted: Boolean = false,
    val error: String? = null,
    val savedUri: String? = null,
    val relativePath: String? = null,
    val statusText: String? = null
)
