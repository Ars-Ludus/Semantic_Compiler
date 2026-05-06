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
	session "semcom_session"
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
	embed              Embedder
	personal           PersonalMatcher
	personalStore      *personal.Store
	personalRetriever  *personal.PersonalRetriever
	sessionTracker     *session.Tracker
	distillStore       *distill.Store
	distillRetriever   *distill.DistillationRetriever
	thresholds         semindex.Thresholds
	store              semanticstore.Store
	retriever          *semcomretrieve.Retriever
	turnSeq            atomic.Int32 // monotonically increasing; seeded from DB at startup
}

// IngestRequest is the input to the Ingest pipeline.
type IngestRequest struct {
	Text      string
	SessionID string
	Source    semanticstore.Source
}

// IngestResult is returned after a successful Ingest.
type IngestResult struct {
	MemoryID    int32
	L0Count     int
	QueryTokens int
	EmbedUs     int64
	StoreUs     int64
}

// Ingest embeds the text and matches personal tokens concurrently, then stores the result.
func (o *Orchestrator) Ingest(ctx context.Context, req IngestRequest) (IngestResult, error) {
	type embedOut struct {
		keys        []uint32
		queryTokens int
		us          int64
	}
	embedCh := make(chan embedOut, 1)
	personalCh := make(chan []uint32, 1)

	t0 := time.Now()
	go func() {
		stats := o.embed.Query(req.Text, o.thresholds)
		keys := make([]uint32, 0, len(stats.L0IDs))
		seen := make(map[uint32]struct{})
		for _, id := range stats.L0IDs {
			u := uint32(id)
			if _, ok := seen[u]; !ok {
				keys = append(keys, u)
				seen[u] = struct{}{}
			}
		}
		embedCh <- embedOut{keys, stats.QueryTokens, time.Since(t0).Microseconds()}
	}()

	go func() {
		if o.personal != nil {
			words := semindex.SplitWords(req.Text)
			ids, _ := o.personal.Match(words)
			personalCh <- ids
		} else {
			personalCh <- nil
		}
	}()

	emb := <-embedCh
	personalIDs := <-personalCh

	t1 := time.Now()
	memoryID, err := o.store.Insert(ctx, &semanticstore.Memory{
		TurnID: o.turnSeq.Add(1),
		Source: req.Source,
		Raw:    req.Text,
		SemKey: emb.keys,
	})
	if err != nil {
		return IngestResult{}, err
	}
	storeUs := time.Since(t1).Microseconds()

	if len(personalIDs) > 0 && o.personalRetriever != nil {
		if err := o.personalStore.LinkMemory(memoryID, personalIDs); err != nil {
			return IngestResult{}, err
		}
		for _, pid := range personalIDs {
			o.personalRetriever.AddLink(pid, memoryID)
		}
	}

	if o.retriever != nil {
		if err := o.retriever.Refresh(ctx); err != nil {
			return IngestResult{}, err
		}
	}

	return IngestResult{
		MemoryID:    memoryID,
		L0Count:     len(emb.keys),
		QueryTokens: emb.queryTokens,
		EmbedUs:     emb.us,
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
	Prompt    string
	SessionID string
	By        semanticstore.Source
	TopK      int
}

// ChatBenchmark holds per-step timing in microseconds.
type ChatBenchmark struct {
	EmbedUs    int64
	RetrieveUs int64
	StoreUs    int64
	TotalUs    int64
}

// ChatResult is returned by Chat.
type ChatResult struct {
	Context   []RetrievalHit
	MemoryID  int32
	Benchmark ChatBenchmark
}

// Chat runs global embedding and personal token matching concurrently, performs
// tiered retrieval, then stores the prompt. Retrieval runs before Insert to
// avoid self-contamination.
func (o *Orchestrator) Chat(ctx context.Context, req ChatRequest) (ChatResult, error) {
	type embedOut struct {
		keys []uint32
		us   int64
	}
	embedCh := make(chan embedOut, 1)
	personalCh := make(chan []uint32, 1)

	t0 := time.Now()
	go func() {
		stats := o.embed.Query(req.Prompt, o.thresholds)
		keys := make([]uint32, 0, len(stats.L0IDs))
		seen := make(map[uint32]struct{})
		for _, id := range stats.L0IDs {
			u := uint32(id)
			if _, ok := seen[u]; !ok {
				keys = append(keys, u)
				seen[u] = struct{}{}
			}
		}
		embedCh <- embedOut{keys, time.Since(t0).Microseconds()}
	}()

	go func() {
		if o.personal != nil {
			words := semindex.SplitWords(req.Prompt)
			ids, _ := o.personal.Match(words)
			personalCh <- ids
		} else {
			personalCh <- nil
		}
	}()

	emb := <-embedCh
	personalIDs := <-personalCh

	t1 := time.Now()
	ctxHits, err := o.tieredRetrieve(ctx, emb.keys, personalIDs, req.SessionID)
	if err != nil {
		return ChatResult{}, err
	}
	retrieveUs := time.Since(t1).Microseconds()

	t2 := time.Now()
	memoryID, err := o.store.Insert(ctx, &semanticstore.Memory{
		TurnID: o.turnSeq.Add(1),
		Source: req.By,
		Raw:    req.Prompt,
		SemKey: emb.keys,
	})
	if err != nil {
		return ChatResult{}, err
	}
	storeUs := time.Since(t2).Microseconds()

	if len(personalIDs) > 0 && o.personalRetriever != nil {
		if err := o.personalStore.LinkMemory(memoryID, personalIDs); err != nil {
			return ChatResult{}, err
		}
		for _, pid := range personalIDs {
			o.personalRetriever.AddLink(pid, memoryID)
		}
	}

	if o.retriever != nil {
		if err := o.retriever.Refresh(ctx); err != nil {
			return ChatResult{}, err
		}
	}

	return ChatResult{
		Context:  ctxHits,
		MemoryID: memoryID,
		Benchmark: ChatBenchmark{
			EmbedUs:    emb.us,
			RetrieveUs: retrieveUs,
			StoreUs:    storeUs,
			TotalUs:    emb.us + retrieveUs + storeUs,
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

	results := o.retriever.Query(globalKeys, k, nil)

	return RetrieveResult{
		Results:     results,
		QueryTokens: stats.QueryTokens,
		L0Count:     len(globalKeys),
		QueryUs:     queryUs,
	}, nil
}
