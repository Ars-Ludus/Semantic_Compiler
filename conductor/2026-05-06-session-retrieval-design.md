# Session Retrieval Tracking Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a new library to track which memory IDs have been retrieved for an active session to prevent duplicate context injection in subsequent turns.

**Architecture:** A new Go package `semcom_session` will manage a persistent SQLite table (`session_retrievals`) on the shared `personal.db` connection. It will expose `GetRetrievedIDs` to fetch a session's history as a `roaring.Bitmap` and `MarkRetrieved` to record newly injected IDs. The orchestrator will use these methods to build an exclusion filter during the retrieval phase of the chat loop.

**Tech Stack:** Go, SQLite (`modernc.org/sqlite`), Roaring Bitmaps (`github.com/RoaringBitmap/roaring`).

---

### Task 1: Create the `semcom_session` package and schema

**Files:**
- Create: `semcom_session/schema.sql`
- Create: `semcom_session/tracker.go`
- Modify: `semcom_orchestrator/main.go:61` (Schema initialization)

- [ ] **Step 1: Write the schema file**

```sql
-- Create: semcom_session/schema.sql
CREATE TABLE IF NOT EXISTS session_retrievals (
    session_id TEXT NOT NULL,
    memory_id  INTEGER NOT NULL,
    PRIMARY KEY (session_id, memory_id)
);
```

- [ ] **Step 2: Write the `Tracker` struct and constructor**

```go
// Create: semcom_session/tracker.go
package semcom_session

import (
	"database/sql"
	_ "embed"
)

//go:embed schema.sql
var Schema string

// Tracker manages the persistence of retrieved memory IDs for active sessions.
type Tracker struct {
	db *sql.DB
}

// NewTracker creates a new Tracker instance.
func NewTracker(db *sql.DB) *Tracker {
	return &Tracker{db: db}
}
```

- [ ] **Step 3: Integrate schema into orchestrator initialization**

```go
// Modify: semcom_orchestrator/main.go
// Add the semcom_session.Schema to the openSharedDB call.
// Update the imports to include "semcom_session"
// Line ~61
	personalDB, err := openSharedDB(personalDBPath, personal.Schema, distill.Schema, semcom_session.Schema)
	if err != nil {
		log.Fatalf("open personal db: %v", err)
	}
	defer personalDB.Close()
```

- [ ] **Step 4: Commit**

```bash
git add semcom_session/schema.sql semcom_session/tracker.go semcom_orchestrator/main.go
git commit -m "feat(session): create semcom_session package and schema"
```

---

### Task 2: Implement `GetRetrievedIDs` with failsafe logging

**Files:**
- Modify: `semcom_session/tracker.go`
- Create: `semcom_session/tracker_test.go`

- [ ] **Step 1: Write the failing test**

```go
// Create: semcom_session/tracker_test.go
package semcom_session

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(Schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestGetRetrievedIDs(t *testing.T) {
	db := openTestDB(t)
	tracker := NewTracker(db)
	ctx := context.Background()

	// Insert test data
	if _, err := db.Exec(`INSERT INTO session_retrievals (session_id, memory_id) VALUES ('sess1', 10), ('sess1', 20)`); err != nil {
		t.Fatal(err)
	}

	bm := tracker.GetRetrievedIDs(ctx, "sess1")
	if bm.GetCardinality() != 2 {
		t.Errorf("expected 2 items, got %d", bm.GetCardinality())
	}
	if !bm.Contains(10) || !bm.Contains(20) {
		t.Errorf("bitmap missing expected IDs")
	}

	// Empty session
	bmEmpty := tracker.GetRetrievedIDs(ctx, "sess2")
	if !bmEmpty.IsEmpty() {
		t.Errorf("expected empty bitmap")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd semcom_session && go test -v`
Expected: FAIL with "tracker.GetRetrievedIDs undefined"

- [ ] **Step 3: Write minimal implementation**

```go
// Modify: semcom_session/tracker.go
// Add these imports: "context", "log/slog", "github.com/RoaringBitmap/roaring"

// GetRetrievedIDs loads the bitmap of previously retrieved IDs.
// Returns an empty bitmap if none exist or if a database error occurs.
func (t *Tracker) GetRetrievedIDs(ctx context.Context, sessionID string) *roaring.Bitmap {
	bm := roaring.New()
	if sessionID == "" {
		return bm
	}

	rows, err := t.db.QueryContext(ctx, `SELECT memory_id FROM session_retrievals WHERE session_id = ?`, sessionID)
	if err != nil {
		slog.Error("failed to query session retrievals", "session_id", sessionID, "error", err)
		return bm
	}
	defer rows.Close()

	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			slog.Error("failed to scan session retrieval row", "session_id", sessionID, "error", err)
			continue
		}
		bm.Add(uint32(id))
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating session retrievals", "session_id", sessionID, "error", err)
	}
	return bm
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd semcom_session && go test -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add semcom_session/tracker.go semcom_session/tracker_test.go
git commit -m "feat(session): implement GetRetrievedIDs with failsafe"
```

---

### Task 3: Implement `MarkRetrieved`

**Files:**
- Modify: `semcom_session/tracker.go`
- Modify: `semcom_session/tracker_test.go`

- [ ] **Step 1: Write the failing test**

```go
// Modify: semcom_session/tracker_test.go
// Add this test function:

func TestMarkRetrieved(t *testing.T) {
	db := openTestDB(t)
	tracker := NewTracker(db)
	ctx := context.Background()

	ids := []int32{5, 15, 25}
	if err := tracker.MarkRetrieved(ctx, "sess3", ids); err != nil {
		t.Fatalf("MarkRetrieved failed: %v", err)
	}

	bm := tracker.GetRetrievedIDs(ctx, "sess3")
	if bm.GetCardinality() != 3 {
		t.Errorf("expected 3 items, got %d", bm.GetCardinality())
	}
	for _, id := range ids {
		if !bm.Contains(uint32(id)) {
			t.Errorf("bitmap missing id %d", id)
		}
	}

	// Test idempotency (inserting same ID again shouldn't error due to OR IGNORE/REPLACE)
	if err := tracker.MarkRetrieved(ctx, "sess3", []int32{15}); err != nil {
		t.Fatalf("MarkRetrieved duplicate failed: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd semcom_session && go test -v`
Expected: FAIL with "tracker.MarkRetrieved undefined"

- [ ] **Step 3: Write minimal implementation**

```go
// Modify: semcom_session/tracker.go

// MarkRetrieved appends newly retrieved memory IDs to the session's record.
func (t *Tracker) MarkRetrieved(ctx context.Context, sessionID string, memoryIDs []int32) error {
	if sessionID == "" || len(memoryIDs) == 0 {
		return nil
	}

	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO session_retrievals (session_id, memory_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range memoryIDs {
		if _, err := stmt.ExecContext(ctx, sessionID, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd semcom_session && go test -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add semcom_session/tracker.go semcom_session/tracker_test.go
git commit -m "feat(session): implement MarkRetrieved"
```

---

### Task 4: Wire Tracker into the Orchestrator

**Files:**
- Modify: `semcom_orchestrator/orchestrator.go`
- Modify: `semcom_orchestrator/main.go`

- [ ] **Step 1: Add Tracker to Orchestrator struct**

```go
// Modify: semcom_orchestrator/orchestrator.go
// Add import "semcom_session" if not present
// Add to Orchestrator struct:

type Orchestrator struct {
	// ... existing fields ...
	retriever          *semcomretrieve.Retriever
	sessionTracker     *semcom_session.Tracker // NEW FIELD
	turnSeq            atomic.Int32
}
```

- [ ] **Step 2: Initialize Tracker in main.go**

```go
// Modify: semcom_orchestrator/main.go
// Right after setting up personalDB, dStore, etc.
// Add import "semcom_session"

	pRetriever, err := personal.NewPersonalRetriever(pStore)
	if err != nil {
		log.Fatalf("create personal retriever: %v", err)
	}

	sessionTracker := semcom_session.NewTracker(personalDB) // NEW LINE

	maxTurn, err := store.MaxTurnID(context.Background())
// ...
	orch := &Orchestrator{
		embed:             idx,
		personal:          pMatcher,
		personalStore:     pStore,
		personalRetriever: pRetriever,
		distillStore:      dStore,
		distillRetriever:  dRetriever,
		thresholds:        semindex.Thresholds{L2: 0.25, L1: 0.20, L0: 0.15},
		store:             store,
		retriever:         retriever,
		sessionTracker:    sessionTracker, // NEW LINE
	}
```

- [ ] **Step 3: Update `tieredRetrieve` to combine exclusion bitmaps**

```go
// Modify: semcom_orchestrator/retrieval.go

// Update the beginning of the tieredRetrieve function:
func (o *Orchestrator) tieredRetrieve(ctx context.Context, l0IDs []uint32, personalIDs []uint32, sessionID string) ([]RetrievalHit, error) {
	// Build exclusion bitmap for current session memories.
	var excludeIDs *roaring.Bitmap
	if sessionID != "" {
		ids, err := o.store.GetIDsBySessionID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			excludeIDs = roaring.New()
			for _, id := range ids {
				excludeIDs.Add(uint32(id))
			}
		}

		// Fetch historically retrieved IDs for this session
		if o.sessionTracker != nil {
			histBM := o.sessionTracker.GetRetrievedIDs(ctx, sessionID)
			if !histBM.IsEmpty() {
				if excludeIDs == nil {
					excludeIDs = histBM
				} else {
					excludeIDs.Or(histBM)
				}
			}
		}
	}

	// ... rest of tieredRetrieve remains the same until the end ...
```

- [ ] **Step 4: Update `tieredRetrieve` to record new hits**

```go
// Modify: semcom_orchestrator/retrieval.go

// At the very end of tieredRetrieve, just before the return:
	// ... (end of budget loop for rawCandidates) ...
		budget -= rawCost
	}

	if o.sessionTracker != nil && sessionID != "" && len(hits) > 0 {
		var newHitIDs []int32
		for _, h := range hits {
			newHitIDs = append(newHitIDs, h.ID)
		}
		if err := o.sessionTracker.MarkRetrieved(ctx, sessionID, newHitIDs); err != nil {
			// Log but don't fail the retrieval
			log.Printf("failed to mark retrieved ids for session %s: %v", sessionID, err)
		}
	}

	return hits, nil
}
```

- [ ] **Step 5: Run tests and commit**

Run: `cd semcom_orchestrator && go test -v`
Expected: PASS

```bash
git add semcom_orchestrator/orchestrator.go semcom_orchestrator/main.go semcom_orchestrator/retrieval.go
git commit -m "feat(orchestrator): integrate session tracker into retrieval loop"
```
