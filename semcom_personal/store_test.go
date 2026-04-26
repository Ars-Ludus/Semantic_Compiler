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
			t.Fatalf("InsertToken: %v", err)
		}
		if id < 1000000 {
			t.Errorf("expected ID >= 1000000, got %d", id)
		}

		token, err := s.GetToken("Alice")
		if err != nil {
			t.Fatalf("GetToken: %v", err)
		}
		if token.Word != "alice" {
			t.Errorf("expected word 'alice', got %q", token.Word)
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
