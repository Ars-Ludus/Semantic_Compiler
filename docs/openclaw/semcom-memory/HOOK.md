---
name: semcom-memory
description: "Injects relevant memories from semcom before the agent responds; ingests agent replies after delivery."
metadata:
  { "openclaw": { "emoji": "🧠", "events": ["before_prompt_build", "before_agent_finalize", "llm_output"] } }
---

# semcom-memory hook

Connects openClaw to the semcom semantic memory server (default: `http://localhost:8080`).

**On `before_prompt_build`**: queries semcom for memories relevant to the incoming user prompt and prepends them as a `<semcom_memory>` block.

**On `llm_output` and `before_agent_finalize`**: ingests the agent's final natural language response into semcom as a `model` turn (fire-and-forget). 
*   `llm_output` handles the embedded Pi-agent.
*   `before_agent_finalize` handles the Codex harness.
*   A deduplication mechanism ensures each response is only ingested once per run.
*   Filtered to only ingest `user` initiated triggers.

If semcom is unreachable, handlers fail silently — the hook never blocks the conversation.

## Configuration

The `semcom-memory` plugin should be configured in `openclaw.json`. It requires `allowConversationAccess` to be enabled for the `before_agent_finalize` hook to capture responses correctly.

```json
{
  "plugins": {
    "entries": {
      "semcom-memory": {
        "enabled": true,
        "hooks": {
          "allowConversationAccess": true
        },
        "config": {
          "url": "http://localhost:8080"
        }
      }
    }
  }
}
```

## See also

`docs/openclaw/INSTALL.md` in the semcom repository for full setup instructions.
