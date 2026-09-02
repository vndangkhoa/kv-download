# Changelog

All notable changes to the **KV Download** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [1.0.12] - 2026-09-02

### 🎬 TikTok & YouTube Channel / Playlist Scanner
- **Live Creator Channel Discovery**: Fast flat-playlist inspection for TikTok creators, YouTube channels, and playlists returning titles, high-resolution covers, durations, view counts, and uploader info.
- **Progressive SSE Live Streaming (`/api/scan/stream`)**: Real-time item streaming with `--lazy-playlist` and unbuffered I/O.
- **Live Video Counter**: Animated increasing video counter showing discovered items in real time.
- **Instant "Stop & Show Videos"**: One-click action to halt channel scanning immediately and open all discovered videos in the selection grid for batch downloading.
- **Batch Pagination (`/api/scan?start=...&limit=...`)**: Quick-fetch recent videos in batches with a **Load More Videos (+24)** button.
- **Fault-Tolerant Scraping**: Query parameter sanitization and `--ignore-errors` fault tolerance for creator profiles with >1,000 videos.

### 📚 Channels & Playlists Gallery View
- **Collection Grouping**: Added a dedicated **Channels, Playlists & Albums** library view grouping completed downloads by channel, creator, and album.
- **4-Thumbnail Collage Cards**: High-res visual collages with creator handles, total track counts, and one-click **Play All** audio/video playback.

### 🎨 Static Asset & CSS MIME Type Fixes
- **URL Path Sanitization**: Fixed `staticHandler` in `src/main.go` to properly serve versioned stylesheet assets (`style.css?v=2.4`) with `Content-Type: text/css`.
- **Google Font Optimization**: Corrected variable font parameter syntax for Material Symbols Outlined.

---

## [1.0.11] - 2026-09-02

### 🌐 In-App Media Sniffer & Web Browser Sandbox
- **Universal Media Sniffing**: Embedded CORS-bypassed proxy browser (`/api/browser/proxy`) with deep sniffing for M3U8 HLS playlists, direct MP4/WebM videos, and audio streams (`/api/browser/sniff`).
- **One-Tap Download Bubble**: Corner action bubble inside the in-app browser for instant queueing of sniffed media.
- **Branded Start Experience**: Interactive dashboard overlay presenting core engine capabilities when the browser is opened.

### 📱 Mobile-First Ergonomics & Viewport Overhaul
- **Strict Viewport Lock**: Applied `overflow-x: hidden` and `max-width: 100vw` constraints globally to prevent horizontal layout drift on narrow screens (e.g., Galaxy S25, iPhone).
- **Responsive Address Bar**: Smart toolbar layout that automatically wraps the URL address bar into a full-width row on mobile viewports, preventing overlap with navigation buttons.
- **Grid & Card Containment**: Refactored all task containers with `minmax(0, 1fr)` and `min-w-0` so titles and code blocks truncate cleanly without stretching cards.
- **Whitespace & Dock Polish**: Compacted bottom padding to seamlessly fit above the fixed floating mobile dock.

### ⚡ Aria2 JSON-RPC Protocol & REST APIs
- **Aria2 JSON-RPC 2.0 Server**: Full compatibility on `/jsonrpc` and `/rpc` for Aria2 browser extensions, YAAW, Camellia, and automation scripts.
- **Interactive REST Documentation**: Built-in API guide with cURL examples for queue ingestion, metadata inspection, and SSE live telemetry.

### 🎬 Media Playback & Dynamic Thumbnails
- **On-Demand Thumbnail Generation**: FFmpeg integration generating dynamic video frames on `/thumbnail` for local saved media.
- **Floating Mini-Player**: Responsive audio/video player dock with scrubber, skip controls, and MediaSession integration.
- **Multi-Platform Cookie Manager**: Redesigned mobile-responsive modal with guides for desktop extensions, Safari bookmarklets, and Android Kiwi.

---

## [1.0.7] - 2026-08-29

### 🎨 UI & Layout Overhaul (MotionSites.ai Design Language)
- **Bento Grid Architecture**: Re-engineered desktop dashboard into a modular Bento Grid with a Hero Download Console, Engine Telemetry card, and High-Speed Platforms showcase.
- **Dynamic Platform Auto-Detection**: Live detection badge that highlights supported platforms (YouTube, TikTok, Instagram, Twitter/X, Reddit, Facebook, Vimeo, SoundCloud, Twitch, Bilibili) as URLs are typed or pasted.
- **One-Tap Clipboard Paste**: Integrated browser Clipboard API button (`📋 Paste`) to quickly paste links into the queue.
- **Expanded Quality / Format Selector**: Fast quality switching pills for `Auto (Best)`, `1080p FHD`, `720p HD`, `480p SD`, `Audio MP3 (320kbps)`, and `Audio M4A`.
- **Luminous CTA Button**: High-energy gradient submit button with animated light shimmer effects and dynamic batch URL count badge.
- **Real-Time Telemetry & Batch Controls**: Live engine speed monitor, active task gauges, completed count, and one-click `Clear Done` / `Retry Failed` actions.

### 📱 Mobile-First Ergonomics
- **Floating Bottom Navigation Dock**: Thumb-friendly glassmorphism dock fixed at the bottom with safe-area padding (`env(safe-area-inset-bottom)`) and live badge counters.
- **Touch-Friendly Controls**: 16px minimum font size to prevent iOS auto-zoom, full-width inputs, and swipeable format carousel.

### ⚡ Media Watcher & Dual Themes
- **Built-in Cinema Media Player**: Direct inline HTML5 video and audio playback for completed downloads.
- **View Mode Switcher**: Toggle seamlessly between **Bento Grid View** and **Compact List View**.
- **Enhanced Matrix Cyberdeck Theme**: Upgraded 60fps digital rain with Japanese katakana glyphs, glow leader characters, and CRT scanlines.
- **Motion Liquid Glass Theme**: Translucent frosted surfaces with multi-layered dynamic aurora mesh glow.

---

## [1.0.6] - 2026-08-28

### Added
- MeTube-style real-time task watcher with SSE progress streaming.
- Per-task progress bar, speed, ETA, and size tracking.
- Synology NAS SPK package builder and DSM integration.
- Fast remuxing to MP4 and auto-update mechanism for yt-dlp nightly releases.
- Browser impersonation support for anti-bot protected platforms.
