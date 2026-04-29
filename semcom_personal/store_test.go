package personal

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

func TestStore(t *testing.T) {
	s := openTestStore(t)

	id, err := s.InsertToken("Alice", "PERSON")
	if err != nil {
		t.Fatalf("InsertToken: %v", err)
	}
	if id == 0 {
		t.Errorf("expected non-zero ID")
	}

	token, err := s.GetToken("Alice")
	if err != nil {
		t.Fatalf("GetToken: %v", err)
	}
	if token.Word != "alice" {
		t.Errorf("expected lowercase word alice, got %s", token.Word)
	}
	if token.Type != "PERSON" {
		t.Errorf("expected type PERSON, got %s", token.Type)
	}
}
