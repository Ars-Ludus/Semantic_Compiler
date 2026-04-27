# semcom_embed

## What It Does

`semcom_embed` is a semantic embedding replacement. Given a text input, it returns a `QueryStats` struct containing `L0IDs` — integer cluster IDs that represent the semantic meaning of the input. These IDs are the primary output and should be treated as the semantic fingerprint of the text.

It does not return a vector. It returns discrete cluster IDs from a 4-level hierarchical compression of a 72,983-word vocabulary. The hierarchy compresses meaning progressively:

- **L3** (117 clusters) — broad semantic direction
- **L2** (575 clusters) — semantic domain
- **L1** (2,841 clusters) — conceptual neighbourhood
- **L0** (14,380 clusters) — specific concept clusters

At query time, the top 5 L3 clusters by token overlap are selected, then each downstream level filters by match ratio (L2: 25%, L1: 20%, L0: 15%). The resulting `L0IDs` array defines the semantic meaning of the input as a set of concept clusters. Words not found in the vocabulary are collected in `OOVWords` rather than silently dropped.

Query time is approximately **130 microseconds** once the index is loaded.

---

## Go Library Interface

`semcom_embed` is an in-process Go library (package `semindex`). Import it directly — no network or RPC involved.

```go
import semindex "semcom_embed"

// Load the index once at startup (~22ms from disk):
idx, err := semindex.Load("index.bin")

// Query is safe for concurrent use (~130µs per call):
stats := idx.Query("your input text here", semindex.Thresholds{
    L2: 0.25,
    L1: 0.20,
    L0: 0.15,
})

// stats.L0IDs       — []int32, the semantic fingerprint (primary output)
// stats.OOVWords    — []string, words not found in the vocabulary
// stats.TokenIDs    — []int32, individual token IDs that contributed
// stats.QueryTokens — int, vocabulary words matched
// stats.L3/L2/L1    — int, cluster counts at each level
```

The index is held in memory for the lifetime of the process. All queries run entirely in memory.

### SplitWords

```go
// SplitWords lowercases and tokenises text (splits on non-letter, non-digit runes).
words := semindex.SplitWords("Hello, World!")  // → ["hello", "world"]

// SplitWordsPreserveCase splits without lowercasing — used for personal token matching.
words := semindex.SplitWordsPreserveCase("Hello, World!")  // → ["Hello", "World"]
```

---

## CLI

```bash
# Build the index from PostgreSQL (run once, or after vocabulary updates):
semcom build --dsn "postgres://user:pass@host:5432/memory_db" --out index.bin

# Query the index:
semcom query --index index.bin "your input text here"
```

### Build flags

| Flag | Default | Description |
|------|---------|-------------|
| `--dsn` | `postgres://ars@localhost:5432/memory_db` | PostgreSQL DSN |
| `--out` | `index.bin` | Output file path |

### Query flags

| Flag | Default | Description |
|------|---------|-------------|
| `--index` | `index.bin` | Path to the built index file |
| `--t2` | `0.25` | L2 match ratio threshold |
| `--t1` | `0.20` | L1 match ratio threshold |
| `--t0` | `0.15` | L0 match ratio threshold |

---

## What to Expect from the Output

`L0IDs` is an unordered array of integers. The count varies with input length and specificity:

| Input type | Typical L0IDs count |
|---|---|
| Single unambiguous word | 1–3 |
| Short phrase (2–4 words) | 5–15 |
| Full sentence (10+ words) | 10–30 |

A short, precise query returns a tight cluster set. A longer query activates more L3 paths and returns a broader but still semantically coherent set. The array as a whole defines meaning — individual IDs are not meaningful in isolation.

Two semantically similar inputs will produce overlapping `L0IDs` arrays. Similarity is measured as set intersection cardinality (the `score` field in semcom_retrieve results).

`OOVWords` contains words that were not found in the 72,983-word vocabulary. The discovery pass uses this list to identify candidate personal tokens — words that are meaningful to the user but absent from the global index.
