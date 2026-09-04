
(function() {
    'use strict';

    // State
    var tasksMap = {};
    var currentFilter = 'all';
    var selectedFormat = 'highest';
    var currentViewMode = localStorage.getItem('kv_view_mode') || 'standard';
    var currentHistoryViewMode = localStorage.getItem('kv_history_view_mode') || 'compact';
    var currentHistoryTypeFilter = 'all';
    var currentThemeMode = localStorage.getItem('kv_theme_mode');
    if (!currentThemeMode) {
        var legacyTheme = localStorage.getItem('kv_theme');
        currentThemeMode = (legacyTheme === 'dark' || legacyTheme === 'light' || legacyTheme === 'system') ? legacyTheme : 'system';
    }
    var currentSpeedLimit = localStorage.getItem('kv_speed_limit') || 'unlimited';
    var activeView = 'download-center';
    var currentAlbumEntries = [];
    var currentPlayingTask = null;
    var currentHlsInstance = null;

    // DOM Elements - Download Center
    var urlInput = document.getElementById('urlInput');
    var submitBtn = document.getElementById('submitBtn');
    var submitBtnText = document.getElementById('submitBtnText');
    var submitBtnIcon = document.getElementById('submitBtnIcon');
    var btnClearInput = document.getElementById('btnClearInput');
    var btnPasteClipboard = document.getElementById('btnPasteClipboard');
    var detectedPlatformBadge = document.getElementById('detectedPlatformBadge');
    var detectedPlatformText = document.getElementById('detectedPlatformText');
    var urlCountIndicator = document.getElementById('urlCountIndicator');
    var urlCountNumber = document.getElementById('urlCountNumber');
    var taskList = document.getElementById('taskList');
    var historyList = document.getElementById('historyList');
    var queueList = document.getElementById('queueList');
    var connStatus = document.getElementById('connStatus');

    // DOM Elements - History & Media Player
    var historySearch = document.getElementById('historySearch');
    var btnClearHistorySearch = document.getElementById('btnClearHistorySearch');
    var historySortSelect = document.getElementById('historySortSelect');
    var btnClearAllHistory = document.getElementById('btnClearAllHistory');
    var androidPlayerBar = document.getElementById('androidPlayerBar');
    var playerBarThumb = document.getElementById('playerBarThumb');
    var playerBarTitle = document.getElementById('playerBarTitle');
    var playerBarSubtitle = document.getElementById('playerBarSubtitle');
    var playerBarPlayIcon = document.getElementById('playerBarPlayIcon');
    var playerBarWave = document.getElementById('playerBarWave');
    var playerBarScrubber = document.getElementById('playerBarScrubber');
    var playerBarCurrentTime = document.getElementById('playerBarCurrentTime');
    var playerBarDuration = document.getElementById('playerBarDuration');
    var btnPlayerPlayPause = document.getElementById('btnPlayerPlayPause');
    var btnPlayerRewind = document.getElementById('btnPlayerRewind');
    var btnPlayerForward = document.getElementById('btnPlayerForward');
    var btnPlayerClose = document.getElementById('btnPlayerClose');
    var globalAudioEngine = document.getElementById('globalAudioEngine');
    var globalVideoEngine = document.getElementById('globalVideoEngine');


    // DOM Elements - Settings & yt-dlp version
    var ytdlpChannelSelect = document.getElementById('ytdlpChannelSelect');
    var btnUpdateYtDlp = document.getElementById('btnUpdateYtDlp');
    var btnUpdateYtDlpIcon = document.getElementById('btnUpdateYtDlpIcon');
    var btnUpdateYtDlpText = document.getElementById('btnUpdateYtDlpText');
    var ytdlpActiveChannelBadge = document.getElementById('ytdlpActiveChannelBadge');
    var ytdlpVersionStatusText = document.getElementById('ytdlpVersionStatusText');
    var navYtDlpVer = document.getElementById('navYtDlpVer');
    var settingsYtDlpVer = document.getElementById('settingsYtDlpVer');
    var footerYtDlpVer = document.getElementById('footerYtDlpVer');
    var speedLimitSelect = document.getElementById('speedLimitSelect');
    var concurrentWorkerSelect = document.getElementById('concurrentWorkerSelect');
    var notifToggle = document.getElementById('notifToggle');
    var themeToggleBtn = document.getElementById('themeToggleBtn');

    // Modals
    var albumModal = document.getElementById('albumModal');
    var albumModalTitle = document.getElementById('albumModalTitle');
    var albumModalCount = document.getElementById('albumModalCount');
    var albumModalLoading = document.getElementById('albumModalLoading');
    var albumItemsGrid = document.getElementById('albumItemsGrid');
    var albumSelectedCount = document.getElementById('albumSelectedCount');
    var albumFormatSelect = document.getElementById('albumFormatSelect');
    var btnDownloadAlbumSelected = document.getElementById('btnDownloadAlbumSelected');
    var btnDownloadAlbumText = document.getElementById('btnDownloadAlbumText');
    var btnCloseAlbumModal = document.getElementById('btnCloseAlbumModal');
    var btnCancelAlbumModal = document.getElementById('btnCancelAlbumModal');
    var btnAlbumSelectAll = document.getElementById('btnAlbumSelectAll');
    var btnAlbumDeselectAll = document.getElementById('btnAlbumDeselectAll');

    // Cookie Modal
    var cookiesModal = document.getElementById('cookiesModal');
    var btnOpenCookiesNav = document.getElementById('btnOpenCookiesNav');
    var btnQuickCookie = document.getElementById('btnQuickCookie');
    var btnCloseCookiesModal = document.getElementById('btnCloseCookiesModal');
    var btnCloseCookiesFooter = document.getElementById('btnCloseCookiesFooter');
    var cookieInputArea = document.getElementById('cookieInputArea');
    var cookieDetectedFormat = document.getElementById('cookieDetectedFormat');
    var btnImportCookieFile = document.getElementById('btnImportCookieFile');
    var cookieFileInput = document.getElementById('cookieFileInput');
    var btnPasteCookieClipboard = document.getElementById('btnPasteCookieClipboard');
    var btnClearSavedCookies = document.getElementById('btnClearSavedCookies');
    var btnSaveCookies = document.getElementById('btnSaveCookies');
    var cookieNavBadge = document.getElementById('cookieNavBadge');
    var btnCopyIosCode = document.getElementById('btnCopyIosCode');

    // View Navigation
    function switchView(viewName) {
        activeView = viewName;
        document.querySelectorAll('.view-section').forEach(function(s) { s.classList.remove('active'); });
        var targetSection = document.getElementById('view-' + viewName);
        if (targetSection) targetSection.classList.add('active');

        document.querySelectorAll('.sidenav-link').forEach(function(link) {
            var isMatch = link.getAttribute('data-view') === viewName;
            link.classList.toggle('active', isMatch);
            if (link.classList.contains('flex-col')) {
                link.classList.toggle('text-primary', isMatch);
                link.classList.toggle('text-on-surface-variant', !isMatch);
            }
        });

        var titles = {
            'download-center': ['Download Center', 'Paste any link or M3U8 stream to fetch or download media in high quality.'],
            'browser': ['In-App Browser', 'Browse any website with live media sniffing, M3U8 stream capture, and instant downloads.'],
            'history': ['Media Gallery', 'Full-featured media preview, Android lockscreen controls, and file management.'],
            'queues': ['Queue Manager', 'Monitor active worker threads and manage queued tasks.'],
            'playlists': ['Playlists & Albums', 'Inspect and batch download full albums and playlists.'],
            'settings': ['Application Settings', 'Manage yt-dlp release channels (stable, nightly), speed limits, and preferences.'],
            'api-docs': ['API Reference & Integrations', 'Interactive REST API and Aria2 JSON-RPC documentation for developers and integrations.']
        };
        var t = titles[viewName] || titles['download-center'];
        document.getElementById('topTitle').textContent = t[0];
        document.getElementById('topSubtitle').textContent = t[1];

        if (viewName === 'history') { fetchGallery(); if (historySearch) historySearch.focus(); }
        if (viewName === 'settings') { checkYtDlpVersionInfo(); }
        if (viewName === 'browser' && !browserInitialized) { initBrowserView(); }
        renderTasks();
    }

    document.querySelectorAll('[data-view]').forEach(function(el) {
        el.addEventListener('click', function(e) {
            e.preventDefault();
            switchView(this.getAttribute('data-view'));
        });
    });

    // Helpers
    function getUrls() {
        var raw = urlInput.value.trim();
        if (!raw) return [];
        return raw.split(/\n+/).map(function(u) { return u.trim(); }).filter(function(u) { return u.length > 0; });
    }
    function formatBytes(bytes) {
        if (!bytes || bytes <= 0) return '0 B';
        var k = 1024;
        var sizes = ['B', 'KB', 'MB', 'GB', 'TB'];
        var i = Math.floor(Math.log(bytes) / Math.log(k));
        if (i >= sizes.length) i = sizes.length - 1;
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + ' ' + sizes[i];
    }

    function formatDuration(sec) {
        if (!sec || isNaN(sec) || sec <= 0) return '';
        var s = Math.floor(sec);
        var m = Math.floor(s / 60);
        var r = s % 60;
        if (m >= 60) {
            var h = Math.floor(m / 60);
            m = m % 60;
            return h + ':' + (m < 10 ? '0' : '') + m + ':' + (r < 10 ? '0' : '') + r;
        }
        return m + ':' + (r < 10 ? '0' : '') + r;
    }

    function isPlaylistUrl(url) {
        if (!url) return false;
        var u = url.toLowerCase().trim();
        // TikTok channel / creator profile (e.g. tiktok.com/@username without /video/ or /photo/)
        if (u.indexOf('tiktok.com/@') !== -1 && u.indexOf('/video/') === -1 && u.indexOf('/photo/') === -1) {
            return true;
        }
        // Facebook profile / page / group / videos / reels tab
        if (u.indexOf('facebook.com') !== -1 || u.indexOf('fb.com') !== -1) {
            var isFbVideo = /(?:\/videos\/|\/posts\/|\/reel\/|watch\?v=|video\.php\?v=)\d{6,}/i.test(u);
            if (!isFbVideo) {
                var cleanPath = u.replace(/^https?:\/\/[^\/]+/, '').replace(/\/+$/, '');
                if (cleanPath && cleanPath !== '') {
                    return true;
                }
            }
        }
        // YouTube channel / user / playlist
        if (u.indexOf('youtube.com/@') !== -1 || u.indexOf('youtube.com/channel/') !== -1 || u.indexOf('youtube.com/c/') !== -1 || u.indexOf('youtube.com/user/') !== -1 || u.indexOf('youtu.be/@') !== -1) {
            return true;
        }
        // SoundCloud sets / users
        if (u.indexOf('soundcloud.com/') !== -1 && (u.indexOf('/sets/') !== -1 || u.indexOf('/albums/') !== -1)) {
            return true;
        }
        // Standard playlist & album tokens
        return u.indexOf('list=') !== -1 || u.indexOf('/playlist') !== -1 || u.indexOf('/album/') !== -1 || u.indexOf('/sets/') !== -1 || u.indexOf('/collection/') !== -1 || u.indexOf('pixiv.net') !== -1 || u.indexOf('imgur.com/a/') !== -1 || u.indexOf('imgur.com/gallery/') !== -1 || u.indexOf('pixeldrain.com/l/') !== -1 || u.indexOf('archive.org/details/') !== -1;
    }

    function detectPlatform(url) {
        if (!url) return null;
        var u = url.toLowerCase();
        if (u.indexOf('.m3u8') !== -1) return { name: 'M3U8 Stream', icon: 'bi-broadcast-pin', color: '#06b6d4' };
        if (u.indexOf('youtube.com') !== -1 || u.indexOf('youtu.be') !== -1) return { name: 'YouTube', icon: 'bi-youtube', color: '#ff4d4d' };
        if (u.indexOf('tiktok.com') !== -1) return { name: 'TikTok', icon: 'bi-tiktok', color: '#00f2fe' };
        if (u.indexOf('threads.net') !== -1) return { name: 'Threads', icon: 'bi-threads', color: '#ffffff' };
        if (u.indexOf('instagram.com') !== -1) return { name: 'Instagram', icon: 'bi-instagram', color: '#f43f5e' };
        if (u.indexOf('twitter.com') !== -1 || u.indexOf('x.com') !== -1) return { name: 'Twitter / X', icon: 'bi-twitter-x', color: '#38bdf8' };
        if (u.indexOf('reddit.com') !== -1) return { name: 'Reddit', icon: 'bi-reddit', color: '#fb923c' };
        if (u.indexOf('vimeo.com') !== -1) return { name: 'Vimeo', icon: 'bi-vimeo', color: '#38bdf8' };
        if (u.indexOf('soundcloud.com') !== -1) return { name: 'SoundCloud', icon: 'bi-soundwave', color: '#fbbf24' };
        if (u.indexOf('facebook.com') !== -1) return { name: 'Facebook', icon: 'bi-facebook', color: '#60a5fa' };
        if (u.indexOf('twitch.tv') !== -1) return { name: 'Twitch', icon: 'bi-twitch', color: '#a855f7' };
        if (u.indexOf('bilibili.com') !== -1) return { name: 'Bilibili', icon: 'bi-tv', color: '#00aeec' };
        if (u.indexOf('duckduckgo.com') !== -1) return { name: 'DuckDuckGo', icon: 'bi-search', color: '#fb923c' };
        if (u.indexOf('http://') !== -1 || u.indexOf('https://') !== -1) return { name: 'Web Media', icon: 'bi-globe2', color: '#a78bfa' };
        return null;
    }

    function formatTime(secs) {
        if (!secs || isNaN(secs) || secs < 0) return '0:00';
        var m = Math.floor(secs / 60);
        var s = Math.floor(secs % 60);
        return m + ':' + (s < 10 ? '0' : '') + s;
    }

    function updateInputDetection() {
        var raw = urlInput.value.trim();
        var urls = getUrls();
        if (btnClearInput) btnClearInput.classList.toggle('d-none', raw.length === 0);
        if (urlCountIndicator) {
            urlCountIndicator.style.opacity = urls.length > 0 ? '1' : '0';
            if (urlCountNumber) urlCountNumber.textContent = urls.length;
        }
        if (urls.length === 0) {
            if (detectedPlatformBadge) detectedPlatformBadge.classList.add('d-none');
            return;
        }
        if (detectedPlatformBadge) detectedPlatformBadge.classList.remove('d-none');
        if (urls.length === 1) {
            var isPl = isPlaylistUrl(urls[0]);
            var plat = detectPlatform(urls[0]);
            if (isPl) {
                if (urls[0].indexOf('facebook.com') !== -1 || urls[0].indexOf('fb.com') !== -1) {
                    detectedPlatformText.textContent = 'Facebook Profile Videos';
                } else if (urls[0].indexOf('tiktok.com/@') !== -1) {
                    detectedPlatformText.textContent = 'TikTok Channel Videos';
                } else {
                    detectedPlatformText.textContent = 'Album / Playlist';
                }
            } else if (plat) {
                detectedPlatformText.textContent = plat.name + ' Detected';
            } else {
                detectedPlatformText.textContent = 'Custom URL';
            }
        } else {
            detectedPlatformText.textContent = 'Batch: ' + urls.length + ' URLs';
        }
    }

    urlInput.addEventListener('input', function() {
        this.style.height = 'auto';
        this.style.height = Math.min(this.scrollHeight, 160) + 'px';
        updateInputDetection();
    });

    btnClearInput.addEventListener('click', function() {
        urlInput.value = '';
        urlInput.style.height = 'auto';
        updateInputDetection();
        urlInput.focus();
    });

    btnPasteClipboard.addEventListener('click', async function() {
        try {
            if (navigator.clipboard && navigator.clipboard.readText) {
                var text = await navigator.clipboard.readText();
                if (text && text.trim()) {
                    urlInput.value = urlInput.value.trim() ? urlInput.value + '\n' + text.trim() : text.trim();
                    urlInput.dispatchEvent(new Event('input'));
                    urlInput.focus();
                    showToast('Pasted URL from clipboard', 'success');
                } else {
                    showToast('Clipboard is empty', 'warning');
                }
            } else {
                urlInput.focus();
                document.execCommand('paste');
            }
        } catch (e) {
            urlInput.focus();
            showToast('Please paste manually (Ctrl+V)', 'info');
        }
    });

    // Quality format buttons
    document.querySelectorAll('.format-pill-card').forEach(function(pill) {
        pill.addEventListener('click', function() {
            document.querySelectorAll('.format-pill-card').forEach(function(p) { p.classList.remove('active'); });
            this.classList.add('active');
            selectedFormat = this.getAttribute('data-format');
            if (albumFormatSelect) albumFormatSelect.value = selectedFormat;
        });
    });

    // =========================================================================
    // 3 MODE GRID VIEWS TOGGLE (Download Center & History)
    // =========================================================================
    function applyViewMode(mode) {
        currentViewMode = mode;
        localStorage.setItem('kv_view_mode', mode);
        taskList.className = 'task-items-container view-' + mode + ' min-h-[140px]';
        document.querySelectorAll('.grid-mode-btn').forEach(function(btn) {
            var match = btn.getAttribute('data-mode') === mode;
            btn.classList.toggle('active', match);
            btn.className = match ? 'grid-mode-btn active p-1.5 rounded-lg bg-white/20 text-white transition-all' : 'grid-mode-btn p-1.5 rounded-lg text-on-surface-variant hover:text-white transition-all';
        });
    }

    document.querySelectorAll('.grid-mode-btn').forEach(function(btn) {
        btn.addEventListener('click', function() { applyViewMode(this.getAttribute('data-mode')); });
    });

    function applyHistoryViewMode(mode) {
        currentHistoryViewMode = mode;
        localStorage.setItem('kv_history_view_mode', mode);
        historyList.className = 'task-items-container view-' + mode;
        document.querySelectorAll('.history-grid-mode-btn').forEach(function(btn) {
            var match = btn.getAttribute('data-mode') === mode;
            btn.classList.toggle('active', match);
            btn.className = match ? 'history-grid-mode-btn active p-1.5 rounded-lg bg-white/20 text-white transition-all' : 'history-grid-mode-btn p-1.5 rounded-lg text-on-surface-variant hover:text-white transition-all';
        });
    }

    document.querySelectorAll('.history-grid-mode-btn').forEach(function(btn) {
        btn.addEventListener('click', function() { applyHistoryViewMode(this.getAttribute('data-mode')); });
    });

    // Gallery / History State from Disk
    var galleryData = { items: [], folders: [], total: 0 };
    var currentGalleryFolder = '';
    var currentHistoryTypeFilter = 'all';

    function fetchGallery() {
        var xhr = new XMLHttpRequest();
        xhr.open('GET', '/api/gallery', true);
        xhr.onload = function() {
            if (xhr.status === 200) {
                try {
                    galleryData = JSON.parse(xhr.responseText);
                    renderGalleryFolders();
                    renderGalleryItems();
                } catch (e) {}
            }
        };
        xhr.send();
    }

    function renderGalleryFolders() {
        var chipsContainer = document.getElementById('galleryFolderChips');
        var totalBadge = document.getElementById('galleryTotalCount');
        if (totalBadge) totalBadge.textContent = galleryData.total || 0;
        if (!chipsContainer) return;

        chipsContainer.innerHTML = '';
        if (galleryData.folders && Array.isArray(galleryData.folders)) {
            galleryData.folders.forEach(function(f) {
                var btn = document.createElement('button');
                btn.type = 'button';
                var isActive = currentGalleryFolder === f.name;
                btn.className = 'gallery-folder-chip px-3 py-1 rounded-lg text-[11px] font-semibold transition-all shrink-0 flex items-center gap-1 ' + (isActive ? 'bg-primary text-black font-bold' : 'bg-white/5 text-on-surface-variant hover:text-white border border-white/5');
                btn.setAttribute('data-folder', f.name);
                btn.innerHTML = '<i class="bi bi-folder-fill text-amber-400"></i> ' + escapeHtml(f.name) + ' <span class="text-[9px] px-1.5 py-0.2 rounded-full ' + (isActive ? 'bg-black/30 text-black' : 'bg-white/10 text-on-surface-variant') + '">' + f.fileCount + '</span>';
                btn.addEventListener('click', function() {
                    currentGalleryFolder = f.name;
                    updateFolderChipStyles();
                    renderGalleryItems();
                });
                chipsContainer.appendChild(btn);
            });
        }
    }

    function updateFolderChipStyles() {
        var allBtn = document.getElementById('btnGalleryFolderAll');
        if (allBtn) {
            var isAll = !currentGalleryFolder;
            allBtn.className = 'gallery-folder-chip px-3 py-1 rounded-lg text-[11px] font-semibold transition-all shrink-0 ' + (isAll ? 'bg-primary text-black font-bold' : 'bg-white/5 text-on-surface-variant hover:text-white border border-white/5');
        }
        document.querySelectorAll('#galleryFolderChips .gallery-folder-chip').forEach(function(c) {
            var act = c.getAttribute('data-folder') === currentGalleryFolder;
            c.className = 'gallery-folder-chip px-3 py-1 rounded-lg text-[11px] font-semibold transition-all shrink-0 flex items-center gap-1 ' + (act ? 'bg-primary text-black font-bold' : 'bg-white/5 text-on-surface-variant hover:text-white border border-white/5');
        });
    }

    var btnGalleryFolderAll = document.getElementById('btnGalleryFolderAll');
    if (btnGalleryFolderAll) {
        btnGalleryFolderAll.addEventListener('click', function() {
            currentGalleryFolder = '';
            updateFolderChipStyles();
            renderGalleryItems();
        });
    }

    function renderGalleryItems() {
        if (!historyList) return;
        var items = (galleryData.items || []).slice();

        // 1. Filter by Folder / Channel
        if (currentGalleryFolder) {
            items = items.filter(function(it) {
                return it.channel === currentGalleryFolder;
            });
        }

        // 2. Filter by Type (all / video / audio / photo)
        if (currentHistoryTypeFilter && currentHistoryTypeFilter !== 'all') {
            items = items.filter(function(it) {
                return it.type === currentHistoryTypeFilter;
            });
        }

        // 3. Search query
        var q = historySearch ? historySearch.value.trim().toLowerCase() : '';
        if (q) {
            items = items.filter(function(it) {
                return (it.title && it.title.toLowerCase().indexOf(q) !== -1) ||
                       (it.name && it.name.toLowerCase().indexOf(q) !== -1) ||
                       (it.channel && it.channel.toLowerCase().indexOf(q) !== -1);
            });
        }

        // Update counts in header
        var allEl = document.getElementById('histCountAll');
        if (allEl) allEl.textContent = (galleryData.items || []).length;

        // 4. Sort
        var sortMode = historySortSelect ? historySortSelect.value : 'newest';
        items.sort(function(a, b) {
            if (sortMode === 'oldest') return new Date(a.modTime) - new Date(b.modTime);
            if (sortMode === 'size') return (b.sizeInBytes || 0) - (a.sizeInBytes || 0);
            if (sortMode === 'name') return (a.title || a.name).localeCompare(b.title || b.name);
            return new Date(b.modTime) - new Date(a.modTime);
        });

        // Map GalleryItem to card format
        var mapped = items.map(function(it) {
            return {
                id: it.id,
                title: it.title || it.name,
                mediaName: it.name,
                channel: it.channel,
                humanSize: it.humanSize,
                totalBytes: it.sizeInBytes,
                status: 'completed',
                mediaId: it.id,
                url: it.url,
                thumbnail: it.thumbnail,
                createdAt: it.modTime,
                format: it.type === 'audio' ? 'audio' : 'video'
            };
        });

        renderTaskGrid(historyList, mapped, currentHistoryViewMode, true);
    }

    // History Search & Filters
    if (historySearch) {
        historySearch.addEventListener('input', function() {
            if (btnClearHistorySearch) btnClearHistorySearch.classList.toggle('d-none', !this.value);
            renderGalleryItems();
        });
    }
    if (btnClearHistorySearch) {
        btnClearHistorySearch.addEventListener('click', function() {
            historySearch.value = '';
            this.classList.add('d-none');
            renderGalleryItems();
            historySearch.focus();
        });
    }

    document.querySelectorAll('.history-type-chip').forEach(function(chip) {
        chip.addEventListener('click', function() {
            document.querySelectorAll('.history-type-chip').forEach(function(c) {
                c.classList.remove('active', 'bg-primary', 'text-black');
                c.classList.add('bg-white/5', 'text-on-surface-variant');
            });
            this.classList.add('active', 'bg-primary', 'text-black');
            this.classList.remove('bg-white/5', 'text-on-surface-variant');
            currentHistoryTypeFilter = this.getAttribute('data-type');
            renderGalleryItems();
        });
    });

    if (historySortSelect) {
        historySortSelect.addEventListener('change', function() { renderGalleryItems(); });
    }

    if (btnClearAllHistory) {
        btnClearAllHistory.addEventListener('click', function() {
            document.getElementById('btnClearCompleted').click();
        });
    }

    // =========================================================================
    // YT-DLP VERSION & CHANNEL MANAGEMENT
    // =========================================================================
    function checkYtDlpVersionInfo() {
        var xhr = new XMLHttpRequest();
        xhr.open('GET', '/api/ytdlp/version', true);
        xhr.onload = function() {
            if (xhr.status === 200) {
                try {
                    var d = JSON.parse(xhr.responseText);
                    if (ytdlpActiveChannelBadge) {
                        ytdlpActiveChannelBadge.textContent = d.channel || 'Stable';
                        ytdlpActiveChannelBadge.className = (d.channel === 'nightly' || d.channel === 'master') ? 'text-[10px] px-2 py-0.5 rounded-full bg-cyan-500/20 text-cyan-300 border border-cyan-500/30 font-bold uppercase font-mono' : 'text-[10px] px-2 py-0.5 rounded-full bg-primary/20 text-primary border border-primary/30 font-bold uppercase font-mono';
                    }
                    if (ytdlpVersionStatusText) {
                        ytdlpVersionStatusText.textContent = 'Installed: ' + d.current + ' • Stable: ' + (d.latestStable || 'latest') + ' • Nightly: ' + (d.latestNightly || 'latest');
                    }
                    if (navYtDlpVer) navYtDlpVer.textContent = d.current;
                    if (settingsYtDlpVer) settingsYtDlpVer.textContent = d.current;
                    if (footerYtDlpVer) footerYtDlpVer.textContent = d.current;
                    if (ytdlpChannelSelect && d.channel) {
                        ytdlpChannelSelect.value = d.channel;
                    }
                } catch (e) {}
            }
        };
        xhr.send();
    }

    if (btnUpdateYtDlp) {
        btnUpdateYtDlp.addEventListener('click', function() {
            var ch = ytdlpChannelSelect ? ytdlpChannelSelect.value : 'stable';
            btnUpdateYtDlp.disabled = true;
            btnUpdateYtDlpIcon.className = 'bi bi-arrow-repeat';
            btnUpdateYtDlpIcon.style.animation = 'spin 1s linear infinite';
            btnUpdateYtDlpText.textContent = 'Switching to ' + ch + '...';

            var xhr = new XMLHttpRequest();
            xhr.open('POST', '/api/ytdlp/update?channel=' + encodeURIComponent(ch), true);
            xhr.onload = function() {
                btnUpdateYtDlp.disabled = false;
                btnUpdateYtDlpIcon.className = 'bi bi-cloud-arrow-down-fill';
                btnUpdateYtDlpIcon.style.animation = '';
                btnUpdateYtDlpText.textContent = 'Update Channel';

                if (xhr.status === 200) {
                    try {
                        var res = JSON.parse(xhr.responseText);
                        showToast(res.message || ('yt-dlp updated to ' + ch), 'success');
                        checkYtDlpVersionInfo();
                    } catch (e) {}
                } else {
                    showToast('Update failed: ' + xhr.statusText, 'error');
                }
            };
            xhr.onerror = function() {
                btnUpdateYtDlp.disabled = false;
                btnUpdateYtDlpIcon.className = 'bi bi-cloud-arrow-down-fill';
                btnUpdateYtDlpIcon.style.animation = '';
                btnUpdateYtDlpText.textContent = 'Update Channel';
                showToast('Network error during update', 'error');
            };
            xhr.send();
        });
    }

    // =========================================================================
    // SETTINGS: SPEED LIMITER & WORKERS
    // =========================================================================
    if (speedLimitSelect) {
        speedLimitSelect.value = currentSpeedLimit;
        speedLimitSelect.addEventListener('change', function() {
            currentSpeedLimit = this.value;
            localStorage.setItem('kv_speed_limit', currentSpeedLimit);
            showToast('Download rate limit updated to ' + (currentSpeedLimit === 'unlimited' ? 'Unlimited' : currentSpeedLimit), 'success');
        });
    }

    if (concurrentWorkerSelect) {
        concurrentWorkerSelect.addEventListener('change', function() {
            var val = parseInt(this.value, 10) || 3;
            localStorage.setItem('kv_max_workers', val);
            var x = new XMLHttpRequest();
            x.open('POST', '/api/queue/workers?count=' + val, true);
            x.onload = function() {
                showToast('Max concurrent downloads updated to ' + val + ' tasks', 'success');
            };
            x.onerror = function() {
                showToast('Max concurrent tasks set to ' + val, 'info');
            };
            x.send();
        });
        var savedWorkers = localStorage.getItem('kv_max_workers');
        if (savedWorkers) {
            concurrentWorkerSelect.value = savedWorkers;
            var xInit = new XMLHttpRequest();
            xInit.open('POST', '/api/queue/workers?count=' + savedWorkers, true);
            xInit.send();
        } else {
            var xGet = new XMLHttpRequest();
            xGet.open('GET', '/api/queue/workers', true);
            xGet.onload = function() {
                try {
                    var data = JSON.parse(xGet.responseText);
                    if (data && data.maxWorkers) {
                        concurrentWorkerSelect.value = data.maxWorkers;
                    }
                } catch(e) {}
            };
            xGet.send();
        }
    }

    // Submit / Enqueue handling
    function enqueueUrls(urls, format, items, playlistMeta) {
        if (!urls || urls.length === 0) return;
        submitBtn.disabled = true;
        submitBtnIcon.textContent = 'sync';
        submitBtnIcon.style.animation = 'spin 1s linear infinite';
        submitBtnText.textContent = 'Enqueuing ' + urls.length + '...';

        var cookies = localStorage.getItem('kv_user_cookies') || '';
        var rateLimit = localStorage.getItem('kv_speed_limit') || '';
        var payload = {
            urls: urls,
            items: items || [],
            format: format,
            cookies: cookies,
            rateLimit: rateLimit,
            playlistTitle: (playlistMeta && playlistMeta.title) || '',
            channel: (playlistMeta && (playlistMeta.channel || playlistMeta.uploader)) || ''
        };

        var xhr = new XMLHttpRequest();
        xhr.open('POST', '/api/queue/add', true);
        xhr.setRequestHeader('Content-Type', 'application/json');
        xhr.onload = function() {
            submitBtn.disabled = false;
            submitBtnIcon.textContent = 'download';
            submitBtnIcon.style.animation = '';
            submitBtnText.textContent = 'Download';

            if (xhr.status === 200) {
                try {
                    var res = JSON.parse(xhr.responseText);
                    if (res.tasks && Array.isArray(res.tasks)) {
                        res.tasks.forEach(function(t) { tasksMap[t.id] = t; });
                    }
                } catch (e) {}
                setFilter('all');
                switchView('download-center');
                showToast(urls.length + (urls.length === 1 ? ' task enqueued!' : ' tasks enqueued!'), 'success');
            } else {
                showToast('Failed to enqueue: ' + xhr.statusText, 'error');
            }
        };
        xhr.onerror = function() {
            submitBtn.disabled = false;
            submitBtnIcon.textContent = 'download';
            submitBtnIcon.style.animation = '';
            submitBtnText.textContent = 'Download';
            showToast('Network error while enqueuing', 'error');
        };
        xhr.send(JSON.stringify(payload));
    }

    submitBtn.addEventListener('click', function() {
        var urls = getUrls();
        if (urls.length === 0) {
            urlInput.focus();
            showToast('Please enter at least one URL', 'warning');
            return;
        }
        if (urls.length === 1 && isPlaylistUrl(urls[0])) {
            openAlbumModal(urls[0]);
            return;
        }
        enqueueUrls(urls, selectedFormat);
    });

    urlInput.addEventListener('keydown', function(e) {
        if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
            e.preventDefault();
            submitBtn.click();
        }
    });

    // =========================================================================
    // CHANNEL / PLAYLIST / ALBUM PICKER MODAL (LIVE STREAMING & STOP-AND-SHOW)
    // =========================================================================
    var currentAlbumUrl = '';
    var currentAlbumScan = null;
    var currentAlbumEntries = [];
    var selectedAlbumIndices = new Set();
    var streamedEntries = [];
    var streamedMeta = null;
    var scanEventSource = null;
    var scanAbortController = null;

    var albumFilterInput = document.getElementById('albumFilterInput');
    var albumModalThumbWrap = document.getElementById('albumModalThumbWrap');
    var albumModalThumb = document.getElementById('albumModalThumb');
    var albumModalIcon = document.getElementById('albumModalIcon');
    var albumModalBadge = document.getElementById('albumModalBadge');
    var albumModalPlatform = document.getElementById('albumModalPlatform');
    var albumLoadMoreWrap = document.getElementById('albumLoadMoreWrap');
    var albumCategoryFilterRow = document.getElementById('albumCategoryFilterRow');
    var albumCategoryTabs = document.getElementById('albumCategoryTabs');
    var currentAlbumCategory = 'all';

    function openAlbumModal(url) {
        var cleanUrl = (url || '').trim();
        if (cleanUrl.indexOf('tiktok.com/@') !== -1 && cleanUrl.indexOf('?') !== -1) {
            cleanUrl = cleanUrl.split('?')[0];
        }

        currentAlbumUrl = cleanUrl;
        currentAlbumEntries = [];
        selectedAlbumIndices.clear();
        streamedEntries = [];
        streamedMeta = null;
        currentAlbumCategory = 'all';
        if (albumCategoryFilterRow) {
            albumCategoryFilterRow.classList.add('d-none');
            albumCategoryFilterRow.style.display = 'none';
        }
        if (albumCategoryTabs) albumCategoryTabs.innerHTML = '';

        if (scanAbortController) {
            scanAbortController.abort();
            scanAbortController = null;
        }
        if (scanEventSource) {
            scanEventSource.close();
            scanEventSource = null;
        }

        albumModal.classList.remove('d-none');
        albumModal.style.display = 'flex';
        
        var plat = detectPlatform(cleanUrl) || { name: 'Media Channel', icon: 'bi-collection-play-fill', color: '#a078ff' };
        if (albumModalBadge) {
            var isFb = cleanUrl.indexOf('facebook.com') !== -1 || cleanUrl.indexOf('fb.com') !== -1;
            var badgeText = cleanUrl.indexOf('tiktok.com/@') !== -1 ? 'TikTok Creator Channel' :
                (isFb ? 'Facebook Profile Videos' :
                (cleanUrl.indexOf('youtube.com') !== -1 ? 'YouTube Playlist / Channel' : 'Album / Playlist'));
            albumModalBadge.innerHTML = '<i class="bi ' + plat.icon + '"></i> ' + badgeText;
        }
        if (albumModalPlatform) albumModalPlatform.textContent = '• ' + plat.name;
        if (albumModalThumb) albumModalThumb.classList.add('d-none');
        if (albumModalIcon) {
            albumModalIcon.className = 'bi ' + plat.icon + ' text-[20px]';
            albumModalIcon.classList.remove('d-none');
        }

        albumModalTitle.textContent = 'Scanning ' + (plat.name || 'channel') + '...';
        albumModalCount.textContent = 'Discovering videos with yt-dlp...';
        if (albumFilterInput) albumFilterInput.value = '';
        if (albumLoadMoreWrap) {
            albumLoadMoreWrap.classList.add('d-none');
            albumLoadMoreWrap.style.display = 'none';
        }
        updateAlbumSelectionCount();

        // Render Loading State with Live Video Counter & Stop Button
        albumModalLoading.classList.remove('d-none');
        albumModalLoading.style.display = 'flex';
        albumItemsGrid.classList.add('d-none');
        albumItemsGrid.style.display = 'none';

        albumModalLoading.innerHTML = '<div style="display:flex;flex-direction:column;align-items:center;gap:16px;padding:32px 16px;text-align:center;max-width:440px;margin:0 auto;">' +
            '<div style="position:relative;display:flex;align-items:center;justify-content:center;">' +
            '<div style="width:54px;height:54px;border:3px solid rgba(255,255,255,.12);border-top-color:#a078ff;border-radius:50%;animation:spin 0.8s linear infinite;"></div>' +
            '<i class="bi ' + plat.icon + '" style="position:absolute;font-size:20px;color:#a078ff;"></i>' +
            '</div>' +
            '<div style="display:flex;flex-direction:column;align-items:center;gap:4px;">' +
            '<div style="display:flex;align-items:center;gap:8px;">' +
            '<span id="albumLiveFoundCount" style="font-size:40px;font-weight:900;color:#fff;font-family:monospace;line-height:1;">0</span>' +
            '<span style="font-size:12px;font-weight:700;color:#a078ff;padding:4px 12px;border-radius:999px;background:rgba(160,120,255,.15);border:1px solid rgba(160,120,255,.3);text-transform:uppercase;letter-spacing:.05em;">Videos Found</span>' +
            '</div>' +
            '<p id="albumLiveStatusText" style="font-size:13px;color:#94a3b8;font-weight:500;margin-top:4px;">Connecting & discovering video list...</p>' +
            '</div>' +
            '<div style="display:flex;align-items:center;gap:10px;margin-top:8px;justify-content:center;flex-wrap:wrap;">' +
            '<button type="button" id="btnStopAndShowVideos" class="btn-primary-glow" style="padding:10px 22px;border-radius:12px;font-size:12px;font-weight:700;display:flex;align-items:center;gap:6px;cursor:pointer;">' +
            '<i class="bi bi-stop-circle-fill text-rose-300 text-[15px]"></i> <span id="btnStopAndShowText">Stop & Show Videos (0)</span>' +
            '</button>' +
            '<button type="button" id="btnCancelScanModal" style="padding:10px 18px;border-radius:12px;background:rgba(255,255,255,.1);border:1px solid rgba(255,255,255,.1);color:#fff;font-size:12px;font-weight:600;cursor:pointer;">Cancel</button>' +
            '</div>' +
            '</div>';

        var liveCountEl = document.getElementById('albumLiveFoundCount');
        var liveStatusEl = document.getElementById('albumLiveStatusText');
        var stopBtn = document.getElementById('btnStopAndShowVideos');
        var stopBtnText = document.getElementById('btnStopAndShowText');
        var cancelBtn = document.getElementById('btnCancelScanModal');

        if (cancelBtn) cancelBtn.addEventListener('click', closeAlbumModal);

        function stopAndShowVideos() {
            if (scanAbortController) {
                scanAbortController.abort();
                scanAbortController = null;
            }
            if (scanEventSource) {
                scanEventSource.close();
                scanEventSource = null;
            }
            if (streamedEntries.length === 0) {
                showToast('Scanning in progress... Please wait a moment for initial videos', 'info');
                return;
            }
            showDiscoveredVideos();
        }

        if (stopBtn) stopBtn.addEventListener('click', stopAndShowVideos);

        function showDiscoveredVideos() {
            currentAlbumEntries = streamedEntries.slice();
            selectedAlbumIndices.clear();
            for (var i = 0; i < currentAlbumEntries.length; i++) {
                selectedAlbumIndices.add(i);
            }

            if (streamedMeta) {
                if (streamedMeta.title) albumModalTitle.textContent = streamedMeta.title;
                if (streamedMeta.thumbnail && albumModalThumb) {
                    albumModalThumb.src = streamedMeta.thumbnail;
                    albumModalThumb.classList.remove('d-none');
                    if (albumModalIcon) albumModalIcon.classList.add('d-none');
                }
            }
            albumModalCount.textContent = currentAlbumEntries.length + ' videos ready to download';

            // Build Category Filter Tabs
            if (albumCategoryFilterRow && albumCategoryTabs) {
                var catCounts = { all: currentAlbumEntries.length, reels: 0, videos: 0, posts: 0 };
                currentAlbumEntries.forEach(function(item) {
                    var c = (item.category || '').toLowerCase();
                    if (c.indexOf('reel') !== -1) catCounts.reels++;
                    else if (c.indexOf('post') !== -1) catCounts.posts++;
                    else catCounts.videos++;
                });

                var hasMultiple = (catCounts.reels > 0 && catCounts.videos > 0) || (catCounts.reels > 0 && catCounts.posts > 0) || (catCounts.videos > 0 && catCounts.posts > 0);
                if (hasMultiple || catCounts.reels > 0 || catCounts.posts > 0) {
                    albumCategoryFilterRow.classList.remove('d-none');
                    albumCategoryFilterRow.style.display = 'flex';
                    albumCategoryTabs.innerHTML = '';

                    var tabsConfig = [
                        { id: 'all', label: 'All', count: catCounts.all, color: '#a078ff' },
                        { id: 'reels', label: 'Reels', count: catCounts.reels, color: '#c084fc' },
                        { id: 'videos', label: 'Videos', count: catCounts.videos, color: '#38bdf8' },
                        { id: 'posts', label: 'Posts', count: catCounts.posts, color: '#60a5fa' }
                    ];

                    tabsConfig.forEach(function(cfg) {
                        if (cfg.id !== 'all' && cfg.count === 0) return;
                        var btn = document.createElement('button');
                        btn.type = 'button';
                        btn.className = 'album-cat-btn';
                        btn.setAttribute('data-cat', cfg.id);
                        var isActive = currentAlbumCategory === cfg.id;
                        btn.style.cssText = isActive
                            ? 'padding:3px 10px;border-radius:8px;font-weight:700;font-size:11px;cursor:pointer;background:' + cfg.color + ';color:#000;border:1px solid ' + cfg.color + ';'
                            : 'padding:3px 10px;border-radius:8px;font-weight:600;font-size:11px;cursor:pointer;background:rgba(255,255,255,.06);color:#e2e8f0;border:1px solid rgba(255,255,255,.1);';
                        btn.textContent = cfg.label + ' (' + cfg.count + ')';

                        btn.addEventListener('click', function() {
                            currentAlbumCategory = cfg.id;
                            albumCategoryTabs.querySelectorAll('.album-cat-btn').forEach(function(b) {
                                var bCat = b.getAttribute('data-cat');
                                var matchCfg = tabsConfig.find(function(tc) { return tc.id === bCat; });
                                if (bCat === currentAlbumCategory) {
                                    b.style.cssText = 'padding:3px 10px;border-radius:8px;font-weight:700;font-size:11px;cursor:pointer;background:' + (matchCfg ? matchCfg.color : '#a078ff') + ';color:#000;border:1px solid ' + (matchCfg ? matchCfg.color : '#a078ff') + ';';
                                } else {
                                    b.style.cssText = 'padding:3px 10px;border-radius:8px;font-weight:600;font-size:11px;cursor:pointer;background:rgba(255,255,255,.06);color:#e2e8f0;border:1px solid rgba(255,255,255,.1);';
                                }
                            });
                            renderAlbumEntries(currentAlbumEntries);
                            updateAlbumSelectionCount();
                        });
                        albumCategoryTabs.appendChild(btn);
                    });
                } else {
                    albumCategoryFilterRow.classList.add('d-none');
                    albumCategoryFilterRow.style.display = 'none';
                }
            }

            renderAlbumEntries(currentAlbumEntries);
            albumModalLoading.classList.add('d-none');
            albumModalLoading.style.display = 'none';
            albumItemsGrid.classList.remove('d-none');
            albumItemsGrid.style.display = 'grid';
            updateAlbumSelectionCount();
        }

        function handleStreamMessage(msg) {
            if (msg.type === 'meta') {
                streamedMeta = msg;
                if (msg.title) albumModalTitle.textContent = msg.title;
                if (liveStatusEl) liveStatusEl.textContent = 'Discovering videos from ' + (msg.title || plat.name) + '...';
            } else if (msg.type === 'item' && msg.entry) {
                streamedEntries.push(msg.entry);
                var count = msg.count || streamedEntries.length;
                if (liveCountEl) liveCountEl.textContent = count;
                if (stopBtnText) stopBtnText.textContent = 'Stop & Show Videos (' + count + ')';
                if (liveStatusEl) liveStatusEl.textContent = 'Found ' + count + ' videos with covers...';
            } else if (msg.type === 'done') {
                if (streamedEntries.length > 0) {
                    showDiscoveredVideos();
                } else {
                    renderScanError(cleanUrl, plat, msg.error || 'No videos found');
                }
            }
        }

        var cookies = localStorage.getItem('kv_user_cookies') || '';
        var sseUrl = '/api/scan/stream?url=' + encodeURIComponent(cleanUrl);
        if (cookies) sseUrl += '&cookies=' + encodeURIComponent(cookies);

        if (window.AbortController && window.ReadableStream) {
            scanAbortController = new AbortController();
            fetch(sseUrl, { signal: scanAbortController.signal })
                .then(function(response) {
                    if (!response.body) throw new Error('ReadableStream not supported');
                    var reader = response.body.getReader();
                    var decoder = new TextDecoder();
                    var buffer = '';

                    function readChunk() {
                        reader.read().then(function(result) {
                            if (result.done) {
                                if (streamedEntries.length > 0) {
                                    showDiscoveredVideos();
                                }
                                return;
                            }
                            buffer += decoder.decode(result.value, { stream: true });
                            var lines = buffer.split('\n');
                            buffer = lines.pop();

                            for (var i = 0; i < lines.length; i++) {
                                var line = lines[i].trim();
                                if (line.startsWith('data:')) {
                                    var jsonStr = line.substring(5).trim();
                                    if (jsonStr) {
                                        try {
                                            var msg = JSON.parse(jsonStr);
                                            handleStreamMessage(msg);
                                        } catch (err) { console.error(err); }
                                    }
                                }
                            }
                            readChunk();
                        }).catch(function(err) {
                            if (err.name === 'AbortError') return;
                            if (streamedEntries.length > 0) {
                                showDiscoveredVideos();
                            } else {
                                renderScanError(cleanUrl, plat, 'Scan interrupted');
                            }
                        });
                    }
                    readChunk();
                })
                .catch(function(err) {
                    if (err.name === 'AbortError') return;
                    if (streamedEntries.length > 0) {
                        showDiscoveredVideos();
                    } else {
                        renderScanError(cleanUrl, plat, err.message || 'Stream error');
                    }
                });
        } else {
            scanEventSource = new EventSource(sseUrl);
            scanEventSource.onmessage = function(event) {
                if (!event.data) return;
                try {
                    var msg = JSON.parse(event.data);
                    handleStreamMessage(msg);
                } catch (e) { console.error(e); }
            };
            scanEventSource.onerror = function() {
                if (scanEventSource) {
                    scanEventSource.close();
                    scanEventSource = null;
                }
                if (streamedEntries.length > 0) {
                    showDiscoveredVideos();
                } else {
                    renderScanError(cleanUrl, plat, 'Scan connection closed');
                }
            };
        }
    }

    function renderScanError(cleanUrl, plat, errorMsg) {
        albumModalLoading.innerHTML = '<div class="flex flex-col items-center gap-3 text-center py-6">' +
            '<i class="bi bi-exclamation-triangle-fill text-amber-400 text-[32px]"></i>' +
            '<p class="text-[13px] text-white font-medium">Could not extract video list from this channel.</p>' +
            '<p class="text-[11px] text-on-surface-variant max-w-sm">' + escapeHtml(errorMsg || 'The creator channel might have privacy restrictions or yt-dlp is rate limited.') + '</p>' +
            '<div class="flex items-center gap-2 mt-2">' +
            '<button type="button" class="btn-retry-scan btn-primary-glow px-4 py-1.5 rounded-xl text-[11px] font-bold">Retry Scan</button>' +
            '<button type="button" class="btn-force-dl px-4 py-1.5 rounded-xl bg-white/10 hover:bg-white/15 text-white text-[11px] font-semibold">Force Download URL</button>' +
            '</div>' +
            '</div>';
        var retryBtn = albumModalLoading.querySelector('.btn-retry-scan');
        if (retryBtn) retryBtn.addEventListener('click', function() { openAlbumModal(cleanUrl); });
        var forceBtn = albumModalLoading.querySelector('.btn-force-dl');
        if (forceBtn) forceBtn.addEventListener('click', function() {
            closeAlbumModal();
            enqueueUrls([cleanUrl], selectedFormat);
        });
    }

    function closeAlbumModal() {
        if (scanAbortController) {
            scanAbortController.abort();
            scanAbortController = null;
        }
        if (scanEventSource) {
            scanEventSource.close();
            scanEventSource = null;
        }
        albumModal.classList.add('d-none');
        albumModal.style.display = 'none';
        currentAlbumEntries = [];
        selectedAlbumIndices.clear();
        streamedEntries = [];
        streamedMeta = null;
    }

    if (btnCloseAlbumModal) btnCloseAlbumModal.addEventListener('click', closeAlbumModal);
    if (btnCancelAlbumModal) btnCancelAlbumModal.addEventListener('click', closeAlbumModal);
    if (albumModal) albumModal.addEventListener('click', function(e) { if (e.target === albumModal) closeAlbumModal(); });

    function renderAlbumEntries(entries) {
        albumItemsGrid.innerHTML = '';
        var query = albumFilterInput ? albumFilterInput.value.trim().toLowerCase() : '';

        entries.forEach(function(item, idx) {
            var title = item.title || ('Video #' + (idx + 1));
            if (query && title.toLowerCase().indexOf(query) === -1 && (item.id || '').toLowerCase().indexOf(query) === -1) {
                return;
            }
            if (currentAlbumCategory && currentAlbumCategory !== 'all') {
                var c = (item.category || '').toLowerCase();
                if (currentAlbumCategory === 'reels' && c.indexOf('reel') === -1) return;
                if (currentAlbumCategory === 'videos' && c.indexOf('video') === -1) return;
                if (currentAlbumCategory === 'posts' && c.indexOf('post') === -1) return;
            }

            var isChecked = selectedAlbumIndices.has(idx);
            var card = document.createElement('div');
            card.className = 'album-item-card';
            card.setAttribute('data-idx', idx);
            card.style.cssText = isChecked 
                ? 'background:rgba(160,120,255,.1);border:1px solid rgba(160,120,255,.45);border-radius:14px;overflow:hidden;cursor:pointer;transition:all .18s ease;display:flex;flex-direction:column;min-height:185px;'
                : 'background:rgba(255,255,255,.03);border:1px solid rgba(255,255,255,.07);border-radius:14px;overflow:hidden;cursor:pointer;transition:all .18s ease;display:flex;flex-direction:column;min-height:185px;';
            
            var thumb = item.thumbnail || '';
            if (!thumb && item.thumbnails && item.thumbnails.length > 0) {
                thumb = item.thumbnails[item.thumbnails.length - 1].url;
            }
            if (!thumb && (item.id || item.url)) {
                var vidId = item.id;
                if (!vidId || vidId.startsWith('http')) {
                    var m = (item.url || item.id).match(/(?:youtu\.be\/|youtube\.com\/(?:watch\?v=|shorts\/|embed\/))([a-zA-Z0-9_-]{11})/);
                    if (m && m[1]) vidId = m[1];
                }
                if (vidId && vidId.length === 11) {
                    thumb = 'https://i.ytimg.com/vi/' + vidId + '/hqdefault.jpg';
                }
            }
            if (!thumb) thumb = '/static/logo.svg';

            var durHtml = '';
            if (item.duration && item.duration > 0) {
                durHtml = '<span style="position:absolute;bottom:6px;right:6px;background:rgba(0,0,0,.8);backdrop-filter:blur(6px);padding:2px 6px;border-radius:6px;font-size:10px;font-weight:700;color:#38bdf8;font-family:monospace;">' + formatDuration(item.duration) + '</span>';
            }

            var catBadge = '';
            if (item.category) {
                var cLow = item.category.toLowerCase();
                if (cLow.indexOf('reel') !== -1) {
                    catBadge = '<span style="position:absolute;top:7px;left:7px;background:rgba(192,132,252,.35);color:#f3e8ff;border:1px solid rgba(192,132,252,.5);backdrop-filter:blur(6px);padding:2px 7px;border-radius:6px;font-size:10px;font-weight:700;letter-spacing:0.02em;z-index:2;">Reel</span>';
                } else if (cLow.indexOf('video') !== -1) {
                    catBadge = '<span style="position:absolute;top:7px;left:7px;background:rgba(56,189,248,.35);color:#e0f2fe;border:1px solid rgba(56,189,248,.5);backdrop-filter:blur(6px);padding:2px 7px;border-radius:6px;font-size:10px;font-weight:700;letter-spacing:0.02em;z-index:2;">Video</span>';
                } else if (cLow.indexOf('post') !== -1) {
                    catBadge = '<span style="position:absolute;top:7px;left:7px;background:rgba(96,165,250,.35);color:#dbeafe;border:1px solid rgba(96,165,250,.5);backdrop-filter:blur(6px);padding:2px 7px;border-radius:6px;font-size:10px;font-weight:700;letter-spacing:0.02em;z-index:2;">Post</span>';
                }
            }

            var isTikTok = (item.url && item.url.indexOf('tiktok.com') !== -1) || (currentAlbumScan && currentAlbumScan.title && currentAlbumScan.title.indexOf('TikTok') !== -1);
            var aspectStyle = isTikTok ? 'aspect-ratio:16/10;' : 'aspect-ratio:16/9;';

            card.innerHTML = '<div style="position:relative;width:100%;' + aspectStyle + 'background:#090b10;overflow:hidden;flex-shrink:0;">' +
                '<img src="' + escapeHtml(thumb) + '" alt="" style="width:100%;height:100%;object-fit:cover;display:block;" loading="lazy" referrerpolicy="no-referrer" onerror="if(!this.dataset.retry && \'' + escapeHtml(thumb) + '\'.indexOf(\'http\')===0){this.dataset.retry=\'1\';this.src=\'/thumbnail?url=\' + encodeURIComponent(\'' + escapeHtml(thumb) + '\');}else{this.src=\'/static/logo.svg\';}">' +
                catBadge +
                '<span style="position:absolute;bottom:6px;left:6px;background:rgba(0,0,0,.8);backdrop-filter:blur(6px);padding:2px 6px;border-radius:6px;font-size:10px;font-weight:700;color:#fff;font-family:monospace;">#' + (idx + 1) + '</span>' +
                durHtml +
                '<div style="position:absolute;top:7px;right:7px;width:24px;height:24px;border-radius:50%;background:#a078ff;display:flex;align-items:center;justify-content:center;box-shadow:0 2px 8px rgba(0,0,0,.6);">' +
                '<input type="checkbox" ' + (isChecked ? 'checked' : '') + ' data-idx="' + idx + '" style="accent-color:#a078ff;width:15px;height:15px;cursor:pointer;">' +
                '</div>' +
                '</div>' +
                '<div style="padding:10px 12px;flex:1;display:flex;flex-direction:column;justify-content:space-between;gap:4px;">' +
                '<h5 style="font-size:12px;font-weight:600;color:#f1f5f9;line-height:1.35;display:-webkit-box;-webkit-line-clamp:2;-webkit-box-orient:vertical;overflow:hidden;" title="' + escapeHtml(title) + '">' + escapeHtml(title) + '</h5>' +
                (item.uploader ? '<span style="font-size:10px;color:#94a3b8;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;">@' + escapeHtml(item.uploader) + '</span>' : '') +
                '</div>';

            card.addEventListener('click', function(e) {
                if (e.target.tagName !== 'INPUT') {
                    var chk = card.querySelector('input[type=checkbox]');
                    if (chk) chk.checked = !chk.checked;
                }
                var chkEl = card.querySelector('input[type=checkbox]');
                var checked = chkEl ? chkEl.checked : false;
                if (checked) {
                    selectedAlbumIndices.add(idx);
                } else {
                    selectedAlbumIndices.delete(idx);
                }
                card.style.borderColor = checked ? 'rgba(160,120,255,.45)' : 'rgba(255,255,255,.07)';
                card.style.background = checked ? 'rgba(160,120,255,.1)' : 'rgba(255,255,255,.03)';
                updateAlbumSelectionCount();
            });
            albumItemsGrid.appendChild(card);
        });
    }

    if (albumFilterInput) {
        albumFilterInput.addEventListener('input', function() {
            if (currentAlbumEntries && currentAlbumEntries.length > 0) {
                renderAlbumEntries(currentAlbumEntries);
                updateAlbumSelectionCount();
            }
        });
    }

    function updateAlbumSelectionCount() {
        var count = selectedAlbumIndices.size;
        if (albumSelectedCount) albumSelectedCount.textContent = count + ' of ' + currentAlbumEntries.length + ' selected';
        if (btnDownloadAlbumText) btnDownloadAlbumText.textContent = 'Download Selected (' + count + ')';
        if (btnDownloadAlbumSelected) btnDownloadAlbumSelected.disabled = (count === 0);
    }

    if (btnAlbumSelectAll) btnAlbumSelectAll.addEventListener('click', function() {
        var query = albumFilterInput ? albumFilterInput.value.trim().toLowerCase() : '';
        for (var i = 0; i < currentAlbumEntries.length; i++) {
            var item = currentAlbumEntries[i];
            var title = item.title || ('Video #' + (i + 1));
            if (query && title.toLowerCase().indexOf(query) === -1 && (item.id || '').toLowerCase().indexOf(query) === -1) {
                continue;
            }
            if (currentAlbumCategory && currentAlbumCategory !== 'all') {
                var c = (item.category || '').toLowerCase();
                if (currentAlbumCategory === 'reels' && c.indexOf('reel') === -1) continue;
                if (currentAlbumCategory === 'videos' && c.indexOf('video') === -1) continue;
                if (currentAlbumCategory === 'posts' && c.indexOf('post') === -1) continue;
            }
            selectedAlbumIndices.add(i);
        }
        renderAlbumEntries(currentAlbumEntries);
        updateAlbumSelectionCount();
    });

    if (btnAlbumDeselectAll) btnAlbumDeselectAll.addEventListener('click', function() {
        var query = albumFilterInput ? albumFilterInput.value.trim().toLowerCase() : '';
        for (var i = 0; i < currentAlbumEntries.length; i++) {
            var item = currentAlbumEntries[i];
            var title = item.title || ('Video #' + (i + 1));
            if (query && title.toLowerCase().indexOf(query) === -1 && (item.id || '').toLowerCase().indexOf(query) === -1) {
                continue;
            }
            if (currentAlbumCategory && currentAlbumCategory !== 'all') {
                var c = (item.category || '').toLowerCase();
                if (currentAlbumCategory === 'reels' && c.indexOf('reel') === -1) continue;
                if (currentAlbumCategory === 'videos' && c.indexOf('video') === -1) continue;
                if (currentAlbumCategory === 'posts' && c.indexOf('post') === -1) continue;
            }
            selectedAlbumIndices.delete(i);
        }
        renderAlbumEntries(currentAlbumEntries);
        updateAlbumSelectionCount();
    });

    if (btnDownloadAlbumSelected) btnDownloadAlbumSelected.addEventListener('click', function() {
        var urls = [];
        var items = [];
        selectedAlbumIndices.forEach(function(idx) {
            var entry = currentAlbumEntries[idx];
            if (entry && (entry.url || entry.id)) {
                var targetUrl = entry.url || entry.id;
                urls.push(targetUrl);
                var thumb = entry.thumbnail || (entry.thumbnails && entry.thumbnails.length ? entry.thumbnails[entry.thumbnails.length - 1].url : '');
                var ch = entry.channel || (currentAlbumScan && currentAlbumScan.channel) || (streamedMeta && (streamedMeta.channel || streamedMeta.uploader || streamedMeta.title)) || '';
                var up = entry.uploader || (currentAlbumScan && currentAlbumScan.uploader) || (streamedMeta && (streamedMeta.uploader || streamedMeta.channel)) || '';
                items.push({
                    url: targetUrl,
                    title: entry.title,
                    thumbnail: thumb,
                    uploader: up,
                    channel: ch
                });
            }
        });
        if (urls.length === 0) {
            showToast('Select at least 1 video', 'warning');
            return;
        }
        var fmt = albumFormatSelect.value || selectedFormat;
        var pMeta = {
            title: (currentAlbumScan && currentAlbumScan.title) || (streamedMeta && streamedMeta.title) || '',
            channel: (currentAlbumScan && (currentAlbumScan.channel || currentAlbumScan.uploader)) || (streamedMeta && (streamedMeta.channel || streamedMeta.uploader || streamedMeta.title)) || ''
        };
        closeAlbumModal();
        urlInput.value = '';
        updateInputDetection();
        enqueueUrls(urls, fmt, items, pMeta);
    });

    // Cookie Modal Logic
    function detectCookieFormat(raw) {
        raw = (raw || '').trim();
        if (!raw) return { name: 'Empty' };
        if (raw.startsWith('[') && raw.endsWith(']')) return { name: 'JSON Array' };
        if (raw.startsWith('{') && raw.endsWith('}')) return { name: 'JSON Object' };
        if (raw.indexOf('\t') !== -1 || raw.startsWith('# Netscape') || raw.startsWith('# HTTP Cookie')) return { name: 'Netscape .txt' };
        if (raw.indexOf('=') !== -1 && (raw.indexOf(';') !== -1 || raw.indexOf('\n') !== -1)) return { name: 'Cookie Header' };
        return { name: 'Raw String' };
    }

    function updateCookieBadge() {
        var saved = localStorage.getItem('kv_user_cookies') || '';
        var isSet = saved.trim().length > 0;
        if (cookieNavBadge) {
            cookieNavBadge.textContent = isSet ? 'Active' : 'Off';
            cookieNavBadge.className = isSet ? 'ml-auto text-[10px] px-2.5 py-0.5 rounded-full bg-emerald-500/20 text-emerald-300 border border-emerald-500/30' : 'ml-auto text-[10px] px-2.5 py-0.5 rounded-full bg-white/10 border border-white/10 text-on-surface-variant';
        }
        if (cookieDetectedFormat) {
            var fmt = detectCookieFormat(cookieInputArea ? cookieInputArea.value : '');
            cookieDetectedFormat.textContent = fmt.name;
        }
    }

    function openCookiesModal() {
        var saved = localStorage.getItem('kv_user_cookies') || '';
        if (cookieInputArea) cookieInputArea.value = saved;
        updateCookieBadge();
        if (cookiesModal) {
            cookiesModal.classList.remove('d-none');
            cookiesModal.style.display = 'flex';
        }
    }

    function closeCookiesModal() {
        if (cookiesModal) {
            cookiesModal.classList.add('d-none');
            cookiesModal.style.display = 'none';
        }
    }

    if (btnOpenCookiesNav) btnOpenCookiesNav.addEventListener('click', openCookiesModal);
    if (btnQuickCookie) btnQuickCookie.addEventListener('click', openCookiesModal);
    if (btnCloseCookiesModal) btnCloseCookiesModal.addEventListener('click', closeCookiesModal);
    if (btnCloseCookiesFooter) btnCloseCookiesFooter.addEventListener('click', closeCookiesModal);
    if (cookiesModal) cookiesModal.addEventListener('click', function(e) { if (e.target === cookiesModal) closeCookiesModal(); });

    if (cookieInputArea) {
        cookieInputArea.addEventListener('input', function() {
            var fmt = detectCookieFormat(this.value);
            if (cookieDetectedFormat) cookieDetectedFormat.textContent = fmt.name;
        });
    }

    if (btnSaveCookies) {
        btnSaveCookies.addEventListener('click', function() {
            var val = (cookieInputArea ? cookieInputArea.value : '').trim();
            if (val) {
                localStorage.setItem('kv_user_cookies', val);
                updateCookieBadge();
                showToast('Cookies saved securely to this device!', 'success');
                closeCookiesModal();
            } else {
                localStorage.removeItem('kv_user_cookies');
                updateCookieBadge();
                showToast('Cookies cleared', 'info');
            }
        });
    }

    if (btnClearSavedCookies) {
        btnClearSavedCookies.addEventListener('click', function() {
            localStorage.removeItem('kv_user_cookies');
            if (cookieInputArea) cookieInputArea.value = '';
            updateCookieBadge();
            showToast('Cookies cleared', 'info');
        });
    }

    if (btnPasteCookieClipboard) {
        btnPasteCookieClipboard.addEventListener('click', async function() {
            try {
                if (navigator.clipboard && navigator.clipboard.readText) {
                    var t = await navigator.clipboard.readText();
                    if (t && t.trim()) {
                        if (cookieInputArea) {
                            cookieInputArea.value = t.trim();
                            cookieInputArea.dispatchEvent(new Event('input'));
                        }
                        showToast('Pasted cookies', 'success');
                    } else showToast('Clipboard empty', 'warning');
                } else {
                    if (cookieInputArea) cookieInputArea.focus();
                    document.execCommand('paste');
                }
            } catch (e) {
                if (cookieInputArea) cookieInputArea.focus();
                showToast('Paste manually', 'info');
            }
        });
    }

    if (btnImportCookieFile && cookieFileInput) {
        btnImportCookieFile.addEventListener('click', function() { cookieFileInput.click(); });
        cookieFileInput.addEventListener('change', function(e) {
            var f = e.target.files && e.target.files[0];
            if (!f) return;
            var r = new FileReader();
            r.onload = function(evt) {
                var c = evt.target.result;
                if (c && cookieInputArea) {
                    cookieInputArea.value = c.trim();
                    cookieInputArea.dispatchEvent(new Event('input'));
                    showToast('Imported: ' + f.name, 'success');
                }
            };
            r.readAsText(f);
        });
    }

    if (btnCopyIosCode) {
        btnCopyIosCode.addEventListener('click', function() {
            var el = document.getElementById('iosBookmarkletCode');
            if (el && navigator.clipboard && navigator.clipboard.writeText) {
                navigator.clipboard.writeText(el.textContent.trim()).then(function() {
                    showToast('Bookmarklet copied to clipboard!', 'success');
                });
            }
        });
    }

    document.querySelectorAll('.cookie-tab-btn').forEach(function(btn) {
        btn.addEventListener('click', function() {
            document.querySelectorAll('.cookie-tab-btn').forEach(function(b) {
                b.classList.remove('active', 'bg-primary', 'text-black');
                b.classList.add('bg-white/5', 'text-on-surface-variant');
            });
            this.classList.add('active', 'bg-primary', 'text-black');
            this.classList.remove('bg-white/5', 'text-on-surface-variant');

            var tab = this.getAttribute('data-guide-tab');
            var pDesktop = document.getElementById('guidePaneDesktop');
            var pIos = document.getElementById('guidePaneIos');
            var pAndroid = document.getElementById('guidePaneAndroid');
            if (pDesktop) pDesktop.classList.toggle('d-none', tab !== 'desktop');
            if (pIos) pIos.classList.toggle('d-none', tab !== 'ios');
            if (pAndroid) pAndroid.classList.toggle('d-none', tab !== 'android');
        });
    });

    // DOM Elements - Video Theater Modal
    var videoModal = document.getElementById('videoModal');
    var videoModalTitle = document.getElementById('videoModalTitle');
    var videoModalPlatform = document.getElementById('videoModalPlatform');
    var videoModalSize = document.getElementById('videoModalSize');
    var videoModalDownloadBtn = document.getElementById('videoModalDownloadBtn');
    var btnCloseVideoModal = document.getElementById('btnCloseVideoModal');

    if (btnCloseVideoModal) {
        btnCloseVideoModal.addEventListener('click', function() {
            if (videoModal) videoModal.classList.add('d-none');
            if (globalVideoEngine) globalVideoEngine.pause();
            cleanupHls();
        });
    }
    if (videoModal) {
        videoModal.addEventListener('click', function(e) {
            if (e.target === videoModal) {
                videoModal.classList.add('d-none');
                if (globalVideoEngine) globalVideoEngine.pause();
                cleanupHls();
            }
        });
    }

    // =========================================================================
    // PWA & ANDROID LOCKSCREEN MEDIA SESSION CONTROLLER
    // =========================================================================
    function playMediaItem(task) {
        if (!task || (!task.mediaId && !task.url)) return;
        currentPlayingTask = task;

        var isAudio = (task.format && task.format.indexOf('audio') !== -1) || (task.mediaName && (task.mediaName.endsWith('.mp3') || task.mediaName.endsWith('.m4a')));

        // Prioritize local mediaId for playback (completed downloads)
        // Use task.url only for remote/streaming URLs that are actual download endpoints
        var hasLocalFile = task.mediaId && (task.mediaId.includes('/') || task.mediaId.includes('\\'));
        var streamUrl;
        var downloadUrl;

        if (hasLocalFile && !task.url) {
            // Pure local file — no remote URL
            streamUrl = '/download?id=' + encodeURIComponent(task.mediaId) + '&inline=true';
            downloadUrl = '/download?id=' + encodeURIComponent(task.mediaId);
        } else if (hasLocalFile && task.url && task.url.indexOf('/download') !== -1) {
            // task.url is a local download endpoint (e.g. from gallery)
            streamUrl = task.url.indexOf('inline=true') !== -1 ? task.url : (task.url + (task.url.indexOf('?') !== -1 ? '&' : '?') + 'inline=true');
            downloadUrl = task.url;
        } else if (hasLocalFile) {
            // task.url is remote URL (e.g. YouTube link), use local file
            streamUrl = '/download?id=' + encodeURIComponent(task.mediaId) + '&inline=true';
            downloadUrl = '/download?id=' + encodeURIComponent(task.mediaId);
        } else {
            // No local file — use remote/stream URL directly
            streamUrl = task.url.indexOf('inline=true') !== -1 ? task.url : (task.url + (task.url.indexOf('?') !== -1 ? '&' : '?') + 'inline=true');
            downloadUrl = task.url;
        }

        var thumbUrl = task.thumbnail || '/static/logo.svg';
        var title = task.title || task.mediaName || 'Media File';
        var plat = detectPlatform(task.url || '') || { name: 'KV Media' };

        // Setup active engine
        var engine = isAudio ? globalAudioEngine : globalVideoEngine;
        var other = isAudio ? globalVideoEngine : globalAudioEngine;
        other.pause();
        cleanupHls();

        engine.src = streamUrl;
        engine.play().then(function() {
            updatePlayerBarState(true);
        }).catch(function(err) {
            console.warn('Playback interrupted:', err);
        });

        // Show floating player bar
        if (playerBarTitle) playerBarTitle.textContent = title;
        if (playerBarSubtitle) playerBarSubtitle.textContent = plat.name + ' • ' + (task.humanSize || 'Direct');
        if (playerBarThumb) playerBarThumb.src = thumbUrl;
        if (androidPlayerBar) androidPlayerBar.classList.add('active');

        // Open Video Theater Modal for video files so the user can watch the video visuals
        if (!isAudio && videoModal) {
            if (videoModalTitle) videoModalTitle.textContent = title;
            if (videoModalPlatform) videoModalPlatform.textContent = plat.name || 'KV Video';
            if (videoModalSize) videoModalSize.textContent = task.humanSize || 'Direct Stream';
            if (videoModalDownloadBtn) {
                videoModalDownloadBtn.href = downloadUrl;
                videoModalDownloadBtn.setAttribute('download', task.mediaName || task.title || 'video.mp4');
            }
            videoModal.classList.remove('d-none');
        }

        // Android / PWA MediaSession API
        if ('mediaSession' in navigator) {
            navigator.mediaSession.metadata = new MediaMetadata({
                title: title,
                artist: plat.name || 'KV Download',
                album: 'KV Pro Media Gallery',
                artwork: [
                    { src: thumbUrl, sizes: '96x96', type: 'image/png' },
                    { src: thumbUrl, sizes: '256x256', type: 'image/png' },
                    { src: thumbUrl, sizes: '512x512', type: 'image/png' }
                ]
            });

            navigator.mediaSession.setActionHandler('play', function() { engine.play(); });
            navigator.mediaSession.setActionHandler('pause', function() { engine.pause(); });
            navigator.mediaSession.setActionHandler('seekbackward', function(details) {
                engine.currentTime = Math.max(0, engine.currentTime - (details.seekOffset || 10));
            });
            navigator.mediaSession.setActionHandler('seekforward', function(details) {
                engine.currentTime = Math.min(engine.duration || 0, engine.currentTime + (details.seekOffset || 10));
            });
            navigator.mediaSession.setActionHandler('seekto', function(details) {
                if (details.seekTime && engine.duration) {
                    engine.currentTime = details.seekTime;
                }
            });
            navigator.mediaSession.setActionHandler('previoustrack', function() { playNextPrevHistory(-1); });
            navigator.mediaSession.setActionHandler('nexttrack', function() { playNextPrevHistory(1); });
        }
    }

    function playNextPrevHistory(offset) {
        var completed = Object.values(tasksMap).filter(function(t) { return t.status === 'completed' && t.mediaId; });
        if (completed.length <= 1 || !currentPlayingTask) return;
        var idx = completed.findIndex(function(t) { return t.id === currentPlayingTask.id; });
        if (idx !== -1) {
            var nextIdx = (idx + offset + completed.length) % completed.length;
            playMediaItem(completed[nextIdx]);
        }
    }

    function updatePlayerBarState(isPlaying) {
        if (playerBarPlayIcon) {
            playerBarPlayIcon.className = isPlaying ? 'bi bi-pause-fill' : 'bi bi-play-fill';
        }
        if (playerBarWave) {
            playerBarWave.classList.toggle('d-none', !isPlaying);
        }
        if ('mediaSession' in navigator) {
            navigator.mediaSession.playbackState = isPlaying ? 'playing' : 'paused';
        }
    }

    // Engine Event Listeners
    [globalAudioEngine, globalVideoEngine].forEach(function(engine) {
        engine.addEventListener('play', function() { updatePlayerBarState(true); });
        engine.addEventListener('pause', function() { updatePlayerBarState(false); });
        engine.addEventListener('timeupdate', function() {
            if (playerBarCurrentTime) playerBarCurrentTime.textContent = formatTime(engine.currentTime);
            if (playerBarDuration) playerBarDuration.textContent = formatTime(engine.duration);
            if (playerBarScrubber && engine.duration) {
                playerBarScrubber.value = (engine.currentTime / engine.duration) * 100;
            }
            if ('mediaSession' in navigator && engine.duration && !isNaN(engine.duration)) {
                try {
                    navigator.mediaSession.setPositionState({
                        duration: engine.duration,
                        playbackRate: engine.playbackRate || 1,
                        position: engine.currentTime
                    });
                } catch (e) {}
            }
        });
        engine.addEventListener('ended', function() {
            playNextPrevHistory(1);
        });
    });

    if (btnPlayerPlayPause) {
        btnPlayerPlayPause.addEventListener('click', function() {
            var engine = globalAudioEngine.src ? globalAudioEngine : globalVideoEngine;
            if (engine.paused) engine.play(); else engine.pause();
        });
    }
    if (btnPlayerRewind) {
        btnPlayerRewind.addEventListener('click', function() {
            var engine = globalAudioEngine.src ? globalAudioEngine : globalVideoEngine;
            engine.currentTime = Math.max(0, engine.currentTime - 10);
        });
    }
    if (btnPlayerForward) {
        btnPlayerForward.addEventListener('click', function() {
            var engine = globalAudioEngine.src ? globalAudioEngine : globalVideoEngine;
            engine.currentTime = Math.min(engine.duration || 0, engine.currentTime + 10);
        });
    }
    if (playerBarScrubber) {
        playerBarScrubber.addEventListener('input', function() {
            var engine = globalAudioEngine.src ? globalAudioEngine : globalVideoEngine;
            if (engine.duration) {
                engine.currentTime = (parseFloat(this.value) / 100) * engine.duration;
            }
        });
    }
    if (btnPlayerClose) {
        btnPlayerClose.addEventListener('click', function() {
            // Hide the mini player bar but KEEP PLAYING in background
            // This allows audio to continue playing like a music player
            if (androidPlayerBar) androidPlayerBar.classList.remove('active');
        });
    }

    // Task Actions: Cancel / Retry / Delete
    function cancelTask(id) {
        var x = new XMLHttpRequest();
        x.open('POST', '/api/queue/cancel?id=' + encodeURIComponent(id), true);
        x.onload = function() { showToast('Task cancelled', 'info'); };
        x.send();
    }

    function retryTask(id) {
        var x = new XMLHttpRequest();
        x.open('POST', '/api/queue/retry?id=' + encodeURIComponent(id), true);
        x.onload = function() { showToast('Task re-enqueued', 'info'); };
        x.send();
    }

    function deleteTask(id, deleteFile) {
        var x = new XMLHttpRequest();
        x.open('DELETE', '/api/queue/item?id=' + encodeURIComponent(id) + '&deleteFile=' + (deleteFile ? 'true' : 'false'), true);
        x.onload = function() { showToast('Task removed', 'info'); };
        x.send();
    }

    taskList.addEventListener('click', function(e) {
        var c = e.target.closest('.btn-action-cancel'); if (c) { cancelTask(c.getAttribute('data-id')); return; }
        var r = e.target.closest('.btn-action-retry'); if (r) { retryTask(r.getAttribute('data-id')); return; }
        var d = e.target.closest('.btn-action-dismiss') || e.target.closest('.btn-action-delete');
        if (d) {
            // In Download Center: ONLY clear/dismiss from view, NEVER delete file on disk
            deleteTask(d.getAttribute('data-id'), false);
            return;
        }
        var p = e.target.closest('.btn-action-play'); if (p) {
            var t = tasksMap[p.getAttribute('data-id')];
            if (t) playMediaItem(t);
            return;
        }
    });

    historyList.addEventListener('click', function(e) {
        var d = e.target.closest('.btn-action-delete') || e.target.closest('.btn-action-delete-file');
        if (d) {
            var fileId = d.getAttribute('data-id');
            var item = (galleryData.items || []).find(function(it) { return it.id === fileId; });
            var task = tasksMap[fileId];
            var name = (item && (item.title || item.name)) || (task && (task.title || task.mediaName)) || fileId.split('/').pop() || 'this media file';
            if (confirm('Delete "' + name + '" completely from disk?')) {
                var xhr = new XMLHttpRequest();
                xhr.open('DELETE', '/api/gallery/file?path=' + encodeURIComponent(fileId), true);
                xhr.onload = function() {
                    if (xhr.status === 200) {
                        showToast('File permanently deleted from disk', 'info');
                        if (tasksMap[fileId]) {
                            delete tasksMap[fileId];
                            renderTasks();
                        }
                        fetchGallery();
                    } else {
                        showToast('Failed to delete file', 'error');
                    }
                };
                xhr.send();
            }
            return;
        }
        var p = e.target.closest('.btn-action-play'); if (p) {
            var fileId = p.getAttribute('data-id');
            var item = (galleryData.items || []).find(function(it) { return it.id === fileId; });
            if (item) {
                playMediaItem({
                    id: item.id,
                    title: item.title || item.name,
                    mediaName: item.name,
                    channel: item.channel,
                    humanSize: item.humanSize,
                    mediaId: item.id,
                    url: item.url,
                    thumbnail: item.thumbnail,
                    format: item.type === 'audio' ? 'audio' : 'video'
                });
            } else if (tasksMap[fileId]) {
                playMediaItem(tasksMap[fileId]);
            }
            return;
        }
    });

    document.getElementById('btnClearCompleted').addEventListener('click', function() {
        var done = Object.values(tasksMap).filter(function(t) { return t.status === 'completed'; });
        if (done.length === 0) { showToast('No completed tasks to clear', 'info'); return; }
        var x = new XMLHttpRequest();
        x.open('POST', '/api/queue/clear-completed', true);
        x.onload = function() {
            if (x.status === 200) {
                done.forEach(function(t) { delete tasksMap[t.id]; });
                renderTasks();
                showToast('Cleared ' + done.length + ' completed tasks', 'success');
            }
        };
        x.send();
    });

    document.getElementById('btnRetryFailed').addEventListener('click', function() {
        var failed = Object.values(tasksMap).filter(function(t) { return t.status === 'failed' || t.status === 'cancelled'; });
        if (failed.length === 0) { showToast('No failed tasks', 'info'); return; }
        var x = new XMLHttpRequest();
        x.open('POST', '/api/queue/retry-failed', true);
        x.onload = function() {
            if (x.status === 200) {
                failed.forEach(function(t) { t.status = 'queued'; t.percent = 0; });
                renderTasks();
                showToast('Retrying ' + failed.length + ' tasks', 'info');
            }
        };
        x.send();
    });

    var btnQueueStartAll = document.getElementById('btnQueueStartAll');
    if (btnQueueStartAll) {
        btnQueueStartAll.addEventListener('click', function() { document.getElementById('btnRetryFailed').click(); });
    }

    // Filter switching
    function setFilter(f) {
        currentFilter = f;
        document.querySelectorAll('.task-tab-btn').forEach(function(btn) {
            var active = btn.getAttribute('data-filter') === f;
            btn.classList.toggle('active', active);
            if (active) {
                btn.className = 'task-tab-btn active px-3.5 py-1.5 rounded-full text-[12px] font-semibold whitespace-nowrap bg-primary text-black transition-all';
            } else {
                btn.className = 'task-tab-btn px-3.5 py-1.5 rounded-full text-[12px] font-semibold whitespace-nowrap bg-white/5 text-on-surface-variant hover:text-white transition-all';
            }
        });
        renderTasks();
    }

    document.querySelectorAll('.task-tab-btn').forEach(function(tab) {
        tab.addEventListener('click', function() { setFilter(this.getAttribute('data-filter')); });
    });

    // Theme Switcher & Real-time System Sync (System / Dark / Light)
    function getSystemTheme() {
        return (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches) ? 'dark' : 'light';
    }

    function getEffectiveTheme(mode) {
        if (mode === 'system') return getSystemTheme();
        return mode === 'light' ? 'light' : 'dark';
    }

    function applyTheme(mode) {
        if (!mode || (mode !== 'system' && mode !== 'dark' && mode !== 'light')) {
            mode = 'system';
        }
        currentThemeMode = mode;
        localStorage.setItem('kv_theme_mode', mode);

        var effective = getEffectiveTheme(mode);

        // Apply classes to HTML and body
        if (effective === 'light') {
            document.documentElement.classList.remove('dark');
            document.documentElement.classList.add('light');
            document.body.classList.remove('theme-dark', 'theme-simple');
            document.body.classList.add('theme-light');
        } else {
            document.documentElement.classList.remove('light');
            document.documentElement.classList.add('dark');
            document.body.classList.remove('theme-light');
            document.body.classList.add('theme-dark', 'theme-simple');
        }

        // Update pills in Settings modal
        document.querySelectorAll('.theme-pill-btn').forEach(function(b) {
            var th = b.getAttribute('data-theme');
            var isMatch = th === mode;
            b.classList.toggle('active', isMatch);
            if (isMatch) {
                b.className = 'theme-pill-btn active px-3 py-1.5 rounded-lg text-[12px] font-semibold bg-primary text-black shadow-sm transition-all';
            } else {
                b.className = 'theme-pill-btn px-3 py-1.5 rounded-lg text-[12px] font-semibold text-on-surface-variant hover:text-white transition-all';
            }
        });

        // Update Top Navigation theme toggle button
        if (themeToggleBtn) {
            if (mode === 'system') {
                themeToggleBtn.innerHTML = effective === 'light'
                    ? '<span class="material-symbols-outlined text-[18px]">light_mode</span>'
                    : '<span class="material-symbols-outlined text-[18px]">dark_mode</span>';
                themeToggleBtn.title = 'Theme: System (' + (effective === 'light' ? 'Light' : 'Dark') + ') — Click to switch to ' + (effective === 'light' ? 'Dark' : 'Light');
            } else if (effective === 'light') {
                themeToggleBtn.innerHTML = '<span class="material-symbols-outlined text-[18px] text-amber-500">light_mode</span>';
                themeToggleBtn.title = 'Theme: Light — Click to switch to Dark';
            } else {
                themeToggleBtn.innerHTML = '<span class="material-symbols-outlined text-[18px] text-indigo-300">dark_mode</span>';
                themeToggleBtn.title = 'Theme: Dark — Click to switch to Light';
            }
        }

        // Update dynamically created elements for the new theme
        updateTaskCardsForTheme();
    }

    // Update dynamically created task cards when theme changes
    function updateTaskCardsForTheme() {
        var taskCards = document.querySelectorAll('.task-card');
        var isLight = document.body.classList.contains('theme-light');

        taskCards.forEach(function(card) {
            var titles = card.querySelectorAll('h4');
            titles.forEach(function(title) {
                if (isLight) {
                    title.style.color = '#0f172a';
                } else {
                    title.style.color = '#ffffff';
                }
            });

            var subtitles = card.querySelectorAll('span.text-on-surface-variant, span.font-mono');
            subtitles.forEach(function(sub) {
                if (isLight) {
                    sub.style.color = '#475569';
                }
            });

            var progressBars = card.querySelectorAll('.h-1, .h-1\\.5');
            progressBars.forEach(function(bar) {
                if (isLight) {
                    bar.style.backgroundColor = 'rgba(0, 0, 0, 0.08)';
                } else {
                    bar.style.backgroundColor = '';
                }
            });

            var statusBadges = card.querySelectorAll('[style*="background:"][style*="color:"]');
            statusBadges.forEach(function(badge) {
                var style = badge.style;
                if (isLight && style.color && style.color.includes('255')) {
                    style.color = '#0f172a';
                }
            });
        });

        // Update Android gallery cards
        var galleryCards = document.querySelectorAll('.android-gallery-card');
        galleryCards.forEach(function(card) {
            var titles = card.querySelectorAll('h4');
            titles.forEach(function(title) {
                if (isLight) {
                    title.style.color = '#0f172a';
                }
            });
        });

        // Update browser wait screen elements
        var waitScreen = document.getElementById('browserWaitScreen');
        if (waitScreen) {
            var title = waitScreen.querySelector('h3');
            var subtitle = waitScreen.querySelector('p');
            if (title) {
                title.style.color = isLight ? '#0f172a' : '#ffffff';
            }
            if (subtitle) {
                subtitle.style.color = isLight ? '#475569' : '#94a3b8';
            }

            // Wait screen badges and labels
            var badges = waitScreen.querySelectorAll('span, div');
            badges.forEach(function(el) {
                var style = window.getComputedStyle(el);
                if (style.color && style.color.includes('255, 255, 255') && !style.color.includes('255, 255, 255, 0')) {
                    el.style.color = isLight ? '#0f172a' : '#ffffff';
                }
                if (style.color && style.color.includes('255, 255, 255, 0.')) {
                    el.style.color = isLight ? '#475569' : style.color;
                }
            });
        }

        // Update media sniffer drawer elements
        var mediaDrawer = document.getElementById('browserMediaDrawer');
        if (mediaDrawer && !mediaDrawer.classList.contains('hidden')) {
            var drawerTitles = mediaDrawer.querySelectorAll('h4');
            drawerTitles.forEach(function(title) {
                title.style.color = isLight ? '#0f172a' : '#ffffff';
            });

            var drawerTexts = mediaDrawer.querySelectorAll('p, span.text-on-surface-variant, span.text-on-surface-variant\/50');
            drawerTexts.forEach(function(el) {
                el.style.color = isLight ? '#475569' : '#94a3b8';
            });

            var drawerFilters = mediaDrawer.querySelectorAll('.drawer-filter');
            drawerFilters.forEach(function(filter) {
                if (filter.classList.contains('active')) {
                    filter.style.color = isLight ? '#7c3aed' : '#d0bcff';
                } else {
                    filter.style.color = isLight ? '#475569' : '#94a3b8';
                }
            });

            var drawerIcons = mediaDrawer.querySelectorAll('.bi, .material-symbols-outlined');
            if (isLight) {
                drawerIcons.forEach(function(icon) {
                    if (icon.closest('.text-primary')) {
                        icon.style.color = '#7c3aed';
                    }
                });
            }
        }

        // Update browser view buttons and navigation
        var browserView = document.getElementById('view-browser');
        if (browserView) {
            var browserBtns = browserView.querySelectorAll('.bg-white\/5, .bg-white\/10, .bg-white\/15');
            browserBtns.forEach(function(btn) {
                if (btn.classList.contains('bg-white\/5')) {
                    btn.style.backgroundColor = isLight ? 'rgba(0, 0, 0, 0.04)' : '';
                }
                if (btn.classList.contains('bg-white\/10')) {
                    btn.style.backgroundColor = isLight ? 'rgba(0, 0, 0, 0.06)' : '';
                }
                if (btn.classList.contains('bg-white\/15')) {
                    btn.style.backgroundColor = isLight ? 'rgba(0, 0, 0, 0.08)' : '';
                }
            });

            var browserTexts = browserView.querySelectorAll('.text-on-surface-variant');
            browserTexts.forEach(function(el) {
                el.style.color = isLight ? '#475569' : '#94a3b8';
            });

            var browserInput = browserView.querySelector('#browserUrlInput');
            if (browserInput) {
                browserInput.style.backgroundColor = isLight ? '#ffffff' : '#090b10';
                browserInput.style.color = isLight ? '#0f172a' : '#ffffff';
                browserInput.style.borderColor = isLight ? 'rgba(0, 0, 0, 0.15)' : 'rgba(255, 255, 255, 0.1)';
            }

            var viewport = browserView.querySelector('.bg-\[\#0c0e14\]');
            if (viewport) {
                viewport.style.backgroundColor = isLight ? '#f1f5f9' : '#0c0e14';
                viewport.style.borderColor = isLight ? 'rgba(0, 0, 0, 0.1)' : 'rgba(255, 255, 255, 0.1)';
            }
        }

        // Update browser bubble badge border
        var bubbleBadge = document.getElementById('browserBubbleBadge');
        if (bubbleBadge) {
            bubbleBadge.style.borderColor = isLight ? '#ffffff' : '#0a0b0d';
        }

        // Update drawer filter tabs background
        var filterTabs = document.querySelector('.drawer-filter');
        if (filterTabs) {
            var parentTab = filterTabs.closest('.bg-black\/20');
            if (parentTab) {
                parentTab.style.backgroundColor = isLight ? 'rgba(0, 0, 0, 0.04)' : '';
                parentTab.style.borderColor = isLight ? 'rgba(0, 0, 0, 0.06)' : '';
            }
        }

        // Update drawer footer
        var drawerFooter = document.querySelector('#browserMediaDrawer .p-3.border-t');
        if (drawerFooter) {
            drawerFooter.style.backgroundColor = isLight ? 'rgba(0, 0, 0, 0.06)' : '';
            drawerFooter.style.borderColor = isLight ? 'rgba(0, 0, 0, 0.08)' : '';
        }

        // Update drawer media list placeholder text
        var drawerMediaList = document.getElementById('drawerMediaList');
        if (drawerMediaList) {
            var placeholderTexts = drawerMediaList.querySelectorAll('p, span');
            placeholderTexts.forEach(function(el) {
                var style = window.getComputedStyle(el);
                if (style.color && style.color.includes('255, 255, 255, 0.')) {
                    el.style.color = isLight ? '#475569' : style.color;
                }
            });
        }
    }

    // Reactive listener for OS system theme changes
    if (window.matchMedia) {
        var darkMatcher = window.matchMedia('(prefers-color-scheme: dark)');
        var onSystemThemeChange = function() {
            if (currentThemeMode === 'system') {
                applyTheme('system');
            }
        };
        if (darkMatcher.addEventListener) {
            darkMatcher.addEventListener('change', onSystemThemeChange);
        } else if (darkMatcher.addListener) {
            darkMatcher.addListener(onSystemThemeChange);
        }
    }

    document.querySelectorAll('.theme-pill-btn').forEach(function(btn) {
        btn.addEventListener('click', function() {
            var th = this.getAttribute('data-theme');
            applyTheme(th);
            var label = th === 'system' ? 'System (' + getSystemTheme() + ')' : (th === 'light' ? 'Light' : 'Dark');
            showToast('Theme set to ' + label, 'info');
        });
    });

    if (themeToggleBtn) {
        themeToggleBtn.addEventListener('click', function() {
            var effective = getEffectiveTheme(currentThemeMode);
            var nextTheme = effective === 'dark' ? 'light' : 'dark';
            applyTheme(nextTheme);
            showToast('Theme switched to ' + (nextTheme === 'light' ? 'Light' : 'Dark'), 'info');
        });
    }

    // Notifications Toggle
    if (notifToggle) {
        notifToggle.addEventListener('click', function() {
            var on = this.getAttribute('aria-checked') === 'true';
            var next = !on;
            this.setAttribute('aria-checked', String(next));
            this.style.background = next ? '#a078ff' : 'rgba(255,255,255,.15)';
            var knob = this.querySelector('.notif-knob');
            if (knob) { knob.style.transform = next ? 'translateX(20px)' : 'translateX(2px)'; }
            if (next && 'Notification' in window && Notification.permission === 'default') {
                Notification.requestPermission();
            }
            showToast(next ? 'Notifications enabled' : 'Notifications muted', 'info');
            try { localStorage.setItem('kv_notif', next ? '1' : '0'); } catch (e) {}
        });
        try {
            var savedNotif = localStorage.getItem('kv_notif');
            if (savedNotif === '0') {
                notifToggle.setAttribute('aria-checked', 'false');
                notifToggle.style.background = 'rgba(255,255,255,.15)';
                var k = notifToggle.querySelector('.notif-knob');
                if (k) k.style.transform = 'translateX(2px)';
            }
        } catch (e) {}
    }

    // Telemetry & Counts update
    function updateTelemetryAndCounts(all) {
        var cAll = all.length;
        var cActive = all.filter(function(t) { return t.status === 'downloading'; }).length;
        var cQueued = all.filter(function(t) { return t.status === 'queued'; }).length;
        var cCompleted = all.filter(function(t) { return t.status === 'completed'; }).length;
        var cFailed = all.filter(function(t) { return t.status === 'failed' || t.status === 'cancelled'; }).length;

        var set = function(id, val) { var el = document.getElementById(id); if (el) el.textContent = val; };
        set('countAll', cAll); set('countActive', cActive); set('countQueued', cQueued); set('countCompleted', cCompleted); set('countFailed', cFailed);
        set('dockCountAll', cAll); set('dockCountActive', cActive); set('dockCountCompleted', cCompleted); set('dockCountFailed', cFailed);
        set('statDownloading', cActive); set('statCompleted', cCompleted); set('statQueued', cQueued); set('statSaved', cAll);
        set('telemetryActive', cActive + ' active'); set('telemetryQueued', cQueued + ' pending'); set('telemetryCompleted', cCompleted + ' done');
        set('queueActiveCount', cActive); set('queuePendingCount', cQueued); set('queueDoneCount', cCompleted);
        set('histCountAll', cCompleted);

        var speeds = [];
        all.forEach(function(t) { if (t.status === 'downloading' && t.speed) speeds.push(t.speed); });
        var spd = speeds.length > 0 ? speeds[0] : '0 KB/s';
        set('telemetrySpeed', spd); set('navGlobalSpeed', spd); set('footerSpeed', spd);
    }

    // Render Tasks across Download Center, History, and Queues
    function renderTasks() {
        var all = Object.values(tasksMap);
        updateTelemetryAndCounts(all);

        // Render Download Center Tasks
        var filteredCenter = all.filter(function(t) {
            if (currentFilter === 'all') return true;
            if (currentFilter === 'downloading') return t.status === 'downloading';
            if (currentFilter === 'queued') return t.status === 'queued';
            if (currentFilter === 'completed') return t.status === 'completed';
            if (currentFilter === 'failed') return t.status === 'failed' || t.status === 'cancelled';
            return true;
        });
        filteredCenter.sort(function(a, b) { return new Date(b.createdAt) - new Date(a.createdAt); });

        renderTaskGrid(taskList, filteredCenter, currentViewMode, false);

        // Render Gallery Items directly from disk
        renderGalleryItems();

        // Render Playlists & Channel Collections
        renderPlaylists();

        // Render Queues
        if (queueList) {
            var activeQueueList = all.filter(function(t) { return t.status === 'downloading' || t.status === 'queued'; });
            renderTaskGrid(queueList, activeQueueList, 'standard', false);
        }

        // Update theme-aware colors for dynamically created elements
        updateTaskCardsForTheme();
    }

    function renderPlaylists() {
        var container = document.getElementById('playlistsContainer');
        if (!container) return;
        var all = Object.values(tasksMap);
        var completedList = all.filter(function(t) { return t.status === 'completed'; });

        // Group completed items by collection/channel/playlist
        var groups = {};
        completedList.forEach(function(t) {
            var groupKey = '';
            var groupTitle = '';
            var channelName = t.channel || t.uploader || '';

            if (t.playlistTitle) {
                groupKey = 'pl:' + t.playlistTitle;
                groupTitle = t.playlistTitle;
            } else if (t.channel) {
                groupKey = 'ch:' + t.channel;
                groupTitle = t.channel + ' (Channel)';
            } else if (t.uploader) {
                groupKey = 'up:' + t.uploader;
                groupTitle = '@' + t.uploader + ' (Creator)';
            } else if (t.url) {
                var ttM = t.url.match(/tiktok\.com\/@([a-zA-Z0-9_.]+)/);
                if (ttM && ttM[1]) {
                    groupKey = 'tt:@' + ttM[1];
                    groupTitle = '@' + ttM[1] + ' (TikTok)';
                    channelName = '@' + ttM[1];
                }
            }

            if (!groupKey) {
                groupKey = 'single:media';
                groupTitle = 'Individual Downloads';
            }

            if (!groups[groupKey]) {
                groups[groupKey] = {
                    key: groupKey,
                    title: groupTitle,
                    channel: channelName,
                    items: [],
                    totalBytes: 0,
                    platform: detectPlatform(t.url) || { name: 'Media', icon: 'bi-collection-play-fill', color: '#a078ff' },
                    thumbnails: []
                };
            }
            groups[groupKey].items.push(t);
            groups[groupKey].totalBytes += (t.totalBytes || 0);
            if (t.thumbnail && groups[groupKey].thumbnails.length < 4) {
                groups[groupKey].thumbnails.push(t.thumbnail);
            }
        });

        var groupList = Object.values(groups);
        if (groupList.length === 0) {
            container.innerHTML = '<div class="glass-panel p-8 flex flex-col items-center justify-center text-center col-span-full">' +
                '<div class="w-14 h-14 rounded-2xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary mb-3">' +
                '<span class="material-symbols-outlined text-[28px]">playlist_play</span>' +
                '</div>' +
                '<h4 class="text-[15px] font-bold text-white">No Channel or Playlist Collections Yet</h4>' +
                '<p class="text-[12px] text-on-surface-variant mt-1 max-w-md">When you download videos from a TikTok channel (e.g. <code>https://www.tiktok.com/@username</code>), YouTube playlist, or album, they will be organized into collections here with 1-click Play All and batch management.</p>' +
                '<button type="button" class="btn-primary-glow px-4 py-2 rounded-xl text-[12px] font-bold mt-4 flex items-center gap-1.5" onclick="document.querySelector(\'[data-view=download-center]\').click()"><i class="bi bi-plus-circle-fill"></i> Download Channel or Playlist</button>' +
                '</div>';
            return;
        }

        container.innerHTML = '';
        groupList.forEach(function(g) {
            var card = document.createElement('div');
            card.className = 'glass-panel p-4 rounded-2xl border border-white/10 flex flex-col justify-between gap-3 relative overflow-hidden';
            
            // Multi-thumbnail preview header
            var thumbsHtml = '';
            if (g.thumbnails.length > 0) {
                thumbsHtml = '<div class="grid ' + (g.thumbnails.length > 1 ? 'grid-cols-2' : 'grid-cols-1') + ' gap-1 rounded-xl overflow-hidden aspect-video bg-black/40 border border-white/5 relative">';
                g.thumbnails.slice(0, 4).forEach(function(th) {
                    thumbsHtml += '<img src="' + escapeHtml(th) + '" class="w-full h-full object-cover" referrerpolicy="no-referrer" onerror="this.src=\'/static/logo.svg\'">';
                });
                thumbsHtml += '<div class="absolute bottom-2 right-2 px-2 py-0.5 rounded-md bg-black/75 backdrop-blur-md text-[10px] font-mono text-cyan-300 font-bold">' + g.items.length + ' Videos</div>';
                thumbsHtml += '</div>';
            } else {
                thumbsHtml = '<div class="w-full aspect-video rounded-xl bg-black/40 border border-white/5 flex items-center justify-center text-on-surface-variant/40"><i class="bi ' + g.platform.icon + ' text-[32px]"></i></div>';
            }

            var humanSize = formatBytes(g.totalBytes);

            card.innerHTML = thumbsHtml +
                '<div class="flex flex-col gap-1 min-w-0">' +
                '<div class="flex items-center gap-1.5">' +
                '<span class="text-[10px] px-2 py-0.5 rounded-full bg-white/5 text-white font-bold flex items-center gap-1"><i class="bi ' + g.platform.icon + '"></i> ' + g.platform.name + '</span>' +
                '<span class="text-[10px] text-on-surface-variant font-mono">' + humanSize + '</span>' +
                '</div>' +
                '<h4 class="text-[14px] font-bold text-white truncate" title="' + escapeHtml(g.title) + '">' + escapeHtml(g.title) + '</h4>' +
                (g.channel ? '<p class="text-[11px] text-on-surface-variant truncate">Channel: ' + escapeHtml(g.channel) + '</p>' : '') +
                '</div>' +
                '<div class="pt-2 border-t border-white/5 flex items-center justify-between gap-2">' +
                '<button type="button" class="btn-play-collection btn-primary-glow px-3.5 py-1.5 rounded-xl text-[11px] font-bold flex items-center gap-1.5" data-key="' + escapeHtml(g.key) + '"><i class="bi bi-play-fill text-[14px]"></i> Play All (' + g.items.length + ')</button>' +
                '<button type="button" class="btn-inspect-collection px-3 py-1.5 rounded-xl bg-white/5 hover:bg-white/10 text-white text-[11px] font-semibold flex items-center gap-1" data-key="' + escapeHtml(g.key) + '"><i class="bi bi-collection"></i> View (' + g.items.length + ')</button>' +
                '</div>';

            container.appendChild(card);
        });
    }

    document.addEventListener('click', function(e) {
        var playBtn = e.target.closest('.btn-play-collection');
        if (playBtn) {
            var key = playBtn.getAttribute('data-key');
            var all = Object.values(tasksMap).filter(function(t) { return t.status === 'completed' && t.mediaId; });
            var matched = all.filter(function(t) {
                return (t.playlistTitle && 'pl:' + t.playlistTitle === key) ||
                       (t.channel && 'ch:' + t.channel === key) ||
                       (t.uploader && 'up:' + t.uploader === key) ||
                       (key.startsWith('tt:') && t.url && t.url.indexOf(key.replace('tt:', '')) !== -1) ||
                       (key === 'single:media');
            });
            if (matched.length > 0) {
                playMediaItem(matched[0]);
                showToast('Playing collection (' + matched.length + ' videos)', 'info');
            }
            return;
        }

        var inspectBtn = e.target.closest('.btn-inspect-collection');
        if (inspectBtn) {
            var key = inspectBtn.getAttribute('data-key');
            var cleanKey = key.replace(/^(pl:|ch:|up:|tt:|single:)/, '').replace(/ \(.*\)$/, '');
            if (historySearch) {
                historySearch.value = cleanKey === 'media' ? '' : cleanKey;
                switchView('history');
                renderTasks();
            }
            return;
        }
    });

    function renderTaskGrid(container, items, viewMode, isHistory) {
        if (!container) return;
        container.innerHTML = '';

        if (items.length === 0) {
            var empty = document.createElement('div');
            empty.className = 'glass-panel p-8 flex flex-col items-center justify-center text-center col-span-full';
            var icon = 'bi-inboxes-fill', title = 'No downloads found', desc = 'Paste a video, audio, or M3U8 link above to start downloading.';
            if (isHistory) { icon = 'bi-collection-play', title = 'Your Media Gallery is empty', desc = 'Completed media downloads will appear here with rich artwork, instant streaming, and Android lockscreen controls.'; }
            else if (currentFilter === 'downloading') { icon = 'bi-arrow-down-circle-fill'; title = 'No active downloads'; desc = 'Active downloads will stream real-time speed & progress here.'; }
            else if (currentFilter === 'queued') { icon = 'bi-hourglass-split'; title = 'Queue is clear'; desc = 'No tasks waiting for a worker slot.'; }
            else if (currentFilter === 'completed') { icon = 'bi-check-circle-fill'; title = 'No completed downloads yet'; desc = 'Finished downloads will appear here with media player & save button.'; }
            else if (currentFilter === 'failed') { icon = 'bi-shield-check'; title = 'Zero failed tasks'; desc = 'All tasks completed without errors.'; }
            empty.innerHTML = '<div class="w-12 h-12 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary mb-3"><i class="bi ' + icon + ' text-[22px]"></i></div><h4 class="text-[14px] font-bold text-white">' + title + '</h4><p class="text-[12px] text-on-surface-variant mt-1 max-w-sm">' + desc + '</p>';
            container.appendChild(empty);
            return;
        }

        items.forEach(function(t) {
            var card = document.createElement('div');
            card.className = 'task-card glass-panel flex flex-col overflow-hidden';
            card.id = 'task-card-' + t.id;

            var percent = Math.min(100, Math.max(0, t.percent || 0)).toFixed(1);
            var inlineStreamUrl = t.mediaId ? ('/download?id=' + encodeURIComponent(t.mediaId) + '&inline=true') : '#';
            var downloadUrl = t.mediaId ? ('/download?id=' + encodeURIComponent(t.mediaId)) : '#';
            var plat = detectPlatform(t.url) || { name: 'Media', icon: 'bi-film', color: '#a78bfa' };
            var thumb = t.thumbnail || '';
            if (!thumb && t.url) {
                var ytM = t.url.match(/(?:youtu\.be\/|youtube\.com\/(?:watch\?v=|shorts\/|embed\/))([a-zA-Z0-9_-]{11})/);
                if (ytM && ytM[1]) thumb = 'https://i.ytimg.com/vi/' + ytM[1] + '/hqdefault.jpg';
            }
            if (!thumb && t.mediaId) {
                var dirPart = t.mediaId.split('/')[0];
                thumb = '/thumbnail?id=' + encodeURIComponent(dirPart);
            }
            if (!thumb && t.id) {
                thumb = '/thumbnail?id=' + encodeURIComponent(t.id);
            }
            if (!thumb) thumb = '/static/logo.svg';

            var statusColor = '#38bdf8';
            var statusBg = 'rgba(56,189,248,.15)';
            var statusBorder = 'rgba(56,189,248,.3)';
            if (t.status === 'completed') { statusColor = '#34d399'; statusBg = 'rgba(52,211,153,.15)'; statusBorder = 'rgba(52,211,153,.3)'; }
            else if (t.status === 'failed') { statusColor = '#f87171'; statusBg = 'rgba(248,113,113,.15)'; statusBorder = 'rgba(248,113,113,.3)'; }
            else if (t.status === 'queued') { statusColor = '#fbbf24'; statusBg = 'rgba(251,191,36,.15)'; statusBorder = 'rgba(251,191,36,.3)'; }

            var isAudio = (t.format && t.format.indexOf('audio') !== -1) || (t.mediaName && (t.mediaName.endsWith('.mp3') || t.mediaName.endsWith('.m4a')));

            // =========================================================
            // 1. GALLERY COMPACT MODE (ANDROID 14 MATERIAL YOU CARDS)
            // =========================================================
            if (isHistory && viewMode === 'compact') {
                card.className = 'android-gallery-card group';
                card.innerHTML = '<div class="android-thumb-wrap">' +
                    '<img src="' + escapeHtml(thumb) + '" alt="" class="w-full h-full object-cover transition-transform duration-300 group-hover:scale-105" onerror="if(!this.dataset.retry){this.dataset.retry=\'1\';this.src=\'/thumbnail?id=' + encodeURIComponent(t.mediaId ? t.mediaId.split('/')[0] : t.id) + '\';}else{this.src=\'/static/logo.svg\';}">' +
                    '<div class="android-play-overlay">' +
                    '<button type="button" class="android-play-btn-circle btn-action-play" data-id="' + t.id + '"><i class="bi bi-play-fill ml-0.5"></i></button>' +
                    '</div>' +
                    '<div class="absolute top-2.5 left-2.5 flex items-center gap-1.5">' +
                    '<span class="text-[10px] px-2 py-0.5 rounded-full bg-black/60 backdrop-blur-md text-white border border-white/10 font-bold flex items-center gap-1"><i class="bi ' + plat.icon + '"></i> ' + plat.name + '</span>' +
                    '</div>' +
                    '<div class="absolute bottom-2.5 right-2.5">' +
                    '<span class="text-[10px] px-2 py-0.5 rounded-full bg-black/70 backdrop-blur-md text-emerald-400 border border-emerald-500/20 font-mono font-bold">' + (t.humanSize || 'Saved') + '</span>' +
                    '</div>' +
                    '</div>' +
                    '<div class="p-3.5 flex flex-col gap-2 flex-1 justify-between">' +
                    '<div>' +
                    '<h4 class="text-[13px] font-bold text-white line-clamp-2 leading-snug" title="' + escapeHtml(t.title || t.mediaName) + '">' + escapeHtml(t.title || t.mediaName) + '</h4>' +
                    '</div>' +
                    '<div class="pt-2 border-t border-white/5 flex items-center justify-between gap-2">' +
                    '<button type="button" class="btn-action-play px-3 py-1.5 rounded-xl bg-primary/15 hover:bg-primary/25 text-primary text-[11px] font-bold flex items-center gap-1.5 transition-all" data-id="' + t.id + '"><i class="bi bi-play-fill"></i> Play</button>' +
                    '<div class="flex items-center gap-1">' +
                    '<a href="' + downloadUrl + '" download="' + escapeHtml(t.mediaName || t.title) + '" class="w-8 h-8 rounded-xl bg-white/5 hover:bg-white/10 text-white flex items-center justify-center text-[12px] transition-all no-underline" title="Save File"><i class="bi bi-download"></i></a>' +
                    '<button type="button" class="btn-action-delete w-8 h-8 rounded-xl bg-white/5 hover:bg-red-500/20 text-on-surface-variant hover:text-red-400 flex items-center justify-center text-[12px] transition-all" data-id="' + t.id + '" title="Delete"><i class="bi bi-trash3"></i></button>' +
                    '</div>' +
                    '</div>' +
                    '</div>';

                container.appendChild(card);
                return;
            }

            // 2. LIST VIEW MODE (Single Row)
            if (viewMode === 'list') {
                card.className = 'task-card glass-panel p-3 flex items-center justify-between gap-3 overflow-hidden rounded-xl';
                var listActions = '';
                if (t.status === 'completed' && t.mediaId) {
                    listActions += '<button type="button" class="btn-action-play px-3 py-1 rounded-lg bg-primary/15 hover:bg-primary/25 text-primary text-[11px] font-bold transition-all shrink-0" data-id="' + t.id + '"><i class="bi bi-play-fill"></i> Play</button>';
                    listActions += '<a href="' + downloadUrl + '" download="' + escapeHtml(t.mediaName || t.title) + '" class="btn-primary-glow px-3 py-1 rounded-lg text-[11px] font-bold transition-all no-underline shrink-0"><i class="bi bi-download"></i> Save</a>';
                }
                if (t.status === 'downloading' || t.status === 'queued') {
                    listActions += '<button type="button" class="btn-action-cancel px-3 py-1 rounded-lg bg-white/5 hover:bg-white/10 text-white text-[11px] font-semibold transition-all shrink-0" data-id="' + t.id + '"><i class="bi bi-x-circle"></i> Cancel</button>';
                }
                if (t.status === 'failed' || t.status === 'cancelled') {
                    listActions += '<button type="button" class="btn-action-retry px-3 py-1 rounded-lg bg-emerald-500/15 text-emerald-300 text-[11px] font-semibold transition-all shrink-0" data-id="' + t.id + '"><i class="bi bi-arrow-counterclockwise"></i> Retry</button>';
                }
                if (isHistory) {
                    listActions += '<button type="button" class="btn-action-delete w-7 h-7 rounded-lg bg-white/5 hover:bg-red-500/20 text-on-surface-variant hover:text-red-400 flex items-center justify-center transition-all shrink-0" data-id="' + t.id + '" title="Delete file completely from disk"><i class="bi bi-trash3 text-[12px]"></i></button>';
                } else {
                    listActions += '<button type="button" class="btn-action-dismiss w-7 h-7 rounded-lg bg-white/5 hover:bg-white/15 text-on-surface-variant hover:text-white flex items-center justify-center transition-all shrink-0" data-id="' + t.id + '" title="Clear from recent downloads"><i class="bi bi-x-lg text-[11px]"></i></button>';
                }

                card.innerHTML = '<div class="w-8 h-8 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary shrink-0"><i class="bi ' + plat.icon + '"></i></div>' +
                    '<div class="flex-1 min-w-0">' +
                    '<div class="flex items-center gap-2">' +
                    '<h4 class="text-[12px] font-bold text-white truncate" title="' + escapeHtml(t.title || t.url) + '">' + escapeHtml(t.title || t.url) + '</h4>' +
                    '<span class="text-[9px] px-1.5 py-0.2 rounded font-mono uppercase shrink-0" style="background:' + statusBg + ';color:' + statusColor + ';">' + t.status + '</span>' +
                    '</div>' +
                    '<div class="flex items-center gap-2 text-[10px] text-on-surface-variant font-mono mt-0.5">' +
                    '<span>' + (t.humanSize || (percent + '%')) + '</span>' +
                    (t.speed ? '<span class="text-cyan-400">• ' + t.speed + '</span>' : '') +
                    '<span class="truncate text-on-surface-variant/60">' + escapeHtml(t.url) + '</span>' +
                    '</div>' +
                    '</div>' +
                    '<div class="flex items-center gap-2 shrink-0">' + listActions + '</div>';

                container.appendChild(card);
                return;
            }

            // 3. COMPACT GRID VIEW MODE FOR DOWNLOAD CENTER
            if (viewMode === 'compact') {
                var compactActions = '';
                if (t.status === 'completed' && t.mediaId) {
                    compactActions += '<button type="button" class="btn-action-play px-2.5 py-1 rounded-lg bg-primary/15 text-primary text-[10px] font-bold" data-id="' + t.id + '"><i class="bi bi-play-fill"></i> Play</button>';
                    compactActions += '<a href="' + downloadUrl + '" download="' + escapeHtml(t.mediaName || t.title) + '" class="btn-primary-glow px-3 py-1 rounded-lg text-[10px] font-bold no-underline"><i class="bi bi-download"></i> Save</a>';
                }
                if (t.status === 'downloading' || t.status === 'queued') {
                    compactActions += '<button type="button" class="btn-action-cancel px-2.5 py-1 rounded-lg bg-white/5 text-white text-[10px] font-semibold" data-id="' + t.id + '">Cancel</button>';
                }
                if (t.status === 'failed') {
                    compactActions += '<button type="button" class="btn-action-retry px-2.5 py-1 rounded-lg bg-emerald-500/15 text-emerald-300 text-[10px] font-semibold" data-id="' + t.id + '">Retry</button>';
                }
                if (isHistory) {
                    compactActions += '<button type="button" class="btn-action-delete w-6 h-6 rounded-md bg-white/5 hover:bg-red-500/20 text-on-surface-variant hover:text-red-400 flex items-center justify-center ml-auto" data-id="' + t.id + '" title="Delete file from disk"><i class="bi bi-trash3 text-[11px]"></i></button>';
                } else {
                    compactActions += '<button type="button" class="btn-action-dismiss w-6 h-6 rounded-md bg-white/5 hover:bg-white/15 text-on-surface-variant hover:text-white flex items-center justify-center ml-auto" data-id="' + t.id + '" title="Clear from recent downloads"><i class="bi bi-x-lg text-[10px]"></i></button>';
                }

                card.innerHTML = '<div class="p-3 flex flex-col gap-2 min-w-0">' +
                    '<div class="flex items-center justify-between gap-2 min-w-0">' +
                    '<div class="flex items-center gap-1.5 min-w-0 flex-1">' +
                    '<i class="bi ' + plat.icon + ' text-primary text-[13px] shrink-0"></i>' +
                    '<h4 class="text-[12px] font-bold text-white truncate" title="' + escapeHtml(t.title || t.url) + '">' + escapeHtml(t.title || t.url) + '</h4>' +
                    '</div>' +
                    '<span class="text-[9px] px-1.5 py-0.2 rounded font-mono uppercase shrink-0" style="background:' + statusBg + ';color:' + statusColor + ';">' + t.status + '</span>' +
                    '</div>' +
                    '<div class="h-1 w-full bg-white/5 rounded-full overflow-hidden">' +
                    '<div class="h-full rounded-full" style="width:' + (t.status === 'completed' ? '100' : percent) + '%;background:' + (t.status === 'completed' ? '#10b981' : t.status === 'downloading' ? '#a078ff' : '#f59e0b') + ';"></div>' +
                    '</div>' +
                    '<div class="flex justify-between items-center text-[10px] text-on-surface-variant font-mono">' +
                    '<span>' + (t.humanSize || (t.speed || (percent + '%'))) + '</span>' +
                    '<span class="text-white">' + (t.status === 'completed' ? '100%' : percent + '%') + '</span>' +
                    '</div>' +
                    '<div class="pt-2 border-t border-white/5 flex items-center gap-1.5">' + compactActions + '</div>' +
                    '</div>';

                container.appendChild(card);
                return;
            }

            // 4. STANDARD GRID VIEW MODE (Rich Cards with Video/Audio Preview)
            var actionsHtml = '';
            if (t.status === 'queued' || t.status === 'downloading') {
                actionsHtml += '<button type="button" class="btn-action-cancel px-3 py-1.5 rounded-full bg-white/5 hover:bg-white/10 border border-white/10 text-white text-[11px] font-semibold transition-all" data-id="' + t.id + '"><i class="bi bi-x-circle"></i> Cancel</button>';
            }
            if (t.status === 'failed' || t.status === 'cancelled') {
                actionsHtml += '<button type="button" class="btn-action-retry px-3 py-1.5 rounded-full bg-emerald-500/15 hover:bg-emerald-500/25 border border-emerald-500/30 text-emerald-300 text-[11px] font-semibold transition-all" data-id="' + t.id + '"><i class="bi bi-arrow-counterclockwise"></i> Retry</button>';
            }
            if (t.status === 'completed' && t.mediaId) {
                actionsHtml += '<button type="button" class="btn-action-play px-3.5 py-1.5 rounded-full bg-cyan-500/15 hover:bg-cyan-500/25 border border-cyan-500/30 text-cyan-300 text-[11px] font-bold transition-all" data-id="' + t.id + '"><i class="bi bi-play-circle-fill"></i> Play</button>';
                actionsHtml += '<a href="' + downloadUrl + '" download="' + escapeHtml(t.mediaName || t.title) + '" class="btn-primary-glow px-4 py-1.5 rounded-full text-[11px] font-bold transition-all no-underline"><i class="bi bi-download"></i> Save File</a>';
            }
            if (isHistory) {
                actionsHtml += '<button type="button" class="btn-action-delete w-7 h-7 rounded-full bg-white/5 hover:bg-red-500/20 text-on-surface-variant hover:text-red-400 flex items-center justify-center transition-all ml-auto" data-id="' + t.id + '" title="Delete file completely from disk"><i class="bi bi-trash3 text-[12px]"></i></button>';
            } else {
                actionsHtml += '<button type="button" class="btn-action-dismiss w-7 h-7 rounded-full bg-white/5 hover:bg-white/15 text-on-surface-variant hover:text-white flex items-center justify-center transition-all ml-auto" data-id="' + t.id + '" title="Clear from recent downloads"><i class="bi bi-x-lg text-[11px]"></i></button>';
            }

            var mediaPreviewHtml = '';
            if (t.status === 'completed' && t.mediaId) {
                mediaPreviewHtml = '<div class="mx-3.5 mb-2.5 rounded-xl overflow-hidden bg-black/60 border border-white/5 relative aspect-video flex items-center justify-center group/preview cursor-pointer btn-action-play" data-id="' + t.id + '">' +
                    '<img src="' + escapeHtml(thumb) + '" alt="" class="w-full h-full object-cover transition-transform duration-300 group-hover/preview:scale-105" onerror="if(!this.dataset.retry){this.dataset.retry=\'1\';this.src=\'/thumbnail?id=' + encodeURIComponent(t.mediaId ? t.mediaId.split('/')[0] : t.id) + '\';}else{this.src=\'/static/logo.svg\';}">' +
                    '<div class="absolute inset-0 bg-black/40 flex items-center justify-center transition-all group-hover/preview:bg-black/20">' +
                    '<div class="w-12 h-12 rounded-full bg-primary/90 text-black flex items-center justify-center text-[20px] shadow-lg transition-transform group-hover/preview:scale-110"><i class="bi ' + (isAudio ? 'bi-music-note-beamed' : 'bi-play-fill ml-0.5') + '"></i></div>' +
                    '</div>' +
                    '<div class="absolute bottom-2 right-2 px-2 py-0.5 rounded-md bg-black/70 backdrop-blur-md text-[10px] font-mono text-emerald-400 font-bold">' + (t.humanSize || 'Saved') + '</div>' +
                    '</div>';
            } else if (t.status === 'failed' && t.errorMessage) {
                mediaPreviewHtml = '<div class="mx-3.5 mb-2.5 p-2.5 rounded-xl bg-red-500/10 border border-red-500/20 text-[11px] text-red-200"><div class="font-bold text-red-400 mb-1 flex items-center gap-1"><i class="bi bi-bug-fill"></i> Error Details:</div><pre class="font-mono text-[10px] whitespace-pre-wrap break-all text-red-300">' + escapeHtml(t.errorMessage) + '</pre></div>';
            }

            card.innerHTML = '<div class="p-3.5 flex gap-3 items-start justify-between">' +
                '<div class="w-9 h-9 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary shrink-0"><i class="bi ' + plat.icon + ' text-[16px]"></i></div>' +
                '<div class="flex-1 min-w-0">' +
                '<h4 class="text-[13px] font-bold text-white truncate" title="' + escapeHtml(t.title || t.url) + '">' + escapeHtml(t.title || t.url) + '</h4>' +
                '<span class="text-[11px] text-on-surface-variant truncate block font-mono" title="' + escapeHtml(t.url) + '">' + escapeHtml(t.url) + '</span>' +
                '<div class="flex items-center gap-2 mt-1.5">' +
                '<span class="text-[10px] px-2 py-0.5 rounded-full bg-white/5 border border-white/10 text-white font-mono">' + escapeHtml(t.format) + '</span>' +
                '<span class="text-[10px] px-2 py-0.5 rounded-full font-semibold uppercase" style="background:' + statusBg + ';color:' + statusColor + ';border:1px solid ' + statusBorder + ';">' + t.status + '</span>' +
                '</div>' +
                '</div>' +
                '</div>' +
                '<div class="px-3.5 pb-2.5">' +
                '<div class="h-1.5 w-full bg-white/5 rounded-full overflow-hidden">' +
                '<div class="h-full rounded-full transition-all duration-300" style="width:' + (t.status === 'completed' ? '100' : percent) + '%;background:' + (t.status === 'completed' ? '#10b981' : t.status === 'failed' ? '#ef4444' : t.status === 'downloading' ? 'linear-gradient(90deg,#06b6d4,#a078ff)' : '#f59e0b') + ';"></div>' +
                '</div>' +
                '<div class="flex justify-between items-center mt-1.5 text-[11px] text-on-surface-variant">' +
                '<div class="flex items-center gap-1.5">' + (t.status === 'downloading' ? '<span class="text-cyan-400 font-mono"><i class="bi bi-speedometer2"></i> ' + (t.speed || '--') + '</span> • <span class="font-mono">ETA: ' + (t.eta || '--') + '</span>' : '<span class="text-emerald-400 font-mono">' + (t.humanSize || 'Finished') + '</span>') + '</div>' +
                '<span class="font-mono text-white">' + (t.status === 'completed' ? '100%' : percent + '%') + '</span>' +
                '</div>' +
                '</div>' +
                mediaPreviewHtml +
                '<div class="p-3 border-t border-white/5 flex items-center gap-2 flex-wrap bg-black/20">' + actionsHtml + '</div>';

            container.appendChild(card);
        });
    }

    // Throttled UI Render Scheduler (Prevents Browser Freeze/Crashes under High SSE Activity)
    var renderScheduled = false;
    function scheduleRenderTasks() {
        if (renderScheduled) return;
        renderScheduled = true;
        requestAnimationFrame(function() {
            renderScheduled = false;
            renderTasks();
        });
    }

    // SSE EventSource for real-time progress
    function initSSE() {
        var es = new EventSource('/api/events');
        es.onopen = function() {
            if (connStatus) connStatus.innerHTML = '<span class="w-2 h-2 rounded-full bg-emerald-400 shadow-[0_0_8px_rgba(52,211,153,0.8)]"></span><span class="font-semibold text-emerald-400">Live</span><span class="text-white/30">•</span><span id="navGlobalSpeed" class="text-white font-medium">' + (document.getElementById('footerSpeed') ? document.getElementById('footerSpeed').textContent : '0 KB/s') + '</span>';
        };
        es.onmessage = function(e) {
            try {
                var d = JSON.parse(e.data);
                if (d.type === 'init' && Array.isArray(d.tasks)) {
                    tasksMap = {};
                    d.tasks.forEach(function(t) { tasksMap[t.id] = t; });
                    scheduleRenderTasks();
                } else if (d.type === 'task_update' && d.task) {
                    tasksMap[d.task.id] = d.task;
                    if (d.task.status === 'completed') {
                        fetchGallery();
                    }
                    scheduleRenderTasks();
                } else if (d.type === 'task_deleted' && d.taskId) {
                    delete tasksMap[d.taskId];
                    scheduleRenderTasks();
                }
            } catch (err) { console.error(err); }
        };
        es.onerror = function() {
            if (connStatus) connStatus.innerHTML = '<span class="w-2 h-2 rounded-full bg-amber-400 animate-pulse"></span><span class="text-amber-400 font-medium">Reconnecting</span>';
        };
    }

    function fetchQueueTasks() {
        var xhr = new XMLHttpRequest();
        xhr.open('GET', '/api/queue', true);
        xhr.onload = function() {
            if (xhr.status === 200) {
                try {
                    var res = JSON.parse(xhr.responseText);
                    if (res.tasks && Array.isArray(res.tasks)) {
                        tasksMap = {};
                        res.tasks.forEach(function(t) { tasksMap[t.id] = t; });
                        renderTasks();
                    }
                } catch (e) {}
            }
        };
        xhr.send();
    }

    // =========================================================================
    // IN-APP BROWSER & REAL-TIME MEDIA SNIFFER CONTROLLER
    // =========================================================================
    var browserInitialized = false;
    var sniffedMediaList = [];
    var sniffedUrlsSet = new Set();
    var currentBrowserUrl = '';
    var currentDrawerFilter = 'all';

    var browserFrame = document.getElementById('browserFrame');
    var browserWaitScreen = document.getElementById('browserWaitScreen');
    var browserUrlInput = document.getElementById('browserUrlInput');
    var btnBrowserGo = document.getElementById('btnBrowserGo');
    var btnBrowserBack = document.getElementById('btnBrowserBack');
    var btnBrowserForward = document.getElementById('btnBrowserForward');
    var btnBrowserReload = document.getElementById('btnBrowserReload');
    var btnBrowserHome = document.getElementById('btnBrowserHome');
    var btnBrowserDeepSniff = document.getElementById('btnBrowserDeepSniff');
    var btnBrowserOpenExternal = document.getElementById('btnBrowserOpenExternal');
    var browserBubbleBtn = document.getElementById('browserBubbleBtn');
    var browserBubbleBadge = document.getElementById('browserBubbleBadge');
    var browserMediaDrawer = document.getElementById('browserMediaDrawer');
    var btnDrawerClose = document.getElementById('btnDrawerClose');
    var btnDrawerRefresh = document.getElementById('btnDrawerRefresh');
    var btnDrawerDownloadAll = document.getElementById('btnDrawerDownloadAll');
    var drawerMediaList = document.getElementById('drawerMediaList');
    var drawerPageTitle = document.getElementById('drawerPageTitle');
    var btnWaitForceShow = document.getElementById('btnWaitForceShow');
    var btnWaitDeepSniff = document.getElementById('btnWaitDeepSniff');
    var browserNavBadge = document.getElementById('browserNavBadge');

    function initBrowserView() {
        if (browserInitialized) return;
        browserInitialized = true;

        if (btnBrowserGo && browserUrlInput) {
            btnBrowserGo.addEventListener('click', function() {
                loadBrowserUrl(browserUrlInput.value);
            });
            browserUrlInput.addEventListener('keydown', function(e) {
                if (e.key === 'Enter') {
                    e.preventDefault();
                    loadBrowserUrl(this.value);
                }
            });
        }

        if (btnBrowserBack && browserFrame) {
            btnBrowserBack.addEventListener('click', function() {
                try { browserFrame.contentWindow.history.back(); } catch(e) {}
            });
        }
        if (btnBrowserForward && browserFrame) {
            btnBrowserForward.addEventListener('click', function() {
                try { browserFrame.contentWindow.history.forward(); } catch(e) {}
            });
        }
        if (btnBrowserReload) {
            btnBrowserReload.addEventListener('click', function() {
                loadBrowserUrl(currentBrowserUrl);
            });
        }
        if (btnBrowserHome) {
            btnBrowserHome.addEventListener('click', function() {
                currentBrowserUrl = '';
                if (browserUrlInput) browserUrlInput.value = '';
                if (browserFrame) browserFrame.src = 'about:blank';
                if (browserWaitScreen) {
                    browserWaitScreen.style.display = 'flex';
                    browserWaitScreen.style.opacity = '1';
                    var titleEl = document.getElementById('waitScreenTitle');
                    if (titleEl) titleEl.textContent = 'Universal Media Sniffer & Stream Engine';
                }
            });
        }
        if (btnBrowserDeepSniff) {
            btnBrowserDeepSniff.addEventListener('click', function() {
                triggerDeepSniff(currentBrowserUrl);
            });
        }
        if (btnBrowserOpenExternal) {
            btnBrowserOpenExternal.addEventListener('click', function() {
                window.open(currentBrowserUrl, '_blank');
            });
        }

        document.querySelectorAll('.browser-shortcut').forEach(function(btn) {
            btn.addEventListener('click', function() {
                document.querySelectorAll('.browser-shortcut').forEach(function(b) { b.classList.remove('active'); });
                this.classList.add('active');
                var u = this.getAttribute('data-url');
                if (u) loadBrowserUrl(u);
            });
        });

        if (browserBubbleBtn) {
            browserBubbleBtn.addEventListener('click', function() {
                toggleMediaDrawer();
            });
        }
        if (btnDrawerClose) {
            btnDrawerClose.addEventListener('click', function() {
                closeMediaDrawer();
            });
        }
        if (btnDrawerRefresh) {
            btnDrawerRefresh.addEventListener('click', function() {
                triggerDeepSniff(currentBrowserUrl);
            });
        }
        if (btnDrawerDownloadAll) {
            btnDrawerDownloadAll.addEventListener('click', downloadAllSniffedMedia);
        }

        if (btnWaitForceShow && browserWaitScreen) {
            btnWaitForceShow.addEventListener('click', function() {
                browserWaitScreen.style.opacity = '0';
                setTimeout(function() { browserWaitScreen.style.display = 'none'; }, 300);
            });
        }
        if (btnWaitDeepSniff) {
            btnWaitDeepSniff.addEventListener('click', function() {
                triggerDeepSniff(currentBrowserUrl);
                browserWaitScreen.style.opacity = '0';
                setTimeout(function() { browserWaitScreen.style.display = 'none'; }, 300);
                openMediaDrawer();
            });
        }

        document.querySelectorAll('.drawer-filter').forEach(function(btn) {
            btn.addEventListener('click', function() {
                document.querySelectorAll('.drawer-filter').forEach(function(b) {
                    b.classList.remove('active', 'text-primary', 'border-b-2', 'border-primary');
                    b.classList.add('text-on-surface-variant');
                });
                this.classList.add('active', 'text-primary', 'border-b-2', 'border-primary');
                this.classList.remove('text-on-surface-variant');
                currentDrawerFilter = this.getAttribute('data-type') || 'all';
                renderDrawerList();
            });
        });

        if (browserFrame) {
            browserFrame.addEventListener('load', function() {
                setTimeout(function() {
                    if (browserWaitScreen) {
                        browserWaitScreen.style.opacity = '0';
                        setTimeout(function() { browserWaitScreen.style.display = 'none'; }, 300);
                    }
                }, 600);
            });
        }

        // Listen for postMessage from sandbox sniffer
        window.addEventListener('message', function(e) {
            if (!e.data || typeof e.data !== 'object') return;
            if (e.data.type === 'kv_browser_media_found' && e.data.item) {
                addSniffedMediaItem(e.data.item);
            } else if (e.data.type === 'kv_browser_page_loaded') {
                if (browserWaitScreen) {
                    browserWaitScreen.style.opacity = '0';
                    setTimeout(function() { browserWaitScreen.style.display = 'none'; }, 300);
                }
                if (e.data.title && drawerPageTitle) {
                    drawerPageTitle.textContent = e.data.title;
                }
            }
        });

        loadBrowserUrl(currentBrowserUrl);
    }

    function loadBrowserUrl(rawUrl) {
        var u = (rawUrl || '').trim();
        if (!u) return;

        // Auto convert search queries
        if (u.indexOf('://') === -1 && !u.startsWith('/') && !u.includes('.')) {
            u = 'https://www.google.com/search?q=' + encodeURIComponent(u);
        } else if (!u.startsWith('http://') && !u.startsWith('https://')) {
            u = 'https://' + u;
        }

        currentBrowserUrl = u;
        if (browserUrlInput) browserUrlInput.value = u;
        if (drawerPageTitle) drawerPageTitle.textContent = u;

        // Reset Wait Screen
        if (browserWaitScreen) {
            browserWaitScreen.style.display = 'flex';
            browserWaitScreen.style.opacity = '1';
            var titleEl = document.getElementById('waitScreenTitle');
            if (titleEl) {
                try { titleEl.textContent = 'Connecting to ' + new URL(u).hostname + '...'; }
                catch(e) { titleEl.textContent = 'Connecting to Sandbox Browser...'; }
            }
        }

        // Clear current sniffer state for new page
        sniffedMediaList = [];
        sniffedUrlsSet.clear();
        renderDrawerList();

        // Load into Sandboxed Proxy
        if (browserFrame) {
            browserFrame.src = '/api/browser/proxy?url=' + encodeURIComponent(u);
        }

        // Trigger deep backend sniff
        triggerDeepSniff(u);
    }

    function triggerDeepSniff(u) {
        var spinIcon = document.getElementById('sniffSpinIcon');
        if (spinIcon) spinIcon.classList.add('animate-spin');

        fetch('/api/browser/sniff?url=' + encodeURIComponent(u))
            .then(function(res) { return res.json(); })
            .then(function(data) {
                if (spinIcon) spinIcon.classList.remove('animate-spin');
                if (data && Array.isArray(data.items)) {
                    if (data.pageTitle && drawerPageTitle) drawerPageTitle.textContent = data.pageTitle;
                    data.items.forEach(function(item) {
                        addSniffedMediaItem(item);
                    });
                    if (data.items.length > 0) {
                        showToast('Sniffed ' + data.items.length + ' media streams from page!', 'info');
                    }
                }
            })
            .catch(function(err) {
                if (spinIcon) spinIcon.classList.remove('animate-spin');
                console.warn('Sniff error:', err);
            });
    }

    function addSniffedMediaItem(item) {
        if (!item || !item.url) return;
        var cleanUrl = item.url.trim();
        if (sniffedUrlsSet.has(cleanUrl)) return;
        sniffedUrlsSet.add(cleanUrl);

        var ext = cleanUrl.split('?')[0].split('.').pop().toLowerCase();
        var type = item.type || 'video';
        if (!item.type) {
            if (ext === 'mp3' || ext === 'm4a' || ext === 'flac' || ext === 'aac' || ext === 'wav' || ext === 'ogg') type = 'audio';
            else if (ext === 'jpg' || ext === 'png' || ext === 'webp' || ext === 'jpeg' || ext === 'gif') type = 'image';
            else type = 'video';
        }

        var formatLabel = item.format || (ext ? ext.toUpperCase() : 'Media Stream');
        if (cleanUrl.indexOf('.m3u8') !== -1) formatLabel = 'M3U8 HLS Stream';

        sniffedMediaList.push({
            id: 'sniff_' + (sniffedMediaList.length + 1),
            url: cleanUrl,
            title: item.title || cleanUrl,
            type: type,
            format: formatLabel,
            thumbnail: item.thumbnail || '',
            size: item.size || ''
        });

        renderDrawerList();
    }

    function renderDrawerList() {
        var total = sniffedMediaList.length;
        var videos = sniffedMediaList.filter(function(i) { return i.type === 'video'; });
        var audios = sniffedMediaList.filter(function(i) { return i.type === 'audio'; });
        var images = sniffedMediaList.filter(function(i) { return i.type === 'image'; });

        // Update counts
        if (browserBubbleBadge) browserBubbleBadge.textContent = total;
        if (browserNavBadge) {
            browserNavBadge.textContent = total;
            browserNavBadge.classList.toggle('hidden', total === 0);
        }
        var cTotal = document.getElementById('drawerTotalCount');
        if (cTotal) cTotal.textContent = total;
        var cAll = document.getElementById('drawerCountAll');
        if (cAll) cAll.textContent = total;
        var cVid = document.getElementById('drawerCountVideo');
        if (cVid) cVid.textContent = videos.length;
        var cAud = document.getElementById('drawerCountAudio');
        if (cAud) cAud.textContent = audios.length;
        var cImg = document.getElementById('drawerCountImage');
        if (cImg) cImg.textContent = images.length;
        var fText = document.getElementById('drawerFooterText');
        if (fText) fText.textContent = total + ' media item' + (total === 1 ? '' : 's') + ' detected';

        if (!drawerMediaList) return;
        var listToRender = sniffedMediaList;
        if (currentDrawerFilter === 'video') listToRender = videos;
        else if (currentDrawerFilter === 'audio') listToRender = audios;
        else if (currentDrawerFilter === 'image') listToRender = images;

        if (listToRender.length === 0) {
            var placeholder = '<div class="py-12 text-center text-on-surface-variant drawer-media-placeholder flex flex-col items-center gap-2">';
            placeholder += '<i class="bi bi-radar text-[32px] ' + (isLight ? 'text-slate-400' : 'text-white/30') + ' animate-pulse"></i>';
            placeholder += '<p class="text-[13px] font-semibold ' + (isLight ? 'text-slate-600' : 'text-white/70') + '">No ' + (currentDrawerFilter === 'all' ? 'media' : currentDrawerFilter + 's') + ' detected yet</p>';
            placeholder += '<p class="text-[11px] ' + (isLight ? 'text-slate-500' : 'text-white/40') + ' max-w-[240px]">Play a video on the page or click Deep Sniff above.</p>';
            placeholder += '</div>';
            drawerMediaList.innerHTML = placeholder;
            return;
        }

        drawerMediaList.innerHTML = '';
        listToRender.forEach(function(item) {
            var row = document.createElement('div');
            row.className = 'p-3 rounded-xl bg-white/5 hover:bg-white/10 border border-white/10 flex flex-col gap-2 transition-all group';

            // Apply current theme to newly created drawer item
            var isLight = document.body.classList.contains('theme-light');
            if (isLight) {
                row.style.backgroundColor = 'rgba(0, 0, 0, 0.04)';
                row.style.borderColor = 'rgba(0, 0, 0, 0.08)';
            }

            var iconClass = item.type === 'video' ? 'bi-play-circle-fill text-purple-400' : (item.type === 'audio' ? 'bi-music-note-beamed text-pink-400' : 'bi-image text-cyan-400');
            var isM3u8 = item.url.indexOf('.m3u8') !== -1;

            row.innerHTML = '<div class="flex items-start gap-2.5">' +
                '<div class="w-8 h-8 rounded-lg bg-black/40 border border-white/10 flex items-center justify-center shrink-0 mt-0.5">' +
                '<i class="bi ' + iconClass + ' text-[16px]"></i>' +
                '</div>' +
                '<div class="flex-1 min-w-0">' +
                '<div class="flex items-center gap-1.5 flex-wrap mb-1">' +
                '<span class="text-[10px] px-2 py-0.5 rounded-full bg-primary/20 text-primary border border-primary/30 font-bold font-mono">' + escapeHtml(item.format) + '</span>' +
                (isM3u8 ? '<span class="text-[10px] px-1.5 py-0.5 rounded-full bg-amber-500/20 text-amber-300 font-bold">HLS Live</span>' : '') +
                '</div>' +
                '<h5 class="text-[12px] font-bold text-white line-clamp-2 leading-snug" title="' + escapeHtml(item.title) + '">' + escapeHtml(item.title) + '</h5>' +
                '<p class="text-[10px] text-on-surface-variant/70 truncate font-mono mt-0.5" title="' + escapeHtml(item.url) + '">' + escapeHtml(item.url) + '</p>' +
                '</div>' +
                '</div>' +
                '<div class="pt-2 border-t border-white/5 flex items-center justify-between gap-1.5 flex-wrap">' +
                '<button type="button" class="btn-sniff-download flex-1 px-3 py-1.5 rounded-lg btn-primary-glow text-[11px] font-bold flex items-center justify-center gap-1"><i class="bi bi-download"></i> Download to KV</button>' +
                '<button type="button" class="btn-sniff-play px-2.5 py-1.5 rounded-lg bg-white/10 hover:bg-white/15 text-white text-[11px] font-semibold flex items-center gap-1" title="Play Preview"><i class="bi bi-play-fill"></i> Play</button>' +
                '<button type="button" class="btn-sniff-copy px-2 py-1.5 rounded-lg bg-white/5 hover:bg-white/10 text-on-surface-variant hover:text-white text-[11px]" title="Copy Link"><i class="bi bi-copy"></i></button>' +
                '</div>';

            row.querySelector('.btn-sniff-download').addEventListener('click', function() {
                enqueueSniffedMedia(item);
            });
            row.querySelector('.btn-sniff-play').addEventListener('click', function() {
                playDirectStream(item.url, item.title, item.type === 'audio');
            });
            row.querySelector('.btn-sniff-copy').addEventListener('click', function() {
                if (navigator.clipboard && navigator.clipboard.writeText) {
                    navigator.clipboard.writeText(item.url);
                    showToast('Link copied to clipboard', 'info');
                }
            });

            drawerMediaList.appendChild(row);
        });
    }

    function enqueueSniffedMedia(item) {
        var xhr = new XMLHttpRequest();
        xhr.open('POST', '/api/queue/add', true);
        xhr.setRequestHeader('Content-Type', 'application/json');
        xhr.onload = function() {
            if (xhr.status === 200) {
                showToast('Enqueued: ' + (item.title || 'Media Stream'), 'success');
                fetchQueueTasks();
            } else {
                showToast('Failed to enqueue media', 'error');
            }
        };
        xhr.send(JSON.stringify({
            url: item.url,
            format: item.type === 'audio' ? 'audio_mp3' : 'best',
            cookies: localStorage.getItem('kv_user_cookies') || ''
        }));
    }

    function downloadAllSniffedMedia() {
        if (sniffedMediaList.length === 0) {
            showToast('No media items to download', 'warning');
            return;
        }
        var count = 0;
        sniffedMediaList.forEach(function(item) {
            enqueueSniffedMedia(item);
            count++;
        });
        showToast('Enqueued ' + count + ' media items to Download Center!', 'success');
        closeMediaDrawer();
    }

    function toggleMediaDrawer() {
        if (!browserMediaDrawer) return;
        var isHidden = browserMediaDrawer.classList.contains('hidden');
        if (isHidden) openMediaDrawer();
        else closeMediaDrawer();
    }

    function openMediaDrawer() {
        if (browserMediaDrawer) browserMediaDrawer.classList.remove('hidden');
    }

    function closeMediaDrawer() {
        if (browserMediaDrawer) browserMediaDrawer.classList.add('hidden');
    }

    function cleanupHls() {
        if (currentHlsInstance) {
            try { currentHlsInstance.destroy(); } catch(e) {}
            currentHlsInstance = null;
        }
        if (globalVideoEngine) {
            globalVideoEngine.removeAttribute('src');
            globalVideoEngine.load();
        }
        if (globalAudioEngine) {
            globalAudioEngine.removeAttribute('src');
            globalAudioEngine.load();
        }
    }

    function playDirectStream(streamUrl, title, isAudio) {
        var engine = isAudio ? globalAudioEngine : globalVideoEngine;
        var other = isAudio ? globalVideoEngine : globalAudioEngine;
        try { other.pause(); } catch(e) {}
        cleanupHls();

        var safeTitle = title || 'Direct Stream';
        var isM3u8 = /\.m3u8(\?|$)/i.test(streamUrl);
        var isMpds = /\.mpd(\?|$)/i.test(streamUrl);
        var directExtPattern = /\.(mp4|webm|mkv|mov|avi|flv|m3u8|mpd|mp3|m4a|aac|wav|flac|ogg|oga)(\?|$)/i;
        var isDirectFile = directExtPattern.test(streamUrl.split('?')[0]);
        var isPageUrl = !isDirectFile;

        if (playerBarTitle) playerBarTitle.textContent = safeTitle;
        if (playerBarSubtitle) playerBarSubtitle.textContent = (isAudio ? 'Audio Stream' : (isM3u8 ? 'HLS Live Stream' : 'Video Stream')) + ' • In-App Browser';
        if (playerBarThumb) playerBarThumb.src = '/static/logo.svg';
        if (androidPlayerBar) androidPlayerBar.classList.add('active');

        if (!isAudio && videoModal) {
            if (videoModalTitle) videoModalTitle.textContent = safeTitle;
            if (videoModalPlatform) videoModalPlatform.textContent = 'In-App Browser Stream';
            if (videoModalSize) videoModalSize.textContent = isM3u8 ? 'HLS Live' : 'Live / Direct';
            if (videoModalDownloadBtn) {
                videoModalDownloadBtn.href = streamUrl;
                videoModalDownloadBtn.setAttribute('download', 'stream.mp4');
            }
            videoModal.classList.remove('d-none');
        }

        var cookies = '';
        try { cookies = localStorage.getItem('kv_user_cookies') || ''; } catch(e) {}

        function startPlayback(srcUrl, isHls) {
            // Ensure any previous error listener is removed; add new one that handles fallback to server mux
            var fallbackAttempted = false;
            var onMediaError = function() {
                var errCode = engine.error ? engine.error.code : 0;
                console.warn('Media element error code', errCode, engine.error, 'src:', srcUrl);
                if (!fallbackAttempted && isPageUrl) {
                    fallbackAttempted = true;
                    // Direct CDN via proxy-media gave 403/404 - fallback to server-side yt-dlp mux stream for page URL
                    var streamFallback = '/api/browser/stream?url=' + encodeURIComponent(streamUrl);
                    if (cookies) streamFallback += '&cookies=' + encodeURIComponent(cookies);
                    if (srcUrl !== streamFallback) {
                        console.log('Media error, retrying via server stream:', streamFallback);
                        showToast('Direct stream blocked (403), retrying via server...', 'info');
                        // cleanup HLS if any
                        cleanupHls();
                        engine.src = streamFallback;
                        engine.load();
                        engine.play().then(function(){ updatePlayerBarState(true); }).catch(function(err){
                            console.warn('Fallback play failed:', err);
                            showToast('Playback failed - try Download to KV', 'error');
                        });
                        return;
                    }
                }
                showToast('Playback failed - try Download to KV', 'error');
            };
            engine.addEventListener('error', onMediaError, { once: true });

            if (isHls && typeof Hls !== 'undefined' && Hls.isSupported()) {
                cleanupHls();
                var hls = new Hls({ enableWorker: true, lowLatencyMode: false });
                currentHlsInstance = hls;
                hls.loadSource(srcUrl);
                hls.attachMedia(engine);
                hls.on(Hls.Events.MANIFEST_PARSED, function() {
                    engine.play().then(function(){ updatePlayerBarState(true); }).catch(function(err){
                        console.warn('HLS play error:', err);
                        showToast('Playback failed: ' + (err.message || err), 'error');
                    });
                });
                hls.on(Hls.Events.ERROR, function(event, data){
                    if (data.fatal) {
                        console.warn('HLS fatal error:', data);
                        switch(data.type) {
                            case Hls.ErrorTypes.NETWORK_ERROR:
                                // If HLS network error and we have page URL, fallback to mp4 stream
                                if (isPageUrl && !fallbackAttempted) {
                                    fallbackAttempted = true;
                                    hls.destroy();
                                    currentHlsInstance = null;
                                    var fb = '/api/browser/stream?url=' + encodeURIComponent(streamUrl);
                                    if (cookies) fb += '&cookies=' + encodeURIComponent(cookies);
                                    showToast('HLS failed, retrying mp4 stream...', 'info');
                                    engine.src = fb;
                                    engine.load();
                                    engine.play().catch(function(e){ showToast('Fallback failed', 'error'); });
                                    break;
                                }
                                hls.startLoad(); break;
                            case Hls.ErrorTypes.MEDIA_ERROR:
                                hls.recoverMediaError(); break;
                            default:
                                showToast('Stream error: ' + data.type + ' - try Download', 'error');
                                hls.destroy(); break;
                        }
                    }
                });
                // Fallback if HLS not needed? Safari native
            } else if (isHls && engine.canPlayType('application/vnd.apple.mpegurl')) {
                engine.src = srcUrl;
                engine.load();
                engine.play().then(function(){ updatePlayerBarState(true); }).catch(function(err){
                    console.warn('Native HLS play error:', err);
                    showToast('Playback failed', 'error');
                });
            } else {
                engine.src = srcUrl;
                engine.load();
                engine.play().then(function() {
                    updatePlayerBarState(true);
                }).catch(function(err) {
                    console.warn('Play stream error:', err);
                    // AbortError is often due to new load replacing previous, ignore
                    if (err && err.name === 'AbortError') return;
                    showToast('Playback failed: ' + (err.message || 'Unsupported format'), 'error');
                    // If direct file failed due to CORS, try via proxy (only if not already proxy/stream)
                    if (!srcUrl.includes('/api/browser/stream') && !srcUrl.includes('/api/browser/proxy-media')) {
                        var fallback = '/api/browser/stream?url=' + encodeURIComponent(streamUrl);
                        if (cookies) fallback += '&cookies=' + encodeURIComponent(cookies);
                        console.log('Retrying via proxy:', fallback);
                        cleanupHls();
                        engine.src = fallback;
                        engine.load();
                        engine.play().catch(function(e2){ console.warn('Proxy retry failed:', e2); });
                    } else if (srcUrl.includes('/api/browser/proxy-media') && isPageUrl) {
                        // proxy-media 403 -> fallback to stream
                        var fb2 = '/api/browser/stream?url=' + encodeURIComponent(streamUrl);
                        if (cookies) fb2 += '&cookies=' + encodeURIComponent(cookies);
                        if (srcUrl !== fb2) {
                            console.log('Proxy 403, retrying via stream:', fb2);
                            cleanupHls();
                            engine.src = fb2;
                            engine.load();
                            engine.play().catch(function(e2){ console.warn('Stream retry failed:', e2); });
                        }
                    }
                });
            }
        }

        // If URL is already direct file (mp4/webm/m3u8/mp3 etc), proxy via server to bypass CORS, with HLS handling
        if (isDirectFile) {
            var proxied = '/api/browser/proxy-media?url=' + encodeURIComponent(streamUrl);
            if (videoModalDownloadBtn) {
                videoModalDownloadBtn.href = proxied;
                videoModalDownloadBtn.setAttribute('download', safeTitle.replace(/[^a-zA-Z0-9._-]/g,'_') + (isAudio ? '.mp3' : '.mp4'));
            }
            // Use HLS flow if needed, proxied URL still ends with m3u8 so HLS will trigger
            if (isM3u8) {
                startPlayback(proxied, true);
            } else {
                startPlayback(proxied, false);
            }
            return;
        }

        // Page URL (TikTok, YouTube, etc.) => for platforms with known CDN 403 / DASH mux needs, skip resolve and go direct to server mux
        var skipResolve = streamUrl.includes('tiktok.com') || streamUrl.includes('youtube.com') || streamUrl.includes('youtu.be') || streamUrl.includes('instagram.com') || streamUrl.includes('facebook.com');
        if (skipResolve) {
            showToast('Fetching via server...', 'info');
            var directStream = '/api/browser/stream?url=' + encodeURIComponent(streamUrl);
            if (cookies) directStream += '&cookies=' + encodeURIComponent(cookies);
            if (videoModalDownloadBtn) {
                videoModalDownloadBtn.href = directStream;
                videoModalDownloadBtn.setAttribute('download', safeTitle.replace(/[^a-zA-Z0-9._-]/g,'_') + '.mp4');
            }
            startPlayback(directStream, false);
            return;
        }

        // Otherwise try resolve to direct CDN first for instant play, else fallback to server-side mux stream
        showToast('Resolving stream...', 'info');
        var resolveUrl = '/api/browser/resolve?url=' + encodeURIComponent(streamUrl);
        if (cookies) resolveUrl += '&cookies=' + encodeURIComponent(cookies);

        fetch(resolveUrl)
            .then(function(r){
                if (!r.ok) throw new Error('resolve failed');
                return r.json();
            })
            .then(function(data){
                if (data && data.url) {
                    var direct = data.url;
                    var directIsHls = /\.m3u8(\?|$)/i.test(direct);
                    // Proxy direct CDN via proxy-media to handle CORS & Range
                    var toPlay = '/api/browser/proxy-media?url=' + encodeURIComponent(direct);
                    if (videoModalDownloadBtn) {
                        videoModalDownloadBtn.href = toPlay;
                        videoModalDownloadBtn.setAttribute('download', safeTitle.replace(/[^a-zA-Z0-9._-]/g,'_') + (directIsHls ? '.m3u8' : (isAudio ? '.mp3' : '.mp4')));
                    }
                    // However keep original direct URL length check: if HLS, let HLS use proxied
                    startPlayback(toPlay, directIsHls);
                    showToast('Playing stream', 'success');
                } else {
                    throw new Error('no url');
                }
            })
            .catch(function(err){
                console.warn('Resolve failed, falling back to server mux stream:', err);
                showToast('Resolving via server transcoding...', 'info');
                var streamEndpoint = '/api/browser/stream?url=' + encodeURIComponent(streamUrl);
                if (cookies) streamEndpoint += '&cookies=' + encodeURIComponent(cookies);
                if (videoModalDownloadBtn) {
                    videoModalDownloadBtn.href = streamEndpoint;
                    videoModalDownloadBtn.setAttribute('download', safeTitle.replace(/[^a-zA-Z0-9._-]/g,'_') + '.mp4');
                }
                // stream endpoint already returns mp4 merged, not HLS
                if (isM3u8 || isMpds) {
                    startPlayback(streamEndpoint, true);
                } else {
                    startPlayback(streamEndpoint, false);
                }
            });
    }

    // Toast notifications
    function showToast(msg, type) {
        type = type || 'success';
        var container = document.getElementById('toastContainer');
        if (!container) return;
        var toast = document.createElement('div');
        toast.style.cssText = 'pointer-events:auto;min-width:280px;max-width:380px;padding:12px 14px;border-radius:12px;background:rgba(18,22,32,0.96);border:1px solid rgba(255,255,255,0.1);backdrop-filter:blur(16px);display:flex;align-items:center;gap:10px;box-shadow:0 10px 30px rgba(0,0,0,0.6);transform:translateY(8px);opacity:0;transition:all 0.25s ease;';

        var iconMap = { success: 'bi-check-circle-fill text-emerald-400', error: 'bi-exclamation-octagon-fill text-red-400', warning: 'bi-exclamation-triangle-fill text-amber-400', info: 'bi-info-circle-fill text-cyan-400' };
        toast.innerHTML = '<i class="bi ' + (iconMap[type] || 'bi-bell-fill text-primary') + ' text-[15px]"></i><span class="flex-1 text-[12px] text-white font-medium">' + escapeHtml(msg) + '</span><button type="button" class="text-on-surface-variant hover:text-white text-[16px] cursor-pointer">&times;</button>';

        toast.querySelector('button').addEventListener('click', function() { dismissToast(toast); });
        container.appendChild(toast);
        requestAnimationFrame(function() {
            toast.style.transform = 'translateY(0)';
            toast.style.opacity = '1';
        });
        setTimeout(function() { dismissToast(toast); }, 3800);
    }

    function dismissToast(t) {
        if (!t || t.classList.contains('dismissing')) return;
        t.classList.add('dismissing');
        t.style.opacity = '0';
        t.style.transform = 'translateY(8px)';
        setTimeout(function() { if (t.parentNode) t.parentNode.removeChild(t); }, 250);
    }

    function escapeHtml(text) {
        var d = document.createElement('div');
        d.appendChild(document.createTextNode(text || ''));
        return d.innerHTML;
    }

    // Initialization
    // Apply initial theme class based on stored preference or system setting
    (function() {
        var themeMode = localStorage.getItem('kv_theme_mode') || 'system';
        var systemPrefersDark = (window.matchMedia && window.matchMedia('(prefers-color-scheme: dark)').matches);
        var initialTheme = (themeMode === 'system') ? (systemPrefersDark ? 'dark' : 'light') : themeMode;

        if (initialTheme === 'light') {
            document.body.classList.add('theme-light');
            document.documentElement.classList.add('light');
            document.documentElement.classList.remove('dark');
        } else {
            document.body.classList.add('theme-dark', 'theme-simple');
            document.documentElement.classList.add('dark');
            document.documentElement.classList.remove('light');
        }
    })();
    applyTheme(currentThemeMode);
    applyViewMode(currentViewMode);
    applyHistoryViewMode(currentHistoryViewMode);
    fetchQueueTasks();
    fetchGallery();
    initSSE();
    updateInputDetection();
    updateCookieBadge();
    checkYtDlpVersionInfo();
    setFilter(currentFilter);
})();
