# semcom Components Reference

Interface reference for all pipeline components. All components are in-process Go imports — no network calls between them.

```
POST /chat
     │
     ▼
[semcom_orchestrator]
     │
     ├─ semcom_embed.Index.Query()       — semantic fingerprint
     ├─ semcom_retrieve.Retriever.Query() — ranked memory hits
     ├─ semcom_store.Store               — memory persistence
     ├─ semcom_personal.Store + Matcher  — personal token registry
     └─ semcom_distill.Store             — distillation persistence
```

---

## semcom_embed

Converts text to a semantic fingerprint: a set of L0 cluster IDs from a 4-level hierarchical index. All queries run in memory.

**Source:** `../semcom_embed`  
**Module:** `semcom_embed` (replace directive)  
**Import:** `semindex "semcom_embed"`

### Index

```go
// Load reads the index file once at startup (~22ms).
idx, err := semindex.Load("index.bin")

// Query is safe for concurrent use (~130µs per call).
stats := idx.Query("input text", semindex.Thresholds{
    L2: 0.25,
    L1: 0.20,
    L0: 0.15,
})
```

### QueryStats

```go
type QueryStats struct {
    QueryTokens int      // vocabulary words matched
    L3          int      // L3 clusters passed
    L2          int      // L2 clusters passed
    L1          int      // L1 clusters passed
    L0IDs       []int32  // passing L0 cluster IDs (semantic fingerprint)
    TokenIDs    []int32  // individual token IDs that contributed
    OOVWords    []string // words not found in the vocabulary
}
```

`L0IDs` is the primary output. `OOVWords` is used by the discovery pass to identify candidate personal tokens.

### Thresholds

| Field | Default | Description |
|-------|---------|-------------|
| `L2` | `0.25` | Minimum match ratio at L2 |
| `L1` | `0.20` | Minimum match ratio at L1 |
| `L0` | `0.15` | Minimum match ratio at L0 |

### CLI

```bash
# Build the index from PostgreSQL:
semcom build --dsn "postgres://user@host:5432/memory_db" --out index.bin

# Query the index:
semcom query --index index.bin "your input text"
```

---

## semcom_store

Persists memory records and their semkey associations. Backed by SQLite.

**Source:** `../semcom_store`  
**Module:** `github.com/ars/semantic_store` (replace directive)  
**Import:** `semanticstore "github.com/ars/semantic_store"`

### Opening

```go
store, err := semanticstore.Open("memory.db")
defer store.Close()
```

### Store Interface

```go
type Store interface {
    Insert(ctx context.Context, m *Memory) (int64, error)
    Get(ctx context.Context, id int64) (*Memory, error)
    GetRaw(ctx context.Context, id int64) (string, error)

    AllSemKeys(ctx context.Context) ([]SemKeyRow, error)
    SemKeysSince(ctx context.Context, afterID int64) ([]SemKeyRow, error)
    MaxTurnID(ctx context.Context) (int64, error)

    UnprocessedMemories(ctx context.Context) ([]*Memory, error)
    MarkMemoryDiscovered(ctx context.Context, memoryID int64) error
    GetChunk(ctx context.Context, startID, endID int64) ([]*Memory, error)
    MemoriesContainingWord(ctx context.Context, word string) ([]*Memory, error)
    UpdateMemoryPersonalTokens(ctx context.Context, memoryID int64, personalIDs []uint32) error

    Close() error
}
```

### Memory

```go
type Memory struct {
    ID          int64
    TurnID      int64
    Source      Source           // "user" | "model"
    Raw         string
    CreatedAt   time.Time
    SemKey      []uint32         // L0 cluster IDs
    PersonalIDs []uint32         // personal token IDs (nil until discovery runs)
    Discovered  bool
}

type Source string
const (
    SourceUser  Source = "user"
    SourceModel Source = "model"
)
```

### Schema

```sql
CREATE TABLE memories (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    turn_id         INTEGER NOT NULL,
    source          TEXT    NOT NULL CHECK(source IN ('user','model')),
    raw_message     TEXT    NOT NULL,
    personal_tokens TEXT,           -- JSON []uint32, set by discovery pass
    discovered      INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE memory_semkeys (
    memory_id    INTEGER NOT NULL REFERENCES memories(id) ON DELETE CASCADE,
    semkey_value INTEGER NOT NULL,
    PRIMARY KEY (memory_id, semkey_value)
);
CREATE INDEX idx_semkeys_value ON memory_semkeys(semkey_value);
```

---

## semcom_retrieve

In-memory roaring bitmap reverse index over semcom_store. Querying is sub-millisecond.

**Source:** `../semcom_retrieve`  
**Module:** `github.com/ars/semcom_retrieve` (replace directive)  
**Import:** `semcomretrieve "github.com/ars/semcom_retrieve"`

### Opening

```go
retriever, err := semcomretrieve.Open(store, semcomretrieve.Options{AutoRefresh: true})
defer retriever.Close()
```

`AutoRefresh: true` starts a background goroutine that calls `SemKeysSince` every 50ms to keep the index current without a full rebuild.

### Querying

```go
results := retriever.Query(l0IDs, topK)  // []Result{MemoryID int64, Score int}
```

`Score` is the count of shared L0 cluster IDs between the query and that memory.

---

## semcom_personal

Personal token registry: stores known entities (people, projects, etc.) and maps them to memory IDs.

**Source:** `../semcom_personal`  
**Module:** `semcom_personal` (replace directive)  
**Import:** `personal "semcom_personal"`

### Opening

Takes a `*sql.DB` — lifecycle managed by the caller.

```go
db, _ := sql.Open("sqlite", "personal.db")
db.Exec(personal.Schema)
pStore := personal.NewStore(db)
```

### Store Methods

```go
func (s *Store) InsertToken(word, tokenType string) (uint32, error)
func (s *Store) GetToken(word string) (*Token, error)
func (s *Store) GetAllTokens() (map[string]uint32, error)
func (s *Store) LinkMemory(memoryID int64, personalIDs []uint32) error
```

### Matcher

```go
pMatcher, err := personal.NewMatcher(pStore) // loads all tokens into memory

hits, unmapped := pMatcher.Match([]string{"Alice", "project"})
// hits     — []uint32, personal token IDs for known words
// unmapped — []string, words not in the registry

pMatcher.AddToken("NewEntity", id) // incremental update without DB round-trip
```

### Token

```go
type Token struct {
    ID   uint32
    Word string  // always lowercase
    Type string  // e.g. "PERSON", "PLACE", "TOPIC"
}
```

### LLMClient Interface

```go
type LLMClient interface {
    GenerateJSON(ctx context.Context, prompt string, target interface{}) error
}

resp, err := personal.Discover(ctx, llm, rawMessage)
// resp.Topics — []string extracted by the LLM
```

---

## semcom_distill

Distillation store and LLM call. Compresses conversation chunks into topic/snippet pairs.

**Source:** `../semcom_distill`  
**Module:** `semcom_distill` (replace directive)  
**Import:** `distill "semcom_distill"`

### Opening

Takes the same `*sql.DB` as semcom_personal — both share one connection.

```go
db.Exec(distill.Schema)
dStore := distill.NewStore(db)
```

### Distill

```go
type LLMClient interface {
    GenerateJSON(ctx context.Context, prompt string, target interface{}) error
}

resp, err := distill.Distill(ctx, llm, conversationChunk)
// resp.Distillations — []DistilledSnippet{{Topic, Snippet}}
```

### Store Methods

```go
func (s *Store) InsertDistillation(d *Distillation) (int64, error)
func (s *Store) GetMetadata(key string) (string, error)   // "" if not set
func (s *Store) SetMetadata(key, value string) error      // upsert
```

### Distillation

```go
type Distillation struct {
    ID          int64
    Topic       string
    Snippet     string
    PersonalIDs []uint32  // related personal token IDs
    SemKeys     []uint32  // L0 cluster IDs for the snippet
}
```

---

## semcom_llm

Concrete LLM client wrapping providertron/gemini. Satisfies the `LLMClient` interface of both semcom_personal and semcom_distill via structural typing.

**Source:** `../semcom_llm`  
**Module:** `semcom_llm` (replace directive)  
**Import:** `llmclient "semcom_llm"`

```go
client, err := llmclient.New(apiKey, model)
// client.GenerateJSON satisfies personal.LLMClient and distill.LLMClient
```

---

## Protocol Rationale

**Everything is a library.** semcom_embed previously ran as a gRPC server; it is now an in-process import. This eliminates a network hop, removes the proto/stub layer, and simplifies deployment to a single binary.

**One SQLite connection for personalization.** semcom_personal and semcom_distill share a single `*sql.DB` opened by the orchestrator. This allows cross-table transactions and joins between personal tokens and distillations without SQLite file-locking complications.

**semcom_store remains separate.** The main memory store (`memory.db`) is distinct from the personalization store (`personal.db`). The two datasets have different growth characteristics and access patterns — memories are written on every chat turn, while personal data is written only during background passes.

**LLMClient is defined locally in each module.** Both semcom_personal and semcom_distill define their own one-method `LLMClient` interface. The concrete `semcom_llm.Client` satisfies both via Go structural typing. This keeps each module's dependency surface minimal.
