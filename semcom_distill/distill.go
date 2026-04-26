package distill

import (
	"context"
	"fmt"
)

// LLMClient is satisfied by any type that can generate JSON from a prompt.
type LLMClient interface {
	GenerateJSON(ctx context.Context, prompt string, target interface{}) error
}

// DistillationResponse is the structured output from the LLM distillation call.
type DistillationResponse struct {
	Distillations []DistilledSnippet `json:"distillations"`
}

// DistilledSnippet is a single topic/knowledge pair extracted from a conversation chunk.
type DistilledSnippet struct {
	Topic   string `json:"topic"`
	Snippet string `json:"snippet"`
}

// Distill sends a conversation chunk to the LLM and returns compressed knowledge snippets.
func Distill(ctx context.Context, client LLMClient, conversation string) (*DistillationResponse, error) {
	if conversation == "" {
		return &DistillationResponse{}, nil
	}

	prompt := fmt.Sprintf(`Extract and distill key personal knowledge snippets from the following conversation chunk.

Focus on information that is NOT expected to exist within your general training data (e.g., personal preferences, specific project details, unique relationships, or facts about specific people).

For each distinct topic or subject, produce a concise, high-density snippet of the knowledge learned.

Conversation:
%s

Return a JSON object with:
- "distillations": an array of objects with "topic" and "snippet".
`, conversation)

	var resp DistillationResponse
	if err := client.GenerateJSON(ctx, prompt, &resp); err != nil {
		return nil, fmt.Errorf("llm distillation failed: %w", err)
	}
	return &resp, nil
}
