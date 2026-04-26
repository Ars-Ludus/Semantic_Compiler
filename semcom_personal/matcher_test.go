package personal

import (
	"os"
	"sync"
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
	id1, err := store.InsertToken("go", "lang")
	if err != nil {
		t.Fatalf("failed to insert token: %v", err)
	}
	id2, err := store.InsertToken("rust", "lang")
	if err != nil {
		t.Fatalf("failed to insert token: %v", err)
	}
	err = store.AddIgnore("the")
	if err != nil {
		t.Fatalf("failed to add ignore: %v", err)
	}
	err = store.AddIgnore("a")
	if err != nil {
		t.Fatalf("failed to add ignore: %v", err)
	}

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

func TestMatcherConcurrency(t *testing.T) {
	dbPath := "test_matcher_concurrency.db"
	defer os.Remove(dbPath)

	store, err := Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open store: %v", err)
	}
	defer store.Close()

	matcher, err := NewMatcher(store)
	if err != nil {
		t.Fatalf("failed to create matcher: %v", err)
	}

	const goroutines = 100
	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent readers
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				matcher.Match([]string{"go", "rust", "unknown"})
			}
		}()
	}

	// Concurrent writers
	for i := 0; i < goroutines; i++ {
		go func(i int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				matcher.AddToken("word", uint32(i*iterations+j))
				matcher.AddIgnore("ignore")
			}
		}(i)
	}

	wg.Wait()
}
