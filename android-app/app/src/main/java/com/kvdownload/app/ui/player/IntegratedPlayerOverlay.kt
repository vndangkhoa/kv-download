package com.kvdownload.app.ui.player

import android.app.PictureInPictureParams
import android.content.pm.ActivityInfo
import android.os.Build
import android.util.Rational
import android.view.LayoutInflater
import androidx.activity.compose.BackHandler
import androidx.annotation.OptIn
import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.gestures.detectTapGestures
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.AspectRatio
import androidx.compose.material.icons.filled.Audiotrack
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.Forward10
import androidx.compose.material.icons.filled.Fullscreen
import androidx.compose.material.icons.filled.FullscreenExit
import androidx.compose.material.icons.filled.HighQuality
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.material.icons.filled.Pause
import androidx.compose.material.icons.filled.PictureInPictureAlt
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Replay
import androidx.compose.material.icons.filled.Replay10
import androidx.compose.material.icons.filled.SaveAlt
import androidx.compose.material.icons.filled.Speed
import androidx.compose.material.icons.filled.VolumeOff
import androidx.compose.material.icons.filled.VolumeUp
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Brush
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.input.pointer.pointerInput
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.compose.ui.viewinterop.AndroidView
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.media3.common.util.UnstableApi
import androidx.media3.ui.PlayerView
import coil.compose.AsyncImage
import com.kvdownload.app.R
import com.kvdownload.app.player.AppPlayerController
import com.kvdownload.app.player.MediaKind
import com.kvdownload.app.player.MediaPlayRequest
import com.kvdownload.app.ui.adaptive.LocalWindowState
import com.kvdownload.app.ui.adaptive.findActivity
import com.kvdownload.app.ui.components.AstryxCard
import com.kvdownload.app.ui.components.AstryxStatusBadge
import com.kvdownload.app.ui.components.AstryxTone
import com.kvdownload.app.ui.theme.LocalAstryxPalette
import kotlinx.coroutines.delay

@OptIn(UnstableApi::class)
@Composable
fun IntegratedPlayerOverlay(
    controller: AppPlayerController,
    recentPlayable: List<MediaPlayRequest> = emptyList(),
    isInPipMode: Boolean = false
) {
    val req by controller.currentRequest.collectAsState()
    val isExpanded by controller.isExpanded.collectAsState()
    if (req == null || !isExpanded) return

    val active = req!!
    val context = LocalContext.current
    val activity = remember(context) { context.findActivity() }
    val windowState = LocalWindowState.current
    val palette = LocalAstryxPalette.current

    val isFullscreen by controller.isFullscreen.collectAsState()
    val isPlaying by controller.isPlaying.collectAsState()
    val isBuffering by controller.isBuffering.collectAsState()
    val isEnded by controller.isEnded.collectAsState()
    val currentPos by controller.currentPositionMs.collectAsState()
    val duration by controller.durationMs.collectAsState()
    val speed by controller.playbackSpeed.collectAsState()
    val isMuted by controller.isMuted.collectAsState()
    val resizeMode by controller.resizeMode.collectAsState()
    val videoWidth by controller.videoWidth.collectAsState()
    val videoHeight by controller.videoHeight.collectAsState()
    val selectedVariant by controller.selectedVariant.collectAsState()
    val errorMsg by controller.errorMessage.collectAsState()

    var showControls by remember(active.id) { mutableStateOf(true) }
    var interactionTick by remember { mutableIntStateOf(0) }
    var seekIndicator by remember { mutableStateOf<String?>(null) }

    LaunchedEffect(showControls, isPlaying, interactionTick) {
        if (showControls && isPlaying) {
            delay(3500L)
            showControls = false
        }
    }

    LaunchedEffect(seekIndicator) {
        if (seekIndicator != null) {
            delay(750L)
            seekIndicator = null
        }
    }

    // Manage landscape sensor orientation & immersive system bars when Fullscreen is active
    DisposableEffect(activity, isFullscreen, isInPipMode) {
        val window = activity?.window
        if (activity != null && window != null) {
            val controllerCompat = WindowCompat.getInsetsController(window, window.decorView)
            if (isFullscreen && !isInPipMode) {
                activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_SENSOR_LANDSCAPE
                controllerCompat.systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
                controllerCompat.hide(WindowInsetsCompat.Type.systemBars())
            } else {
                activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
                controllerCompat.show(WindowInsetsCompat.Type.systemBars())
            }
        }
        onDispose {
            if (activity != null && activity.window != null) {
                activity.requestedOrientation = ActivityInfo.SCREEN_ORIENTATION_UNSPECIFIED
                WindowCompat.getInsetsController(activity.window, activity.window.decorView)
                    .show(WindowInsetsCompat.Type.systemBars())
            }
        }
    }

    BackHandler(enabled = !isInPipMode) {
        if (isFullscreen) {
            controller.setFullscreen(false)
        } else {
            controller.minimizeToMiniPlayer()
        }
    }

    fun triggerPip() {
        if (activity != null && Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            val aspectW = videoWidth.coerceAtLeast(16)
            val aspectH = videoHeight.coerceAtLeast(9)
            val params = PictureInPictureParams.Builder()
                .setAspectRatio(Rational(aspectW, aspectH))
                .build()
            activity.enterPictureInPictureMode(params)
        }
    }

    @Composable
    fun VideoSurfaceBox(
        modifier: Modifier,
        showOverlayControls: Boolean = !isInPipMode
    ) {
        Box(
            modifier = modifier
                .background(Color.Black)
                .pointerInput(active.id) {
                    detectTapGestures(
                        onTap = {
                            showControls = !showControls
                            interactionTick++
                        },
                        onDoubleTap = { offset ->
                            val third = size.width / 3f
                            when {
                                offset.x < third -> {
                                    controller.seekRelative(-10_000L)
                                    seekIndicator = "-10s"
                                    interactionTick++
                                }
                                offset.x > size.width - third -> {
                                    controller.seekRelative(10_000L)
                                    seekIndicator = "+10s"
                                    interactionTick++
                                }
                                else -> {
                                    controller.togglePlayPause()
                                    interactionTick++
                                }
                            }
                        }
                    )
                }
        ) {
            if (active.kind == MediaKind.AUDIO) {
                // Dedicated rich Audio Player Canvas
                Box(
                    modifier = Modifier
                        .fillMaxSize()
                        .background(
                            Brush.radialGradient(
                                listOf(palette.accent.copy(alpha = 0.28f), Color(0xFF0A0B10))
                            )
                        ),
                    contentAlignment = Alignment.Center
                ) {
                    Column(
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(12.dp),
                        modifier = Modifier.padding(24.dp)
                    ) {
                        Box(
                            modifier = Modifier
                                .size(116.dp)
                                .clip(RoundedCornerShape(20.dp))
                                .background(palette.accentContainer),
                            contentAlignment = Alignment.Center
                        ) {
                            if (!active.thumbnailUrl.isNullOrBlank()) {
                                AsyncImage(
                                    model = active.thumbnailUrl,
                                    contentDescription = null,
                                    modifier = Modifier.fillMaxSize(),
                                    contentScale = ContentScale.Crop
                                )
                            } else {
                                Icon(
                                    imageVector = Icons.Default.Audiotrack,
                                    contentDescription = null,
                                    tint = palette.accentText,
                                    modifier = Modifier.size(52.dp)
                                )
                            }
                        }
                        if (!isFullscreen) {
                            Text(
                                text = active.title,
                                color = Color.White,
                                style = MaterialTheme.typography.titleSmall,
                                maxLines = 1,
                                overflow = TextOverflow.Ellipsis
                            )
                        }
                    }
                }
            } else {
                AndroidView(
                    factory = { ctx ->
                        (LayoutInflater.from(ctx).inflate(R.layout.view_kv_player, null, false) as PlayerView).apply {
                            player = controller.exoPlayer
                            this.resizeMode = resizeMode
                        }
                    },
                    update = { view ->
                        if (view.player !== controller.exoPlayer) {
                            view.player = controller.exoPlayer
                        }
                        if (view.resizeMode != resizeMode) {
                            view.resizeMode = resizeMode
                        }
                    },
                    onRelease = { view ->
                        if (view.player === controller.exoPlayer) {
                            view.player = null
                        }
                    },
                    modifier = Modifier.fillMaxSize()
                )
            }

            if (isBuffering && errorMsg == null && !isInPipMode && !(showOverlayControls && (showControls || isEnded))) {
                CircularProgressIndicator(
                    modifier = Modifier.align(Alignment.Center),
                    color = palette.accent
                )
            }

            seekIndicator?.let { badge ->
                Surface(
                    modifier = Modifier.align(Alignment.Center),
                    shape = RoundedCornerShape(999.dp),
                    color = Color.Black.copy(alpha = 0.72f)
                ) {
                    Text(
                        text = badge,
                        modifier = Modifier.padding(horizontal = 18.dp, vertical = 10.dp),
                        color = Color.White,
                        style = MaterialTheme.typography.titleMedium,
                        fontWeight = FontWeight.Bold
                    )
                }
            }

            if (errorMsg != null && !isInPipMode) {
                Surface(
                    modifier = Modifier
                        .align(Alignment.Center)
                        .padding(16.dp),
                    shape = RoundedCornerShape(14.dp),
                    color = Color(0xE6141622),
                    border = BorderStroke(1.dp, palette.error.copy(alpha = 0.5f))
                ) {
                    Column(
                        modifier = Modifier.padding(16.dp),
                        horizontalAlignment = Alignment.CenterHorizontally,
                        verticalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        Text(
                            text = "Playback issue",
                            color = palette.error,
                            style = MaterialTheme.typography.titleSmall
                        )
                        Text(
                            text = errorMsg ?: "",
                            color = Color.White.copy(alpha = 0.85f),
                            style = MaterialTheme.typography.bodySmall,
                            maxLines = 3,
                            overflow = TextOverflow.Ellipsis
                        )
                        Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                            Button(onClick = { controller.retry() }) {
                                Icon(Icons.Default.Refresh, contentDescription = null, modifier = Modifier.size(16.dp))
                                Text("Retry", modifier = Modifier.padding(start = 6.dp))
                            }
                            OutlinedButton(onClick = { controller.minimizeToMiniPlayer() }) {
                                Text("Minimize", color = Color.White)
                            }
                        }
                    }
                }
            }

            if (showOverlayControls) {
                AnimatedVisibility(
                    visible = showControls || isEnded,
                    enter = fadeIn(),
                    exit = fadeOut(),
                    modifier = Modifier.fillMaxSize()
                ) {
                    PlayerTransportControlsOverlay(
                        active = active,
                        isPlaying = isPlaying,
                        isBuffering = isBuffering,
                        isEnded = isEnded,
                        currentPos = currentPos,
                        duration = duration,
                        speed = speed,
                        isMuted = isMuted,
                        resizeLabel = controller.resizeModeLabel(resizeMode),
                        isFullscreen = isFullscreen,
                        selectedQualityLabel = selectedVariant?.label,
                        onMinimize = {
                            if (isFullscreen) controller.setFullscreen(false)
                            else controller.minimizeToMiniPlayer()
                        },
                        onClose = { controller.close() },
                        onTogglePlay = {
                            controller.togglePlayPause()
                            interactionTick++
                        },
                        onSeekRelative = { delta ->
                            controller.seekRelative(delta)
                            seekIndicator = if (delta < 0) "-10s" else "+10s"
                            interactionTick++
                        },
                        onSeekTo = { ms ->
                            controller.seekTo(ms)
                            interactionTick++
                        },
                        onToggleMute = {
                            controller.toggleMute()
                            interactionTick++
                        },
                        onSpeedSelected = { sp ->
                            controller.setSpeed(sp)
                            interactionTick++
                        },
                        onVariantSelected = { v ->
                            controller.selectVariant(v)
                            interactionTick++
                        },
                        onCycleResize = {
                            controller.cycleResizeMode()
                            interactionTick++
                        },
                        onToggleFullscreen = {
                            controller.setFullscreen(!isFullscreen)
                            interactionTick++
                        },
                        onTriggerPip = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O && active.kind == MediaKind.VIDEO) {
                            { triggerPip() }
                        } else null
                    )
                }
            }
        }
    }

    // Main responsive container
    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(if (isFullscreen || isInPipMode) Color.Black else palette.body)
    ) {
        when {
            isInPipMode || isFullscreen -> {
                VideoSurfaceBox(
                    modifier = Modifier.fillMaxSize(),
                    showOverlayControls = !isInPipMode
                )
            }

            windowState.isTabletop -> {
                // Foldable Tabletop: Cinema above hinge, Tactile Deck below hinge
                val topPaneHeight = (windowState.hingeBoundsDp?.top ?: 260).dp
                Column(modifier = Modifier.fillMaxSize()) {
                    VideoSurfaceBox(
                        modifier = Modifier
                            .fillMaxWidth()
                            .height(topPaneHeight),
                        showOverlayControls = false
                    )
                    TabletopControlDeck(
                        active = active,
                        controller = controller,
                        isPlaying = isPlaying,
                        isEnded = isEnded,
                        currentPos = currentPos,
                        duration = duration,
                        speed = speed,
                        isMuted = isMuted,
                        selectedQualityLabel = selectedVariant?.label,
                        modifier = Modifier
                            .weight(1f)
                            .fillMaxWidth()
                            .navigationBarsPadding()
                    )
                }
            }

            windowState.isTwoPane -> {
                // Tablet / Foldable Book / Wide Landscape: 2-Pane Theater View
                Row(
                    modifier = Modifier
                        .fillMaxSize()
                        .statusBarsPadding()
                        .navigationBarsPadding()
                        .padding(14.dp),
                    horizontalArrangement = Arrangement.spacedBy(14.dp)
                ) {
                    Column(
                        modifier = Modifier
                            .weight(0.64f)
                            .fillMaxHeight(),
                        verticalArrangement = Arrangement.spacedBy(12.dp)
                    ) {
                        VideoSurfaceBox(
                            modifier = Modifier
                                .fillMaxWidth()
                                .aspectRatio(16f / 9f)
                                .clip(RoundedCornerShape(16.dp))
                        )
                        PlayerDetailsCard(
                            active = active,
                            selectedQualityLabel = selectedVariant?.label,
                            onSelectVariant = { controller.selectVariant(it) },
                            onSaveToPhone = active.onSaveToPhone,
                            onFullscreen = { controller.setFullscreen(true) },
                            onMinimize = { controller.minimizeToMiniPlayer() },
                            onClose = { controller.close() }
                        )
                    }

                    PlayerUpNextPane(
                        recentPlayable = recentPlayable.filter { it.id != active.id && it.kind != MediaKind.IMAGE },
                        onSelect = { controller.open(it) },
                        modifier = Modifier
                            .weight(0.36f)
                            .fillMaxHeight()
                    )
                }
            }

            else -> {
                // Compact Portrait Phone: 16:9 Player on top + Details & Up Next below
                Column(
                    modifier = Modifier
                        .fillMaxSize()
                        .statusBarsPadding()
                        .navigationBarsPadding()
                ) {
                    VideoSurfaceBox(
                        modifier = Modifier
                            .fillMaxWidth()
                            .aspectRatio(16f / 9f)
                    )

                    LazyColumn(
                        modifier = Modifier
                            .weight(1f)
                            .fillMaxWidth(),
                        contentPadding = PaddingValues(14.dp),
                        verticalArrangement = Arrangement.spacedBy(12.dp)
                    ) {
                        item {
                            PlayerDetailsCard(
                                active = active,
                                selectedQualityLabel = selectedVariant?.label,
                                onSelectVariant = { controller.selectVariant(it) },
                                onSaveToPhone = active.onSaveToPhone,
                                onFullscreen = { controller.setFullscreen(true) },
                                onMinimize = { controller.minimizeToMiniPlayer() },
                                onClose = { controller.close() }
                            )
                        }

                        val upNext = recentPlayable.filter { it.id != active.id && it.kind != MediaKind.IMAGE }
                        if (upNext.isNotEmpty()) {
                            item {
                                Text(
                                    text = "Up Next & Quick Switch",
                                    style = MaterialTheme.typography.titleSmall,
                                    color = palette.textPrimary
                                )
                            }
                            items(upNext.take(12), key = { it.id + it.primaryUrl }) { item ->
                                UpNextRowCard(item = item, onClick = { controller.open(item) })
                            }
                        }
                    }
                }
            }
        }
    }
}

@Composable
private fun PlayerTransportControlsOverlay(
    active: MediaPlayRequest,
    isPlaying: Boolean,
    isBuffering: Boolean,
    isEnded: Boolean,
    currentPos: Long,
    duration: Long,
    speed: Float,
    isMuted: Boolean,
    resizeLabel: String,
    isFullscreen: Boolean,
    selectedQualityLabel: String?,
    onMinimize: () -> Unit,
    onClose: () -> Unit,
    onTogglePlay: () -> Unit,
    onSeekRelative: (Long) -> Unit,
    onSeekTo: (Long) -> Unit,
    onToggleMute: () -> Unit,
    onSpeedSelected: (Float) -> Unit,
    onVariantSelected: (com.kvdownload.app.api.StreamVariant) -> Unit,
    onCycleResize: () -> Unit,
    onToggleFullscreen: () -> Unit,
    onTriggerPip: (() -> Unit)?
) {
    var showSpeedMenu by remember { mutableStateOf(false) }
    var showQualityMenu by remember { mutableStateOf(false) }
    var isDragging by remember { mutableStateOf(false) }
    var dragFraction by remember { mutableFloatStateOf(0f) }

    val sliderFraction = if (isDragging) {
        dragFraction
    } else if (duration > 0L) {
        (currentPos.toFloat() / duration.toFloat()).coerceIn(0f, 1f)
    } else 0f

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(Color.Black.copy(alpha = 0.36f))
    ) {
        // Top gradient bar
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .align(Alignment.TopCenter)
                .background(
                    Brush.verticalGradient(
                        listOf(Color.Black.copy(alpha = 0.78f), Color.Transparent)
                    )
                )
                .padding(horizontal = 8.dp, vertical = 6.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            IconButton(onClick = onMinimize) {
                Icon(
                    imageVector = Icons.Default.KeyboardArrowDown,
                    contentDescription = "Minimize",
                    tint = Color.White
                )
            }
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = active.title,
                    color = Color.White,
                    style = MaterialTheme.typography.titleSmall,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                val meta = listOfNotNull(
                    active.subtitle,
                    selectedQualityLabel,
                    if (active.isHls) "HLS" else null,
                    if (active.isLocalUri) "ON PHONE" else null
                ).joinToString(" · ")
                if (meta.isNotBlank()) {
                    Text(
                        text = meta,
                        color = Color.White.copy(alpha = 0.75f),
                        style = MaterialTheme.typography.labelSmall,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }

            if (active.kind == MediaKind.VIDEO) {
                IconButton(onClick = onCycleResize) {
                    Icon(Icons.Default.AspectRatio, contentDescription = "Resize: $resizeLabel", tint = Color.White)
                }
            }
            if (onTriggerPip != null) {
                IconButton(onClick = onTriggerPip) {
                    Icon(Icons.Default.PictureInPictureAlt, contentDescription = "Picture in Picture", tint = Color.White)
                }
            }
            IconButton(onClick = onClose) {
                Icon(Icons.Default.Close, contentDescription = "Close", tint = Color.White)
            }
        }

        // Center Transport Buttons
        Row(
            modifier = Modifier.align(Alignment.Center),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(26.dp)
        ) {
            IconButton(
                onClick = { onSeekRelative(-10_000L) },
                modifier = Modifier
                    .size(46.dp)
                    .clip(CircleShape)
                    .background(Color.Black.copy(alpha = 0.45f))
            ) {
                Icon(Icons.Default.Replay10, contentDescription = "Back 10s", tint = Color.White, modifier = Modifier.size(26.dp))
            }

            if (isBuffering) {
                CircularProgressIndicator(modifier = Modifier.size(48.dp), color = Color.White)
            } else {
                IconButton(
                    onClick = onTogglePlay,
                    modifier = Modifier
                        .size(62.dp)
                        .clip(CircleShape)
                        .background(Color(0xD98B5CF6))
                ) {
                    Icon(
                        imageVector = when {
                            isEnded -> Icons.Default.Replay
                            isPlaying -> Icons.Default.Pause
                            else -> Icons.Default.PlayArrow
                        },
                        contentDescription = if (isPlaying) "Pause" else "Play",
                        tint = Color.White,
                        modifier = Modifier.size(34.dp)
                    )
                }
            }

            IconButton(
                onClick = { onSeekRelative(10_000L) },
                modifier = Modifier
                    .size(46.dp)
                    .clip(CircleShape)
                    .background(Color.Black.copy(alpha = 0.45f))
            ) {
                Icon(Icons.Default.Forward10, contentDescription = "Forward 10s", tint = Color.White, modifier = Modifier.size(26.dp))
            }
        }

        // Bottom Timeline + Speed / Quality / Mute / Fullscreen Bar
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .align(Alignment.BottomCenter)
                .background(
                    Brush.verticalGradient(
                        listOf(Color.Transparent, Color.Black.copy(alpha = 0.85f))
                    )
                )
                .padding(horizontal = 12.dp, vertical = 6.dp)
        ) {
            Slider(
                value = sliderFraction,
                onValueChange = { f ->
                    isDragging = true
                    dragFraction = f
                },
                onValueChangeFinished = {
                    if (duration > 0L) {
                        onSeekTo((dragFraction * duration).toLong())
                    }
                    isDragging = false
                },
                enabled = duration > 0L,
                colors = SliderDefaults.colors(
                    thumbColor = Color(0xFF8B5CF6),
                    activeTrackColor = Color(0xFF8B5CF6),
                    inactiveTrackColor = Color.White.copy(alpha = 0.3f)
                ),
                modifier = Modifier
                    .fillMaxWidth()
                    .height(26.dp)
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    val displayMs = if (isDragging && duration > 0L) (dragFraction * duration).toLong() else currentPos
                    Text(
                        text = "${AppPlayerController.formatDurationMs(displayMs)} / ${AppPlayerController.formatDurationMs(duration)}",
                        color = Color.White,
                        style = MaterialTheme.typography.labelSmall
                    )
                    IconButton(onClick = onToggleMute, modifier = Modifier.size(34.dp)) {
                        Icon(
                            imageVector = if (isMuted) Icons.Default.VolumeOff else Icons.Default.VolumeUp,
                            contentDescription = if (isMuted) "Unmute" else "Mute",
                            tint = Color.White,
                            modifier = Modifier.size(18.dp)
                        )
                    }
                }

                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                ) {
                    Box {
                        Surface(
                            shape = RoundedCornerShape(8.dp),
                            color = Color.White.copy(alpha = 0.14f),
                            modifier = Modifier.clickable { showSpeedMenu = true }
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Icon(Icons.Default.Speed, contentDescription = null, tint = Color.White, modifier = Modifier.size(14.dp))
                                Text(
                                    text = "${speed}x",
                                    color = Color.White,
                                    fontSize = MaterialTheme.typography.labelSmall.fontSize,
                                    fontFamily = FontFamily.Monospace
                                )
                            }
                        }
                        DropdownMenu(expanded = showSpeedMenu, onDismissRequest = { showSpeedMenu = false }) {
                            listOf(0.5f, 0.75f, 1.0f, 1.25f, 1.5f, 2.0f).forEach { sp ->
                                DropdownMenuItem(
                                    text = { Text("${sp}x") },
                                    onClick = {
                                        onSpeedSelected(sp)
                                        showSpeedMenu = false
                                    }
                                )
                            }
                        }
                    }

                    if (active.variants.size > 1) {
                        Box {
                            Surface(
                                shape = RoundedCornerShape(8.dp),
                                color = Color(0xFF8B5CF6).copy(alpha = 0.45f),
                                modifier = Modifier.clickable { showQualityMenu = true }
                            ) {
                                Row(
                                    modifier = Modifier.padding(horizontal = 8.dp, vertical = 4.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(4.dp)
                                ) {
                                    Icon(Icons.Default.HighQuality, contentDescription = null, tint = Color.White, modifier = Modifier.size(14.dp))
                                    Text(
                                        text = selectedQualityLabel ?: "Auto",
                                        color = Color.White,
                                        style = MaterialTheme.typography.labelSmall
                                    )
                                }
                            }
                            DropdownMenu(expanded = showQualityMenu, onDismissRequest = { showQualityMenu = false }) {
                                active.variants.forEach { variant ->
                                    DropdownMenuItem(
                                        text = {
                                            Text(
                                                text = variant.label + if (variant.label == selectedQualityLabel) " (Active)" else ""
                                            )
                                        },
                                        onClick = {
                                            onVariantSelected(variant)
                                            showQualityMenu = false
                                        }
                                    )
                                }
                            }
                        }
                    }

                    IconButton(onClick = onToggleFullscreen, modifier = Modifier.size(36.dp)) {
                        Icon(
                            imageVector = if (isFullscreen) Icons.Default.FullscreenExit else Icons.Default.Fullscreen,
                            contentDescription = if (isFullscreen) "Exit Fullscreen" else "Fullscreen",
                            tint = Color.White
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun TabletopControlDeck(
    active: MediaPlayRequest,
    controller: AppPlayerController,
    isPlaying: Boolean,
    isEnded: Boolean,
    currentPos: Long,
    duration: Long,
    speed: Float,
    isMuted: Boolean,
    selectedQualityLabel: String?,
    modifier: Modifier = Modifier
) {
    val palette = LocalAstryxPalette.current
    var isDragging by remember { mutableStateOf(false) }
    var dragFrac by remember { mutableFloatStateOf(0f) }
    val frac = if (isDragging) dragFrac else if (duration > 0L) (currentPos.toFloat() / duration.toFloat()).coerceIn(0f, 1f) else 0f

    Surface(
        modifier = modifier,
        color = palette.surface
    ) {
        Column(
            modifier = Modifier
                .fillMaxSize()
                .padding(16.dp),
            verticalArrangement = Arrangement.SpaceBetween
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = active.title,
                        style = MaterialTheme.typography.titleMedium,
                        color = palette.textPrimary,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                    Text(
                        text = listOfNotNull(active.subtitle, selectedQualityLabel, "Tabletop Posture").joinToString(" · "),
                        style = MaterialTheme.typography.labelSmall,
                        color = palette.accentText
                    )
                }
                Row {
                    IconButton(onClick = { controller.minimizeToMiniPlayer() }) {
                        Icon(Icons.Default.KeyboardArrowDown, contentDescription = "Minimize", tint = palette.textPrimary)
                    }
                    IconButton(onClick = { controller.close() }) {
                        Icon(Icons.Default.Close, contentDescription = "Close", tint = palette.textSecondary)
                    }
                }
            }

            Column {
                Slider(
                    value = frac,
                    onValueChange = {
                        isDragging = true
                        dragFrac = it
                    },
                    onValueChangeFinished = {
                        if (duration > 0L) controller.seekTo((dragFrac * duration).toLong())
                        isDragging = false
                    },
                    enabled = duration > 0L
                )
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Text(AppPlayerController.formatDurationMs(currentPos), style = MaterialTheme.typography.labelSmall, color = palette.textSecondary)
                    Text(AppPlayerController.formatDurationMs(duration), style = MaterialTheme.typography.labelSmall, color = palette.textSecondary)
                }
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.Center,
                verticalAlignment = Alignment.CenterVertically
            ) {
                IconButton(onClick = { controller.toggleMute() }) {
                    Icon(if (isMuted) Icons.Default.VolumeOff else Icons.Default.VolumeUp, contentDescription = "Mute")
                }
                Spacer(Modifier.width(12.dp))
                OutlinedButton(onClick = { controller.seekRelative(-10_000L) }) {
                    Icon(Icons.Default.Replay10, contentDescription = null)
                    Text("10s", modifier = Modifier.padding(start = 4.dp))
                }
                Spacer(Modifier.width(14.dp))
                Button(
                    onClick = { controller.togglePlayPause() },
                    colors = ButtonDefaults.buttonColors(containerColor = palette.accent),
                    modifier = Modifier.size(64.dp),
                    shape = CircleShape,
                    contentPadding = PaddingValues(0.dp)
                ) {
                    Icon(
                        imageVector = when {
                            isEnded -> Icons.Default.Replay
                            isPlaying -> Icons.Default.Pause
                            else -> Icons.Default.PlayArrow
                        },
                        contentDescription = "Play/Pause",
                        modifier = Modifier.size(32.dp)
                    )
                }
                Spacer(Modifier.width(14.dp))
                OutlinedButton(onClick = { controller.seekRelative(10_000L) }) {
                    Icon(Icons.Default.Forward10, contentDescription = null)
                    Text("10s", modifier = Modifier.padding(start = 4.dp))
                }
                Spacer(Modifier.width(12.dp))
                IconButton(onClick = { controller.setFullscreen(true) }) {
                    Icon(Icons.Default.Fullscreen, contentDescription = "Fullscreen")
                }
            }

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                listOf(0.75f, 1.0f, 1.25f, 1.5f).forEach { sp ->
                    FilterChip(
                        selected = speed == sp,
                        onClick = { controller.setSpeed(sp) },
                        label = { Text("${sp}x") }
                    )
                }
                active.variants.forEach { v ->
                    FilterChip(
                        selected = v.label == selectedQualityLabel,
                        onClick = { controller.selectVariant(v) },
                        label = { Text(v.label) }
                    )
                }
                if (active.onSaveToPhone != null && !active.isLocalUri) {
                    OutlinedButton(onClick = active.onSaveToPhone) {
                        Icon(Icons.Default.SaveAlt, contentDescription = null, modifier = Modifier.size(16.dp))
                        Text("Save to Phone", modifier = Modifier.padding(start = 4.dp))
                    }
                }
            }
        }
    }
}

@Composable
private fun PlayerDetailsCard(
    active: MediaPlayRequest,
    selectedQualityLabel: String?,
    onSelectVariant: (com.kvdownload.app.api.StreamVariant) -> Unit,
    onSaveToPhone: (() -> Unit)?,
    onFullscreen: () -> Unit,
    onMinimize: () -> Unit,
    onClose: () -> Unit
) {
    val palette = LocalAstryxPalette.current
    AstryxCard(emphasized = true) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.Top
        ) {
            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    text = active.title,
                    style = MaterialTheme.typography.titleMedium,
                    color = palette.textPrimary,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis
                )
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    if (!active.subtitle.isNullOrBlank()) {
                        AstryxStatusBadge(active.subtitle, AstryxTone.Info)
                    }
                    if (!selectedQualityLabel.isNullOrBlank()) {
                        AstryxStatusBadge(selectedQualityLabel, AstryxTone.Accent)
                    }
                    if (active.isHls) {
                        AstryxStatusBadge("M3U8 HLS", AstryxTone.Accent)
                    }
                    if (active.isLocalUri) {
                        AstryxStatusBadge("ON PHONE", AstryxTone.Success)
                    }
                }
            }
        }

        if (active.variants.size > 1) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                Text("Quality:", style = MaterialTheme.typography.labelMedium, color = palette.textSecondary)
                active.variants.forEach { variant ->
                    FilterChip(
                        selected = variant.label == selectedQualityLabel,
                        onClick = { onSelectVariant(variant) },
                        label = { Text(variant.label) }
                    )
                }
            }
        }

        Row(
            modifier = Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Button(
                onClick = onFullscreen,
                colors = ButtonDefaults.buttonColors(containerColor = palette.accent)
            ) {
                Icon(Icons.Default.Fullscreen, contentDescription = null, modifier = Modifier.size(16.dp))
                Text("Fullscreen", modifier = Modifier.padding(start = 6.dp))
            }
            if (onSaveToPhone != null && !active.isLocalUri) {
                OutlinedButton(onClick = onSaveToPhone) {
                    Icon(Icons.Default.SaveAlt, contentDescription = null, modifier = Modifier.size(16.dp))
                    Text("Save to Phone", modifier = Modifier.padding(start = 6.dp))
                }
            }
            OutlinedButton(onClick = onMinimize) {
                Icon(Icons.Default.KeyboardArrowDown, contentDescription = null, modifier = Modifier.size(16.dp))
                Text("Mini-Player", modifier = Modifier.padding(start = 4.dp))
            }
            OutlinedButton(onClick = onClose) {
                Icon(Icons.Default.Close, contentDescription = null, modifier = Modifier.size(16.dp))
                Text("Close", modifier = Modifier.padding(start = 4.dp))
            }
        }
    }
}

@Composable
private fun PlayerUpNextPane(
    recentPlayable: List<MediaPlayRequest>,
    onSelect: (MediaPlayRequest) -> Unit,
    modifier: Modifier = Modifier
) {
    val palette = LocalAstryxPalette.current
    AstryxCard(modifier = modifier, contentPadding = PaddingValues(12.dp)) {
        Text(
            text = "Recent Library (${recentPlayable.size})",
            style = MaterialTheme.typography.titleSmall,
            color = palette.textPrimary
        )
        if (recentPlayable.isEmpty()) {
            Text(
                text = "Completed videos and audio tracks appear here for quick switching.",
                style = MaterialTheme.typography.bodySmall,
                color = palette.textSecondary
            )
        } else {
            LazyColumn(
                modifier = Modifier.fillMaxSize(),
                verticalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                items(recentPlayable.take(20), key = { it.id + it.primaryUrl }) { item ->
                    UpNextRowCard(item = item, onClick = { onSelect(item) })
                }
            }
        }
    }
}

@Composable
private fun UpNextRowCard(
    item: MediaPlayRequest,
    onClick: () -> Unit
) {
    val palette = LocalAstryxPalette.current
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
        shape = RoundedCornerShape(10.dp),
        color = palette.muted,
        border = BorderStroke(1.dp, palette.borderSubtle)
    ) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .padding(8.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            Box(
                modifier = Modifier
                    .size(64.dp, 40.dp)
                    .clip(RoundedCornerShape(6.dp))
                    .background(palette.elevated),
                contentAlignment = Alignment.Center
            ) {
                if (!item.thumbnailUrl.isNullOrBlank()) {
                    AsyncImage(
                        model = item.thumbnailUrl,
                        contentDescription = null,
                        modifier = Modifier.fillMaxSize(),
                        contentScale = ContentScale.Crop
                    )
                } else {
                    Icon(
                        imageVector = if (item.kind == MediaKind.AUDIO) Icons.Default.Audiotrack else Icons.Default.PlayArrow,
                        contentDescription = null,
                        tint = palette.accent,
                        modifier = Modifier.size(18.dp)
                    )
                }
            }
            Column(modifier = Modifier.weight(1f)) {
                Text(
                    text = item.title,
                    style = MaterialTheme.typography.bodySmall,
                    fontWeight = FontWeight.SemiBold,
                    color = palette.textPrimary,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                if (!item.subtitle.isNullOrBlank()) {
                    Text(
                        text = item.subtitle,
                        style = MaterialTheme.typography.labelSmall,
                        color = palette.textSecondary,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }
        }
    }
}
