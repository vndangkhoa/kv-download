package com.kvdownload.app.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.DriveFileMove
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.FolderZip
import androidx.compose.material.icons.filled.Layers
import androidx.compose.material.icons.filled.List
import androidx.compose.material.icons.filled.OpenInNew
import androidx.compose.material.icons.filled.PhoneAndroid
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.SaveAlt
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Share
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
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
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SegmentedButton
import androidx.compose.material3.SegmentedButtonDefaults
import androidx.compose.material3.SingleChoiceSegmentedButtonRow
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.draw.clip
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import coil.compose.AsyncImage
import com.kvdownload.app.api.GalleryFolder
import com.kvdownload.app.api.GalleryItem
import com.kvdownload.app.api.PhoneTransferState
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.SavedPhoneFile
import com.kvdownload.app.storage.PhoneDownloadManager
import com.kvdownload.app.ui.components.PlaybackRequest
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MediaGalleryScreen(
    phoneDownloadManager: PhoneDownloadManager,
    onPlayMedia: (PlaybackRequest) -> Unit,
    onShowMessage: (String) -> Unit
) {
    val scope = rememberCoroutineScope()
    val savedFiles by phoneDownloadManager.savedFiles.collectAsState()
    val phoneTransfers by phoneDownloadManager.transfers.collectAsState()

    var sourceTab by remember { mutableStateOf("server") } // "server" | "phone"
    var serverItems by remember { mutableStateOf<List<GalleryItem>>(emptyList()) }
    var serverFolders by remember { mutableStateOf<List<GalleryFolder>>(emptyList()) }
    var isLoading by remember { mutableStateOf(false) }

    var searchQuery by remember { mutableStateOf("") }
    var categoryFilter by remember { mutableStateOf("all") }
    var selectedFolder by remember { mutableStateOf("") }
    var sortBy by remember { mutableStateOf("newest") }
    var showSortMenu by remember { mutableStateOf(false) }
    var groupByFolder by remember { mutableStateOf(true) }

    var pendingDeleteServer by remember { mutableStateOf<GalleryItem?>(null) }
    var pendingDeletePhone by remember { mutableStateOf<SavedPhoneFile?>(null) }

    fun loadServerGallery() {
        isLoading = true
        scope.launch {
            try {
                val res = RetrofitClient.getApi().getGallery(sort = sortBy)
                serverItems = res.items
                serverFolders = res.folders
            } catch (e: Exception) {
                onShowMessage(e.message ?: "Failed to load server gallery")
            } finally {
                isLoading = false
            }
        }
    }

    LaunchedEffect(sortBy) {
        loadServerGallery()
    }

    val filteredServerItems = remember(serverItems, categoryFilter, selectedFolder, searchQuery, sortBy) {
        val q = searchQuery.trim().lowercase()
        val list = serverItems.filter { item ->
            val matchCat = categoryFilter == "all" || item.type == categoryFilter
            val matchFolder = selectedFolder.isBlank() ||
                item.channel.equals(selectedFolder, ignoreCase = true) ||
                item.group.equals(selectedFolder, ignoreCase = true) ||
                item.path.contains("/$selectedFolder/", ignoreCase = true)
            val matchSearch = q.isBlank() ||
                item.displayTitle().lowercase().contains(q) ||
                (item.channel ?: "").lowercase().contains(q) ||
                (item.group ?: "").lowercase().contains(q) ||
                (item.platform ?: "").lowercase().contains(q)
            matchCat && matchFolder && matchSearch
        }

        when (sortBy) {
            "oldest" -> list.sortedBy { it.modTime }
            "size_desc" -> list.sortedByDescending { it.sizeInBytes }
            "size_asc" -> list.sortedBy { it.sizeInBytes }
            "name_asc" -> list.sortedBy { it.displayTitle().lowercase() }
            "name_desc" -> list.sortedByDescending { it.displayTitle().lowercase() }
            "channel_asc" -> list.sortedBy { (it.channel ?: "zzz").lowercase() }
            else -> list.sortedByDescending { it.modTime }
        }
    }

    val filteredPhoneFiles = remember(savedFiles, categoryFilter, searchQuery, sortBy) {
        val q = searchQuery.trim().lowercase()
        val list = savedFiles.filter { f ->
            val matchCat = categoryFilter == "all" || f.category == categoryFilter
            val matchSearch = q.isBlank() ||
                f.title.lowercase().contains(q) ||
                f.fileName.lowercase().contains(q) ||
                (f.channel ?: "").lowercase().contains(q) ||
                f.relativePath.lowercase().contains(q)
            matchCat && matchSearch
        }

        when (sortBy) {
            "oldest" -> list.sortedBy { it.savedAt }
            "size_desc" -> list.sortedByDescending { it.sizeBytes }
            "size_asc" -> list.sortedBy { it.sizeBytes }
            "name_asc" -> list.sortedBy { it.title.lowercase() }
            "name_desc" -> list.sortedByDescending { it.title.lowercase() }
            "channel_asc" -> list.sortedBy { (it.channel ?: "zzz").lowercase() }
            else -> list.sortedByDescending { it.savedAt }
        }
    }

    LazyColumn(
        modifier = Modifier.fillMaxSize(),
        contentPadding = PaddingValues(14.dp),
        verticalArrangement = Arrangement.spacedBy(10.dp)
    ) {
        // Source Switcher & Toolbar
        item {
            Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
                SingleChoiceSegmentedButtonRow(modifier = Modifier.fillMaxWidth()) {
                    SegmentedButton(
                        selected = sourceTab == "server",
                        onClick = { sourceTab = "server" },
                        shape = SegmentedButtonDefaults.itemShape(index = 0, count = 2),
                        icon = { Icon(Icons.Default.Folder, contentDescription = null, modifier = Modifier.size(16.dp)) }
                    ) { Text("Server (${serverItems.size})") }
                    SegmentedButton(
                        selected = sourceTab == "phone",
                        onClick = { sourceTab = "phone" },
                        shape = SegmentedButtonDefaults.itemShape(index = 1, count = 2),
                        icon = { Icon(Icons.Default.PhoneAndroid, contentDescription = null, modifier = Modifier.size(16.dp)) }
                    ) { Text("On Phone (${savedFiles.size})") }
                }

                OutlinedTextField(
                    value = searchQuery,
                    onValueChange = { searchQuery = it },
                    modifier = Modifier.fillMaxWidth(),
                    singleLine = true,
                    leadingIcon = { Icon(Icons.Default.Search, contentDescription = null) },
                    trailingIcon = {
                        if (searchQuery.isNotEmpty()) {
                            IconButton(onClick = { searchQuery = "" }) {
                                Icon(Icons.Default.Close, contentDescription = "Clear")
                            }
                        }
                    },
                    placeholder = { Text("Search title, channel, or folder...") }
                )

                Row(
                    modifier = Modifier
                        .fillMaxWidth()
                        .horizontalScroll(rememberScrollState()),
                    horizontalArrangement = Arrangement.spacedBy(6.dp)
                ) {
                    listOf(
                        "all" to "All",
                        "video" to "Videos",
                        "audio" to "Audio",
                        "photo" to "Photos"
                    ).forEach { (key, label) ->
                        FilterChip(
                            selected = categoryFilter == key,
                            onClick = { categoryFilter = key },
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
                    Box {
                        AssistChip(
                            onClick = { showSortMenu = true },
                            label = { Text("Sort: ${sortLabel(sortBy)}") }
                        )
                        DropdownMenu(expanded = showSortMenu, onDismissRequest = { showSortMenu = false }) {
                            listOf(
                                "newest",
                                "oldest",
                                "size_desc",
                                "size_asc",
                                "name_asc",
                                "name_desc",
                                "channel_asc"
                            ).forEach { mode ->
                                DropdownMenuItem(
                                    text = { Text(sortLabel(mode)) },
                                    onClick = {
                                        sortBy = mode
                                        showSortMenu = false
                                    }
                                )
                            }
                        }
                    }

                    FilterChip(
                        selected = groupByFolder,
                        onClick = { groupByFolder = !groupByFolder },
                        leadingIcon = {
                            Icon(
                                if (groupByFolder) Icons.Default.Layers else Icons.Default.List,
                                contentDescription = null,
                                modifier = Modifier.size(16.dp)
                            )
                        },
                        label = { Text(if (groupByFolder) "By Folder" else "Flat") }
                    )

                    if (sourceTab == "server") {
                        AssistChip(
                            onClick = {
                                scope.launch {
                                    try {
                                        val res = RetrofitClient.getApi().organizeLibrary()
                                        onShowMessage("Organized (${res.movedFiles} files moved, ${res.organizeMode})")
                                        loadServerGallery()
                                    } catch (e: Exception) {
                                        onShowMessage(e.message ?: "Organize failed")
                                    }
                                }
                            },
                            leadingIcon = { Icon(Icons.Default.DriveFileMove, contentDescription = null, modifier = Modifier.size(16.dp)) },
                            label = { Text("Auto-Organize") }
                        )

                        if (selectedFolder.isNotBlank()) {
                            AssistChip(
                                onClick = {
                                    phoneDownloadManager.saveZipArchiveToPhone(
                                        zipUrl = RetrofitClient.folderZipUrl(selectedFolder),
                                        archiveLabel = selectedFolder,
                                        channelOrGroup = selectedFolder
                                    )
                                    onShowMessage("Saving ZIP for \"$selectedFolder\" to Downloads/KV Download")
                                },
                                leadingIcon = { Icon(Icons.Default.FolderZip, contentDescription = null, modifier = Modifier.size(16.dp)) },
                                label = { Text("Save Folder ZIP") }
                            )
                        }
                    }
                }

                if (sourceTab == "server" && serverFolders.isNotEmpty()) {
                    Row(
                        modifier = Modifier
                            .fillMaxWidth()
                            .horizontalScroll(rememberScrollState()),
                        horizontalArrangement = Arrangement.spacedBy(6.dp)
                    ) {
                        FilterChip(
                            selected = selectedFolder.isBlank(),
                            onClick = { selectedFolder = "" },
                            label = { Text("All Folders") }
                        )
                        serverFolders.forEach { folder ->
                            FilterChip(
                                selected = selectedFolder == folder.name,
                                onClick = {
                                    selectedFolder = if (selectedFolder == folder.name) "" else folder.name
                                },
                                label = { Text("${folder.name} (${folder.fileCount})") }
                            )
                        }
                    }
                }
            }
        }

        if (sourceTab == "server") {
            if (isLoading) {
                item {
                    Box(
                        modifier = Modifier
                            .fillMaxWidth()
                            .padding(32.dp),
                        contentAlignment = Alignment.Center
                    ) {
                        CircularProgressIndicator()
                    }
                }
            } else if (filteredServerItems.isEmpty()) {
                item {
                    EmptyGalleryCard("No server media found", "Completed server downloads appear here.")
                }
            } else if (groupByFolder && selectedFolder.isBlank()) {
                val sections = filteredServerItems.groupBy { it.channel ?: it.group ?: "General" }
                    .toSortedMap(String.CASE_INSENSITIVE_ORDER)
                sections.forEach { (folderTitle, sectionItems) ->
                    item(key = "hdr:$folderTitle") {
                        FolderSectionHeader(
                            title = folderTitle,
                            count = sectionItems.size,
                            onSaveZip = if (folderTitle != "General") {
                                {
                                    phoneDownloadManager.saveZipArchiveToPhone(
                                        zipUrl = RetrofitClient.folderZipUrl(folderTitle),
                                        archiveLabel = folderTitle,
                                        channelOrGroup = folderTitle
                                    )
                                    onShowMessage("Saving ZIP of \"$folderTitle\" to phone storage")
                                }
                            } else null
                        )
                    }
                    items(sectionItems, key = { it.path }) { item ->
                        ServerGalleryItemCard(
                            item = item,
                            savedOnPhone = phoneDownloadManager.findSavedForGallery(item.path),
                            transfer = phoneTransfers["gallery:${item.path}"],
                            onPlay = {
                                onPlayMedia(
                                    PlaybackRequest(
                                        title = item.displayTitle(),
                                        subtitle = item.channel ?: item.platform,
                                        streamUrl = RetrofitClient.downloadGalleryUrl(item.path, inline = true),
                                        onSaveToPhone = {
                                            phoneDownloadManager.saveGalleryItemToPhone(item)
                                            onShowMessage("Saving to phone storage...")
                                        }
                                    )
                                )
                            },
                            onSaveToPhone = {
                                phoneDownloadManager.saveGalleryItemToPhone(item)
                                onShowMessage("Downloading \"${item.displayTitle()}\" to phone storage")
                            },
                            onOpenSaved = { saved -> phoneDownloadManager.openOnDevice(saved) },
                            onDelete = { pendingDeleteServer = item }
                        )
                    }
                }
            } else {
                items(filteredServerItems, key = { it.path }) { item ->
                    ServerGalleryItemCard(
                        item = item,
                        savedOnPhone = phoneDownloadManager.findSavedForGallery(item.path),
                        transfer = phoneTransfers["gallery:${item.path}"],
                        onPlay = {
                            onPlayMedia(
                                PlaybackRequest(
                                    title = item.displayTitle(),
                                    subtitle = item.channel ?: item.platform,
                                    streamUrl = RetrofitClient.downloadGalleryUrl(item.path, inline = true),
                                    onSaveToPhone = {
                                        phoneDownloadManager.saveGalleryItemToPhone(item)
                                        onShowMessage("Saving to phone storage...")
                                    }
                                )
                            )
                        },
                        onSaveToPhone = {
                            phoneDownloadManager.saveGalleryItemToPhone(item)
                            onShowMessage("Downloading \"${item.displayTitle()}\" to phone storage")
                        },
                        onOpenSaved = { saved -> phoneDownloadManager.openOnDevice(saved) },
                        onDelete = { pendingDeleteServer = item }
                    )
                }
            }
        } else {
            // On This Phone Storage Tab
            if (filteredPhoneFiles.isEmpty()) {
                item {
                    EmptyGalleryCard(
                        "No files saved on this phone yet",
                        "Tap 'Save to Phone' on any completed task, gallery file, or browser stream to save directly into Movies / Music / Pictures / Downloads."
                    )
                }
            } else if (groupByFolder) {
                val sections = filteredPhoneFiles.groupBy { it.channel?.ifBlank { null } ?: it.group?.ifBlank { null } ?: "General" }
                    .toSortedMap(String.CASE_INSENSITIVE_ORDER)
                sections.forEach { (folderTitle, sectionFiles) ->
                    item(key = "ph_hdr:$folderTitle") {
                        FolderSectionHeader(title = folderTitle, count = sectionFiles.size, onSaveZip = null)
                    }
                    items(sectionFiles, key = { it.id }) { file ->
                        PhoneSavedItemCard(
                            file = file,
                            onPlay = {
                                onPlayMedia(
                                    PlaybackRequest(
                                        title = file.title,
                                        subtitle = file.relativePath,
                                        streamUrl = file.contentUri,
                                        isLocalUri = true
                                    )
                                )
                            },
                            onOpen = { phoneDownloadManager.openOnDevice(file) },
                            onShare = { phoneDownloadManager.shareFile(file) },
                            onDelete = { pendingDeletePhone = file }
                        )
                    }
                }
            } else {
                items(filteredPhoneFiles, key = { it.id }) { file ->
                    PhoneSavedItemCard(
                        file = file,
                        onPlay = {
                            onPlayMedia(
                                PlaybackRequest(
                                    title = file.title,
                                    subtitle = file.relativePath,
                                    streamUrl = file.contentUri,
                                    isLocalUri = true
                                )
                            )
                        },
                        onOpen = { phoneDownloadManager.openOnDevice(file) },
                        onShare = { phoneDownloadManager.shareFile(file) },
                        onDelete = { pendingDeletePhone = file }
                    )
                }
            }
        }
    }

    pendingDeleteServer?.let { item ->
        AlertDialog(
            onDismissRequest = { pendingDeleteServer = null },
            title = { Text("Delete File from Server?") },
            text = { Text("Permanently delete \"${item.displayTitle()}\" (${item.humanSize}) from server disk?") },
            confirmButton = {
                TextButton(
                    onClick = {
                        val path = item.path
                        pendingDeleteServer = null
                        scope.launch {
                            try {
                                RetrofitClient.getApi().deleteGalleryFile(path)
                                loadServerGallery()
                                onShowMessage("Deleted file from server")
                            } catch (e: Exception) {
                                onShowMessage(e.message ?: "Delete failed")
                            }
                        }
                    }
                ) { Text("Delete", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteServer = null }) { Text("Cancel") }
            }
        )
    }

    pendingDeletePhone?.let { file ->
        AlertDialog(
            onDismissRequest = { pendingDeletePhone = null },
            title = { Text("Remove from Phone Storage?") },
            text = { Text("Delete \"${file.fileName}\" at ${file.relativePath}?") },
            confirmButton = {
                TextButton(
                    onClick = {
                        val target = file
                        pendingDeletePhone = null
                        phoneDownloadManager.deleteSavedPhoneFile(target)
                        onShowMessage("Deleted file from phone storage")
                    }
                ) { Text("Delete", color = MaterialTheme.colorScheme.error) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeletePhone = null }) { Text("Cancel") }
            }
        )
    }
}

@Composable
private fun FolderSectionHeader(
    title: String,
    count: Int,
    onSaveZip: (() -> Unit)?
) {
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp)
            ) {
                Icon(Icons.Default.Folder, contentDescription = null, tint = MaterialTheme.colorScheme.primary, modifier = Modifier.size(18.dp))
                Text(title, style = MaterialTheme.typography.titleSmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                Text("($count)", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
            if (onSaveZip != null) {
                TextButton(onClick = onSaveZip) {
                    Icon(Icons.Default.FolderZip, contentDescription = null, modifier = Modifier.size(16.dp))
                    Text("ZIP", modifier = Modifier.padding(start = 4.dp))
                }
            }
        }
        HorizontalDivider(color = MaterialTheme.colorScheme.outlineVariant)
    }
}

@Composable
private fun ServerGalleryItemCard(
    item: GalleryItem,
    savedOnPhone: SavedPhoneFile?,
    transfer: PhoneTransferState?,
    onPlay: () -> Unit,
    onSaveToPhone: () -> Unit,
    onOpenSaved: (SavedPhoneFile) -> Unit,
    onDelete: () -> Unit
) {
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer),
        shape = RoundedCornerShape(12.dp)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(10.dp),
            verticalArrangement = Arrangement.spacedBy(8.dp)
        ) {
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.spacedBy(10.dp),
                verticalAlignment = Alignment.CenterVertically
            ) {
                AsyncImage(
                    model = RetrofitClient.galleryThumbnailUrl(item),
                    contentDescription = null,
                    modifier = Modifier
                        .size(84.dp, 54.dp)
                        .clip(RoundedCornerShape(8.dp))
                        .background(MaterialTheme.colorScheme.surfaceContainerHigh),
                    contentScale = ContentScale.Crop
                )

                Column(modifier = Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    Text(
                        text = item.displayTitle(),
                        style = MaterialTheme.typography.bodyMedium,
                        fontWeight = FontWeight.SemiBold,
                        maxLines = 2,
                        overflow = TextOverflow.Ellipsis
                    )
                    Text(
                        text = listOfNotNull(item.channel, item.platform, item.humanSize, item.duration)
                            .filter { it.isNotBlank() }
                            .joinToString(" · "),
                        style = MaterialTheme.typography.labelSmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                        maxLines = 1,
                        overflow = TextOverflow.Ellipsis
                    )
                }
            }

            if (transfer != null && !transfer.isCompleted && transfer.error == null) {
                LinearProgressIndicator(progress = { transfer.progress }, modifier = Modifier.fillMaxWidth())
            }

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    Button(
                        onClick = onPlay,
                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                    ) {
                        Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(16.dp))
                        Text("Play", modifier = Modifier.padding(start = 4.dp))
                    }

                    if (savedOnPhone != null) {
                        OutlinedButton(
                            onClick = { onOpenSaved(savedOnPhone) },
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                        ) {
                            Icon(Icons.Default.CheckCircle, contentDescription = null, modifier = Modifier.size(15.dp), tint = MaterialTheme.colorScheme.tertiary)
                            Text("On Phone", modifier = Modifier.padding(start = 4.dp))
                        }
                    } else {
                        OutlinedButton(
                            onClick = onSaveToPhone,
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                        ) {
                            Icon(Icons.Default.SaveAlt, contentDescription = null, modifier = Modifier.size(16.dp))
                            Text("Save to Phone", modifier = Modifier.padding(start = 4.dp))
                        }
                    }
                }

                IconButton(onClick = onDelete) {
                    Icon(Icons.Default.DeleteOutline, contentDescription = "Delete", tint = MaterialTheme.colorScheme.error)
                }
            }
        }
    }
}

@Composable
private fun PhoneSavedItemCard(
    file: SavedPhoneFile,
    onPlay: () -> Unit,
    onOpen: () -> Unit,
    onShare: () -> Unit,
    onDelete: () -> Unit
) {
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer),
        shape = RoundedCornerShape(12.dp)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(12.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(file.title, style = MaterialTheme.typography.titleSmall, maxLines = 2, overflow = TextOverflow.Ellipsis)
            Text(
                text = "${file.relativePath} · ${PhoneDownloadManager.formatFileSize(file.sizeBytes)}",
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 2,
                overflow = TextOverflow.Ellipsis
            )

            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween,
                verticalAlignment = Alignment.CenterVertically
            ) {
                Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                    if (file.category == "video" || file.category == "audio") {
                        Button(
                            onClick = onPlay,
                            contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                        ) {
                            Icon(Icons.Default.PlayArrow, contentDescription = null, modifier = Modifier.size(16.dp))
                            Text("Play", modifier = Modifier.padding(start = 4.dp))
                        }
                    }
                    OutlinedButton(
                        onClick = onOpen,
                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                    ) {
                        Icon(Icons.Default.OpenInNew, contentDescription = null, modifier = Modifier.size(16.dp))
                        Text("Open", modifier = Modifier.padding(start = 4.dp))
                    }
                    IconButton(onClick = onShare) {
                        Icon(Icons.Default.Share, contentDescription = "Share")
                    }
                }
                IconButton(onClick = onDelete) {
                    Icon(Icons.Default.DeleteOutline, contentDescription = "Delete", tint = MaterialTheme.colorScheme.error)
                }
            }
        }
    }
}

@Composable
private fun EmptyGalleryCard(title: String, body: String) {
    Card(
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.surfaceContainer),
        shape = RoundedCornerShape(14.dp)
    ) {
        Column(
            modifier = Modifier
                .fillMaxWidth()
                .padding(24.dp),
            verticalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            Text(title, style = MaterialTheme.typography.titleMedium)
            Text(body, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

private fun sortLabel(mode: String): String = when (mode) {
    "oldest" -> "Oldest"
    "size_desc" -> "Largest"
    "size_asc" -> "Smallest"
    "name_asc" -> "Title A-Z"
    "name_desc" -> "Title Z-A"
    "channel_asc" -> "Channel"
    else -> "Newest"
}
