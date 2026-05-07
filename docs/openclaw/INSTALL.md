# Semcom Install Guide

Semcom is a local semantic memory server. It stores every conversation turn and retrieves relevant context before the agent responds. It runs as a background process alongside openClaw.

**Requirements on the target machine:** none beyond a Linux x86-64 environment. No Go, no root, no package manager access needed. The binary is statically compiled and self-contained.

---

## Step 1 — Build the release (on your local machine)

This step requires Go and the semcom source. Run it once on your development machine, not on the VPS or container.

```bash
cd ~/lab/projects/semcom
bash build-release.sh
```

Output: `semcom-release.tar.gz` containing the binary, index, hooks, plugin, and install script.

Copy it to the target machine:

```bash
scp semcom-release.tar.gz user@host:~/
```

---

## Step 2 — Install (on the target machine)

```bash
tar xzf semcom-release.tar.gz
bash install.sh
```

The script:
1. Copies the binary and `index.bin` to `~/.local/share/semcom/`
2. Writes `start.sh` and `stop.sh` in the same directory
3. Starts the server immediately via `nohup`
4. Installs the `semcom-start` hook and enables it
5. Installs the `semcom-memory` Plugin SDK plugin and registers it with openClaw
6. Runs a smoke test

No root required. No systemd. No cron.

Override install directory or port if needed:

```bash
SEMCOM_DIR=~/.local/share/semcom SEMCOM_PORT=8080 bash install.sh
```

---

## What gets installed

| Path | What it is |
|------|-----------|
| `~/.local/share/semcom/semcom` | Server binary (static) |
| `~/.local/share/semcom/index.bin` | Pre-built semantic index (read-only) |
| `~/.local/share/semcom/memory.db` | Conversation memory (created on first run) |
| `~/.local/share/semcom/personal.db` | Named entities + distillations (created on first run) |
| `~/.local/share/semcom/start.sh` | Start the server (idempotent) |
| `~/.local/share/semcom/stop.sh` | Stop the server |
| `~/.local/share/semcom/semcom.log` | Server stdout/stderr |
| `~/.local/share/semcom/plugin/` | Plugin SDK plugin (memory injection) |
| `~/.openclaw/hooks/semcom-start/` | Starts semcom when the openClaw gateway boots |

---

## Step 3 — Add your API key

The API key persists in `~/.local/share/semcom/.env` and survives gateway and container restarts:

```bash
echo 'GEMINI_API_KEY=your_key_here' >> ~/.local/share/semcom/.env
```

The `semcom-start` hook and `start.sh` both source this file before launching the binary.

---

## Step 4 — Restart the gateway

The plugin is loaded at gateway startup. Restart openClaw to activate it:

```bash
openclaw restart
# or restart however you normally restart the gateway
```

After restart, `openclaw plugins list | grep semcom` should show the plugin as `loaded`.

---

## How memory injection works

**`semcom-memory` Plugin SDK plugin** (`~/.local/share/semcom/plugin/`) registers a `before_prompt_build` hook. This fires on every inbound message, queries semcom for relevant memories, and returns a `prependContext` block that openClaw injects directly into the prompt before the LLM call. The agent sees a `<semcom_memory>` block above the user message.

After the agent replies, a `message_sent` handler ingests the response as a `model` turn (fire-and-forget).

If semcom is unreachable, both handlers fail silently — message delivery is never blocked.

**`semcom-start` hook** (`~/.openclaw/hooks/semcom-start/`) fires on `gateway:startup`. It checks whether semcom is already listening and spawns it if not. Every time openClaw starts, semcom starts with it — no init system required.

---

## Verify

### 1. Plugin is loaded

```bash
openclaw plugins list | grep semcom
# Expected: semcom-memory  loaded
```

### 2. Server is responding

```bash
curl -s -X POST http://localhost:8080/chat \
  -H 'Content-Type: application/json' \
  -d '{"operation":"ingest","prompt":"install verification","by":"user"}'
# Expected: {}
```

### 3. Hook is enabled

```bash
openclaw hooks list | grep semcom
# Expected: semcom-start ✓ ready
```

### 4. Watch the live log

```bash
tail -f ~/.local/share/semcom/semcom-hooks.log
```

Send a message through openClaw. You should see:

```
[semcom-plugin] before_prompt_build — prompt length=NNN
[semcom-plugin] semcom returned N hit(s): id=X score=Y, ...
```

### 5. End-to-end retrieval

```bash
curl -s -X POST http://localhost:8080/chat -H 'Content-Type: application/json' \
  -d '{"operation":"ingest","prompt":"Alice works on the retrieval layer.","by":"user"}'

curl -s -X POST http://localhost:8080/chat -H 'Content-Type: application/json' \
  -d '{"operation":"chat","prompt":"Who works on retrieval?","by":"user","top_k":3}'
# Expected: context array containing the Alice memory
```

---

## Import existing conversation history

```bash
~/.local/share/semcom/semcom ingest-sessions
```

Reads `~/.openclaw/agents/main/sessions/*.jsonl`, skips already-imported sessions, stores every user/assistant turn. Idempotent — safe to re-run.

Override the sessions path:

```bash
~/.local/share/semcom/semcom ingest-sessions \
  --sessions-dir ~/.openclaw/agents/main/sessions
```

---

## Run the distillation pass (optional)

Compresses chunks into dense topic/snippet pairs and extracts named entities. Improves retrieval quality. Requires a Gemini API key.

```bash
~/.local/share/semcom/semcom distill
```

The API key is read from `~/.local/share/semcom/.env` (or `GEMINI_API_KEY` env var). Progress is checkpointed every 15 memories — interrupting and restarting is safe.

---

## Process management

```bash
# Start
~/.local/share/semcom/start.sh

# Stop
~/.local/share/semcom/stop.sh

# Logs
tail -f ~/.local/share/semcom/semcom.log
tail -f ~/.local/share/semcom/semcom-hooks.log
```

---

## Configuration

### Change the semcom URL (non-default port)

Edit `~/.local/share/semcom/.env`:

```bash
PORT=8081
```

Then update the plugin's env config in `~/.openclaw/openclaw.json` so it queries the right port:

```json
{
  "plugins": {
    "entries": {
      "semcom-memory": {
        "enabled": true,
        "pluginConfig": { "url": "http://localhost:8081" }
      }
    }
  }
}
```

And update the `semcom-start` hook env so it passes the right port when spawning:

```json
{
  "hooks": {
    "internal": {
      "entries": {
        "semcom-start": { "enabled": true, "env": { "SEMCOM_PORT": "8081" } }
      }
    }
  }
}
```

---

## Troubleshooting

**`semcom binary not found` during install**

You ran `install.sh` without unpacking the release first:

```bash
tar xzf semcom-release.tar.gz
bash install.sh
```

**`Connection refused` on port 8080**

Start the server manually and check the log:

```bash
~/.local/share/semcom/start.sh
cat ~/.local/share/semcom/semcom.log
```

**Plugin shows `disabled` or `not loaded` in `openclaw plugins list`**

Re-run the install command manually:

```bash
openclaw plugins install --link --dangerously-force-unsafe-install ~/.local/share/semcom/plugin/
```

Then restart the gateway.

**No `<semcom_memory>` block visible in chat**

Check the hook log after sending a message:

```bash
tail -20 ~/.local/share/semcom/semcom-hooks.log
```

- If `before_prompt_build` lines appear with hits — the plugin is working; the memory block is being injected but may not be visible in your client's UI.
- If `semcom unreachable` — the server isn't running: `~/.local/share/semcom/start.sh`
- If no `before_prompt_build` lines appear — the plugin is not loaded; check `openclaw plugins list` and restart the gateway.

**Context is always empty (0 hits)**

Expected on a fresh install — no memories yet. Run:

```bash
~/.local/share/semcom/semcom ingest-sessions
```

---

## Uninstall

```bash
~/.local/share/semcom/stop.sh
openclaw plugins uninstall semcom-memory 2>/dev/null || true
openclaw hooks disable semcom-start 2>/dev/null || true
rm -rf ~/.local/share/semcom
rm -rf ~/.openclaw/hooks/semcom-start
```

Back up `memory.db` and `personal.db` first if you want to keep your history.
