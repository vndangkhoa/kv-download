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
docker pull git.khoavo.myds.me/vndangkhoa/kv-download:v1
```

### Build Your Own Image

```bash
docker build -t git.khoavo.myds.me/vndangkhoa/kv-download:v1 --platform linux/amd64 .
docker push git.khoavo.myds.me/vndangkhoa/kv-download:v1
```

## Deploy on Synology NAS

### Method 1: Docker Compose (Recommended)

1. SSH into your Synology NAS

2. Create a project directory and copy the `docker-compose.yml` there

3. Run:
```bash
cd /path/to/project
docker compose up -d
```

### Method 2: Synology Container Manager (Docker GUI)

1. Open **Container Manager** in DSM

2. Go to **Registry** → **Add** → enter:
   - Server: `git.khoavo.myds.me`
   - Username: `vndangkhoa`
   - Password: `Thieugia19`

3. Search for `vndangkhoa/kv-download` and download the `v1` tag

4. Go to **Image** → select `kv-download` → **Create Container**

5. Configure the container:
   - **Container Name**: `kv-download`
   - **Port Settings**: Local `9292` → Container `9292`
   - **Volume Settings**:
     - Add folder: `/volume2/docker/kv-download/download` → Mount path `/download`
   - **Environment Variables**:
     - `MR_DOWNLOAD_DIR` = `/download`
     - `TZ` = `Asia/Ho_Chi_Minh`
   - **Restart Policy**: `Always restart the container`

6. Click **Done** to start the container

### Method 3: Docker CLI

```bash
docker run -d \
  --name kv-download \
  --restart unless-stopped \
  -p 9292:9292 \
  -v ./download:/download \
  -e MR_DOWNLOAD_DIR=/download \
  -e TZ=Asia/Ho_Chi_Minh \
  git.khoavo.myds.me/vndangkhoa/kv-download:v1
```

## Docker Environment Variables
* `MR_DOWNLOAD_DIR` where videos are saved. Defaults to `/download`
* `MR_PROXY` will pass the value to yt-dlp with the `--proxy` argument. Defaults to empty

## File Structure

After downloading videos, files are organized as follows:
```
download/
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
