#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
JAR_DIR="$HOME/.local/share/jdbc-cli"
JAR_PATH="$JAR_DIR/jdbc-cli-all.jar"
PLIST_LABEL="com.scriptease.jdbc-cli"
PLIST_DST="$HOME/Library/LaunchAgents/$PLIST_LABEL.plist"
WRAPPER="/opt/homebrew/bin/jdbc-cli"
LOG_DIR="$HOME/Library/Logs/jdbc-cli"
UID_VAL="$(id -u)"

echo "==> Building fat-jar..."
(cd "$REPO_DIR" && ./gradlew shadowJar -q)

echo "==> Installing JAR to $JAR_PATH..."
mkdir -p "$JAR_DIR"
cp "$REPO_DIR/build/libs/jdbc-cli-all.jar" "$JAR_PATH"

echo "==> Creating log directory..."
mkdir -p "$LOG_DIR"

echo "==> Writing launchd plist to $PLIST_DST..."
mkdir -p "$HOME/Library/LaunchAgents"
sed \
  -e "s|__HOME__|$HOME|g" \
  -e "s|__JAR_PATH__|$JAR_PATH|g" \
  "$REPO_DIR/launchd/$PLIST_LABEL.plist" > "$PLIST_DST"

echo "==> Bootstrapping launchd service..."
# Unload if already loaded (ignore errors)
launchctl bootout "gui/$UID_VAL/$PLIST_LABEL" 2>/dev/null || true
launchctl bootstrap "gui/$UID_VAL" "$PLIST_DST"

echo "==> Installing wrapper to $WRAPPER..."
cat > "$WRAPPER" <<'EOF'
#!/usr/bin/env bash
SOCKET="$HOME/.jdbc-cli.sock"
exec curl --silent --unix-socket "$SOCKET" "$@" 2>/dev/null || \
  exec java -cp ~/.local/share/jdbc-cli/jdbc-cli-all.jar MainKt "$@"
EOF

# Actually the wrapper should call the client subcommand
cat > "$WRAPPER" <<EOF
#!/usr/bin/env bash
exec java -jar "\$HOME/.local/share/jdbc-cli/jdbc-cli-all.jar" "\$@"
EOF
chmod +x "$WRAPPER"

echo "==> Waiting for daemon to start..."
for i in $(seq 1 10); do
  if java -jar "$JAR_PATH" ping 2>/dev/null | grep -q ok; then
    echo "==> jdbc-cli daemon is up."
    exit 0
  fi
  sleep 1
done

echo "WARNING: daemon did not respond to ping within 10 seconds. Check $LOG_DIR/daemon.log"
exit 1
