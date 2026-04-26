package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"semcom_personal"
	semanticstore "github.com/ars/semantic_store"
	semindex "semcom_embed"
)

func TestDiscoveryWorker(t *testing.T) {
	tempDir := t.TempDir()
	personalDBPath := filepath.Join(tempDir, "personal.db")

	pStore, err := personal.Open(personalDBPath)
	if err != nil {
		t.Fatalf("personal.Open: %v", err)
	}
	defer pStore.Close()

	pMatcher, err := personal.NewMatcher(pStore)
	if err != nil {
		t.Fatalf("personal.NewMatcher: %v", err)
	}

	unmappedCh := make(chan []string, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start worker with MockLLM
	go startDiscoveryWorker(ctx, pStore, pMatcher, unmappedCh, &MockLLM{})

	// 1. Send "Alice" to unmapped channel
	unmappedCh <- []string{"Alice"}

	// 2. Wait for it to be processed
	// DiscoveryWorker has a 2s ticker, so we wait a bit more
	time.Sleep(3 * time.Second)

	// 3. Check if Alice was learned
	hits, unmapped := pMatcher.Match([]string{"Alice"})
	if len(hits) != 1 {
		t.Errorf("expected Alice to be matched, got 0 hits. Unmapped: %v", unmapped)
	}
	if len(unmapped) != 0 {
		t.Errorf("expected Alice to be removed from unmapped, got %v", unmapped)
	}

	// 4. Send "bob" (should be ignored by MockLLM)
	unmappedCh <- []string{"bob"}
	time.Sleep(3 * time.Second)

	hits, unmapped = pMatcher.Match([]string{"bob"})
	if len(hits) != 0 {
		t.Errorf("expected bob to NOT be matched, got %d hits", len(hits))
	}
	if len(unmapped) != 0 {
		t.Errorf("expected bob to be ignored (so not unmapped), got %v", unmapped)
	}
}

func TestOrchestratorDiscovery(t *testing.T) {
	tempDir := t.TempDir()
	personalDBPath := filepath.Join(tempDir, "personal.db")

	pStore, err := personal.Open(personalDBPath)
	if err != nil {
		t.Fatalf("personal.Open: %v", err)
	}
	defer pStore.Close()

	pMatcher, err := personal.NewMatcher(pStore)
	if err != nil {
		t.Fatalf("personal.NewMatcher: %v", err)
	}

	unmappedCh := make(chan []string, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	orch := &Orchestrator{
		embed: &mockEmbedder{
			l0IDs:    []int32{1, 2},
			oovWords: []string{"Alice"},
		},
		personal:   pMatcher,
		thresholds: semindex.Thresholds{L0: 0.1},
		store:      openTestStore(t),
		unmappedCh: unmappedCh,
	}

	// Start worker
	go startDiscoveryWorker(ctx, pStore, pMatcher, unmappedCh, &MockLLM{})

	// 1. Ingest message with "Alice"
	_, err = orch.Ingest(ctx, IngestRequest{
		Text:   "Alice is here",
		Source: semanticstore.SourceUser,
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// 2. Wait for discovery
	time.Sleep(3 * time.Second)

	// 3. Check if Alice is now matched
	hits, unmapped := pMatcher.Match([]string{"Alice"})
	if len(hits) != 1 {
		t.Fatalf("expected Alice to be matched, got 0 hits. Unmapped: %v", unmapped)
	}

	// 4. Ingest again, should now include Alice in L0Count
	result, err := orch.Ingest(ctx, IngestRequest{
		Text:   "Alice is here again",
		Source: semanticstore.SourceUser,
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	// 1 global OOV + 2 global L0 + 1 personal = 3? 
	// Wait, mockEmbedder.Query returns l0IDs []int32{1, 2}.
	// Alice is OOV, so it's NOT in stats.L0IDs.
	// After discovery, pMatcher.Match("Alice") returns 1 hit.
	// embedAndMatch combines them.
	// In first Ingest: global L0 IDs: {1, 2}, personal: {}, unmapped: {"Alice"} -> semKeys: {1, 2} (len 2)
	// In second Ingest: global L0 IDs: {1, 2}, personal: {ID_ALICE}, unmapped: {} -> semKeys: {1, 2, ID_ALICE} (len 3)
	
	if result.L0Count != 3 {
		t.Errorf("L0Count: got %d, want 3", result.L0Count)
	}
}
