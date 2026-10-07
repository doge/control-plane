#!/bin/bash
set -euo pipefail

mkdir -p /mnt/server
chown steam:steam /mnt/server
su --preserve-environment --shell /bin/bash steam --command '
set -e
cd /home/steam/steamcmd
./steamcmd.sh +force_install_dir /mnt/server +login "$STEAM_USERNAME" "$STEAM_PASSWORD" +app_update 223350 validate +quit
'
if [ ! -x /mnt/server/DayZServer ]; then
  echo "SteamCMD completed without installing DayZServer; verify the Steam account owns or can access app 223350." >&2
  exit 1
fi
chown -R steam:steam /mnt/server
