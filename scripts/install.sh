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
BUILD_PATH="$ROOT/build/jdbc-cli"
STAGED_BIN="$APP_HOME/.jdbc-cli.new"
PLIST_TMP=""
WRAPPER_TMP=""

cleanup() {
  rm -f "$STAGED_BIN"
  if [[ -n "$PLIST_TMP" ]]; then
    rm -f "$PLIST_TMP"
  fi
  if [[ -n "$WRAPPER_TMP" ]]; then
    rm -f "$WRAPPER_TMP"
  fi
}
trap cleanup EXIT

mkdir -p "$ROOT/build" "$APP_HOME" "$HOME/Library/LaunchAgents" "$LOG_DIR" "$(dirname "$WRAPPER_PATH")"

(
  cd "$ROOT"
  CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$BUILD_PATH" .
)

install -m 0755 "$BUILD_PATH" "$STAGED_BIN"

PLIST_TMP="$(mktemp "$PLIST_TARGET.XXXXXX")"

sed \
  -e "s#__HOME__#$HOME#g" \
  -e "s#__BIN_PATH__#$BIN_PATH#g" \
  "$PLIST_SOURCE" > "$PLIST_TMP"

WRAPPER_TMP="$(mktemp "$WRAPPER_PATH.XXXXXX")"
printf '%s\n' \
  '#!/usr/bin/env bash' \
  'set -euo pipefail' \
  'exec "$HOME/.local/share/jdbc-cli/jdbc-cli" "$@"' > "$WRAPPER_TMP"
chmod 0755 "$WRAPPER_TMP"

mv "$STAGED_BIN" "$BIN_PATH"
mv "$PLIST_TMP" "$PLIST_TARGET"
mv "$WRAPPER_TMP" "$WRAPPER_PATH"

launchctl bootout "gui/$(id -u)/$PLIST_NAME" 2>/dev/null || true
launchctl bootstrap "gui/$(id -u)" "$PLIST_TARGET"
launchctl kickstart -k "gui/$(id -u)/$PLIST_NAME"

echo "Installed jdbc-cli"
echo "Binary:  $BIN_PATH"
echo "Wrapper: $WRAPPER_PATH"
echo "Logs:    $LOG_DIR"
