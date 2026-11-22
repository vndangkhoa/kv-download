package com.kvdownload.app.ui.screens

import android.annotation.SuppressLint
import android.net.Uri
import android.webkit.CookieManager
import android.webkit.JavascriptInterface
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
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
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.DesktopWindows
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Home
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.Language
import androidx.compose.material.icons.filled.PhoneAndroid
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Radar
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Shield
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
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateListOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
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
import kotlinx.coroutines.launch
import org.json.JSONObject
import java.net.URLDecoder
import java.net.URLEncoder

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
    var isSniffingServer by remember { mutableStateOf(false) }

    var webViewRef by remember { mutableStateOf<WebView?>(null) }
    var canGoBack by remember { mutableStateOf(false) }
    var canGoForward by remember { mutableStateOf(false) }

    val sniffedItems = remember { mutableStateListOf<SniffedMedia>() }
    var resolvedAV01 by remember { mutableStateOf<AV01ResolvedMedia?>(null) }
    var showSnifferSheet by remember { mutableStateOf(false) }
    var sniffTab by remember { mutableStateOf("video") }
    var selectedUrls by remember { mutableStateOf<Set<String>>(emptySet()) }
    var formatQuality by remember { mutableStateOf("highest") }

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

    fun resolveAV01DirectOnPhone(target: String) {
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
            } catch (_: Exception) {
                // Server sniff still runs below as backup
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
                if (!AV01Resolver.isAV01Url(target) || resolvedAV01 == null) {
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
        sniffedItems.clear()
        resolvedAV01 = null
        selectedUrls = emptySet()

        detectPlayablePageItem(clean, null)?.let { mergeSniffed(listOf(it)) }
        if (AV01Resolver.isAV01Url(clean)) {
            resolveAV01DirectOnPhone(clean)
        } else {
            triggerServerSniff(clean)
        }

        val loadUrl = if (forceProxy) RetrofitClient.browserProxyUrl(clean) else clean
        webViewRef?.loadUrl(loadUrl)
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
                    modifier = if (compactSheet) Modifier.fillMaxWidth().height(360.dp) else Modifier.weight(1f).fillMaxWidth(),
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
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .horizontalScroll(rememberScrollState()),
                                    horizontalArrangement = Arrangement.spacedBy(6.dp)
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
                                                        onShowMessage("Saving directly to phone storage...")
                                                    }
                                                )
                                            )
                                        }
                                    ) {
                                        Icon(
                                            imageVector = if (isImage) Icons.Default.Image else Icons.Default.PlayArrow,
                                            contentDescription = null,
                                            modifier = Modifier.size(16.dp)
                                        )
                                        Text(if (isImage) "View" else "Preview", modifier = Modifier.padding(start = 4.dp))
                                    }

                                    OutlinedButton(
                                        onClick = {
                                            phoneDownloadManager.saveDirectMediaToPhone(
                                                mediaUrl = item.url,
                                                title = item.title,
                                                type = item.type,
                                                channel = item.channel,
                                                referer = effReferer,
                                                formatPref = item.resolution ?: formatQuality
                                            )
                                            onShowMessage(
                                                if (itemIsHls) "Downloading M3U8 segments to Movies/KV Download..."
                                                else "Downloading directly to phone storage..."
                                            )
                                        }
                                    ) {
                                        Icon(Icons.Default.PhoneAndroid, contentDescription = null, modifier = Modifier.size(16.dp))
                                        Text("To Phone", modifier = Modifier.padding(start = 4.dp))
                                    }

                                    Button(
                                        onClick = {
                                            scope.launch {
                                                // If AV01, send canonical av01.media URL when available so server refreshes its own IP JWT cleanly
                                                val serverEnqueueUrl = if (AV01Resolver.isAV01Url(currentUrl)) currentUrl else item.url
                                                val serverFmt = item.resolution?.lowercase()?.takeIf {
                                                    it in setOf("1080p", "720p", "480p", "360p")
                                                } ?: formatQuality
                                                try {
                                                    RetrofitClient.getApi().addQueue(
                                                        QueueAddRequest(
                                                            items = listOf(
                                                                EnqueueItemInput(
                                                                    url = serverEnqueueUrl,
                                                                    title = item.title,
                                                                    thumbnail = item.thumbnail,
                                                                    channel = item.channel,
                                                                    duration = item.duration,
                                                                    referer = effReferer
                                                                )
                                                            ),
                                                            format = serverFmt
                                                        )
                                                    )
                                                    onRefreshQueue()
                                                    onShowMessage("Queued on server: ${item.title}")
                                                } catch (e: Exception) {
                                                    onShowMessage(e.message ?: "Server queue failed")
                                                }
                                            }
                                        }
                                    ) {
                                        Text("Server")
                                    }
                                }
                            }
                        }
                    }
                }

                if (selectedUrls.isNotEmpty()) {
                    AstryxGradientButton(
                        text = "Queue Selected as Group (${selectedUrls.size})",
                        icon = Icons.Default.Download,
                        onClick = {
                            val chosen = sniffedItems.filter { selectedUrls.contains(it.url) }
                            showSnifferSheet = false
                            scope.launch {
                                try {
                                    RetrofitClient.getApi().addQueue(
                                        QueueAddRequest(
                                            items = chosen.map {
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
                                            groupTitle = "Browser Batch (${chosen.size})",
                                            groupType = "batch"
                                        )
                                    )
                                    selectedUrls = emptySet()
                                    onRefreshQueue()
                                    onShowMessage("Queued ${chosen.size} items as a group")
                                } catch (e: Exception) {
                                    onShowMessage(e.message ?: "Batch queue failed")
                                }
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
            // Address Bar & Controls
            Surface(color = palette.surface) {
                Column(
                    modifier = Modifier
                        .fillMaxWidth()
                        .padding(horizontal = 10.dp, vertical = 8.dp),
                    verticalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        verticalAlignment = Alignment.CenterVertically,
                        horizontalArrangement = Arrangement.spacedBy(4.dp)
                    ) {
                        IconButton(onClick = { webViewRef?.goBack() }, enabled = canGoBack) {
                            Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
                        }
                        IconButton(onClick = { webViewRef?.goForward() }, enabled = canGoForward) {
                            Icon(Icons.AutoMirrored.Filled.ArrowForward, contentDescription = "Forward")
                        }
                        IconButton(
                            onClick = {
                                currentUrl = ""
                                addressInput = ""
                                webViewRef = null
                                canGoBack = false
                                canGoForward = false
                                sniffedItems.clear()
                                resolvedAV01 = null
                            }
                        ) {
                            Icon(Icons.Default.Home, contentDescription = "Home")
                        }

                        OutlinedTextField(
                            value = addressInput,
                            onValueChange = { addressInput = it },
                            modifier = Modifier.weight(1f),
                            singleLine = true,
                            placeholder = { Text("Search Google or enter URL (youtube.com, tiktok.com)...") },
                            keyboardOptions = KeyboardOptions(imeAction = ImeAction.Go),
                            keyboardActions = KeyboardActions(
                                onGo = { if (addressInput.isNotBlank()) loadTarget(addressInput) }
                            )
                        )

                        IconButton(
                            onClick = {
                                if (addressInput.isNotBlank()) loadTarget(addressInput)
                            }
                        ) {
                            Icon(Icons.Default.Refresh, contentDescription = "Go / Reload")
                        }
                    }

                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(8.dp),
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        FilterChip(
                            selected = desktopMode,
                            onClick = {
                                val next = !desktopMode
                                desktopMode = next
                                webViewRef?.let { wv ->
                                    wv.settings.userAgentString = if (next) DESKTOP_CHROME_UA else defaultMobileUa
                                    wv.settings.useWideViewPort = true
                                    wv.settings.loadWithOverviewMode = true
                                    if (currentUrl.isNotBlank()) {
                                        wv.reload()
                                    }
                                }
                            },
                            leadingIcon = { Icon(Icons.Default.DesktopWindows, contentDescription = null, modifier = Modifier.size(16.dp)) },
                            label = { Text(if (desktopMode) "Desktop UA" else "Mobile Chrome") }
                        )

                        FilterChip(
                            selected = useServerProxy,
                            onClick = {
                                val next = !useServerProxy
                                useServerProxy = next
                                if (currentUrl.isNotBlank()) loadTarget(currentUrl, forceProxy = next)
                            },
                            leadingIcon = { Icon(Icons.Default.Shield, contentDescription = null, modifier = Modifier.size(16.dp)) },
                            label = { Text(if (useServerProxy) "Proxy Mode ON" else "Direct Mode") }
                        )

                        if (!windowState.isTwoPane) {
                            AssistChip(
                                onClick = {
                                    if (currentUrl.isNotBlank()) {
                                        if (AV01Resolver.isAV01Url(currentUrl)) resolveAV01DirectOnPhone(currentUrl)
                                        else triggerServerSniff(currentUrl)
                                    }
                                    showSnifferSheet = true
                                },
                                leadingIcon = {
                                    BadgedBox(
                                        badge = {
                                            if (sniffedItems.isNotEmpty()) {
                                                Badge(containerColor = palette.accent) {
                                                    Text("${sniffedItems.size}", color = androidx.compose.ui.graphics.Color.White)
                                                }
                                            }
                                        }
                                    ) {
                                        Icon(Icons.Default.Radar, contentDescription = null, modifier = Modifier.size(16.dp))
                                    }
                                },
                                label = { Text(if (isSniffingServer) "Sniffing..." else "Live Sniffer") }
                            )
                        }

                        if (currentUrl.isNotBlank()) {
                            AssistChip(
                                onClick = {
                                    scope.launch {
                                        try {
                                            RetrofitClient.getApi().addQueue(
                                                QueueAddRequest(urls = listOf(currentUrl), format = formatQuality)
                                            )
                                            onRefreshQueue()
                                            onShowMessage("Sent current page to download queue")
                                        } catch (e: Exception) {
                                            onShowMessage(e.message ?: "Failed")
                                        }
                                    }
                                },
                                leadingIcon = { Icon(Icons.Default.Download, contentDescription = null, modifier = Modifier.size(16.dp)) },
                                label = { Text("Queue Page") }
                            )
                        }
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
                                            fallbackUrl = RetrofitClient.browserStreamUrl(currentUrl, av.referer),
                                            kind = MediaKind.VIDEO,
                                            isHls = true,
                                            referer = av.referer,
                                            variants = av.variants,
                                            onSaveToPhone = {
                                                phoneDownloadManager.saveHlsOrAV01ToPhone(
                                                    url = currentUrl,
                                                    title = av.title,
                                                    formatPref = formatQuality,
                                                    channel = "AV01",
                                                    referer = av.referer
                                                )
                                                onShowMessage("Downloading M3U8 to Movies/KV Download/AV01...")
                                            }
                                        )
                                    )
                                }
                            ) {
                                Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(16.dp))
                                Text("Play Fullscreen", modifier = Modifier.padding(start = 4.dp))
                            }
                            OutlinedButton(
                                onClick = {
                                    phoneDownloadManager.saveHlsOrAV01ToPhone(
                                        url = currentUrl,
                                        title = av.title,
                                        formatPref = formatQuality,
                                        channel = "AV01",
                                        referer = av.referer
                                    )
                                    onShowMessage("Saving AV01 M3U8 segments to phone storage...")
                                }
                            ) {
                                Icon(Icons.Default.PhoneAndroid, contentDescription = null, modifier = Modifier.size(16.dp))
                                Text("Save M3U8", modifier = Modifier.padding(start = 4.dp))
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
                            "Paste any av01.media video URL directly in the bar above (even on ISP-blocked Wi-Fi) — M3U8 streams resolve automatically.",
                            style = MaterialTheme.typography.bodySmall,
                            color = palette.textSecondary
                        )
                    }

                    val bookmarks = listOf(
                        Triple("Google Search", "https://www.google.com", false),
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
                Box(modifier = Modifier.weight(1f).fillMaxWidth()) {
                    AndroidView(
                        modifier = Modifier.fillMaxSize(),
                        factory = { ctx ->
                            configureWebView(
                                WebView(ctx),
                                desktopMode = desktopMode,
                                onInitMobileUa = { defaultMobileUa = it },
                                onPageStarted = { pageUrl ->
                                    isLoadingPage = true
                                    if (!pageUrl.contains("/api/browser/proxy")) {
                                        currentUrl = pageUrl
                                        addressInput = pageUrl
                                        detectPlayablePageItem(pageUrl, null)?.let { mergeSniffed(listOf(it)) }
                                        if (AV01Resolver.isAV01Url(pageUrl) && resolvedAV01?.videoId != AV01Resolver.parseVideoIdAndSlug(pageUrl)?.first) {
                                            resolveAV01DirectOnPhone(pageUrl)
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
                                        view.evaluateJavascript(INJECTED_SNIFFER_JS, null)
                                    }
                                },
                                onPageFinished = { view ->
                                    isLoadingPage = false
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

                    if (isLoadingPage) {
                        CircularProgressIndicator(
                            modifier = Modifier
                                .align(Alignment.TopEnd)
                                .padding(12.dp)
                                .size(22.dp),
                            strokeWidth = 2.5.dp,
                            color = palette.accent
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
}

@SuppressLint("SetJavaScriptEnabled")
private fun configureWebView(
    webView: WebView,
    desktopMode: Boolean,
    onInitMobileUa: (String) -> Unit,
    onPageStarted: (String) -> Unit,
    onVisitedUrl: (WebView, String) -> Unit,
    onPageFinished: (WebView) -> Unit,
    onMediaDetected: (SniffedMedia) -> Unit
): WebView {
    CookieManager.getInstance().setAcceptCookie(true)
    CookieManager.getInstance().setAcceptThirdPartyCookies(webView, true)

    webView.settings.apply {
        javaScriptEnabled = true
        domStorageEnabled = true
        mediaPlaybackRequiresUserGesture = false
        mixedContentMode = WebSettings.MIXED_CONTENT_COMPATIBILITY_MODE
        loadWithOverviewMode = true
        useWideViewPort = true
        // Remove "; wv" token so Google, YouTube, and TikTok treat this as standard Chrome
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

    webView.webChromeClient = WebChromeClient()
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
            }
            return super.shouldInterceptRequest(view, request)
        }

        override fun onPageStarted(view: WebView?, url: String?, favicon: android.graphics.Bitmap?) {
            if (url != null) onPageStarted(url)
        }

        override fun onPageFinished(view: WebView?, url: String?) {
            if (view != null) onPageFinished(view)
        }
    }

    return webView
}
