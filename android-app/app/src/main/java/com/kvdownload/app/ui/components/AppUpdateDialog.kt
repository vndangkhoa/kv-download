package com.kvdownload.app.ui.components

import android.content.Intent
import android.net.Uri
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.OpenInBrowser
import androidx.compose.material.icons.filled.SystemUpdateAlt
import androidx.compose.material3.Icon
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableLongStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.unit.dp
import androidx.compose.ui.window.Dialog
import com.kvdownload.app.ui.theme.LocalAstryxPalette
import com.kvdownload.app.update.AppUpdateInfo
import com.kvdownload.app.update.AppUpdateManager
import kotlinx.coroutines.launch
import java.io.File
import java.util.Locale

@Composable
fun AppUpdateDialog(
    updateInfo: AppUpdateInfo,
    updateManager: AppUpdateManager,
    onDismiss: () -> Unit,
    onShowMessage: (String) -> Unit
) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    val palette = LocalAstryxPalette.current

    var isDownloading by remember { mutableStateOf(false) }
    var progress by remember { mutableFloatStateOf(0f) }
    var bytesRead by remember { mutableLongStateOf(0L) }
    var totalBytes by remember { mutableLongStateOf(-1L) }
    var errorMsg by remember { mutableStateOf<String?>(null) }
    var pendingApkFile by remember { mutableStateOf<File?>(null) }

    val unknownSourcesLauncher = rememberLauncherForActivityResult(
        contract = ActivityResultContracts.StartActivityForResult()
    ) {
        val apk = pendingApkFile
        if (apk != null && apk.exists()) {
            if (updateManager.canRequestPackageInstalls()) {
                updateManager.launchInstaller(apk).onSuccess {
                    onDismiss()
                }.onFailure { err ->
                    errorMsg = err.message ?: "Unable to open APK installer"
                }
            } else {
                errorMsg = "Allow \"Install unknown apps\" for KV Download to install updates."
            }
        }
    }

    fun startDownloadAndInstall() {
        if (isDownloading) return
        isDownloading = true
        errorMsg = null
        progress = 0f
        bytesRead = 0L
        totalBytes = -1L

        scope.launch {
            val dlResult = updateManager.downloadApk(updateInfo) { p, read, total ->
                progress = p
                bytesRead = read
                totalBytes = total
            }
            isDownloading = false

            dlResult.onSuccess { apkFile ->
                pendingApkFile = apkFile
                if (updateManager.canRequestPackageInstalls()) {
                    updateManager.launchInstaller(apkFile).onSuccess {
                        onDismiss()
                    }.onFailure { err ->
                        errorMsg = err.message ?: "Failed to launch Android package installer"
                    }
                } else {
                    onShowMessage("Enable \"Install unknown apps\" to continue installation")
                    try {
                        unknownSourcesLauncher.launch(updateManager.buildUnknownSourcesSettingsIntent())
                    } catch (e: Exception) {
                        errorMsg = e.message ?: "Open Settings to enable unknown app installs"
                    }
                }
            }.onFailure { err ->
                errorMsg = err.message ?: "Failed to download APK"
            }
        }
    }

    Dialog(onDismissRequest = { if (!isDownloading) onDismiss() }) {
        Surface(
            shape = RoundedCornerShape(18.dp),
            color = palette.surface,
            border = BorderStroke(1.dp, palette.borderStrong),
            modifier = Modifier.fillMaxWidth()
        ) {
            Column(
                modifier = Modifier
                    .fillMaxWidth()
                    .padding(18.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                Row(
                    verticalAlignment = Alignment.CenterVertically,
                    horizontalArrangement = Arrangement.spacedBy(10.dp)
                ) {
                    Icon(
                        imageVector = Icons.Default.SystemUpdateAlt,
                        contentDescription = null,
                        tint = palette.accentText,
                        modifier = Modifier.size(24.dp)
                    )
                    Column(modifier = Modifier.weight(1f)) {
                        Text(
                            text = if (updateInfo.hasUpdate) "App Update Available" else "KV Download APK",
                            style = MaterialTheme.typography.titleMedium,
                            color = palette.textPrimary
                        )
                        Text(
                            text = "v${updateInfo.currentVersion} -> v${updateInfo.latestVersion}",
                            style = MaterialTheme.typography.labelMedium.copy(fontFamily = FontFamily.Monospace),
                            color = palette.accentText
                        )
                    }
                    AstryxStatusBadge(
                        text = if (updateInfo.hasUpdate) "NEW" else "LATEST",
                        tone = if (updateInfo.hasUpdate) AstryxTone.Warning else AstryxTone.Success
                    )
                }

                if (updateInfo.releaseNotes.isNotBlank()) {
                    Surface(
                        shape = RoundedCornerShape(10.dp),
                        color = palette.muted,
                        border = BorderStroke(1.dp, palette.borderSubtle),
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(
                            text = updateInfo.releaseNotes,
                            style = MaterialTheme.typography.bodySmall,
                            color = palette.textSecondary,
                            modifier = Modifier
                                .fillMaxWidth()
                                .verticalScroll(rememberScrollState())
                                .padding(10.dp),
                            maxLines = 8
                        )
                    }
                }

                if (isDownloading) {
                    Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
                        if (progress >= 0f) {
                            LinearProgressIndicator(
                                progress = { progress },
                                modifier = Modifier.fillMaxWidth(),
                                color = palette.accent
                            )
                        } else {
                            LinearProgressIndicator(
                                modifier = Modifier.fillMaxWidth(),
                                color = palette.accent
                            )
                        }
                        Text(
                            text = formatDownloadProgress(bytesRead, totalBytes, progress),
                            style = MaterialTheme.typography.labelSmall.copy(fontFamily = FontFamily.Monospace),
                            color = palette.textSecondary
                        )
                    }
                }

                if (!errorMsg.isNullOrBlank()) {
                    Surface(
                        shape = RoundedCornerShape(10.dp),
                        color = palette.error.copy(alpha = 0.14f),
                        border = BorderStroke(1.dp, palette.error.copy(alpha = 0.35f)),
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(
                            text = errorMsg!!,
                            style = MaterialTheme.typography.bodySmall,
                            color = palette.error,
                            modifier = Modifier.padding(10.dp)
                        )
                    }
                }

                AstryxGradientButton(
                    text = if (isDownloading) "Downloading APK..." else "Download & Install v${updateInfo.latestVersion}",
                    icon = Icons.Default.Download,
                    loading = isDownloading,
                    enabled = !isDownloading,
                    onClick = { startDownloadAndInstall() },
                    modifier = Modifier.fillMaxWidth()
                )

                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    OutlinedButton(
                        onClick = {
                            val target = updateInfo.downloadUrl.ifBlank { updateInfo.releasePageUrl }
                            try {
                                context.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(target)))
                            } catch (_: Exception) {
                                onShowMessage("No browser available to open $target")
                            }
                        }
                    ) {
                        Icon(Icons.Default.OpenInBrowser, contentDescription = null, modifier = Modifier.size(16.dp))
                        Text("Open in Browser", modifier = Modifier.padding(start = 6.dp))
                    }

                    TextButton(
                        onClick = onDismiss,
                        enabled = !isDownloading
                    ) {
                        Text("Close")
                    }
                }
            }
        }
    }
}

private fun formatDownloadProgress(bytesRead: Long, totalBytes: Long, progress: Float): String {
    val readMb = String.format(Locale.US, "%.1f MB", bytesRead / (1024f * 1024f))
    return if (totalBytes > 0) {
        val totalMb = String.format(Locale.US, "%.1f MB", totalBytes / (1024f * 1024f))
        val pct = (progress * 100f).toInt().coerceIn(0, 100)
        "Downloading APK: $readMb / $totalMb ($pct%)"
    } else {
        "Downloading APK: $readMb..."
    }
}
