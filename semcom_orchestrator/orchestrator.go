package main

import (
	"context"
	"sync/atomic"
	"time"

	semanticstore "github.com/ars/semantic_store"
	semcomretrieve "github.com/ars/semcom_retrieve"
	distill "semcom_distill"
	semindex "semcom_embed"
	personal "semcom_personal"
)

// Embedder wraps Index.Query for testability.
type Embedder interface {
	Query(text string, th semindex.Thresholds) semindex.QueryStats
}

// PersonalMatcher wraps personal.Matcher.Match for testability.
type PersonalMatcher interface {
	Match(words []string) ([]uint32, []string)
	AddToken(word string, id uint32)
}

// Orchestrator wires semcom_embed, semcom_store, and semcom_retrieve into pipelines.
type Orchestrator struct {
	embed         Embedder
	personal      PersonalMatcher
	personalStore *personal.Store
	distillStore  *distill.Store
	thresholds    semindex.Thresholds
	store         semanticstore.Store
	retriever     *semcomretrieve.Retriever
	turnSeq       atomic.Int64 // monotonically increasing; seeded from DB at startup
}

// IngestRequest is the input to the Ingest pipeline.
type IngestRequest struct {
	Text   string
	Source semanticstore.Source
}

// IngestResult is returned after a successful Ingest.
type IngestResult struct {
	MemoryID    int64
	L0Count     int
	QueryTokens int
	EmbedUs     int64
	StoreUs     int64
}

// Ingest embeds the text, then stores the result in semcom_store.
func (o *Orchestrator) Ingest(ctx context.Context, req IngestRequest) (IngestResult, error) {
	t0 := time.Now()
	stats := o.embed.Query(req.Text, o.thresholds)
	embedUs := time.Since(t0).Microseconds()

	globalKeys := make([]uint32, 0, len(stats.L0IDs))
	seen := make(map[uint32]struct{})
	for _, id := range stats.L0IDs {
		u := uint32(id)
		if _, ok := seen[u]; !ok {
			globalKeys = append(globalKeys, u)
			seen[u] = struct{}{}
		}
	}

	t1 := time.Now()
	// personal_tokens column is left NULL (represented by nil PersonalIDs in Memory struct)
	memoryID, err := o.store.Insert(ctx, &semanticstore.Memory{
		TurnID: o.turnSeq.Add(1),
		Source: req.Source,
		Raw:    req.Text,
		SemKey: globalKeys,
	})
	if err != nil {
		return IngestResult{}, err
	}
	storeUs := time.Since(t1).Microseconds()

	return IngestResult{
		MemoryID:    memoryID,
		L0Count:     len(globalKeys),
		QueryTokens: stats.QueryTokens,
		EmbedUs:     embedUs,
		StoreUs:     storeUs,
	}, nil
}

// RetrieveResult is returned by a Retrieve call.
type RetrieveResult struct {
	Results     []semcomretrieve.Result
	QueryTokens int
	L0Count     int
	QueryUs     int64
}

// ChatRequest is the input to the Chat pipeline.
type ChatRequest struct {
	Prompt string
	By     semanticstore.Source
	TopK   int
}

// ChatBenchmark holds per-step timing in microseconds.
type ChatBenchmark struct {
	EmbedUs    int64
	RetrieveUs int64
	StoreUs    int64
	TotalUs    int64
}

// MemoryHit is a single retrieved memory with its content and score.
type MemoryHit struct {
	MemoryID int64
	Score    int
	Raw      string
}

// ChatResult is returned by Chat.
type ChatResult struct {
	Results   []MemoryHit
	MemoryID  int64
	Benchmark ChatBenchmark
}

// Chat embeds the prompt once, retrieves relevant past memories, then stores the
// prompt. Retrieve runs before Insert to avoid self-contamination.
func (o *Orchestrator) Chat(ctx context.Context, req ChatRequest) (ChatResult, error) {
	t0 := time.Now()
	stats := o.embed.Query(req.Prompt, o.thresholds)
	embedUs := time.Since(t0).Microseconds()

	globalKeys := make([]uint32, 0, len(stats.L0IDs))
	seen := make(map[uint32]struct{})
	for _, id := range stats.L0IDs {
		u := uint32(id)
		if _, ok := seen[u]; !ok {
			globalKeys = append(globalKeys, u)
			seen[u] = struct{}{}
		}
	}

	t1 := time.Now()
	retrieved := o.retriever.Query(globalKeys, req.TopK)
	retrieveUs := time.Since(t1).Microseconds()

	hits := make([]MemoryHit, 0, len(retrieved))
	for _, r := range retrieved {
		raw, err := o.store.GetRaw(ctx, r.MemoryID)
		if err != nil {
			return ChatResult{}, err
		}
		hits = append(hits, MemoryHit{MemoryID: r.MemoryID, Score: r.Score, Raw: raw})
	}

	t2 := time.Now()
	memoryID, err := o.store.Insert(ctx, &semanticstore.Memory{
		TurnID: o.turnSeq.Add(1),
		Source: req.By,
		Raw:    req.Prompt,
		SemKey: globalKeys,
	})
	if err != nil {
		return ChatResult{}, err
	}
	storeUs := time.Since(t2).Microseconds()

	return ChatResult{
		Results:  hits,
		MemoryID: memoryID,
		Benchmark: ChatBenchmark{
			EmbedUs:    embedUs,
			RetrieveUs: retrieveUs,
			StoreUs:    storeUs,
			TotalUs:    embedUs + retrieveUs + storeUs,
		},
	}, nil
}

// Retrieve embeds the query text and returns ranked memory_id + score pairs.
func (o *Orchestrator) Retrieve(ctx context.Context, text string, k int) (RetrieveResult, error) {
	t0 := time.Now()
	stats := o.embed.Query(text, o.thresholds)
	queryUs := time.Since(t0).Microseconds()

	globalKeys := make([]uint32, 0, len(stats.L0IDs))
	seen := make(map[uint32]struct{})
	for _, id := range stats.L0IDs {
		u := uint32(id)
		if _, ok := seen[u]; !ok {
			globalKeys = append(globalKeys, u)
			seen[u] = struct{}{}
		}
	}

	results := o.retriever.Query(globalKeys, k)

	return RetrieveResult{
		Results:     results,
		QueryTokens: stats.QueryTokens,
		L0Count:     len(globalKeys),
		QueryUs:     queryUs,
	}, nil
}
