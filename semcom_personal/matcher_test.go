package personal

import (
	"os"
	"testing"
)

func TestMatcher(t *testing.T) {
	dbPath := "test_matcher.db"
	defer os.Remove(dbPath)

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	// Insert test data
	id1, _ := store.InsertToken("go", "lang")
	id2, _ := store.InsertToken("rust", "lang")
	_ = store.AddIgnore("the")
	_ = store.AddIgnore("a")

	matcher, err := NewMatcher(store)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	words := []string{"go", "is", "better", "than", "rust", "the", "end"}
	hits, unmapped := matcher.Match(words)

	expectedHits := []uint32{id1, id2}
	if len(hits) != len(expectedHits) {
		t.Errorf("expected %d hits, got %d", len(expectedHits), len(hits))
	}
	for i, h := range hits {
		if h != expectedHits[i] {
			t.Errorf("expected hit %d to be %d, got %d", i, expectedHits[i], h)
		}
	}

	expectedUnmapped := []string{"is", "better", "than", "end"}
	if len(unmapped) != len(expectedUnmapped) {
		t.Errorf("expected %d unmapped words, got %d", len(expectedUnmapped), len(unmapped))
	}
	for i, u := range unmapped {
		if u != expectedUnmapped[i] {
			t.Errorf("expected unmapped word %d to be %s, got %s", i, expectedUnmapped[i], u)
		}
	}
}
