package com.kvdownload.app

import com.kvdownload.app.api.AV01Resolver
import com.kvdownload.app.storage.HlsSegmentDownloader
import com.kvdownload.app.ui.adaptive.FoldPosture
import com.kvdownload.app.ui.adaptive.HingeRectDp
import com.kvdownload.app.ui.adaptive.WidthClass
import com.kvdownload.app.ui.adaptive.computeWindowState
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.charset.StandardCharsets
import java.util.Base64

class MediaAndAdaptiveTest {

    @Test
    fun av01UrlParsingAndJwtCodeExtraction() {
        assertTrue(AV01Resolver.isAV01Url("https://av01.media/jp/video/226601/fc2-ppv-5011616"))
        assertTrue(AV01Resolver.isAV01Url("https://av01.tv/en/video/987654"))
        assertTrue(AV01Resolver.isAV01Url("https://customers.iw01.xyz/api/v1/videos/226601/manifest/master.m3u8"))
        assertFalse(AV01Resolver.isAV01Url("https://av01.media/"))
        assertFalse(AV01Resolver.isAV01Url("https://www.youtube.com/watch?v=dQw4w9WgXcQ"))

        val parsed = AV01Resolver.parseVideoIdAndSlug("https://av01.media/jp/video/226601/fc2-ppv-5011616")
        assertEquals("226601" to "fc2-ppv-5011616", parsed)

        val payloadJson = """{"sub":"/fmp4/20261121f/FC2-PPV-5011616/map/","exp":1800000000}"""
        val encPayload = Base64.getUrlEncoder().withoutPadding()
            .encodeToString(payloadJson.toByteArray(StandardCharsets.UTF_8))
        val fakeJwt = "eyJhbGciOiJIUzI1NiJ9.$encPayload.sig"
        assertEquals("FC2-PPV-5011616", AV01Resolver.extractCodeFromJwt(fakeJwt))
    }

    @Test
    fun av01MasterPlaylistParsingAndVariantSelection() {
        val master = """
            #EXTM3U
            #EXT-X-VERSION:6
            #EXT-X-STREAM-INF:BANDWIDTH=800000,RESOLUTION=640x360
            index360-sv3-v1-a1.m3u8
            #EXT-X-STREAM-INF:BANDWIDTH=2400000,RESOLUTION=1280x720
            index720-sv3-v1-a1.m3u8
            #EXT-X-STREAM-INF:BANDWIDTH=5200000,RESOLUTION=1920x1080
            index1080-sv3-v1-a1.m3u8
        """.trimIndent()

        val variants = AV01Resolver.parseMasterM3U8(
            content = master,
            manifestBaseDir = "https://customers.iw01.xyz/api/v1/videos/226601/manifest/",
            accessToken = "tok.abc"
        )
        assertEquals(3, variants.size)
        assertEquals("1080p", variants[0].label)
        assertEquals("720p", variants[1].label)
        assertEquals("360p", variants[2].label)
        assertTrue(variants[0].url.contains("index1080-sv3-v1-a1.m3u8?access_token="))

        assertEquals("1080p", AV01Resolver.selectVariant(variants, "highest").label)
        assertEquals("720p", AV01Resolver.selectVariant(variants, "720p").label)
        assertEquals("360p", AV01Resolver.selectVariant(variants, "480p").label)
    }

    @Test
    fun hlsMediaPlaylistParsingFmp4AndAesKey() {
        assertTrue(HlsSegmentDownloader.looksLikeHlsUrl("https://customers.iw01.xyz/api/v1/videos/226601/manifest/master.m3u8?access_token=xyz"))
        assertTrue(HlsSegmentDownloader.looksLikeHlsUrl("https://dl.khoavo.myds.me/api/browser/stream?url=https%3A%2F%2Fcdn.example.com%2Fmaster.m3u8%3Ftok%3D1"))
        assertFalse(HlsSegmentDownloader.looksLikeHlsUrl("https://dl.khoavo.myds.me/download?file=videos/sample.mp4&inline=true"))

        val playlist = """
            #EXTM3U
            #EXT-X-VERSION:6
            #EXT-X-MEDIA-SEQUENCE:10
            #EXT-X-KEY:METHOD=AES-128,URI="key.bin",IV=0x00000000000000000000000000000001
            #EXT-X-MAP:URI="init-sv3.mp4"
            #EXTINF:4.0,
            file-1.m4s
            #EXTINF:4.0,
            https://cdn.other.example/seg-2.m4s
        """.trimIndent()

        val spec = HlsSegmentDownloader.parseMediaPlaylist(
            playlistText = playlist,
            baseUrl = "https://customers.iw01.xyz/api/v1/videos/226601/manifest/index1080.m3u8?access_token=abc"
        )
        assertTrue(spec.isFmp4)
        assertNotNull(spec.initMapUrl)
        assertTrue(spec.initMapUrl!!.contains("init-sv3.mp4?access_token=abc"))
        assertEquals(2, spec.segments.size)
        assertTrue(spec.segments[0].url.contains("file-1.m4s?access_token=abc"))
        assertEquals("https://cdn.other.example/seg-2.m4s", spec.segments[1].url)
        assertNotNull(spec.segments[0].keyUrl)
        assertEquals(10L, spec.segments[0].mediaSequenceNumber)
        assertEquals(11L, spec.segments[1].mediaSequenceNumber)
    }

    @Test
    fun foldableAndTabletAdaptiveWindowStates() {
        val phonePortrait = computeWindowState(widthDp = 390, heightDp = 844)
        assertEquals(WidthClass.Compact, phonePortrait.widthClass)
        assertFalse(phonePortrait.useNavigationRail)
        assertFalse(phonePortrait.isTwoPane)
        assertEquals(1, phonePortrait.queueGridColumns)
        assertEquals(1, phonePortrait.galleryColumns)

        val foldableTabletop = computeWindowState(
            widthDp = 673,
            heightDp = 841,
            isHalfOpened = true,
            isHorizontalHinge = true,
            hingeBoundsDp = HingeRectDp(0, 420, 673, 421)
        )
        assertEquals(FoldPosture.Tabletop, foldableTabletop.foldPosture)
        assertTrue(foldableTabletop.isTabletop)
        assertFalse(foldableTabletop.isBook)

        val foldableBook = computeWindowState(
            widthDp = 841,
            heightDp = 673,
            isHalfOpened = true,
            isHorizontalHinge = false,
            hingeBoundsDp = HingeRectDp(420, 0, 421, 673)
        )
        assertEquals(FoldPosture.Book, foldableBook.foldPosture)
        assertTrue(foldableBook.isBook)
        assertTrue(foldableBook.isTwoPane)
        assertTrue(foldableBook.useNavigationRail)

        val tabletLandscape = computeWindowState(widthDp = 1280, heightDp = 800)
        assertEquals(WidthClass.Expanded, tabletLandscape.widthClass)
        assertTrue(tabletLandscape.useNavigationRail)
        assertTrue(tabletLandscape.isTwoPane)
        assertEquals(3, tabletLandscape.queueGridColumns)
        assertEquals(4, tabletLandscape.galleryColumns)
    }
}
