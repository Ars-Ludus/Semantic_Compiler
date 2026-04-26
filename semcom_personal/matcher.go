package personal

import (
	"sync"
)

type Matcher struct {
	tokens map[string]uint32
	ignore map[string]struct{}
	mu     sync.RWMutex
}

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

func (m *Matcher) Match(words []string) (hits []uint32, unmapped []string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, word := range words {
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
