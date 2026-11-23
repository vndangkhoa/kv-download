package com.kvdownload.app.player

import android.content.Context
import android.net.Uri
import android.webkit.CookieManager
import androidx.annotation.OptIn
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.MediaItem
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.VideoSize
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.DefaultDataSource
import androidx.media3.datasource.FileDataSource
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.exoplayer.DefaultLoadControl
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.exoplayer.source.MediaSource
import androidx.media3.extractor.DefaultExtractorsFactory
import androidx.media3.ui.AspectRatioFrameLayout
import com.kvdownload.app.api.AV01Resolver
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.StreamVariant
import com.kvdownload.app.storage.HlsSegmentDownloader
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import java.io.File
import java.net.URI
import java.util.UUID

enum class MediaKind {
    VIDEO,
    AUDIO,
    IMAGE
}

data class MediaPlayRequest(
    val id: String = UUID.randomUUID().toString(),
    val title: String,
    val subtitle: String? = null,
    val thumbnailUrl: String? = null,
    val primaryUrl: String,
    val fallbackUrl: String? = null,
    val kind: MediaKind = MediaKind.VIDEO,
    val isLocalUri: Boolean = false,
    val isHls: Boolean = false,
    val referer: String? = null,
    val headers: Map<String, String> = emptyMap(),
    val variants: List<StreamVariant> = emptyList(),
    val onSaveToPhone: (() -> Unit)? = null
)

@OptIn(UnstableApi::class)
class AppPlayerController(context: Context) {

    private val appContext = context.applicationContext
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    val exoPlayer: ExoPlayer = ExoPlayer.Builder(
        appContext,
        DefaultRenderersFactory(appContext).setEnableDecoderFallback(true)
    )
        .setLoadControl(
            DefaultLoadControl.Builder()
                .setBufferDurationsMs(10_000, 35_000, 500, 1_500)
                .build()
        )
        .build()
        .apply {
            setAudioAttributes(
                AudioAttributes.Builder()
                    .setUsage(C.USAGE_MEDIA)
                    .setContentType(C.AUDIO_CONTENT_TYPE_MOVIE)
                    .build(),
                true
            )
            setHandleAudioBecomingNoisy(true)
        }

    private val _currentRequest = MutableStateFlow<MediaPlayRequest?>(null)
    val currentRequest: StateFlow<MediaPlayRequest?> = _currentRequest.asStateFlow()

    private val _photoLightboxRequest = MutableStateFlow<MediaPlayRequest?>(null)
    val photoLightboxRequest: StateFlow<MediaPlayRequest?> = _photoLightboxRequest.asStateFlow()

    private val _isExpanded = MutableStateFlow(false)
    val isExpanded: StateFlow<Boolean> = _isExpanded.asStateFlow()

    private val _isFullscreen = MutableStateFlow(false)
    val isFullscreen: StateFlow<Boolean> = _isFullscreen.asStateFlow()

    private val _isPlaying = MutableStateFlow(false)
    val isPlaying: StateFlow<Boolean> = _isPlaying.asStateFlow()

    private val _isBuffering = MutableStateFlow(false)
    val isBuffering: StateFlow<Boolean> = _isBuffering.asStateFlow()

    private val _isEnded = MutableStateFlow(false)
    val isEnded: StateFlow<Boolean> = _isEnded.asStateFlow()

    private val _currentPositionMs = MutableStateFlow(0L)
    val currentPositionMs: StateFlow<Long> = _currentPositionMs.asStateFlow()

    private val _durationMs = MutableStateFlow(0L)
    val durationMs: StateFlow<Long> = _durationMs.asStateFlow()

    private val _bufferedPercent = MutableStateFlow(0)
    val bufferedPercent: StateFlow<Int> = _bufferedPercent.asStateFlow()

    private val _playbackSpeed = MutableStateFlow(1f)
    val playbackSpeed: StateFlow<Float> = _playbackSpeed.asStateFlow()

    private val _isMuted = MutableStateFlow(false)
    val isMuted: StateFlow<Boolean> = _isMuted.asStateFlow()

    private val _resizeMode = MutableStateFlow(AspectRatioFrameLayout.RESIZE_MODE_FIT)
    val resizeMode: StateFlow<Int> = _resizeMode.asStateFlow()

    private val _videoWidth = MutableStateFlow(16)
    val videoWidth: StateFlow<Int> = _videoWidth.asStateFlow()

    private val _videoHeight = MutableStateFlow(9)
    val videoHeight: StateFlow<Int> = _videoHeight.asStateFlow()

    private val _selectedVariant = MutableStateFlow<StreamVariant?>(null)
    val selectedVariant: StateFlow<StreamVariant?> = _selectedVariant.asStateFlow()

    private val _errorMessage = MutableStateFlow<String?>(null)
    val errorMessage: StateFlow<String?> = _errorMessage.asStateFlow()

    private var activeStreamUrl: String = ""
    private var triedFallback: Boolean = false
    private var retriedStallOnce: Boolean = false
    private var bufferingStallMs: Long = 0L
    private var lastTickerPosMs: Long = -1L
    private var resolveJob: Job? = null
    private var tickerJob: Job? = null

    private val playerListener = object : Player.Listener {
        override fun onIsPlayingChanged(isPlaying: Boolean) {
            _isPlaying.value = isPlaying
            if (isPlaying) {
                _isBuffering.value = false
                bufferingStallMs = 0L
            }
        }

        override fun onRenderedFirstFrame() {
            val state = exoPlayer.playbackState
            _isBuffering.value = state == Player.STATE_BUFFERING && !exoPlayer.isPlaying
        }

        override fun onPlaybackStateChanged(playbackState: Int) {
            _isBuffering.value = playbackState == Player.STATE_BUFFERING && !exoPlayer.isPlaying
            _isEnded.value = playbackState == Player.STATE_ENDED
            if (playbackState == Player.STATE_READY || playbackState == Player.STATE_ENDED) {
                bufferingStallMs = 0L
            }
            val dur = exoPlayer.duration
            if (dur != C.TIME_UNSET && dur > 0L) {
                _durationMs.value = dur
            }
        }

        override fun onVideoSizeChanged(videoSize: VideoSize) {
            if (videoSize.width > 0 && videoSize.height > 0) {
                _videoWidth.value = videoSize.width
                _videoHeight.value = videoSize.height
            }
        }

        override fun onPlayerError(error: PlaybackException) {
            bufferingStallMs = 0L
            val req = _currentRequest.value
            val fallback = req?.fallbackUrl?.takeIf { it.isNotBlank() && it != activeStreamUrl }
            if (req != null && !triedFallback && fallback != null) {
                triedFallback = true
                _errorMessage.value = null
                _isBuffering.value = true
                val resumeMs = exoPlayer.currentPosition.coerceAtLeast(0L)
                loadPlaybackUrl(
                    uriStr = fallback,
                    forceHls = req.isHls || HlsSegmentDownloader.looksLikeHlsUrl(fallback),
                    referer = req.referer,
                    extraHeaders = req.headers,
                    seekPositionMs = resumeMs
                )
            } else {
                _isBuffering.value = false
                _errorMessage.value = error.localizedMessage ?: error.errorCodeName
            }
        }
    }

    init {
        exoPlayer.addListener(playerListener)
        startTicker()
    }

    private fun startTicker() {
        tickerJob?.cancel()
        tickerJob = scope.launch {
            while (isActive) {
                val req = _currentRequest.value
                if (req != null) {
                    val posNow = exoPlayer.currentPosition.coerceAtLeast(0L)
                    _currentPositionMs.value = posNow
                    val d = exoPlayer.duration
                    _durationMs.value = if (d != C.TIME_UNSET && d > 0L) d else 0L
                    _bufferedPercent.value = exoPlayer.bufferedPercentage.coerceIn(0, 100)

                    val state = exoPlayer.playbackState
                    val playingNow = exoPlayer.isPlaying
                    _isPlaying.value = playingNow
                    _isBuffering.value = state == Player.STATE_BUFFERING && !playingNow

                    if (state == Player.STATE_BUFFERING && exoPlayer.playWhenReady && !playingNow && _errorMessage.value == null) {
                        if (posNow == lastTickerPosMs) {
                            bufferingStallMs += 250L
                        } else {
                            bufferingStallMs = 0L
                        }
                        if (bufferingStallMs >= 10_000L) {
                            bufferingStallMs = 0L
                            val fallback = req.fallbackUrl?.takeIf { it.isNotBlank() && it != activeStreamUrl }
                            if (!triedFallback && fallback != null) {
                                triedFallback = true
                                _errorMessage.value = null
                                loadPlaybackUrl(
                                    uriStr = fallback,
                                    forceHls = req.isHls || HlsSegmentDownloader.looksLikeHlsUrl(fallback),
                                    referer = req.referer,
                                    extraHeaders = req.headers,
                                    seekPositionMs = posNow
                                )
                            } else if (!retriedStallOnce) {
                                retriedStallOnce = true
                                exoPlayer.prepare()
                                exoPlayer.play()
                            } else {
                                _isBuffering.value = false
                                _errorMessage.value = "Playback stalled while buffering. Tap Retry."
                            }
                        }
                    } else {
                        bufferingStallMs = 0L
                    }
                    lastTickerPosMs = posNow
                }
                delay(250L)
            }
        }
    }

    fun open(request: MediaPlayRequest) {
        if (request.kind == MediaKind.IMAGE) {
            _photoLightboxRequest.value = request
            return
        }

        resolveJob?.cancel()
        _photoLightboxRequest.value = null
        _currentRequest.value = request
        _isExpanded.value = true
        _errorMessage.value = null
        _isEnded.value = false
        _currentPositionMs.value = 0L
        _durationMs.value = 0L
        triedFallback = false
        retriedStallOnce = false
        bufferingStallMs = 0L

        val initialVariants = request.variants
        val initialVariant = initialVariants.firstOrNull { it.url == request.primaryUrl } ?: initialVariants.firstOrNull()
        _selectedVariant.value = initialVariant

        if (AV01Resolver.isAV01Url(request.primaryUrl) && request.variants.isEmpty()) {
            _isBuffering.value = true
            exoPlayer.stop()
            resolveJob = scope.launch {
                try {
                    val resolved = AV01Resolver.resolve(request.primaryUrl)
                    val enriched = request.copy(
                        title = if (request.title.startsWith("http") || request.title.startsWith("AV01 #")) resolved.title else request.title,
                        subtitle = request.subtitle ?: "AV01 HLS",
                        thumbnailUrl = request.thumbnailUrl ?: resolved.thumbnail,
                        variants = resolved.variants,
                        referer = resolved.referer,
                        isHls = true
                    )
                    val top = resolved.variants.first()
                    _currentRequest.value = enriched
                    _selectedVariant.value = top
                    loadPlaybackUrl(
                        uriStr = top.url,
                        forceHls = true,
                        referer = resolved.referer,
                        extraHeaders = request.headers,
                        seekPositionMs = 0L
                    )
                } catch (e: Exception) {
                    val fb = request.fallbackUrl
                    if (!fb.isNullOrBlank()) {
                        triedFallback = true
                        loadPlaybackUrl(
                            uriStr = fb,
                            forceHls = true,
                            referer = request.referer ?: AV01Resolver.REFERER,
                            extraHeaders = request.headers,
                            seekPositionMs = 0L
                        )
                    } else {
                        _isBuffering.value = false
                        _errorMessage.value = e.message ?: "Unable to resolve AV01 stream"
                    }
                }
            }
            return
        }

        val targetUrl = initialVariant?.url ?: request.primaryUrl
        loadPlaybackUrl(
            uriStr = targetUrl,
            forceHls = request.isHls || request.variants.isNotEmpty() || HlsSegmentDownloader.looksLikeHlsUrl(targetUrl),
            referer = request.referer,
            extraHeaders = request.headers,
            seekPositionMs = 0L
        )
    }

    fun selectVariant(variant: StreamVariant) {
        val req = _currentRequest.value ?: return
        _selectedVariant.value = variant
        _errorMessage.value = null
        val resumeMs = exoPlayer.currentPosition.coerceAtLeast(0L)
        loadPlaybackUrl(
            uriStr = variant.url,
            forceHls = true,
            referer = req.referer,
            extraHeaders = req.headers,
            seekPositionMs = resumeMs
        )
    }

    fun retry() {
        val req = _currentRequest.value ?: return
        triedFallback = false
        retriedStallOnce = false
        bufferingStallMs = 0L
        retriedStallOnce = false
        bufferingStallMs = 0L
        _errorMessage.value = null
        val resumeMs = exoPlayer.currentPosition.coerceAtLeast(0L)
        val url = _selectedVariant.value?.url ?: req.primaryUrl
        loadPlaybackUrl(
            uriStr = url,
            forceHls = req.isHls || req.variants.isNotEmpty() || HlsSegmentDownloader.looksLikeHlsUrl(url),
            referer = req.referer,
            extraHeaders = req.headers,
            seekPositionMs = resumeMs
        )
    }

    private fun loadPlaybackUrl(
        uriStr: String,
        forceHls: Boolean,
        referer: String?,
        extraHeaders: Map<String, String>,
        seekPositionMs: Long
    ) {
        activeStreamUrl = uriStr
        bufferingStallMs = 0L
        _isBuffering.value = true
        try {
            val mediaSource = buildMediaSource(uriStr, forceHls, referer, extraHeaders)
            exoPlayer.setMediaSource(mediaSource)
            if (seekPositionMs > 0L) {
                exoPlayer.seekTo(seekPositionMs)
            }
            exoPlayer.prepare()
            exoPlayer.playWhenReady = true
        } catch (e: Exception) {
            _isBuffering.value = false
            _errorMessage.value = e.message ?: "Unable to open media file"
        }
    }

    private fun buildMediaSource(
        uriStr: String,
        forceHls: Boolean,
        referer: String?,
        extraHeaders: Map<String, String>
    ): MediaSource {
        val extractorsFactory = DefaultExtractorsFactory()
            .setConstantBitrateSeekingEnabled(true)

        val isLocalFile = uriStr.startsWith("file://") || uriStr.startsWith("/")
        if (isLocalFile) {
            val path = if (uriStr.startsWith("file://")) Uri.parse(uriStr).path.orEmpty() else uriStr
            val localFile = File(path)
            if (!localFile.exists() || localFile.length() <= 0L) {
                throw IllegalStateException("Local video file is missing or empty")
            }
            val item = MediaItem.Builder()
                .setUri(Uri.fromFile(localFile))
                .build()
            return DefaultMediaSourceFactory(FileDataSource.Factory(), extractorsFactory)
                .createMediaSource(item)
        }

        val effHeaders = linkedMapOf<String, String>()
        effHeaders["Accept"] = "*/*"
        val effRef = referer?.takeIf { it.isNotBlank() }
            ?: if (uriStr.contains("iw01.xyz", ignoreCase = true)) AV01Resolver.REFERER else null
        if (!effRef.isNullOrBlank()) {
            effHeaders["Referer"] = effRef
            try {
                val parsed = URI.create(effRef)
                if (!parsed.scheme.isNullOrBlank() && !parsed.host.isNullOrBlank()) {
                    effHeaders["Origin"] = "${parsed.scheme}://${parsed.host}"
                }
            } catch (_: Exception) {}
        }
        try {
            if (uriStr.startsWith("http://") || uriStr.startsWith("https://")) {
                CookieManager.getInstance().getCookie(uriStr)?.takeIf { it.isNotBlank() }?.let {
                    effHeaders["Cookie"] = it
                }
            }
        } catch (_: Exception) {}
        effHeaders.putAll(extraHeaders)

        val okHttpFactory = OkHttpDataSource.Factory(RetrofitClient.playerHttpClient)
            .setUserAgent(DEFAULT_UA)
            .setDefaultRequestProperties(effHeaders)
        val dataSourceFactory = DefaultDataSource.Factory(appContext, okHttpFactory)

        val itemBuilder = MediaItem.Builder().setUri(Uri.parse(uriStr))
        val cleanPath = uriStr.substringBefore('?').substringBefore('#').lowercase()
        val useHls = forceHls || cleanPath.endsWith(".m3u8") || HlsSegmentDownloader.looksLikeHlsUrl(uriStr)
        if (useHls) {
            itemBuilder.setMimeType(MimeTypes.APPLICATION_M3U8)
        } else if (cleanPath.endsWith(".mpd")) {
            itemBuilder.setMimeType(MimeTypes.APPLICATION_MPD)
        }

        return DefaultMediaSourceFactory(dataSourceFactory, extractorsFactory)
            .createMediaSource(itemBuilder.build())
    }

    fun togglePlayPause() {
        if (exoPlayer.playbackState == Player.STATE_ENDED) {
            exoPlayer.seekTo(0L)
            exoPlayer.play()
        } else if (exoPlayer.isPlaying) {
            exoPlayer.pause()
        } else {
            exoPlayer.play()
        }
    }

    fun seekTo(positionMs: Long) {
        val maxDur = _durationMs.value
        val clamped = if (maxDur > 0L) positionMs.coerceIn(0L, maxDur) else positionMs.coerceAtLeast(0L)
        exoPlayer.seekTo(clamped)
        _currentPositionMs.value = clamped
    }

    fun seekRelative(deltaMs: Long) {
        seekTo(exoPlayer.currentPosition + deltaMs)
    }

    fun setSpeed(speed: Float) {
        val clamped = speed.coerceIn(0.25f, 3.0f)
        exoPlayer.setPlaybackSpeed(clamped)
        _playbackSpeed.value = clamped
    }

    fun toggleMute() {
        val next = !_isMuted.value
        _isMuted.value = next
        exoPlayer.volume = if (next) 0f else 1f
    }

    fun cycleResizeMode() {
        _resizeMode.value = when (_resizeMode.value) {
            AspectRatioFrameLayout.RESIZE_MODE_FIT -> AspectRatioFrameLayout.RESIZE_MODE_ZOOM
            AspectRatioFrameLayout.RESIZE_MODE_ZOOM -> AspectRatioFrameLayout.RESIZE_MODE_FILL
            else -> AspectRatioFrameLayout.RESIZE_MODE_FIT
        }
    }

    fun resizeModeLabel(mode: Int): String = when (mode) {
        AspectRatioFrameLayout.RESIZE_MODE_ZOOM -> "Crop"
        AspectRatioFrameLayout.RESIZE_MODE_FILL -> "Stretch"
        else -> "Fit"
    }

    fun minimizeToMiniPlayer() {
        _isFullscreen.value = false
        _isExpanded.value = false
    }

    fun expandPlayer() {
        if (_currentRequest.value != null) {
            _isExpanded.value = true
        }
    }

    fun setFullscreen(fullscreen: Boolean) {
        _isFullscreen.value = fullscreen
        if (fullscreen && _currentRequest.value != null) {
            _isExpanded.value = true
        }
    }

    fun close() {
        resolveJob?.cancel()
        exoPlayer.stop()
        exoPlayer.clearMediaItems()
        _isFullscreen.value = false
        _isExpanded.value = false
        _currentRequest.value = null
        _selectedVariant.value = null
        _errorMessage.value = null
        _currentPositionMs.value = 0L
        _durationMs.value = 0L
    }

    fun closePhotoLightbox() {
        _photoLightboxRequest.value = null
    }

    fun release() {
        resolveJob?.cancel()
        tickerJob?.cancel()
        exoPlayer.removeListener(playerListener)
        exoPlayer.release()
    }

    companion object {
        private const val DEFAULT_UA =
            "Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36"

        fun formatDurationMs(ms: Long): String {
            if (ms <= 0L) return "0:00"
            val totalSeconds = ms / 1000L
            val s = totalSeconds % 60L
            val m = (totalSeconds / 60L) % 60L
            val h = totalSeconds / 3600L
            return if (h > 0L) "%d:%02d:%02d".format(h, m, s) else "%d:%02d".format(m, s)
        }
    }
}
