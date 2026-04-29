package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strconv"
	"strings"

	distill "semcom_distill"
	semindex "semcom_embed"
)

// RunDistillationPass extracts knowledge snippets and personal entities from conversation chunks.
func RunDistillationPass(ctx context.Context, o *Orchestrator, llm distill.LLMClient) error {
	log.Println("starting distillation pass...")

	lastIDStr, err := o.distillStore.GetMetadata("last_distilled_id")
	if err != nil {
		return fmt.Errorf("get metadata: %w", err)
	}
	lastID64, _ := strconv.ParseInt(lastIDStr, 10, 64)
	lastID := int32(lastID64)

	maxID, err := o.store.MaxID(ctx)
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

		// Process distillations.
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
			o.distillRetriever.Add(id, semKeys, personalIDs)
			log.Printf("stored distillation %d: topic=%q", id, snippet.Topic)
		}

		// Process entities: upsert personal tokens for chunk-level named entities.
		for _, entity := range resp.Entities {
			// OOV filter: skip entities fully covered by the global vocabulary —
			// their semkeys already handle retrieval without a personal token.
			stats := o.embed.Query(entity.Text, o.thresholds)
			if len(stats.OOVWords) == 0 {
				continue
			}

			token, err := o.personalStore.GetToken(entity.Text)
			if err == nil {
				_ = token // already registered
				continue
			}
			if err != sql.ErrNoRows {
				log.Printf("lookup token %q: %v", entity.Text, err)
				continue
			}

			id, err := o.personalStore.InsertToken(entity.Text, entity.Type)
			if err != nil {
				log.Printf("insert token %q: %v", entity.Text, err)
				continue
			}
			o.personal.AddToken(entity.Text, id)
			log.Printf("learned entity: %s type=%s (id=%d)", entity.Text, entity.Type, id)
		}

		// Link each memory in the chunk to any matching personal tokens.
		for _, m := range memories {
			words := semindex.SplitWords(m.Raw)
			personalIDs, _ := o.personal.Match(words)
			if len(personalIDs) == 0 {
				continue
			}
			if err := o.personalStore.LinkMemory(m.ID, personalIDs); err != nil {
				log.Printf("link memory %d: %v", m.ID, err)
				continue
			}
			for _, pid := range personalIDs {
				o.personalRetriever.AddLink(pid, m.ID)
			}
		}

		lastID = end
		if err := o.distillStore.SetMetadata("last_distilled_id", strconv.FormatInt(int64(lastID), 10)); err != nil {
			return fmt.Errorf("set metadata: %w", err)
		}
	}

	log.Println("distillation pass complete.")
	return nil
}
