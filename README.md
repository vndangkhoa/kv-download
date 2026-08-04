# KV Download

A responsive web app for downloading videos from social media platforms. Built with Go and [yt-dlp](https://github.com/yt-dlp/yt-dlp).

Supports YouTube, TikTok, Instagram, Twitter/X, Vimeo, Reddit, and more.

![Screenshot](static/images/screenshot.png)

## Features

- Download single or multiple videos at once
- Responsive design (desktop, tablet, mobile)
- Inline progress bar with real-time status
- Video preview before saving
- Auto-updates yt-dlp every 6 hours
- Dark mode UI

## Quick Start

Prerequisites: [Go](https://go.dev/), [yt-dlp](https://github.com/yt-dlp/yt-dlp), [FFmpeg](https://ffmpeg.org/)

```bash
git clone <repo-url> && cd kv-download
./run.sh
```

Open http://localhost:9292

## Docker

```bash
docker compose up -d
```

Or build locally:

```bash
./docker-build.sh
./docker-run.sh
```

### Docker Environment Variables

| Variable | Description | Default |
|---|---|---|
| `MR_DOWNLOAD_DIR` | Where videos are saved | `/download` |
| `MR_PROXY` | Proxy for yt-dlp (`--proxy`) | empty |

## API

Get video info as JSON:

```
GET /api/info?url=VIDEO_URL
```

Download a video directly:

```
GET /api/download?url=VIDEO_URL
```

Download by ID (after fetching info):

```
GET /download?id=VIDEO_ID
```

### Bookmarklet

Drag this to your bookmarks bar for one-click downloads:

```
javascript:(location.href="http://127.0.0.1:9292/fetch?url="+encodeURIComponent(location.href));
```

## File Structure

```
<download-dir>/
├── <hash>/
│   └── <video-id>.mp4
└── json/
    └── <video-id>.info.json
```

## License

MIT
