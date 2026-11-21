package com.kvdownload.app.ui.screens

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.grid.GridCells
import androidx.compose.foundation.lazy.grid.GridItemSpan
import androidx.compose.foundation.lazy.grid.LazyVerticalGrid
import androidx.compose.foundation.lazy.grid.items
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Cancel
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.ContentPaste
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.Download
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.FolderZip
import androidx.compose.material.icons.filled.Layers
import androidx.compose.material.icons.filled.List
import androidx.compose.material.icons.filled.OpenInNew
import androidx.compose.material.icons.filled.PhoneAndroid
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.PlaylistAddCheck
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.SaveAlt
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Checkbox
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.LinearProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.collectAsState
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateMapOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.platform.LocalClipboardManager
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import com.kvdownload.app.api.AV01Resolver
import com.kvdownload.app.api.DownloadTask
import com.kvdownload.app.api.EnqueueItemInput
import com.kvdownload.app.api.PhoneTransferState
import com.kvdownload.app.api.QueueAddRequest
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.SavedPhoneFile
import com.kvdownload.app.api.ScanEntry
import com.kvdownload.app.api.TaskGroup
import com.kvdownload.app.player.MediaKind
import com.kvdownload.app.player.MediaPlayRequest
import com.kvdownload.app.storage.HlsSegmentDownloader
import com.kvdownload.app.storage.PhoneDownloadManager
import com.kvdownload.app.ui.adaptive.LocalWindowState
import com.kvdownload.app.ui.components.AstryxCard
import com.kvdownload.app.ui.components.AstryxGradientButton
import com.kvdownload.app.ui.components.AstryxMediaThumbnail
import com.kvdownload.app.ui.components.AstryxStatPill
import com.kvdownload.app.ui.components.AstryxStatusBadge
import com.kvdownload.app.ui.components.AstryxTone
import com.kvdownload.app.ui.theme.LocalAstryxPalette
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DownloadCenterScreen(
    tasks: List<DownloadTask>,
    initialSharedUrl: String?,
    onSharedUrlConsumed: () -> Unit,
    phoneDownloadManager: PhoneDownloadManager,
    onRefreshQueue: () -> Unit,
    onPlayMedia: (MediaPlayRequest) -> Unit,
    onShowMessage: (String) -> Unit
) {
    val scope = rememberCoroutineScope()
    val clipboardManager = LocalClipboardManager.current
    val windowState = LocalWindowState.current
    val palette = LocalAstryxPalette.current

    val savedFiles by phoneDownloadManager.savedFiles.collectAsState()
    val phoneTransfers by phoneDownloadManager.transfers.collectAsState()

    var urlText by remember { mutableStateOf("") }
    var selectedFormat by remember { mutableStateOf("highest") }
    var showFormatMenu by remember { mutableStateOf(false) }
    var isSubmitting by remember { mutableStateOf(false) }

    var searchQuery by remember { mutableStateOf("") }
    var statusFilter by remember { mutableStateOf("all") }
    var groupedMode by remember { mutableStateOf(true) }
    var sortBy by remember { mutableStateOf("newest") }
    var showSortMenu by remember { mutableStateOf(false) }

    // Playlist / Channel inspector state
    var showScanSheet by remember { mutableStateOf(false) }
    var scanLoading by remember { mutableStateOf(false) }
    var scanTitle by remember { mutableStateOf("Channel / Playlist") }
    var scanChannel by remember { mutableStateOf("") }
    var scanEntries by remember { mutableStateOf<List<ScanEntry>>(emptyList()) }
    var scanSelectedUrls by remember { mutableStateOf<Set<String>>(emptySet()) }
    var scanError by remember { mutableStateOf<String?>(null) }

    val expandedGroups = remember { mutableStateMapOf<String, Boolean>() }
    var pendingGroupDelete by remember { mutableStateOf<TaskGroup?>(null) }

    LaunchedEffect(initialSharedUrl) {
        if (!initialSharedUrl.isNullOrBlank()) {
            urlText = if (urlText.isBlank()) initialSharedUrl else "$urlText\n$initialSharedUrl"
            onSharedUrlConsumed()
        }
    }

    val parsedUrls = remember(urlText) {
        urlText.lines()
            .map { it.trim() }
            .filter { it.startsWith("http://") || it.startsWith("https://") }
    }

    val directHlsOrAv01Url = remember(parsedUrls) {
        parsedUrls.firstOrNull { u ->
            AV01Resolver.isAV01Url(u) || HlsSegmentDownloader.looksLikeHlsUrl(u)
        }
    }

    val filteredTasks = remember(tasks, statusFilter, searchQuery, sortBy) {
        val q = searchQuery.trim().lowercase()
        val list = tasks.filter { t ->
            val matchStatus = when (statusFilter) {
                "active" -> t.status == "downloading" || t.status == "queued"
                "completed" -> t.status == "completed"
                "failed" -> t.status == "failed" || t.status == "cancelled"
                else -> true
            }
            val matchSearch = q.isBlank() ||
                t.displayTitle().lowercase().contains(q) ||
                t.displayChannel().lowercase().contains(q) ||
                (t.groupTitle ?: "").lowercase().contains(q) ||
                t.url.lowercase().contains(q)
            matchStatus && matchSearch
        }

        when (sortBy) {
            "oldest" -> list.sortedBy { it.createdAt }
            "title" -> list.sortedBy { it.displayTitle().lowercase() }
            "status" -> list.sortedBy { statusOrder(it.status) }
            else -> list.sortedByDescending { it.createdAt }
        }
    }

    val (groups, singles) = remember(filteredTasks, sortBy) {
        buildGroupsAndSingles(filteredTasks, sortBy)
    }

    fun inspectUrl(targetUrl: String) {
        showScanSheet = true
        scanLoading = true
        scanError = null
        scanEntries = emptyList()
        scope.launch {
            try {
                val res = RetrofitClient.getApi().scanUrl(targetUrl, start = 1, limit = 50)
                if (!res.error.isNullOrBlank()) {
                    scanError = res.error
                } else {
                    scanTitle = res.title ?: res.playlistTitle ?: "Channel / Playlist"
                    scanChannel = res.channel ?: res.uploader ?: ""
                    val list = res.entries.orEmpty().ifEmpty {
                        val u = res.webpage_url ?: targetUrl
                        listOf(
                            ScanEntry(
                                id = res.id ?: "single",
                                url = u,
                                title = res.title ?: u,
                                thumbnail = res.thumbnail,
                                channel = scanChannel
                            )
                        )
                    }
                    scanEntries = list
                    scanSelectedUrls = list.map { it.resolvedUrl() }.toSet()
                }
            } catch (e: Exception) {
                scanError = e.message ?: "Scan failed"
            } finally {
                scanLoading = false
            }
        }
    }

    fun submitToServerQueue() {
        if (parsedUrls.isEmpty() || isSubmitting) return
        isSubmitting = true
        scope.launch {
            try {
                val resp = RetrofitClient.getApi().addQueue(
                    QueueAddRequest(
                        urls = parsedUrls,
                        format = selectedFormat,
                        groupType = if (parsedUrls.size > 1) "batch" else null,
                        groupTitle = if (parsedUrls.size > 1) "Mobile Batch (${parsedUrls.size} URLs)" else null
                    )
                )
                if (resp.success) {
                    urlText = ""
                    onRefreshQueue()
                    onShowMessage("Enqueued ${parsedUrls.size} URL(s)")
                } else {
                    onShowMessage(resp.error ?: "Failed to enqueue")
                }
            } catch (e: Exception) {
                onShowMessage(e.message ?: "Request failed")
            } finally {
                isSubmitting = false
            }
        }
    }

    @Composable
    fun ComposerAndFiltersBlock(includeStats: Boolean = true) {
        Column(verticalArrangement = Arrangement.spacedBy(12.dp)) {
            if (includeStats) {
                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    AstryxStatPill("Queue", "${tasks.size}", AstryxTone.Accent)
                    AstryxStatPill(
                        "Active",
                        "${tasks.count { it.status == "downloading" || it.status == "queued" }}",
                        AstryxTone.Info
                    )
                    AstryxStatPill("Done", "${tasks.count { it.status == "completed" }}", AstryxTone.Success)
                    AstryxStatPill("On Phone", "${savedFiles.size}", AstryxTone.Success)
                }
            }

            AstryxCard(emphasized = parsedUrls.isNotEmpty()) {
                Row(
                    modifier = Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween,
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    Column {
                        Text(
                            "Add Media, AV01, or Channel Links",
                            style = MaterialTheme.typography.titleMedium,
                            color = palette.textPrimary
                        )
                        Text(
                            "Supports playlists, .m3u8 HLS streams & AV01 videos",
                            style = MaterialTheme.typography.bodySmall,
                            color = palette.textSecondary
                        )
                    }
                    if (directHlsOrAv01Url != null) {
                        AstryxStatusBadge(
                            text = if (AV01Resolver.isAV01Url(directHlsOrAv01Url)) "AV01 HLS" else "M3U8 HLS",
                            tone = AstryxTone.Accent
                        )
                    }
                }

                OutlinedTextField(
                    value = urlText,
                    onValueChange = { urlText = it },
                    modifier = Modifier.fillMaxWidth(),
                    minLines = 2,
                    maxLines = 5,
                    placeholder = {
                        Text("Paste URLs (one per line) — YouTube, TikTok, Facebook, AV01, .m3u8...")
                    }
                )

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    OutlinedButton(
                        onClick = {
                            clipboardManager.getText()?.text?.let { clip ->
                                val clean = clip.trim()
                                urlText = if (urlText.isBlank()) clean else "$urlText\n$clean"
                            }
                        }
                    ) {
                        Icon(Icons.Default.ContentPaste, contentDescription = null, modifier = Modifier.size(16.dp))
                        Text("Paste", modifier = Modifier.padding(start = 6.dp))
                    }

                    Box {
                        OutlinedButton(onClick = { showFormatMenu = true }) {
                            Text(formatLabel(selectedFormat))
                        }
                        DropdownMenu(
                            expanded = showFormatMenu,
                            onDismissRequest = { showFormatMenu = false }
                        ) {
                            listOf(
                                "highest" to "Highest (8K/4K)",
                                "1080p" to "1080p MP4",
                                "720p" to "720p MP4",
                                "480p" to "480p MP4",
                                "audio" to "Audio (MP3)"
                            ).forEach { (value, label) ->
                                DropdownMenuItem(
                                    text = { Text(label) },
                                    onClick = {
                                        selectedFormat = value
                                        showFormatMenu = false
                                    }
                                )
                            }
                        }
                    }

                    if (parsedUrls.size == 1) {
                        TextButton(onClick = { inspectUrl(parsedUrls.first()) }) {
                            Icon(Icons.Default.PlaylistAddCheck, contentDescription = null, modifier = Modifier.size(18.dp))
                            Text("Pick Items", modifier = Modifier.padding(start = 4.dp))
                        }
                    }
                }

                // Instant Phone HLS / AV01 Stream Actions when detected
                if (directHlsOrAv01Url != null) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.spacedBy(8.dp)
                    ) {
                        OutlinedButton(
                            onClick = {
                                onPlayMedia(
                                    MediaPlayRequest(
                                        id = "direct:${directHlsOrAv01Url.hashCode()}",
                                        title = if (AV01Resolver.isAV01Url(directHlsOrAv01Url)) "AV01 Video Stream" else "HLS M3U8 Stream",
                                        subtitle = "Direct Phone Playback",
                                        primaryUrl = directHlsOrAv01Url,
                                        fallbackUrl = RetrofitClient.browserStreamUrl(directHlsOrAv01Url, AV01Resolver.REFERER),
                                        kind = MediaKind.VIDEO,
                                        isHls = true,
                                        referer = if (AV01Resolver.isAV01Url(directHlsOrAv01Url)) AV01Resolver.REFERER else null,
                                        onSaveToPhone = {
                                            phoneDownloadManager.saveHlsOrAV01ToPhone(
                                                url = directHlsOrAv01Url,
                                                formatPref = selectedFormat
                                            )
                                            onShowMessage("Downloading M3U8 segments to Movies/KV Download...")
                                        }
                                    )
                                )
                            },
                            modifier = Modifier.weight(1f)
                        ) {
                            Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(16.dp))
                            Text("Preview Stream", modifier = Modifier.padding(start = 4.dp), maxLines = 1)
                        }

                        OutlinedButton(
                            onClick = {
                                phoneDownloadManager.saveHlsOrAV01ToPhone(
                                    url = directHlsOrAv01Url,
                                    formatPref = selectedFormat
                                )
                                onShowMessage("Downloading M3U8 stream to phone storage...")
                            },
                            modifier = Modifier.weight(1f)
                        ) {
                            Icon(Icons.Default.PhoneAndroid, contentDescription = null, modifier = Modifier.size(16.dp))
                            Text("M3U8 To Phone", modifier = Modifier.padding(start = 4.dp), maxLines = 1)
                        }
                    }
                }

                AstryxGradientButton(
                    text = if (parsedUrls.size > 1) "Download Group (${parsedUrls.size} links)" else "Queue on Server",
                    icon = Icons.Default.Download,
                    enabled = parsedUrls.isNotEmpty() && !isSubmitting,
                    loading = isSubmitting,
                    onClick = { submitToServerQueue() },
                    modifier = Modifier.fillMaxWidth()
                )
            }

            // Active phone HLS / transfer progress cards
            val activeTransfers = phoneTransfers.values.filter { !it.isCompleted || it.error != null }.take(4)
            if (activeTransfers.isNotEmpty()) {
                AstryxCard(contentPadding = PaddingValues(12.dp)) {
                    Row(
                        modifier = Modifier.fillMaxWidth(),
                        horizontalArrangement = Arrangement.SpaceBetween,
                        verticalAlignment = Alignment.CenterVertically
                    ) {
                        Text("Phone Transfers", style = MaterialTheme.typography.titleSmall, color = palette.textPrimary)
                        TextButton(onClick = { phoneDownloadManager.clearCompletedTransferBadges() }) {
                            Text("Clear")
                        }
                    }
                    activeTransfers.forEach { tr ->
                        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
                            Text(tr.title, style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold, maxLines = 1, overflow = TextOverflow.Ellipsis)
                            if (tr.error != null) {
                                Text(tr.error, style = MaterialTheme.typography.labelSmall, color = palette.error, maxLines = 2)
                            } else {
                                LinearProgressIndicator(
                                    progress = { tr.progress },
                                    modifier = Modifier.fillMaxWidth(),
                                    color = palette.success
                                )
                                Text(
                                    text = tr.statusText ?: "${(tr.progress * 100).toInt()}%",
                                    style = MaterialTheme.typography.labelSmall,
                                    color = palette.success
                                )
                            }
                        }
                    }
                }
            }

            // Filter & Grouping Toolbar
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                OutlinedTextField(
                    value = searchQuery,
                    onValueChange = { searchQuery = it },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                    leadingIcon = { Icon(Icons.Default.Search, contentDescription = null) },
                    trailingIcon = {
                        if (searchQuery.isNotEmpty()) {
                            IconButton(onClick = { searchQuery = "" }) {
                                Icon(Icons.Default.Close, contentDescription = "Clear search")
                            }
                        }
                    },
                    placeholder = { Text("Search title, channel, or group...") }
                )

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                ) {
                    listOf(
                        "all" to "All (${tasks.size})",
                        "active" to "Active (${tasks.count { it.status == "downloading" || it.status == "queued" }})",
                        "completed" to "Done (${tasks.count { it.status == "completed" }})",
                        "failed" to "Failed (${tasks.count { it.status == "failed" || it.status == "cancelled" }})"
                    ).forEach { (key, label) ->
                        FilterChip(
                            selected = statusFilter == key,
                            onClick = { statusFilter = key },
                            label = { Text(label) }
                        )
                    }
                }

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(8.dp),
                    verticalAlignment = Alignment.CenterVertically
                ) {
                    SingleChoiceSegmentedButtonRow {
                        SegmentedButton(
                            selected = groupedMode,
                            onClick = { groupedMode = true },
                            shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
                            icon = { Icon(Icons.Default.Layers, contentDescription = null, modifier = Modifier.size(16.dp)) }
                        ) { Text("Grouped") }
                        SegmentedButton(
                            selected = !groupedMode,
                            onClick = { groupedMode = false },
                            shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
                            icon = { Icon(Icons.Default.List, contentDescription = null, modifier = Modifier.size(16.dp)) }
                        ) { Text("Flat") }
                    }

                    Box {
                        AssistChip(
                            onClick = { showSortMenu = true },
                            label = { Text("Sort: $sortBy") }
                        )
                        DropdownMenu(expanded = showSortMenu, onDismissRequest = { showSortMenu = false }) {
                            listOf("newest", "oldest", "status", "title").forEach { s ->
                                DropdownMenuItem(
                                    text = { Text(s.replaceFirstChar { it.uppercase() }) },
                                    onClick = {
                                        sortBy = s
                                        showSortMenu = false
                                    }
                                )
                            }
                        }
                    }

                    if (tasks.any { it.status == "completed" }) {
                        TextButton(
                            onClick = {
                                scope.launch {
                                    try {
                                        RetrofitClient.getApi().clearCompleted()
                                        onRefreshQueue()
                                    } catch (_: Exception) {}
                                }
                            }
                        ) { Text("Clear Done") }
                    }
                }
            }
        }
    }

    if (windowState.isTwoPane) {
        // Tablet / Unfolded Foldable / Landscape 2-Pane Split
        Row(
            modifier = Modifier
                .fillMaxSize()
                .padding(14.dp),
            horizontalArrangement = Arrangement.spacedBy(14.dp)
        ) {
            Column(
                modifier = Modifier
                    .width(380.dp)
                    .fillMaxHeight()
                    .verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                ComposerAndFiltersBlock(includeStats = true)
            }

            LazyVerticalGrid(
                columns = GridCells.Fixed(windowState.queueGridColumns.coerceAtLeast(1)),
                modifier = Modifier
                    .weight(1f)
                    .fillMaxHeight(),
                contentPadding = PaddingValues(bottom = 24.dp),
                horizontalArrangement = Arrangement.spacedBy(12.dp),
                verticalArrangement = Arrangement.spacedBy(12.dp)
            ) {
                if (filteredTasks.isEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        EmptyQueueCard()
                    }
                } else if (groupedMode) {
                    items(groups, key = { it.groupId }, span = { GridItemSpan(maxLineSpan) }) { group ->
                        val isExpanded = expandedGroups[group.groupId] ?: (group.downloadingCount > 0 || group.tasks.size <= 3)
                        TaskGroupCardView(
                            group = group,
                            isExpanded = isExpanded,
                            onToggleExpand = { expandedGroups[group.groupId] = !isExpanded },
                            savedFiles = savedFiles,
                            phoneTransfers = phoneTransfers,
                            phoneDownloadManager = phoneDownloadManager,
                            onRetryGroup = {
                                scope.launch {
                                    RetrofitClient.getApi().retryGroup(group.groupId)
                                    onRefreshQueue()
                                }
                            },
                            onCancelGroup = {
                                scope.launch {
                                    RetrofitClient.getApi().cancelGroup(group.groupId)
                                    onRefreshQueue()
                                }
                            },
                            onRequestDeleteGroup = { pendingGroupDelete = group },
                            onRefreshQueue = onRefreshQueue,
                            onPlayMedia = onPlayMedia,
                            onShowMessage = onShowMessage
                        )
                    }
                    items(singles, key = { it.id }) { task ->
                        TaskCardView(
                            task = task,
                            savedRecord = savedFiles.firstOrNull { it.serverTaskId == task.id },
                            transfer = phoneTransfers["task:${task.id}"],
                            phoneDownloadManager = phoneDownloadManager,
                            onRefreshQueue = onRefreshQueue,
                            onPlayMedia = onPlayMedia,
                            onShowMessage = onShowMessage
                        )
                    }
                } else {
                    items(filteredTasks, key = { it.id }) { task ->
                        TaskCardView(
                            task = task,
                            savedRecord = savedFiles.firstOrNull { it.serverTaskId == task.id },
                            transfer = phoneTransfers["task:${task.id}"],
                            phoneDownloadManager = phoneDownloadManager,
                            onRefreshQueue = onRefreshQueue,
                            onPlayMedia = onPlayMedia,
                            onShowMessage = onShowMessage
                        )
                    }
                }
            }
        }
    } else {
        // Compact Portrait Phone Layout
        LazyColumn(
            modifier = Modifier.fillMaxSize(),
            contentPadding = PaddingValues(14.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            item {
                ComposerAndFiltersBlock(includeStats = true)
            }

            if (filteredTasks.isEmpty()) {
                item { EmptyQueueCard() }
            } else if (groupedMode) {
                items(groups, key = { it.groupId }) { group ->
                    val isExpanded = expandedGroups[group.groupId] ?: (group.downloadingCount > 0 || group.tasks.size <= 3)
                    TaskGroupCardView(
                        group = group,
                        isExpanded = isExpanded,
                        onToggleExpand = { expandedGroups[group.groupId] = !isExpanded },
                        savedFiles = savedFiles,
                        phoneTransfers = phoneTransfers,
                        phoneDownloadManager = phoneDownloadManager,
                        onRetryGroup = {
                            scope.launch {
                                RetrofitClient.getApi().retryGroup(group.groupId)
                                onRefreshQueue()
                            }
                        },
                        onCancelGroup = {
                            scope.launch {
                                RetrofitClient.getApi().cancelGroup(group.groupId)
                                onRefreshQueue()
                            }
                        },
                        onRequestDeleteGroup = { pendingGroupDelete = group },
                        onRefreshQueue = onRefreshQueue,
                        onPlayMedia = onPlayMedia,
                        onShowMessage = onShowMessage
                    )
                }

                items(singles, key = { it.id }) { task ->
                    TaskCardView(
                        task = task,
                        savedRecord = savedFiles.firstOrNull { it.serverTaskId == task.id },
                        transfer = phoneTransfers["task:${task.id}"],
                        phoneDownloadManager = phoneDownloadManager,
                        onRefreshQueue = onRefreshQueue,
                        onPlayMedia = onPlayMedia,
                        onShowMessage = onShowMessage
                    )
                }
            } else {
                items(filteredTasks, key = { it.id }) { task ->
                    TaskCardView(
                        task = task,
                        savedRecord = savedFiles.firstOrNull { it.serverTaskId == task.id },
                        transfer = phoneTransfers["task:${task.id}"],
                        phoneDownloadManager = phoneDownloadManager,
                        onRefreshQueue = onRefreshQueue,
                        onPlayMedia = onPlayMedia,
                        onShowMessage = onShowMessage
                    )
                }
            }
        }
    }

    pendingGroupDelete?.let { grp ->
        AlertDialog(
            onDismissRequest = { pendingGroupDelete = null },
            title = { Text("Remove Group") },
            text = {
                Text("Remove all ${grp.tasks.size} items in \"${grp.title}\" from queue only, or also delete downloaded files on the server?")
            },
            confirmButton = {
                TextButton(
                    onClick = {
                        val id = grp.groupId
                        pendingGroupDelete = null
                        scope.launch {
                            RetrofitClient.getApi().deleteGroup(id, deleteFiles = true)
                            onRefreshQueue()
                        }
                    }
                ) { Text("Delete & Files", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = {
                Row {
                    TextButton(onClick = { pendingGroupDelete = null }) { Text("Cancel") }
                    TextButton(
                        onClick = {
                            val id = grp.groupId
                            pendingGroupDelete = null
                            scope.launch {
                                RetrofitClient.getApi().deleteGroup(id, deleteFiles = false)
                                onRefreshQueue()
                            }
                        }
                    ) { Text("Queue Only") }
                }
            }
        )
    }

    if (showScanSheet) {
        ModalBottomSheet(onDismissRequest = { showScanSheet = false }) {
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
                    Column(modifier = Modifier.weight(1f)) {
                        Text(scanTitle, style = MaterialTheme.typography.titleMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                        if (scanChannel.isNotBlank()) {
                            Text(scanChannel, style = MaterialTheme.typography.bodySmall, color = palette.textSecondary)
                        }
                    }
                    TextButton(
                        onClick = {
                            val allUrls = scanEntries.map { it.resolvedUrl() }.toSet()
                            scanSelectedUrls = if (scanSelectedUrls.size == allUrls.size) emptySet() else allUrls
                        }
                    ) {
                        Text(if (scanSelectedUrls.size == scanEntries.size) "Clear" else "Select All")
                    }
                }

                when {
                    scanLoading -> {
                        Row(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(24.dp),
                            horizontalArrangement = Arrangement.Center,
                            verticalAlignment = Alignment.CenterVertically
                        ) {
                            CircularProgressIndicator(modifier = Modifier.size(22.dp))
                            Text("Scanning entries...", modifier = Modifier.padding(start = 10.dp))
                        }
                    }
                    scanError != null -> {
                        Text(scanError ?: "Scan failed", color = palette.error)
                    }
                    else -> {
                        LazyColumn(
                            modifier = Modifier
                                .fillMaxWidth()
                                .height(320.dp),
                            verticalArrangement = Arrangement.spacedBy(6.dp)
                        ) {
                            items(scanEntries, key = { it.id + it.resolvedUrl() }) { entry ->
                                val itemUrl = entry.resolvedUrl()
                                val checked = scanSelectedUrls.contains(itemUrl)
                                Row(
                                    modifier = Modifier
                                        .fillMaxWidth()
                                        .clip(RoundedCornerShape(10.dp))
                                        .clickable {
                                            scanSelectedUrls = if (checked) scanSelectedUrls - itemUrl else scanSelectedUrls + itemUrl
                                        }
                                        .padding(6.dp),
                                    verticalAlignment = Alignment.CenterVertically,
                                    horizontalArrangement = Arrangement.spacedBy(8.dp)
                                ) {
                                    Checkbox(
                                        checked = checked,
                                        onCheckedChange = {
                                            scanSelectedUrls = if (checked) scanSelectedUrls - itemUrl else scanSelectedUrls + itemUrl
                                        }
                                    )
                                    if (!entry.thumbnail.isNullOrBlank()) {
                                        AsyncImage(
                                            model = entry.thumbnail,
                                            contentDescription = null,
                                            modifier = Modifier
                                                .size(56.dp, 36.dp)
                                                .clip(RoundedCornerShape(6.dp)),
                                            contentScale = ContentScale.Crop
                                        )
                                    }
                                    Column(modifier = Modifier.weight(1f)) {
                                        Text(entry.title, style = MaterialTheme.typography.bodyMedium, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                        entry.resolvedDuration()?.let { d ->
                                            Text(d, style = MaterialTheme.typography.labelSmall, color = palette.textSecondary)
                                        }
                                    }
                                }
                            }
                        }

                        AstryxGradientButton(
                            text = "Download Selected (${scanSelectedUrls.size})",
                            icon = Icons.Default.Download,
                            enabled = scanSelectedUrls.isNotEmpty(),
                            onClick = {
                                val chosen = scanEntries.filter { scanSelectedUrls.contains(it.resolvedUrl()) }
                                if (chosen.isEmpty()) return@AstryxGradientButton
                                showScanSheet = false
                                scope.launch {
                                    try {
                                        val resp = RetrofitClient.getApi().addQueue(
                                            QueueAddRequest(
                                                items = chosen.map {
                                                    EnqueueItemInput(
                                                        url = it.resolvedUrl(),
                                                        title = it.title,
                                                        thumbnail = it.thumbnail,
                                                        uploader = it.uploader ?: scanChannel,
                                                        channel = it.channel ?: scanChannel,
                                                        duration = it.resolvedDuration(),
                                                        infoJsonPath = it.infoJsonPath
                                                    )
                                                },
                                                format = selectedFormat,
                                                playlistTitle = scanTitle,
                                                channel = scanChannel.ifBlank { null },
                                                groupTitle = scanTitle,
                                                groupType = if (chosen.size > 1) "playlist" else "single"
                                            )
                                        )
                                        if (resp.success) {
                                            urlText = ""
                                            onRefreshQueue()
                                            onShowMessage("Enqueued ${chosen.size} items as \"$scanTitle\"")
                                        }
                                    } catch (e: Exception) {
                                        onShowMessage(e.message ?: "Failed")
                                    }
                                }
                            },
                            modifier = Modifier.fillMaxWidth()
                        )
                    }
                }
            }
        }
    }
}

@Composable
private fun EmptyQueueCard() {
    val palette = LocalAstryxPalette.current
    AstryxCard {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(16.dp),
            horizontalAlignment = Alignment.CenterHorizontally,
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Icon(
                Icons.Default.Download,
                contentDescription = null,
                modifier = Modifier.size(36.dp),
                tint = palette.accent
            )
            Text("No downloads match", style = MaterialTheme.typography.titleMedium, color = palette.textPrimary)
            Text(
                "Paste links above or open the Browser tab to sniff and capture M3U8 streams.",
                style = MaterialTheme.typography.bodySmall,
                color = palette.textSecondary
            )
        }
    }
}

@Composable
private fun TaskGroupCardView(
    group: TaskGroup,
    isExpanded: Boolean,
    onToggleExpand: () -> Unit,
    savedFiles: List<SavedPhoneFile>,
    phoneTransfers: Map<String, PhoneTransferState>,
    phoneDownloadManager: PhoneDownloadManager,
    onRetryGroup: () -> Unit,
    onCancelGroup: () -> Unit,
    onRequestDeleteGroup: () -> Unit,
    onRefreshQueue: () -> Unit,
    onPlayMedia: (MediaPlayRequest) -> Unit,
    onShowMessage: (String) -> Unit
) {
    val palette = LocalAstryxPalette.current
    AstryxCard(emphasized = group.downloadingCount > 0) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .clickable(onClick = onToggleExpand),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(10.dp)
        ) {
            val thumb = group.tasks.firstOrNull()?.let { RetrofitClient.taskThumbnailUrl(it) }
            AsyncImage(
                model = thumb,
                contentDescription = null,
                modifier = Modifier
                    .size(72.dp, 46.dp)
                    .clip(RoundedCornerShape(8.dp))
                    .background(palette.muted),
                contentScale = ContentScale.Crop
            )

            Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
                Text(
                    text = group.title,
                    style = MaterialTheme.typography.titleSmall,
                    color = palette.textPrimary,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
                Row(
                    modifier = Modifier.horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    AstryxStatusBadge(group.platform, AstryxTone.Accent)
                    AstryxStatusBadge("${group.completedCount}/${group.tasks.size} DONE", AstryxTone.Success)
                    if (group.failedCount > 0) {
                        AstryxStatusBadge("${group.failedCount} FAILED", AstryxTone.Error)
                    }
                }
            }

            Icon(
                imageVector = if (isExpanded) Icons.Default.ExpandLess else Icons.Default.ExpandMore,
                contentDescription = null,
                tint = palette.textSecondary
            )
        }

        if (group.completedCount < group.tasks.size) {
            LinearProgressIndicator(
                progress = { (group.overallPercent / 100f).coerceIn(0f, 1f) },
                modifier = Modifier.fillMaxWidth(),
                color = palette.accent
            )
        }

        Row(
            modifier = Modifier
                .fillMaxWidth()
                .horizontalScroll(rememberScrollState()),
            horizontalArrangement = Arrangement.spacedBy(6.dp),
            verticalAlignment = Alignment.CenterVertically
        ) {
            if (group.completedCount > 0) {
                AssistChip(
                    onClick = {
                        phoneDownloadManager.saveGroupToPhone(group.tasks)
                        onShowMessage("Saving completed items in \"${group.title}\" to phone storage")
                    },
                    leadingIcon = { Icon(Icons.Default.PhoneAndroid, contentDescription = null, modifier = Modifier.size(16.dp)) },
                    label = { Text("Save All (${group.completedCount})") }
                )
                AssistChip(
                    onClick = {
                        phoneDownloadManager.saveZipArchiveToPhone(
                            zipUrl = RetrofitClient.groupZipUrl(group.groupId),
                            archiveLabel = group.title,
                            channelOrGroup = group.title
                        )
                        onShowMessage("Downloading ZIP archive to Downloads/KV Download")
                    },
                    leadingIcon = { Icon(Icons.Default.FolderZip, contentDescription = null, modifier = Modifier.size(16.dp)) },
                    label = { Text("Save ZIP") }
                )
            }
            if (group.failedCount > 0 || group.cancelledCount > 0) {
                AssistChip(
                    onClick = onRetryGroup,
                    leadingIcon = { Icon(Icons.Default.Refresh, contentDescription = null, modifier = Modifier.size(16.dp)) },
                    label = { Text("Retry Failed") }
                )
            }
            if (group.downloadingCount > 0 || group.queuedCount > 0) {
                AssistChip(
                    onClick = onCancelGroup,
                    leadingIcon = { Icon(Icons.Default.Cancel, contentDescription = null, modifier = Modifier.size(16.dp)) },
                    label = { Text("Cancel Active") }
                )
            }
            IconButton(onClick = onRequestDeleteGroup) {
                Icon(
                    Icons.Default.DeleteOutline,
                    contentDescription = "Delete group",
                    tint = palette.error
                )
            }
        }

        if (isExpanded) {
            HorizontalDivider(color = palette.borderSubtle)
            val slice = group.tasks.take(25)
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                slice.forEach { task ->
                    TaskCardView(
                        task = task,
                        savedRecord = savedFiles.firstOrNull { it.serverTaskId == task.id },
                        transfer = phoneTransfers["task:${task.id}"],
                        phoneDownloadManager = phoneDownloadManager,
                        onRefreshQueue = onRefreshQueue,
                        onPlayMedia = onPlayMedia,
                        onShowMessage = onShowMessage
                    )
                }
                if (group.tasks.size > slice.size) {
                    Text(
                        text = "Showing ${slice.size} of ${group.tasks.size} items in this group",
                        style = MaterialTheme.typography.bodySmall,
                        color = palette.textSecondary
                    )
                }
            }
        }
    }
}

@Composable
private fun TaskCardView(
    task: DownloadTask,
    savedRecord: SavedPhoneFile?,
    transfer: PhoneTransferState?,
    phoneDownloadManager: PhoneDownloadManager,
    onRefreshQueue: () -> Unit,
    onPlayMedia: (MediaPlayRequest) -> Unit,
    onShowMessage: (String) -> Unit
) {
    val scope = rememberCoroutineScope()
    val palette = LocalAstryxPalette.current
    val isCompleted = task.status == "completed"
    val isDownloading = task.status == "downloading"
    val isRetryable = task.status == "failed" || task.status == "cancelled"

    val lowerName = (task.mediaName ?: "").lowercase()
    val mediaKind = when {
        task.format?.contains("audio", ignoreCase = true) == true ||
            lowerName.endsWith(".mp3") || lowerName.endsWith(".m4a") ||
            lowerName.endsWith(".flac") || lowerName.endsWith(".wav") -> MediaKind.AUDIO
        lowerName.endsWith(".jpg") || lowerName.endsWith(".jpeg") ||
            lowerName.endsWith(".png") || lowerName.endsWith(".webp") -> MediaKind.IMAGE
        else -> MediaKind.VIDEO
    }
    val typeString = when (mediaKind) {
        MediaKind.AUDIO -> "audio"
        MediaKind.IMAGE -> "photo"
        MediaKind.VIDEO -> "video"
    }

    fun launchPlayback() {
        val serverInline = RetrofitClient.downloadTaskUrl(task.id, inline = true, mediaId = task.mediaId)
        onPlayMedia(
            MediaPlayRequest(
                id = "task:${task.id}",
                title = task.displayTitle(),
                subtitle = task.displayChannel(),
                thumbnailUrl = RetrofitClient.taskThumbnailUrl(task),
                primaryUrl = savedRecord?.contentUri ?: serverInline,
                fallbackUrl = "$serverInline&compat=1",
                kind = mediaKind,
                isLocalUri = savedRecord != null,
                onSaveToPhone = {
                    phoneDownloadManager.saveTaskToPhone(task)
                    onShowMessage("Saving to phone storage...")
                }
            )
        )
    }

    Surface(
        shape = RoundedCornerShape(12.dp),
        color = palette.muted,
        border = BorderStroke(1.dp, palette.borderSubtle)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(10.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            AstryxMediaThumbnail(
                imageUrl = RetrofitClient.taskThumbnailUrl(task),
                mediaType = typeString,
                badgeText = task.platform ?: task.format?.uppercase(),
                durationText = task.duration ?: task.humanSize,
                showPlayOverlay = isCompleted,
                onClick = if (isCompleted) ::launchPlayback else null
            )

            Text(
                text = task.displayTitle(),
                style = MaterialTheme.typography.titleSmall,
                color = palette.textPrimary,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis
            )

            Row(
                modifier = Modifier
                    .fillMaxWidth()
                    .horizontalScroll(rememberScrollState()),
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                AstryxStatusBadge(
                    text = task.status.uppercase(),
                    tone = when (task.status) {
                        "completed" -> AstryxTone.Success
                        "downloading" -> AstryxTone.Accent
                        "queued" -> AstryxTone.Info
                        else -> AstryxTone.Error
                    }
                )
                AstryxStatusBadge(task.displayChannel(), AstryxTone.Neutral)
                task.humanSize?.let { AstryxStatusBadge(it, AstryxTone.Neutral) }
            }

            if (!isCompleted && task.errorMessage.isNullOrBlank()) {
                LinearProgressIndicator(
                    progress = { (task.percent / 100.0).toFloat().coerceIn(0f, 1f) },
                    modifier = Modifier.fillMaxWidth(),
                    color = palette.accent
                )
                if (isDownloading) {
                    Text(
                        text = "${task.percent.toInt()}% · ${task.speed ?: "..."} ${task.eta?.let { "· ETA $it" } ?: ""}",
                        style = MaterialTheme.typography.labelSmall,
                        color = palette.accentText
                    )
                }
            }

            if (!task.errorMessage.isNullOrBlank()) {
                Text(
                    text = task.errorMessage,
                    style = MaterialTheme.typography.bodySmall,
                    color = palette.error,
                    maxLines = 2,
                    overflow = TextOverflow.Ellipsis
                )
            }

            if (transfer != null && !transfer.isCompleted && transfer.error == null) {
                LinearProgressIndicator(
                    progress = { transfer.progress },
                    modifier = Modifier.fillMaxWidth(),
                    color = palette.success
                )
                Text(
                    text = transfer.statusText ?: "Saving to phone storage ${(transfer.progress * 100).toInt()}%",
                    style = MaterialTheme.typography.labelSmall,
                    color = palette.success
                )
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    if (isCompleted) {
                        Button(
                            onClick = ::launchPlayback,
                            contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp)
                        ) {
                            Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(16.dp))
                            Text(if (mediaKind == MediaKind.IMAGE) "View" else "Play", modifier = Modifier.padding(start = 4.dp))
                        }

                        if (savedRecord != null) {
                            OutlinedButton(
                                onClick = { phoneDownloadManager.openOnDevice(savedRecord) },
                                contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                            ) {
                                Icon(Icons.Default.CheckCircle, contentDescription = null, modifier = Modifier.size(15.dp), tint = palette.success)
                                Spacer(Modifier.width(4.dp))
                                Icon(Icons.Default.OpenInNew, contentDescription = null, modifier = Modifier.size(14.dp))
                                Text("Saved", modifier = Modifier.padding(start = 4.dp))
                            }
                        } else {
                            OutlinedButton(
                                onClick = {
                                    phoneDownloadManager.saveTaskToPhone(task)
                                    onShowMessage("Downloading \"${task.displayTitle()}\" to phone storage")
                                },
                                contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                            ) {
                                Icon(Icons.Default.SaveAlt, contentDescription = null, modifier = Modifier.size(16.dp))
                                Text("To Phone", modifier = Modifier.padding(start = 4.dp))
                            }
                        }
                    } else if (isRetryable) {
                        OutlinedButton(
                            onClick = {
                                scope.launch {
                                    RetrofitClient.getApi().retryTask(task.id)
                                    onRefreshQueue()
                                }
                            },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                        ) {
                            Icon(Icons.Default.Refresh, contentDescription = null, modifier = Modifier.size(16.dp))
                            Text("Retry", modifier = Modifier.padding(start = 4.dp))
                        }
                    } else {
                        OutlinedButton(
                            onClick = {
                                scope.launch {
                                    RetrofitClient.getApi().cancelTask(task.id)
                                    onRefreshQueue()
                                }
                            },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                        ) {
                            Icon(Icons.Default.Cancel, contentDescription = null, modifier = Modifier.size(16.dp))
                            Text("Cancel", modifier = Modifier.padding(start = 4.dp))
                        }
                    }
                }

                IconButton(
                    onClick = {
                        scope.launch {
                            RetrofitClient.getApi().deleteTask(task.id, deleteFile = false)
                            onRefreshQueue()
                        }
                    }
                ) {
                    Icon(Icons.Default.Close, contentDescription = "Remove task", tint = palette.textSecondary)
                }
            }
        }
    }
}

private fun formatLabel(format: String): String = when (format) {
    "1080p" -> "1080p"
    "720p" -> "720p"
    "480p" -> "480p"
    "audio" -> "MP3"
    else -> "Highest"
}

private fun statusOrder(status: String): Int = when (status) {
    "downloading" -> 0
    "queued" -> 1
    "failed" -> 2
    "cancelled" -> 3
    else -> 4
}

private fun buildGroupsAndSingles(
    tasks: List<DownloadTask>,
    sortBy: String
): Pair<List<TaskGroup>, List<DownloadTask>> {
    val buckets = linkedMapOf<String, MutableList<DownloadTask>>()
    val singles = mutableListOf<DownloadTask>()

    for (task in tasks) {
        val key = task.groupId?.takeIf { it.isNotBlank() }
            ?: task.playlistTitle?.takeIf { it.isNotBlank() }?.let { "pl:$it" }
            ?: task.channel?.takeIf { it.isNotBlank() }?.let { "ch:${task.platform ?: "Web"}:$it" }
            ?: ""
        if (key.isBlank()) {
            singles.add(task)
        } else {
            buckets.getOrPut(key) { mutableListOf() }.add(task)
        }
    }

    val groups = mutableListOf<TaskGroup>()
    for ((key, list) in buckets) {
        val explicit = !list.first().groupId.isNullOrBlank() || !list.first().playlistTitle.isNullOrBlank()
        if (list.size == 1 && !explicit) {
            singles.add(list.first())
            continue
        }
        var done = 0
        var active = 0
        var queued = 0
        var failed = 0
        var cancelled = 0
        var totalBytes = 0L
        var pctSum = 0.0
        var latest = list.first().createdAt

        for (t in list) {
            when (t.status) {
                "completed" -> {
                    done++
                    pctSum += 100.0
                }
                "downloading" -> {
                    active++
                    pctSum += t.percent.coerceIn(0.0, 100.0)
                }
                "queued" -> queued++
                "failed" -> failed++
                "cancelled" -> cancelled++
            }
            totalBytes += t.totalBytes ?: 0L
            if (t.createdAt > latest) latest = t.createdAt
        }

        val first = list.first()
        groups.add(
            TaskGroup(
                groupId = first.groupId ?: key,
                title = first.groupTitle ?: first.playlistTitle ?: first.channel ?: "Batch (${list.size})",
                groupType = first.groupType ?: if (!first.playlistTitle.isNullOrBlank()) "playlist" else "channel",
                platform = first.platform ?: "Web",
                tasks = list,
                completedCount = done,
                downloadingCount = active,
                queuedCount = queued,
                failedCount = failed,
                cancelledCount = cancelled,
                totalBytes = totalBytes,
                overallPercent = if (list.isNotEmpty()) (pctSum / list.size).toInt() else 0,
                latestCreatedAt = latest
            )
        )
    }

    val sortedGroups = when (sortBy) {
        "oldest" -> groups.sortedBy { it.latestCreatedAt }
        "title" -> groups.sortedBy { it.title.lowercase() }
        "status" -> groups.sortedByDescending { it.downloadingCount }
        else -> groups.sortedByDescending { it.latestCreatedAt }
    }

    return sortedGroups to singles
}
