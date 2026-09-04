#!/usr/bin/env bash
# Quick launch script for KV Download
# Usage: ./launch.sh [port]
# Example: ./launch.sh 8080

cd "$(dirname "$0")"

PORT="${1:-9292}"
echo "🚀 Launching KV Download on port ${PORT}..."
exec ./run.sh --build --release --port "$PORT" --open
