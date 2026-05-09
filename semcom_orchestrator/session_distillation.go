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

// RunSessionDistillationPass distils each completed session into a set of
// knowledge snippets stored in personal.db. Sessions with no new memories
// since their last distillation are skipped.
func RunSessionDistillationPass(ctx context.Context, o *Orchestrator, llm distill.LLMClient, userLabel, modelLabel string, force bool) error {
	log.Println("starting session distillation pass...")

	sessionIDs, err := o.store.GetDistinctSessionIDs(ctx)
	if err != nil {
		return fmt.Errorf("get session ids: %w", err)
	}

	for _, sessionID := range sessionIDs {
		if err := distillOneSession(ctx, o, llm, sessionID, userLabel, modelLabel, force); err != nil {
			return err
		}
	}

	log.Println("session distillation pass complete.")
	return nil
}

// distillOneSession distils a single session. The full session transcript is
// sent to the LLM on every update. If the session was previously distilled,
// the new snippets are merged with the existing ones via ConsolidateSnippets
// so no previously captured knowledge is lost. The retriever is rebuilt after
// every replace to evict stale entries.
//
// If force is true, the session is re-distilled unconditionally.
func distillOneSession(ctx context.Context, o *Orchestrator, llm distill.LLMClient, sessionID, userLabel, modelLabel string, force bool) error {
	metaKey := "session_distilled:" + sessionID

	if force {
		if err := o.distillStore.DeleteDistillationsBySessionID(ctx, sessionID); err != nil {
			return fmt.Errorf("clear distillations for session %s: %w", sessionID, err)
		}
		if err := o.distillStore.SetMetadata(metaKey, ""); err != nil {
			return fmt.Errorf("clear metadata for session %s: %w", sessionID, err)
		}
	}

	// Watermark is the max memory ID included in the last distillation.
	// Parses to 0 on empty string or any non-numeric legacy value.
	watermarkStr, err := o.distillStore.GetMetadata(metaKey)
	if err != nil {
		return fmt.Errorf("get metadata for session %s: %w", sessionID, err)
	}
	watermark64, _ := strconv.ParseInt(watermarkStr, 10, 64)
	watermark := int32(watermark64)

	allMemories, err := o.store.GetMemoriesBySessionID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get memories for session %s: %w", sessionID, err)
	}
	if len(allMemories) == 0 {
		return nil
	}

	maxMemoryID := allMemories[len(allMemories)-1].ID
	if maxMemoryID <= watermark {
		log.Printf("session %s: no new memories since watermark %d, skipping", sessionID, watermark)
		return nil
	}

	// Send the full session to the LLM.
	var sb strings.Builder
	for _, m := range allMemories {
		fmt.Fprintf(&sb, "[%s]: %s\n", m.Source, m.Raw)
	}
	log.Printf("distilling session %s (%d memories)...", sessionID, len(allMemories))

	nextResp, err := distill.SessionDistill(ctx, llm, sb.String(), userLabel, modelLabel)
	if err != nil {
		log.Printf("session distillation error for %s: %v", sessionID, err)
		return nil // non-fatal: log and move on
	}

	var finalSnippets []distill.Snippet

	if watermark > 0 {
		// Merge new snippets with existing so no previously captured knowledge is lost.
		existing, err := o.distillStore.GetSnippetsBySessionID(ctx, sessionID)
		if err != nil {
			return fmt.Errorf("get existing snippets for session %s: %w", sessionID, err)
		}
		merged, err := distill.ConsolidateSnippets(ctx, llm, existing, nextResp.Snippets)
		if err != nil {
			// Non-fatal: leave existing distillations intact rather than risk losing knowledge.
			log.Printf("consolidation error for session %s: %v; skipping", sessionID, err)
			return nil
		}
		if err := o.distillStore.DeleteDistillationsBySessionID(ctx, sessionID); err != nil {
			return fmt.Errorf("delete old distillations for session %s: %w", sessionID, err)
		}
		finalSnippets = merged.Snippets
	} else {
		finalSnippets = nextResp.Snippets
	}

	for _, snippet := range finalSnippets {
		stats := o.embed.Query(snippet.Snippet, o.thresholds)
		semKeys := make([]uint32, len(stats.L0IDs))
		for i, id := range stats.L0IDs {
			semKeys[i] = uint32(id)
		}

		var personalIDs []uint32
		if snippet.Entity != "" {
			entityStats := o.embed.Query(snippet.Entity, o.thresholds)
			if len(entityStats.OOVWords) > 0 {
				token, err := o.personalStore.GetToken(snippet.Entity)
				if err == sql.ErrNoRows {
					id, err := o.personalStore.InsertToken(snippet.Entity, snippet.EntityType)
					if err != nil {
						log.Printf("insert token %q: %v", snippet.Entity, err)
					} else {
						o.personal.AddToken(snippet.Entity, id)
						personalIDs = []uint32{id}
						log.Printf("learned entity: %s type=%s (id=%d)", snippet.Entity, snippet.EntityType, id)
					}
				} else if err == nil {
					personalIDs = []uint32{token.ID}
				} else {
					log.Printf("lookup token %q: %v", snippet.Entity, err)
				}
			}
		}

		id, err := o.distillStore.InsertDistillation(&distill.Distillation{
			Topic:       snippet.Topic,
			Snippet:     snippet.Snippet,
			SessionID:   sessionID,
			Entity:      snippet.Entity,
			EntityType:  snippet.EntityType,
			PersonalIDs: personalIDs,
			SemKeys:     semKeys,
		})
		if err != nil {
			log.Printf("insert distillation: %v", err)
			continue
		}
		log.Printf("stored distillation %d: topic=%q", id, snippet.Topic)
	}

	// Link all session memories to personal tokens (INSERT OR IGNORE — idempotent).
	for _, m := range allMemories {
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

	// Rebuild the distillation retriever so deleted entries are evicted.
	if err := o.distillRetriever.Rebuild(o.distillStore); err != nil {
		log.Printf("rebuild distillation retriever for session %s: %v", sessionID, err)
	}

	if err := o.distillStore.SetMetadata(metaKey, strconv.FormatInt(int64(maxMemoryID), 10)); err != nil {
		return fmt.Errorf("set metadata for session %s: %w", sessionID, err)
	}
	log.Printf("session %s distillation complete (%d snippets, watermark=%d)", sessionID, len(finalSnippets), maxMemoryID)
	return nil
}
