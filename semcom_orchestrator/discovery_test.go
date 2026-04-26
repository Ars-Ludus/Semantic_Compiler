package main

import (
	"context"
	"database/sql"
	"testing"

	"semcom_personal"
	semanticstore "github.com/ars/semantic_store"
	semindex "semcom_embed"

	_ "modernc.org/sqlite"
)

type MockLLM struct{}

func (m *MockLLM) GenerateJSON(ctx context.Context, prompt string, target interface{}) error {
	resp := target.(*personal.DiscoveryResponse)
	resp.Topics = []string{"Alice", "Providertron", "project"}
	return nil
}

func openTestPersonalDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if _, err := db.Exec(personal.Schema); err != nil {
		db.Close()
		t.Fatalf("apply schema: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDiscoveryPass(t *testing.T) {
	pDB := openTestPersonalDB(t)
	pStore := personal.NewStore(pDB)

	pMatcher, err := personal.NewMatcher(pStore)
	if err != nil {
		t.Fatalf("personal.NewMatcher: %v", err)
	}

	store := openTestStore(t)

	orch := &Orchestrator{
		embed: &mockEmbedder{
			l0IDs: []int32{1, 2},
			oovMap: map[string]bool{
				"Alice":        true,
				"Providertron": true,
				// "project" absent — Query returns no OOVWords, so it's skipped
			},
		},
		personal:      pMatcher,
		personalStore: pStore,
		thresholds:    semindex.Thresholds{L0: 0.1},
		store:         store,
	}

	ctx := context.Background()

	_, err = orch.Ingest(ctx, IngestRequest{
		Text:   "Working on Providertron with Alice",
		Source: semanticstore.SourceUser,
	})
	if err != nil {
		t.Fatalf("Ingest: %v", err)
	}

	unprocessed, _ := store.UnprocessedMemories(ctx)
	if len(unprocessed) != 1 {
		t.Fatalf("expected 1 unprocessed memory, got %d", len(unprocessed))
	}

	if err := RunDiscoveryPass(ctx, orch, &MockLLM{}); err != nil {
		t.Fatalf("RunDiscoveryPass: %v", err)
	}

	unprocessed, _ = store.UnprocessedMemories(ctx)
	if len(unprocessed) != 0 {
		t.Errorf("expected 0 unprocessed memories after pass, got %d", len(unprocessed))
	}

	hits, _ := pMatcher.Match([]string{"Alice", "Providertron"})
	if len(hits) != 2 {
		t.Errorf("expected 2 personal tokens matched, got %d", len(hits))
	}

	hits, _ = pMatcher.Match([]string{"project"})
	if len(hits) != 0 {
		t.Errorf("expected 'project' skipped (known to global index)")
	}

	m, _ := store.Get(ctx, 1)
	if !m.Discovered {
		t.Errorf("expected memory marked discovered")
	}
	if len(m.PersonalIDs) != 2 {
		t.Errorf("expected 2 personal IDs in memory, got %v", m.PersonalIDs)
	}
}
