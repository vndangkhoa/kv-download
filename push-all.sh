#!/usr/bin/env bash
# ==============================================================================
# KV Download — All-in-One Push & Release Pipeline
#
# Automates:
#   1. Git commit & push: GitHub + Forgejo (2 instances: vndns & myds)
#   2. Docker multi-registry build & push: Docker Hub + Forgejo (2) + GHCR
#   3. Synology SPK Package bump, build & upload to https://syno.vndns.net
#   4. Mirroring to manual download site https://syno.vndns.net
#   5. Synology NAS cache clearance & instant feed refresh via SSH
#   6. GitHub & Forgejo Releases
# ==============================================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SPK_SHIP="$(cd "$SCRIPT_DIR/../spk" && pwd)/ship.sh"
[ -f "$SPK_SHIP" ] || { echo "ERROR: $SPK_SHIP not found" >&2; exit 1; }

exec bash "$SPK_SHIP" kvdownload push-all "$@"
