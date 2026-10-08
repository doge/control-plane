#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
  echo "Usage: sudo $0 <control-plane-linux-ARCH.tar.gz | bundle-directory> <panel-url> <node-token>" >&2
  exit 2
fi
if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this installer with sudo." >&2
  exit 1
fi
if [[ ! -r /etc/os-release ]]; then
  echo "This installer supports Debian and Ubuntu." >&2
  exit 1
fi
. /etc/os-release
if [[ "${ID:-}" != debian && "${ID:-}" != ubuntu ]]; then
  echo "This installer supports Debian and Ubuntu." >&2
  exit 1
fi

SOURCE="$1"
PANEL_URL="${2%/}"
NODE_TOKEN="$3"
if [[ ! "$PANEL_URL" =~ ^https?://[A-Za-z0-9.-]+(:[0-9]+)?$ ]]; then
  echo "Panel URL must be an HTTP or HTTPS origin, such as https://panel.example.com." >&2
  exit 2
fi
if [[ ! "$NODE_TOKEN" =~ ^[A-Za-z0-9._~-]+$ ]]; then
  echo "Node tokens must contain only letters, numbers, dot, underscore, tilde, and hyphen." >&2
  exit 2
fi

TEMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TEMP_DIR"' EXIT
if [[ -d "$SOURCE" ]]; then
  BUNDLE_DIR="$(cd "$SOURCE" && pwd)"
else
  [[ -f "$SOURCE" ]] || { echo "Bundle not found: $SOURCE" >&2; exit 1; }
  tar -xzf "$SOURCE" -C "$TEMP_DIR"
  BUNDLE_DIR="$TEMP_DIR"
fi
[[ -f "$BUNDLE_DIR/control-plane-node" ]] || { echo "Release bundle is missing control-plane-node" >&2; exit 1; }
HOST_ARCH="$(uname -m)"
case "$HOST_ARCH" in
  x86_64) HOST_GOARCH=amd64 ;;
  aarch64|arm64) HOST_GOARCH=arm64 ;;
  *) echo "Unsupported host architecture: $HOST_ARCH" >&2; exit 1 ;;
esac
if [[ ! -f "$BUNDLE_DIR/release.env" ]]; then
  echo "Release bundle is missing release.env; build it with scripts/build-release.sh." >&2
  exit 1
fi
BUNDLE_GOARCH="$(sed -n 's/^GOARCH=//p' "$BUNDLE_DIR/release.env")"
if [[ "$BUNDLE_GOARCH" != "$HOST_GOARCH" ]]; then
  echo "This bundle is for linux/$BUNDLE_GOARCH, but this host is linux/$HOST_GOARCH." >&2
  exit 1
fi

for package in docker.io docker-doc docker-compose podman-docker containerd runc; do
  if dpkg-query -W -f='${db:Status-Status}' "$package" 2>/dev/null | grep -qx installed; then
    echo "Conflicting package $package is installed. Remove it following Docker's official instructions before retrying." >&2
    exit 1
  fi
done

apt-get update
apt-get install -y ca-certificates curl e2fsprogs ufw util-linux
install -m 0755 -d /etc/apt/keyrings
curl -fsSL "https://download.docker.com/linux/$ID/gpg" -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
ARCH="$(dpkg --print-architecture)"
CODENAME="${UBUNTU_CODENAME:-${VERSION_CODENAME:-}}"
cat > /etc/apt/sources.list.d/docker.sources <<REPO
Types: deb
URIs: https://download.docker.com/linux/$ID
Suites: $CODENAME
Components: stable
Architectures: $ARCH
Signed-By: /etc/apt/keyrings/docker.asc
REPO
apt-get update
apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker

install -o root -g root -m 0755 "$BUNDLE_DIR/control-plane-node" /usr/local/bin/control-plane-node
install -d -o root -g root -m 0750 /etc/control-plane
cat > /etc/control-plane/node.env <<ENV
PANEL_URL=$PANEL_URL
NODE_TOKEN=$NODE_TOKEN
NODE_ADDR=127.0.0.1:8090
ENV
chmod 0600 /etc/control-plane/node.env
cat > /etc/systemd/system/control-plane-node.service <<'UNIT'
[Unit]
Description=Control Plane node agent
After=network-online.target docker.service
Wants=network-online.target
Requires=docker.service

[Service]
Type=simple
User=root
EnvironmentFile=/etc/control-plane/node.env
Environment=XDG_CONFIG_HOME=/var/lib/control-plane-node
StateDirectory=control-plane-node
ExecStart=/usr/local/bin/control-plane-node
ExecStartPre=/usr/local/bin/control-plane-node disable-managed-server-restarts
ExecStopPost=/usr/local/bin/control-plane-node stop-managed-servers
Restart=always
RestartSec=3
LimitNOFILE=65536
NoNewPrivileges=true
ProtectHome=true
ProtectSystem=full
ReadWritePaths=/etc/systemd/system /etc/ufw

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable control-plane-node
systemctl restart control-plane-node
cat <<'INFO'

Control Plane node agent is installed.
  Service: systemctl status control-plane-node
  Logs:    journalctl -u control-plane-node -f
  Config:  /etc/control-plane/node.env

The agent runs as root to access Docker's root-owned socket and administer containers.
Keep the node token private and do not expose the Docker API over TCP.
INFO
