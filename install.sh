#!/usr/bin/env bash
# Install semcom on the current machine from a release tarball.
# Run from the directory containing the unpacked release files:
#
#   tar xzf semcom-release.tar.gz
#   bash install.sh
#
# No root or sudo required. No Go required.
#
# Optional env overrides:
#   SEMCOM_DIR   — install directory (default: ~/.local/share/semcom)
#   SEMCOM_PORT  — HTTP port (default: 8080)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SEMCOM_DIR="${SEMCOM_DIR:-$HOME/.local/share/semcom}"
SEMCOM_PORT="${SEMCOM_PORT:-8080}"

echo "==> semcom install"
echo "    source:  $SCRIPT_DIR"
echo "    install: $SEMCOM_DIR"
echo "    port:    $SEMCOM_PORT"
echo ""

# --- 1. Install files ---
echo "==> Installing files..."
mkdir -p "$SEMCOM_DIR"

if [ ! -f "$SCRIPT_DIR/semcom" ]; then
  echo "ERROR: semcom binary not found at $SCRIPT_DIR/semcom" >&2
  echo "       Run build-release.sh on your local machine first, then scp the tarball here." >&2
  exit 1
fi

cp "$SCRIPT_DIR/semcom" "$SEMCOM_DIR/semcom"
chmod +x "$SEMCOM_DIR/semcom"
cp "$SCRIPT_DIR/semcom_embed/index.bin" "$SEMCOM_DIR/index.bin"
echo "    OK: binary + index.bin"

# Write .env template if one doesn't already exist.
if [ ! -f "$SEMCOM_DIR/.env" ]; then
  cat > "$SEMCOM_DIR/.env" <<'EOF'
# semcom environment — sourced by start.sh and the semcom-start hook on every boot.
# Set your Gemini API key here so it persists across gateway and container restarts.

GOOGLE_API_KEY=

# Optional overrides:
# GEMINI_MODEL=gemini-3.1-flash-lite-preview
# PORT=8080
EOF
  echo "    OK: .env template written — add your GOOGLE_API_KEY to $SEMCOM_DIR/.env"
else
  echo "    OK: .env already exists — leaving it unchanged"
fi

# Write start.sh — sources .env before launching the binary.
cat > "$SEMCOM_DIR/start.sh" <<EOF
#!/usr/bin/env bash
# Start semcom if it is not already running.
PIDFILE="$SEMCOM_DIR/semcom.pid"
if [ -f "\$PIDFILE" ] && kill -0 "\$(cat \$PIDFILE)" 2>/dev/null; then
  exit 0  # already running
fi
# Load persisted config (API key, model overrides, etc.)
if [ -f "$SEMCOM_DIR/.env" ]; then
  set -a; . "$SEMCOM_DIR/.env"; set +a
fi
PORT="\${PORT:-$SEMCOM_PORT}" \\
nohup "$SEMCOM_DIR/semcom" serve >"$SEMCOM_DIR/semcom.log" 2>&1 &
echo \$! > "\$PIDFILE"
EOF
chmod +x "$SEMCOM_DIR/start.sh"

cat > "$SEMCOM_DIR/stop.sh" <<EOF
#!/usr/bin/env bash
PIDFILE="$SEMCOM_DIR/semcom.pid"
if [ -f "\$PIDFILE" ]; then
  kill "\$(cat \$PIDFILE)" 2>/dev/null && rm -f "\$PIDFILE"
fi
EOF
chmod +x "$SEMCOM_DIR/stop.sh"
echo "    OK: start.sh / stop.sh"

# --- 2. Start the server now ---
echo "==> Starting semcom..."
"$SEMCOM_DIR/start.sh"
sleep 1
PIDFILE="$SEMCOM_DIR/semcom.pid"
if [ -f "$PIDFILE" ] && kill -0 "$(cat "$PIDFILE")" 2>/dev/null; then
  echo "    OK: running (pid $(cat "$PIDFILE"), port $SEMCOM_PORT)"
else
  echo "    WARN: process did not start — check $SEMCOM_DIR/semcom.log"
fi

# --- 3. Platform integrations (openClaw) ---
HOOK_SRC="$SCRIPT_DIR/docs/openclaw"
HOOK_DST="$HOME/.openclaw/hooks"
PLUGIN_DST="$SEMCOM_DIR/plugin"

if [ -d "$HOME/.openclaw" ]; then
  echo "==> Installing openClaw hooks..."
  mkdir -p "$HOOK_DST/semcom-start"

  cp "$HOOK_SRC/semcom-start/HOOK.md"     "$HOOK_DST/semcom-start/HOOK.md"
  cp "$HOOK_SRC/semcom-start/handler.ts"  "$HOOK_DST/semcom-start/handler.ts"

  if command -v openclaw &>/dev/null; then
    openclaw hooks enable semcom-start 2>/dev/null && echo "    OK: semcom-start enabled" \
      || echo "    WARN: run: openclaw hooks enable semcom-start"
    # Disable the legacy hook in case it was previously enabled.
    openclaw hooks disable semcom-memory 2>/dev/null || true
  else
    echo "    OK: semcom-start hook files installed"
    echo "    Run: openclaw hooks enable semcom-start"
  fi

  echo "==> Installing semcom Plugin SDK plugin..."
  mkdir -p "$PLUGIN_DST"
  cp "$HOOK_SRC/semcom-plugin/package.json"          "$PLUGIN_DST/package.json"
  cp "$HOOK_SRC/semcom-plugin/openclaw.plugin.json"  "$PLUGIN_DST/openclaw.plugin.json"
  cp "$HOOK_SRC/semcom-plugin/index.js"              "$PLUGIN_DST/index.js"

  if command -v openclaw &>/dev/null; then
    openclaw plugins install --link --dangerously-force-unsafe-install "$PLUGIN_DST" 2>/dev/null \
      && echo "    OK: semcom-memory plugin installed (restart gateway to activate)" \
      || echo "    WARN: plugin install failed — run manually:"
    echo "          openclaw plugins install --link --dangerously-force-unsafe-install $PLUGIN_DST"
  else
    echo "    OK: plugin files copied to $PLUGIN_DST"
    echo "    Run: openclaw plugins install --link --dangerously-force-unsafe-install $PLUGIN_DST"
  fi
else
  echo "    SKIP: ~/.openclaw not found — hooks and plugin not installed"
fi

# --- 4. Smoke test ---
echo "==> Smoke test..."
sleep 1
if curl -sf -X POST "http://localhost:$SEMCOM_PORT/chat" \
    -H 'Content-Type: application/json' \
    -d '{"operation":"ingest","prompt":"install verification","by":"user"}' \
    --connect-timeout 3 --max-time 5 -o /dev/null; then
  echo "    OK: /chat endpoint responding"
else
  echo "    WARN: no response — server may still be starting"
  echo "    Check: cat $SEMCOM_DIR/semcom.log"
fi

echo ""
echo "==> Install complete."
echo ""
echo "Next steps:"
echo "  1. Add your Gemini API key:      echo 'GOOGLE_API_KEY=your_key' >> $SEMCOM_DIR/.env"
echo "  2. Import conversation history:  $SEMCOM_DIR/semcom ingest-sessions"
echo "  3. Run distillation:             $SEMCOM_DIR/start.sh && $SEMCOM_DIR/semcom distill"
echo "  Logs:                            tail -f $SEMCOM_DIR/semcom.log"
