package com.kvdownload.app.ui.screens

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.horizontalScroll
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
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
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Close
import androidx.compose.material.icons.filled.DeleteOutline
import androidx.compose.material.icons.filled.DriveFileMove
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.FolderZip
import androidx.compose.material.icons.filled.Image
import androidx.compose.material.icons.filled.Layers
import androidx.compose.material.icons.filled.List
import androidx.compose.material.icons.filled.PhoneAndroid
import androidx.compose.material.icons.filled.PlayArrow
import androidx.compose.material.icons.filled.SaveAlt
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.AssistChip
import androidx.compose.material3.Button
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
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.VerticalDivider
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
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.kvdownload.app.api.GalleryFolder
import com.kvdownload.app.api.GalleryItem
import com.kvdownload.app.api.PhoneTransferState
import com.kvdownload.app.api.RetrofitClient
import com.kvdownload.app.api.SavedPhoneFile
import com.kvdownload.app.player.MediaKind
import com.kvdownload.app.player.MediaPlayRequest
import com.kvdownload.app.storage.PhoneDownloadManager
import com.kvdownload.app.ui.adaptive.LocalWindowState
import com.kvdownload.app.ui.components.AstryxCard
import com.kvdownload.app.ui.components.AstryxMediaThumbnail
import com.kvdownload.app.ui.components.AstryxStatusBadge
import com.kvdownload.app.ui.components.AstryxTone
import com.kvdownload.app.ui.theme.LocalAstryxPalette
import kotlinx.coroutines.launch

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun MediaGalleryScreen(
    phoneDownloadManager: PhoneDownloadManager,
    onPlayMedia: (MediaPlayRequest) -> Unit,
    onShowMessage: (String) -> Unit
) {
    val scope = rememberCoroutineScope()
    val windowState = LocalWindowState.current
    val palette = LocalAstryxPalette.current
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
    var pendingDeleteFolder by remember { mutableStateOf<Pair<String, List<SavedPhoneFile>>?>(null) }
    var pendingDeleteAllPhone by remember { mutableStateOf(false) }

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
            val matchCat = when (categoryFilter) {
                "all" -> true
                "photo" -> f.category == "photo" || f.category == "image"
                else -> f.category == categoryFilter
            }
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

    fun playServerItem(item: GalleryItem) {
        val saved = phoneDownloadManager.findSavedForGallery(item.path)
        val kind = when (item.type) {
            "photo", "image" -> MediaKind.IMAGE
            "audio" -> MediaKind.AUDIO
            else -> MediaKind.VIDEO
        }
        val serverInline = RetrofitClient.downloadGalleryUrl(item.path, inline = true)
        onPlayMedia(
            MediaPlayRequest(
                id = "gallery:${item.path}",
                title = item.displayTitle(),
                subtitle = item.channel ?: item.platform ?: item.humanSize,
                thumbnailUrl = RetrofitClient.galleryThumbnailUrl(item),
                primaryUrl = saved?.contentUri ?: serverInline,
                fallbackUrl = "$serverInline&compat=1",
                kind = kind,
                isLocalUri = saved != null,
                onSaveToPhone = {
                    phoneDownloadManager.saveGalleryItemToPhone(item)
                    onShowMessage("Saving to phone storage...")
                }
            )
        )
    }

    fun playPhoneFile(file: SavedPhoneFile) {
        val kind = when {
            file.category == "photo" || file.category == "image" || file.mimeType.startsWith("image/") -> MediaKind.IMAGE
            file.category == "audio" || file.mimeType.startsWith("audio/") -> MediaKind.AUDIO
            else -> MediaKind.VIDEO
        }
        onPlayMedia(
            MediaPlayRequest(
                id = "phone:${file.id}",
                title = file.title,
                subtitle = file.relativePath,
                thumbnailUrl = file.thumbnailUri ?: if (kind == MediaKind.IMAGE) file.contentUri else null,
                primaryUrl = file.contentUri,
                kind = kind,
                isLocalUri = true
            )
        )
    }

    val showLeftFolderSidebar = windowState.isTwoPane && sourceTab == "server" && serverFolders.isNotEmpty()

    Row(modifier = Modifier.fillMaxSize()) {
        if (showLeftFolderSidebar) {
            LazyColumn(
                modifier = Modifier
                    .width(230.dp)
                    .fillMaxHeight()
                    .background(palette.surface),
                contentPadding = PaddingValues(12.dp),
                verticalArrangement = Arrangement.spacedBy(4.dp)
            ) {
                item {
                    Text(
                        text = "Folders (${serverFolders.size})",
                        style = MaterialTheme.typography.titleSmall,
                        color = palette.textPrimary,
                        modifier = Modifier.padding(horizontal = 8.dp, vertical = 6.dp)
                    )
                }
                item {
                    FolderSidebarRow(
                        label = "All Media",
                        count = serverItems.size,
                        selected = selectedFolder.isBlank(),
                        onClick = { selectedFolder = "" }
                    )
                }
                items(serverFolders, key = { it.name }) { folder ->
                    FolderSidebarRow(
                        label = folder.name,
                        count = folder.fileCount,
                        selected = selectedFolder == folder.name,
                        onClick = {
                            selectedFolder = if (selectedFolder == folder.name) "" else folder.name
                        }
                    )
                }
            }
            VerticalDivider(color = palette.borderSubtle)
        }

        LazyVerticalGrid(
            columns = GridCells.Fixed(windowState.galleryColumns.coerceAtLeast(1)),
            modifier = Modifier
                .weight(1f)
                .fillMaxHeight(),
            contentPadding = PaddingValues(14.dp),
            horizontalArrangement = Arrangement.spacedBy(12.dp),
            verticalArrangement = Arrangement.spacedBy(12.dp)
        ) {
            // Source Switcher & Filters
            item(span = { GridItemSpan(maxLineSpan) }) {
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

                        if (sourceTab == "phone" && savedFiles.isNotEmpty()) {
                            AssistChip(
                                onClick = { pendingDeleteAllPhone = true },
                                leadingIcon = { Icon(Icons.Default.DeleteOutline, contentDescription = null, tint = palette.error, modifier = Modifier.size(16.dp)) },
                                label = { Text("Delete All", color = palette.error) }
                            )
                        }

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

                    if (!showLeftFolderSidebar && sourceTab == "server" && serverFolders.isNotEmpty()) {
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
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        Box(
                            modifier = Modifier
                                .fillMaxWidth()
                                .padding(32.dp),
                            contentAlignment = Alignment.Center
                        ) {
                            CircularProgressIndicator(color = palette.accent)
                        }
                    }
                } else if (filteredServerItems.isEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        EmptyGalleryCard("No server media found", "Completed server downloads appear here.")
                    }
                } else if (groupByFolder && selectedFolder.isBlank()) {
                    val sections = filteredServerItems.groupBy { it.channel ?: it.group ?: "General" }
                        .toSortedMap(String.CASE_INSENSITIVE_ORDER)
                    sections.forEach { (folderTitle, sectionItems) ->
                        item(key = "hdr:$folderTitle", span = { GridItemSpan(maxLineSpan) }) {
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
                                onPlay = { playServerItem(item) },
                                onSaveToPhone = {
                                    phoneDownloadManager.saveGalleryItemToPhone(item)
                                    onShowMessage("Saving \"${item.displayTitle()}\" to Private App Vault")
                                },
                                onOpenSaved = { saved -> playPhoneFile(saved) },
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
                            onPlay = { playServerItem(item) },
                            onSaveToPhone = {
                                phoneDownloadManager.saveGalleryItemToPhone(item)
                                onShowMessage("Saving \"${item.displayTitle()}\" to Private App Vault")
                            },
                            onOpenSaved = { saved -> playPhoneFile(saved) },
                            onDelete = { pendingDeleteServer = item }
                        )
                    }
                }
            } else {
                // Active transfers banner on Phone tab so M3U8 segment downloads are visible right here
                val activePhone = phoneTransfers.values.filter { !it.isCompleted && it.error == null }
                if (activePhone.isNotEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        AstryxCard(emphasized = true) {
                            Text("Saving to Private Vault (${activePhone.size})", style = MaterialTheme.typography.titleSmall, color = palette.textPrimary)
                            activePhone.forEach { tr ->
                                Text(tr.title, style = MaterialTheme.typography.bodySmall, maxLines = 1, overflow = TextOverflow.Ellipsis)
                                LinearProgressIndicator(progress = { tr.progress }, modifier = Modifier.fillMaxWidth(), color = palette.success)
                                Text(tr.statusText ?: "${(tr.progress * 100).toInt()}%", style = MaterialTheme.typography.labelSmall, color = palette.success)
                            }
                        }
                    }
                }

                item(span = { GridItemSpan(maxLineSpan) }) {
                    Text(
                        text = "Private vault: ${phoneDownloadManager.vaultLocationHint()}\nAndroid hides this app-only folder from file managers and other apps — browse, play and delete it here.",
                        style = MaterialTheme.typography.labelSmall,
                        color = palette.textSecondary
                    )
                }

                if (filteredPhoneFiles.isEmpty()) {
                    item(span = { GridItemSpan(maxLineSpan) }) {
                        EmptyGalleryCard(
                            "No private files on this phone yet",
                            "Tap 'To Phone' or pick Phone (Private Vault) in the Browser — files are stored strictly inside KV Download and hidden from Gallery and other apps."
                        )
                    }
                } else if (groupByFolder) {
                    val sections = filteredPhoneFiles.groupBy { phoneDownloadManager.vaultFolderOf(it) }
                        .toSortedMap(String.CASE_INSENSITIVE_ORDER)
                    sections.forEach { (folderTitle, sectionFiles) ->
                        item(key = "ph_hdr:$folderTitle", span = { GridItemSpan(maxLineSpan) }) {
                            FolderSectionHeader(
                                title = folderTitle,
                                count = sectionFiles.size,
                                onSaveZip = null,
                                onDeleteFolder = { pendingDeleteFolder = folderTitle to sectionFiles }
                            )
                        }
                        items(sectionFiles, key = { it.id }) { file ->
                            PhoneSavedItemCard(
                                file = file,
                                onPlay = { playPhoneFile(file) },
                                onDelete = { pendingDeletePhone = file }
                            )
                        }
                    }
                } else {
                    items(filteredPhoneFiles, key = { it.id }) { file ->
                        PhoneSavedItemCard(
                            file = file,
                            onPlay = { playPhoneFile(file) },
                            onDelete = { pendingDeletePhone = file }
                        )
                    }
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
                ) { Text("Delete", color = palette.error) }
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
                ) { Text("Delete", color = palette.error) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeletePhone = null }) { Text("Cancel") }
            }
        )
    }
    pendingDeleteFolder?.let { (folderName, files) ->
        AlertDialog(
            onDismissRequest = { pendingDeleteFolder = null },
            title = { Text("Delete folder \"$folderName\"?") },
            text = { Text("Permanently delete ${files.size} file(s) in this folder from the private vault? This cannot be undone.") },
            confirmButton = {
                TextButton(
                    onClick = {
                        pendingDeleteFolder = null
                        phoneDownloadManager.deleteSavedPhoneFiles(files)
                        onShowMessage("Deleted folder \"$folderName\" (${files.size} files)")
                    }
                ) { Text("Delete folder", color = palette.error) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteFolder = null }) { Text("Cancel") }
            }
        )
    }

    if (pendingDeleteAllPhone) {
        AlertDialog(
            onDismissRequest = { pendingDeleteAllPhone = false },
            title = { Text("Delete all private files?") },
            text = { Text("Permanently delete all ${savedFiles.size} file(s) stored in KV Download's private vault on this phone? Server copies are not affected.") },
            confirmButton = {
                TextButton(
                    onClick = {
                        pendingDeleteAllPhone = false
                        val all = savedFiles
                        phoneDownloadManager.deleteSavedPhoneFiles(all)
                        onShowMessage("Deleted ${all.size} private file(s)")
                    }
                ) { Text("Delete all", color = palette.error) }
            },
            dismissButton = {
                TextButton(onClick = { pendingDeleteAllPhone = false }) { Text("Cancel") }
            }
        )
    }
}

@Composable
private fun FolderSidebarRow(
    label: String,
    count: Int,
    selected: Boolean,
    onClick: () -> Unit
) {
    val palette = LocalAstryxPalette.current
    Surface(
        modifier = Modifier
            .fillMaxWidth()
            .clickable(onClick = onClick),
        shape = RoundedCornerShape(10.dp),
        color = if (selected) palette.accentContainer else Color.Transparent
    ) {
        Row(
            modifier = Modifier.padding(horizontal = 10.dp, vertical = 9.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.SpaceBetween
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                modifier = Modifier.weight(1f)
            ) {
                Icon(
                    imageVector = Icons.Default.Folder,
                    contentDescription = null,
                    tint = if (selected) palette.accentText else palette.textSecondary,
                    modifier = Modifier.size(16.dp)
                )
                Text(
                    text = label,
                    style = MaterialTheme.typography.bodySmall,
                    fontWeight = if (selected) FontWeight.SemiBold else FontWeight.Normal,
                    color = if (selected) palette.accentText else palette.textPrimary,
                    maxLines = 1,
                    overflow = TextOverflow.Ellipsis
                )
            }
            Text(
                text = "$count",
                style = MaterialTheme.typography.labelSmall,
                color = if (selected) palette.accentText else palette.textSecondary
            )
        }
    }
}

@Composable
private fun FolderSectionHeader(
    title: String,
    count: Int,
    onSaveZip: (() -> Unit)?,
    onDeleteFolder: (() -> Unit)? = null
) {
    val palette = LocalAstryxPalette.current
    Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Row(
                verticalAlignment = Alignment.CenterVertically,
                horizontalArrangement = Arrangement.spacedBy(6.dp),
                modifier = Modifier.weight(1f)
            ) {
                Icon(Icons.Default.Folder, contentDescription = null, tint = palette.accent, modifier = Modifier.size(18.dp))
                Text(title, style = MaterialTheme.typography.titleSmall, color = palette.textPrimary, maxLines = 1, overflow = TextOverflow.Ellipsis, modifier = Modifier.weight(1f, fill = false))
                Text("($count)", style = MaterialTheme.typography.bodySmall, color = palette.textSecondary)
            }
            if (onSaveZip != null) {
                TextButton(onClick = onSaveZip) {
                    Icon(Icons.Default.FolderZip, contentDescription = null, modifier = Modifier.size(16.dp))
                    Text("ZIP", modifier = Modifier.padding(start = 4.dp))
                }
            }
            if (onDeleteFolder != null) {
                TextButton(onClick = onDeleteFolder) {
                    Icon(Icons.Default.DeleteOutline, contentDescription = null, tint = palette.error, modifier = Modifier.size(16.dp))
                    Text("Delete folder", color = palette.error, modifier = Modifier.padding(start = 4.dp))
                }
            }
        }
        HorizontalDivider(color = palette.borderSubtle)
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
    val palette = LocalAstryxPalette.current
    val isPhoto = item.type == "photo" || item.type == "image"

    AstryxCard(contentPadding = PaddingValues(10.dp)) {
        AstryxMediaThumbnail(
            imageUrl = RetrofitClient.galleryThumbnailUrl(item),
            mediaType = item.type,
            badgeText = item.channel ?: item.platform,
            durationText = item.duration ?: item.humanSize,
            showPlayOverlay = true,
            onClick = onPlay
        )

        Text(
            text = item.displayTitle(),
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
            AstryxStatusBadge(item.type.uppercase(), AstryxTone.Accent)
            if (item.humanSize.isNotBlank()) AstryxStatusBadge(item.humanSize, AstryxTone.Neutral)
            item.platform?.takeIf { it.isNotBlank() }?.let { AstryxStatusBadge(it, AstryxTone.Info) }
        }

        if (transfer != null && !transfer.isCompleted && transfer.error == null) {
            LinearProgressIndicator(progress = { transfer.progress }, modifier = Modifier.fillMaxWidth(), color = palette.success)
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                Button(
                    onClick = onPlay,
                    contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp)
                ) {
                    Icon(
                        imageVector = if (isPhoto) Icons.Default.Image else Icons.Default.PlayArrow,
                        contentDescription = null,
                        modifier = Modifier.size(16.dp)
                    )
                    Text(if (isPhoto) "View" else "Play", modifier = Modifier.padding(start = 4.dp))
                }

                if (savedOnPhone != null) {
                    OutlinedButton(
                        onClick = { onOpenSaved(savedOnPhone) },
                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                    ) {
                        Icon(Icons.Default.CheckCircle, contentDescription = null, modifier = Modifier.size(15.dp), tint = palette.success)
                        Text("On Phone", modifier = Modifier.padding(start = 4.dp))
                    }
                } else {
                    OutlinedButton(
                        onClick = onSaveToPhone,
                        contentPadding = PaddingValues(horizontal = 10.dp, vertical = 4.dp)
                    ) {
                        Icon(Icons.Default.SaveAlt, contentDescription = null, modifier = Modifier.size(16.dp))
                        Text("To Phone", modifier = Modifier.padding(start = 4.dp))
                    }
                }
            }

            IconButton(onClick = onDelete) {
                Icon(Icons.Default.DeleteOutline, contentDescription = "Delete", tint = palette.error)
            }
        }
    }
}

@Composable
private fun PhoneSavedItemCard(
    file: SavedPhoneFile,
    onPlay: () -> Unit,
    onDelete: () -> Unit
) {
    val palette = LocalAstryxPalette.current
    val isPhoto = file.category == "photo" || file.category == "image" || file.mimeType.startsWith("image/")
    val isPlayable = file.category == "video" || file.category == "audio" || isPhoto

    AstryxCard(contentPadding = PaddingValues(10.dp)) {
        if (isPlayable) {
            AstryxMediaThumbnail(
                imageUrl = file.thumbnailUri ?: if (isPhoto) file.contentUri else null,
                mediaType = file.category,
                badgeText = file.channel ?: "PRIVATE",
                durationText = PhoneDownloadManager.formatFileSize(file.sizeBytes),
                showPlayOverlay = true,
                onClick = onPlay
            )
        }

        Text(
            text = file.title,
            style = MaterialTheme.typography.titleSmall,
            color = palette.textPrimary,
            maxLines = 2,
            overflow = TextOverflow.Ellipsis
        )
        Row(
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(6.dp)
        ) {
            AstryxStatusBadge("PRIVATE", AstryxTone.Accent)
            Text(
                text = "${file.relativePath} · ${PhoneDownloadManager.formatFileSize(file.sizeBytes)}",
                style = MaterialTheme.typography.labelSmall,
                color = palette.textSecondary,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
                modifier = Modifier.weight(1f)
            )
        }

        Row(
            modifier = Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically
        ) {
            Row(horizontalArrangement = Arrangement.spacedBy(6.dp)) {
                if (isPlayable) {
                    Button(
                        onClick = onPlay,
                        contentPadding = PaddingValues(horizontal = 12.dp, vertical = 4.dp)
                    ) {
                        Icon(
                            imageVector = if (isPhoto) Icons.Default.Image else Icons.Default.PlayArrow,
                            contentDescription = null,
                            modifier = Modifier.size(16.dp)
                        )
                        Text(if (isPhoto) "View" else "Play In-App", modifier = Modifier.padding(start = 4.dp))
                    }
                } else {
                    AstryxStatusBadge("APP VAULT ONLY", AstryxTone.Neutral)
                }
            }
            IconButton(onClick = onDelete) {
                Icon(Icons.Default.DeleteOutline, contentDescription = "Delete", tint = palette.error)
            }
        }
    }
}

@Composable
private fun EmptyGalleryCard(title: String, body: String) {
    val palette = LocalAstryxPalette.current
    AstryxCard(contentPadding = PaddingValues(20.dp)) {
        Text(title, style = MaterialTheme.typography.titleMedium, color = palette.textPrimary)
        Text(body, style = MaterialTheme.typography.bodySmall, color = palette.textSecondary)
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
