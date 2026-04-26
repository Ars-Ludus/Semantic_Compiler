package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"

	distill "semcom_distill"
)

// RunDistillationPass extracts knowledge snippets from conversation chunks.
func RunDistillationPass(ctx context.Context, o *Orchestrator, llm distill.LLMClient) error {
	log.Println("starting distillation pass...")

	lastIDStr, err := o.distillStore.GetMetadata("last_distilled_id")
	if err != nil {
		return fmt.Errorf("get metadata: %w", err)
	}
	lastID, _ := strconv.ParseInt(lastIDStr, 10, 64)

	maxID, err := o.store.MaxTurnID(ctx)
	if err != nil {
		return fmt.Errorf("get max id: %w", err)
	}

	for lastID+15 <= maxID {
		start := lastID - 3
		if start < 1 {
			start = 1
		}
		end := lastID + 15

		log.Printf("distilling chunk [%d, %d]...", start, end)

		memories, err := o.store.GetChunk(ctx, start, end)
		if err != nil {
			return fmt.Errorf("get chunk: %w", err)
		}

		if len(memories) == 0 {
			lastID = end
			continue
		}

		var sb strings.Builder
		for _, m := range memories {
			fmt.Fprintf(&sb, "[%s]: %s\n", m.Source, m.Raw)
		}

		resp, err := distill.Distill(ctx, llm, sb.String())
		if err != nil {
			log.Printf("distillation error for chunk [%d, %d]: %v", start, end, err)
			lastID = end
			continue
		}

		for _, snippet := range resp.Distillations {
			stats := o.embed.Query(snippet.Snippet, o.thresholds)

			words := strings.Fields(snippet.Topic)
			personalIDs, _ := o.personal.Match(words)

			semKeys := make([]uint32, len(stats.L0IDs))
			for i, id := range stats.L0IDs {
				semKeys[i] = uint32(id)
			}

			id, err := o.distillStore.InsertDistillation(&distill.Distillation{
				Topic:       snippet.Topic,
				Snippet:     snippet.Snippet,
				PersonalIDs: personalIDs,
				SemKeys:     semKeys,
			})
			if err != nil {
				log.Printf("insert distillation: %v", err)
				continue
			}
			log.Printf("stored distillation %d: topic=%q", id, snippet.Topic)
		}

		lastID = end
		if err := o.distillStore.SetMetadata("last_distilled_id", strconv.FormatInt(lastID, 10)); err != nil {
			return fmt.Errorf("set metadata: %w", err)
		}
	}

	log.Println("distillation pass complete.")
	return nil
}
