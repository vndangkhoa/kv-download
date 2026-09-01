#!/usr/bin/env bash
# ==============================================================================
# KV Download — All-in-One Push & Release Pipeline
#
# Automates:
#   1. Git commit & push (Forgejo + GitHub) with tags
#   2. Docker multi-registry build & push (Docker Hub + Forgejo + GHCR)
#   3. SPK build, spkrepo publish & activation on https://pkg.khoavo.myds.me
#   4. Mirroring to manual download site https://spk.khoavo.myds.me
#   5. Synology NAS cache clearance & instant feed refresh via SSH
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PC_DIR="$(dirname "$SCRIPT_DIR")"
SECRETS="$PC_DIR/.secrets.env"
SPK_DIR="$PC_DIR/spk"
APP="kvdownload"

if [ -f "$SECRETS" ]; then
    set -a
    # shellcheck disable=SC1090
    . "$SECRETS"
    set +a
else
    echo "⚠️ Warning: $SECRETS not found"
fi

IMAGE_TAG="${1:-1.0.7}"
COMMIT_MSG="${2:-feat: update KV Download UI, format options, and engine optimizations}"

echo "===================================================================="
echo "🚀 Starting Full Release Pipeline for KV Download (v$IMAGE_TAG)"
echo "===================================================================="

# ── 1. Git Push (Forgejo + GitHub) ──────────────────────────────────────────
echo
echo "📦 [1/5] Checking Git Status & Pushing Code..."
cd "$SCRIPT_DIR"

if [ -n "$(git status --porcelain)" ]; then
    git add -A
    git commit -m "$COMMIT_MSG" || true
fi

git tag -f -a "v$IMAGE_TAG" -m "KV Download v$IMAGE_TAG"

echo "  -> Pushing master to Forgejo..."
git push origin master --force 2>/dev/null || git push origin master
echo "  -> Pushing master to GitHub..."
git push github master --force 2>/dev/null || git push github master
echo "  -> Pushing v$IMAGE_TAG tag to Forgejo & GitHub..."
git push origin "v$IMAGE_TAG" --force 2>/dev/null || true
git push github "v$IMAGE_TAG" --force 2>/dev/null || true

# ── 2. Docker Image Build & Push (3 Registries) ──────────────────────────────
echo
echo "🐳 [2/5] Building & Pushing Docker Images..."
docker build \
  -t "vndangkhoa/kv-download:latest" \
  -t "vndangkhoa/kv-download:$IMAGE_TAG" \
  -t "git.khoavo.myds.me/vndangkhoa/kv-download:latest" \
  -t "git.khoavo.myds.me/vndangkhoa/kv-download:$IMAGE_TAG" \
  -t "ghcr.io/vndangkhoa/kv-download:latest" \
  -t "ghcr.io/vndangkhoa/kv-download:$IMAGE_TAG" \
  .

echo "  -> Pushing to Docker Hub..."
docker push "vndangkhoa/kv-download:latest"
docker push "vndangkhoa/kv-download:$IMAGE_TAG"

echo "  -> Pushing to Forgejo Registry..."
docker push "git.khoavo.myds.me/vndangkhoa/kv-download:latest"
docker push "git.khoavo.myds.me/vndangkhoa/kv-download:$IMAGE_TAG"

echo "  -> Pushing to GitHub Container Registry..."
docker push "ghcr.io/vndangkhoa/kv-download:latest"
docker push "ghcr.io/vndangkhoa/kv-download:$IMAGE_TAG"

# ── 3. Synology SPK Package Bump, Build & Publish ───────────────────────────
echo
echo "📦 [3/5] Building & Publishing Synology SPK Package..."
cd "$SPK_DIR"

# Get next SPK build version
CURRENT_VER=$(sed -n -E 's/^VERSION="?([^"]*)"?.*/\1/p' "$SPK_DIR/apps/$APP/build.conf" | head -1)
PREFIX=$(echo "$CURRENT_VER" | cut -d'-' -f1)
BUILD_NUM=$(echo "$CURRENT_VER" | cut -d'-' -f2)
NEXT_BUILD=$((BUILD_NUM + 1))
NEXT_VER="${PREFIX}-${NEXT_BUILD}"

echo "  -> Current SPK Version: $CURRENT_VER"
echo "  -> Next SPK Version:    $NEXT_VER (Image Tag: $IMAGE_TAG)"

# Run Universal Ship Pipeline
./ship.sh "$APP" spk "$NEXT_VER" "$IMAGE_TAG"

# ── 4. Git Push SPK Repo ───────────────────────────────────────────────────
echo
echo "🐙 [4/5] Pushing SPK Repository Updates..."
cd "$SPK_DIR"
git add "apps/$APP/build.conf"
git commit -m "chore($APP): bump SPK to $NEXT_VER (image: $IMAGE_TAG)" || true
git push forgejo main 2>/dev/null || true
git push github main 2>/dev/null || true

# ── 5. NAS Catalog & Cache Invalidation ─────────────────────────────────────
echo
echo "🔄 [5/5] Refreshing DSM Package Center Feed on NAS..."
if [ -f "$SPK_DIR/tools/ssh-run.py" ]; then
    python3 "$SPK_DIR/tools/ssh-run.py" "sudo rm -rf /var/run/synopkg/pkglist/* /var/cache/synopkg/badge_count_records/*; sudo /volume2/docker/spk/refresh-spk-site.sh; sudo /volume2/docker/spkrepo/scripts/clear-pkglist-cache.sh; sudo /usr/syno/bin/synopkg chkupgradepkg >/dev/null 2>&1 || true" || true
fi

echo
echo "===================================================================="
echo "✅ All Done! KV Download v$IMAGE_TAG (SPK v$NEXT_VER) is Live!"
echo "   - Web Feed: https://pkg.khoavo.myds.me/package/kvdownload"
echo "   - Download: https://spk.khoavo.myds.me/kvdownload-$NEXT_VER.spk"
echo "===================================================================="
