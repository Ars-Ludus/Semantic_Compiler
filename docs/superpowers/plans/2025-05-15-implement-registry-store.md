# Implement Registry Store Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the Registry Store for the personal module to manage personal tokens and ignored words.

**Architecture:** Use SQLite with `modernc.org/sqlite` driver. The `Store` struct will wrap a `*sql.DB`.

**Tech Stack:** Go, SQLite (`modernc.org/sqlite`).

---

### Task 1: Define Token Struct

**Files:**
- Modify: `semcom_personal/personal.go`

- [ ] **Step 1: Add Token struct to personal.go**

```go
package personal

import (
	_ "embed"
)

// Schema is the SQL schema for the personal module.
//go:embed schema.sql
var Schema string

// Token represents a personal token.
type Token struct {
	ID   uint32
	Word string
	Type string
}
```

- [ ] **Step 2: Commit**

```bash
git add semcom_personal/personal.go
git commit -m "feat(personal): define Token struct"
```

### Task 2: Implement Store and Basic Methods

**Files:**
- Create: `semcom_personal/store.go`

- [ ] **Step 1: Write Store implementation**

```go
package personal

import (
	"database/sql"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(Schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) InsertToken(word, t string) (uint32, error) {
	res, err := s.db.Exec(`INSERT INTO personal_tokens (word, type) VALUES (?, ?)`, word, t)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint32(id), nil
}

func (s *Store) GetToken(word string) (*Token, error) {
	row := s.db.QueryRow(`SELECT id, word, type FROM personal_tokens WHERE word = ?`, word)
	var t Token
	if err := row.Scan(&t.ID, &t.Word, &t.Type); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) AddIgnore(word string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO personal_ignore (word) VALUES (?)`, word)
	return err
}

func (s *Store) IsIgnored(word string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM personal_ignore WHERE word = ?)`, word).Scan(&exists)
	return exists, err
}
```

- [ ] **Step 2: Commit**

```bash
git add semcom_personal/store.go
git commit -m "feat(personal): implement registry store methods"
```

### Task 3: Write and Run Tests

**Files:**
- Create: `semcom_personal/store_test.go`

- [ ] **Step 1: Write the tests**

```go
package personal

import (
	"testing"
)

func openTestStore(t *testing.T) *Store {
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("failed to open test store: %v", err)
	}
	return s
}

func TestStore(t *testing.T) {
	s := openTestStore(t)
	defer s.Close()

	t.Run("TokenOperations", func(t *testing.T) {
		id, err := s.InsertToken("Alice", "PERSON")
		if err != nil {
			t.Fatalf("InsertToken failed: %v", id)
		}

		token, err := s.GetToken("Alice")
		if err != nil {
			t.Fatalf("GetToken failed: %v", err)
		}

		if token.Word != "Alice" || token.ID != id || token.Type != "PERSON" {
			t.Errorf("token mismatch: got %+v, want ID=%d Word=Alice Type=PERSON", token, id)
		}
	})

	t.Run("IgnoreOperations", func(t *testing.T) {
		word := "the"
		if err := s.AddIgnore(word); err != nil {
			t.Fatalf("AddIgnore failed: %v", err)
		}

		ignored, err := s.IsIgnored(word)
		if err != nil {
			t.Fatalf("IsIgnored failed: %v", err)
		}
		if !ignored {
			t.Error("expected word to be ignored")
		}

		ignored, err = s.IsIgnored("unknown")
		if err != nil {
			t.Fatalf("IsIgnored failed: %v", err)
		}
		if ignored {
			t.Error("expected word to NOT be ignored")
		}
	})
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./semcom_personal/... -v`
Expected: PASS

- [ ] **Step 3: Verify and Commit final changes**

```bash
git add semcom_personal/store_test.go
git commit -m "test(personal): add registry store tests"
```
