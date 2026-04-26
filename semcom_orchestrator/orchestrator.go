package main

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	semanticstore "github.com/ars/semantic_store"
	semcomretrieve "github.com/ars/semcom_retrieve"
	semindex "semcom_embed"
)

// Embedder wraps Index.Query for testability.
type Embedder interface {
	Query(text string, th semindex.Thresholds) ([]int32, semindex.QueryStats)
}

// PersonalMatcher wraps personal.Matcher.Match for testability.
type PersonalMatcher interface {
	Match(words []string) ([]uint32, []string)
}

// Orchestrator wires semcom_embed, semcom_store, and semcom_retrieve into pipelines.
type Orchestrator struct {
	embed      Embedder
	personal   PersonalMatcher
	thresholds semindex.Thresholds
	store      semanticstore.Store
	retriever  *semcomretrieve.Retriever
	turnSeq    atomic.Int64  // monotonically increasing; seeded from DB at startup
	unmappedCh chan []string // for background discovery
}

// IngestRequest is the input to the Ingest pipeline.
type IngestRequest struct {
	Text      string
	Source    semanticstore.Source
	SummaryID *int64
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
	words := semindex.SplitWords(req.Text)
	var stats semindex.QueryStats
	var personalIDs []uint32
	var unmapped []string

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, stats = o.embed.Query(req.Text, o.thresholds)
	}()
	go func() {
		defer wg.Done()
		if o.personal != nil {
			personalIDs, unmapped = o.personal.Match(words)
		}
	}()
	wg.Wait()
	embedUs := time.Since(t0).Microseconds()

	// Handle unmapped words: only those that were ALSO not in the global index.
	if len(unmapped) > 0 && o.unmappedCh != nil {
		oovSet := make(map[string]struct{})
		for _, w := range stats.OOVWords {
			oovSet[w] = struct{}{}
		}
		var filtered []string
		for _, w := range unmapped {
			if _, ok := oovSet[w]; ok {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			select {
			case o.unmappedCh <- filtered:
			default:
			}
		}
	}

	semKeys := make([]uint32, 0, len(stats.L0IDs)+len(personalIDs))
	for _, id := range stats.L0IDs {
		semKeys = append(semKeys, uint32(id))
	}
	semKeys = append(semKeys, personalIDs...)

	t1 := time.Now()
	memoryID, err := o.store.Insert(ctx, &semanticstore.Memory{
		TurnID:    o.turnSeq.Add(1),
		SummaryID: req.SummaryID,
		Source:    req.Source,
		Raw:       req.Text,
		SemKey:    semKeys,
	})
	if err != nil {
		return IngestResult{}, err
	}
	storeUs := time.Since(t1).Microseconds()

	return IngestResult{
		MemoryID:    memoryID,
		L0Count:     len(semKeys),
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
	words := semindex.SplitWords(req.Prompt)
	var stats semindex.QueryStats
	var personalIDs []uint32
	var unmapped []string

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, stats = o.embed.Query(req.Prompt, o.thresholds)
	}()
	go func() {
		defer wg.Done()
		if o.personal != nil {
			personalIDs, unmapped = o.personal.Match(words)
		}
	}()
	wg.Wait()
	embedUs := time.Since(t0).Microseconds()

	// Handle unmapped words
	if len(unmapped) > 0 && o.unmappedCh != nil {
		oovSet := make(map[string]struct{})
		for _, w := range stats.OOVWords {
			oovSet[w] = struct{}{}
		}
		var filtered []string
		for _, w := range unmapped {
			if _, ok := oovSet[w]; ok {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			select {
			case o.unmappedCh <- filtered:
			default:
			}
		}
	}

	semKeys := make([]uint32, 0, len(stats.L0IDs)+len(personalIDs))
	for _, id := range stats.L0IDs {
		semKeys = append(semKeys, uint32(id))
	}
	semKeys = append(semKeys, personalIDs...)

	t1 := time.Now()
	retrieved := o.retriever.Query(semKeys, req.TopK)
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
		SemKey: semKeys,
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
	words := semindex.SplitWords(text)
	var stats semindex.QueryStats
	var personalIDs []uint32
	var unmapped []string

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, stats = o.embed.Query(text, o.thresholds)
	}()
	go func() {
		defer wg.Done()
		if o.personal != nil {
			personalIDs, unmapped = o.personal.Match(words)
		}
	}()
	wg.Wait()
	queryUs := time.Since(t0).Microseconds()

	// Handle unmapped words: only those that were ALSO not in the global index.
	if len(unmapped) > 0 && o.unmappedCh != nil {
		oovSet := make(map[string]struct{})
		for _, w := range stats.OOVWords {
			oovSet[w] = struct{}{}
		}
		var filtered []string
		for _, w := range unmapped {
			if _, ok := oovSet[w]; ok {
				filtered = append(filtered, w)
			}
		}
		if len(filtered) > 0 {
			select {
			case o.unmappedCh <- filtered:
			default:
			}
		}
	}

	queryL0IDs := make([]uint32, 0, len(stats.L0IDs)+len(personalIDs))
	for _, id := range stats.L0IDs {
		queryL0IDs = append(queryL0IDs, uint32(id))
	}
	queryL0IDs = append(queryL0IDs, personalIDs...)

	results := o.retriever.Query(queryL0IDs, k)

	return RetrieveResult{
		Results:     results,
		QueryTokens: stats.QueryTokens,
		L0Count:     len(queryL0IDs),
		QueryUs:     queryUs,
	}, nil
}
