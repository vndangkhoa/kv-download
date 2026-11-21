package com.kvdownload.app

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.TravelExplore
import androidx.compose.material.icons.filled.VideoLibrary
import androidx.compose.material3.Badge
import androidx.compose.material3.BadgedBox
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.NavigationBar
import androidx.compose.material3.NavigationBarItem
import androidx.compose.material3.Scaffold
import androidx.compose.material3.SnackbarHost
import androidx.compose.material3.SnackbarHostState
import androidx.compose.material3.Text
import androidx.compose.material3.TopAppBar
import androidx.compose.material3.TopAppBarDefaults
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.core.content.ContextCompat
import com.kvdownload.app.api.DownloadTask
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.storage.PhoneDownloadManager
import com.kvdownload.app.ui.components.InAppPlayerDialog
import com.kvdownload.app.ui.components.PlaybackRequest
import com.kvdownload.app.ui.screens.DownloadCenterScreen
import com.kvdownload.app.ui.screens.InAppBrowserScreen
import com.kvdownload.app.ui.screens.MediaGalleryScreen
import com.kvdownload.app.ui.screens.SettingsScreen
import com.kvdownload.app.ui.theme.KVDownloadTheme
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

class MainActivity : ComponentActivity() {
    private val sharedUrlState = mutableStateOf<String?>(null)

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
                    phoneDownloadManager = phoneDownloadManager
                )
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        handleIncomingIntent(intent)
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

@OptIn(ExperimentalMaterial3Api::class)
@Composable
private fun KVDownloadApp(
    serverUrl: String,
    onServerUrlChanged: (String) -> Unit,
    themeMode: String,
    onThemeModeChanged: (String) -> Unit,
    initialSharedUrl: String?,
    onSharedUrlConsumed: () -> Unit,
    phoneDownloadManager: PhoneDownloadManager
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val snackbarHostState = remember { SnackbarHostState() }

    var activeTab by remember { mutableStateOf("queue") }
    var tasks by remember { mutableStateOf<List<DownloadTask>>(emptyList()) }
    var activePlayback by remember { mutableStateOf<PlaybackRequest?>(null) }
    val transfers by phoneDownloadManager.transfers.collectAsState()

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
                phoneDownloadManager.autoSaveIfNeeded(res.tasks)
            } catch (_: Exception) {}
        }
    }

    LaunchedEffect(serverUrl) {
        while (isActive) {
            try {
                val res = RetrofitClient.getApi().getQueue()
                tasks = res.tasks
                phoneDownloadManager.autoSaveIfNeeded(res.tasks)
            } catch (_: Exception) {}
            delay(3000)
        }
    }

    val activePhoneTransfersCount = transfers.values.count { !it.isCompleted && it.error == null }

    Scaffold(
        snackbarHost = { SnackbarHost(snackbarHostState) },
        topBar = {
            TopAppBar(
                title = {
                    Column {
                        Text(
                            text = when (activeTab) {
                                "queue" -> "KV Download"
                                "browser" -> "In-App Browser"
                                "gallery" -> "Media Gallery"
                                else -> "Settings"
                            },
                            style = MaterialTheme.typography.titleLarge,
                            fontWeight = FontWeight.Bold
                        )
                        if (activePhoneTransfersCount > 0) {
                            Text(
                                text = "Saving $activePhoneTransfersCount file(s) to phone...",
                                style = MaterialTheme.typography.labelSmall,
                                color = MaterialTheme.colorScheme.primary
                            )
                        }
                    }
                },
                colors = TopAppBarDefaults.topAppBarColors(
                    containerColor = MaterialTheme.colorScheme.surface
                )
            )
        },
        bottomBar = {
            NavigationBar(containerColor = MaterialTheme.colorScheme.surface) {
                NavigationBarItem(
                    selected = activeTab == "queue",
                    onClick = { activeTab = "queue" },
                    icon = {
                        BadgedBox(
                            badge = {
                                if (tasks.isNotEmpty()) {
                                    Badge { Text("${tasks.size}") }
                                }
                            }
                        ) {
                            Icon(Icons.Default.Download, contentDescription = "Queue")
                        }
                    },
                    label = { Text("Queue") }
                )
                NavigationBarItem(
                    selected = activeTab == "browser",
                    onClick = { activeTab = "browser" },
                    icon = { Icon(Icons.Default.TravelExplore, contentDescription = "Browser") },
                    label = { Text("Browser") }
                )
                NavigationBarItem(
                    selected = activeTab == "gallery",
                    onClick = { activeTab = "gallery" },
                    icon = { Icon(Icons.Default.VideoLibrary, contentDescription = "Gallery") },
                    label = { Text("Gallery") }
                )
                NavigationBarItem(
                    selected = activeTab == "settings",
                    onClick = { activeTab = "settings" },
                    icon = { Icon(Icons.Default.Settings, contentDescription = "Settings") },
                    label = { Text("Settings") }
                )
            }
        }
    ) { paddingValues ->
        Box(
            modifier = Modifier
                .fillMaxSize()
                .padding(paddingValues)
        ) {
            when (activeTab) {
                "queue" -> DownloadCenterScreen(
                    tasks = tasks,
                    initialSharedUrl = initialSharedUrl,
                    onSharedUrlConsumed = onSharedUrlConsumed,
                    phoneDownloadManager = phoneDownloadManager,
                    onRefreshQueue = ::refreshQueueNow,
                    onPlayMedia = { activePlayback = it },
                    onShowMessage = { msg -> scope.launch { snackbarHostState.showSnackbar(msg) } }
                )
                "browser" -> InAppBrowserScreen(
                    phoneDownloadManager = phoneDownloadManager,
                    onRefreshQueue = ::refreshQueueNow,
                    onPlayMedia = { activePlayback = it },
                    onShowMessage = { msg -> scope.launch { snackbarHostState.showSnackbar(msg) } }
                )
                "gallery" -> MediaGalleryScreen(
                    phoneDownloadManager = phoneDownloadManager,
                    onPlayMedia = { activePlayback = it },
                    onShowMessage = { msg -> scope.launch { snackbarHostState.showSnackbar(msg) } }
                )
                "settings" -> SettingsScreen(
                    serverUrl = serverUrl,
                    onServerUrlChanged = onServerUrlChanged,
                    themeMode = themeMode,
                    onThemeModeChanged = onThemeModeChanged,
                    phoneDownloadManager = phoneDownloadManager,
                    onShowMessage = { msg -> scope.launch { snackbarHostState.showSnackbar(msg) } }
                )
            }
        }
    }

    activePlayback?.let { req ->
        InAppPlayerDialog(
            request = req,
            onDismiss = { activePlayback = null }
        )
    }
}
