package com.kvdownload.app.ui.screens

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
import androidx.compose.material3.Button
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
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
import androidx.compose.ui.unit.dp
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.StorageUpdateRequest
import com.kvdownload.app.storage.PhoneDownloadManager
import com.kvdownload.app.ui.adaptive.LocalWindowState
import com.kvdownload.app.ui.components.AstryxCard
import com.kvdownload.app.ui.components.AstryxGradientButton
import com.kvdownload.app.ui.theme.LocalAstryxPalette
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SettingsScreen(
    serverUrl: String,
    onServerUrlChanged: (String) -> Unit,
    themeMode: String,
    onThemeModeChanged: (String) -> Unit,
    phoneDownloadManager: PhoneDownloadManager,
    onShowMessage: (String) -> Unit
) {
    val scope = rememberCoroutineScope()
    val windowState = LocalWindowState.current
    val palette = LocalAstryxPalette.current

    val autoSave by phoneDownloadManager.autoSaveEnabled.collectAsState()
    val organizeByChannel by phoneDownloadManager.organizeByChannel.collectAsState()
    val organizeByType by phoneDownloadManager.organizeByMediaType.collectAsState()

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
        SettingsCard("Phone Storage & Organization") {
            SettingSwitchRow(
                title = "Auto-Save Completed Downloads",
                subtitle = "Automatically export completed server tasks into public phone storage",
                checked = autoSave,
                onCheckedChange = { phoneDownloadManager.setAutoSaveEnabled(it) }
            )
            SettingSwitchRow(
                title = "Group Folders by Channel / Album",
                subtitle = "Creates Movies/KV Download/<Channel> & Download/KV Download/<Group>",
                checked = organizeByChannel,
                onCheckedChange = { phoneDownloadManager.setOrganizeByChannel(it) }
            )
            SettingSwitchRow(
                title = "Separate by Media Type",
                subtitle = "Routes videos to Movies/, audio to Music/, photos to Pictures/",
                checked = organizeByType,
                onCheckedChange = { phoneDownloadManager.setOrganizeByMediaType(it) }
            )
            Text(
                text = if (organizeByType) {
                    "Active folders: Movies/KV Download/, Music/KV Download/, Pictures/KV Download/, Download/KV Download/"
                } else {
                    "Active folder: Download/KV Download/"
                },
                style = MaterialTheme.typography.labelSmall,
                color = palette.accentText
            )
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

    if (windowState.isTwoPane) {
        LazyVerticalGrid(
            columns = GridCells.Fixed(2),
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(16.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp),
            verticalArrangement = Arrangement.spacedBy(14.dp)
        ) {
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
