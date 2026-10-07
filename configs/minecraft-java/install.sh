#!/bin/bash
set -euo pipefail

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates curl jq

if [[ ! "$SERVER_JARFILE" =~ ^[A-Za-z0-9._-]+\.jar$ ]]; then
  echo "SERVER_JARFILE must be a jar filename without a directory." >&2
  exit 1
fi

manifest="$(curl -fsSL https://piston-meta.mojang.com/mc/game/version_manifest_v2.json)"
version="$VERSION"
if [ "$version" = "LATEST" ]; then
  version="$(printf '%s' "$manifest" | jq -r '.latest.release')"
fi
version_url="$(printf '%s' "$manifest" | jq -r --arg version "$version" '.versions[] | select(.id == $version) | .url' | head -n 1)"
if [ -z "$version_url" ] || [ "$version_url" = "null" ]; then
  echo "Minecraft version $version was not found in Mojang's version manifest." >&2
  exit 1
fi
download_url="$(curl -fsSL "$version_url" | jq -r '.downloads.server.url // empty')"
if [ -z "$download_url" ]; then
  echo "Minecraft version $version has no server jar download." >&2
  exit 1
fi

mkdir -p /mnt/server
curl -fL "$download_url" -o "/mnt/server/${SERVER_JARFILE}"
printf 'eula=true\n' > /mnt/server/eula.txt
chown -R 1000:1000 /mnt/server
echo "Installed vanilla Minecraft ${version}."
