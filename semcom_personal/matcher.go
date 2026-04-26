package personal

import (
	"strings"
	"sync"
)

// Matcher provides fast, thread-safe lookup of token IDs and ignore rules.
// It maintains an in-memory cache of tokens and ignored words from the store.
type Matcher struct {
	tokens map[string]uint32
	ignore map[string]struct{}
	mu     sync.RWMutex
}

// NewMatcher creates a new Matcher by loading all tokens and ignore rules from the store.
func NewMatcher(s *Store) (*Matcher, error) {
	tokens, err := s.GetAllTokens()
	if err != nil {
		return nil, err
	}

	ignore, err := s.GetAllIgnore()
	if err != nil {
		return nil, err
	}

	return &Matcher{
		tokens: tokens,
		ignore: ignore,
	}, nil
}

// Match takes a list of words and returns the IDs of matching tokens
// and the list of words that were not found in the token registry or ignore list.
func (m *Matcher) Match(words []string) (hits []uint32, unmapped []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	hits = make([]uint32, 0, len(words))
	unmapped = make([]string, 0, len(words))

	for _, word := range words {
		word = strings.ToLower(word)
		if _, ok := m.ignore[word]; ok {
			continue
		}
		if id, ok := m.tokens[word]; ok {
			hits = append(hits, id)
		} else {
			unmapped = append(unmapped, word)
		}
	}
	return hits, unmapped
}

// AddToken incrementally adds a token to the matcher's memory.
func (m *Matcher) AddToken(word string, id uint32) {
	word = strings.ToLower(word)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tokens[word] = id
}

// AddIgnore incrementally adds a word to the ignore list in memory.
func (m *Matcher) AddIgnore(word string) {
	word = strings.ToLower(word)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ignore[word] = struct{}{}
}
