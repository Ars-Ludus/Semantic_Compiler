package personal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStore(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer s.Close()

	t.Run("TokenOperations", func(t *testing.T) {
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
	})

	t.Run("LinkMemory", func(t *testing.T) {
		err := s.LinkMemory(123, []uint32{1, 2})
		if err != nil {
			t.Errorf("LinkMemory: %v", err)
		}
	})
}

func TestSchema(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "schema_test.db")

	s, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	s.Close()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("expected DB file to exist")
	}
}
