package personal

import (
	"strings"
	"sync"
)

// Matcher provides fast, thread-safe lookup of token IDs.
// It maintains an in-memory cache of tokens from the store.
type Matcher struct {
	tokens map[string]uint32
	mu     sync.RWMutex
}

// NewMatcher creates a new Matcher by loading all tokens from the store.
func NewMatcher(s *Store) (*Matcher, error) {
	tokens, err := s.GetAllTokens()
	if err != nil {
		return nil, err
	}

	return &Matcher{
		tokens: tokens,
	}, nil
}

// Match performs a longest-match forward scan over words against the token
// registry. Multi-word phrases registered as single tokens are preferred over
// their constituent words. Returns matched token IDs and unmatched words.
func (m *Matcher) Match(words []string) (hits []uint32, unmapped []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	lower := make([]string, len(words))
	for i, w := range words {
		lower[i] = strings.ToLower(w)
	}

	for i := 0; i < len(lower); {
		matched := false
		for end := len(lower); end > i; end-- {
			phrase := strings.Join(lower[i:end], " ")
			if id, ok := m.tokens[phrase]; ok {
				hits = append(hits, id)
				i = end
				matched = true
				break
			}
		}
		if !matched {
			unmapped = append(unmapped, lower[i])
			i++
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
