package semanticstore

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"sort"
	"testing"
)

func openTemp(t *testing.T) Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestInsertGet(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	id, err := s.Insert(ctx, &Memory{
		TurnID:      1,
		Source:      SourceUser,
		Raw:         "hello world",
		SemKey:      []uint32{3, 7, 42},
		PersonalIDs: []uint32{101, 102},
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	m, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if m.TurnID != 1 || m.Source != SourceUser || m.Raw != "hello world" {
		t.Errorf("field mismatch: %+v", m)
	}
	if m.SummaryID != nil {
		t.Errorf("expected nil SummaryID, got %v", m.SummaryID)
	}

	got := append([]uint32{}, m.SemKey...)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	want := []uint32{3, 7, 42}
	if !slices.Equal(got, want) {
		t.Errorf("SemKey: got %v, want %v", got, want)
	}

	if !slices.Equal(m.PersonalIDs, []uint32{101, 102}) {
		t.Errorf("PersonalIDs: got %v, want %v", m.PersonalIDs, []uint32{101, 102})
	}
}

func TestAllSemKeys(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	id1, _ := s.Insert(ctx, &Memory{TurnID: 1, Source: SourceUser, Raw: "a", SemKey: []uint32{1, 2}})
	id2, _ := s.Insert(ctx, &Memory{TurnID: 2, Source: SourceModel, Raw: "b", SemKey: []uint32{2, 3}})

	rows, err := s.AllSemKeys(ctx)
	if err != nil {
		t.Fatalf("AllSemKeys: %v", err)
	}

	// collect per memory_id
	byID := map[int64][]uint32{}
	for _, r := range rows {
		byID[r.MemoryID] = append(byID[r.MemoryID], r.Value)
	}

	sort.Slice(byID[id1], func(i, j int) bool { return byID[id1][i] < byID[id1][j] })
	sort.Slice(byID[id2], func(i, j int) bool { return byID[id2][i] < byID[id2][j] })

	if !slices.Equal(byID[id1], []uint32{1, 2}) {
		t.Errorf("id1 semkeys: got %v", byID[id1])
	}
	if !slices.Equal(byID[id2], []uint32{2, 3}) {
		t.Errorf("id2 semkeys: got %v", byID[id2])
	}
}

func TestSemKeysSince(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	id1, _ := s.Insert(ctx, &Memory{TurnID: 1, Source: SourceUser, Raw: "a", SemKey: []uint32{10}})
	_, _ = s.Insert(ctx, &Memory{TurnID: 2, Source: SourceModel, Raw: "b", SemKey: []uint32{20}})

	rows, err := s.SemKeysSince(ctx, id1)
	if err != nil {
		t.Fatalf("SemKeysSince: %v", err)
	}

	for _, r := range rows {
		if r.MemoryID <= id1 {
			t.Errorf("expected memory_id > %d, got %d", id1, r.MemoryID)
		}
	}
	if len(rows) != 1 || rows[0].Value != 20 {
		t.Errorf("unexpected rows: %v", rows)
	}
}

func TestReopenPreservesData(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	s1, _ := Open(filepath.Join(dir, "mem.db"))
	id, _ := s1.Insert(ctx, &Memory{TurnID: 5, Source: SourceModel, Raw: "persist", SemKey: []uint32{99}})
	s1.Close()

	s2, _ := Open(filepath.Join(dir, "mem.db"))
	defer s2.Close()
	m, err := s2.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after reopen: %v", err)
	}
	if m.Raw != "persist" || len(m.SemKey) != 1 || m.SemKey[0] != 99 {
		t.Errorf("data mismatch after reopen: %+v", m)
	}
}

func TestMigration(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "old.db")

	// Create a DB with the OLD schema (no personal_tokens)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open old db: %v", err)
	}
	_, err = db.Exec(`
		CREATE TABLE memories (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			turn_id     INTEGER NOT NULL,
			summary_id  INTEGER,
			source      TEXT    NOT NULL CHECK(source IN ('user', 'model')),
			raw_message TEXT    NOT NULL,
			created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		);
	`)
	if err != nil {
		db.Close()
		t.Fatalf("failed to create old schema: %v", err)
	}
	db.Close()

	// Now open it with our Store - this should trigger migration
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed on old db: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	// Try to insert a record with PersonalIDs - if migration failed, this will fail
	// because the column is missing.
	id, err := s.Insert(ctx, &Memory{
		TurnID:      1,
		Source:      SourceUser,
		Raw:         "migration test",
		PersonalIDs: []uint32{456},
	})
	if err != nil {
		t.Fatalf("Insert failed after migration: %v", err)
	}

	m, err := s.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed after migration: %v", err)
	}

	if !slices.Equal(m.PersonalIDs, []uint32{456}) {
		t.Errorf("PersonalIDs mismatch: got %v, want %v", m.PersonalIDs, []uint32{456})
	}
}

func TestEdgeCases(t *testing.T) {
	s := openTemp(t)
	ctx := context.Background()

	cases := []struct {
		name string
		ids  []uint32
	}{
		{"nil", nil},
		{"empty", []uint32{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, err := s.Insert(ctx, &Memory{
				TurnID:      1,
				Source:      SourceUser,
				Raw:         "edge case test",
				PersonalIDs: tc.ids,
			})
			if err != nil {
				t.Fatalf("Insert: %v", err)
			}

			m, err := s.Get(ctx, id)
			if err != nil {
				t.Fatalf("Get: %v", err)
			}

			if len(m.PersonalIDs) != 0 {
				t.Errorf("expected 0 PersonalIDs, got %v", m.PersonalIDs)
			}
		})
	}
}

func TestJSONHandling(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "json.db")
	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	id, err := s.Insert(ctx, &Memory{
		TurnID:      1,
		Source:      SourceUser,
		Raw:         "json test",
		PersonalIDs: nil,
	})
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}

	// Now check the DB directly
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	defer db.Close()

	var personalTokens string
	err = db.QueryRow("SELECT personal_tokens FROM memories WHERE id = ?", id).Scan(&personalTokens)
	if err != nil {
		t.Fatalf("QueryRow: %v", err)
	}

	if personalTokens != "" {
		t.Errorf("expected empty string in DB for nil PersonalIDs, got %q", personalTokens)
	}
}
