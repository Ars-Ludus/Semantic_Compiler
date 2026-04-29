package main

import (
	"context"
	"sort"

	distill "semcom_distill"
)

const (
	retrievalBudget   = 100
	distilledCost     = 10
	rawCost           = 20
	personalWeight    = 5
	minRelevanceScore = 3
)

// HitType distinguishes distilled snippets from raw memories.
type HitType string

const (
	HitDistilled HitType = "distilled"
	HitRaw       HitType = "raw"
)

// RetrievalHit is a single budget-allocated context item.
type RetrievalHit struct {
	Type    HitType
	ID      int32
	Score   int
	Topic   string // distilled only
	Content string // snippet for distilled, raw message for raw
}

// tieredRetrieve scores all candidates using global L0 IDs and personal token IDs,
// then fills the 100-point budget with distilled snippets first (10pts each)
// followed by raw memories (20pts each). Personal token matches add 5× to scores.
func (o *Orchestrator) tieredRetrieve(ctx context.Context, l0IDs []uint32, personalIDs []uint32) ([]RetrievalHit, error) {
	// Run distillation lookup concurrently with global memory retrieval.
	distillCh := make(chan []distill.DistillationResult, 1)
	go func() {
		distillCh <- o.distillRetriever.Query(l0IDs, personalIDs, personalWeight)
	}()

	rawScores := make(map[int32]int)
	for _, r := range o.retriever.Query(l0IDs, 0) {
		rawScores[r.MemoryID] += r.Score
	}
	if o.personalRetriever != nil && len(personalIDs) > 0 {
		for memID, count := range o.personalRetriever.MemoryTokenCounts(personalIDs) {
			rawScores[memID] += count * personalWeight
		}
	}

	distCandidates := <-distillCh

	// Drop distilled candidates below the relevance threshold.
	for i, dc := range distCandidates {
		if dc.Score < minRelevanceScore {
			distCandidates = distCandidates[:i]
			break
		}
	}

	// Sort raw candidates descending by score, dropping below threshold.
	type scoredMem struct {
		id    int32
		score int
	}
	rawCandidates := make([]scoredMem, 0, len(rawScores))
	for id, score := range rawScores {
		if score >= minRelevanceScore {
			rawCandidates = append(rawCandidates, scoredMem{id, score})
		}
	}
	sort.Slice(rawCandidates, func(i, j int) bool {
		return rawCandidates[i].score > rawCandidates[j].score
	})

	// Select distilled IDs that fit within budget, batch-fetch their text.
	budget := retrievalBudget
	distScoreByID := make(map[int32]int, len(distCandidates))
	var distIDs []int32
	for _, dc := range distCandidates {
		if budget < distilledCost {
			break
		}
		distIDs = append(distIDs, dc.ID)
		distScoreByID[dc.ID] = dc.Score
		budget -= distilledCost
	}

	var hits []RetrievalHit

	if len(distIDs) > 0 {
		distillations, err := o.distillStore.GetDistillationsByIDs(ctx, distIDs)
		if err != nil {
			return nil, err
		}
		for _, d := range distillations {
			hits = append(hits, RetrievalHit{
				Type:    HitDistilled,
				ID:      d.ID,
				Score:   distScoreByID[d.ID],
				Topic:   d.Topic,
				Content: d.Snippet,
			})
		}
		sort.Slice(hits, func(i, j int) bool {
			return hits[i].Score > hits[j].Score
		})
	}

	for _, rc := range rawCandidates {
		if budget < rawCost {
			break
		}
		raw, err := o.store.GetRaw(ctx, rc.id)
		if err != nil {
			continue
		}
		hits = append(hits, RetrievalHit{
			Type:    HitRaw,
			ID:      rc.id,
			Score:   rc.score,
			Content: raw,
		})
		budget -= rawCost
	}

	return hits, nil
}
