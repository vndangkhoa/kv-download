# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.25.3-alpine3.22 AS builder

ARG TARGETOS
ARG TARGETARCH
RUN apk add --no-cache curl

WORKDIR /app

COPY src src
COPY templates templates
COPY go.mod go.mod
COPY go.sum go.sum

RUN go mod download
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -x -o media-roller ./src

# yt-dlp needs python
FROM --platform=$TARGETPLATFORM python:3.13.7-alpine3.22

ARG TARGETARCH

LABEL org.opencontainers.image.source="https://git.khoavo.myds.me/vndangkhoa/kv-download"
LABEL org.opencontainers.image.description="Media Roller - Mobile friendly video downloader"
LABEL org.opencontainers.image.licenses="MIT"

# This is where the downloaded files will be saved in the container.
ENV MR_DOWNLOAD_DIR="/download"

RUN apk add --update --no-cache \
    # https://github.com/yt-dlp/yt-dlp/issues/14404 \
    deno \
    curl

# https://hub.docker.com/r/mwader/static-ffmpeg/tags
# https://github.com/wader/static-ffmpeg
COPY --from=mwader/static-ffmpeg:8.0 /ffmpeg  /usr/local/bin/
COPY --from=mwader/static-ffmpeg:8.0 /ffprobe /usr/local/bin/
COPY --from=builder /app/media-roller /app/media-roller
COPY templates /app/templates
COPY static /app/static

WORKDIR /app

# Get new releases here https://github.com/yt-dlp/yt-dlp/releases
RUN curl -L https://github.com/yt-dlp/yt-dlp/releases/download/2025.09.26/yt-dlp -o /usr/local/bin/yt-dlp && \
    echo "9215a371883aea75f0f2102c679333d813d9a5c3bceca212879a4a741a5b4657 /usr/local/bin/yt-dlp" | sha256sum -c - && \
    chmod a+rx /usr/local/bin/yt-dlp

RUN yt-dlp --update --update-to nightly

# Sanity check
RUN yt-dlp --version && \
    ffmpeg -version

EXPOSE 9292

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:9292/ || exit 1

ENTRYPOINT ["/app/media-roller"]
