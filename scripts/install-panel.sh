#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 3 ]]; then
  echo "Usage: sudo $0 <control-plane-linux-ARCH.tar.gz | bundle-directory> [panel-domain] [certificate-email]" >&2
  echo "Example: sudo $0 ./control-plane-linux-amd64.tar.gz panel.example.com admin@example.com" >&2
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
PANEL_DOMAIN="${2:-}"
CERTIFICATE_EMAIL="${3:-}"
if [[ -n "$CERTIFICATE_EMAIL" && -z "$PANEL_DOMAIN" ]]; then
  echo "Provide a panel domain before the optional certificate email." >&2
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
for required in control-plane-panel web/index.html; do
  [[ -f "$BUNDLE_DIR/$required" ]] || { echo "Release bundle is missing $required" >&2; exit 1; }
done
if [[ -n "$PANEL_DOMAIN" && ! -f "$BUNDLE_DIR/scripts/install-https.sh" ]]; then
  echo "Release bundle is missing scripts/install-https.sh; rebuild it with scripts/build-release.sh." >&2
  exit 1
fi

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

ARCH="$(dpkg --print-architecture)"
CODENAME="${UBUNTU_CODENAME:-${VERSION_CODENAME:-}}"
if [[ "$ID" == debian ]]; then
  if [[ "$CODENAME" != bookworm || "$ARCH" != amd64 ]]; then
    echo "The bundled MongoDB 8.0 setup supports Debian 12 amd64. Use a compatible external MONGO_URI on other systems." >&2
    exit 1
  fi
fi
if [[ "$ID" == ubuntu && ! "$CODENAME" =~ ^(focal|jammy|noble)$ ]]; then
  echo "The bundled MongoDB 8.0 setup supports Ubuntu 20.04, 22.04, and 24.04. Use a compatible external MONGO_URI on other systems." >&2
  exit 1
fi

if ! command -v mongod >/dev/null 2>&1; then
  apt-get update
  apt-get install -y ca-certificates curl gnupg
  install -m 0755 -d /usr/share/keyrings
  curl -fsSL https://pgp.mongodb.com/server-8.0.asc | gpg --dearmor --yes -o /usr/share/keyrings/mongodb-server-8.0.gpg
  chmod 0644 /usr/share/keyrings/mongodb-server-8.0.gpg
  if [[ "$ID" == ubuntu ]]; then
    printf 'deb [ arch=%s signed-by=/usr/share/keyrings/mongodb-server-8.0.gpg ] https://repo.mongodb.org/apt/ubuntu %s/mongodb-org/8.0 multiverse\n' "$ARCH" "$CODENAME" > /etc/apt/sources.list.d/mongodb-org-8.0.list
  else
    printf 'deb [ arch=%s signed-by=/usr/share/keyrings/mongodb-server-8.0.gpg ] https://repo.mongodb.org/apt/debian %s/mongodb-org/8.0 main\n' "$ARCH" "$CODENAME" > /etc/apt/sources.list.d/mongodb-org-8.0.list
  fi
  apt-get update
  apt-get install -y mongodb-org
fi
systemctl enable --now mongod

if ! id control-plane >/dev/null 2>&1; then
  useradd --system --user-group --home-dir /var/lib/control-plane --create-home --shell /usr/sbin/nologin control-plane
fi
install -d -o root -g root -m 0755 /opt/control-plane/bin /opt/control-plane/web /opt/control-plane/scripts
install -o root -g root -m 0755 "$BUNDLE_DIR/control-plane-panel" /opt/control-plane/bin/control-plane-panel
cp -a "$BUNDLE_DIR/web/." /opt/control-plane/web/
chown -R root:root /opt/control-plane/web
find /opt/control-plane/web -type d -exec chmod 0755 {} +
find /opt/control-plane/web -type f -exec chmod 0644 {} +
if [[ -f "$BUNDLE_DIR/scripts/install-https.sh" ]]; then
  install -o root -g root -m 0755 "$BUNDLE_DIR/scripts/install-https.sh" /opt/control-plane/scripts/install-https.sh
fi

install -d -o root -g control-plane -m 0750 /etc/control-plane
if [[ ! -f /etc/control-plane/panel.env ]]; then
  cat > /etc/control-plane/panel.env <<'ENV'
PANEL_ADDR=127.0.0.1:8080
MONGO_URI=mongodb://127.0.0.1:27017/control-plane?directConnection=true
MONGO_DB=control-plane
SESSION_TTL_HOURS=168
STATIC_DIR=/opt/control-plane/web
ENV
fi
chown root:control-plane /etc/control-plane/panel.env
chmod 0640 /etc/control-plane/panel.env

cat > /etc/systemd/system/control-plane-panel.service <<'UNIT'
[Unit]
Description=Control Plane panel
After=network-online.target mongod.service
Wants=network-online.target
Requires=mongod.service

[Service]
Type=simple
User=control-plane
Group=control-plane
WorkingDirectory=/opt/control-plane
EnvironmentFile=/etc/control-plane/panel.env
ExecStart=/opt/control-plane/bin/control-plane-panel
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=full

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable control-plane-panel
systemctl restart control-plane-panel
if [[ -n "$PANEL_DOMAIN" ]]; then
  /opt/control-plane/scripts/install-https.sh "$PANEL_DOMAIN" "$CERTIFICATE_EMAIL"
  cat <<INFO

Control Plane panel is installed at https://$PANEL_DOMAIN
  Panel service: systemctl status control-plane-panel
  Caddy service: systemctl status caddy
  Logs:           journalctl -u control-plane-panel -f
  Config:         /etc/control-plane/panel.env

Point DNS for $PANEL_DOMAIN at this host and allow inbound TCP ports 80 and 443.
INFO
else
  cat <<'INFO'

Control Plane panel is installed.
  Service: systemctl status control-plane-panel
  Logs:    journalctl -u control-plane-panel -f
  Config:  /etc/control-plane/panel.env

The default panel listener is loopback-only at 127.0.0.1:8080. To use a public URL
without a port, rerun this installer with a panel domain and optional certificate email,
or run /opt/control-plane/scripts/install-https.sh with the configured hostname.
INFO
fi
