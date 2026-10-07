#!/bin/bash
set -euo pipefail

exec ./RustDedicated -batchmode \
  +server.port "$GAME_PORT" \
  +server.queryport "$QUERY_PORT" \
  +server.identity rust-server \
  +server.hostname "$SERVER_NAME" \
  +server.maxplayers "$MAX_PLAYERS" \
  +server.worldsize "$WORLD_SIZE" \
  +server.seed "$WORLD_SEED" \
  +rcon.port "$RCON_PORT" \
  +rcon.password "$RCON_PASSWORD" \
  +rcon.web 1
