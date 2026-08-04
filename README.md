<p align="center">
  <img src="static/images/screenshot.png" alt="KV Download" width="100%">
</p>

<h1 align="center">KV Download</h1>

<p align="center">
  A fast, mobile-friendly video downloader for social media platforms.<br>
  Built with <a href="https://go.dev/">Go</a> + <a href="https://github.com/yt-dlp/yt-dlp">yt-dlp</a>.
</p>

<p align="center">
  <a href="#features">Features</a> &bull;
  <a href="#quick-start">Quick Start</a> &bull;
  <a href="#docker">Docker</a> &bull;
  <a href="#api">API</a> &bull;
  <a href="#configuration">Configuration</a>
</p>

---

## Features

- **Multi-URL** &mdash; paste multiple links, download them all in sequence
- **Download queue** &mdash; real-time per-URL status with progress bar
- **ZIP export** &mdash; bundle all videos into a single download
- **Responsive** &mdash; works on desktop, tablet, and mobile
- **Video preview** &mdash; watch before you save
- **Auto-update** &mdash; yt-dlp refreshes every 6 hours
- **Dark mode** &mdash; sleek glassmorphism UI

Supported platforms: YouTube, TikTok, Instagram, Twitter/X, Vimeo, Reddit, and [many more](https://github.com/yt-dlp/yt-dlp/blob/master/supportedsites.md).

---

## Quick Start

**Prerequisites:** [Go](https://go.dev/dl/), [yt-dlp](https://github.com/yt-dlp/yt-dlp#installation), [FFmpeg](https://ffmpeg.org/download.html)

```bash
git clone https://github.com/vndangkhoa/kv-download.git
cd kv-download
./run.sh
```

Then open **http://localhost:9292**

---

## Docker

### Docker Hub

```bash
docker pull vndangkhoa/kv-download:latest
docker run -d -p 9292:9292 -v ./downloads:/download vndangkhoa/kv-download:latest
```

### Forgejo Registry

```bash
docker pull git.khoavo.myds.me/vndangkhoa/kv-download:latest
docker run -d -p 9292:9292 -v ./downloads:/download git.khoavo.myds.me/vndangkhoa/kv-download:latest
```

### Docker Compose

```yaml
services:
  kv-download:
    image: vndangkhoa/kv-download:latest
    container_name: kv-download
    restart: unless-stopped
    ports:
      - "9292:9292"
    volumes:
      - ./downloads:/download
    environment:
      - TZ=Asia/Ho_Chi_Minh
```

```bash
docker compose up -d
```

### Build Locally

```bash
./docker-build.sh
./docker-run.sh
```

---

## Configuration

| Variable | Description | Default |
|---|---|---|
| `MR_DOWNLOAD_DIR` | Directory where videos are saved | `/download` (Docker) / `downloads/` (local) |
| `MR_PROXY` | Proxy URL passed to yt-dlp via `--proxy` | _(empty)_ |

---

## API

### Get video info (JSON)

```
GET /api/info?url=VIDEO_URL
```

Returns:

```json
{
  "media": [
    {
      "Id": "abc123/video.mp4",
      "Name": "video.mp4",
      "SizeInBytes": 1048576,
      "HumanSize": "1.0 MB"
    }
  ],
  "error": ""
}
```

### Download a video

```
GET /download?id=VIDEO_ID
```

Streams the video file directly.

### Download all as ZIP

```
GET /download/zip?id=ID1&id=ID2&id=ID3
```

Bundles multiple videos into a single `.zip` file.

### Bookmarklet

Drag this to your bookmarks bar for one-click downloads from any page:

```
javascript:(location.href="http://127.0.0.1:9292/fetch?url="+encodeURIComponent(location.href));
```

---

## File Structure

```
<download-dir>/
├── <hash>/
│   ├── <video-id>.mp4
│   └── <video-id>.mp4
└── json/
    └── <video-id>.info.json
```

---

## License

[MIT](LICENSE)
