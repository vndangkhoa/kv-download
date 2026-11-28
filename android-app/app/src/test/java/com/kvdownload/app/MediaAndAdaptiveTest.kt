package com.kvdownload.app

import android.media.MediaCodec
import android.media.MediaExtractor
import com.kvdownload.app.api.AV01Resolver
import com.kvdownload.app.api.DownloadTask
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.XStatusRef
import com.kvdownload.app.api.XTwitterResolver
import com.kvdownload.app.storage.HlsSegmentDownloader
import com.kvdownload.app.ui.adaptive.FoldPosture
import com.kvdownload.app.ui.adaptive.HingeRectDp
import com.kvdownload.app.ui.adaptive.WidthClass
import com.kvdownload.app.ui.adaptive.computeWindowState
import com.kvdownload.app.ui.screens.detectPlayablePageItem
import com.kvdownload.app.ui.screens.extractUrlsFromText
import com.kvdownload.app.ui.screens.normalizeAv01QuickInput
import com.kvdownload.app.ui.screens.normalizeBrowserOmniboxUrl
import com.kvdownload.app.ui.screens.resolveSafeWebNavigationUrl
import com.kvdownload.app.update.AppUpdateManager
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

    @Test
    fun updateVersionComparisonIgnoresSpkTagsAndComparesSemver() {
        assertTrue(AppUpdateManager.isAppReleaseTag("1.0.43"))
        assertTrue(AppUpdateManager.isAppReleaseTag("v1.0.44"))
        assertFalse(AppUpdateManager.isAppReleaseTag("v1.0.0-68"))
        assertFalse(AppUpdateManager.isAppReleaseTag("1.0.0-69"))

        assertTrue(AppUpdateManager.isNewerVersion("v1.0.44", "1.0.43"))
        assertTrue(AppUpdateManager.isNewerVersion("1.1.0", "1.0.43"))
        assertFalse(AppUpdateManager.isNewerVersion("1.0.43", "1.0.43"))
        assertFalse(AppUpdateManager.isNewerVersion("1.0.42", "1.0.43"))
        assertFalse(AppUpdateManager.isNewerVersion("v1.0.0-69", "1.0.43"))
    }

    @Test
    fun downloadTaskUrlAndThumbnailIncludeMediaIdFallback() {
        RetrofitClient.configure("https://dl.khoavo.myds.me")
        val streamUrl = RetrofitClient.downloadTaskUrl(
            taskId = "502f2593-0bce-4df0-9dc0-80e5019e64c1",
            inline = true,
            mediaId = "3faf55e3913cb4054bdbf3aba9f2487c/sample reel.mp4"
        )
        assertTrue(streamUrl.contains("/download?id=502f2593-0bce-4df0-9dc0-80e5019e64c1"))
        assertTrue(streamUrl.contains("mediaId=3faf55e3913cb4054bdbf3aba9f2487c%2Fsample%20reel.mp4"))
        assertTrue(streamUrl.endsWith("&inline=true"))

        val task = DownloadTask(
            id = "502f2593-0bce-4df0-9dc0-80e5019e64c1",
            mediaId = "3faf55e3913cb4054bdbf3aba9f2487c/sample reel.mp4"
        )
        val thumbUrl = RetrofitClient.taskThumbnailUrl(task)
        assertTrue(thumbUrl.contains("/thumbnail?id=502f2593-0bce-4df0-9dc0-80e5019e64c1"))
        assertTrue(thumbUrl.contains("mediaId=3faf55e3913cb4054bdbf3aba9f2487c%2Fsample%20reel.mp4"))
    }

    @Test
    fun androidBrowserOmniboxAndSchemeFiltering() {
        assertEquals("https://www.google.com", normalizeBrowserOmniboxUrl(""))
        assertEquals("https://youtube.com", normalizeBrowserOmniboxUrl("youtube.com"))
        assertEquals(
            "https://www.google.com/search?q=lofi+beats",
            normalizeBrowserOmniboxUrl("lofi beats")
        )
        assertEquals(
            "https://www.google.com/search?q=tiktok",
            normalizeBrowserOmniboxUrl("tiktok")
        )

        assertEquals(null, resolveSafeWebNavigationUrl("https://www.tiktok.com/@creator/video/123"))
        assertEquals("", resolveSafeWebNavigationUrl("snssdk1180://aweme/detail/123"))
        assertEquals("", resolveSafeWebNavigationUrl("vnd.youtube://dQw4w9WgXcQ"))
        assertEquals(
            "https://www.tiktok.com/t/ZP8/",
            resolveSafeWebNavigationUrl("intent://aweme/detail/123#Intent;scheme=snssdk1180;S.browser_fallback_url=https%3A%2F%2Fwww.tiktok.com%2Ft%2FZP8%2F;end")
        )

        val ytItem = detectPlayablePageItem("https://m.youtube.com/watch?v=dQw4w9WgXcQ", "Rick Astley")
        assertNotNull(ytItem)
        assertEquals("https://www.youtube.com/watch?v=dQw4w9WgXcQ", ytItem!!.url)
        assertEquals("YOUTUBE", ytItem.format)

        val ttItem = detectPlayablePageItem("https://www.tiktok.com/@user/video/7400000000000000001", "Dance")
        assertNotNull(ttItem)
        assertEquals("TIKTOK", ttItem!!.format)

        assertEquals("https://av01.media/jp/video/226601", normalizeAv01QuickInput("226601"))
        assertEquals(
            "https://av01.media/jp/video/226601/fc2-ppv-5011616",
            normalizeAv01QuickInput("https://av01.media/jp/video/226601/fc2-ppv-5011616")
        )
        assertEquals(null, normalizeAv01QuickInput("https://av01.media/"))
        assertEquals(null, normalizeAv01QuickInput("abc"))
    }

    @Test
    fun remuxFlagTranslationAndMonotonicPts() {
        assertEquals(
            MediaCodec.BUFFER_FLAG_KEY_FRAME,
            HlsSegmentDownloader.translateSampleFlags(MediaExtractor.SAMPLE_FLAG_SYNC)
        )
        assertEquals(
            MediaCodec.BUFFER_FLAG_KEY_FRAME,
            HlsSegmentDownloader.translateSampleFlags(
                MediaExtractor.SAMPLE_FLAG_SYNC or MediaExtractor.SAMPLE_FLAG_PARTIAL_FRAME
            )
        )
        assertEquals(
            0,
            HlsSegmentDownloader.translateSampleFlags(MediaExtractor.SAMPLE_FLAG_PARTIAL_FRAME)
        )

        assertEquals(0L, HlsSegmentDownloader.normalizeTrackPtsUs(5_000L, 5_000L, -1L))
        assertEquals(33_333L, HlsSegmentDownloader.normalizeTrackPtsUs(38_333L, 5_000L, 0L))
        assertEquals(33_334L, HlsSegmentDownloader.normalizeTrackPtsUs(38_333L, 5_000L, 33_333L))
        assertEquals(33_335L, HlsSegmentDownloader.normalizeTrackPtsUs(1_000L, 5_000L, 33_334L))
    }

    @Test
    fun xTwitterUrlParsingAndShareTextExtraction() {
        val sample = "https://x.com/qqzzllyy/status/2126583986168488147?a=6E90jXRCBI7YPx1mZbo&s=19"
        assertTrue(XTwitterResolver.isXStatusUrl(sample))
        val parsed = XTwitterResolver.parseStatusUrl(sample)
        assertEquals(XStatusRef("qqzzllyy", "2126583986168488147", 0), parsed)

        assertEquals(
            XStatusRef("i", "2126583986168488147", 2),
            XTwitterResolver.parseStatusUrl("https://mobile.x.com/i/web/status/2126583986168488147/video/2")
        )
        assertEquals(
            XStatusRef("jack", "1234567890", 0),
            XTwitterResolver.parseStatusUrl("https://twitter.com/jack/statuses/1234567890")
        )
        assertTrue(XTwitterResolver.isXStatusUrl("https://fxtwitter.com/qqzzllyy/status/2126583986168488147"))
        assertTrue(XTwitterResolver.isXStatusUrl("https://vxtwitter.com/qqzzllyy/status/2126583986168488147"))
        assertFalse(XTwitterResolver.isXStatusUrl("https://x.com/home"))
        assertFalse(XTwitterResolver.isXStatusUrl("https://x.com/qqzzllyy"))

        val extracted = extractUrlsFromText("Check this video! $sample, really cool!")
        assertEquals(listOf(sample), extracted)

        val detected = detectPlayablePageItem(sample, "X Post")
        assertNotNull(detected)
        assertEquals("X VIDEO", detected!!.format)
        assertEquals("X / Twitter", detected.channel)
    }

    @Test
    fun xTwitterFxAndVxJsonParsingAndVariantSelection() {
        val fxJson = """
            {
              "code": 200,
              "tweet": {
                "url": "https://x.com/qqzzllyy/status/2126583986168488147",
                "id": "2126583986168488147",
                "text": "Sample tweet https://t.co/RZEEUrFil1",
                "author": {"name": "Author", "screen_name": "qqzzllyy"},
                "media": {
                  "videos": [{
                    "id": "2126583831310548992",
                    "url": "https://video.twimg.com/amplify_video/2126583831310548992/vid/avc1/448x796/7BA28qwJLCkMsM2h.mp4?tag=29",
                    "thumbnail_url": "https://pbs.twimg.com/thumb.jpg",
                    "duration": 46.788,
                    "width": 448,
                    "height": 796,
                    "type": "video",
                    "variants": [
                      {"url": "https://video.twimg.com/amplify_video/2126583831310548992/pl/NCL4wt0QLwK-n-Xn.m3u8?tag=29", "bitrate": 0, "content_type": "application/x-mpegURL"},
                      {"url": "https://video.twimg.com/amplify_video/2126583831310548992/vid/avc1/320x568/awFr8Ml0ZgPFns4O.mp4?tag=29", "bitrate": 632000, "content_type": "video/mp4"},
                      {"url": "https://video.twimg.com/amplify_video/2126583831310548992/vid/avc1/448x796/7BA28qwJLCkMsM2h.mp4?tag=29", "bitrate": 950000, "content_type": "video/mp4"}
                    ]
                  }]
                }
              }
            }
        """.trimIndent()

        val ref = XStatusRef("qqzzllyy", "2126583986168488147", 0)
        val parsed = XTwitterResolver.parseFxTwitterJson(fxJson, ref)
        assertEquals("@qqzzllyy", parsed.channel)
        assertEquals("Sample tweet", parsed.title)
        assertEquals(1, parsed.videos.size)
        val variants = parsed.videos[0].variants
        assertEquals(3, variants.size)
        assertEquals("448p", variants[0].label)
        assertEquals("320p", variants[1].label)
        assertEquals("HLS", variants[2].label)

        assertEquals("448p", XTwitterResolver.selectVariant(variants, "highest").label)
        assertEquals("448p", XTwitterResolver.selectVariant(variants, "720p").label)
        assertEquals("320p", XTwitterResolver.selectVariant(variants, "360p").label)

        val vxJson = """
            {
              "tweetID": "2126583986168488147",
              "text": "Quote fallback",
              "user_name": "Quoter",
              "user_screen_name": "quoter",
              "media_extended": [],
              "qrt": {
                "media_extended": [{
                  "duration_millis": 12500,
                  "id_str": "999",
                  "size": {"width": 720, "height": 1280},
                  "thumbnail_url": "https://pbs.twimg.com/q.jpg",
                  "type": "gif",
                  "url": "https://video.twimg.com/ext_tw_video/999/pu/vid/720x1280/q.mp4"
                }]
              }
            }
        """.trimIndent()
        val vxParsed = XTwitterResolver.parseVxTwitterJson(vxJson, ref)
        assertEquals(1, vxParsed.videos.size)
        assertTrue(vxParsed.videos[0].isGif)
        assertEquals(12.5, vxParsed.videos[0].durationSec, 0.01)
        assertEquals("720p", vxParsed.videos[0].variants[0].label)
    }
}
