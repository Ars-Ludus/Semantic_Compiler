package personal

import (
	"context"
	"fmt"
)

// LLMClient is satisfied by any type that can generate JSON from a prompt.
type LLMClient interface {
	GenerateJSON(ctx context.Context, prompt string, target interface{}) error
}

// DiscoveryResponse is the structured output from the LLM discovery call.
type DiscoveryResponse struct {
	Topics []string `json:"topics"`
}

// Discover extracts key topics from a raw message using the LLM.
func Discover(ctx context.Context, client LLMClient, message string) (*DiscoveryResponse, error) {
	if message == "" {
		return &DiscoveryResponse{}, nil
	}

	prompt := fmt.Sprintf(`Extract the key topics or subjects from the following message. These should be meaningful entities, names, project titles, or specific terms that define the subject matter.

Message: %s

Return a JSON object with:
- "topics": an array of strings representing the extracted topics.
`, message)

	var resp DiscoveryResponse
	if err := client.GenerateJSON(ctx, prompt, &resp); err != nil {
		return nil, fmt.Errorf("llm discovery failed: %w", err)
	}
	return &resp, nil
}
