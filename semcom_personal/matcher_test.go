package personal

import (
	"sync"
	"testing"
)

func TestMatcher(t *testing.T) {
	m := &Matcher{
		tokens: map[string]uint32{
			"alice": 1,
			"bob":   2,
		},
	}

	hits, unmapped := m.Match([]string{"Alice", "Bob", "Charlie"})

	if len(hits) != 2 {
		t.Errorf("expected 2 hits, got %d", len(hits))
	}
	if hits[0] != 1 || hits[1] != 2 {
		t.Errorf("hits mismatch: %v", hits)
	}

	if len(unmapped) != 1 {
		t.Errorf("expected 1 unmapped word, got %d", len(unmapped))
	}
	if unmapped[0] != "charlie" {
		t.Errorf("expected charlie to be unmapped, got %s", unmapped[0])
	}
}

func TestMatcherConcurrency(t *testing.T) {
	m := &Matcher{
		tokens: make(map[string]uint32),
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			m.AddToken("word", uint32(n))
			m.Match([]string{"word"})
		}(i)
	}
	wg.Wait()
}
