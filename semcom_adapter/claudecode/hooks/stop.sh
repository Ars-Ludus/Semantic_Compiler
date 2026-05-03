#!/usr/bin/env bash
# semcom Stop hook for Claude Code.
# Extracts the last assistant response from the session JSONL file and ingests it
# into semcom memory. Fire-and-forget — always exits 0.

SEMCOM_URL="${SEMCOM_URL:-http://localhost:${SEMCOM_PORT:-8080}}"
SEMCOM_DIR="${SEMCOM_DIR:-$HOME/.local/share/semcom}"
LOG_FILE="$SEMCOM_DIR/semcom-hooks.log"

log() {
  printf '[%s] [semcom-claudecode] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)" "$1" >> "$LOG_FILE" 2>/dev/null || true
}

input=$(cat)

session_id=$(printf '%s' "$input" | python3 -c "import sys,json; print(json.load(sys.stdin).get('session_id',''))" 2>/dev/null)
if [ -z "$session_id" ]; then
  log "Stop: no session_id in input — skipping"
  exit 0
fi

# Find the session JSONL file across all project directories
session_file=$(find "$HOME/.claude/projects" -name "${session_id}.jsonl" 2>/dev/null | head -1)
if [ -z "$session_file" ]; then
  log "Stop: session file not found for ${session_id} — skipping"
  exit 0
fi

# Extract the last assistant text content from the session JSONL
last_text=$(python3 - "$session_file" <<'PYEOF'
import sys, json

last_text = ""
with open(sys.argv[1]) as f:
    for line in f:
        try:
            msg = json.loads(line.strip())
            if msg.get("type") == "assistant":
                content = msg.get("message", {}).get("content", [])
                texts = [b["text"] for b in content if isinstance(b, dict) and b.get("type") == "text" and b.get("text")]
                if texts:
                    last_text = " ".join(texts)
        except Exception:
            pass
print(last_text)
PYEOF
)

if [ -z "$last_text" ]; then
  log "Stop: no assistant text found in session — skipping"
  exit 0
fi

log "Stop: ingesting model reply (length=${#last_text})"

payload=$(python3 -c "import json,sys; print(json.dumps({'operation':'ingest','prompt':sys.argv[1],'by':'model'}))" "$last_text" 2>/dev/null)
if [ -z "$payload" ]; then
  log "Stop: failed to build ingest payload — skipping"
  exit 0
fi

curl -sf -X POST "$SEMCOM_URL/chat" \
  -H 'Content-Type: application/json' \
  --data-raw "$payload" \
  --connect-timeout 2 --max-time 5 \
  -o /dev/null 2>/dev/null || log "Stop: semcom ingest failed (server unreachable)"

exit 0
