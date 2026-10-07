#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."

MONGO_PORT="${MONGO_PORT:-27018}"
MONGO_URI="${MONGO_URI:-mongodb://127.0.0.1:${MONGO_PORT}}"
MONGO_DB="${MONGO_DB:-control-plane}"
PANEL_ADDR="${PANEL_ADDR:-127.0.0.1:8080}"
MONGO_DATA_DIR="data/dev-mongo"
MONGO_LOG="${MONGO_DATA_DIR}/mongod.log"
MONGO_PID=""
PANEL_PID=""
VITE_PID=""

cleanup() {
  trap - EXIT INT TERM
  for pid in "$VITE_PID" "$PANEL_PID" "$MONGO_PID"; do
    if [[ -n "$pid" ]]; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

if ! command -v mongosh >/dev/null 2>&1; then
  echo "mongosh is required to check the local dev database." >&2
  exit 1
fi

if ! mongosh "$MONGO_URI" --quiet --eval 'db.adminCommand({ ping: 1 }).ok' >/dev/null 2>&1; then
  if ! command -v mongod >/dev/null 2>&1; then
    echo "MongoDB is not running and mongod is not installed." >&2
    exit 1
  fi
  mkdir -p "$MONGO_DATA_DIR"
  mongod --dbpath "$MONGO_DATA_DIR" --bind_ip 127.0.0.1 --port "$MONGO_PORT" --noauth --nounixsocket --quiet >"$MONGO_LOG" 2>&1 &
  MONGO_PID=$!
  for _ in {1..30}; do
    if mongosh "$MONGO_URI" --quiet --eval 'db.adminCommand({ ping: 1 }).ok' >/dev/null 2>&1; then
      break
    fi
    if ! kill -0 "$MONGO_PID" 2>/dev/null; then
      echo "MongoDB failed to start. See $MONGO_LOG" >&2
      exit 1
    fi
    sleep 1
  done
  if ! mongosh "$MONGO_URI" --quiet --eval 'db.adminCommand({ ping: 1 }).ok' >/dev/null 2>&1; then
    echo "MongoDB did not become ready. See $MONGO_LOG" >&2
    exit 1
  fi
fi

if [[ ! -x web/node_modules/.bin/vite ]]; then
  (cd web && npm install)
fi

mkdir -p data
go build -o data/dev-panel ./cmd/panel

PANEL_ADDR="$PANEL_ADDR" \
MONGO_URI="$MONGO_URI" \
MONGO_DB="$MONGO_DB" \
./data/dev-panel &
PANEL_PID=$!

for _ in {1..30}; do
  if curl -fsS "http://${PANEL_ADDR}/api/health" >/dev/null 2>&1; then
    break
  fi
  if ! kill -0 "$PANEL_PID" 2>/dev/null; then
    wait "$PANEL_PID" || true
    echo "The Go panel stopped during startup." >&2
    exit 1
  fi
  sleep 1
done
if ! curl -fsS "http://${PANEL_ADDR}/api/health" >/dev/null 2>&1; then
  echo "The Go panel did not become ready on ${PANEL_ADDR}." >&2
  exit 1
fi

(cd web && exec ./node_modules/.bin/vite --host 127.0.0.1) &
VITE_PID=$!

echo "Starting Control Plane dev server. Vite will print its local URL below."
while kill -0 "$PANEL_PID" 2>/dev/null && kill -0 "$VITE_PID" 2>/dev/null; do
  sleep 1
done

if ! kill -0 "$PANEL_PID" 2>/dev/null; then
  wait "$PANEL_PID" || true
  echo "The Go panel stopped." >&2
  exit 1
fi
wait "$VITE_PID"
