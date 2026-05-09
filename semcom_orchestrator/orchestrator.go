package main

import (
	"context"
	"log"
	"sync"
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

	// Auto-distillation: when set, the orchestrator spawns a background distillation
	// of the previous session whenever it detects a session ID change.
	llmClient       distill.LLMClient
	userLabel       string
	modelLabel      string
	activeSessionID atomic.Value // stores string; tracks the most recently seen session ID

	// shutdownCtx is the server's lifecycle context. Background goroutines use it
	// so they stop when the server shuts down.
	shutdownCtx context.Context
	bgWg        sync.WaitGroup
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
	o.maybeTriggerDistill(req.SessionID)

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

	go func() { personalCh <- o.matchPersonal(req.Text) }()

	emb := <-embedCh
	personalIDs := <-personalCh

	t1 := time.Now()
	memoryID, err := o.store.Insert(ctx, &semanticstore.Memory{
		TurnID:    o.turnSeq.Add(1),
		Source:    req.Source,
		Raw:       req.Text,
		SessionID: req.SessionID,
		SemKey:    emb.keys,
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
	o.maybeTriggerDistill(req.SessionID)

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

	go func() { personalCh <- o.matchPersonal(req.Prompt) }()

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
		TurnID:    o.turnSeq.Add(1),
		Source:    req.By,
		Raw:       req.Prompt,
		SessionID: req.SessionID,
		SemKey:    emb.keys,
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

// maybeTriggerDistill checks whether the session ID has changed since the last call.
// If it has, and an LLM client is configured, it spawns a background goroutine to
// force-re-distill the session that just ended.
func (o *Orchestrator) maybeTriggerDistill(sessionID string) {
	if o.llmClient == nil || sessionID == "" {
		return
	}
	prev, _ := o.activeSessionID.Swap(sessionID).(string)
	if prev == "" || prev == sessionID {
		return
	}
	ctx := o.shutdownCtx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	log.Printf("session change detected (%s → %s); scheduling auto-distill of %s", prev, sessionID, prev)
	o.bgWg.Add(1)
	go func() {
		defer o.bgWg.Done()
		if err := distillOneSession(ctx, o, o.llmClient, prev, o.userLabel, o.modelLabel, false); err != nil && ctx.Err() == nil {
			log.Printf("auto-distill session %s: %v", prev, err)
		}
	}()
}

func (o *Orchestrator) matchPersonal(text string) []uint32 {
	if o.personal == nil {
		return nil
	}
	ids, _ := o.personal.Match(semindex.SplitWords(text))
	return ids
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
