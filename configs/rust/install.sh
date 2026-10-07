#!/bin/bash
set -euo pipefail

mkdir -p /mnt/server
chown steam:steam /mnt/server
su --preserve-environment --shell /bin/bash steam --command '
set -e
cd /home/steam/steamcmd
./steamcmd.sh +force_install_dir /mnt/server +login anonymous +app_update 258550 validate +quit
'
if [ ! -x /mnt/server/RustDedicated ]; then
  echo "SteamCMD completed without installing RustDedicated." >&2
  exit 1
fi
chown -R steam:steam /mnt/server
