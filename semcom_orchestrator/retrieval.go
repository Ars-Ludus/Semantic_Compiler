package main

import (
	"context"
	"log"

	"github.com/RoaringBitmap/roaring"
	distill "semcom_distill"
)

const (
	topKDistilled     = 5
	wikiTopKPerToken  = 5
	personalWeight    = 5
	minRelevanceScore = 6
)

// HitType distinguishes scored distillation snippets from wiki (entity-direct) lookups.
type HitType string

const (
	HitDistilled HitType = "distilled"
	HitWiki      HitType = "wiki"
)

// RetrievalHit is a single context item returned to the caller.
type RetrievalHit struct {
	Type    HitType
	ID      int32
	Score   int
	Topic   string // always set
	Content string // snippet text
}

// tieredRetrieve returns up to topKDistilled scored distillations plus wiki context for each
// matched personal token. Previously returned distillation IDs (within the same session) are
// excluded. Raw memory retrieval is not performed.
func (o *Orchestrator) tieredRetrieve(ctx context.Context, l0IDs []uint32, personalIDs []uint32, sessionID string) ([]RetrievalHit, error) {
	// Build exclusion bitmap from distillations already returned in this session.
	var excludeIDs *roaring.Bitmap
	if sessionID != "" && o.sessionTracker != nil {
		bm := o.sessionTracker.GetRetrievedDistillationIDs(ctx, sessionID)
		if !bm.IsEmpty() {
			excludeIDs = bm
		}
	}

	// Score distillations against the query's L0 IDs and personal tokens.
	candidates := o.distillRetriever.Query(l0IDs, personalIDs, personalWeight, excludeIDs)

	// Take the top topKDistilled results that meet the minimum score threshold.
	scoredIDSet := make(map[int32]struct{})
	var scoredDistillations []distill.DistillationResult
	for _, c := range candidates {
		if c.Score < minRelevanceScore {
			break
		}
		if len(scoredDistillations) >= topKDistilled {
			break
		}
		scoredDistillations = append(scoredDistillations, c)
		scoredIDSet[c.ID] = struct{}{}
	}

	// Wiki lookup: for each personal token, fetch the most recent associated distillations.
	useExclusion := excludeIDs != nil && !excludeIDs.IsEmpty()
	var wikiIDs []int32
	for _, pid := range personalIDs {
		count := 0
		for _, did := range o.distillRetriever.GetByPersonalID(pid) {
			if count >= wikiTopKPerToken {
				break
			}
			if _, inScored := scoredIDSet[did]; inScored {
				continue
			}
			if useExclusion && excludeIDs.Contains(uint32(did)) {
				continue
			}
			wikiIDs = append(wikiIDs, did)
			scoredIDSet[did] = struct{}{} // prevent the same ID from appearing twice across different personal tokens
			count++
		}
	}

	// Collect all IDs to fetch.
	allIDs := make([]int32, 0, len(scoredDistillations)+len(wikiIDs))
	for _, d := range scoredDistillations {
		allIDs = append(allIDs, d.ID)
	}
	allIDs = append(allIDs, wikiIDs...)

	if len(allIDs) == 0 {
		return nil, nil
	}

	distillations, err := o.distillStore.GetDistillationsByIDs(ctx, allIDs)
	if err != nil {
		return nil, err
	}

	// Index fetched distillations by ID for ordered assembly.
	byID := make(map[int32]*distill.Distillation, len(distillations))
	for _, d := range distillations {
		byID[d.ID] = d
	}

	hits := make([]RetrievalHit, 0, len(allIDs))

	// Scored distillations first, in score-descending order.
	for _, sd := range scoredDistillations {
		d, ok := byID[sd.ID]
		if !ok {
			continue
		}
		hits = append(hits, RetrievalHit{
			Type:    HitDistilled,
			ID:      d.ID,
			Score:   sd.Score,
			Topic:   d.Topic,
			Content: d.Snippet,
		})
	}

	// Wiki results appended after scored results.
	for _, id := range wikiIDs {
		d, ok := byID[id]
		if !ok {
			continue
		}
		hits = append(hits, RetrievalHit{
			Type:    HitWiki,
			ID:      d.ID,
			Score:   0,
			Topic:   d.Topic,
			Content: d.Snippet,
		})
	}

	if o.sessionTracker != nil && sessionID != "" {
		returnedIDs := make([]int32, len(hits))
		for i, h := range hits {
			returnedIDs[i] = h.ID
		}
		if err := o.sessionTracker.MarkDistillationRetrieved(ctx, sessionID, returnedIDs); err != nil {
			log.Printf("failed to mark distillation retrieved ids for session %s: %v", sessionID, err)
		}
	}

	return hits, nil
}
