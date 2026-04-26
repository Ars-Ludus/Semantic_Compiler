package main

import (
	"context"
	"log"

	"semcom_personal"
)

// RunDiscoveryPass scans for all unprocessed memories and enriches them with personal tokens.
func RunDiscoveryPass(ctx context.Context, o *Orchestrator, llm personal.LLMClient) error {
	log.Println("starting discovery pass...")

	memories, err := o.store.UnprocessedMemories(ctx)
	if err != nil {
		return err
	}
	if len(memories) == 0 {
		log.Println("no unprocessed memories found.")
		return nil
	}

	log.Printf("processing %d memories", len(memories))

	for _, m := range memories {
		// LLM Topic Extraction
		resp, err := personal.Discover(ctx, llm, m.Raw)
		if err != nil {
			log.Printf("discovery error for mem %d: %v", m.ID, err)
			continue
		}

		var personalIDs []uint32
		for _, topic := range resp.Topics {
			// Global Tokenizer Filter
			// We call Query on the topic. If it returns OOV words, it means the global system
			// doesn't fully understand this topic, so we treat it as a personal token.
			stats := o.embed.Query(topic, o.thresholds)
			
			if len(stats.OOVWords) == 0 {
				log.Printf("discovery: skipping known topic '%s'", topic)
				continue
			}

			// It's a personal token. Check if we already have it.
			id := uint32(0)
			token, err := o.personalStore.GetToken(topic)
			if err == nil {
				id = token.ID
			} else {
				// Insert new personal token. Use "TOPIC" as default type.
				id, err = o.personalStore.InsertToken(topic, "TOPIC")
				if err != nil {
					log.Printf("error inserting token %s: %v", topic, err)
					continue
				}
				o.personal.AddToken(topic, id)
				log.Printf("discovery: learned new entity: %s (id=%d)", topic, id)
			}
			personalIDs = append(personalIDs, id)
		}

		// Update main store (marks as discovered and saves IDs)
		if err := o.store.UpdateMemoryPersonalTokens(ctx, m.ID, personalIDs); err != nil {
			log.Printf("error updating main store for mem %d: %v", m.ID, err)
			continue
		}
		if err := o.store.MarkMemoryDiscovered(ctx, m.ID); err != nil {
			log.Printf("error marking mem %d discovered: %v", m.ID, err)
			continue
		}

		// Link to personal reverse index
		if len(personalIDs) > 0 {
			if err := o.personalStore.LinkMemory(m.ID, personalIDs); err != nil {
				log.Printf("error linking personal index for mem %d: %v", m.ID, err)
			}
		}
		
		log.Printf("processed mem %d: found %d personal topics", m.ID, len(personalIDs))
	}

	log.Println("discovery pass complete.")
	return nil
}
