# semcom_orchestrator

## What It Does

`semcom_orchestrator` is the composition root for the semcom semantic memory pipeline. It:

- Exposes an HTTP endpoint for chat and ingest operations
- Provides CLI subcommands for background LLM discovery and distillation passes
- Opens all shared resources (SQLite connections, index, retriever) and injects them into the pipeline libraries

All semcom components are in-process Go imports — there are no network calls between pipeline stages.

---

## Module Structure

```
semcom/
├── semcom_embed/        — semantic index; Index.Query() → QueryStats
├── semcom_store/        — main memory store; SQLite-backed Store interface
├── semcom_retrieve/     — roaring bitmap reverse index over semcom_store
├── semcom_personal/     — personal token registry; Store + Matcher
├── semcom_distill/      — distillation logic; Store + Distill() LLM call
├── semcom_llm/          — LLM client wrapping providertron/gemini
└── semcom_orchestrator/ — composition root (this module)
```

The orchestrator owns the lifecycle of all shared resources: it opens the SQLite connections, loads the index, constructs all stores and matchers, and passes them as dependencies to the pipeline methods.

---

## Pipeline

### serve (HTTP)

```
POST /chat
  │
  ▼
semcom_embed.Index.Query(text, thresholds)
  → QueryStats{L0IDs, OOVWords, ...}
  │
  ├─ [chat] semcom_retrieve.Query(l0IDs, topK)  → scored memory hits
  │          + store.GetRaw() for each hit
  │
  └─ semcom_store.Insert(Memory{...})
  │
  ▼
JSON response
```

Retrieve runs before insert so the current prompt never appears in its own results.

### discover (CLI)

```
semcom_store.UnprocessedMemories()
  │
  for each memory:
    semcom_personal.Discover(llm, raw)  → topics
    for each topic:
      Index.Query(topic) — OOVWords test (skip if globally known)
      personalStore.InsertToken / GetToken
      matcher.AddToken
    store.UpdateMemoryPersonalTokens + MarkMemoryDiscovered
```

### distill (CLI)

Runs discovery first, then:

```
distillStore.GetMetadata("last_distilled_id")
  │
  for each 15-turn window (3-turn overlap):
    store.GetChunk(start, end)
    semcom_distill.Distill(llm, conversation) → [{topic, snippet}]
    for each snippet:
      Index.Query(snippet) → semkeys
      matcher.Match(topicWords) → personalIDs
      distillStore.InsertDistillation(...)
    distillStore.SetMetadata("last_distilled_id", ...)
```

---

## HTTP API

### POST /chat

**Request:**

```json
{
  "operation": "chat",
  "prompt":    "Alice is working on Providertron",
  "by":        "user",
  "top_k":     5,
  "benchmark": "verbose"
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `operation` | string | yes | — | `"chat"` (retrieve + store) or `"ingest"` (store only) |
| `prompt` | string | yes | — | Message text |
| `by` | string | yes | — | `"user"` or `"model"` |
| `top_k` | int | no | `5` | Max memories to return (`chat` only) |
| `benchmark` | string | no | `"ignore"` | `"ignore"`, `"total"`, or `"verbose"` |

**Response (200):**

```json
{
  "memories": [
    {"memory_id": 3, "score": 7, "content": "Alice joined the team"},
    {"memory_id": 1, "score": 4, "content": "Providertron handles LLM routing"}
  ],
  "benchmark": {
    "embed_us":    142,
    "retrieve_us": 18,
    "store_us":    2100,
    "total_us":    2260
  }
}
```

`score` is the count of shared L0 cluster IDs between the prompt and that memory. `memories` is omitted when no matches are found.

Benchmark modes:

| Mode | Shape |
|------|-------|
| `"ignore"` | `benchmark` key omitted |
| `"total"` | `{"total_us": N}` |
| `"verbose"` | `{"embed_us": N, "retrieve_us": N, "store_us": N, "total_us": N}` |

**Error response (4xx/5xx):**

```json
{"error": "prompt is required"}
```

| Status | Cause |
|--------|-------|
| 400 | `operation` not `"chat"` or `"ingest"` |
| 400 | `prompt` missing or empty |
| 400 | `by` not `"user"` or `"model"` |
| 400 | `benchmark` not a recognised mode |
| 400 | Malformed JSON |
| 405 | Non-POST request |
| 500 | embed, store, or retrieve failure |

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `INDEX_PATH` | `index.bin` | Path to the built semcom_embed index |
| `DB_PATH` | `memory.db` | Main memory SQLite file |
| `PERSONAL_DB_PATH` | `personal.db` | Shared SQLite file for personal tokens and distillations |
| `PORT` | `8080` | HTTP listen port |
| `GOOGLE_API_KEY` / `GEMINI_API_KEY` | — | Required for `discover` / `distill` |
| `GEMINI_MODEL` | `gemini-2.5-flash-preview-04-17` | Model for LLM passes |

---

## Shared Database

`personal.db` is a single SQLite file opened once in `main` and shared across two modules via `*sql.DB`:

- `semcom_personal.NewStore(db)` — manages `personal_tokens`, `personal_semkeys`
- `semcom_distill.NewStore(db)` — manages `distillations`, `distillation_semkeys`, `distill_metadata`

Both schemas are applied at startup via `openSharedDB`. Cross-table transactions are possible because they share a connection.

---

## Storage Model

### memory.db

| Table | Purpose |
|-------|---------|
| `memories` | One row per stored message. Fields: `id`, `turn_id`, `source`, `raw_message`, `personal_tokens` (JSON), `discovered` (0/1), `created_at` |
| `memory_semkeys` | One row per L0 ID per memory — backing store for semcom_retrieve's reverse index |

### personal.db

| Table | Purpose |
|-------|---------|
| `personal_tokens` | Known personal entities (`word`, `type`) |
| `personal_semkeys` | Maps personal token IDs → memory IDs |
| `distillations` | Compressed knowledge snippets (`topic`, `snippet`, `personal_tokens` JSON) |
| `distillation_semkeys` | Maps distillation IDs → semkey values |
| `distill_metadata` | Key/value store for pass progress (e.g. `last_distilled_id`) |

---

## Source Layout

```
semcom_orchestrator/
├── main.go               # startup, resource wiring, subcommand dispatch, openSharedDB
├── orchestrator.go       # Orchestrator struct; Ingest, Chat, Retrieve methods
├── orchestrator_test.go
├── server.go             # HTTP handler and JSON request/response types
├── discovery.go          # RunDiscoveryPass
├── discovery_test.go
├── distillation.go       # RunDistillationPass
├── go.mod
├── COMPONENTS.md         # interface reference for all pipeline components
├── DOC.md                # this file
└── README.md             # quick start
```
