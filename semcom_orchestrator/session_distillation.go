package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"

	distill "semcom_distill"
	semindex "semcom_embed"
)

// RunSessionDistillationPass distils each completed session into a set of
// knowledge snippets stored in personal.db. Each snippet is embedded and
// indexed for retrieval; snippets with OOV entities also register personal tokens.
//
// Sessions already processed are skipped unless force is true, in which case
// their existing distillations are deleted and re-generated from scratch.
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

// distillOneSession distils a single session. If force is true, existing
// distillations for the session are deleted and re-generated.
func distillOneSession(ctx context.Context, o *Orchestrator, llm distill.LLMClient, sessionID, userLabel, modelLabel string, force bool) error {
	metaKey := "session_distilled:" + sessionID

	if !force {
		val, err := o.distillStore.GetMetadata(metaKey)
		if err != nil {
			return fmt.Errorf("get metadata for session %s: %w", sessionID, err)
		}
		if val != "" {
			log.Printf("skipping session %s (already distilled)", sessionID)
			return nil
		}
	} else {
		if err := o.distillStore.DeleteDistillationsBySessionID(ctx, sessionID); err != nil {
			return fmt.Errorf("clear distillations for session %s: %w", sessionID, err)
		}
		if err := o.distillStore.SetMetadata(metaKey, ""); err != nil {
			return fmt.Errorf("clear metadata for session %s: %w", sessionID, err)
		}
	}

	memories, err := o.store.GetMemoriesBySessionID(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("get memories for session %s: %w", sessionID, err)
	}
	if len(memories) == 0 {
		return nil
	}

	var sb strings.Builder
	for _, m := range memories {
		fmt.Fprintf(&sb, "[%s]: %s\n", m.Source, m.Raw)
	}

	log.Printf("distilling session %s (%d memories)...", sessionID, len(memories))

	resp, err := distill.SessionDistill(ctx, llm, sb.String(), userLabel, modelLabel)
	if err != nil {
		log.Printf("session distillation error for %s: %v", sessionID, err)
		return nil // non-fatal: log and move on
	}

	for _, snippet := range resp.Snippets {
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

	// Link session memories to all personal tokens now known to the matcher.
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

	if err := o.distillStore.SetMetadata(metaKey, "1"); err != nil {
		return fmt.Errorf("set metadata for session %s: %w", sessionID, err)
	}
	log.Printf("session %s distillation complete (%d snippets)", sessionID, len(resp.Snippets))
	return nil
}
