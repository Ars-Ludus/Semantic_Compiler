---
name: semcom-start
description: "Starts the semcom memory server when the openClaw gateway boots, if it is not already running."
metadata:
  { "openclaw": { "emoji": "🧠", "events": ["gateway:startup"] } }
---

# semcom-start hook

Fires on `gateway:startup` and spawns the semcom server as a detached background process if it is not already listening. This replaces the need for systemd, cron, or any other process manager.

## Configuration

Set `SEMCOM_DIR` and `SEMCOM_PORT` in the hook's env config in `openclaw.json` if your install directory or port differs from the defaults:

```json
{
  "hooks": {
    "internal": {
      "entries": {
        "semcom-start": {
          "enabled": true,
          "env": {
            "SEMCOM_DIR": "/home/user/.openclaw/semcom",
            "SEMCOM_PORT": "8080"
          }
        }
      }
    }
  }
}
```
