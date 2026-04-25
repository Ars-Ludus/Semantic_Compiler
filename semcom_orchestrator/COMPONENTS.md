# semcom Components Reference

Single-file reference for all three components in the semantic memory pipeline.

```
[caller] --HTTP POST /ingest--> [semcom_orchestrator]
                                        |
                          gRPC Query()  |  library Insert()
                                   +----|----+
                                   v         v
                           [semcom_embed] [semcom_store]
                           (gRPC :50051)  (SQLite file)
```

---

## semcom_embed

Converts text into a semantic fingerprint: an array of `l0_ids` (integer cluster IDs). Does not return a vector. Runs as a standalone gRPC server with a preloaded in-memory index.

**Source:** `../semcom_embed`  
**Module:** `semcom_embed` (local)  
**Protocol:** gRPC, default port `:50051`

### Proto Definition

```proto
service SemComEmbed {
  rpc Query (QueryRequest) returns (QueryResponse);
}

message QueryRequest {
  string text = 1;
  float t2 = 2;  // L2 threshold override (default: 0.25, zero = use server default)
  float t1 = 3;  // L1 threshold override (default: 0.20)
  float t0 = 4;  // L0 threshold override (default: 0.15)
}

message QueryResponse {
  repeated int32 l0_ids      = 1;  // semantic fingerprint — primary output
  int32          query_tokens = 2; // vocabulary words matched
  int32          l3_count    = 3;
  int32          l2_count    = 4;
  int32          l1_count    = 5;
  int32          l0_count    = 6;
  int64          query_us    = 7;  // processing time in microseconds
}
```

Generated stubs: `semcom_embed/proto/semcom.pb.go`, `semcom_embed/proto/semcom_grpc.pb.go`

### Go Client

```go
import (
    pb "semcom_embed/proto"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
)

conn, _ := grpc.NewClient(":50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
client := pb.NewSemComEmbedClient(conn)
resp, _ := client.Query(ctx, &pb.QueryRequest{Text: "input text"})
// resp.L0Ids — []int32 semantic fingerprint
```

### Starting the Server

```bash
cd ~/lab/projects/semcom_embed
./semcom serve --index index.bin --addr :50051
```

The index takes ~22ms to load; all queries are served from memory in ~130µs.

### Server Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--index` | `index.bin` | Path to built index file |
| `--addr` | `:50051` | TCP listen address |
| `--t2` | `0.25` | L2 match ratio threshold |
| `--t1` | `0.20` | L1 match ratio threshold |
| `--t0` | `0.15` | L0 match ratio threshold |

### Output Characteristics

`l0_ids` is an unordered set of integers. Typical counts: 1–3 for a single word, 5–15 for a short phrase, 10–30 for a full sentence. Similarity between two inputs is measured by set intersection (Jaccard or cardinality).

---

## semcom_store

Persists memory records with their semantic keys in a SQLite database. Imported as a Go library — not a network service.

**Source:** `../semcom_store`  
**Module:** `github.com/ars/semantic_store` (use `replace` directive, see go.mod)  
**Protocol:** Direct library import

### Store Interface

```go
import semanticstore "github.com/ars/semantic_store"

// Open creates or opens a SQLite store at path; schema is auto-created.
store, err := semanticstore.Open("/path/to/memories.db")

type Store interface {
    Insert(ctx context.Context, m *Memory) (int64, error)
    Get(ctx context.Context, id int64) (*Memory, error)
    AllSemKeys(ctx context.Context) ([]SemKeyRow, error)       // full index rebuild
    SemKeysSince(ctx context.Context, afterID int64) ([]SemKeyRow, error) // incremental
    Close() error
}
```

### Data Types

```go
type Memory struct {
    ID        int64
    TurnID    int64
    SummaryID *int64           // optional
    Source    Source           // "user" or "model"
    Raw       string
    CreatedAt time.Time
    SemKey    []uint32         // L0 cluster IDs from semcom_embed
}

type Source string
const (
    SourceUser  Source = "user"
    SourceModel Source = "model"
)

type SemKeyRow struct {
    Value    uint32
    MemoryID int64
}
```

### Schema (auto-applied)

```sql
CREATE TABLE memories (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    turn_id    INTEGER NOT NULL,
    summary_id INTEGER,
    source     TEXT NOT NULL CHECK(source IN ('user','model')),
    raw_message TEXT NOT NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE memory_semkeys (
    memory_id    INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    semkey_value INTEGER NOT NULL,
    PRIMARY KEY (memory_id, semkey_value)
);

CREATE INDEX idx_semkeys_value ON memory_semkeys(semkey_value);
```

---

## semcom_orchestrator

Receives memory ingestion requests over HTTP, calls semcom_embed for the semantic fingerprint, then persists the result via semcom_store.

**Source:** `../semcom_orchestrator` (this repo)  
**Protocol:** HTTP REST, default port `:8080`

### POST /ingest

**Request:**
```json
{
  "text":       "the raw message text",
  "turn_id":    1,
  "source":     "user",
  "summary_id": null
}
```

- `text` — required, non-empty
- `source` — required, `"user"` or `"model"`
- `summary_id` — optional integer

**Response (200):**
```json
{
  "memory_id":    1,
  "l0_count":     12,
  "query_tokens": 5,
  "query_us":     130
}
```

**Error (4xx/5xx):**
```json
{"error": "text is required"}
```

### Configuration (environment variables)

| Variable | Default | Description |
|----------|---------|-------------|
| `EMBED_ADDR` | `:50051` | semcom_embed gRPC address |
| `DB_PATH` | `memory.db` | SQLite database file path |
| `PORT` | `8080` | HTTP listen port |

### Starting the Server

```bash
cd ~/lab/projects/semcom_orchestrator
EMBED_ADDR=:50051 DB_PATH=memory.db PORT=8080 ./semcom_orchestrator
```

### Startup Sequence

1. Open SQLite store (creates file + schema if missing)
2. Dial semcom_embed via gRPC (lazy — no index load happens here)
3. Register `POST /ingest` handler
4. Serve HTTP; shutdown cleanly on SIGINT/SIGTERM

---

## Protocol Rationale

**semcom_embed is gRPC** because it holds a stateful 27MB in-memory index and must run as its own process. gRPC is the right protocol for a long-lived service called from Go with well-defined request/response types.

**semcom_store is a library** because it wraps SQLite, an embedded database. The orchestrator is its only consumer and is already Go. Wrapping SQLite in a gRPC server would add process isolation with no benefit and risk write-lock contention (SQLite supports one writer). If distribution is ever needed, the `Store` interface is the right extension point — only the implementation changes, not the orchestrator's import.

**semcom_orchestrator exposes HTTP REST** because its callers are LLM application code (Python, shell scripts, tool-use frameworks) that speak HTTP natively. gRPC would require stub generation on every caller.

---

## Known Issues

- `sqlite.go` in semcom_store parses `created_at` with a fixed-precision layout (`2006-01-02T15:04:05.999Z`). This may fail on timestamps without fractional seconds (e.g. `2024-01-01T00:00:00Z`). SQLite's `strftime` with `%f` always emits three decimal places, so this is unlikely to trigger in practice — but worth fixing if `Get` is called on externally inserted rows.
