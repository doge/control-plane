#!/usr/bin/env bash
set -euo pipefail

if [[ $# -lt 1 || $# -gt 3 ]]; then
  echo "Usage: sudo $0 <panel-domain> [certificate-email] [panel-upstream]" >&2
  echo "Example: sudo $0 panel.example.com admin@example.com 127.0.0.1:8080" >&2
  exit 2
fi

DOMAIN="$1"
EMAIL="${2:-}"
PANEL_UPSTREAM="${3:-${PANEL_UPSTREAM:-127.0.0.1:8080}}"

if [[ ! "$DOMAIN" =~ ^([A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?\.)+[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?$ || ${#DOMAIN} -gt 253 ]]; then
  echo "Provide a public DNS hostname such as panel.example.com." >&2
  exit 2
fi
if [[ -n "$EMAIL" && ! "$EMAIL" =~ ^[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}$ ]]; then
  echo "Provide a valid certificate contact email address." >&2
  exit 2
fi
if [[ ! "$PANEL_UPSTREAM" =~ ^127\.0\.0\.1:([0-9]{1,5})$ ]] || (( 10#${BASH_REMATCH[1]:-0} < 1 || 10#${BASH_REMATCH[1]:-0} > 65535 )); then
  echo "The panel upstream must be a loopback address, for example 127.0.0.1:8080." >&2
  exit 2
fi
PANEL_PORT="${BASH_REMATCH[1]}"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this installer with sudo." >&2
  exit 1
fi
if [[ ! -r /etc/os-release ]]; then
  echo "This installer supports Debian and Ubuntu hosts." >&2
  exit 1
fi
. /etc/os-release
if [[ "${ID:-}" != "debian" && "${ID:-}" != "ubuntu" ]]; then
  echo "This installer supports Debian and Ubuntu hosts." >&2
  exit 1
fi

# Remove only Caddy's known apt source files before apt-get update. A failed
# earlier install can leave one pointing at an expired key, which would make
# apt reject the repository before this script gets a chance to refresh it.
rm -f /etc/apt/sources.list.d/caddy-stable.list /etc/apt/sources.list.d/caddy-stable.sources
apt-get update
apt-get install -y ca-certificates curl gnupg

# Fetch the current signing key to a temporary file and replace the keyring
# atomically, avoiding a partial keyring if the download is interrupted.
KEYRING_TMP="$(mktemp /usr/share/keyrings/caddy-stable-archive-keyring.gpg.XXXXXX)"
trap 'rm -f "$KEYRING_TMP"' EXIT
curl --fail --show-error --silent --location --retry 3 \
  'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' \
  | gpg --dearmor --yes -o "$KEYRING_TMP"
chmod 0644 "$KEYRING_TMP"
mv "$KEYRING_TMP" /usr/share/keyrings/caddy-stable-archive-keyring.gpg
trap - EXIT

curl --fail --show-error --silent --location --retry 3 \
  'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' \
  -o /etc/apt/sources.list.d/caddy-stable.list
chmod 0644 /etc/apt/sources.list.d/caddy-stable.list
if apt-get update; then
  apt-get install -y caddy
else
  echo "Caddy's upstream package repository failed signature verification; using the OS-signed Caddy package instead." >&2
  rm -f /etc/apt/sources.list.d/caddy-stable.list /etc/apt/sources.list.d/caddy-stable.sources
  rm -f /var/lib/apt/lists/*dl.cloudsmith.io_public_caddy_stable*
  apt-get update
  apt-get install -y caddy
fi

install -d -m 0755 /etc/caddy
if [[ -f /etc/caddy/Caddyfile ]]; then
  cp /etc/caddy/Caddyfile "/etc/caddy/Caddyfile.backup.$(date +%Y%m%d%H%M%S)"
fi
cat > /etc/caddy/Caddyfile <<CADDY
{
  auto_https disable_redirects
}

http://$DOMAIN {
  encode zstd gzip
  reverse_proxy $PANEL_UPSTREAM
}

https://$DOMAIN {
CADDY
if [[ -n "$EMAIL" ]]; then printf '  tls %s\n' "$EMAIL" >> /etc/caddy/Caddyfile; fi
cat >> /etc/caddy/Caddyfile <<CADDY
  encode zstd gzip
  reverse_proxy $PANEL_UPSTREAM
}
CADDY

caddy fmt --overwrite /etc/caddy/Caddyfile
systemctl enable --now caddy
systemctl reload caddy

cat <<INFO

Caddy is configured for $DOMAIN and will obtain and renew TLS certificates automatically.
Panel upstream: $PANEL_UPSTREAM

Before enabling HTTPS enforcement in Control Plane:
  1. Point DNS for $DOMAIN to this host and allow inbound TCP ports 80 and 443.
  2. Ensure Control Plane listens on $PANEL_UPSTREAM (PANEL_ADDR=$PANEL_UPSTREAM).
  3. Open https://$DOMAIN, sign in, then enable HTTPS required in Settings.

Use the same Settings control to allow HTTP again. Caddy proxies WebSocket connections
as well, so node agents can use WSS through the same origin.
INFO
