# KV-Download (Media Roller)

A mobile friendly tool for downloading videos from social media.
The backend is a Golang server that will take a URL (YouTube, Reddit, Twitter, TikTok, Instagram, etc),
download the video file, and return a URL to directly download the video. The video will be transcoded to produce a single mp4 file.

This is built on [yt-dlp](https://github.com/yt-dlp/yt-dlp). yt-dlp will auto update every 12 hours to make sure it's running the latest nightly build.

Note: This was written to run on a home network and should not be exposed to public traffic. There's no auth.

![Screenshot 1](https://i.imgur.com/lxwf1qU.png)

![Screenshot 2](https://i.imgur.com/TWAtM7k.png)

# Running

Make sure you have [yt-dlp](https://github.com/yt-dlp/yt-dlp) and [FFmpeg](https://github.com/FFmpeg/FFmpeg) installed then pull the repo and run:
```bash
./run.sh
```
Or for docker locally:
```bash
./docker-build.sh
./docker-run.sh
```

## Docker Image

The Docker image is available from the Forgejo container registry:

```bash
docker pull git.khoavo.myds.me/vndangkhoa/kv-download:latest
```

### Deploy on Synology NAS

1. Load the image (if using local transfer):
```bash
docker load -i kv-download.tar
```

2. Run with docker-compose:
```yaml
services:
  kv-download:
    image: git.khoavo.myds.me/vndangkhoa/kv-download:latest
    container_name: kv-download
    restart: unless-stopped
    ports:
      - "9292:9292"
    volumes:
      - /volume2/docker/kv-download/download:/download
    environment:
      - MR_DOWNLOAD_DIR=/download
      - TZ=Asia/Ho_Chi_Minh
```

```bash
docker compose up -d
```

### Build Your Own Image

```bash
docker build -t kv-download:latest --platform linux/amd64 .
```

## Docker Environment Variables
* `MR_DOWNLOAD_DIR` where videos are saved. Defaults to `/download`
* `MR_PROXY` will pass the value to yt-dlp with the `--proxy` argument. Defaults to empty

## File Structure

After downloading videos, files are organized as follows:
```
/download/
├── <hash>/
│   └── <video-id>.mp4        # Video files
└── json/
    └── <video-id>.info.json   # Metadata files (separate folder)
```

# API

To download a video directly, use the API endpoint:

```
/api/download?url=SOME_URL
```

Create a bookmarklet, allowing one click downloads (From a PC):

```
javascript:(location.href="http://127.0.0.1:9292/fetch?url="+encodeURIComponent(location.href));
```

# Integrating with mobile

After you have your server up, install this shortcut. Update the endpoint to your server address by editing the shortcut before running it. 

https://www.icloud.com/shortcuts/d3b05b78eb434496ab28dd91e1c79615

# Unraid

media-roller is available in Unraid and can be found on the "Apps" tab by searching its name.
