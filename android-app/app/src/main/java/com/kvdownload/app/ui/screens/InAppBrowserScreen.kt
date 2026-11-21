package com.kvdownload.app.ui.screens

import android.annotation.SuppressLint
import android.webkit.CookieManager
import android.webkit.JavascriptInterface
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebSettings
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardActions
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.automirrored.filled.ArrowForward
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Home
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
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import com.kvdownload.app.api.EnqueueItemInput
import com.kvdownload.app.api.QueueAddRequest
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.SniffedMedia
import com.kvdownload.app.storage.PhoneDownloadManager
import com.kvdownload.app.ui.components.PlaybackRequest
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import org.json.JSONObject

private const val INJECTED_SNIFFER_JS = """
(function() {
  if (window.__kvAndroidSnifferInstalled) return;
  window.__kvAndroidSnifferInstalled = true;

  function emit(url, type, fmt, thumb, title) {
    if (!url || typeof url !== 'string') return;
    if (url.startsWith('data:') || url.startsWith('blob:')) return;
    try {
      window.KVSniffer.onMediaFound(JSON.stringify({
        url: url,
        type: type || 'video',
        format: fmt || '',
        thumbnail: thumb || '',
        title: title || document.title || ''
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
      if (reqUrl && /\.(m3u8|mpd|mp4|webm|mp3|m4a)(\?|$)/i.test(reqUrl)) {
        emit(reqUrl, /\.(mp3|m4a)/i.test(reqUrl) ? 'audio' : 'video', 'STREAM', '', document.title);
      }
    } catch (e) {}
    return origFetch.apply(this, arguments);
  };

  var origOpen = XMLHttpRequest.prototype.open;
  XMLHttpRequest.prototype.open = function(method, url) {
    try {
      if (typeof url === 'string' && /\.(m3u8|mpd|mp4|webm|mp3|m4a)(\?|$)/i.test(url)) {
        emit(url, /\.(mp3|m4a)/i.test(url) ? 'audio' : 'video', 'STREAM', '', document.title);
      }
    } catch (e) {}
    return origOpen.apply(this, arguments);
  };

  setTimeout(scanDom, 800);
  setInterval(scanDom, 3500);
})();
"""

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun InAppBrowserScreen(
    phoneDownloadManager: PhoneDownloadManager,
    onRefreshQueue: () -> Unit,
    onPlayMedia: (PlaybackRequest) -> Unit,
    onShowMessage: (String) -> Unit
) {
    val scope = rememberCoroutineScope()

    var addressInput by remember { mutableStateOf("") }
    var currentUrl by remember { mutableStateOf("") }
    var useServerProxy by remember { mutableStateOf(false) }
    var isLoadingPage by remember { mutableStateOf(false) }
    var isSniffingServer by remember { mutableStateOf(false) }

    var webViewRef by remember { mutableStateOf<WebView?>(null) }
    var canGoBack by remember { mutableStateOf(false) }
    var canGoForward by remember { mutableStateOf(false) }

    val sniffedItems = remember { mutableStateListOf<SniffedMedia>() }
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

    fun triggerServerSniff(target: String) {
        if (target.isBlank()) return
        isSniffingServer = true
        scope.launch {
            try {
                val res = RetrofitClient.getApi().browserSniff(target)
                mergeSniffed(res.items)
            } catch (_: Exception) {
            } finally {
                isSniffingServer = false
            }
        }
    }

    fun loadTarget(raw: String, forceProxy: Boolean = useServerProxy) {
        val clean = raw.trim().let {
            if (it.startsWith("http://") || it.startsWith("https://")) it else "https://$it"
        }
        addressInput = clean
        currentUrl = clean
        sniffedItems.clear()
        selectedUrls = emptySet()

        val loadUrl = if (forceProxy) RetrofitClient.browserProxyUrl(clean) else clean
        webViewRef?.loadUrl(loadUrl)
        triggerServerSniff(clean)
    }

    Column(modifier = Modifier.fillMaxSize()) {
        // Address Bar & Controls
        Surface(tonalElevation = 2.dp, color = MaterialTheme.colorScheme.surface) {
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
                            sniffedItems.clear()
                        }
                    ) {
                        Icon(Icons.Default.Home, contentDescription = "Home")
                    }

                    OutlinedTextField(
                        value = addressInput,
                        onValueChange = { addressInput = it },
                        modifier = Modifier.weight(1f),
                        singleLine = true,
                        placeholder = { Text("https://...") },
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
                        selected = useServerProxy,
                        onClick = {
                            val next = !useServerProxy
                            useServerProxy = next
                            if (currentUrl.isNotBlank()) loadTarget(currentUrl, forceProxy = next)
                        },
                        leadingIcon = { Icon(Icons.Default.Shield, contentDescription = null, modifier = Modifier.size(16.dp)) },
                        label = { Text(if (useServerProxy) "Proxy Mode ON" else "Proxy Mode OFF") }
                    )

                    AssistChip(
                        onClick = {
                            if (currentUrl.isNotBlank()) triggerServerSniff(currentUrl)
                            showSnifferSheet = true
                        },
                        leadingIcon = {
                            BadgedBox(
                                badge = {
                                    if (sniffedItems.isNotEmpty()) {
                                        Badge { Text("${sniffedItems.size}") }
                                    }
                                }
                            ) {
                                Icon(Icons.Default.Radar, contentDescription = null, modifier = Modifier.size(16.dp))
                            }
                        },
                        label = { Text(if (isSniffingServer) "Sniffing..." else "Live Sniffer") }
                    )

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
                            label = { Text("Download Page") }
                        )
                    }
                }
            }
        }

        // Viewport: Home Bookmarks vs Live WebView
        if (currentUrl.isBlank()) {
            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                contentPadding = PaddingValues(14.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                item {
                    Text("Quick Bookmarks", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "Tap any site or enter a URL above. Enable Proxy Mode if a domain is DNS-blocked on your network.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }

                val bookmarks = listOf(
                    Triple("YouTube", "https://m.youtube.com", false),
                    Triple("Tinhte.vn", "https://tinhte.vn", false),
                    Triple("AV01 Media (Proxy)", "https://av01.media", true),
                    Triple("SoundCloud", "https://soundcloud.com", false),
                    Triple("TikTok", "https://www.tiktok.com", false)
                )

                items(bookmarks) { (title, url, proxyRecommended) ->
                    Card(
                        modifier = Modifier
                            .fillMaxWidth()
                            .clickable {
                                useServerProxy = proxyRecommended
                                loadTarget(url, forceProxy = proxyRecommended)
                            },
                        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer),
                        shape = RoundedCornerShape(12.dp)
                    ) {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(14.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(12.dp)
                        ) {
                            Icon(Icons.Default.Language, contentDescription = null, tint = MaterialTheme.colorScheme.primary)
                            Column(modifier = Modifier.weight(1f)) {
                                Text(title, style = MaterialTheme.typography.titleSmall)
                                Text(url, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
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
                            onPageStarted = { pageUrl ->
                                isLoadingPage = true
                                if (!pageUrl.contains("/api/browser/proxy")) {
                                    currentUrl = pageUrl
                                    addressInput = pageUrl
                                }
                            },
                            onPageFinished = { view ->
                                isLoadingPage = false
                                canGoBack = view.canGoBack()
                                canGoForward = view.canGoForward()
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
                        strokeWidth = 2.5.dp
                    )
                }
            }
        }
    }

    // Live Sniffer Modal Sheet
    if (showSnifferSheet) {
        val visibleItems = sniffedItems.filter { sniffTab == "all" || it.type == sniffTab }

        ModalBottomSheet(onDismissRequest = { showSnifferSheet = false }) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(horizontal = 16.dp)
                    .padding(bottom = 24.dp),
                verticalArrangement = Arrangement.spacedBy(10.dp)
            ) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Text("Live Media Sniffer (${sniffedItems.size})", style = MaterialTheme.typography.titleMedium)
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

                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    listOf(
                        "video" to "Videos (${sniffedItems.count { it.type == "video" }})",
                        "audio" to "Audio (${sniffedItems.count { it.type == "audio" }})",
                        "image" to "Images (${sniffedItems.count { it.type == "image" }})",
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
                    Text(
                        text = if (isSniffingServer) "Scanning current page for videos and streams..." else "No media items found in this filter.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        modifier = Modifier.padding(vertical = 20.dp)
                    )
                } else {
                    LazyColumn(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(340.dp),
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        items(visibleItems, key = { it.url }) { item ->
                            val isSelected = selectedUrls.contains(item.url)
                            Surface(
                                shape = RoundedCornerShape(10.dp),
                                color = MaterialTheme.colorScheme.surfaceContainer,
                                tonalElevation = 2.dp
                            ) {
                                Column(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .padding(10.dp),
                                    verticalArrangement = Arrangement.spacedBy(6.dp)
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
                                                    .size(56.dp, 38.dp)
                                                    .clip(RoundedCornerShape(6.dp)),
                                                contentScale = ContentScale.Crop
                                            )
                                        }
                                        Column(modifier = Modifier.weight(1f)) {
                                            Text(item.title, style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                            Text(
                                                listOfNotNull(item.format, item.resolution, item.duration, item.source).joinToString(" · "),
                                                style = MaterialTheme.typography.labelSmall,
                                                color = MaterialTheme.colorScheme.onSurfaceVariant
                                            )
                                        }
                                    }

                                    Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                                        if (item.type != "image") {
                                            OutlinedButton(
                                                onClick = {
                                                    onPlayMedia(
                                                        PlaybackRequest(
                                                            title = item.title,
                                                            subtitle = item.channel ?: item.format,
                                                            streamUrl = RetrofitClient.browserStreamUrl(item.url),
                                                            onSaveToPhone = {
                                                                phoneDownloadManager.saveDirectMediaToPhone(item.url, item.title, item.type, item.channel)
                                                                onShowMessage("Saving stream to phone storage...")
                                                            }
                                                        )
                                                    )
                                                }
                                            ) {
                                                Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(16.dp))
                                                Text("Preview", modifier = Modifier.padding(start = 4.dp))
                                            }
                                        }

                                        OutlinedButton(
                                            onClick = {
                                                phoneDownloadManager.saveDirectMediaToPhone(
                                                    mediaUrl = item.url,
                                                    title = item.title,
                                                    type = item.type,
                                                    channel = item.channel
                                                )
                                                onShowMessage("Downloading directly to phone storage")
                                            }
                                        ) {
                                            Icon(Icons.Default.PhoneAndroid, contentDescription = null, modifier = Modifier.size(16.dp))
                                            Text("To Phone", modifier = Modifier.padding(start = 4.dp))
                                        }

                                        Button(
                                            onClick = {
                                                scope.launch {
                                                    RetrofitClient.getApi().addQueue(
                                                        QueueAddRequest(
                                                            items = listOf(
                                                                EnqueueItemInput(
                                                                    url = item.url,
                                                                    title = item.title,
                                                                    thumbnail = item.thumbnail,
                                                                    channel = item.channel,
                                                                    duration = item.duration
                                                                )
                                                            ),
                                                            format = formatQuality
                                                        )
                                                    )
                                                    onRefreshQueue()
                                                    onShowMessage("Queued on server: ${item.title}")
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
                        Button(
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
                                                        duration = it.duration
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
                        ) {
                            Text("Queue Selected as Group (${selectedUrls.size})")
                        }
                    }
                }
            }
        }
    }
}

@SuppressLint("SetJavaScriptEnabled")
private fun configureWebView(
    webView: WebView,
    onPageStarted: (String) -> Unit,
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
                        onMediaDetected(
                            SniffedMedia(
                                id = "wv_${u.hashCode()}",
                                url = u,
                                type = type,
                                format = fmt.ifBlank { type.uppercase() },
                                title = title.ifBlank { "Detected Media" },
                                thumbnail = thumb,
                                source = "dom",
                                isStream = u.contains(".m3u8") || u.contains(".mpd")
                            )
                        )
                    }
                } catch (_: Exception) {}
            }
        },
        "KVSniffer"
    )

    webView.webChromeClient = WebChromeClient()
    webView.webViewClient = object : WebViewClient() {
        override fun shouldOverrideUrlLoading(view: WebView?, request: WebResourceRequest?): Boolean {
            return false
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
