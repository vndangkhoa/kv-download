#!/usr/bin/env bash
set -e

echo "==> Building Next.js + TypeScript Frontend..."
(cd frontend && pnpm install && pnpm build)

echo "==> Building Go Binary (kv-download)..."
go build -o kv-download src/main.go

echo "==> Build complete! Run ./kv-download to start the server on port 9292."
