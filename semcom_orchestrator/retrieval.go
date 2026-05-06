package main

import (
	"context"
	"log"
	"sort"

	"github.com/RoaringBitmap/roaring"
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
// It ignores memories belonging to the current sessionID and those already retrieved.
func (o *Orchestrator) tieredRetrieve(ctx context.Context, l0IDs []uint32, personalIDs []uint32, sessionID string) ([]RetrievalHit, error) {
	// Build exclusion bitmap for current session memories.
	var excludeIDs *roaring.Bitmap
	if sessionID != "" {
		ids, err := o.store.GetIDsBySessionID(ctx, sessionID)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			excludeIDs = roaring.New()
			for _, id := range ids {
				excludeIDs.Add(uint32(id))
			}
		}

		// Fetch historically retrieved IDs for this session
		if o.sessionTracker != nil {
			histBM := o.sessionTracker.GetRetrievedIDs(ctx, sessionID)
			if !histBM.IsEmpty() {
				if excludeIDs == nil {
					excludeIDs = histBM
				} else {
					excludeIDs.Or(histBM)
				}
			}
		}
	}

	// Run distillation lookup concurrently with global memory retrieval.
	distillCh := make(chan []distill.DistillationResult, 1)
	go func() {
		distillCh <- o.distillRetriever.Query(l0IDs, personalIDs, personalWeight)
	}()

	rawScores := make(map[int32]int)
	for _, r := range o.retriever.Query(l0IDs, 0, excludeIDs) {
		rawScores[r.MemoryID] += r.Score
	}
	if o.personalRetriever != nil && len(personalIDs) > 0 {
		for memID, count := range o.personalRetriever.MemoryTokenCounts(personalIDs, excludeIDs) {
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

	if o.sessionTracker != nil && sessionID != "" && len(hits) > 0 {
		var newHitIDs []int32
		for _, h := range hits {
			newHitIDs = append(newHitIDs, h.ID)
		}
		if err := o.sessionTracker.MarkRetrieved(ctx, sessionID, newHitIDs); err != nil {
			// Log but don't fail the retrieval
			log.Printf("failed to mark retrieved ids for session %s: %v", sessionID, err)
		}
	}

	return hits, nil
}
