#!/usr/bin/env bash
# semcom UserPromptSubmit hook for Claude Code.
# Pipes the raw hook event JSON to the semcom server, which retrieves relevant
# memories and returns them as additionalContext for injection before the LLM call.
# Fails silently — if semcom is unreachable, Claude Code continues without context.

SEMCOM_URL="${SEMCOM_URL:-http://localhost:${SEMCOM_PORT:-8080}}"
SEMCOM_DIR="${SEMCOM_DIR:-$HOME/.local/share/semcom}"
LOG_FILE="$SEMCOM_DIR/semcom-hooks.log"

log() {
  printf '[%s] [semcom-claudecode] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%3NZ)" "$1" >> "$LOG_FILE" 2>/dev/null || true
}

input=$(cat)
prompt_len=$(printf '%s' "$input" | python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('prompt','')))" 2>/dev/null || echo "?")
log "UserPromptSubmit — prompt length=${prompt_len}"

result=$(printf '%s' "$input" | curl -sf -X POST "$SEMCOM_URL/hooks/claude" \
    -H 'Content-Type: application/json' \
    --data @- \
    --connect-timeout 2 --max-time 5 2>/dev/null)

if [ $? -ne 0 ] || [ -z "$result" ]; then
  log "semcom unreachable or empty response"
  echo "{}"
  exit 0
fi

# Log hit count if present
hit_count=$(printf '%s' "$result" | python3 -c "
import sys, json
d = json.load(sys.stdin)
ctx = d.get('hookSpecificOutput', {}).get('additionalContext', '')
print(ctx.count('[distilled') + ctx.count('[raw'))
" 2>/dev/null || echo "0")
if [ "$hit_count" = "0" ]; then
  log "semcom returned 0 context hits"
else
  log "semcom returned ${hit_count} hit(s) — injecting semcom_memory block"
fi

printf '%s' "$result"
