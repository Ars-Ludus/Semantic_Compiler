package distill

import (
	"context"
	"fmt"
)

// LLMClient is satisfied by any type that can generate JSON from a prompt.
type LLMClient interface {
	GenerateJSON(ctx context.Context, prompt string, target interface{}) error
}

// Entity is a named entity extracted from a conversation chunk.
type Entity struct {
	Text string `json:"text"`
	Type string `json:"type"` // "PERSON", "PLACE", "PROJECT", "ORGANIZATION", "TOPIC"
}

// DistillationResponse is the structured output from the LLM distillation call.
type DistillationResponse struct {
	Distillations []DistilledSnippet `json:"distillations"`
	Entities      []Entity           `json:"entities"`
}

// DistilledSnippet is a single topic/knowledge pair extracted from a conversation chunk.
type DistilledSnippet struct {
	Topic   string `json:"topic"`
	Snippet string `json:"snippet"`
}

// Distill sends a conversation chunk to the LLM and returns compressed knowledge
// snippets and named entities in a single call.
func Distill(ctx context.Context, client LLMClient, conversation string) (*DistillationResponse, error) {
	if conversation == "" {
		return &DistillationResponse{}, nil
	}

	prompt := fmt.Sprintf(`Analyze the following conversation chunk and extract two things:

1. Knowledge distillations: compressed topic/snippet pairs capturing personal, project-specific, or contextual knowledge that would NOT be found in generic training data (e.g. personal preferences, specific project details, unique relationships, domain-specific facts about named people or organizations).

2. Named entities: specific proper names that appear in the conversation — people, places, projects, organizations, or domain-specific terms unique to this person's context. Do NOT include generic concepts (e.g. "database", "meeting", "issue").

Conversation:
%s

Return a JSON object with:
- "distillations": array of {"topic": string, "snippet": string}
- "entities": array of {"text": string, "type": "PERSON"|"PLACE"|"PROJECT"|"ORGANIZATION"|"TOPIC"}
`, conversation)

	var resp DistillationResponse
	if err := client.GenerateJSON(ctx, prompt, &resp); err != nil {
		return nil, fmt.Errorf("llm distillation failed: %w", err)
	}
	return &resp, nil
}
