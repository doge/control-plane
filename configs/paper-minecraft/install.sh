#!/bin/bash
set -euo pipefail

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates curl jq
mkdir -p /mnt/server

user_agent="control-plane/1.0 (https://play.gabe.cat)"
version="$VERSION"
if [ "$version" = "LATEST" ]; then
  version="$(curl -fsSL -H "User-Agent: $user_agent" https://fill.papermc.io/v3/projects/paper | jq -r '.versions | to_entries[0] | .value[0]')"
fi
if [[ ! "$SERVER_JARFILE" =~ ^[A-Za-z0-9._-]+\.jar$ ]]; then
  echo "SERVER_JARFILE must be a jar filename without a directory." >&2
  exit 1
fi
if [[ ! "$version" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "VERSION contains invalid characters." >&2
  exit 1
fi
build_json=""
if [ "$VERSION" = "LATEST" ]; then
  versions="$(curl -fsSL -H "User-Agent: $user_agent" https://fill.papermc.io/v3/projects/paper | jq -r '.versions | to_entries[] | .value[]' | sort -V -r)"
  for candidate in $versions; do
    builds="$(curl -fsSL -H "User-Agent: $user_agent" "https://fill.papermc.io/v3/projects/paper/versions/${candidate}/builds")"
    build_json="$(printf '%s' "$builds" | jq -c 'first(.[] | select(.channel == "STABLE")) // {}')"
    if [ "$build_json" != "{}" ]; then
      version="$candidate"
      break
    fi
  done
else
  builds="$(curl -fsSL -H "User-Agent: $user_agent" "https://fill.papermc.io/v3/projects/paper/versions/${version}/builds")"
  build_json="$(printf '%s' "$builds" | jq -c 'first(.[] | select(.channel == "STABLE")) // {}')"
fi
download_url="$(printf '%s' "$build_json" | jq -r '.downloads."server:default".url // "null"')"
build="$(printf '%s' "$build_json" | jq -r '.id // "unknown"')"
if [ "$download_url" = "null" ] || [ -z "$download_url" ]; then
  echo "No stable Paper build found for Minecraft ${version}." >&2
  exit 1
fi
curl -fL -H "User-Agent: $user_agent" "$download_url" -o "/mnt/server/${SERVER_JARFILE}"
test -s "/mnt/server/${SERVER_JARFILE}"
printf 'eula=true\n' > /mnt/server/eula.txt
chown -R 1000:1000 /mnt/server
echo "Installed Paper ${version} build ${build}."
