package com.kvdownload.app.ui.screens

import android.content.Context
import android.content.Intent
import android.os.PowerManager
import android.provider.Settings
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.rememberScrollState
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.SystemUpdateAlt
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.StorageUpdateRequest
import com.kvdownload.app.storage.PhoneDownloadManager
import com.kvdownload.app.ui.adaptive.LocalWindowState
import com.kvdownload.app.ui.components.AstryxCard
import com.kvdownload.app.ui.components.AstryxGradientButton
import com.kvdownload.app.ui.components.AstryxStatusBadge
import com.kvdownload.app.ui.components.AstryxTone
import com.kvdownload.app.ui.theme.LocalAstryxPalette
import com.kvdownload.app.update.AppUpdateInfo
import com.kvdownload.app.update.AppUpdateManager
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    serverUrl: String,
    onServerUrlChanged: (String) -> Unit,
    themeMode: String,
    onThemeModeChanged: (String) -> Unit,
    phoneDownloadManager: PhoneDownloadManager,
    appUpdateManager: AppUpdateManager,
    availableUpdate: AppUpdateInfo?,
    onUpdateChecked: (AppUpdateInfo) -> Unit,
    onOpenUpdateDialog: (AppUpdateInfo) -> Unit,
    onShowMessage: (String) -> Unit
) {
    val scope = rememberCoroutineScope()
    val windowState = LocalWindowState.current
    val palette = LocalAstryxPalette.current

    val autoSave by phoneDownloadManager.autoSaveEnabled.collectAsState()
    val organizeByChannel by phoneDownloadManager.organizeByChannel.collectAsState()
    val organizeByType by phoneDownloadManager.organizeByMediaType.collectAsState()

    val (installedVersion, installedCode) = remember { appUpdateManager.getInstalledVersion() }
    var isCheckingUpdate by remember { mutableStateOf(false) }
    var updateStatusText by remember(availableUpdate) {
        mutableStateOf(
            when {
                availableUpdate == null -> "Tap \"Check for Updates\" to check GitHub & syno.vndns.net"
                availableUpdate.hasUpdate -> "New update ready: v${availableUpdate.latestVersion}"
                else -> "Up to date (v${availableUpdate.latestVersion})"
            }
        )
    }

    var serverUrlInput by remember(serverUrl) { mutableStateOf(serverUrl) }
    var connectionStatus by remember { mutableStateOf("Not tested yet") }

    var serverDownloadPath by remember { mutableStateOf("") }
    var serverOrganizeMode by remember { mutableStateOf("category_channel") }
    var ytdlpVersion by remember { mutableStateOf("...") }
    var ytdlpChannel by remember { mutableStateOf("stable") }
    var workerCount by remember { mutableStateOf(3) }

    LaunchedEffect(serverUrl) {
        try {
            val st = RetrofitClient.getApi().getStorageSettings()
            serverDownloadPath = st.effectivePath()
            if (!st.organizeMode.isNullOrBlank()) serverOrganizeMode = st.organizeMode
        } catch (_: Exception) {}

        try {
            val ver = RetrofitClient.getApi().getYtDlpVersion()
            ytdlpVersion = ver.version
            if (!ver.channel.isNullOrBlank()) ytdlpChannel = ver.channel
        } catch (_: Exception) {}

        try {
            workerCount = RetrofitClient.getApi().getWorkers().maxWorkers
        } catch (_: Exception) {}
    }

    @Composable
    fun ServerCard() {
        SettingsCard("KV Download Server") {
            OutlinedTextField(
                value = serverUrlInput,
                onValueChange = { serverUrlInput = it },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
                label = { Text("Server URL (LAN or Public Domain)") },
                placeholder = { Text(RetrofitClient.DEFAULT_SERVER_URL) }
            )
            Text(
                text = "Status: $connectionStatus",
                style = MaterialTheme.typography.labelSmall,
                color = palette.textSecondary
            )
            AstryxGradientButton(
                text = "Save & Test Connection",
                onClick = {
                    onServerUrlChanged(serverUrlInput)
                    connectionStatus = "Testing..."
                    scope.launch {
                        try {
                            val resp = RetrofitClient.getApi().health()
                            connectionStatus = if (resp.isSuccessful) {
                                "Connected (${RetrofitClient.getBaseUrl()})"
                            } else {
                                "HTTP ${resp.code()}"
                            }
                        } catch (e: Exception) {
                            connectionStatus = "Error: ${e.message}"
                        }
                    }
                },
                modifier = Modifier.fillMaxWidth()
            )
        }
    }

    @Composable
    fun PhoneStorageCard() {
        SettingsCard("Private Phone Vault (App-Only)") {
            SettingSwitchRow(
                title = "Auto-Save Completed Downloads",
                subtitle = "Automatically save completed server tasks privately inside KV Download",
                checked = autoSave,
                onCheckedChange = { phoneDownloadManager.setAutoSaveEnabled(it) }
            )
            SettingSwitchRow(
                title = "Group Folders by Channel / Album",
                subtitle = "Organizes Private Vault/<Category>/<Channel> inside app storage",
                checked = organizeByChannel,
                onCheckedChange = { phoneDownloadManager.setOrganizeByChannel(it) }
            )
            SettingSwitchRow(
                title = "Separate by Media Type",
                subtitle = "Organizes Videos, Audio, Photos & Archives inside app-private storage",
                checked = organizeByType,
                onCheckedChange = { phoneDownloadManager.setOrganizeByMediaType(it) }
            )
            Text(
                text = "App-only isolation active: downloaded phone files stay strictly inside KV Download's private vault and never appear in Gallery or other apps.",
                style = MaterialTheme.typography.labelSmall,
                color = palette.accentText
            )
            val ctx = LocalContext.current
            val pm = remember { ctx.getSystemService(Context.POWER_SERVICE) as? PowerManager }
            val isUnrestricted = remember(pm) {
                pm?.isIgnoringBatteryOptimizations(ctx.packageName) == true
            }
            Text(
                text = "Speed & Background Engine: Foreground dataSync service + up to 3 concurrent files (6 Range connections per direct file / 12 workers per HLS stream).",
                style = MaterialTheme.typography.labelSmall,
                color = palette.textSecondary
            )
            if (!isUnrestricted) {
                OutlinedButton(
                    onClick = {
                        runCatching {
                            ctx.startActivity(
                                Intent(Settings.ACTION_IGNORE_BATTERY_OPTIMIZATION_SETTINGS).apply {
                                    addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
                                }
                            )
                        }
                    },
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text("Allow Unrestricted Battery for Long Background Downloads")
                }
            } else {
                Text(
                    text = "Battery saver exempt: Unrestricted background execution active.",
                    style = MaterialTheme.typography.labelSmall,
                    color = palette.success
                )
            }
        }
    }

    @Composable
    fun ServerOrgCard() {
        SettingsCard("Server Folder Organization") {
            OutlinedTextField(
                value = serverDownloadPath,
                onValueChange = { serverDownloadPath = it },
                modifier = Modifier.fillMaxWidth(),
                singleLine = true,
                label = { Text("Server Download Directory") }
            )

            Text("Folder Structure Mode", style = MaterialTheme.typography.labelMedium, color = palette.textSecondary)
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                listOf(
                    "category_channel" to "Category/Channel",
                    "channel_category" to "Channel/Category",
                    "platform_channel" to "Platform/Channel",
                    "flat_category" to "Flat Category"
                ).forEach { (key, label) ->
                    FilterChip(
                        selected = serverOrganizeMode == key,
                        onClick = { serverOrganizeMode = key },
                        label = { Text(label) }
                    )
                }
            }

            Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedButton(
                    onClick = {
                        scope.launch {
                            try {
                                RetrofitClient.getApi().updateStorageSettings(
                                    StorageUpdateRequest(
                                        downloadDir = serverDownloadPath.trim(),
                                        path = serverDownloadPath.trim(),
                                        organizeMode = serverOrganizeMode
                                    )
                                )
                                val res = RetrofitClient.getApi().organizeLibrary()
                                onShowMessage("Organized server library (${res.movedFiles} moved)")
                            } catch (e: Exception) {
                                onShowMessage(e.message ?: "Organize failed")
                            }
                        }
                    },
                    modifier = Modifier.weight(1f)
                ) { Text("Re-Organize Now") }

                Button(
                    onClick = {
                        scope.launch {
                            try {
                                RetrofitClient.getApi().updateStorageSettings(
                                    StorageUpdateRequest(
                                        downloadDir = serverDownloadPath.trim(),
                                        path = serverDownloadPath.trim(),
                                        organizeMode = serverOrganizeMode
                                    )
                                )
                                onShowMessage("Saved server organization settings")
                            } catch (e: Exception) {
                                onShowMessage(e.message ?: "Save failed")
                            }
                        }
                    },
                    modifier = Modifier.weight(1f)
                ) { Text("Save Server") }
            }
        }
    }

    @Composable
    fun EngineCard() {
        SettingsCard("Extraction & Queue Engine") {
            Text(
                text = "yt-dlp version: $ytdlpVersion",
                style = MaterialTheme.typography.labelSmall,
                color = palette.textSecondary
            )

            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                listOf("stable", "nightly", "master").forEach { ch ->
                    FilterChip(
                        selected = ytdlpChannel == ch,
                        onClick = { ytdlpChannel = ch },
                        label = { Text(ch.replaceFirstChar { it.uppercase() }) }
                    )
                }
            }

            OutlinedButton(
                onClick = {
                    scope.launch {
                        try {
                            val res = RetrofitClient.getApi().updateYtDlpChannel(ytdlpChannel)
                            val ver = RetrofitClient.getApi().getYtDlpVersion()
                            ytdlpVersion = ver.version
                            onShowMessage(res.message)
                        } catch (e: Exception) {
                            onShowMessage(e.message ?: "Update failed")
                        }
                    }
                }
            ) { Text("Update yt-dlp Engine") }

            Text("Concurrent Server Workers: $workerCount", style = MaterialTheme.typography.labelMedium, color = palette.textSecondary)
            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                listOf(1, 2, 3, 4, 6, 8).forEach { count ->
                    FilterChip(
                        selected = workerCount == count,
                        onClick = {
                            workerCount = count
                            scope.launch {
                                try {
                                    RetrofitClient.getApi().setWorkers(count)
                                    onShowMessage("Workers set to $count")
                                } catch (_: Exception) {}
                            }
                        },
                        label = { Text("$count") }
                    )
                }
            }
        }
    }

    @Composable
    fun AppearanceCard() {
        SettingsCard("Appearance (Astryx Studio)") {
            SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                listOf("system" to "System", "light" to "Light", "dark" to "Dark").forEachIndexed { idx, (mode, label) ->
                    SegmentedButton(
                        selected = themeMode == mode,
                        onClick = { onThemeModeChanged(mode) },
                        shape = SegmentedButtonDefaults.itemShape(index = idx, count = 3)
                    ) { Text(label) }
                }
            }
        }
    }

    @Composable
    fun AppUpdateCard() {
        SettingsCard("App Version & Updates") {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Column(modifier = Modifier.weight(1f)) {
                    Text(
                        text = "Installed: v$installedVersion (Build $installedCode)",
                        style = MaterialTheme.typography.titleSmall,
                        color = palette.textPrimary
                    )
                    Text(
                        text = updateStatusText,
                        style = MaterialTheme.typography.labelSmall,
                        color = if (availableUpdate?.hasUpdate == true) palette.warning else palette.textSecondary
                    )
                }
                AstryxStatusBadge(
                    text = if (availableUpdate?.hasUpdate == true) {
                        "UPDATE v${availableUpdate.latestVersion}"
                    } else {
                        "UP TO DATE"
                    },
                    tone = if (availableUpdate?.hasUpdate == true) AstryxTone.Warning else AstryxTone.Success
                )
            }

            if (availableUpdate?.hasUpdate == true && availableUpdate.releaseNotes.isNotBlank()) {
                Text(
                    text = availableUpdate.releaseNotes,
                    style = MaterialTheme.typography.bodySmall,
                    color = palette.textSecondary,
                    maxLines = 3
                )
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(8.dp)
            ) {
                AstryxGradientButton(
                    text = if (isCheckingUpdate) "Checking..." else "Check for Updates",
                    icon = Icons.Default.SystemUpdateAlt,
                    loading = isCheckingUpdate,
                    enabled = !isCheckingUpdate,
                    onClick = {
                        isCheckingUpdate = true
                        updateStatusText = "Checking server & GitHub releases..."
                        scope.launch {
                            val result = appUpdateManager.checkForUpdate()
                            isCheckingUpdate = false
                            result.onSuccess { info ->
                                onUpdateChecked(info)
                                if (info.hasUpdate) {
                                    updateStatusText = "Update available: v${info.latestVersion}"
                                    onOpenUpdateDialog(info)
                                } else {
                                    updateStatusText = "Up to date (v${info.currentVersion})"
                                    onShowMessage("You're on the latest version (v${info.currentVersion})")
                                }
                            }.onFailure { err ->
                                val msg = err.message ?: "Unable to check updates"
                                updateStatusText = "Check failed: $msg"
                                onShowMessage("Update check failed: $msg")
                            }
                        }
                    },
                    modifier = Modifier.weight(1f)
                )

                OutlinedButton(
                    onClick = {
                        val existing = availableUpdate
                        if (existing != null) {
                            onOpenUpdateDialog(existing)
                        } else {
                            scope.launch {
                                isCheckingUpdate = true
                                val result = appUpdateManager.checkForUpdate()
                                isCheckingUpdate = false
                                result.onSuccess { info ->
                                    onOpenUpdateDialog(info)
                                }.onFailure {
                                    onOpenUpdateDialog(
                                        AppUpdateInfo(
                                            currentVersion = installedVersion,
                                            currentVersionCode = installedCode,
                                            latestVersion = installedVersion,
                                            downloadUrl = AppUpdateManager.DEFAULT_SYNO_LATEST_APK,
                                            fallbackUrls = emptyList(),
                                            releaseNotes = "Direct download from syno.vndns.net",
                                            releasePageUrl = AppUpdateManager.DEFAULT_SYNO_RELEASE_PAGE,
                                            hasUpdate = false
                                        )
                                    )
                                }
                            }
                        }
                    },
                    enabled = !isCheckingUpdate
                ) {
                    Icon(Icons.Default.Download, contentDescription = null)
                    Text("Install APK", modifier = Modifier.padding(start = 6.dp))
                }
            }
        }
    }

    if (windowState.isTwoPane) {
        LazyVerticalGrid(
            columns = GridCells.Fixed(2),
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(16.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            item { AppUpdateCard() }
            item { ServerCard() }
            item { PhoneStorageCard() }
            item { ServerOrgCard() }
            item { EngineCard() }
            item { AppearanceCard() }
        }
    } else {
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(14.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            item { AppUpdateCard() }
            item { ServerCard() }
            item { PhoneStorageCard() }
            item { ServerOrgCard() }
            item { EngineCard() }
            item { AppearanceCard() }
        }
    }
}

@Composable
private fun SettingsCard(
    title: String,
    content: @Composable () -> Unit
) {
    val palette = LocalAstryxPalette.current
    AstryxCard {
        Text(title, style = MaterialTheme.typography.titleMedium, color = palette.textPrimary)
        content()
    }
}

@Composable
private fun SettingSwitchRow(
    title: String,
    subtitle: String,
    checked: Boolean,
    onCheckedChange: (Boolean) -> Unit
) {
    val palette = LocalAstryxPalette.current
    Row(
        modifier = Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically
    ) {
        Column(modifier = Modifier.weight(1f).padding(end = 10.dp)) {
            Text(title, style = MaterialTheme.typography.bodyMedium, color = palette.textPrimary)
            Text(subtitle, style = MaterialTheme.typography.bodySmall, color = palette.textSecondary)
        }
        Switch(checked = checked, onCheckedChange = onCheckedChange)
    }
}
