package main

import (
	"context"
	"log"
	"strings"
	"time"

	"semcom_personal"
)

type MockLLM struct{}

func (m *MockLLM) GenerateJSON(ctx context.Context, prompt string, target interface{}) error {
	// A simple mock that "discovers" words containing "Alice"
	resp := target.(*personal.DiscoveryResponse)
	
	// Extract words from prompt to see what we are evaluating
	// Prompt contains "Words to evaluate: word1, word2, ..."
	lines := strings.Split(prompt, "\n")
	var wordsLine string
	for _, line := range lines {
		if strings.HasPrefix(line, "Words to evaluate: ") {
			wordsLine = strings.TrimPrefix(line, "Words to evaluate: ")
			break
		}
	}

	words := strings.Split(wordsLine, ", ")
	for _, w := range words {
		w = strings.TrimSpace(w)
		if strings.Contains(strings.ToLower(w), "alice") {
			resp.New = append(resp.New, personal.Entity{Word: w, Type: "PERSON"})
		} else {
			resp.Ignore = append(resp.Ignore, w)
		}
	}

	return nil
}

func startDiscoveryWorker(
	ctx context.Context,
	store *personal.Store,
	matcher *personal.Matcher,
	unmappedCh <-chan []string,
	llm personal.LLMClient,
) {
	log.Println("starting discovery worker")
	
	// Batching ticker
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var batch []string
	
	processBatch := func() {
		if len(batch) == 0 {
			return
		}

		// Filter against now-known tokens in the Matcher (in case they were just learned)
		var filtered []string
		_, unmapped := matcher.Match(batch)
		
		// Dedup unmapped
		seen := make(map[string]struct{})
		for _, w := range unmapped {
			if _, ok := seen[w]; !ok {
				filtered = append(filtered, w)
				seen[w] = struct{}{}
			}
		}

		if len(filtered) == 0 {
			batch = nil
			return
		}

		log.Printf("discovery: processing %d words: %v", len(filtered), filtered)

		resp, err := personal.Discover(ctx, llm, filtered, "User chat context")
		if err != nil {
			log.Printf("discovery error: %v", err)
			batch = nil
			return
		}

		for _, entity := range resp.New {
			id, err := store.InsertToken(entity.Word, entity.Type)
			if err != nil {
				log.Printf("error inserting token %s: %v", entity.Word, err)
				continue
			}
			matcher.AddToken(entity.Word, id)
			log.Printf("discovery: learned new entity: %s (id=%d)", entity.Word, id)
		}

		for _, word := range resp.Ignore {
			err := store.AddIgnore(word)
			if err != nil {
				log.Printf("error adding ignore %s: %v", word, err)
				continue
			}
			matcher.AddIgnore(word)
			log.Printf("discovery: ignoring word: %s", word)
		}

		batch = nil
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("stopping discovery worker")
			return
		case words := <-unmappedCh:
			batch = append(batch, words...)
			if len(batch) > 10 {
				processBatch()
			}
		case <-ticker.C:
			processBatch()
		}
	}
}
