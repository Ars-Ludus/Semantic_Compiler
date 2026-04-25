# semcom_orchestrator

The orchestration layer for the semcom semantic memory pipeline. Receives requests over HTTP, embeds input via `semcom_embed`, retrieves relevant past memories via `semcom_retrieve`, and persists the result via `semcom_store`.

## Prerequisites

- `semcom_embed` running as a gRPC server (default `:50051`)
- `semcom_store` — included as a Go library, no separate process needed
- `semcom_retrieve` — included as a Go library, no separate process needed

## Build

```bash
go build -o semcom_orchestrator .
```

## Run

```bash
# Defaults: embed at :50051, db at ./memory.db, HTTP on :8080
./semcom_orchestrator

# Override via environment variables
EMBED_ADDR=:50051 DB_PATH=/var/lib/semcom/memory.db PORT=8080 ./semcom_orchestrator
```

## Usage

**Chat — store a message and retrieve relevant context:**

```bash
curl -X POST http://localhost:8080/chat \
  -H 'Content-Type: application/json' \
  -d '{
    "operation": "chat",
    "prompt":    "the user said something",
    "by":        "user",
    "turn_id":   1,
    "top_k":     5,
    "benchmark": "verbose"
  }'
```

```json
{
  "memories": [
    {"memory_id": 3, "score": 7},
    {"memory_id": 1, "score": 4}
  ],
  "benchmark": {
    "embed_us":    134,
    "retrieve_us": 11,
    "store_us":    2180,
    "total_us":    2325
  }
}
```

`memories` contains past memories ranked by semantic overlap with the prompt (`score` = number of shared L0 cluster IDs). The prompt itself is stored after retrieval, so it never appears in its own results.

`benchmark` is controlled by the `benchmark` field:

| Value | Output |
|-------|--------|
| `"ignore"` (default) | `benchmark` key omitted |
| `"total"` | `{"total_us": N}` |
| `"verbose"` | all four timing fields |

## Request Fields

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `operation` | yes | — | Must be `"chat"` |
| `prompt` | yes | — | Message text |
| `by` | yes | — | `"user"` or `"model"` |
| `source` | no | — | Reserved for future document tagging |
| `turn_id` | no | `0` | Conversation turn identifier |
| `top_k` | no | `5` | Max memories to return |
| `benchmark` | no | `"ignore"` | `"ignore"`, `"total"`, or `"verbose"` |

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `EMBED_ADDR` | `:50051` | semcom_embed gRPC address |
| `DB_PATH` | `memory.db` | SQLite database file path |
| `PORT` | `8080` | HTTP listen port |

## See Also

- `DOC.md` — architecture, data flow, API reference, and protocol decisions
- `COMPONENTS.md` — full interface reference for all pipeline components
