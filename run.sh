#!/usr/bin/env bash
set -euo pipefail

# ==============================================================================
# KV Download — High-Performance Media Downloader Launcher
# ==============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ==============================================================================
# Go Runtime Detection (works with snap, system, or manual installs)
# ==============================================================================
if command -v go >/dev/null 2>&1; then
    GO_CMD="$(command -v go)"
elif [ -x "/snap/go/current/bin/go" ]; then
    GO_CMD="/snap/go/current/bin/go"
elif [ -x "/usr/lib/go-1.24/bin/go" ]; then
    GO_CMD="/usr/lib/go-1.24/bin/go"
elif [ -x "/usr/local/go/bin/go" ]; then
    GO_CMD="/usr/local/go/bin/go"
else
    echo "ERROR: Go is not installed or not in PATH."
    echo "Install Go 1.23+ from https://go.dev/dl/"
    exit 1
fi

# Verify Go works (some snap installs have DBus issues)
if ! "$GO_CMD" version >/dev/null 2>&1; then
    echo "WARN: Primary Go path has issues, trying fallbacks..."
    for fallback in /snap/go/current/bin/go /usr/lib/go-1.24/bin/go /usr/local/go/bin/go; do
        if [ -x "$fallback" ] && "$fallback" version >/dev/null 2>&1; then
            GO_CMD="$fallback"
            echo "FOUND Go at: $GO_CMD"
            break
        fi
    done
    if ! "$GO_CMD" version >/dev/null 2>&1; then
        echo "ERROR: No working Go installation found."
        exit 1
    fi
fi

echo "Using Go: $($GO_CMD version | head -1)"

# ==============================================================================
# Defaults
# ==============================================================================
PORT="${PORT:-9292}"
DOWNLOAD_DIR="${MR_DOWNLOAD_DIR:-./downloads}"
OPEN_BROWSER=false
FAST_BUILD=false
BUILD_MODE="auto"  # auto | dev | release

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
  echo "  -b, --build               Compile binary before starting"
  echo "  -r, --release             Build with optimization (release mode)"
  echo "  -o, --open                Open web UI in default browser upon launch"
  echo "  -v, --view <mode>         Build mode: dev (faster) or release (optimized)"
  echo "  -h, --help                Show this help message"
  echo ""
  echo "Environment Variables:"
  echo "  PORT                      Web server port (default: 9292)"
  echo "  MR_DOWNLOAD_DIR           Target folder for downloaded files"
  echo "  MR_COOKIES_BROWSER        Browser name for auto-cookie extraction"
  echo "  MR_IMPERSONATE            User-agent impersonation (chrome/edge/safari)"
  echo ""
  echo "Examples:"
  echo "  ./run.sh                          # Quick dev mode"
  echo "  ./run.sh --build                  # Build + run"
  echo "  ./run.sh --build --release        # Production build"
  echo "  ./run.sh --build --release --open # Build, run & open browser"
  echo ""
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
    -r|--release)
      BUILD_MODE="release"
      shift
      ;;
    -v|--view)
      BUILD_MODE="$2"
      shift 2
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

# ==============================================================================
# Dependency Validation
# ==============================================================================
check_command() {
  local cmd="$1"
  local label="$2"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo -e "${RED}[ERROR] Required dependency '$label' ($cmd) not found.${RESET}" >&2
    return 1
  fi
  return 0
}

MISSING_DEPS=0
check_command "yt-dlp" "yt-dlp" || { echo -e "${YELLOW}Tip: pip install -U yt-dlp${RESET}" >&2; MISSING_DEPS=1; }
check_command "ffmpeg" "ffmpeg" || { echo -e "${YELLOW}Tip: sudo apt install ffmpeg${RESET}" >&2; MISSING_DEPS=1; }

if [ "$MISSING_DEPS" -eq 1 ]; then
    echo -e "${YELLOW}[WARN] Some dependencies missing. The app may not work correctly.${RESET}" >&2
fi

# ==============================================================================
# Create required directories
# ==============================================================================
mkdir -p "$DOWNLOAD_DIR/videos" "$DOWNLOAD_DIR/music" "$DOWNLOAD_DIR/photos" "$DOWNLOAD_DIR/other"
mkdir -p "$DOWNLOAD_DIR/json" "$DOWNLOAD_DIR/thumbnails"

# ==============================================================================
# Export runtime environment
# ==============================================================================
export PORT="$PORT"
export MR_DOWNLOAD_DIR="$DOWNLOAD_DIR"
export MR_IMPERSONATE="${MR_IMPERSONATE:-chrome}"

# Get Local IP Address for LAN access
LOCAL_IP="127.0.0.1"
if command -v hostname >/dev/null 2>&1; then
    LOCAL_IP="$(hostname -I 2>/dev/null | awk '{print $1}' || echo "127.0.0.1")"
fi

# ==============================================================================
# Print Startup Banner
# ==============================================================================
echo -e "${CYAN}================================================================${RESET}"
echo -e "${BOLD}${MAGENTA}   ██╗  ██╗██╗   ██╗    ██████╗  ██████╗ ██╗    ██╗███╗   ██╗${RESET}"
echo -e "${BOLD}${MAGENTA}   ██║ ██╔╝██║   ██║    ██╔══██╗██╔═══██╗██║    ██║████╗  ██║${RESET}"
echo -e "${BOLD}${MAGENTA}   █████╔╝ ██║   ██║    ██║  ██║██║   ██║██║ █╗ ██║██╔██╗ ██║${RESET}"
echo -e "${BOLD}${MAGENTA}   ██╔═██╗ ╚██╗ ██╔╝    ██║  ██║██║   ██║██║███╗██║██║╚██╗██║${RESET}"
echo -e "${BOLD}${MAGENTA}   ██║  ██╗ ╚████╔╝     ██████╔╝╚██████╔╝╚███╔███╔╝██║ ╚████║${RESET}"
echo -e "${BOLD}${MAGENTA}   ╚═╝  ╚═╝  ╚═══╝      ╚═════╝  ╚═════╝  ╚══╝╚══╝ ╚═╝  ╚═══╝${RESET}"
echo -e "${CYAN}================================================================${RESET}"
echo ""

# Build configuration
BUILD_ARGS=""
BUILD_DESC=""
if [ "$BUILD_MODE" = "release" ]; then
    BUILD_ARGS='-ldflags="-w -s"'
    BUILD_DESC="release (optimized)"
elif [ "$BUILD_MODE" = "dev" ]; then
    BUILD_DESC="development (fast)"
else
    # auto detect
    if [ "$FAST_BUILD" = true ]; then
        BUILD_ARGS='-ldflags="-w -s"'
        BUILD_DESC="release (optimized)"
    else
        BUILD_DESC="development (fast)"
    fi
fi

echo -e "${GREEN}✓ Go:${RESET}          $($GO_CMD version | head -1)"
echo -e "${GREEN}✓ Port:${RESET}        ${BOLD}${PORT}${RESET}"
echo -e "${GREEN}✓ Build Mode:${RESET}  ${BOLD}${BUILD_DESC}${RESET}"
echo -e "${GREEN}✓ Local URL:${RESET}   ${BOLD}http://localhost:${PORT}${RESET}"
echo -e "${GREEN}✓ Network URL:${RESET} ${BOLD}http://${LOCAL_IP}:${PORT}${RESET}"
echo -e "${GREEN}✓ Downloads:${RESET}   ${BOLD}${DOWNLOAD_DIR}${RESET}"

if command -v yt-dlp >/dev/null 2>&1; then
    echo -e "${GREEN}✓ yt-dlp:${RESET}     $(yt-dlp --version)"
fi
if command -v ffmpeg >/dev/null 2>&1; then
    echo -e "${GREEN}✓ ffmpeg:${RESET}     $(ffmpeg -version 2>/dev/null | head -n 1 | cut -d' ' -f3)"
fi
echo -e "${CYAN}----------------------------------------------------------------${RESET}"
echo -e "${YELLOW}Press Ctrl+C to gracefully stop.${RESET}"
echo -e "${CYAN}================================================================${RESET}"
echo ""

# ==============================================================================
# Build if requested
# ==============================================================================
if [ "$FAST_BUILD" = true ]; then
    echo -e "${CYAN}[BUILD] Compiling KV Download...${RESET}"
    if "$GO_CMD" build $BUILD_ARGS -o kv-download src/main.go 2>&1; then
        echo -e "${GREEN}[BUILD] ✓ Binary ready: ./kv-download${RESET}"
        chmod +x kv-download
    else
        echo -e "${RED}[BUILD] ✗ Build failed. Falling back to go run...${RESET}" >&2
        FAST_BUILD=false
    fi
fi

# ==============================================================================
# Optional browser opener
# ==============================================================================
if [ "$OPEN_BROWSER" = true ]; then
    (
        sleep 1.5
        if command -v xdg-open >/dev/null 2>&1; then
            xdg-open "http://localhost:${PORT}" >/dev/null 2>&1 || true
        elif command -v open >/dev/null 2>&1; then
            open "http://localhost:${PORT}" >/dev/null 2>&1 || true
        elif command -v start >/dev/null 2>&1; then
            start "http://localhost:${PORT}" 2>/dev/null || true
        fi
    ) &
fi

# ==============================================================================
# Launch Application
# ==============================================================================
if [ "$FAST_BUILD" = true ] && [ -x "./kv-download" ]; then
    exec ./kv-download
else
    echo -e "${YELLOW}[DEV MODE] Running with go run (slower, use --build for release)${RESET}"
    exec "$GO_CMD" run ./src/main.go
fi
