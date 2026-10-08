#!/bin/bash
set -euo pipefail

apt-get update
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends ca-certificates curl jq

if [[ ! "$SERVER_JARFILE" =~ ^[A-Za-z0-9._-]+\.jar$ ]]; then
  echo "SERVER_JARFILE must be a jar filename without a directory." >&2
  exit 1
fi

api="https://meta.fabricmc.net/v2/versions"
game_version="$VERSION"
loader_version="$LOADER_VERSION"
installer_version="$INSTALLER_VERSION"

if [ "$game_version" = "LATEST" ]; then
  game_version="$(curl -fsSL "$api/game" | jq -er '[.[] | select(.stable == true)][0].version')"
fi
if [ "$loader_version" = "LATEST" ]; then
  loader_version="$(curl -fsSL "$api/loader/$game_version" | jq -er '[.[] | select(.loader.stable == true)][0].loader.version')"
fi
if [ "$installer_version" = "LATEST" ]; then
  installer_version="$(curl -fsSL "$api/installer" | jq -er '[.[] | select(.stable == true)][0].version')"
fi

for version in "$game_version" "$loader_version" "$installer_version"; do
  if [[ ! "$version" =~ ^[A-Za-z0-9._+-]+$ ]]; then
    echo "Fabric version contains invalid characters: $version" >&2
    exit 1
  fi
done

mkdir -p /mnt/server
download_url="$api/loader/$game_version/$loader_version/$installer_version/server/jar"
curl -fL "$download_url" -o "/mnt/server/${SERVER_JARFILE}"
test -s "/mnt/server/${SERVER_JARFILE}"
printf 'eula=true\n' > /mnt/server/eula.txt
chown -R 1000:1000 /mnt/server
echo "Installed Fabric for Minecraft ${game_version} with Loader ${loader_version} (launcher ${installer_version})."
