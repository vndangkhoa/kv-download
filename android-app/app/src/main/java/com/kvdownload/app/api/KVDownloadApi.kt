package com.kvdownload.app.api

import okhttp3.ResponseBody
import retrofit2.Response
import retrofit2.http.Body
import retrofit2.http.DELETE
import retrofit2.http.GET
import retrofit2.http.POST
import retrofit2.http.Query

interface KVDownloadApi {
    @GET("/health")
    suspend fun health(): Response<ResponseBody>

    @GET("/api/queue")
    suspend fun getQueue(): QueueResponse

    @POST("/api/queue/add")
    suspend fun addQueue(@Body body: QueueAddRequest): QueueAddResponse

    @POST("/api/queue/cancel")
    suspend fun cancelTask(@Query("id") id: String): Response<ResponseBody>

    @POST("/api/queue/retry")
    suspend fun retryTask(@Query("id") id: String): Response<ResponseBody>

    @DELETE("/api/queue/item")
    suspend fun deleteTask(
        @Query("id") id: String,
        @Query("deleteFile") deleteFile: Boolean = false
    ): Response<ResponseBody>

    @POST("/api/queue/group/cancel")
    suspend fun cancelGroup(@Query("groupId") groupId: String): Response<ResponseBody>

    @POST("/api/queue/group/retry")
    suspend fun retryGroup(@Query("groupId") groupId: String): Response<ResponseBody>

    @DELETE("/api/queue/group")
    suspend fun deleteGroup(
        @Query("groupId") groupId: String,
        @Query("deleteFiles") deleteFiles: Boolean = false
    ): Response<ResponseBody>

    @POST("/api/queue/clear-completed")
    suspend fun clearCompleted(): Response<ResponseBody>

    @POST("/api/queue/retry-failed")
    suspend fun retryFailed(): Response<ResponseBody>

    @GET("/api/scan")
    suspend fun scanUrl(
        @Query("url") url: String,
        @Query("start") start: Int = 1,
        @Query("limit") limit: Int = 50
    ): ScanResponse

    @GET("/api/browser/sniff")
    suspend fun browserSniff(@Query("url") url: String): BrowserSniffResponse

    @GET("/api/gallery")
    suspend fun getGallery(
        @Query("sort") sort: String = "newest",
        @Query("category") category: String? = null,
        @Query("folder") folder: String? = null,
        @Query("search") search: String? = null
    ): GalleryResponse

    @POST("/api/gallery/organize")
    suspend fun organizeLibrary(): GalleryOrganizeResponse

    @DELETE("/api/gallery/file")
    suspend fun deleteGalleryFile(@Query("path") path: String): Response<ResponseBody>

    @POST("/api/gallery/delete-multiple")
    suspend fun deleteMultipleGalleryFiles(@Body body: DeleteMultipleRequest): DeleteMultipleResponse

    @GET("/api/settings/storage")
    suspend fun getStorageSettings(): StorageSettings

    @POST("/api/settings/storage")
    suspend fun updateStorageSettings(@Body body: StorageUpdateRequest): StorageSettings

    @GET("/api/ytdlp/version")
    suspend fun getYtDlpVersion(): YtDlpVersionInfo

    @POST("/api/ytdlp/update")
    suspend fun updateYtDlpChannel(@Query("channel") channel: String): UpdateYtdlpResponse

    @GET("/api/queue/workers")
    suspend fun getWorkers(): WorkersResponse

    @POST("/api/queue/workers")
    suspend fun setWorkers(@Query("count") count: Int): Response<ResponseBody>
}
