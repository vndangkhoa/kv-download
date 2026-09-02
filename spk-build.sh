#!/usr/bin/env bash
set -e

VERSION="1.0.11"
PKG_NAME="kv-download"
BUILD_DIR="$(pwd)/build_spk"
DIST_DIR="$(pwd)/dist"

echo "=== Building Synology SPK Package for $PKG_NAME v$VERSION ==="

rm -rf "$BUILD_DIR" "$DIST_DIR"
mkdir -p "$BUILD_DIR/package" "$BUILD_DIR/scripts" "$DIST_DIR"

# 1. Prepare package.tgz contents (Docker compose + configuration)
cat << 'EOF' > "$BUILD_DIR/package/docker-compose.yml"
services:
  kv-download:
    image: vndangkhoa/kv-download:latest
    container_name: kv-download
    restart: unless-stopped
    ports:
      - "9292:9292"
    volumes:
      - ./downloads:/download
      - ./cookies.txt:/app/cookies.txt
    environment:
      - TZ=Asia/Ho_Chi_Minh
EOF

if [ -f "static/logo.svg" ]; [ -f "static/apple-touch-icon.png" ]; then
    cp static/apple-touch-icon.png "$BUILD_DIR/package/icon.png"
fi

cd "$BUILD_DIR/package"
tar czf "$BUILD_DIR/package.tgz" *
cd - > /dev/null

# 2. Prepare Synology INFO metadata file
cat << EOF > "$BUILD_DIR/INFO"
package="$PKG_NAME"
version="$VERSION"
os_min_ver="7.0-40000"
displayname="KV Download"
description="Fast, multi-platform social media video downloader with MeTube task watcher."
arch="noarch"
maintainer="vndangkhoa"
maintainer_url="https://github.com/vndangkhoa/kv-download"
distributor="vndangkhoa"
helpurl="https://github.com/vndangkhoa/kv-download"
install_dep_packages="ContainerManager"
dsmappname="SYNO.SDS.kv-download"
dsmapppage="index.html"
adminport="9292"
adminurl="http://[HOST]:9292"
EOF

# 3. Prepare Synology package control scripts
cat << 'EOF' > "$BUILD_DIR/scripts/start-stop-status"
#!/bin/sh

case "$1" in
    start)
        cd "$SYNOPKG_PKGDEST"
        docker compose up -d
        exit 0
        ;;
    stop)
        cd "$SYNOPKG_PKGDEST"
        docker compose down
        exit 0
        ;;
    status)
        if docker ps --format '{{.Names}}' | grep -q "^kv-download$"; then
            exit 0
        else
            exit 1
        fi
        ;;
    *)
        exit 1
        ;;
esac
EOF

cat << 'EOF' > "$BUILD_DIR/scripts/postinst"
#!/bin/sh
cd "$SYNOPKG_PKGDEST"
docker compose pull || true
exit 0
EOF

cat << 'EOF' > "$BUILD_DIR/scripts/postuninst"
#!/bin/sh
cd "$SYNOPKG_PKGDEST"
docker compose down --volumes --remove-orphans || true
exit 0
EOF

chmod +x "$BUILD_DIR/scripts/"*

# 4. Copy icons if available
if [ -f "static/apple-touch-icon.png" ]; then
    cp static/apple-touch-icon.png "$BUILD_DIR/PACKAGE_ICON.PNG"
    cp static/apple-touch-icon.png "$BUILD_DIR/PACKAGE_ICON_256.PNG"
fi

# 5. Pack into final .spk archive (tar format)
cd "$BUILD_DIR"
tar -cf "$DIST_DIR/${PKG_NAME}_${VERSION}.spk" INFO package.tgz scripts PACKAGE_ICON.PNG PACKAGE_ICON_256.PNG 2>/dev/null || tar -cf "$DIST_DIR/${PKG_NAME}_${VERSION}.spk" INFO package.tgz scripts
cd - > /dev/null

rm -rf "$BUILD_DIR"
echo "✅ SPK package created successfully: $DIST_DIR/${PKG_NAME}_${VERSION}.spk"
