package com.kvdownload.app

import android.Manifest
import android.app.PictureInPictureParams
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.os.Build
import android.os.Bundle
import android.util.Rational
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.navigationBarsPadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.SystemUpdateAlt
import androidx.compose.material.icons.filled.TravelExplore
import androidx.compose.material.icons.filled.VideoLibrary
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.NavigationBarItemDefaults
import androidx.compose.material3.NavigationRail
import androidx.compose.material3.NavigationRailItem
import androidx.compose.material3.NavigationRailItemDefaults
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.VerticalDivider
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.core.content.ContextCompat
import com.kvdownload.app.api.DownloadTask
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.player.AppPlayerController
import com.kvdownload.app.player.MediaKind
import com.kvdownload.app.player.MediaPlayRequest
import com.kvdownload.app.storage.PhoneDownloadManager
import com.kvdownload.app.ui.adaptive.LocalWindowState
import com.kvdownload.app.ui.adaptive.ProvideWindowState
import com.kvdownload.app.ui.components.AppUpdateDialog
import com.kvdownload.app.ui.components.AstryxBrandMark
import com.kvdownload.app.ui.components.AstryxStatusBadge
import com.kvdownload.app.ui.components.AstryxTone
import com.kvdownload.app.update.AppUpdateInfo
import com.kvdownload.app.update.AppUpdateManager
import com.kvdownload.app.ui.player.IntegratedPlayerOverlay
import com.kvdownload.app.ui.player.PersistentMiniPlayer
import com.kvdownload.app.ui.player.PhotoLightboxOverlay
import com.kvdownload.app.ui.screens.DownloadCenterScreen
import com.kvdownload.app.ui.screens.InAppBrowserScreen
import com.kvdownload.app.ui.screens.MediaGalleryScreen
import com.kvdownload.app.ui.screens.SettingsScreen
import com.kvdownload.app.ui.theme.KVDownloadTheme
import com.kvdownload.app.ui.theme.LocalAstryxPalette
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    private val sharedUrlState = mutableStateOf<String?>(null)
    private val isInPipModeState = mutableStateOf(false)
    private var attachedController: AppPlayerController? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        handleIncomingIntent(intent)

        setContent {
            val context = LocalContext.current
            val prefs = remember { context.getSharedPreferences(APP_PREFS, Context.MODE_PRIVATE) }

            var serverUrl by remember {
                val initial = prefs.getString(KEY_SERVER_URL, RetrofitClient.DEFAULT_SERVER_URL)
                    ?: RetrofitClient.DEFAULT_SERVER_URL
                RetrofitClient.configure(initial)
                mutableStateOf(initial)
            }
            var themeMode by remember {
                mutableStateOf(prefs.getString(KEY_THEME_MODE, "system") ?: "system")
            }

            val phoneDownloadManager = remember { PhoneDownloadManager(context.applicationContext) }
            val incomingUrl by sharedUrlState
            val isInPipMode by isInPipModeState

            ProvideWindowState(activity = this@MainActivity) {
                KVDownloadTheme(themeMode = themeMode) {
                    KVDownloadApp(
                        serverUrl = serverUrl,
                        onServerUrlChanged = { next ->
                            val normalized = RetrofitClient.normalizeUrl(next)
                            serverUrl = normalized
                            prefs.edit().putString(KEY_SERVER_URL, normalized).apply()
                            RetrofitClient.configure(normalized)
                        },
                        themeMode = themeMode,
                        onThemeModeChanged = { next ->
                            themeMode = next
                            prefs.edit().putString(KEY_THEME_MODE, next).apply()
                        },
                        initialSharedUrl = incomingUrl,
                        onSharedUrlConsumed = { sharedUrlState.value = null },
                        phoneDownloadManager = phoneDownloadManager,
                        isInPipMode = isInPipMode,
                        onAttachController = { attachedController = it }
                    )
                }
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIncomingIntent(intent)
    }

    override fun onUserLeaveHint() {
        super.onUserLeaveHint()
        val ctrl = attachedController
        if (ctrl != null &&
            Build.VERSION.SDK_INT >= Build.VERSION_CODES.O &&
            ctrl.isExpanded.value &&
            ctrl.isPlaying.value &&
            ctrl.currentRequest.value?.kind == MediaKind.VIDEO
        ) {
            val w = ctrl.videoWidth.value.coerceAtLeast(16)
            val h = ctrl.videoHeight.value.coerceAtLeast(9)
            try {
                enterPictureInPictureMode(
                    PictureInPictureParams.Builder()
                        .setAspectRatio(Rational(w, h))
                        .build()
                )
            } catch (_: Exception) {}
        }
    }

    override fun onPictureInPictureModeChanged(
        isInPictureInPictureMode: Boolean,
        newConfig: Configuration
    ) {
        super.onPictureInPictureModeChanged(isInPictureInPictureMode, newConfig)
        isInPipModeState.value = isInPictureInPictureMode
    }

    private fun handleIncomingIntent(intent: Intent?) {
        if (intent?.action == Intent.ACTION_SEND && intent.type == "text/plain") {
            val sharedText = intent.getStringExtra(Intent.EXTRA_TEXT)?.trim() ?: return
            val urlMatch = Regex("""https?://\S+""").find(sharedText)?.value ?: sharedText
            sharedUrlState.value = urlMatch
        }
    }

    companion object {
        private const val APP_PREFS = "kv_download_app_prefs"
        private const val KEY_SERVER_URL = "server_url"
        private const val KEY_THEME_MODE = "theme_mode"
    }
}

private data class NavDestination(
    val key: String,
    val label: String,
    val subtitle: String,
    val icon: ImageVector
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun KVDownloadApp(
    serverUrl: String,
    onServerUrlChanged: (String) -> Unit,
    themeMode: String,
    onThemeModeChanged: (String) -> Unit,
    initialSharedUrl: String?,
    onSharedUrlConsumed: () -> Unit,
    phoneDownloadManager: PhoneDownloadManager,
    isInPipMode: Boolean,
    onAttachController: (AppPlayerController) -> Unit
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val snackbarHostState = remember { SnackbarHostState() }
    val windowState = LocalWindowState.current
    val palette = LocalAstryxPalette.current

    val playerController = remember { AppPlayerController(context.applicationContext) }
    DisposableEffect(playerController) {
        onAttachController(playerController)
        onDispose {
            playerController.release()
        }
    }

    var activeTab by remember { mutableStateOf("queue") }
    var tasks by remember { mutableStateOf<List<DownloadTask>>(emptyList()) }
    var serverOnline by remember { mutableStateOf<Boolean?>(null) }

    val appUpdateManager = remember { AppUpdateManager(context.applicationContext) }
    var availableUpdate by remember { mutableStateOf<AppUpdateInfo?>(null) }
    var dialogUpdateInfo by remember { mutableStateOf<AppUpdateInfo?>(null) }

    val transfers by phoneDownloadManager.transfers.collectAsState()
    val savedFiles by phoneDownloadManager.savedFiles.collectAsState()

    val permissionLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.RequestMultiplePermissions()
    ) { }

    LaunchedEffect(Unit) {
        val needed = buildList {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                add(Manifest.permission.READ_MEDIA_VIDEO)
                add(Manifest.permission.READ_MEDIA_AUDIO)
                add(Manifest.permission.READ_MEDIA_IMAGES)
                add(Manifest.permission.POST_NOTIFICATIONS)
            } else if (Build.VERSION.SDK_INT <= Build.VERSION_CODES.P) {
                add(Manifest.permission.WRITE_EXTERNAL_STORAGE)
                add(Manifest.permission.READ_EXTERNAL_STORAGE)
            }
        }.filter { perm ->
            ContextCompat.checkSelfPermission(context, perm) != PackageManager.PERMISSION_GRANTED
        }
        if (needed.isNotEmpty()) {
            permissionLauncher.launch(needed.toTypedArray())
        }
    }

    LaunchedEffect(initialSharedUrl) {
        if (!initialSharedUrl.isNullOrBlank()) {
            activeTab = "queue"
        }
    }

    fun refreshQueueNow() {
        scope.launch {
            try {
                val res = RetrofitClient.getApi().getQueue()
                tasks = res.tasks
                serverOnline = true
                phoneDownloadManager.autoSaveIfNeeded(res.tasks)
            } catch (_: Exception) {
                serverOnline = false
            }
        }
    }

    LaunchedEffect(serverUrl) {
        launch {
            appUpdateManager.checkForUpdate().onSuccess { info ->
                availableUpdate = info
            }
        }
        while (isActive) {
            try {
                val res = RetrofitClient.getApi().getQueue()
                tasks = res.tasks
                serverOnline = true
                phoneDownloadManager.autoSaveIfNeeded(res.tasks)
            } catch (_: Exception) {
                serverOnline = false
            }
            delay(3000)
        }
    }

    val activePhoneTransfers = transfers.values.filter { !it.isCompleted && it.error == null }
    val activePhoneTransfersCount = activePhoneTransfers.size
    val activeQueueCount = tasks.count { it.status == "downloading" || it.status == "queued" }

    val recentPlayable = remember(tasks, savedFiles) {
        tasks.filter { it.status == "completed" }
            .sortedByDescending { it.createdAt }
            .take(15)
            .map { t ->
                val saved = savedFiles.firstOrNull { it.serverTaskId == t.id }
                val lowerName = (t.mediaName ?: "").lowercase()
                val kind = when {
                    t.format?.contains("audio", ignoreCase = true) == true ||
                        lowerName.endsWith(".mp3") || lowerName.endsWith(".m4a") ||
                        lowerName.endsWith(".flac") || lowerName.endsWith(".wav") -> MediaKind.AUDIO
                    lowerName.endsWith(".jpg") || lowerName.endsWith(".jpeg") ||
                        lowerName.endsWith(".png") || lowerName.endsWith(".webp") -> MediaKind.IMAGE
                    else -> MediaKind.VIDEO
                }
                val serverStream = RetrofitClient.downloadTaskUrl(t.id, inline = true, mediaId = t.mediaId)
                MediaPlayRequest(
                    id = "task:${t.id}",
                    title = t.displayTitle(),
                    subtitle = t.displayChannel(),
                    thumbnailUrl = RetrofitClient.taskThumbnailUrl(t),
                    primaryUrl = saved?.contentUri ?: serverStream,
                    fallbackUrl = "$serverStream&compat=1",
                    kind = kind,
                    isLocalUri = saved != null,
                    onSaveToPhone = {
                        phoneDownloadManager.saveTaskToPhone(t)
                        scope.launch { snackbarHostState.showSnackbar("Saving \"${t.displayTitle()}\" to phone...") }
                    }
                )
            }
    }

    val destinations = remember {
        listOf(
            NavDestination("queue", "Queue", "Transfer Engine & Groups", Icons.Default.Download),
            NavDestination("browser", "Browser", "Stream Sniffer & M3U8", Icons.Default.TravelExplore),
            NavDestination("gallery", "Gallery", "Server & Phone Media", Icons.Default.VideoLibrary),
            NavDestination("settings", "Settings", "Storage & Astryx", Icons.Default.Settings)
        )
    }
    val currentDest = destinations.first { it.key == activeTab }

    @Composable
    fun ScreenContent(modifier: Modifier = Modifier) {
        Box(modifier = modifier.background(palette.body)) {
            when (activeTab) {
                "queue" -> DownloadCenterScreen(
                    tasks = tasks,
                    initialSharedUrl = initialSharedUrl,
                    onSharedUrlConsumed = onSharedUrlConsumed,
                    phoneDownloadManager = phoneDownloadManager,
                    onRefreshQueue = ::refreshQueueNow,
                    onPlayMedia = { playerController.open(it) },
                    onShowMessage = { msg -> scope.launch { snackbarHostState.showSnackbar(msg) } }
                )
                "browser" -> InAppBrowserScreen(
                    phoneDownloadManager = phoneDownloadManager,
                    onRefreshQueue = ::refreshQueueNow,
                    onPlayMedia = { playerController.open(it) },
                    onShowMessage = { msg -> scope.launch { snackbarHostState.showSnackbar(msg) } }
                )
                "gallery" -> MediaGalleryScreen(
                    phoneDownloadManager = phoneDownloadManager,
                    onPlayMedia = { playerController.open(it) },
                    onShowMessage = { msg -> scope.launch { snackbarHostState.showSnackbar(msg) } }
                )
                "settings" -> SettingsScreen(
                    serverUrl = serverUrl,
                    onServerUrlChanged = onServerUrlChanged,
                    themeMode = themeMode,
                    onThemeModeChanged = onThemeModeChanged,
                    phoneDownloadManager = phoneDownloadManager,
                    appUpdateManager = appUpdateManager,
                    availableUpdate = availableUpdate,
                    onUpdateChecked = { info -> availableUpdate = info },
                    onOpenUpdateDialog = { info ->
                        availableUpdate = info
                        dialogUpdateInfo = info
                    },
                    onShowMessage = { msg -> scope.launch { snackbarHostState.showSnackbar(msg) } }
                )
            }
        }
    }

    Box(
        modifier = Modifier
            .fillMaxSize()
            .background(palette.body)
    ) {
        if (windowState.useNavigationRail) {
            // Tablet / Unfolded Foldable / Wide Landscape Rail Shell
            Row(modifier = Modifier.fillMaxSize()) {
                NavigationRail(
                    containerColor = palette.surface,
                    modifier = Modifier
                        .fillMaxHeight()
                        .statusBarsPadding()
                        .navigationBarsPadding(),
                    header = {
                        Column(
                            horizontalAlignment = Alignment.CenterHorizontally,
                            verticalArrangement = Arrangement.spacedBy(6.dp),
                            modifier = Modifier.padding(top = 8.dp, bottom = 12.dp)
                        ) {
                            AstryxBrandMark(size = 38.dp)
                            Text(
                                text = "KV",
                                style = MaterialTheme.typography.labelSmall,
                                color = palette.accentText
                            )
                        }
                    }
                ) {
                    Spacer(Modifier.height(4.dp))
                    destinations.forEach { dest ->
                        val badgeCount = when (dest.key) {
                            "queue" -> activeQueueCount.takeIf { it > 0 } ?: tasks.size
                            else -> 0
                        }
                        NavigationRailItem(
                            selected = activeTab == dest.key,
                            onClick = { activeTab = dest.key },
                            colors = NavigationRailItemDefaults.colors(
                                selectedIconColor = Color.White,
                                selectedTextColor = palette.accentText,
                                indicatorColor = palette.accent,
                                unselectedIconColor = palette.textSecondary,
                                unselectedTextColor = palette.textSecondary
                            ),
                            icon = {
                                BadgedBox(
                                    badge = {
                                        if (badgeCount > 0 && dest.key == "queue") {
                                            Badge(containerColor = palette.accent) {
                                                Text("$badgeCount", color = Color.White)
                                            }
                                        } else if (dest.key == "settings" && availableUpdate?.hasUpdate == true) {
                                            Badge(containerColor = palette.warning)
                                        }
                                    }
                                ) {
                                    Icon(dest.icon, contentDescription = dest.label)
                                }
                            },
                            label = { Text(dest.label) }
                        )
                    }
                    Spacer(Modifier.weight(1f))
                    // Live engine dot
                    Box(
                        modifier = Modifier
                            .padding(bottom = 14.dp)
                            .size(10.dp)
                            .clip(CircleShape)
                            .background(
                                when (serverOnline) {
                                    true -> palette.success
                                    false -> palette.error
                                    null -> palette.warning
                                }
                            )
                    )
                }

                VerticalDivider(color = palette.borderSubtle)

                Scaffold(
                    snackbarHost = { SnackbarHost(snackbarHostState) },
                    containerColor = palette.body,
                    topBar = {
                        AstryxAppHeader(
                            title = if (activeTab == "queue") "KV Download Studio" else currentDest.label,
                            subtitle = if (activePhoneTransfersCount > 0) {
                                activePhoneTransfers.firstOrNull()?.statusText ?: "Saving $activePhoneTransfersCount to phone..."
                            } else {
                                currentDest.subtitle
                            },
                            serverOnline = serverOnline,
                            activeTransfers = activePhoneTransfersCount,
                            showBrand = false,
                            updateBadgeVersion = availableUpdate?.takeIf { it.hasUpdate }?.latestVersion,
                            onUpdateBadgeClick = {
                                availableUpdate?.let { dialogUpdateInfo = it }
                            }
                        )
                    },
                    bottomBar = {
                        PersistentMiniPlayer(
                            controller = playerController,
                            modifier = Modifier.navigationBarsPadding()
                        )
                    }
                ) { paddingValues ->
                    ScreenContent(
                        modifier = Modifier
                            .fillMaxSize()
                            .padding(paddingValues)
                    )
                }
            }
        } else {
            // Compact Phone Shell
            Scaffold(
                snackbarHost = { SnackbarHost(snackbarHostState) },
                containerColor = palette.body,
                topBar = {
                    AstryxAppHeader(
                        title = if (activeTab == "queue") "KV Download" else currentDest.label,
                        subtitle = if (activePhoneTransfersCount > 0) {
                            activePhoneTransfers.firstOrNull()?.statusText ?: "Saving $activePhoneTransfersCount file(s)..."
                        } else {
                            currentDest.subtitle
                        },
                        serverOnline = serverOnline,
                        activeTransfers = activePhoneTransfersCount,
                        showBrand = true,
                        updateBadgeVersion = availableUpdate?.takeIf { it.hasUpdate }?.latestVersion,
                        onUpdateBadgeClick = {
                            availableUpdate?.let { dialogUpdateInfo = it }
                        }
                    )
                },
                bottomBar = {
                    Column(modifier = Modifier.background(palette.surface)) {
                        PersistentMiniPlayer(controller = playerController)
                        HorizontalDivider(color = palette.borderSubtle)
                        NavigationBar(
                            containerColor = palette.surface,
                            tonalElevation = 0.dp
                        ) {
                            destinations.forEach { dest ->
                                val badgeCount = if (dest.key == "queue") tasks.size else 0
                                NavigationBarItem(
                                    selected = activeTab == dest.key,
                                    onClick = { activeTab = dest.key },
                                    colors = NavigationBarItemDefaults.colors(
                                        selectedIconColor = palette.accentText,
                                        selectedTextColor = palette.accentText,
                                        indicatorColor = palette.accentContainer,
                                        unselectedIconColor = palette.textSecondary,
                                        unselectedTextColor = palette.textSecondary
                                    ),
                                    icon = {
                                        BadgedBox(
                                            badge = {
                                                if (badgeCount > 0) {
                                                    Badge(containerColor = palette.accent) {
                                                        Text("$badgeCount", color = Color.White)
                                                    }
                                                } else if (dest.key == "settings" && availableUpdate?.hasUpdate == true) {
                                                    Badge(containerColor = palette.warning)
                                                }
                                            }
                                        ) {
                                            Icon(dest.icon, contentDescription = dest.label)
                                        }
                                    },
                                    label = { Text(dest.label, maxLines = 1, overflow = TextOverflow.Ellipsis) }
                                )
                            }
                        }
                    }
                }
            ) { paddingValues ->
                ScreenContent(
                    modifier = Modifier
                        .fillMaxSize()
                        .padding(paddingValues)
                )
            }
        }

        IntegratedPlayerOverlay(
            controller = playerController,
            recentPlayable = recentPlayable,
            isInPipMode = isInPipMode
        )

        PhotoLightboxOverlay(controller = playerController)

        dialogUpdateInfo?.let { info ->
            AppUpdateDialog(
                updateInfo = info,
                updateManager = appUpdateManager,
                onDismiss = { dialogUpdateInfo = null },
                onShowMessage = { msg -> scope.launch { snackbarHostState.showSnackbar(msg) } }
            )
        }
    }
}

@Composable
private fun AstryxAppHeader(
    title: String,
    subtitle: String,
    serverOnline: Boolean?,
    activeTransfers: Int,
    showBrand: Boolean,
    updateBadgeVersion: String? = null,
    onUpdateBadgeClick: (() -> Unit)? = null
) {
    val palette = LocalAstryxPalette.current
    Surface(
        color = palette.surface,
        border = BorderStroke(0.dp, Color.Transparent)
    ) {
        Column(modifier = Modifier.fillMaxWidth()) {
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .statusBarsPadding()
                    .padding(horizontal = 14.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp),
                    modifier = Modifier.weight(1f)
                ) {
                    if (showBrand) {
                        AstryxBrandMark(size = 34.dp)
                    }
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = title,
                            style = MaterialTheme.typography.titleLarge,
                            color = palette.textPrimary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                        Text(
                            text = subtitle,
                            style = MaterialTheme.typography.labelSmall,
                            color = if (activeTransfers > 0) palette.success else palette.textSecondary,
                            maxLines = 1,
                            overflow = TextOverflow.Ellipsis
                        )
                    }
                }

                Spacer(Modifier.width(8.dp))

                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    if (!updateBadgeVersion.isNullOrBlank() && onUpdateBadgeClick != null) {
                        Surface(
                            shape = RoundedCornerShape(999.dp),
                            color = palette.warning.copy(alpha = 0.16f),
                            border = BorderStroke(1.dp, palette.warning.copy(alpha = 0.45f)),
                            modifier = Modifier.clickable(onClick = onUpdateBadgeClick)
                        ) {
                            Row(
                                modifier = Modifier.padding(horizontal = 9.dp, vertical = 5.dp),
                                verticalAlignment = Alignment.CenterVertically,
                                horizontalArrangement = Arrangement.spacedBy(4.dp)
                            ) {
                                Icon(
                                    imageVector = Icons.Default.SystemUpdateAlt,
                                    contentDescription = "Update available",
                                    tint = palette.warning,
                                    modifier = Modifier.size(13.dp)
                                )
                                Text(
                                    text = "v$updateBadgeVersion",
                                    style = MaterialTheme.typography.labelSmall,
                                    color = palette.warning,
                                    maxLines = 1
                                )
                            }
                        }
                    }

                    Surface(
                        shape = RoundedCornerShape(999.dp),
                        color = palette.muted,
                        border = BorderStroke(1.dp, palette.borderSubtle)
                    ) {
                        Row(
                            modifier = Modifier.padding(horizontal = 10.dp, vertical = 5.dp),
                            verticalAlignment = Alignment.CenterVertically,
                            horizontalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            Box(
                                modifier = Modifier
                                    .size(7.dp)
                                    .clip(CircleShape)
                                    .background(
                                        when (serverOnline) {
                                            true -> palette.success
                                            false -> palette.error
                                            null -> palette.warning
                                        }
                                    )
                            )
                            Text(
                                text = when {
                                    activeTransfers > 0 -> "PHONE $activeTransfers"
                                    serverOnline == true -> "ONLINE"
                                    serverOnline == false -> "OFFLINE"
                                    else -> "SYNC"
                                },
                                style = MaterialTheme.typography.labelSmall,
                                color = palette.textPrimary
                            )
                        }
                    }
                }
            }
            HorizontalDivider(color = palette.borderSubtle)
        }
    }
}
