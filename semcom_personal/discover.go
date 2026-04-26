package personal

import (
	"context"
	"fmt"
	"strings"
)

// DiscoveryResponse is the structured response from the LLM discovery process.
type DiscoveryResponse struct {
	New    []Entity `json:"new"`
	Ignore []string `json:"ignore"`
}

// Entity represents a discovered personal token.
type Entity struct {
	Word string `json:"word"`
	Type string `json:"type"`
}

// LLMClient defines the interface for communicating with an LLM.
type LLMClient interface {
	GenerateJSON(ctx context.Context, prompt string, target interface{}) error
}

// Discover takes a list of candidate words and their context, and uses an LLM
// to identify which are new personal entities and which should be ignored.
func Discover(ctx context.Context, client LLMClient, words []string, contextStr string) (*DiscoveryResponse, error) {
	if len(words) == 0 {
		return &DiscoveryResponse{
			New:    []Entity{},
			Ignore: []string{},
		}, nil
	}

	prompt := fmt.Sprintf(`Identify which of the following words are personal entities (like people, places, projects, or specialized terms) that should be remembered, and which should be ignored.

Context: %s

Words to evaluate: %s

Return a JSON object with:
- "new": an array of objects with "word" and "type" (e.g., "PERSON", "PLACE", "PROJECT", "ORG") for entities we should learn.
- "ignore": an array of words that are common words or otherwise should not be tracked as personal entities.
`, contextStr, strings.Join(words, ", "))

	var resp DiscoveryResponse
	err := client.GenerateJSON(ctx, prompt, &resp)
	if err != nil {
		return nil, fmt.Errorf("llm discovery failed: %w", err)
	}

	return &resp, nil
}
