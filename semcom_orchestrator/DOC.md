# semcom_orchestrator

## What It Does

`semcom_orchestrator` is the entry point for the semcom semantic memory pipeline. It accepts requests over HTTP, dispatches them through the embedding, retrieval, and storage pipeline, and returns retrieved context with optional timing data.

The orchestrator does not implement embedding, retrieval, or storage itself — it coordinates the three components that do:

- **semcom_embed** converts text to a semantic fingerprint (an array of L0 cluster IDs)
- **semcom_retrieve** queries an in-memory roaring bitmap reverse index to find relevant past memories
- **semcom_store** persists the original text alongside its fingerprint in SQLite

---

## Operations

The orchestrator is designed around named operations. Each operation defines a specific composition of the pipeline components. The current operation is `chat`.

### `chat`

The `chat` operation handles a conversational turn. It:

1. Embeds the prompt (single gRPC call to semcom_embed)
2. Retrieves the top-K most semantically relevant past memories (in-memory, sub-millisecond)
3. Stores the prompt and its embedding in SQLite

Retrieve runs before insert so the current prompt does not appear in its own results.

---

## Pipeline

```
caller
  │
  │  POST /chat  (HTTP JSON)
  ▼
semcom_orchestrator
  │
  │  Query(text)  (gRPC)
  ▼
semcom_embed
  │
  │  []l0_ids
  ▼
semcom_orchestrator
  ├──────────────────────────────────┐
  │  retriever.Query(l0_ids, top_k)  │  (in-memory, Go library call)
  │  ▼                               │
  │  semcom_retrieve                 │
  │  (roaring bitmap reverse index)  │
  │                                  │
  │  store.Insert(Memory{...})        │  (Go library call)
  │  ▼                               │
  │  semcom_store → SQLite           │
  └──────────────────────────────────┘
  │
  │  {"memories": [...], "benchmark": {...}}  (HTTP JSON)
  ▼
caller
```

A typical sentence takes ~130µs for the embed gRPC call, ~10–30µs for the in-memory retrieve, and a few milliseconds for the SQLite write. End-to-end the retrieve step is well under 1ms.

---

## HTTP API

### POST /chat

Embeds the prompt, retrieves relevant past memories, and stores the prompt.

**Request body:**

```json
{
  "operation":  "chat",
  "prompt":     "the user's message",
  "by":         "user",
  "source":     "conversation",
  "turn_id":    1,
  "top_k":      5,
  "benchmark":  "verbose"
}
```

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `operation` | string | yes | — | Must be `"chat"` |
| `prompt` | string | yes | — | Raw message text to embed, retrieve against, and store |
| `by` | string | yes | — | `"user"` or `"model"` — who sent the message |
| `source` | string | no | — | Accepted and stored for future document tagging; currently unused |
| `turn_id` | integer | no | `0` | Conversation turn identifier |
| `top_k` | integer | no | `5` | Maximum number of memories to return |
| `benchmark` | string | no | `"ignore"` | `"ignore"`, `"total"`, or `"verbose"` — controls timing output |

**Response (200):**

```json
{
  "memories": [
    {"memory_id": 42, "score": 7},
    {"memory_id": 38, "score": 5}
  ],
  "benchmark": {
    "embed_us":    130,
    "retrieve_us": 12,
    "store_us":    2100,
    "total_us":    2242
  }
}
```

| Field | Description |
|-------|-------------|
| `memories` | Ranked list of past memories by semantic overlap. Omitted if no matches. Each `score` is the count of shared L0 cluster IDs between the prompt and that memory. |
| `benchmark` | Timing block. Shape depends on the `benchmark` field in the request (see below). Omitted when `benchmark` is `"ignore"`. |

**Benchmark modes:**

| Mode | Response shape |
|------|----------------|
| `"ignore"` | `benchmark` key omitted entirely |
| `"total"` | `{"total_us": N}` |
| `"verbose"` | `{"embed_us": N, "retrieve_us": N, "store_us": N, "total_us": N}` |

`total_us` is the sum of the three step times, not wall-clock elapsed. This makes each step's contribution explicit and additive.

**Error response (4xx/5xx):**

```json
{"error": "prompt is required"}
```

| Status | Cause |
|--------|-------|
| 400 | `operation` missing or not `"chat"` |
| 400 | `prompt` missing or empty |
| 400 | `by` not `"user"` or `"model"` |
| 400 | `benchmark` not a recognized mode |
| 400 | Malformed JSON |
| 405 | Non-POST request |
| 500 | semcom_embed unreachable or semcom_store write failed |

---

## Communication with semcom_embed

semcom_embed runs as a standalone gRPC server. The orchestrator connects to it at startup using a persistent `grpc.ClientConn` and calls the `Query` RPC for each request. A single embed call feeds both the retrieve and insert steps — the prompt is never embedded twice.

**Why gRPC:** semcom_embed holds a 27MB in-memory index that takes ~22ms to load. It runs as its own process and serves multiple callers. gRPC is the right protocol for a stateful, long-lived service with a well-defined request/response schema.

**Proto:**

```proto
service SemComEmbed {
  rpc Query (QueryRequest) returns (QueryResponse);
}

message QueryRequest {
  string text = 1;
  float t2 = 2;  // L2 threshold override (zero = use server default: 0.25)
  float t1 = 3;  // L1 threshold override (zero = use server default: 0.20)
  float t0 = 4;  // L0 threshold override (zero = use server default: 0.15)
}

message QueryResponse {
  repeated int32 l0_ids      = 1;  // semantic fingerprint
  int32          query_tokens = 2;
  int32          l3_count    = 3;
  int32          l2_count    = 4;
  int32          l1_count    = 5;
  int32          l0_count    = 6;
  int64          query_us    = 7;
}
```

The orchestrator sends only `text` and leaves the threshold fields at zero, deferring to the server defaults. The primary output is `l0_ids`: an unordered array of integer cluster IDs that forms the semantic fingerprint of the input.

The generated stubs are imported directly from the `semcom_embed` module via a `replace` directive in `go.mod`:

```
replace semcom_embed => ../semcom_embed
```

---

## Communication with semcom_retrieve

semcom_retrieve is imported as a Go library. It holds an in-memory `map[uint32]*roaring.Bitmap` reverse index: each key is an L0 cluster ID; each value is a bitmap of the memory IDs that carry that cluster. Querying it takes ~10–30µs.

The index is built from SQLite at startup via `AllSemKeys()` and kept current by a background goroutine that calls `SemKeysSince()` every 50ms. Because the retrieve step runs before the insert step within each request, the current prompt never appears in its own results regardless of refresh timing.

```go
retriever, err := semcomretrieve.Open(store, semcomretrieve.Options{AutoRefresh: true})
results := retriever.Query(l0IDs, topK)  // []Result{MemoryID, Score}
```

The module is imported via:

```
replace github.com/ars/semcom_retrieve => ../semcom_retrieve
```

---

## Communication with semcom_store

semcom_store is imported as a Go library — there is no network call, no separate process, and no protocol. The orchestrator calls `store.Insert()` directly in the same process.

**Why a library:** semcom_store wraps SQLite, an embedded database. Running it as a separate process would add a network hop with no benefit, and a two-process setup sharing a SQLite file risks write-lock contention. The `Store` interface is the right abstraction boundary: if a network backend is ever needed, only the implementation changes — the orchestrator's call site stays the same.

The `SemKey` field passed to `Insert` is the `l0_ids` array from semcom_embed, converted from `[]int32` to `[]uint32`. These become rows in the `memory_semkeys` table, indexed by `semkey_value` for fast reverse lookup.

```go
store.Insert(ctx, &Memory{
    TurnID: req.TurnID,
    Source: req.By,     // semanticstore.Source("user" | "model")
    Raw:    req.Prompt,
    SemKey: l0IDs,      // L0 IDs from semcom_embed
})
```

The store module is imported via:

```
replace github.com/ars/semantic_store => ../semcom_store
```

---

## Configuration

All configuration is read from environment variables at startup. There are no config files.

| Variable | Default | Description |
|----------|---------|-------------|
| `EMBED_ADDR` | `:50051` | semcom_embed gRPC address |
| `DB_PATH` | `memory.db` | SQLite file path; created automatically if missing |
| `PORT` | `8080` | HTTP listen port |

---

## Storage Model

Each chat request writes two things to SQLite:

1. A row in `memories` — the original text, source (`by`), turn ID, and timestamp
2. One row per L0 cluster ID in `memory_semkeys` — the semantic fingerprint, stored as individual integer rows indexed by value

The `memory_semkeys` table is the backing store for the roaring bitmap reverse index in semcom_retrieve. The index has a fixed width bounded by the 14,380 possible L0 clusters; growth in the table is linear at O(N × K) where K is the average number of L0 IDs per memory (~15 for a typical sentence).

---

## Source Layout

```
semcom_orchestrator/
├── main.go               # config, wiring, HTTP server, graceful shutdown
├── orchestrator.go       # Orchestrator struct; Ingest, Retrieve, and Chat pipeline methods
├── orchestrator_test.go
├── server.go             # HTTP handler and JSON request/response types
├── go.mod                # replace directives for semcom_embed, semcom_store, semcom_retrieve
├── COMPONENTS.md         # interface reference for all pipeline components
├── DOC.md                # this file
└── README.md             # quick start
```
