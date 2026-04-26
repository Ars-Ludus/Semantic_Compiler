package distill

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec(Schema); err != nil {
		db.Close()
		t.Fatalf("apply schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewStore(db)
}

func TestInsertDistillation(t *testing.T) {
	s := openTestStore(t)

	id, err := s.InsertDistillation(&Distillation{
		Topic:       "Go modules",
		Snippet:     "User prefers small focused modules with clear boundaries.",
		PersonalIDs: []uint32{1, 2},
		SemKeys:     []uint32{100, 200},
	})
	if err != nil {
		t.Fatalf("InsertDistillation: %v", err)
	}
	if id <= 0 {
		t.Errorf("expected positive ID, got %d", id)
	}
}

func TestMetadata(t *testing.T) {
	s := openTestStore(t)

	val, err := s.GetMetadata("last_distilled_id")
	if err != nil {
		t.Fatalf("GetMetadata (missing): %v", err)
	}
	if val != "" {
		t.Errorf("expected empty string for missing key, got %q", val)
	}

	if err := s.SetMetadata("last_distilled_id", "42"); err != nil {
		t.Fatalf("SetMetadata: %v", err)
	}

	val, err = s.GetMetadata("last_distilled_id")
	if err != nil {
		t.Fatalf("GetMetadata (after set): %v", err)
	}
	if val != "42" {
		t.Errorf("expected %q, got %q", "42", val)
	}

	// Upsert
	if err := s.SetMetadata("last_distilled_id", "99"); err != nil {
		t.Fatalf("SetMetadata (upsert): %v", err)
	}
	val, _ = s.GetMetadata("last_distilled_id")
	if val != "99" {
		t.Errorf("expected %q after upsert, got %q", "99", val)
	}
}
