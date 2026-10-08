#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET_OS="${GOOS:-linux}"
TARGET_ARCH="${GOARCH:-amd64}"
if [[ "$TARGET_OS" != "linux" || ( "$TARGET_ARCH" != "amd64" && "$TARGET_ARCH" != "arm64" ) ]]; then
  echo "Supported build targets are linux/amd64 and linux/arm64." >&2
  exit 2
fi

OUT_DIR="$ROOT_DIR/dist"
STAGE_DIR="$(mktemp -d)"
trap 'rm -rf "$STAGE_DIR"' EXIT
mkdir -p "$OUT_DIR" "$STAGE_DIR/web" "$STAGE_DIR/scripts"
cat > "$STAGE_DIR/release.env" <<RELEASE
GOOS=$TARGET_OS
GOARCH=$TARGET_ARCH
RELEASE

cd "$ROOT_DIR/web"
npm ci
npm run build
cd "$ROOT_DIR"
cp "$ROOT_DIR/web/dist/index.html" "$STAGE_DIR/web/index.html"
cp -R "$ROOT_DIR/web/dist/assets" "$STAGE_DIR/web/assets"
cp "$ROOT_DIR/scripts/install-https.sh" "$ROOT_DIR/scripts/install-node.sh" "$ROOT_DIR/scripts/install-panel.sh" "$STAGE_DIR/scripts/"
chmod 0755 "$STAGE_DIR/scripts/"*.sh
cp "$ROOT_DIR/scripts/install-panel.sh" "$ROOT_DIR/scripts/install-node.sh" "$ROOT_DIR/scripts/install-https.sh" "$OUT_DIR/"
chmod 0755 "$OUT_DIR/install-panel.sh" "$OUT_DIR/install-node.sh" "$OUT_DIR/install-https.sh"
GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$STAGE_DIR/control-plane-panel" ./cmd/panel
GOOS="$TARGET_OS" GOARCH="$TARGET_ARCH" CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$STAGE_DIR/control-plane-node" ./cmd/node
chmod 0755 "$STAGE_DIR/control-plane-panel" "$STAGE_DIR/control-plane-node"

ARCHIVE="$OUT_DIR/control-plane-${TARGET_OS}-${TARGET_ARCH}.tar.gz"
COPYFILE_DISABLE=1 tar -C "$STAGE_DIR" -czf "$ARCHIVE" .
printf 'Built release bundle: %s\n' "$ARCHIVE"
printf 'Install the panel with: sudo %s/install-panel.sh %s\n' "$OUT_DIR" "$ARCHIVE"
printf 'Install a node with: sudo %s/install-node.sh %s <panel-url> <node-token>\n' "$OUT_DIR" "$ARCHIVE"
