package com.kvdownload.app.storage

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.wifi.WifiManager
import android.os.Build
import android.os.IBinder
import android.os.PowerManager
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import com.kvdownload.app.MainActivity
import com.kvdownload.app.api.PhoneTransferState
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch

/**
 * Keeps Android downloads running at full network speed when the app is minimized
 * or the screen is locked, holding partial wake + Wi-Fi locks only while transfers run.
 */
class DownloadForegroundService : Service() {

    private val serviceScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)
    private var observerJob: Job? = null
    private var wakeLock: PowerManager.WakeLock? = null
    private var wifiLock: WifiManager.WifiLock? = null
    private var completedInRun = 0
    private var lastCompletedSnapshotKeys = emptySet<String>()

    override fun onCreate() {
        super.onCreate()
        ensureChannels(this)
        val pm = getSystemService(Context.POWER_SERVICE) as? PowerManager
        wakeLock = pm?.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "kvdownload:DownloadWakeLock")?.apply {
            setReferenceCounted(false)
        }
        val wm = applicationContext.getSystemService(Context.WIFI_SERVICE) as? WifiManager
        val wifiMode = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            WifiManager.WIFI_MODE_FULL_LOW_LATENCY
        } else {
            @Suppress("DEPRECATION")
            WifiManager.WIFI_MODE_FULL_HIGH_PERF
        }
        wifiLock = wm?.createWifiLock(wifiMode, "kvdownload:DownloadWifiLock")?.apply {
            setReferenceCounted(false)
        }
        lastCompletedSnapshotKeys = PhoneDownloadManager.getInstance(applicationContext)
            .transfers.value.filterValues { it.isCompleted }.keys.toSet()
        startObservingTransfers()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        val manager = PhoneDownloadManager.getInstance(applicationContext)
        val activeNow = activeList(manager.transfers.value)

        // Always promote immediately to avoid ForegroundServiceDidNotStartInTimeException.
        val initialNotif = buildActiveNotification(activeNow)
        val fgsType = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC
        } else {
            0
        }
        ServiceCompat.startForeground(this, ACTIVE_NOTIF_ID, initialNotif, fgsType)

        if (intent?.action == ACTION_CANCEL_ALL) {
            manager.cancelAllTransfers()
            stopServiceCleanly(postDoneSummary = false)
            return START_NOT_STICKY
        }

        if (activeNow.isEmpty()) {
            stopServiceCleanly(postDoneSummary = false)
            return START_NOT_STICKY
        }

        acquireLocks()
        return START_NOT_STICKY
    }

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        observerJob?.cancel()
        serviceScope.cancel()
        releaseLocks()
        super.onDestroy()
    }

    private fun startObservingTransfers() {
        observerJob?.cancel()
        val manager = PhoneDownloadManager.getInstance(applicationContext)
        observerJob = serviceScope.launch {
            manager.transfers.collect { map ->
                val doneKeys = map.filterValues { it.isCompleted }.keys
                val newlyDone = (doneKeys - lastCompletedSnapshotKeys).size
                if (newlyDone > 0) {
                    completedInRun += newlyDone
                }
                lastCompletedSnapshotKeys = doneKeys.toSet()

                val active = activeList(map)
                if (active.isEmpty()) {
                    stopServiceCleanly(postDoneSummary = completedInRun > 0)
                } else {
                    acquireLocks()
                    val nm = getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager
                    runCatching { nm?.notify(ACTIVE_NOTIF_ID, buildActiveNotification(active)) }
                    // Throttle notification updates so Android never rate-limits progress bars.
                    delay(450L)
                }
            }
        }
    }

    private fun stopServiceCleanly(postDoneSummary: Boolean) {
        val countDone = completedInRun
        completedInRun = 0
        releaseLocks()
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        if (postDoneSummary && countDone > 0) {
            postCompletionNotification(countDone)
        }
        stopSelf()
    }

    private fun acquireLocks() {
        runCatching {
            wakeLock?.let { if (!it.isHeld) it.acquire(4 * 60 * 60 * 1000L) }
        }
        runCatching {
            wifiLock?.let { if (!it.isHeld) it.acquire() }
        }
    }

    private fun releaseLocks() {
        runCatching { wakeLock?.let { if (it.isHeld) it.release() } }
        runCatching { wifiLock?.let { if (it.isHeld) it.release() } }
    }

    private fun activeList(map: Map<String, PhoneTransferState>): List<PhoneTransferState> =
        map.values.filter { !it.isCompleted && it.error == null }

    private fun buildActiveNotification(active: List<PhoneTransferState>): Notification {
        val openIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val openPi = PendingIntent.getActivity(
            this,
            101,
            openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        val cancelIntent = Intent(this, DownloadForegroundService::class.java).apply {
            action = ACTION_CANCEL_ALL
        }
        val cancelPi = PendingIntent.getService(
            this,
            102,
            cancelIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )

        if (active.isEmpty()) {
            return NotificationCompat.Builder(this, CHANNEL_ACTIVE)
                .setSmallIcon(android.R.drawable.stat_sys_download)
                .setContentTitle("KV Download")
                .setContentText("Preparing phone vault transfers...")
                .setContentIntent(openPi)
                .setOnlyAlertOnce(true)
                .setOngoing(true)
                .setSilent(true)
                .build()
        }

        val totalSpeed = active.sumOf { it.speedBytesPerSec }
        val avgProgress = active.map { it.progress.coerceIn(0f, 1f) }.average().toFloat()
        val pct = (avgProgress * 100f).toInt().coerceIn(0, 100)
        val hasProgress = active.any { it.progress > 0f || it.bytesDownloaded > 0L }

        val title = if (active.size == 1) {
            active[0].title
        } else {
            "Downloading ${active.size} files ($pct%)"
        }

        val speedPart = if (totalSpeed > 0L) PhoneDownloadManager.formatSpeed(totalSpeed) else null
        val subText = if (active.size == 1) {
            val item = active[0]
            listOfNotNull(
                if (hasProgress) "$pct%" else "Queued",
                speedPart,
                if (item.bytesDownloaded > 0L) {
                    if (item.totalBytes > 0L) {
                        "${PhoneDownloadManager.formatFileSize(item.bytesDownloaded)} / ${PhoneDownloadManager.formatFileSize(item.totalBytes)}"
                    } else {
                        PhoneDownloadManager.formatFileSize(item.bytesDownloaded)
                    }
                } else null
            ).joinToString(" · ").ifBlank { item.statusText ?: "Downloading..." }
        } else {
            val runningCount = active.count { it.bytesDownloaded > 0L || it.progress > 0f }
            val queuedCount = (active.size - runningCount).coerceAtLeast(0)
            listOfNotNull(
                "$pct%",
                speedPart,
                "$runningCount active" + if (queuedCount > 0) ", $queuedCount queued" else ""
            ).joinToString(" · ")
        }

        return NotificationCompat.Builder(this, CHANNEL_ACTIVE)
            .setSmallIcon(android.R.drawable.stat_sys_download)
            .setContentTitle(title)
            .setContentText(subText)
            .setContentIntent(openPi)
            .setOnlyAlertOnce(true)
            .setOngoing(true)
            .setSilent(true)
            .setPriority(NotificationCompat.PRIORITY_LOW)
            .setCategory(NotificationCompat.CATEGORY_PROGRESS)
            .setProgress(100, pct, !hasProgress)
            .addAction(android.R.drawable.ic_menu_close_clear_cancel, "Cancel All", cancelPi)
            .build()
    }

    private fun postCompletionNotification(doneCount: Int) {
        val openIntent = Intent(this, MainActivity::class.java).apply {
            flags = Intent.FLAG_ACTIVITY_SINGLE_TOP or Intent.FLAG_ACTIVITY_CLEAR_TOP
        }
        val openPi = PendingIntent.getActivity(
            this,
            103,
            openIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
        )
        val notif = NotificationCompat.Builder(this, CHANNEL_DONE)
            .setSmallIcon(android.R.drawable.stat_sys_download_done)
            .setContentTitle("Saved $doneCount file${if (doneCount == 1) "" else "s"} to Private Vault")
            .setContentText("Tap KV Download to play or manage your private files.")
            .setContentIntent(openPi)
            .setAutoCancel(true)
            .build()
        val nm = getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager
        runCatching { nm?.notify(DONE_NOTIF_ID, notif) }
    }

    companion object {
        private const val CHANNEL_ACTIVE = "kv_downloads_active"
        private const val CHANNEL_DONE = "kv_downloads_done"
        private const val ACTIVE_NOTIF_ID = 4701
        private const val DONE_NOTIF_ID = 4702
        const val ACTION_SYNC = "com.kvdownload.app.action.SYNC_DOWNLOADS"
        const val ACTION_CANCEL_ALL = "com.kvdownload.app.action.CANCEL_ALL_DOWNLOADS"

        fun ensureChannels(context: Context) {
            if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
            val nm = context.getSystemService(Context.NOTIFICATION_SERVICE) as? NotificationManager ?: return
            if (nm.getNotificationChannel(CHANNEL_ACTIVE) == null) {
                nm.createNotificationChannel(
                    NotificationChannel(
                        CHANNEL_ACTIVE,
                        "Background Downloads",
                        NotificationManager.IMPORTANCE_LOW
                    ).apply {
                        description = "Shows live download progress and speed while saving files"
                        setShowBadge(false)
                    }
                )
            }
            if (nm.getNotificationChannel(CHANNEL_DONE) == null) {
                nm.createNotificationChannel(
                    NotificationChannel(
                        CHANNEL_DONE,
                        "Download Alerts",
                        NotificationManager.IMPORTANCE_DEFAULT
                    ).apply {
                        description = "Alerts when background phone downloads finish"
                    }
                )
            }
        }

        fun sync(context: Context) {
            val appCtx = context.applicationContext
            ensureChannels(appCtx)
            val intent = Intent(appCtx, DownloadForegroundService::class.java).apply {
                action = ACTION_SYNC
            }
            runCatching {
                ContextCompat.startForegroundService(appCtx, intent)
            }
        }
    }
}
