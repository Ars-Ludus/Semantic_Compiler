# semcom Components Reference

Interface reference for all pipeline components. All components are in-process Go imports — no network calls between them.

> For a plain-language description of each pipeline and its correct step order, see [`LOOPS.md`](./LOOPS.md).

```
POST /chat  (any harness)
     │
     ▼
[semcom_adapter.NewHandler]
     │  Harness.Decode → CanonicalRequest → validate → Dispatcher
     │
     ▼
[semcom_orchestrator dispatcher closure]  — in main.go
     │  maps CanonicalRequest → IngestRequest / ChatRequest
     │
     ▼
[semcom_orchestrator.Orchestrator]
     │
     ├─ semcom_embed.Index.Query()              — global semantic fingerprint (L0 IDs)
     ├─ semcom_personal.Matcher.Match()         — personal token IDs (concurrent with embed)
     ├─ semcom_retrieve.Retriever               — global reverse index (semkey → memory_ids)
     ├─ semcom_personal.PersonalRetriever       — personal reverse index (personal_id → memory_ids)
     ├─ semcom_distill.DistillationRetriever    — distillation reverse index (l0+personal → distillation_ids)
     ├─ semcom_store.Store                      — memory persistence (memory.db)
     ├─ semcom_session.Tracker                  — session retrieval deduplication (memory.db, separate connection)
     └─ semcom_personal.Store + semcom_distill.Store  — personalization persistence (personal.db)
```

### HTTP entry point: `semcom_adapter`

`adapter.NewHandler(harness, dispatcher)` returns the `http.HandlerFunc` registered at `/chat`. It owns method checking, body reading, harness decode, validation, dispatch, and harness encode. The orchestrator never implements `http.Handler` directly.

To add a new harness, implement `adapter.Harness` in a new sub-package (e.g. `semcom_adapter/claudecode`) and register a second route in `main.go`:
```go
mux.Handle("/hooks/claude", adapter.NewHandler(claudecode.Harness{}, dispatcher))
```

## Structural Note: Global and Personal Tagging Are Parallel

Both tagging mechanisms share the same shape:

| | Global (semkeys) | Personal (tokens) |
|---|---|---|
| **ID source** | Hierarchical L0 cluster traversal in embedding index | Longest-match scan against registered vocabulary |
| **Vocabulary** | Fixed — pre-trained index geometry | Dynamic — grows as entities are discovered |
| **Junction table** | `memory_semkeys (memory_id, semkey_value)` | `memory_personal_tokens (memory_id, personal_id)` |
| **Reverse index** | `semcom_retrieve.Retriever` | `semcom_personal.PersonalRetriever` |
| **Retrieval weight** | 1× | 5× |

The two indexes are always kept separate — L0 IDs and personal token IDs share the same integer space and must never be combined in a single bitmap map.

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
// Longest-match phrase scan: multi-word phrases in the vocabulary are
// preferred over their constituent words.
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
    OOVWords    []string // words not found in the vocabulary (used for personal token OOV filter)
}
```

### Thresholds

| Field | Default | Description |
|-------|---------|-------------|
| `L2` | `0.25` | Minimum match ratio at L2 |
| `L1` | `0.20` | Minimum match ratio at L1 |
| `L0` | `0.15` | Minimum match ratio at L0 |

---

## semcom_store

Persists memory records and their semkey associations. Backed by SQLite (`memory.db`).

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
    Insert(ctx context.Context, m *Memory) (int32, error)
    Get(ctx context.Context, id int32) (*Memory, error)
    GetRaw(ctx context.Context, id int32) (string, error)

    AllSemKeys(ctx context.Context) ([]SemKeyRow, error)
    SemKeysSince(ctx context.Context, afterID int32) ([]SemKeyRow, error)

    MaxTurnID(ctx context.Context) (int32, error)
    MaxID(ctx context.Context) (int32, error)

    GetIDsBySessionID(ctx context.Context, sessionID string) ([]int32, error)
    GetDistinctSessionIDs(ctx context.Context) ([]string, error)
    GetMemoriesBySessionID(ctx context.Context, sessionID string) ([]*Memory, error)

    GetChunk(ctx context.Context, startID, endID int32) ([]*Memory, error)
    MemoriesContainingWord(ctx context.Context, word string) ([]*Memory, error)

    Close() error
}
```

### Memory

```go
type Memory struct {
    ID        int32
    TurnID    int32
    Source    Source     // "user" | "model"
    Raw       string
    SessionID string     // originating session; empty for memories ingested without a session
    CreatedAt time.Time
    SemKey    []uint32   // L0 cluster IDs
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
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    turn_id     INTEGER NOT NULL,
    source      TEXT    NOT NULL CHECK(source IN ('user','model')),
    raw_message TEXT    NOT NULL,
    session_id  TEXT,
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
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

In-memory roaring bitmap reverse index over `memory_semkeys`. Querying is sub-millisecond.

**Source:** `../semcom_retrieve`
**Module:** `github.com/ars/semcom_retrieve` (replace directive)
**Import:** `semcomretrieve "github.com/ars/semcom_retrieve"`

### Opening and Refreshing

```go
retriever, err := semcomretrieve.Open(store)  // builds full index at startup

// Call after every store.Insert to pull new semkeys into the in-memory index.
err := retriever.Refresh(ctx)
```

### Querying

```go
results := retriever.Query(l0IDs, topK, excludeIDs)  // topK=0 returns all matches; excludeIDs may be nil
// []Result{MemoryID int32, Score int}
// Score = count of shared L0 cluster IDs between query and memory
```

---

## semcom_personal

Personal token registry, matcher, and reverse index for memories. Backed by `personal.db` (shared with semcom_distill).

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
func (s *Store) GetToken(word string) (*Token, error)          // returns sql.ErrNoRows if not found
func (s *Store) GetAllTokens() (map[string]uint32, error)
func (s *Store) LinkMemory(memoryID int32, personalIDs []uint32) error  // INSERT OR IGNORE, idempotent
func (s *Store) GetAllLinks() ([]MemoryLink, error)
```

### Matcher

```go
pMatcher, err := personal.NewMatcher(pStore) // loads all tokens into memory

hits, unmapped := pMatcher.Match(words)  // words = semindex.SplitWords(text)
// hits     — []uint32, personal token IDs for known phrases/words
// unmapped — []string, words not in the registry

pMatcher.AddToken("NewEntity", id) // incremental update without DB round-trip
```

### PersonalRetriever

In-memory roaring bitmap reverse index: `personal_id → set of memory_ids`. Parallel structure to `semcom_retrieve.Retriever` but over personal tokens instead of L0 semkeys.

```go
pRetriever, err := personal.NewPersonalRetriever(pStore)  // loads all links at startup

pRetriever.AddLink(personalID uint32, memoryID int32)  // call after LinkMemory

counts := pRetriever.MemoryTokenCounts(personalIDs, excludeIDs)  // excludeIDs may be nil; map[int32]int (memory_id → hit count)
// Multiply by personalWeight (5) when merging into retrieval scores.
```

### Token / MemoryLink

```go
type Token struct {
    ID   uint32
    Word string   // always lowercase
    Type string   // "PERSON", "PLACE", "PROJECT", "ORGANIZATION", "TOPIC"
}

type MemoryLink struct {
    MemoryID   int32
    PersonalID uint32
}
```

### Schema

```sql
CREATE TABLE personal_tokens (
    id   INTEGER PRIMARY KEY AUTOINCREMENT,
    word TEXT NOT NULL UNIQUE,
    type TEXT NOT NULL
);

CREATE TABLE memory_personal_tokens (
    memory_id   INTEGER NOT NULL,
    personal_id INTEGER NOT NULL,
    PRIMARY KEY (memory_id, personal_id)
);
CREATE INDEX idx_mpt_personal ON memory_personal_tokens (personal_id);
```

---

## semcom_distill

Distillation store, LLM extraction, and distillation reverse index. Supports two extraction modes: a legacy sliding-window pass (`Distill`) and a session-scoped pass (`SessionDistill`). Backed by `personal.db` (shared with semcom_personal).

**Source:** `../semcom_distill`
**Module:** `semcom_distill` (replace directive)
**Import:** `distill "semcom_distill"`

### Opening

Takes the same `*sql.DB` as semcom_personal.

```go
db.Exec(distill.Schema)
dStore := distill.NewStore(db)
```

### Distill (legacy)

Single LLM call per 15-memory chunk — returns distillations and named entities as separate arrays.

```go
type LLMClient interface {
    GenerateJSON(ctx context.Context, prompt string, target interface{}) error
}

resp, err := distill.Distill(ctx, llm, conversationChunk)
// resp.Distillations — []DistilledSnippet{{Topic, Snippet}}
// resp.Entities      — []Entity{{Text, Type}}
```

### SessionDistill

Single LLM call per full session. Returns a unified `snippets` array — each entry carries a topic, a fact-dense snippet, and an optional entity name/type. No separate entity array; general knowledge (no entity) and entity-specific knowledge are both represented.

```go
resp, err := distill.SessionDistill(ctx, llm, conversationText, userLabel, modelLabel)
// resp.Snippets — []Snippet
```

`userLabel` and `modelLabel` are injected into the prompt so the LLM names people explicitly rather than using pronouns. Callers read them from the `SEMCOM_USER_NAME` / `SEMCOM_MODEL_NAME` env vars.

### Store Methods

```go
func (s *Store) InsertDistillation(d *Distillation) (int32, error)
func (s *Store) GetDistillationsByIDs(ctx context.Context, ids []int32) ([]Distillation, error)
func (s *Store) GetMetadata(key string) (string, error)   // "" if not set
func (s *Store) SetMetadata(key, value string) error      // upsert
func (s *Store) DeleteDistillationsBySessionID(ctx context.Context, sessionID string) error  // cascade removes semkey rows via FK
```

### DistillationRetriever

In-memory roaring bitmap reverse index with two separate maps: L0 semkeys and personal token IDs. Kept separate because L0 IDs and personal token IDs share the same integer space.

```go
dRetriever, err := distill.NewDistillationRetriever(dStore)  // loads all at startup

dRetriever.Add(id int32, semKeys []uint32, personalIDs []uint32)  // call after InsertDistillation

results := dRetriever.Query(l0IDs, personalIDs, personalWeight, excludeIDs)  // excludeIDs may be nil; []DistillationResult sorted desc
// DistillationResult{ID int32, Score int}
// Score = L0 hits + (personal hits × personalWeight)

ids := dRetriever.GetByPersonalID(tokenID)  // []int32 sorted desc by ID (most recent first); nil if none
```

### Types

```go
type Distillation struct {
    ID          int32
    Topic       string
    Snippet     string
    SessionID   string   // originating session; empty for legacy distillations
    PersonalIDs []uint32 // personal token IDs matched against topic words
    SemKeys     []uint32 // L0 cluster IDs for the snippet
}

// Snippet is the unit of output from SessionDistill.
type Snippet struct {
    Topic      string `json:"topic"`
    Snippet    string `json:"snippet"`
    Entity     string `json:"entity,omitempty"`
    EntityType string `json:"entity_type,omitempty"`
}

type SessionDistillationResponse struct {
    Snippets []Snippet `json:"snippets"`
}

// DistilledSnippet and Entity are used by the legacy Distill function only.
type Entity struct {
    Text string  // e.g. "Alice Chen"
    Type string  // "PERSON", "PLACE", "PROJECT", "ORGANIZATION", "TOPIC"
}
```

---

## semcom_llm

Concrete LLM client wrapping providertron/gemini. Satisfies the `LLMClient` interface of semcom_distill via structural typing.

**Source:** `../semcom_llm`
**Module:** `semcom_llm` (replace directive)
**Import:** `llmclient "semcom_llm"`

```go
client, err := llmclient.New(apiKey, model)
// client.GenerateJSON satisfies distill.LLMClient
```

---

## semcom_session

Tracks which memory IDs have been retrieved in each session. Backed by a dedicated `*sql.DB` connection to `memory.db` (not `personal.db` — session data belongs alongside the memories it references).

**Source:** `../semcom_session`
**Module:** `semcom_session` (replace directive)
**Import:** `session "semcom_session"`

### Opening

Takes a `*sql.DB` — lifecycle managed by the caller. Schema must be applied before creating the tracker.

```go
db, _ := sql.Open("sqlite", "memory.db")
db.Exec(session.Schema)
tracker := session.NewTracker(db)
```

### Tracker Methods

```go
// GetRetrievedIDs returns a bitmap of memory IDs previously retrieved in this session.
// Returns an empty bitmap (never nil) on error or missing session.
func (t *Tracker) GetRetrievedIDs(ctx context.Context, sessionID string) *roaring.Bitmap

// MarkRetrieved records that the given memory IDs were retrieved in this session.
// Uses INSERT OR IGNORE — safe to call with duplicate IDs.
func (t *Tracker) MarkRetrieved(ctx context.Context, sessionID string, memoryIDs []int32) error

// GetRetrievedDistillationIDs returns a bitmap of distillation IDs already returned in this session.
// Returns an empty bitmap (never nil) on error or missing session.
func (t *Tracker) GetRetrievedDistillationIDs(ctx context.Context, sessionID string) *roaring.Bitmap

// MarkDistillationRetrieved records that the given distillation IDs were returned in this session.
// Uses INSERT OR IGNORE — safe to call with duplicate IDs.
func (t *Tracker) MarkDistillationRetrieved(ctx context.Context, sessionID string, ids []int32) error
```

### Schema

```sql
CREATE TABLE session_retrievals (
    session_id TEXT    NOT NULL,
    memory_id  INTEGER NOT NULL,
    PRIMARY KEY (session_id, memory_id)
);

CREATE TABLE session_distillation_retrievals (
    session_id      TEXT    NOT NULL,
    distillation_id INTEGER NOT NULL,
    PRIMARY KEY (session_id, distillation_id)
);
```

---

## Protocol Rationale

**Everything is a library.** semcom_embed previously ran as a gRPC server; it is now an in-process import. This eliminates a network hop, removes the proto/stub layer, and simplifies deployment to a single binary.

**One SQLite connection for personalization.** semcom_personal and semcom_distill share a single `*sql.DB` opened by the orchestrator. This allows cross-table transactions and joins without SQLite file-locking complications.

**semcom_store remains separate.** The main memory store (`memory.db`) is distinct from the personalization store (`personal.db`). Memories are written on every chat turn; personal data is written only during background distillation passes.

**LLMClient is defined locally in semcom_distill.** The concrete `semcom_llm.Client` satisfies it via Go structural typing, keeping each module's dependency surface minimal.
