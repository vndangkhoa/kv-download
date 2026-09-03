#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# KV Download — High-Performance Media Downloader Launcher
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# Defaults
PORT="${PORT:-9292}"
DOWNLOAD_DIR="${MR_DOWNLOAD_DIR:-./downloads}"
OPEN_BROWSER=false
FAST_BUILD=false

# ANSI Colors
BOLD="\033[1m"
CYAN="\033[36m"
GREEN="\033[32m"
YELLOW="\033[33m"
RED="\033[31m"
MAGENTA="\033[35m"
RESET="\033[0m"

show_help() {
  echo -e "${BOLD}${CYAN}KV Download Launcher${RESET}"
  echo ""
  echo "Usage:"
  echo "  ./run.sh [options]"
  echo ""
  echo "Options:"
  echo "  -p, --port <number>       Set web server port (default: ${PORT})"
  echo "  -d, --dir <path>          Set media download directory (default: ${DOWNLOAD_DIR})"
  echo "  -b, --build               Compile binary before starting for maximum performance"
  echo "  -o, --open                Open web UI in default browser upon launch"
  echo "  -h, --help                Show this help message"
  echo ""
  echo "Environment Variables:"
  echo "  PORT                      Web server port (default: 9292)"
  echo "  MR_DOWNLOAD_DIR           Target folder for downloaded files"
  echo "  MR_COOKIES_BROWSER        Browser name for auto-cookie extraction (e.g. chrome, edge)"
  echo ""
  echo "Examples:"
  echo "  ./run.sh"
  echo "  ./run.sh --port 8080 --dir ~/Downloads"
  echo "  ./run.sh --build --open"
  exit 0
}

# Parse Arguments
while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--port)
      PORT="$2"
      shift 2
      ;;
    -d|--dir|--download-dir)
      DOWNLOAD_DIR="$2"
      shift 2
      ;;
    -b|--build)
      FAST_BUILD=true
      shift
      ;;
    -o|--open)
      OPEN_BROWSER=true
      shift
      ;;
    -h|--help)
      show_help
      ;;
    *)
      echo -e "${RED}[ERROR] Unknown argument: $1${RESET}" >&2
      show_help
      ;;
  esac
done

# Dependency Validation
check_command() {
  local cmd="$1"
  local install_tip="$2"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo -e "${RED}[ERROR] Required dependency '$cmd' is not installed or not in PATH.${RESET}" >&2
    echo -e "${YELLOW}Tip: $install_tip${RESET}" >&2
    exit 1
  fi
}

check_command "go" "Install Go 1.21+ from https://go.dev/dl/"
check_command "yt-dlp" "Install yt-dlp via: pip install -U yt-dlp"
check_command "ffmpeg" "Install ffmpeg via your package manager (e.g. sudo apt install ffmpeg)"

# Create required directory hierarchy (organized: videos/music/photos)
mkdir -p "$DOWNLOAD_DIR/videos" "$DOWNLOAD_DIR/music" "$DOWNLOAD_DIR/photos" "$DOWNLOAD_DIR/other"
mkdir -p "$DOWNLOAD_DIR/json"

# Export runtime environment
export PORT="$PORT"
export MR_DOWNLOAD_DIR="$DOWNLOAD_DIR"
export MR_IMPERSONATE="${MR_IMPERSONATE:-chrome}"

# Get Local IP Address for LAN access display
LOCAL_IP="127.0.0.1"
if command -v hostname >/dev/null 2>&1; then
  LOCAL_IP="$(hostname -I 2>/dev/null | awk '{print $1}' || echo "127.0.0.1")"
fi

# Print Startup Banner
echo -e "${CYAN}================================================================${RESET}"
echo -e "${BOLD}${MAGENTA}   ██╗  ██╗██╗   ██╗    ██████╗  ██████╗ ██╗    ██╗███╗   ██╗${RESET}"
echo -e "${BOLD}${MAGENTA}   ██║ ██╔╝██║   ██║    ██╔══██╗██╔═══██╗██║    ██║████╗  ██║${RESET}"
echo -e "${BOLD}${MAGENTA}   █████╔╝ ██║   ██║    ██║  ██║██║   ██║██║ █╗ ██║██╔██╗ ██║${RESET}"
echo -e "${BOLD}${MAGENTA}   ██╔═██╗ ╚██╗ ██╔╝    ██║  ██║██║   ██║██║███╗██║██║╚██╗██║${RESET}"
echo -e "${BOLD}${MAGENTA}   ██║  ██╗ ╚████╔╝     ██████╔╝╚██████╔╝╚███╔███╔╝██║ ╚████║${RESET}"
echo -e "${BOLD}${MAGENTA}   ╚═╝  ╚═╝  ╚═══╝      ╚═════╝  ╚═════╝  ╚══╝╚══╝ ╚═╝  ╚═══╝${RESET}"
echo -e "${CYAN}================================================================${RESET}"
echo -e "${GREEN}✓ Status:${RESET}        Ready to launch"
echo -e "${GREEN}✓ Local URL:${RESET}     ${BOLD}http://localhost:${PORT}${RESET}"
echo -e "${GREEN}✓ Network URL:${RESET}   ${BOLD}http://${LOCAL_IP}:${PORT}${RESET}"
echo -e "${GREEN}✓ Downloads:${RESET}     ${BOLD}${DOWNLOAD_DIR}${RESET}"
echo -e "${GREEN}✓ yt-dlp:${RESET}       $(yt-dlp --version 2>/dev/null || echo 'detected')"
echo -e "${GREEN}✓ ffmpeg:${RESET}       $(ffmpeg -version 2>/dev/null | head -n 1 | awk '{print $3}' || echo 'detected')"
echo -e "${CYAN}----------------------------------------------------------------${RESET}"
echo -e "${YELLOW}Press Ctrl+C to gracefully stop the server.${RESET}"
echo -e "${CYAN}================================================================${RESET}"
echo ""

# Optional browser opener in background
if [ "$OPEN_BROWSER" = true ]; then
  (
    sleep 1.2
    if command -v xdg-open >/dev/null 2>&1; then
      xdg-open "http://localhost:${PORT}" >/dev/null 2>&1 || true
    elif command -v open >/dev/null 2>&1; then
      open "http://localhost:${PORT}" >/dev/null 2>&1 || true
    fi
  ) &
fi

# Execute application
if [ "$FAST_BUILD" = true ]; then
  echo -e "${CYAN}[KV Download] Compiling binary for high performance...${RESET}"
  go build -o kv-download src/main.go
  exec ./kv-download
else
  exec go run ./src
fi