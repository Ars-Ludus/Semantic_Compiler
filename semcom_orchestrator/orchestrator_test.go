package main

import (
	"context"
	"path/filepath"
	"testing"

	semanticstore "github.com/ars/semantic_store"
	semcomretrieve "github.com/ars/semcom_retrieve"
	semindex "semcom_embed"
)

// mockEmbedder implements Embedder with canned responses.
type mockEmbedder struct {
	l0IDs       []int32
	queryTokens int
	oovWords    []string
}

func (m *mockEmbedder) Query(_ string, _ semindex.Thresholds) ([]int32, semindex.QueryStats) {
	return m.l0IDs, semindex.QueryStats{
		L0IDs:       m.l0IDs,
		QueryTokens: m.queryTokens,
		OOVWords:    m.oovWords,
	}
}

// mockPersonalMatcher implements PersonalMatcher.
type mockPersonalMatcher struct {
	personalIDs []uint32
	unmapped    []string
}

func (m *mockPersonalMatcher) Match(words []string) ([]uint32, []string) {
	return m.personalIDs, m.unmapped
}

func openTestStore(t *testing.T) semanticstore.Store {
	t.Helper()
	s, err := semanticstore.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestIngest(t *testing.T) {
	tests := []struct {
		name        string
		req         IngestRequest
		l0IDs       []int32
		queryTokens int
		oovWords    []string
		personalIDs []uint32
		unmapped    []string
		wantL0      int
		wantUnmapped []string
	}{
		{
			name: "user message",
			req: IngestRequest{
				Text:   "hello world",
				Source: semanticstore.SourceUser,
			},
			l0IDs:       []int32{3, 7, 42},
			queryTokens: 2,
			wantL0:      3,
		},
		{
			name: "with personal tokens",
			req: IngestRequest{
				Text:   "special project",
				Source: semanticstore.SourceUser,
			},
			l0IDs:       []int32{100},
			queryTokens: 2,
			personalIDs: []uint32{0xFF000001, 0xFF000002},
			wantL0:      3, // 1 global + 2 personal
		},
		{
			name: "unmapped words filtering",
			req: IngestRequest{
				Text:   "unknown word",
				Source: semanticstore.SourceUser,
			},
			l0IDs:       []int32{5},
			queryTokens: 2,
			oovWords:    []string{"unknown"},
			unmapped:    []string{"unknown", "known"},
			wantL0:      1,
			wantUnmapped: []string{"unknown"}, // "known" is in global index (not OOV)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			unmappedCh := make(chan []string, 1)
			orch := &Orchestrator{
				embed: &mockEmbedder{
					l0IDs:       tt.l0IDs,
					queryTokens: tt.queryTokens,
					oovWords:    tt.oovWords,
				},
				personal: &mockPersonalMatcher{
					personalIDs: tt.personalIDs,
					unmapped:    tt.unmapped,
				},
				thresholds: semindex.Thresholds{L2: 0.25, L1: 0.20, L0: 0.15},
				store:      openTestStore(t),
				unmappedCh: unmappedCh,
			}

			result, err := orch.Ingest(context.Background(), tt.req)
			if err != nil {
				t.Fatalf("Ingest: %v", err)
			}
			if result.MemoryID <= 0 {
				t.Errorf("expected positive MemoryID, got %d", result.MemoryID)
			}
			if result.L0Count != tt.wantL0 {
				t.Errorf("L0Count: got %d, want %d", result.L0Count, tt.wantL0)
			}

			if len(tt.wantUnmapped) > 0 {
				select {
				case got := <-unmappedCh:
					if len(got) != len(tt.wantUnmapped) {
						t.Errorf("unmapped count: got %v, want %v", got, tt.wantUnmapped)
					}
				default:
					t.Errorf("expected unmapped words on channel, got none")
				}
			}
		})
	}
}

func TestRetrieve(t *testing.T) {
	orch := &Orchestrator{
		embed: &mockEmbedder{
			l0IDs:       []int32{42},
			queryTokens: 1,
		},
		personal: &mockPersonalMatcher{
			personalIDs: []uint32{0xFF000001},
		},
		thresholds: semindex.Thresholds{L2: 0.25, L1: 0.20, L0: 0.15},
		retriever:  &semcomretrieve.Retriever{}, // Mocking retriever is harder due to its structure, but we can check L0Count
	}

	result, err := orch.Retrieve(context.Background(), "test query", 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}

	if result.L0Count != 2 {
		t.Errorf("L0Count: got %d, want 2 (1 global + 1 personal)", result.L0Count)
	}
	if result.QueryTokens != 1 {
		t.Errorf("QueryTokens: got %d, want 1", result.QueryTokens)
	}
}
