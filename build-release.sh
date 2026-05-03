#!/usr/bin/env bash
# Build a release tarball for deployment to a remote machine.
# Run from the repository root. Requires Go + ../providertron.
# Output: semcom-release.tar.gz
#
# Optional overrides:
#   GOOS    (default: linux)
#   GOARCH  (default: amd64)

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
GOOS="${GOOS:-linux}"
GOARCH="${GOARCH:-amd64}"
OUT="$REPO_DIR/semcom-release.tar.gz"

echo "==> Building semcom (${GOOS}/${GOARCH}, static)..."
cd "$REPO_DIR"
GOOS="$GOOS" GOARCH="$GOARCH" CGO_ENABLED=0 \
  go build -o "$REPO_DIR/semcom" ./semcom_orchestrator/
echo "    OK: $(ls -lh "$REPO_DIR/semcom" | awk '{print $5}')"

echo "==> Packaging release..."
tar czf "$OUT" \
  -C "$REPO_DIR" \
  semcom \
  semcom_embed/index.bin \
  docs/openclaw/semcom-start \
  docs/openclaw/semcom-plugin \
  semcom_adapter/claudecode/hooks \
  install.sh
echo "    OK: $OUT ($(ls -lh "$OUT" | awk '{print $5}'))"

echo ""
echo "Deploy to a remote machine:"
echo "  scp $OUT user@host:~/"
echo "  ssh user@host 'bash -s' < install.sh"
echo "  # or: ssh user@host 'tar xzf semcom-release.tar.gz && bash install.sh'"
