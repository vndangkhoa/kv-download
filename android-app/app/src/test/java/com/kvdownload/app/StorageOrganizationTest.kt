package com.kvdownload.app

import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.storage.PhoneDownloadManager
import org.junit.Assert.assertEquals
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
}
