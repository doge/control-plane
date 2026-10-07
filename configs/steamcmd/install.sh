#!/bin/bash
set -euo pipefail

mkdir -p /mnt/server
chown steam:steam /mnt/server
su --preserve-environment --shell /bin/bash steam --command '
set -e
cd /home/steam/steamcmd
if [ -n "$STEAM_PASSWORD" ]; then
  ./steamcmd.sh +force_install_dir /mnt/server +login "$STEAM_USERNAME" "$STEAM_PASSWORD" +app_update "$STEAM_APP_ID" validate +quit
else
  ./steamcmd.sh +force_install_dir /mnt/server +login "$STEAM_USERNAME" +app_update "$STEAM_APP_ID" validate +quit
fi
'
chown -R steam:steam /mnt/server
