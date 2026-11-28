package com.kvdownload.app.ui.screens

import android.annotation.SuppressLint
import android.graphics.Bitmap
import android.net.Uri
import android.view.View
import android.view.ViewGroup
import android.webkit.CookieManager
import android.webkit.JavascriptInterface
import android.webkit.WebChromeClient
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.BasicTextField
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.CloudUpload
import androidx.compose.material.icons.filled.DesktopWindows
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.Lock
import androidx.compose.material.icons.filled.PhoneAndroid
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Radar
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Shield
import androidx.compose.material.icons.filled.WifiOff
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.SolidColor
import androidx.compose.ui.graphics.toArgb
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import coil.compose.AsyncImage
import com.kvdownload.app.api.AV01ResolvedMedia
import com.kvdownload.app.api.AV01Resolver
import com.kvdownload.app.api.EnqueueItemInput
import com.kvdownload.app.api.QueueAddRequest
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.SniffedMedia
import com.kvdownload.app.api.XResolvedTweet
import com.kvdownload.app.api.XTwitterResolver
import com.kvdownload.app.player.MediaKind
import com.kvdownload.app.player.MediaPlayRequest
import com.kvdownload.app.storage.HlsSegmentDownloader
import com.kvdownload.app.storage.PhoneDownloadManager
import com.kvdownload.app.ui.adaptive.LocalWindowState
import com.kvdownload.app.ui.components.AstryxCard
import com.kvdownload.app.ui.components.AstryxGradientButton
import com.kvdownload.app.ui.components.AstryxStatusBadge
import com.kvdownload.app.ui.components.AstryxTone
import com.kvdownload.app.ui.theme.LocalAstryxPalette
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import okhttp3.Request
import org.json.JSONObject
import java.io.ByteArrayInputStream
import java.net.URLDecoder
import java.net.URLEncoder
import java.util.Collections
import java.util.concurrent.TimeUnit

internal fun normalizeBrowserOmniboxUrl(raw: String): String {
    val trimmed = raw.trim()
    if (trimmed.isBlank()) return "https://www.google.com"
    val lower = trimmed.lowercase()
    if (lower.startsWith("http://") || lower.startsWith("https://")) return trimmed
    if (trimmed.contains(" ") || (!trimmed.contains(".") && !lower.startsWith("localhost"))) {
        return "https://www.google.com/search?q=" + URLEncoder.encode(trimmed, "UTF-8")
    }
    return "https://$trimmed"
}

internal fun normalizeAv01QuickInput(raw: String): String? {
    val trimmed = raw.trim()
    if (trimmed.isBlank()) return null
    if (AV01Resolver.isAV01Url(trimmed)) return trimmed
    if (trimmed.all { it.isDigit() } && trimmed.length >= 3) {
        return "https://av01.media/jp/video/$trimmed"
    }
    return null
}

/**
 * Resolves custom non-HTTP URL schemes inside Android WebView so TikTok / YouTube intent links
 * never crash with ERR_UNKNOWN_URL_SCHEME. Returns:
 * - null -> regular HTTP(S) URL, let WebView proceed normally
 * - non-empty HTTP(S) string -> extracted intent fallback URL to load inside WebView
 * - "" (empty string) -> blocked custom scheme (handle with return true)
 */
internal fun resolveSafeWebNavigationUrl(rawUrl: String?): String? {
    val u = rawUrl?.trim().orEmpty()
    if (u.isEmpty()) return ""
    val lower = u.lowercase()
    if (lower.startsWith("http://") || lower.startsWith("https://") || lower.startsWith("about:blank")) {
        return null
    }
    if (lower.startsWith("intent://")) {
        val marker = "S.browser_fallback_url="
        val idx = u.indexOf(marker)
        if (idx >= 0) {
            val encoded = u.substring(idx + marker.length).substringBefore(";")
            val decoded = runCatching { URLDecoder.decode(encoded, "UTF-8") }.getOrDefault("")
            if (decoded.startsWith("http://") || decoded.startsWith("https://")) {
                return decoded
            }
        }
    }
    return ""
}

private val ANDROID_YT_RE = Regex("""(?:youtube\.com/watch\?[^#]*v=|youtu\.be/|youtube\.com/shorts/)([a-zA-Z0-9_-]{11})""")
private val ANDROID_TT_RE = Regex("""tiktok\.com/(?:@[^/?#]+/video/\d+|t/[a-zA-Z0-9]+)""", RegexOption.IGNORE_CASE)

internal fun detectPlayablePageItem(rawUrl: String, pageTitle: String?): SniffedMedia? {
    val trimmed = rawUrl.trim()
    if (!trimmed.startsWith("http://") && !trimmed.startsWith("https://")) return null
    val ytMatch = ANDROID_YT_RE.find(trimmed)
    if (ytMatch != null) {
        val vid = ytMatch.groupValues[1]
        val title = pageTitle?.takeIf { it.isNotBlank() && !it.equals("YouTube", ignoreCase = true) }
            ?: "YouTube Video $vid"
        return SniffedMedia(
            id = "yt_$vid",
            url = "https://www.youtube.com/watch?v=$vid",
            type = "video",
            format = "YOUTUBE",
            title = title,
            thumbnail = "https://i.ytimg.com/vi/$vid/hqdefault.jpg",
            source = "webview_page",
            channel = "YouTube"
        )
    }
    if (XTwitterResolver.isXStatusUrl(trimmed)) {
        val cleanTitle = pageTitle?.takeIf {
            it.isNotBlank() && !it.equals("X", ignoreCase = true) && !it.equals("Twitter", ignoreCase = true)
        } ?: "X / Twitter Video"
        return SniffedMedia(
            id = "x_${trimmed.hashCode()}",
            url = trimmed,
            type = "video",
            format = "X VIDEO",
            title = cleanTitle,
            source = "webview_page",
            channel = "X / Twitter"
        )
    }
    if (ANDROID_TT_RE.containsMatchIn(trimmed)) {
        return SniffedMedia(
            id = "tt_${trimmed.hashCode()}",
            url = trimmed,
            type = "video",
            format = "TIKTOK",
            title = pageTitle?.takeIf { it.isNotBlank() } ?: "TikTok Video",
            source = "webview_page",
            channel = "TikTok"
        )
    }
    return null
}

private const val DESKTOP_CHROME_UA =
    "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"

private const val INJECTED_SNIFFER_JS = """
(function() {
  if (window.__kvAndroidSnifferInstalled) return;
  window.__kvAndroidSnifferInstalled = true;

  try {
    var origOpenWin = window.open;
    window.open = function(u) {
      if (u && typeof u === 'string' && /^https?:/i.test(u)) {
        location.href = u;
        return window;
      }
      return origOpenWin ? origOpenWin.apply(this, arguments) : null;
    };
  } catch (e) {}

  function emit(url, type, fmt, thumb, title) {
    if (!url || typeof url !== 'string') return;
    if (url.indexOf('data:application/vnd.apple.mpegurl;base64,') === 0 || url.indexOf('data:audio/mpegurl;base64,') === 0) {
      try {
        var b64 = url.split(',')[1] || '';
        var decoded = atob(b64);
        var urls = decoded.match(/https?:\/\/[^\s"'<>]+/g) || [];
        urls.forEach(function(m) { emit(m, 'video', 'M3U8 HLS', thumb, title); });
      } catch (e) {}
      return;
    }
    if (url.startsWith('data:') || url.startsWith('blob:')) return;
    try {
      window.KVSniffer.onMediaFound(JSON.stringify({
        url: url,
        type: type || 'video',
        format: fmt || '',
        thumbnail: thumb || '',
        title: title || document.title || '',
        referer: location.href || ''
      }));
    } catch (e) {}
  }

  function scanDom() {
    var title = document.title || '';
    document.querySelectorAll('video, audio').forEach(function(el) {
      var kind = el.tagName === 'AUDIO' ? 'audio' : 'video';
      var poster = el.getAttribute('poster') || '';
      if (el.currentSrc) emit(el.currentSrc, kind, '', poster, title);
      if (el.src) emit(el.src, kind, '', poster, title);
      el.querySelectorAll('source').forEach(function(s) {
        if (s.src) emit(s.src, kind, s.type || '', poster, title);
      });
    });

    document.querySelectorAll('img').forEach(function(img) {
      var w = img.naturalWidth || 0;
      var h = img.naturalHeight || 0;
      if (w >= 360 && h >= 240 && img.src) {
        emit(img.src, 'image', 'IMAGE', img.src, img.alt || title);
      }
    });
  }

  var origFetch = window.fetch;
  window.fetch = function() {
    try {
      var reqUrl = typeof arguments[0] === 'string' ? arguments[0] : (arguments[0] && arguments[0].url);
      if (reqUrl && /\.(m3u8|mpd|mp4|webm|mp3|m4a|aac)(\?|$)/i.test(reqUrl)) {
        emit(reqUrl, /\.(mp3|m4a|aac)/i.test(reqUrl) ? 'audio' : 'video', /\.m3u8/i.test(reqUrl) ? 'M3U8 HLS' : 'STREAM', '', document.title);
      }
    } catch (e) {}
    return origFetch.apply(this, arguments);
  };

  var origOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url) {
    try {
      if (typeof url === 'string' && /\.(m3u8|mpd|mp4|webm|mp3|m4a|aac)(\?|$)/i.test(url)) {
        emit(url, /\.(mp3|m4a|aac)/i.test(url) ? 'audio' : 'video', /\.m3u8/i.test(url) ? 'M3U8 HLS' : 'STREAM', '', document.title);
      }
    } catch (e) {}
    return origOpen.apply(this, arguments);
  };

  setTimeout(scanDom, 800);
  setInterval(scanDom, 3000);
})();
"""

private data class PendingBrowserDownload(
    val label: String,
    val items: List<SniffedMedia>,
    val canonicalServerUrl: String? = null,
    val isBatch: Boolean = false
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun InAppBrowserScreen(
    phoneDownloadManager: PhoneDownloadManager,
    onRefreshQueue: () -> Unit,
    onPlayMedia: (MediaPlayRequest) -> Unit,
    onShowMessage: (String) -> Unit
) {
    val scope = rememberCoroutineScope()
    val windowState = LocalWindowState.current
    val palette = LocalAstryxPalette.current

    var addressInput by remember { mutableStateOf("") }
    var currentUrl by remember { mutableStateOf("") }
    var useServerProxy by remember { mutableStateOf(false) }
    var desktopMode by remember { mutableStateOf(false) }
    var defaultMobileUa by remember { mutableStateOf<String?>(null) }

    var isLoadingPage by remember { mutableStateOf(false) }
    var pageLoadProgress by remember { mutableIntStateOf(0) }
    var pageCommitted by remember { mutableStateOf(false) }
    var pageErrorText by remember { mutableStateOf<String?>(null) }
    var pageLoadTimedOut by remember { mutableStateOf(false) }
    var loadNonce by remember { mutableIntStateOf(0) }

    var isSniffingServer by remember { mutableStateOf(false) }
    var webViewRef by remember { mutableStateOf<WebView?>(null) }
    var customFullscreenView by remember { mutableStateOf<View?>(null) }
    var customFullscreenCallback by remember { mutableStateOf<WebChromeClient.CustomViewCallback?>(null) }
    var canGoBack by remember { mutableStateOf(false) }
    var canGoForward by remember { mutableStateOf(false) }

    val dohFallbackHosts = remember { Collections.synchronizedSet(mutableSetOf<String>()) }

    val sniffedItems = remember { mutableStateListOf<SniffedMedia>() }
    var resolvedAV01 by remember { mutableStateOf<AV01ResolvedMedia?>(null) }
    var resolvedX by remember { mutableStateOf<XResolvedTweet?>(null) }
    var av01QuickInput by remember { mutableStateOf("") }
    var isResolvingQuickAv01 by remember { mutableStateOf(false) }

    var showSnifferSheet by remember { mutableStateOf(false) }
    var sniffTab by remember { mutableStateOf("video") }
    var selectedUrls by remember { mutableStateOf<Set<String>>(emptySet()) }
    var formatQuality by remember { mutableStateOf("highest") }
    var pendingDownload by remember { mutableStateOf<PendingBrowserDownload?>(null) }

    fun mergeSniffed(incoming: List<SniffedMedia>) {
        incoming.forEach { item ->
            if (item.url.isNotBlank() && sniffedItems.none { it.url == item.url }) {
                sniffedItems.add(item)
            }
        }
        sniffedItems.sortBy {
            when (it.type) {
                "video" -> if (it.isStream == true) 0 else 1
                "audio" -> 2
                else -> 3
            }
        }
    }

    fun resolveAV01DirectOnPhone(target: String, onResolvedCallback: ((AV01ResolvedMedia) -> Unit)? = null) {
        if (!AV01Resolver.isAV01Url(target)) return
        isSniffingServer = true
        scope.launch {
            try {
                val av = AV01Resolver.resolve(target)
                resolvedAV01 = av
                val avItems = av.variants.map { variant ->
                    SniffedMedia(
                        id = "av01_${av.videoId}_${variant.label}",
                        url = variant.url,
                        type = "video",
                        format = "${variant.label} M3U8 HLS",
                        title = "${av.title} (${variant.label})",
                        thumbnail = av.thumbnail,
                        source = "av01_phone",
                        resolution = variant.label,
                        channel = "AV01",
                        isStream = true,
                        referer = av.referer,
                        variants = av.variants
                    )
                }
                mergeSniffed(avItems)
                onResolvedCallback?.invoke(av)
            } catch (e: Exception) {
                if (onResolvedCallback != null) {
                    onShowMessage(e.message ?: "Unable to resolve AV01 stream")
                }
            } finally {
                isSniffingServer = false
            }
        }
    }

    fun resolveXDirectOnPhone(target: String, onResolvedCallback: ((XResolvedTweet) -> Unit)? = null) {
        if (!XTwitterResolver.isXStatusUrl(target)) return
        isSniffingServer = true
        scope.launch {
            try {
                val xt = XTwitterResolver.resolve(target)
                resolvedX = xt
                val statusRef = XTwitterResolver.parseStatusUrl(target)
                val videosToEmit = if (statusRef != null && statusRef.videoIndex > 0) {
                    listOfNotNull(XTwitterResolver.selectVideo(xt, statusRef.videoIndex))
                } else {
                    xt.videos
                }
                val xItems = videosToEmit.flatMap { vid ->
                    vid.variants.map { variant ->
                        val isHls = HlsSegmentDownloader.looksLikeHlsUrl(variant.url)
                        SniffedMedia(
                            id = "x_${vid.id}_${variant.label}",
                            url = variant.url,
                            type = "video",
                            format = if (isHls) "M3U8 HLS" else "${variant.label} MP4",
                            title = "${vid.title} (${variant.label})",
                            thumbnail = vid.thumbnail ?: xt.thumbnail,
                            source = "x_phone",
                            resolution = variant.label,
                            channel = xt.channel,
                            isStream = isHls,
                            referer = xt.referer,
                            variants = vid.variants
                        )
                    }
                }
                mergeSniffed(xItems)
                onResolvedCallback?.invoke(xt)
            } catch (e: Exception) {
                if (onResolvedCallback != null) {
                    onShowMessage(e.message ?: "Unable to resolve X video")
                }
            } finally {
                isSniffingServer = false
            }
        }
    }

    fun triggerServerSniff(target: String) {
        if (target.isBlank()) return
        isSniffingServer = true
        scope.launch {
            try {
                val res = RetrofitClient.getApi().browserSniff(target)
                val skipOverride = (AV01Resolver.isAV01Url(target) && resolvedAV01 != null) ||
                    (XTwitterResolver.isXStatusUrl(target) && resolvedX != null)
                if (!skipOverride) {
                    mergeSniffed(res.items.map {
                        it.copy(referer = it.referer ?: target)
                    })
                }
            } catch (_: Exception) {
            } finally {
                isSniffingServer = false
            }
        }
    }

    fun loadTarget(raw: String, forceProxy: Boolean = useServerProxy) {
        val clean = normalizeBrowserOmniboxUrl(raw)
        addressInput = clean
        currentUrl = clean
        isLoadingPage = true
        pageLoadProgress = 8
        pageCommitted = false
        pageErrorText = null
        pageLoadTimedOut = false
        loadNonce++
        sniffedItems.clear()
        resolvedAV01 = null
        resolvedX = null
        selectedUrls = emptySet()

        detectPlayablePageItem(clean, null)?.let { mergeSniffed(listOf(it)) }
        if (AV01Resolver.isAV01Url(clean)) {
            resolveAV01DirectOnPhone(clean)
        } else if (XTwitterResolver.isXStatusUrl(clean)) {
            resolveXDirectOnPhone(clean)
        } else {
            triggerServerSniff(clean)
        }

        val loadUrl = if (forceProxy) RetrofitClient.browserProxyUrl(clean) else clean
        webViewRef?.loadUrl(loadUrl)
    }

    // Timeout watchdog: prevents blank/black WebView when ISP drops TLS handshake (e.g. av01.media)
    LaunchedEffect(currentUrl, loadNonce, pageCommitted, pageErrorText) {
        if (currentUrl.isBlank() || pageCommitted || pageErrorText != null) return@LaunchedEffect
        delay(12_000L)
        if (!pageCommitted && pageErrorText == null) {
            isLoadingPage = false
            pageLoadTimedOut = true
        }
    }

    fun openCurrentPageDownloadPicker() {
        val av = resolvedAV01
        if (av != null && av.variants.isNotEmpty()) {
            val picked = AV01Resolver.selectVariant(av.variants, formatQuality)
            pendingDownload = PendingBrowserDownload(
                label = av.title,
                items = listOf(
                    SniffedMedia(
                        id = "av01:${av.videoId}",
                        url = picked.url,
                        type = "video",
                        format = "${picked.label} M3U8 HLS",
                        title = av.title,
                        thumbnail = av.thumbnail,
                        source = "av01_phone",
                        resolution = picked.label,
                        channel = "AV01",
                        isStream = true,
                        referer = av.referer,
                        variants = av.variants
                    )
                ),
                canonicalServerUrl = currentUrl.ifBlank { "https://av01.media/jp/video/${av.videoId}" }
            )
            return
        }

        val xt = resolvedX
        val xRef = XTwitterResolver.parseStatusUrl(currentUrl)
        val xVid = if (xt != null) XTwitterResolver.selectVideo(xt, xRef?.videoIndex ?: 0) else null
        if (xt != null && xVid != null && xVid.variants.isNotEmpty()) {
            val picked = XTwitterResolver.selectVariant(xVid.variants, formatQuality)
            pendingDownload = PendingBrowserDownload(
                label = xVid.title,
                items = listOf(
                    SniffedMedia(
                        id = "x:${xt.tweetId}",
                        url = picked.url,
                        type = "video",
                        format = "${picked.label} MP4",
                        title = xVid.title,
                        thumbnail = xVid.thumbnail ?: xt.thumbnail,
                        source = "x_phone",
                        resolution = picked.label,
                        channel = xt.channel,
                        isStream = HlsSegmentDownloader.looksLikeHlsUrl(picked.url),
                        referer = xt.referer,
                        variants = xVid.variants
                    )
                ),
                canonicalServerUrl = currentUrl.ifBlank { xt.canonicalUrl }
            )
            return
        }

        val detectedVideo = sniffedItems.firstOrNull { it.type == "video" }
        if (detectedVideo != null) {
            pendingDownload = PendingBrowserDownload(
                label = detectedVideo.title.ifBlank { currentUrl },
                items = listOf(detectedVideo),
                canonicalServerUrl = when {
                    AV01Resolver.isAV01Url(currentUrl) -> currentUrl
                    XTwitterResolver.isXStatusUrl(currentUrl) -> currentUrl
                    else -> detectedVideo.url
                }
            )
            return
        }

        val pageUrl = currentUrl.ifBlank { return }
        pendingDownload = PendingBrowserDownload(
            label = webViewRef?.title?.takeIf { it.isNotBlank() } ?: pageUrl,
            items = listOf(
                SniffedMedia(
                    id = "page:${pageUrl.hashCode()}",
                    url = pageUrl,
                    type = "video",
                    format = "PAGE",
                    title = webViewRef?.title?.takeIf { it.isNotBlank() } ?: pageUrl,
                    referer = pageUrl
                )
            ),
            canonicalServerUrl = pageUrl
        )
    }

    fun executePendingDownloadToNas(req: PendingBrowserDownload) {
        pendingDownload = null
        scope.launch {
            try {
                if (req.isBatch && req.items.size > 1) {
                    RetrofitClient.getApi().addQueue(
                        QueueAddRequest(
                            items = req.items.map {
                                EnqueueItemInput(
                                    url = it.url,
                                    title = it.title,
                                    thumbnail = it.thumbnail,
                                    channel = it.channel,
                                    duration = it.duration,
                                    referer = it.referer ?: currentUrl.ifBlank { null }
                                )
                            },
                            format = formatQuality,
                            groupTitle = req.label,
                            groupType = "batch"
                        )
                    )
                    selectedUrls = emptySet()
                    showSnifferSheet = false
                    onRefreshQueue()
                    onShowMessage("Queued ${req.items.size} items to NAS server")
                } else {
                    val item = req.items.firstOrNull()
                    val serverUrl = req.canonicalServerUrl?.takeIf { it.isNotBlank() } ?: item?.url ?: return@launch
                    val serverFmt = item?.resolution?.lowercase()?.takeIf {
                        it in setOf("1080p", "720p", "480p", "360p")
                    } ?: formatQuality
                    RetrofitClient.getApi().addQueue(
                        QueueAddRequest(
                            items = listOf(
                                EnqueueItemInput(
                                    url = serverUrl,
                                    title = item?.title ?: req.label,
                                    thumbnail = item?.thumbnail,
                                    channel = item?.channel,
                                    duration = item?.duration,
                                    referer = item?.referer ?: currentUrl.ifBlank { null }
                                )
                            ),
                            format = serverFmt
                        )
                    )
                    onRefreshQueue()
                    onShowMessage("Queued on NAS server: ${req.label}")
                }
            } catch (e: Exception) {
                onShowMessage(e.message ?: "Failed to queue on NAS server")
            }
        }
    }

    fun executePendingDownloadToPhone(req: PendingBrowserDownload) {
        pendingDownload = null
        if (req.isBatch && req.items.size > 1) {
            phoneDownloadManager.saveBatchToPhone(
                items = req.items,
                formatPref = formatQuality,
                defaultReferer = currentUrl.ifBlank { null },
                groupName = req.label
            )
            selectedUrls = emptySet()
            showSnifferSheet = false
            onShowMessage("Saving ${req.items.size} items to Private App Vault...")
        } else {
            val item = req.items.firstOrNull() ?: return
            val effReferer = item.referer ?: currentUrl.ifBlank { null }
            val avPage = req.canonicalServerUrl?.takeIf { AV01Resolver.isAV01Url(it) }
            val xPage = req.canonicalServerUrl?.takeIf { XTwitterResolver.isXStatusUrl(it) }
                ?: item.url.takeIf { XTwitterResolver.isXStatusUrl(it) }
            if (avPage != null) {
                phoneDownloadManager.saveHlsOrAV01ToPhone(
                    url = avPage,
                    title = item.title.ifBlank { req.label },
                    formatPref = item.resolution ?: formatQuality,
                    channel = "AV01",
                    referer = effReferer ?: AV01Resolver.REFERER
                )
            } else if (xPage != null) {
                phoneDownloadManager.saveXVideoToPhone(
                    url = xPage,
                    title = item.title.ifBlank { req.label },
                    formatPref = item.resolution ?: formatQuality,
                    channel = item.channel,
                    referer = effReferer ?: XTwitterResolver.REFERER
                )
            } else {
                phoneDownloadManager.saveDirectMediaToPhone(
                    mediaUrl = item.url,
                    title = item.title.ifBlank { req.label },
                    type = item.type,
                    channel = item.channel,
                    referer = effReferer,
                    formatPref = item.resolution ?: formatQuality
                )
            }
            onShowMessage("Saving privately inside KV Download...")
        }
    }

    @Composable
    fun CompactOmniboxPill(modifier: Modifier = Modifier) {
        Surface(
            modifier = modifier.height(38.dp),
            shape = RoundedCornerShape(12.dp),
            color = palette.muted,
            border = BorderStroke(1.dp, palette.borderSubtle)
        ) {
            Row(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(horizontal = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(
                    imageVector = if (useServerProxy) Icons.Default.Shield else Icons.Default.Language,
                    contentDescription = null,
                    tint = palette.accent,
                    modifier = Modifier.size(16.dp)
                )
                Box(
                    modifier = Modifier.weight(1f),
                    contentAlignment = Alignment.CenterStart
                ) {
                    if (addressInput.isEmpty()) {
                        Text(
                            text = "Search or enter URL...",
                            style = MaterialTheme.typography.bodySmall,
                            color = palette.textSecondary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                    BasicTextField(
                        value = addressInput,
                        onValueChange = { addressInput = it },
                        singleLine = true,
                        textStyle = MaterialTheme.typography.bodySmall.copy(color = palette.textPrimary),
                        cursorBrush = SolidColor(palette.accent),
                        keyboardOptions = KeyboardOptions(imeAction = ImeAction.Go),
                        keyboardActions = KeyboardActions(
                            onGo = { if (addressInput.isNotBlank()) loadTarget(addressInput) }
                        ),
                        modifier = Modifier.fillMaxWidth()
                    )
                }
                if (addressInput.isNotEmpty()) {
                    Icon(
                        imageVector = Icons.Default.Close,
                        contentDescription = "Clear address",
                        tint = palette.textSecondary,
                        modifier = Modifier
                            .size(16.dp)
                            .clickable { addressInput = "" }
                    )
                }
                Icon(
                    imageVector = Icons.Default.Refresh,
                    contentDescription = "Go or Reload",
                    tint = if (addressInput.isNotBlank()) palette.accentText else palette.textSecondary,
                    modifier = Modifier
                        .size(18.dp)
                        .clickable {
                            val target = addressInput.ifBlank { currentUrl }
                            if (target.isNotBlank()) loadTarget(target)
                        }
                )
            }
        }
    }

    @Composable
    fun SnifferPanelContent(modifier: Modifier = Modifier, compactSheet: Boolean = false) {
        val visibleItems = sniffedItems.filter { sniffTab == "all" || it.type == sniffTab }

        Column(
            modifier = modifier
                .fillMaxWidth()
                .padding(horizontal = if (compactSheet) 16.dp else 12.dp)
                .padding(bottom = if (compactSheet) 24.dp else 12.dp),
            verticalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Icon(Icons.Default.Radar, contentDescription = null, tint = palette.accent, modifier = Modifier.size(18.dp))
                    Text("Live Sniffer (${sniffedItems.size})", style = MaterialTheme.typography.titleMedium, color = palette.textPrimary)
                }
                Row {
                    listOf("highest" to "Best", "1080p" to "1080p", "audio" to "MP3").forEach { (q, lbl) ->
                        FilterChip(
                            selected = formatQuality == q,
                            onClick = { formatQuality = q },
                            label = { Text(lbl) },
                            modifier = Modifier.padding(start = 4.dp)
                        )
                    }
                }
            }

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                listOf(
                    "video" to "Videos (${sniffedItems.count { it.type == "video" }})",
                    "audio" to "Audio (${sniffedItems.count { it.type == "audio" }})",
                    "image" to "Photos (${sniffedItems.count { it.type == "image" }})",
                    "all" to "All (${sniffedItems.size})"
                ).forEach { (tabKey, label) ->
                    FilterChip(
                        selected = sniffTab == tabKey,
                        onClick = { sniffTab = tabKey },
                        label = { Text(label) }
                    )
                }
            }

            if (visibleItems.isEmpty()) {
                AstryxCard {
                    Text(
                        text = if (isSniffingServer) {
                            "Resolving M3U8 streams and page media..."
                        } else {
                            "No streams detected yet. Play a video or paste a direct URL (such as av01.media/jp/video/...)."
                        },
                        style = MaterialTheme.typography.bodySmall,
                        color = palette.textSecondary
                    )
                }
            } else {
                LazyColumn(
                    modifier = if (compactSheet) Modifier.fillMaxWidth().height(340.dp) else Modifier.weight(1f).fillMaxWidth(),
                    verticalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    items(visibleItems, key = { it.url }) { item ->
                        val isSelected = selectedUrls.contains(item.url)
                        val isImage = item.type == "image" || item.type == "photo"
                        val effReferer = item.referer ?: currentUrl.ifBlank { null }
                        val itemIsHls = item.isStream == true || HlsSegmentDownloader.looksLikeHlsUrl(item.url)

                        Surface(
                            shape = RoundedCornerShape(12.dp),
                            color = palette.card,
                            border = BorderStroke(1.dp, if (isSelected) palette.accent else palette.borderSubtle)
                        ) {
                            Column(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .padding(10.dp),
                                verticalArrangement = Arrangement.spacedBy(8.dp)
                            ) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    Checkbox(
                                        checked = isSelected,
                                        onCheckedChange = {
                                            selectedUrls = if (isSelected) selectedUrls - item.url else selectedUrls + item.url
                                        }
                                    )
                                    if (!item.thumbnail.isNullOrBlank()) {
                                        AsyncImage(
                                            model = item.thumbnail,
                                            contentDescription = null,
                                            modifier = Modifier
                                                .size(64.dp, 40.dp)
                                                .clip(RoundedCornerShape(6.dp)),
                                            contentScale = ContentScale.Crop
                                        )
                                    }
                                    Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
                                        Text(
                                            text = item.title,
                                            style = MaterialTheme.typography.bodyMedium,
                                            fontWeight = FontWeight.SemiBold,
                                            color = palette.textPrimary,
                                            maxLines = 1,
                                            overflow = TextOverflow.Ellipsis
                                        )
                                        Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                            item.resolution?.let { AstryxStatusBadge(it, AstryxTone.Accent) }
                                            if (itemIsHls) AstryxStatusBadge("M3U8 HLS", AstryxTone.Info)
                                            else item.format?.let { AstryxStatusBadge(it, AstryxTone.Neutral) }
                                        }
                                    }
                                }

                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    OutlinedButton(
                                        onClick = {
                                            val kind = when {
                                                isImage -> MediaKind.IMAGE
                                                item.type == "audio" -> MediaKind.AUDIO
                                                else -> MediaKind.VIDEO
                                            }
                                            onPlayMedia(
                                                MediaPlayRequest(
                                                    id = "sniff:${item.url.hashCode()}",
                                                    title = item.title,
                                                    subtitle = item.channel ?: item.format,
                                                    thumbnailUrl = item.thumbnail,
                                                    primaryUrl = item.url,
                                                    fallbackUrl = if (isImage) null else RetrofitClient.browserStreamUrl(item.url, effReferer),
                                                    kind = kind,
                                                    isHls = itemIsHls,
                                                    referer = effReferer,
                                                    variants = item.variants.orEmpty(),
                                                    onSaveToPhone = {
                                                        phoneDownloadManager.saveDirectMediaToPhone(
                                                            mediaUrl = item.url,
                                                            title = item.title,
                                                            type = item.type,
                                                            channel = item.channel,
                                                            referer = effReferer,
                                                            formatPref = item.resolution ?: formatQuality
                                                        )
                                                        onShowMessage("Saving privately inside KV Download...")
                                                    }
                                                )
                                            )
                                        },
                                        modifier = Modifier.weight(1f)
                                    ) {
                                        Icon(
                                            imageVector = if (isImage) Icons.Default.Image else Icons.Default.PlayArrow,
                                            contentDescription = null,
                                            modifier = Modifier.size(16.dp)
                                        )
                                        Text(if (isImage) "View" else "Preview", modifier = Modifier.padding(start = 4.dp))
                                    }

                                    Button(
                                        onClick = {
                                            val canonicalUrl = when {
                                                AV01Resolver.isAV01Url(currentUrl) -> currentUrl
                                                XTwitterResolver.isXStatusUrl(currentUrl) -> currentUrl
                                                else -> item.url
                                            }
                                            pendingDownload = PendingBrowserDownload(
                                                label = item.title.ifBlank { canonicalUrl },
                                                items = listOf(item),
                                                canonicalServerUrl = canonicalUrl
                                            )
                                        },
                                        modifier = Modifier.weight(1f)
                                    ) {
                                        Icon(Icons.Default.Download, contentDescription = null, modifier = Modifier.size(16.dp))
                                        Text("Download", modifier = Modifier.padding(start = 4.dp))
                                    }
                                }
                            }
                        }
                    }
                }

                if (selectedUrls.isNotEmpty()) {
                    AstryxGradientButton(
                        text = "Download Selected (${selectedUrls.size})",
                        icon = Icons.Default.Download,
                        onClick = {
                            val chosen = sniffedItems.filter { selectedUrls.contains(it.url) }
                            if (chosen.isNotEmpty()) {
                                pendingDownload = PendingBrowserDownload(
                                    label = "Browser Batch (${chosen.size})",
                                    items = chosen,
                                    isBatch = true
                                )
                            }
                        },
                        modifier = Modifier.fillMaxWidth()
                    )
                }
            }
        }
    }

    @Composable
    fun BrowserViewportPane(modifier: Modifier = Modifier) {
        Column(modifier = modifier.fillMaxSize()) {
            // Responsive Address & Control Chrome
            Surface(color = palette.surface) {
                Column(modifier = Modifier.fillMaxWidth()) {
                    if (windowState.isTwoPane) {
                        // Wide Tablet / Unfolded Foldable: Single Unified Chrome Row
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 10.dp, vertical = 6.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            IconButton(
                                onClick = { webViewRef?.goBack() },
                                enabled = canGoBack,
                                modifier = Modifier.size(34.dp)
                            ) {
                                Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back", modifier = Modifier.size(18.dp))
                            }
                            IconButton(
                                onClick = { webViewRef?.goForward() },
                                enabled = canGoForward,
                                modifier = Modifier.size(34.dp)
                            ) {
                                Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = "Forward", modifier = Modifier.size(18.dp))
                            }
                            IconButton(
                                onClick = {
                                    currentUrl = ""
                                    addressInput = ""
                                    webViewRef = null
                                    canGoBack = false
                                    canGoForward = false
                                    isLoadingPage = false
                                    pageCommitted = false
                                    pageErrorText = null
                                    pageLoadTimedOut = false
                                    sniffedItems.clear()
                                    resolvedAV01 = null
                                    resolvedX = null
                                },
                                modifier = Modifier.size(34.dp)
                            ) {
                                Icon(Icons.Default.Home, contentDescription = "Home", modifier = Modifier.size(18.dp))
                            }

                            CompactOmniboxPill(modifier = Modifier.weight(1f))

                            if (currentUrl.isNotBlank()) {
                                Button(
                                    onClick = { openCurrentPageDownloadPicker() },
                                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 6.dp),
                                    modifier = Modifier.height(36.dp)
                                ) {
                                    Icon(Icons.Default.Download, contentDescription = null, modifier = Modifier.size(16.dp))
                                    Text("Download", modifier = Modifier.padding(start = 4.dp), style = MaterialTheme.typography.labelLarge)
                                }
                            }

                            FilterChip(
                                selected = desktopMode,
                                onClick = {
                                    val next = !desktopMode
                                    desktopMode = next
                                    webViewRef?.let { wv ->
                                        wv.settings.userAgentString = if (next) DESKTOP_CHROME_UA else defaultMobileUa
                                        wv.settings.useWideViewPort = true
                                        wv.settings.loadWithOverviewMode = true
                                        if (currentUrl.isNotBlank()) wv.reload()
                                    }
                                },
                                leadingIcon = { Icon(Icons.Default.DesktopWindows, contentDescription = null, modifier = Modifier.size(15.dp)) },
                                label = { Text(if (desktopMode) "Desktop" else "Mobile") }
                            )

                            FilterChip(
                                selected = useServerProxy,
                                onClick = {
                                    val next = !useServerProxy
                                    useServerProxy = next
                                    if (currentUrl.isNotBlank()) loadTarget(currentUrl, forceProxy = next)
                                },
                                leadingIcon = { Icon(Icons.Default.Shield, contentDescription = null, modifier = Modifier.size(15.dp)) },
                                label = { Text(if (useServerProxy) "Proxy" else "Direct") }
                            )
                        }
                    } else {
                        // Compact Phone: Dedicated Full-Width Pill Omnibox + Compact Action Strip
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(horizontal = 8.dp, vertical = 6.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(6.dp)
                            ) {
                                IconButton(
                                    onClick = {
                                        if (canGoBack) webViewRef?.goBack()
                                        else {
                                            currentUrl = ""
                                            addressInput = ""
                                            webViewRef = null
                                            pageErrorText = null
                                            pageLoadTimedOut = false
                                            resolvedAV01 = null
                                            resolvedX = null
                                            sniffedItems.clear()
                                        }
                                    },
                                    enabled = currentUrl.isNotBlank() || canGoBack,
                                    modifier = Modifier.size(32.dp)
                                ) {
                                    Icon(
                                        imageVector = if (canGoBack) Icons.AutoMirrored.Filled.ArrowBack else Icons.Default.Home,
                                        contentDescription = if (canGoBack) "Back" else "Home",
                                        modifier = Modifier.size(18.dp)
                                    )
                                }

                                CompactOmniboxPill(modifier = Modifier.weight(1f))
                            }

                            Row(
                                modifier = Modifier
                                    .fillMaxWidth()
                                    .horizontalScroll(rememberScrollState()),
                                horizontalArrangement = Arrangement.spacedBy(6.dp),
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                IconButton(
                                    onClick = { webViewRef?.goBack() },
                                    enabled = canGoBack,
                                    modifier = Modifier.size(30.dp)
                                ) {
                                    Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back", modifier = Modifier.size(16.dp))
                                }
                                IconButton(
                                    onClick = { webViewRef?.goForward() },
                                    enabled = canGoForward,
                                    modifier = Modifier.size(30.dp)
                                ) {
                                    Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = "Forward", modifier = Modifier.size(16.dp))
                                }
                                IconButton(
                                    onClick = {
                                        currentUrl = ""
                                        addressInput = ""
                                        webViewRef = null
                                        canGoBack = false
                                        canGoForward = false
                                        isLoadingPage = false
                                        pageCommitted = false
                                        pageErrorText = null
                                        pageLoadTimedOut = false
                                        sniffedItems.clear()
                                        resolvedAV01 = null
                                        resolvedX = null
                                    },
                                    modifier = Modifier.size(30.dp)
                                ) {
                                    Icon(Icons.Default.Home, contentDescription = "Home", modifier = Modifier.size(16.dp))
                                }

                                if (currentUrl.isNotBlank()) {
                                    AssistChip(
                                        onClick = { openCurrentPageDownloadPicker() },
                                        leadingIcon = { Icon(Icons.Default.Download, contentDescription = null, modifier = Modifier.size(15.dp), tint = palette.accent) },
                                        label = { Text("Download") }
                                    )
                                }

                                AssistChip(
                                    onClick = {
                                        if (currentUrl.isNotBlank()) {
                                            when {
                                                AV01Resolver.isAV01Url(currentUrl) -> resolveAV01DirectOnPhone(currentUrl)
                                                XTwitterResolver.isXStatusUrl(currentUrl) -> resolveXDirectOnPhone(currentUrl)
                                                else -> triggerServerSniff(currentUrl)
                                            }
                                        }
                                        showSnifferSheet = true
                                    },
                                    leadingIcon = {
                                        BadgedBox(
                                            badge = {
                                                if (sniffedItems.isNotEmpty()) {
                                                    Badge(containerColor = palette.accent) {
                                                        Text("${sniffedItems.size}", color = Color.White)
                                                    }
                                                }
                                            }
                                        ) {
                                            Icon(Icons.Default.Radar, contentDescription = null, modifier = Modifier.size(15.dp))
                                        }
                                    },
                                    label = { Text(if (isSniffingServer) "Sniffing..." else "Sniffer") }
                                )

                                FilterChip(
                                    selected = desktopMode,
                                    onClick = {
                                        val next = !desktopMode
                                        desktopMode = next
                                        webViewRef?.let { wv ->
                                            wv.settings.userAgentString = if (next) DESKTOP_CHROME_UA else defaultMobileUa
                                            wv.settings.useWideViewPort = true
                                            wv.settings.loadWithOverviewMode = true
                                            if (currentUrl.isNotBlank()) wv.reload()
                                        }
                                    },
                                    leadingIcon = { Icon(Icons.Default.DesktopWindows, contentDescription = null, modifier = Modifier.size(14.dp)) },
                                    label = { Text(if (desktopMode) "Desktop" else "Mobile") }
                                )

                                FilterChip(
                                    selected = useServerProxy,
                                    onClick = {
                                        val next = !useServerProxy
                                        useServerProxy = next
                                        if (currentUrl.isNotBlank()) loadTarget(currentUrl, forceProxy = next)
                                    },
                                    leadingIcon = { Icon(Icons.Default.Shield, contentDescription = null, modifier = Modifier.size(14.dp)) },
                                    label = { Text(if (useServerProxy) "Proxy" else "Direct") }
                                )
                            }
                        }
                    }

                    if (isLoadingPage && pageLoadProgress in 1..99) {
                        LinearProgressIndicator(
                            progress = { pageLoadProgress / 100f },
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(2.dp),
                            color = palette.accent,
                            trackColor = Color.Transparent
                        )
                    }
                }
            }
            HorizontalDivider(color = palette.borderSubtle)

            // Instant AV01 Extraction Banner (works even when ISP blocks av01.media HTML!)
            resolvedAV01?.let { av ->
                Surface(
                    color = palette.accentContainer,
                    border = BorderStroke(1.dp, palette.accent.copy(alpha = 0.4f)),
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 10.dp, vertical = 6.dp),
                    shape = RoundedCornerShape(12.dp)
                ) {
                    Column(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(10.dp),
                        verticalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        Row(
                            modifier = Modifier.fillMaxWidth(),
                            horizontalArrangement = Arrangement.SpaceBetween,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            Column(modifier = Modifier.weight(1f)) {
                                Text(
                                    text = av.title,
                                    style = MaterialTheme.typography.titleSmall,
                                    color = palette.accentText,
                                    maxLines = 1,
                                    overflow = TextOverflow.Ellipsis
                                )
                                Text(
                                    text = "Extracted ${av.variants.joinToString(", ") { it.label }} M3U8 streams via CDN",
                                    style = MaterialTheme.typography.labelSmall,
                                    color = palette.textSecondary
                                )
                            }
                            AstryxStatusBadge("M3U8 READY", AstryxTone.Success)
                        }
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Button(
                                onClick = {
                                    val picked = AV01Resolver.selectVariant(av.variants, formatQuality)
                                    onPlayMedia(
                                        MediaPlayRequest(
                                            id = "av01:${av.videoId}",
                                            title = av.title,
                                            subtitle = "AV01 · ${picked.label}",
                                            thumbnailUrl = av.thumbnail,
                                            primaryUrl = picked.url,
                                            fallbackUrl = RetrofitClient.browserStreamUrl(currentUrl.ifBlank { "https://av01.media/jp/video/${av.videoId}" }, av.referer),
                                            kind = MediaKind.VIDEO,
                                            isHls = true,
                                            referer = av.referer,
                                            variants = av.variants,
                                            onSaveToPhone = {
                                                phoneDownloadManager.saveHlsOrAV01ToPhone(
                                                    url = currentUrl.ifBlank { "https://av01.media/jp/video/${av.videoId}" },
                                                    title = av.title,
                                                    formatPref = formatQuality,
                                                    channel = "AV01",
                                                    referer = av.referer
                                                )
                                                onShowMessage("Saving AV01 M3U8 privately inside KV Download...")
                                            }
                                        )
                                    )
                                },
                                modifier = Modifier.weight(1f)
                            ) {
                                Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(16.dp))
                                Text("Play", modifier = Modifier.padding(start = 4.dp))
                            }
                            OutlinedButton(
                                onClick = {
                                    val picked = AV01Resolver.selectVariant(av.variants, formatQuality)
                                    val canonical = currentUrl.ifBlank { "https://av01.media/jp/video/${av.videoId}" }
                                    pendingDownload = PendingBrowserDownload(
                                        label = av.title,
                                        items = listOf(
                                            SniffedMedia(
                                                id = "av01:${av.videoId}",
                                                url = picked.url,
                                                type = "video",
                                                format = "${picked.label} M3U8 HLS",
                                                title = av.title,
                                                thumbnail = av.thumbnail,
                                                source = "av01_phone",
                                                resolution = picked.label,
                                                channel = "AV01",
                                                isStream = true,
                                                referer = av.referer,
                                                variants = av.variants
                                            )
                                        ),
                                        canonicalServerUrl = canonical
                                    )
                                },
                                modifier = Modifier.weight(1f)
                            ) {
                                Icon(Icons.Default.Download, contentDescription = null, modifier = Modifier.size(16.dp))
                                Text("Download", modifier = Modifier.padding(start = 4.dp))
                            }
                        }
                    }
                }
            }

            // Instant X / Twitter Video Extraction Banner
            resolvedX?.let { xt ->
                val xRef = XTwitterResolver.parseStatusUrl(currentUrl)
                val vid = XTwitterResolver.selectVideo(xt, xRef?.videoIndex ?: 0)
                if (vid != null && vid.variants.isNotEmpty()) {
                    Surface(
                        color = palette.accentContainer,
                        border = BorderStroke(1.dp, palette.accent.copy(alpha = 0.4f)),
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(horizontal = 10.dp, vertical = 6.dp),
                        shape = RoundedCornerShape(12.dp)
                    ) {
                        Column(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(10.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                horizontalArrangement = Arrangement.SpaceBetween,
                                verticalAlignment = Alignment.CenterVertically
                            ) {
                                Column(modifier = Modifier.weight(1f)) {
                                    Text(
                                        text = vid.title,
                                        style = MaterialTheme.typography.titleSmall,
                                        color = palette.accentText,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Text(
                                        text = "${xt.channel} · ${vid.variants.joinToString(", ") { it.label }}",
                                        style = MaterialTheme.typography.labelSmall,
                                        color = palette.textSecondary
                                    )
                                }
                                AstryxStatusBadge("X VIDEO READY", AstryxTone.Success)
                            }
                            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                Button(
                                    onClick = {
                                        val picked = XTwitterResolver.selectVariant(vid.variants, formatQuality)
                                        val canonical = currentUrl.ifBlank { xt.canonicalUrl }
                                        onPlayMedia(
                                            MediaPlayRequest(
                                                id = "x:${xt.tweetId}",
                                                title = vid.title,
                                                subtitle = "${xt.channel} · ${picked.label}",
                                                thumbnailUrl = vid.thumbnail ?: xt.thumbnail,
                                                primaryUrl = picked.url,
                                                fallbackUrl = RetrofitClient.browserStreamUrl(canonical, xt.referer),
                                                kind = MediaKind.VIDEO,
                                                isHls = HlsSegmentDownloader.looksLikeHlsUrl(picked.url),
                                                referer = xt.referer,
                                                variants = vid.variants,
                                                onSaveToPhone = {
                                                    phoneDownloadManager.saveXVideoToPhone(
                                                        url = canonical,
                                                        title = vid.title,
                                                        formatPref = formatQuality,
                                                        channel = xt.channel,
                                                        referer = xt.referer
                                                    )
                                                    onShowMessage("Saving X video privately inside KV Download...")
                                                }
                                            )
                                        )
                                    },
                                    modifier = Modifier.weight(1f)
                                ) {
                                    Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(16.dp))
                                    Text("Play", modifier = Modifier.padding(start = 4.dp))
                                }
                                OutlinedButton(
                                    onClick = {
                                        val picked = XTwitterResolver.selectVariant(vid.variants, formatQuality)
                                        val canonical = currentUrl.ifBlank { xt.canonicalUrl }
                                        pendingDownload = PendingBrowserDownload(
                                            label = vid.title,
                                            items = listOf(
                                                SniffedMedia(
                                                    id = "x:${xt.tweetId}",
                                                    url = picked.url,
                                                    type = "video",
                                                    format = "${picked.label} MP4",
                                                    title = vid.title,
                                                    thumbnail = vid.thumbnail ?: xt.thumbnail,
                                                    source = "x_phone",
                                                    resolution = picked.label,
                                                    channel = xt.channel,
                                                    isStream = HlsSegmentDownloader.looksLikeHlsUrl(picked.url),
                                                    referer = xt.referer,
                                                    variants = vid.variants
                                                )
                                            ),
                                            canonicalServerUrl = canonical
                                        )
                                    },
                                    modifier = Modifier.weight(1f)
                                ) {
                                    Icon(Icons.Default.Download, contentDescription = null, modifier = Modifier.size(16.dp))
                                    Text("Download", modifier = Modifier.padding(start = 4.dp))
                                }
                            }
                        }
                    }
                }
            }

            if (currentUrl.isBlank()) {
                LazyColumn(
                    modifier = Modifier.fillMaxSize(),
                    contentPadding = PaddingValues(14.dp),
                    verticalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    item {
                        Text("Quick Bookmarks & AV01 Extractor", style = MaterialTheme.typography.titleMedium, color = palette.textPrimary)
                        Text(
                            "Paste any av01.media URL or video ID below (works even on ISP-blocked Wi-Fi) — M3U8 streams resolve directly via CDN.",
                            style = MaterialTheme.typography.bodySmall,
                            color = palette.textSecondary
                        )
                    }

                    item {
                        AstryxCard(emphasized = true, contentPadding = PaddingValues(12.dp)) {
                            Text("AV01 Direct Video Extractor", style = MaterialTheme.typography.titleSmall, color = palette.accentText)
                            OutlinedTextField(
                                value = av01QuickInput,
                                onValueChange = { av01QuickInput = it },
                                modifier = Modifier.fillMaxWidth(),
                                singleLine = true,
                                placeholder = { Text("Paste av01.media/jp/video/... URL or video ID (e.g. 226601)") }
                            )
                            Button(
                                onClick = {
                                    val normalized = normalizeAv01QuickInput(av01QuickInput)
                                    if (normalized == null) {
                                        onShowMessage("Enter an av01.media video URL or numeric ID")
                                    } else {
                                        isResolvingQuickAv01 = true
                                        addressInput = normalized
                                        resolveAV01DirectOnPhone(normalized) { av ->
                                            isResolvingQuickAv01 = false
                                            onShowMessage("Resolved ${av.title} (${av.variants.size} M3U8 qualities)")
                                        }
                                    }
                                },
                                enabled = av01QuickInput.isNotBlank() && !isResolvingQuickAv01,
                                modifier = Modifier.fillMaxWidth()
                            ) {
                                if (isResolvingQuickAv01) {
                                    CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp, color = Color.White)
                                } else {
                                    Icon(Icons.Default.Radar, contentDescription = null, modifier = Modifier.size(16.dp))
                                }
                                Text(if (isResolvingQuickAv01) "Resolving CDN..." else "Extract AV01 M3U8", modifier = Modifier.padding(start = 6.dp))
                            }
                        }
                    }

                    val bookmarks = listOf(
                        Triple("Google Search", "https://www.google.com", false),
                        Triple("X (Twitter)", "https://x.com", false),
                        Triple("YouTube", "https://m.youtube.com", false),
                        Triple("TikTok", "https://www.tiktok.com", false),
                        Triple("AV01 Media", "https://av01.media", false),
                        Triple("Tinhte.vn", "https://tinhte.vn", false),
                        Triple("SoundCloud", "https://soundcloud.com", false)
                    )

                    items(bookmarks) { (title, url, proxyRecommended) ->
                        AstryxCard(
                            onClick = {
                                useServerProxy = proxyRecommended
                                loadTarget(url, forceProxy = proxyRecommended)
                            },
                            contentPadding = PaddingValues(14.dp)
                        ) {
                            Row(
                                modifier = Modifier.fillMaxWidth(),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(12.dp)
                            ) {
                                Icon(Icons.Default.Language, contentDescription = null, tint = palette.accent)
                                Column(modifier = Modifier.weight(1f)) {
                                    Text(title, style = MaterialTheme.typography.titleSmall, color = palette.textPrimary)
                                    Text(url, style = MaterialTheme.typography.bodySmall, color = palette.textSecondary)
                                }
                                if (url.contains("av01")) {
                                    AstryxStatusBadge("M3U8 HLS", AstryxTone.Accent)
                                }
                            }
                        }
                    }
                }
            } else {
                val showRecoveryOverlay = pageErrorText != null || pageLoadTimedOut
                val showLoadingOverlay = isLoadingPage && !pageCommitted && !showRecoveryOverlay
                val isAv01Site = currentUrl.contains("av01", ignoreCase = true)

                Box(
                    modifier = Modifier
                        .weight(1f)
                        .fillMaxWidth()
                        .background(palette.surface)
                ) {
                    val bgArgb = palette.surface.toArgb()
                    AndroidView(
                        modifier = Modifier.fillMaxSize(),
                        factory = { ctx ->
                            configureWebView(
                                WebView(ctx),
                                backgroundColorArgb = bgArgb,
                                desktopMode = desktopMode,
                                dohFallbackHosts = dohFallbackHosts,
                                onInitMobileUa = { defaultMobileUa = it },
                                onProgressChanged = { prog ->
                                    pageLoadProgress = prog
                                    if (prog >= 85) isLoadingPage = false
                                },
                                onPageCommitVisible = {
                                    pageCommitted = true
                                    isLoadingPage = false
                                    pageLoadTimedOut = false
                                    pageErrorText = null
                                },
                                onReceivedMainError = { msg ->
                                    isLoadingPage = false
                                    pageErrorText = msg
                                },
                                onEnterCustomFullscreen = { view, callback ->
                                    customFullscreenView = view
                                    customFullscreenCallback = callback
                                },
                                onExitCustomFullscreen = {
                                    customFullscreenView = null
                                    customFullscreenCallback = null
                                },
                                onPageStarted = { pageUrl ->
                                    isLoadingPage = true
                                    pageErrorText = null
                                    pageLoadTimedOut = false
                                    if (!pageUrl.contains("/api/browser/proxy")) {
                                        currentUrl = pageUrl
                                        addressInput = pageUrl
                                        detectPlayablePageItem(pageUrl, null)?.let { mergeSniffed(listOf(it)) }
                                        if (AV01Resolver.isAV01Url(pageUrl) && resolvedAV01?.videoId != AV01Resolver.parseVideoIdAndSlug(pageUrl)?.first) {
                                            resolveAV01DirectOnPhone(pageUrl)
                                        }
                                        val xRef = XTwitterResolver.parseStatusUrl(pageUrl)
                                        if (xRef != null && resolvedX?.tweetId != xRef.tweetId) {
                                            resolveXDirectOnPhone(pageUrl)
                                        } else if (xRef == null) {
                                            resolvedX = null
                                        }
                                    }
                                },
                                onVisitedUrl = { view, updatedUrl ->
                                    canGoBack = view.canGoBack()
                                    canGoForward = view.canGoForward()
                                    if (updatedUrl.startsWith("http") && !updatedUrl.contains("/api/browser/proxy")) {
                                        currentUrl = updatedUrl
                                        addressInput = updatedUrl
                                        detectPlayablePageItem(updatedUrl, view.title)?.let { mergeSniffed(listOf(it)) }
                                        val xRef = XTwitterResolver.parseStatusUrl(updatedUrl)
                                        if (xRef != null && resolvedX?.tweetId != xRef.tweetId) {
                                            resolveXDirectOnPhone(updatedUrl)
                                        } else if (xRef == null) {
                                            resolvedX = null
                                        }
                                        view.evaluateJavascript(INJECTED_SNIFFER_JS, null)
                                    }
                                },
                                onPageFinished = { view ->
                                    isLoadingPage = false
                                    pageLoadProgress = 100
                                    pageCommitted = true
                                    canGoBack = view.canGoBack()
                                    canGoForward = view.canGoForward()
                                    view.title?.let { t ->
                                        view.url?.let { u ->
                                            detectPlayablePageItem(u, t)?.let { mergeSniffed(listOf(it)) }
                                        }
                                    }
                                    view.evaluateJavascript(INJECTED_SNIFFER_JS, null)
                                },
                                onMediaDetected = { item ->
                                    scope.launch(Dispatchers.Main) {
                                        mergeSniffed(listOf(item))
                                    }
                                }
                            ).also { wv ->
                                webViewRef = wv
                                val loadUrl = if (useServerProxy) RetrofitClient.browserProxyUrl(currentUrl) else currentUrl
                                wv.loadUrl(loadUrl)
                            }
                        }
                    )

                    if (showLoadingOverlay) {
                        Box(
                            modifier = Modifier
                                .fillMaxSize()
                                .background(palette.body.copy(alpha = 0.96f))
                                .padding(20.dp),
                            contentAlignment = Alignment.Center
                        ) {
                            AstryxCard(emphasized = true, modifier = Modifier.fillMaxWidth()) {
                                Column(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalAlignment = Alignment.CenterHorizontally,
                                    verticalArrangement = Arrangement.spacedBy(10.dp)
                                ) {
                                    CircularProgressIndicator(color = palette.accent)
                                    Text(
                                        text = "Loading ${Uri.parse(currentUrl).host ?: currentUrl}...",
                                        style = MaterialTheme.typography.titleSmall,
                                        color = palette.textPrimary,
                                        maxLines = 1,
                                        overflow = TextOverflow.Ellipsis
                                    )
                                    Text(
                                        text = if (pageLoadProgress in 1..99) {
                                            "Connecting (${pageLoadProgress}%) · ${if (useServerProxy) "Server Proxy" else "Direct DoH"}"
                                        } else {
                                            "Connecting via ${if (useServerProxy) "Server Proxy" else "Direct Mode"}..."
                                        },
                                        style = MaterialTheme.typography.bodySmall,
                                        color = palette.textSecondary
                                    )
                                    Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                                        OutlinedButton(
                                            onClick = {
                                                val next = !useServerProxy
                                                useServerProxy = next
                                                loadTarget(currentUrl, forceProxy = next)
                                            }
                                        ) {
                                            Text(if (useServerProxy) "Try Direct Mode" else "Try Proxy Mode")
                                        }
                                        OutlinedButton(
                                            onClick = {
                                                webViewRef?.stopLoading()
                                                isLoadingPage = false
                                                pageLoadTimedOut = true
                                            }
                                        ) {
                                            Text("Cancel")
                                        }
                                    }
                                }
                            }
                        }
                    }

                    if (showRecoveryOverlay) {
                        Box(
                            modifier = Modifier
                                .fillMaxSize()
                                .background(palette.body)
                                .verticalScroll(rememberScrollState())
                                .padding(16.dp),
                            contentAlignment = Alignment.Center
                        ) {
                            AstryxCard(emphasized = true, modifier = Modifier.fillMaxWidth()) {
                                Row(
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                                ) {
                                    Icon(Icons.Default.WifiOff, contentDescription = null, tint = palette.warning, modifier = Modifier.size(28.dp))
                                    Column(modifier = Modifier.weight(1f)) {
                                        Text(
                                            text = if (pageLoadTimedOut) "Site timed out or ISP-blocked" else "Page could not load",
                                            style = MaterialTheme.typography.titleMedium,
                                            color = palette.textPrimary
                                        )
                                        Text(
                                            text = pageErrorText ?: "No response within 12s from ${Uri.parse(currentUrl).host ?: currentUrl}. Your ISP may block HTTPS/TLS directly.",
                                            style = MaterialTheme.typography.bodySmall,
                                            color = palette.textSecondary
                                        )
                                    }
                                }

                                Row(
                                    modifier = Modifier.fillMaxWidth(),
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    Button(
                                        onClick = { loadTarget(currentUrl, forceProxy = useServerProxy) },
                                        modifier = Modifier.weight(1f)
                                    ) {
                                        Icon(Icons.Default.Refresh, contentDescription = null, modifier = Modifier.size(16.dp))
                                        Text("Retry", modifier = Modifier.padding(start = 4.dp))
                                    }
                                    OutlinedButton(
                                        onClick = {
                                            val next = !useServerProxy
                                            useServerProxy = next
                                            loadTarget(currentUrl, forceProxy = next)
                                        },
                                        modifier = Modifier.weight(1f)
                                    ) {
                                        Icon(Icons.Default.Shield, contentDescription = null, modifier = Modifier.size(16.dp))
                                        Text(if (useServerProxy) "Direct Mode" else "Server Proxy", modifier = Modifier.padding(start = 4.dp))
                                    }
                                }

                                if (isAv01Site) {
                                    HorizontalDivider(color = palette.borderSubtle)
                                    Text(
                                        text = "AV01 CDN Extractor (bypasses av01.media HTML block)",
                                        style = MaterialTheme.typography.titleSmall,
                                        color = palette.accentText
                                    )
                                    Text(
                                        text = "When your ISP blocks av01.media web pages, paste any AV01 video URL or numeric ID below to stream & download M3U8 directly from iw01.xyz CDN.",
                                        style = MaterialTheme.typography.bodySmall,
                                        color = palette.textSecondary
                                    )
                                    OutlinedTextField(
                                        value = av01QuickInput,
                                        onValueChange = { av01QuickInput = it },
                                        modifier = Modifier.fillMaxWidth(),
                                        singleLine = true,
                                        placeholder = { Text("av01.media/jp/video/226601 or 226601") }
                                    )
                                    Button(
                                        onClick = {
                                            val candidate = normalizeAv01QuickInput(av01QuickInput)
                                                ?: normalizeAv01QuickInput(currentUrl)
                                            if (candidate == null) {
                                                onShowMessage("Paste an av01.media video URL or numeric ID")
                                            } else {
                                                isResolvingQuickAv01 = true
                                                resolveAV01DirectOnPhone(candidate) { av ->
                                                    isResolvingQuickAv01 = false
                                                    pageErrorText = null
                                                    pageLoadTimedOut = false
                                                    onShowMessage("Resolved ${av.title} — tap Play or Download above")
                                                }
                                            }
                                        },
                                        enabled = !isResolvingQuickAv01 && (av01QuickInput.isNotBlank() || AV01Resolver.isAV01Url(currentUrl)),
                                        modifier = Modifier.fillMaxWidth()
                                    ) {
                                        Icon(Icons.Default.Radar, contentDescription = null, modifier = Modifier.size(16.dp))
                                        Text("Extract AV01 Video Now", modifier = Modifier.padding(start = 6.dp))
                                    }
                                }
                            }
                        }
                    }

                    if (customFullscreenView != null) {
                        AndroidView(
                            factory = { customFullscreenView!! },
                            modifier = Modifier
                                .fillMaxSize()
                                .background(Color.Black),
                            onRelease = {
                                customFullscreenCallback?.onCustomViewHidden()
                            }
                        )
                    }
                }
            }
        }
    }

    if (windowState.isTwoPane) {
        // Tablet / Foldable 2-Pane Split: Live Browser (Left) + Docked Live Sniffer Panel (Right)
        Row(modifier = Modifier.fillMaxSize()) {
            BrowserViewportPane(
                modifier = Modifier
                    .weight(0.58f)
                    .fillMaxHeight()
            )
            VerticalDivider(color = palette.borderSubtle)
            Box(
                modifier = Modifier
                    .weight(0.42f)
                    .fillMaxHeight()
                    .background(palette.surface)
                    .padding(top = 12.dp)
            ) {
                SnifferPanelContent(modifier = Modifier.fillMaxSize(), compactSheet = false)
            }
        }
    } else {
        BrowserViewportPane(modifier = Modifier.fillMaxSize())
    }

    if (showSnifferSheet && !windowState.isTwoPane) {
        ModalBottomSheet(
            onDismissRequest = { showSnifferSheet = false },
            containerColor = palette.surface
        ) {
            SnifferPanelContent(compactSheet = true)
        }
    }

    // Unified Destination Chooser: NAS (Server) vs This Phone (Private App Vault)
    pendingDownload?.let { req ->
        ModalBottomSheet(
            onDismissRequest = { pendingDownload = null },
            containerColor = palette.surface
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp)
                    .padding(bottom = 24.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Text(
                    text = "Choose Download Destination",
                    style = MaterialTheme.typography.titleMedium,
                    color = palette.textPrimary
                )
                Text(
                    text = req.label,
                    style = MaterialTheme.typography.bodySmall,
                    color = palette.textSecondary,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis
                )

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    listOf("highest" to "Best Quality", "1080p" to "1080p", "720p" to "720p", "audio" to "MP3 Audio").forEach { (q, lbl) ->
                        FilterChip(
                            selected = formatQuality == q,
                            onClick = { formatQuality = q },
                            label = { Text(lbl) }
                        )
                    }
                }

                AstryxCard(
                    emphasized = true,
                    onClick = { executePendingDownloadToNas(req) },
                    contentPadding = PaddingValues(14.dp)
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.CloudUpload,
                            contentDescription = null,
                            tint = palette.accent,
                            modifier = Modifier.size(28.dp)
                        )
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = "NAS (Server)",
                                style = MaterialTheme.typography.titleSmall,
                                color = palette.textPrimary
                            )
                            Text(
                                text = "Queue download on ${RetrofitClient.getBaseUrl().removePrefix("https://").removePrefix("http://")} with yt-dlp & folder organization",
                                style = MaterialTheme.typography.bodySmall,
                                color = palette.textSecondary
                            )
                        }
                        AstryxStatusBadge("NAS", AstryxTone.Accent)
                    }
                }

                AstryxCard(
                    onClick = { executePendingDownloadToPhone(req) },
                    contentPadding = PaddingValues(14.dp)
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(12.dp)
                    ) {
                        Icon(
                            imageVector = Icons.Default.Lock,
                            contentDescription = null,
                            tint = palette.success,
                            modifier = Modifier.size(28.dp)
                        )
                        Column(modifier = Modifier.weight(1f)) {
                            Text(
                                text = "Phone (Private Vault)",
                                style = MaterialTheme.typography.titleSmall,
                                color = palette.textPrimary
                            )
                            Text(
                                text = "Save directly inside KV Download only — hidden from Gallery and other apps",
                                style = MaterialTheme.typography.bodySmall,
                                color = palette.textSecondary
                            )
                        }
                        AstryxStatusBadge("PRIVATE", AstryxTone.Success)
                    }
                }
            }
        }
    }
}

@SuppressLint("SetJavaScriptEnabled")
private fun configureWebView(
    webView: WebView,
    backgroundColorArgb: Int,
    desktopMode: Boolean,
    dohFallbackHosts: MutableSet<String>,
    onInitMobileUa: (String) -> Unit,
    onProgressChanged: (Int) -> Unit,
    onPageCommitVisible: () -> Unit,
    onReceivedMainError: (String) -> Unit,
    onEnterCustomFullscreen: (View, WebChromeClient.CustomViewCallback?) -> Unit,
    onExitCustomFullscreen: () -> Unit,
    onPageStarted: (String) -> Unit,
    onVisitedUrl: (WebView, String) -> Unit,
    onPageFinished: (WebView) -> Unit,
    onMediaDetected: (SniffedMedia) -> Unit
): WebView {
    CookieManager.getInstance().setAcceptCookie(true)
    CookieManager.getInstance().setAcceptThirdPartyCookies(webView, true)

    webView.setBackgroundColor(backgroundColorArgb)
    webView.setLayerType(View.LAYER_TYPE_HARDWARE, null)

    webView.settings.apply {
        javaScriptEnabled = true
        domStorageEnabled = true
        databaseEnabled = true
        javaScriptCanOpenWindowsAutomatically = true
        setSupportMultipleWindows(false)
        cacheMode = WebSettings.LOAD_DEFAULT
        mediaPlaybackRequiresUserGesture = false
        mixedContentMode = WebSettings.MIXED_CONTENT_COMPATIBILITY_MODE
        loadWithOverviewMode = true
        useWideViewPort = true
        val cleanMobileUa = userAgentString.replace("; wv)", ")").replace("; wv", "")
        onInitMobileUa(cleanMobileUa)
        userAgentString = if (desktopMode) DESKTOP_CHROME_UA else cleanMobileUa
    }

    webView.addJavascriptInterface(
        object {
            @JavascriptInterface
            fun onMediaFound(rawJson: String) {
                try {
                    val obj = JSONObject(rawJson)
                    val u = obj.optString("url")
                    if (u.isNotBlank()) {
                        val type = obj.optString("type", "video")
                        val fmt = obj.optString("format", "")
                        val title = obj.optString("title", "Detected Media")
                        val thumb = obj.optString("thumbnail", "").ifBlank { null }
                        val ref = obj.optString("referer", "").ifBlank { null }
                        onMediaDetected(
                            SniffedMedia(
                                id = "wv_${u.hashCode()}",
                                url = u,
                                type = type,
                                format = fmt.ifBlank { type.uppercase() },
                                title = title.ifBlank { "Detected Media" },
                                thumbnail = thumb,
                                source = "dom",
                                isStream = u.contains(".m3u8") || u.contains(".mpd"),
                                referer = ref
                            )
                        )
                    }
                } catch (_: Exception) {}
            }
        },
        "KVSniffer"
    )

    val streamInterceptRe = Regex("""\.(m3u8|mpd|mp4|webm|m4a|mp3|aac|flac)(\?|$)""", RegexOption.IGNORE_CASE)

    webView.webChromeClient = object : WebChromeClient() {
        override fun onProgressChanged(view: WebView?, newProgress: Int) {
            onProgressChanged(newProgress)
        }

        override fun onShowCustomView(view: View?, callback: CustomViewCallback?) {
            if (view != null) {
                view.layoutParams = FrameLayout.LayoutParams(
                    ViewGroup.LayoutParams.MATCH_PARENT,
                    ViewGroup.LayoutParams.MATCH_PARENT
                )
                onEnterCustomFullscreen(view, callback)
            }
        }

        override fun onHideCustomView() {
            onExitCustomFullscreen()
        }
    }

    webView.webViewClient = object : WebViewClient() {
        override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
            val safe = resolveSafeWebNavigationUrl(request?.url?.toString())
            return when {
                safe == null -> false
                safe.isNotEmpty() -> {
                    view?.loadUrl(safe)
                    true
                }
                else -> true
            }
        }

        override fun doUpdateVisitedHistory(view: WebView?, url: String?, isReload: Boolean) {
            super.doUpdateVisitedHistory(view, url, isReload)
            if (view != null && url != null) {
                onVisitedUrl(view, url)
            }
        }

        override fun onPageCommitVisible(view: WebView?, url: String?) {
            super.onPageCommitVisible(view, url)
            onPageCommitVisible()
        }

        override fun onReceivedError(view: WebView?, request: WebResourceRequest?, error: WebResourceError?) {
            super.onReceivedError(view, request, error)
            if (request?.isForMainFrame == true) {
                val host = request.url?.host?.lowercase()
                val code = error?.errorCode ?: -1
                if (code == ERROR_HOST_LOOKUP && !host.isNullOrBlank() && dohFallbackHosts.add(host)) {
                    view?.post { view.loadUrl(request.url.toString()) }
                    return
                }
                val desc = error?.description?.toString()?.takeIf { it.isNotBlank() } ?: "Connection error"
                onReceivedMainError("$desc (code $code)")
            }
        }

        override fun shouldInterceptRequest(view: WebView?, request: WebResourceRequest?): WebResourceResponse? {
            val reqUrl = request?.url?.toString()
            if (!reqUrl.isNullOrBlank()) {
                val lowerFull = reqUrl.lowercase()
                val isTikTokMp4 = lowerFull.contains("tiktokcdn") && lowerFull.contains("mime_type=video_mp4")
                if (streamInterceptRe.containsMatchIn(reqUrl) || isTikTokMp4) {
                    val lower = reqUrl.substringBefore('?').lowercase()
                    if (!lower.endsWith(".m4s") && !lower.endsWith(".ts") && !lowerFull.contains("googlevideo.com/videoplayback")) {
                        val isAudio = lower.endsWith(".mp3") || lower.endsWith(".m4a") || lower.endsWith(".aac") || lower.endsWith(".flac")
                        val isHls = lower.endsWith(".m3u8") || lower.endsWith(".mpd")
                        val ref = request.requestHeaders?.get("Referer")
                        onMediaDetected(
                            SniffedMedia(
                                id = "net_${reqUrl.hashCode()}",
                                url = reqUrl,
                                type = if (isAudio) "audio" else "video",
                                format = if (isHls) "M3U8 HLS" else if (isAudio) "AUDIO" else "MP4",
                                title = reqUrl.substringBefore('?').substringAfterLast('/').ifBlank { "Captured Stream" },
                                source = "webview_net",
                                isStream = isHls,
                                referer = ref
                            )
                        )
                    }
                }

                // DoH fallback fetch for ISP DNS-blocked hosts
                val host = request.url?.host?.lowercase()
                if (!host.isNullOrBlank() && dohFallbackHosts.contains(host) && request.method.equals("GET", ignoreCase = true)) {
                    try {
                        val client = RetrofitClient.okHttpClient.newBuilder()
                            .callTimeout(8, TimeUnit.SECONDS)
                            .connectTimeout(6, TimeUnit.SECONDS)
                            .readTimeout(6, TimeUnit.SECONDS)
                            .build()
                        val b = Request.Builder().url(reqUrl)
                        request.requestHeaders?.forEach { (k, v) ->
                            if (!k.equals("Host", ignoreCase = true)) b.header(k, v)
                        }
                        runCatching { CookieManager.getInstance().getCookie(reqUrl) }.getOrNull()?.let { c ->
                            if (c.isNotBlank()) b.header("Cookie", c)
                        }
                        client.newCall(b.build()).execute().use { resp ->
                            val rawCt = resp.header("Content-Type") ?: "text/html; charset=utf-8"
                            val mime = rawCt.substringBefore(';').trim().ifBlank { "text/html" }
                            val charset = Regex("""charset=([^;\s]+)""", RegexOption.IGNORE_CASE)
                                .find(rawCt)?.groupValues?.getOrNull(1) ?: "UTF-8"
                            val bytes = resp.body?.bytes() ?: ByteArray(0)
                            val statusCode = resp.code.takeIf { it in 100..599 } ?: 200
                            return WebResourceResponse(
                                mime,
                                charset,
                                statusCode,
                                resp.message.ifBlank { "OK" },
                                emptyMap(),
                                ByteArrayInputStream(bytes)
                            )
                        }
                    } catch (_: Exception) {
                    }
                }
            }
            return super.shouldInterceptRequest(view, request)
        }

        override fun onPageStarted(view: WebView?, url: String?, favicon: Bitmap?) {
            if (url != null) onPageStarted(url)
        }

        override fun onPageFinished(view: WebView?, url: String?) {
            if (view != null) onPageFinished(view)
        }
    }

    return webView
}
