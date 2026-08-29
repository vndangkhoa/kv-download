# Changelog

All notable changes to the **KV Download** project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

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
