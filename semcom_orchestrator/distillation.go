package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	"semcom_personal"
)

// RunDistillationPass extracts knowledge snippets from conversation chunks.
func RunDistillationPass(ctx context.Context, o *Orchestrator, llm personal.LLMClient) error {
	log.Println("starting distillation pass...")

	// 1. Get last processed ID
	lastIDStr, err := o.personalStore.GetMetadata("last_distilled_id")
	if err != nil {
		return fmt.Errorf("failed to get metadata: %w", err)
	}
	lastID, _ := strconv.ParseInt(lastIDStr, 10, 64)

	// 2. Check current max ID
	maxID, err := o.store.MaxTurnID(ctx) // This is turn_seq, but we need row ID. 
	// Actually, Store.Insert returns LastInsertId which is the Row ID. 
	// MaxTurnID in Store implementation actually returns MAX(id) in memories.
	if err != nil {
		return fmt.Errorf("failed to get max id: %w", err)
	}

	// 3. Process in chunks of 15 with 3-turn overlap
	// Window: [lastID-3, lastID+15]
	// If lastID=0, start from 1.
	for lastID+15 <= maxID {
		start := lastID - 3
		if start < 1 {
			start = 1
		}
		end := lastID + 15

		log.Printf("distilling chunk [%d, %d]...", start, end)

		memories, err := o.store.GetChunk(ctx, start, end)
		if err != nil {
			return fmt.Errorf("failed to get chunk: %w", err)
		}

		if len(memories) == 0 {
			lastID = end
			continue
		}

		// Assemble context string
		var sb strings.Builder
		for _, m := range memories {
			fmt.Fprintf(&sb, "[%s]: %s\n", m.Source, m.Raw)
		}

		// LLM Distill
		resp, err := personal.Distill(ctx, llm, sb.String())
		if err != nil {
			log.Printf("distillation error for chunk [%d, %d]: %v", start, end, err)
			// Move on to avoid getting stuck? Or stop?
			// Let's increment lastID anyway to satisfy the condition, but maybe we should retry.
			lastID = end
			continue
		}

		for _, snippet := range resp.Distillations {
			// 1. Embed snippet to get semkeys
			stats := o.embed.Query(snippet.Snippet, o.thresholds)
			
			// 2. Match topic to personal tokens (tagging only)
			// We split the topic into words and match them.
			words := strings.Fields(snippet.Topic)
			personalIDs, _ := o.personal.Match(words)

			// 3. Store distillation
			d := &personal.Distillation{
				Topic:       snippet.Topic,
				Snippet:     snippet.Snippet,
				PersonalIDs: personalIDs,
				SemKeys:     make([]uint32, len(stats.L0IDs)),
			}
			for i, id := range stats.L0IDs {
				d.SemKeys[i] = uint32(id)
			}

			id, err := o.personalStore.InsertDistillation(d)
			if err != nil {
				log.Printf("failed to insert distillation: %v", err)
				continue
			}
			log.Printf("stored distillation %d: topic='%s'", id, d.Topic)
		}

		lastID = end
		if err := o.personalStore.SetMetadata("last_distilled_id", strconv.FormatInt(lastID, 10)); err != nil {
			return fmt.Errorf("failed to update metadata: %w", err)
		}
	}

	log.Println("distillation pass complete.")
	return nil
}
