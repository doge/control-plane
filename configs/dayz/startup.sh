#!/bin/bash
set -euo pipefail

if [ ! -f serverDZ.cfg ]; then
  printf 'hostname = "%s";\npassword = "";\npasswordAdmin = "%s";\nmaxPlayers = %s;\nverifySignatures = 2;\nforceSameBuild = 1;\nsteamQueryPort = %s;\nclass Missions { class DayZ { template = "dayzOffline.chernarusplus"; difficulty = "Regular"; }; };\n' \
    "$SERVER_NAME" "$ADMIN_PASSWORD" "$MAX_PLAYERS" "$QUERY_PORT" > serverDZ.cfg
fi

exec ./DayZServer \
  -config=serverDZ.cfg \
  -port="$GAME_PORT" \
  -profiles=profiles \
  -BEpath=battleye \
  -dologs -adminlog -netlog -freezecheck
