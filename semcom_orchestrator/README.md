# semcom_orchestrator

Composition root for the semcom semantic memory pipeline. Exposes an HTTP API for chat/ingest and CLI subcommands for background LLM passes.

## Prerequisites

- `index.bin` — built by `semcom_embed/cmd/semcom build`
- `memory.db` — SQLite file; created automatically on first run
- `personal.db` — SQLite file; created automatically on first run (shared by personal token and distillation stores)

All semcom libraries (`semcom_embed`, `semcom_store`, `semcom_retrieve`, `semcom_personal`, `semcom_distill`) are in-process Go imports — no separate processes needed.

## Build

```bash
go build -o semcom_orchestrator .
```

## Run

```bash
# Defaults: index.bin, memory.db, personal.db, HTTP on :8080
./semcom_orchestrator

# Or with explicit paths
INDEX_PATH=/var/lib/semcom/index.bin \
DB_PATH=/var/lib/semcom/memory.db \
PERSONAL_DB_PATH=/var/lib/semcom/personal.db \
PORT=8080 \
./semcom_orchestrator serve
```

## Subcommands

| Subcommand | Description |
|------------|-------------|
| `serve` (default) | Start the HTTP server |
| `discover` | LLM pass — scan unprocessed memories and extract personal tokens |
| `distill` | LLM pass — run discovery, then compress conversation chunks into snippets |

`distill` always runs discovery first to ensure personal tokens are up to date.

```bash
GOOGLE_API_KEY=... ./semcom_orchestrator discover
GOOGLE_API_KEY=... ./semcom_orchestrator distill
```

## HTTP API

### POST /chat — store a message and retrieve relevant context

```bash
curl -X POST http://localhost:8080/chat \
  -H 'Content-Type: application/json' \
  -d '{"operation": "chat", "prompt": "Alice is working on Providertron", "by": "user", "top_k": 5}'
```

```json
{
  "memories": [
    {"memory_id": 3, "score": 7, "content": "Alice joined the team last week"},
    {"memory_id": 1, "score": 4, "content": "Providertron handles LLM routing"}
  ]
}
```

### POST /chat — store only (no retrieval)

```bash
curl -X POST http://localhost:8080/chat \
  -H 'Content-Type: application/json' \
  -d '{"operation": "ingest", "prompt": "a model response", "by": "model"}'
```

## Request Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `operation` | yes | — | `"chat"` (retrieve + store) or `"ingest"` (store only) |
| `prompt` | yes | — | Message text |
| `by` | yes | — | `"user"` or `"model"` |
| `top_k` | no | `5` | Max memories to return (chat only) |
| `benchmark` | no | `"ignore"` | `"ignore"`, `"total"`, or `"verbose"` |

## Response Fields

| Field | Description |
|-------|-------------|
| `memories` | Ranked past memories; omitted if none. `score` = shared L0 cluster count. |
| `benchmark` | Timing data; omitted when `benchmark` is `"ignore"`. |

Benchmark modes:

| Mode | Shape |
|------|-------|
| `"ignore"` | key omitted |
| `"total"` | `{"total_us": N}` |
| `"verbose"` | `{"embed_us": N, "retrieve_us": N, "store_us": N, "total_us": N}` |

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `INDEX_PATH` | `index.bin` | Semantic index built by semcom_embed |
| `DB_PATH` | `memory.db` | Main memory SQLite file |
| `PERSONAL_DB_PATH` | `personal.db` | Personal token + distillation SQLite file |
| `PORT` | `8080` | HTTP listen port |
| `GOOGLE_API_KEY` / `GEMINI_API_KEY` | — | Required for `discover` / `distill` subcommands |
| `GEMINI_MODEL` | `gemini-2.5-flash-preview-04-17` | Model used for LLM passes |

## See Also

- `DOC.md` — architecture, data flow, and API reference
- `COMPONENTS.md` — interface reference for all pipeline components
