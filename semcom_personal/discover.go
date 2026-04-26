package personal

import (
	"context"
	"fmt"
)

// DiscoveryResponse is the structured response from the LLM discovery process.
type DiscoveryResponse struct {
	Topics []string `json:"topics"`
}

// DistillationResponse is the structured response for topic-distilled snippets.
type DistillationResponse struct {
	Distillations []DistilledSnippet `json:"distillations"`
}

type DistilledSnippet struct {
	Topic   string `json:"topic"`
	Snippet string `json:"snippet"`
}

// LLMClient defines the interface for communicating with an LLM.
type LLMClient interface {
	GenerateJSON(ctx context.Context, prompt string, target interface{}) error
}

// Discover takes a raw message and uses an LLM to extract key topics or subjects.
func Discover(ctx context.Context, client LLMClient, message string) (*DiscoveryResponse, error) {
	if message == "" {
		return &DiscoveryResponse{
			Topics: []string{},
		}, nil
	}

	prompt := fmt.Sprintf(`Extract the key topics or subjects from the following message. These should be meaningful entities, names, project titles, or specific terms that define the subject matter.

Message: %s

Return a JSON object with:
- "topics": an array of strings representing the extracted topics.
`, message)

	var resp DiscoveryResponse
	err := client.GenerateJSON(ctx, prompt, &resp)
	if err != nil {
		return nil, fmt.Errorf("llm discovery failed: %w", err)
	}

	return &resp, nil
}

// Distill takes a chunk of conversation and uses an LLM to extract and distill topics into snippets.
func Distill(ctx context.Context, client LLMClient, contextStr string) (*DistillationResponse, error) {
	if contextStr == "" {
		return &DistillationResponse{
			Distillations: []DistilledSnippet{},
		}, nil
	}

	prompt := fmt.Sprintf(`Extract and distill key personal knowledge snippets from the following conversation chunk. 

Focus on information that is NOT expected to exist within your general training data (e.g., personal preferences, specific project details, unique relationships, or facts about specific people).

For each distinct topic or subject, produce a concise, high-density snippet of the knowledge learned.

Conversation:
%s

Return a JSON object with:
- "distillations": an array of objects with "topic" and "snippet".
`, contextStr)

	var resp DistillationResponse
	err := client.GenerateJSON(ctx, prompt, &resp)
	if err != nil {
		return nil, fmt.Errorf("llm distillation failed: %w", err)
	}

	return &resp, nil
}
