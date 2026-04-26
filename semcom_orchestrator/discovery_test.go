package main

import (
	"context"
	"path/filepath"
	"testing"

	"semcom_personal"
	semanticstore "github.com/ars/semantic_store"
	semindex "semcom_embed"
)

type MockLLM struct{}

func (m *MockLLM) GenerateJSON(ctx context.Context, prompt string, target interface{}) error {
	// A simple mock that extracts "Alice", "Providertron", and "project" as topics
	resp := target.(*personal.DiscoveryResponse)
	resp.Topics = []string{"Alice", "Providertron", "project"}
	return nil
}

func TestDiscoveryPass(t *testing.T) {
	tempDir := t.TempDir()
	personalDBPath := filepath.Join(tempDir, "personal.db")
	mainDBPath := filepath.Join(tempDir, "main.db")

	pStore, err := personal.Open(personalDBPath)
	if err != nil {
		t.Fatalf("personal.Open: %v", err)
	}
	defer pStore.Close()

	pMatcher, err := personal.NewMatcher(pStore)
	if err != nil {
		t.Fatalf("personal.NewMatcher: %v", err)
	}

	store, err := semanticstore.Open(mainDBPath)
	if err != nil {
		t.Fatalf("semanticstore.Open: %v", err)
	}
	defer store.Close()

	orch := &Orchestrator{
		embed: &mockEmbedder{
			l0IDs: []int32{1, 2},
			oovMap: map[string]bool{
				"Alice":        true,
				"Providertron": true,
				// "project" is NOT here, so Query("project") will return OOVWords=nil
			},
		},
		personal:      pMatcher,
		personalStore: pStore,
		thresholds:    semindex.Thresholds{L0: 0.1},
		store:         store,
	}

	ctx := context.Background()

	// 1. Ingest a message
	_, err = orch.Ingest(ctx, IngestRequest{
		Text:   "Working on Providertron with Alice",
		Source: semanticstore.SourceUser,
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// 2. Verify unprocessed state
	unprocessed, _ := store.UnprocessedMemories(ctx)
	if len(unprocessed) != 1 {
		t.Fatalf("expected 1 unprocessed memory, got %d", len(unprocessed))
	}

	// 3. Run Discovery Pass
	if err := RunDiscoveryPass(ctx, orch, &MockLLM{}); err != nil {
		t.Fatalf("RunDiscoveryPass: %v", err)
	}

	// 4. Verify enriched state
	unprocessed, _ = store.UnprocessedMemories(ctx)
	if len(unprocessed) != 0 {
		t.Errorf("expected 0 unprocessed memories, got %d", len(unprocessed))
	}

	// Check if Alice and Providertron were learned
	// "project" should be skipped because Query("project") returns no OOV words
	hits, _ := pMatcher.Match([]string{"Alice", "Providertron"})
	if len(hits) != 2 {
		t.Errorf("expected 2 personal tokens to be matched, got %d", len(hits))
	}

	hits, _ = pMatcher.Match([]string{"project"})
	if len(hits) != 0 {
		t.Errorf("expected 'project' to be skipped (already known to global)")
	}

	// Check main store record
	m, _ := store.Get(ctx, 1)
	if !m.Discovered {
		t.Errorf("expected memory to be marked discovered")
	}
	if len(m.PersonalIDs) != 2 {
		t.Errorf("expected 2 personal IDs in memory record, got %v", m.PersonalIDs)
	}
}
