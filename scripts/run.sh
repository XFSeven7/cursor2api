#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BINARY="$ROOT/cursor2api"
CONFIG="$ROOT/config.json"
LOG_DIR="$ROOT/logs"

mkdir -p "$LOG_DIR"

if [[ ! -f "$CONFIG" ]]; then
  cp "$ROOT/config.example.json" "$CONFIG"
  echo "已创建 $CONFIG"
fi

if [[ ! -x "$BINARY" ]]; then
  echo "正在编译 cursor2api..."
  (cd "$ROOT" && go build -o cursor2api ./src)
fi

AGENT_BIN="$(command -v agent || true)"
PATH_PREFIX="/usr/local/bin:/usr/bin:/bin"
if [[ -n "$AGENT_BIN" ]]; then
  PATH_PREFIX="$(dirname "$AGENT_BIN"):$PATH_PREFIX"
fi
PATH_PREFIX="$HOME/.local/bin:$PATH_PREFIX"

exec "$BINARY" "$CONFIG"
