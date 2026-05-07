# Semcom Uninstall

Run these commands in order.

## 1. Stop the server

```bash
~/.local/share/semcom/stop.sh
```

## 2. Disable and remove the hooks

```bash
openclaw hooks disable semcom-memory 2>/dev/null || true
openclaw hooks disable semcom-start  2>/dev/null || true
rm -rf ~/.openclaw/hooks/semcom-memory
rm -rf ~/.openclaw/hooks/semcom-start
```

## 3. Remove the install directory

If you want to keep your memory history, back up the databases first:

```bash
cp ~/.local/share/semcom/memory.db   ~/semcom-memory-backup.db
cp ~/.local/share/semcom/personal.db ~/semcom-personal-backup.db
```

Then delete:

```bash
rm -rf ~/.local/share/semcom
```

## 4. Verify

```bash
openclaw hooks list | grep semcom   # should return nothing
curl -s http://localhost:8080/chat  # should return connection refused
```
