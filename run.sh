#!/usr/bin/env bash
set -euo pipefail

PORT="${PORT:-9292}"
DOWNLOAD_DIR="${MR_DOWNLOAD_DIR:-./download}"

log() {
  echo "[kv-download] $*"
}

die() {
  echo "[kv-download] ERROR: $*" >&2
  exit 1
}

# Check dependencies
command -v go >/dev/null 2>&1 || die "go is not installed"
command -v yt-dlp >/dev/null 2>&1 || die "yt-dlp is not installed (https://github.com/yt-dlp/yt-dlp)"
command -v ffmpeg >/dev/null 2>&1 || die "ffmpeg is not installed (https://github.com/FFmpeg/FFmpeg)"

# Prepare the download directory
mkdir -p "$DOWNLOAD_DIR"

log "Running on http://localhost:${PORT}"
log "Download directory: $DOWNLOAD_DIR"
log "Press Ctrl+C to stop (graceful shutdown)"

export MR_DOWNLOAD_DIR="$DOWNLOAD_DIR"
exec go run ./src