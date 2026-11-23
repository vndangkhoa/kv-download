package com.kvdownload.app

import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.storage.PhoneDownloadManager
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class StorageOrganizationTest {
    @Test
    fun normalizeUrlHandlesTrailingSlashAndScheme() {
        assertEquals("https://dl.khoavo.myds.me", RetrofitClient.normalizeUrl("https://dl.khoavo.myds.me/"))
        assertEquals("http://192.168.1.50:9292", RetrofitClient.normalizeUrl("http://192.168.1.50:9292/"))
        assertEquals("https://example.com", RetrofitClient.normalizeUrl("example.com/"))
        assertEquals(RetrofitClient.DEFAULT_SERVER_URL, RetrofitClient.normalizeUrl("   "))
    }

    @Test
    fun formatFileSizeFormatsReadableUnits() {
        assertEquals("0 B", PhoneDownloadManager.formatFileSize(0L))
        assertEquals("512 B", PhoneDownloadManager.formatFileSize(512L))
        assertEquals("2.0 KB", PhoneDownloadManager.formatFileSize(2048L))
        assertEquals("1.5 MB", PhoneDownloadManager.formatFileSize(1572864L))
    }

    @Test
    fun directMediaLinkRoutingSeparatesWebPagesFromStreams() {
        assertFalse(PhoneDownloadManager.isDirectMediaLink("https://www.youtube.com/watch?v=dQw4w9WgXcQ"))
        assertFalse(PhoneDownloadManager.isDirectMediaLink("https://www.tiktok.com/@creator/video/7400000000000000001"))
        assertFalse(PhoneDownloadManager.isDirectMediaLink("https://tinhte.vn/thread/123"))

        assertTrue(PhoneDownloadManager.isDirectMediaLink("https://cdn.example.com/video/clip.mp4?token=abc"))
        assertTrue(PhoneDownloadManager.isDirectMediaLink("https://cdn.example.com/hls/master.m3u8"))
        assertTrue(PhoneDownloadManager.isDirectMediaLink("https://v16-webapp-prime.tiktokcdn.com/video/tos/123?mime_type=video_mp4"))
        assertTrue(PhoneDownloadManager.isDirectMediaLink("https://dl.khoavo.myds.me/download?id=abc"))
        assertTrue(PhoneDownloadManager.isDirectMediaLink("https://dl.khoavo.myds.me/api/browser/stream?url=https%3A%2F%2Fyoutube.com"))
    }
}
