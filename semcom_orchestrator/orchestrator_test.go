package main

import (
	"context"
	"path/filepath"
	"testing"

	semanticstore "github.com/ars/semantic_store"
	semindex "semcom_embed"
)

// mockEmbedder implements Embedder with canned responses.
type mockEmbedder struct {
	l0IDs       []int32
	queryTokens int
}

func (m *mockEmbedder) Query(_ string, _ semindex.Thresholds) semindex.QueryStats {
	return semindex.QueryStats{
		L0IDs:       m.l0IDs,
		QueryTokens: m.queryTokens,
	}
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
		wantL0      int
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
			name: "model message",
			req: IngestRequest{
				Text:   "a response",
				Source: semanticstore.SourceModel,
			},
			l0IDs:       []int32{1},
			queryTokens: 1,
			wantL0:      1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orch := &Orchestrator{
				embed:      &mockEmbedder{l0IDs: tt.l0IDs, queryTokens: tt.queryTokens},
				thresholds: semindex.Thresholds{L2: 0.25, L1: 0.20, L0: 0.15},
				store:      openTestStore(t),
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
			if result.EmbedUs < 0 {
				t.Errorf("EmbedUs: got negative value %d", result.EmbedUs)
			}
		})
	}
}
