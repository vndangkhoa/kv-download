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
  <a href="#cookies-optional">Cookies</a> &bull;
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

### Synology NAS

1. Open **Container Manager** (or Docker on older DSM)
2. Create a new project/stack:

```yaml
services:
  kv-download:
    image: vndangkhoa/kv-download:latest
    container_name: kv-download
    restart: unless-stopped
    ports:
      - "9292:9292"
    volumes:
      - /volume2/docker/kv-download/downloads:/download
      - /volume2/docker/kv-download/cookies.txt:/app/cookies.txt:ro
    environment:
      - TZ=Asia/Ho_Chi_Minh
```

3. Place `cookies.txt` in `/volume2/docker/kv-download/`
4. Access at `http://NAS_IP:9292`

#### Clean up downloads

```bash
# SSH into NAS, then delete all downloaded files
rm -rf /volume2/docker/kv-download/downloads/*

# Or delete files older than 7 days
find /volume2/docker/kv-download/downloads/ -type f -mtime +7 -delete
```

To automate cleanup, add a cron job via **Task Scheduler** in DSM:

```bash
# Run daily at 3 AM, delete files older than 7 days
0 3 * * * find /volume2/docker/kv-download/downloads/ -type f -mtime +7 -delete
```

---

## Cookies (optional)

Some platforms (TikTok, Instagram, Twitter/X) require authentication to download private or age-restricted content. Create a `cookies.txt` file in Netscape format:

### Option 1: Browser extension (recommended)

1. Install [Get cookies.txt LOCALLY](https://chromewebstore.google.com/detail/get-cookiestxt-locally/cclelndahbckbenkjhflpdbgdldlbecc) (Chrome) or [cookies.txt](https://addons.mozilla.org/en-US/firefox/addon/cookies-txt/) (Firefox)
2. Log in to the platform (TikTok, Instagram, etc.)
3. Click the extension icon and export cookies as `cookies.txt`
4. Place the file in the project root (same directory as `run.sh`)

### Option 2: yt-dlp browser extraction (recommended)

yt-dlp can extract cookies directly from your installed browser.

#### Windows

```cmd
:: Install secretstorage dependency (run once)
pip install secretstorage

:: Extract cookies from Chrome (browser must be closed)
yt-dlp --cookies-from-browser chrome --cookies cookies.txt "https://www.youtube.com/"

:: Or from Firefox
yt-dlp --cookies-from-browser firefox --cookies cookies.txt "https://www.youtube.com/"
```

#### Linux

```bash
# Install secretstorage dependency (Ubuntu/Debian)
sudo apt install python3-secretstorage

# Extract cookies from Chrome (browser must be closed)
yt-dlp --cookies-from-browser chrome --cookies cookies.txt "https://www.youtube.com/"

# Or from Firefox
yt-dlp --cookies-from-browser firefox --cookies cookies.txt "https://www.youtube.com/"
```

#### macOS

```bash
# Install secretstorage dependency
pip3 install secretstorage

# Extract cookies from Chrome (browser must be closed)
yt-dlp --cookies-from-browser chrome --cookies cookies.txt "https://www.youtube.com/"

# Or from Safari
yt-dlp --cookies-from-browser safari --cookies cookies.txt "https://www.youtube.com/"

# Or from Firefox
yt-dlp --cookies-from-browser firefox --cookies cookies.txt "https://www.youtube.com/"
```

> **Note:** The browser must be fully closed (not running in background) when extracting cookies.

### Using cookies.txt

**Local:**
```bash
# Place cookies.txt in the project root, run.sh will pass it automatically
./run.sh
```

**Docker:**
```bash
docker run -d -p 9292:9292 \
  -v ./downloads:/download \
  -v ./cookies.txt:/app/cookies.txt:ro \
  vndangkhoa/kv-download:latest
```

**Docker Compose:**
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
      - ./cookies.txt:/app/cookies.txt:ro
    environment:
      - TZ=Asia/Ho_Chi_Minh
```

> **Note:** `cookies.txt` contains session tokens. Never commit it to git or share it publicly.

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
