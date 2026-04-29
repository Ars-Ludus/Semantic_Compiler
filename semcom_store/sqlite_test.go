package semanticstore

import (
	"context"
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
		TurnID: 1,
		Source: SourceUser,
		Raw:    "hello world",
		SemKey: []uint32{3, 7, 42},
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

	got := append([]uint32{}, m.SemKey...)
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if !slices.Equal(got, []uint32{3, 7, 42}) {
		t.Errorf("SemKey: got %v, want %v", got, []uint32{3, 7, 42})
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

	byID := map[int32][]uint32{}
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
