#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
APP_HOME="$HOME/.local/share/jdbc-cli"
BIN_PATH="$APP_HOME/jdbc-cli"
WRAPPER_PATH="/opt/homebrew/bin/jdbc-cli"
PLIST_NAME="com.scriptease.jdbc-cli.plist"
PLIST_SOURCE="$ROOT/launchd/$PLIST_NAME"
PLIST_TARGET="$HOME/Library/LaunchAgents/$PLIST_NAME"
LOG_DIR="$HOME/Library/Logs/jdbc-cli"

mkdir -p "$ROOT/build" "$APP_HOME" "$HOME/Library/LaunchAgents" "$LOG_DIR"

(
  cd "$ROOT"
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o build/jdbc-cli .
)

install -m 0755 "$ROOT/build/jdbc-cli" "$BIN_PATH"

sed \
  -e "s#__HOME__#$HOME#g" \
  -e "s#__BIN_PATH__#$BIN_PATH#g" \
  "$PLIST_SOURCE" > "$PLIST_TARGET"

launchctl bootout "gui/$(id -u)/$PLIST_NAME" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_TARGET"
launchctl kickstart -k "gui/$(id -u)/$PLIST_NAME"

mkdir -p "$(dirname "$WRAPPER_PATH")"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'exec "$HOME/.local/share/jdbc-cli/jdbc-cli" "$@"' > "$WRAPPER_PATH"
chmod +x "$WRAPPER_PATH"

echo "Installed jdbc-cli"
echo "Binary:  $BIN_PATH"
echo "Wrapper: $WRAPPER_PATH"
echo "Logs:    $LOG_DIR"
