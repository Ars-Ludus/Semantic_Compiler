package distill

import (
	"context"
	"encoding/json"
	"fmt"
)

// Snippet is a single knowledge entry produced by session distillation.
// Entity and EntityType are optional; when absent the snippet captures general
// knowledge not tied to a specific named thing.
type Snippet struct {
	Topic      string `json:"topic"`
	Snippet    string `json:"snippet"`
	Entity     string `json:"entity,omitempty"`
	EntityType string `json:"entity_type,omitempty"`
}

// SessionDistillationResponse is the structured output from SessionDistill and ConsolidateSnippets.
type SessionDistillationResponse struct {
	Snippets []Snippet `json:"snippets"`
}

// SessionDistill sends a complete session transcript to the LLM and returns
// a unified set of knowledge snippets. userLabel and modelLabel are injected
// into the prompt so the model uses deterministic names instead of pronouns.
func SessionDistill(ctx context.Context, client LLMClient, conversation, userLabel, modelLabel string) (*SessionDistillationResponse, error) {
	if conversation == "" {
		return &SessionDistillationResponse{}, nil
	}

	prompt := fmt.Sprintf(`You are building a long-term memory index for an AI assistant. Analyze the conversation below and extract the knowledge that would help the assistant orient itself in a future session with the same person.

In this conversation, [user] is %s and [model] is %s. Never use pronouns (I, you, he, she, they, we, his, her, their). Always refer to people and the assistant by name — write "%s prefers Python", not "they prefer Python" or "the user prefers Python".

For each distinct topic, produce a memory note: concise, fact-dense, present-tense. Write as if noting it for your own future reference — specific and direct, not narrative. Skip anything generic or derivable from common knowledge (what Python is, how Git works, etc.). Focus on what is specific to %s's context: preferences, decisions, projects, constraints, working patterns.

If a snippet is about a specific named person, project, organization, place, or domain-specific term, include its name and type.

Return JSON:
{
  "snippets": [
    {
      "topic": "<brief label>",
      "snippet": "<fact-dense memory note, present-tense, no pronouns>",
      "entity": "<proper name — omit if not entity-specific>",
      "entity_type": "<PERSON | PLACE | PROJECT | ORGANIZATION | TOPIC — omit if no entity>"
    }
  ]
}

Example (user = "Ars", model = "Claude"):
{
  "snippets": [
    {
      "topic": "language preference",
      "snippet": "Ars prefers Python for scripting and tooling; uses Go for systems work but reaches for Python first on one-off tasks.",
      "entity": "Python",
      "entity_type": "TOPIC"
    },
    {
      "topic": "CI configuration",
      "snippet": "Ars's GitHub Actions pipeline uses non-standard -X ldflags for ARM cross-compilation; undocumented in the repo.",
      "entity": "GitHub Actions",
      "entity_type": "PROJECT"
    },
    {
      "topic": "testing philosophy",
      "snippet": "Ars treats test-first as non-negotiable; always writes a failing test before any implementation, including small fixes."
    }
  ]
}

Conversation:
%s`, userLabel, modelLabel, userLabel, userLabel, conversation)

	var resp SessionDistillationResponse
	if err := client.GenerateJSON(ctx, prompt, &resp); err != nil {
		return nil, fmt.Errorf("llm session distillation failed: %w", err)
	}
	return &resp, nil
}

// ConsolidateSnippets merges an existing set of distillation snippets with a
// new set produced from the full session. Where snippets overlap on topic,
// newer information takes precedence. All unique knowledge is preserved.
// Returns either input unchanged when the other is empty (no LLM call).
func ConsolidateSnippets(ctx context.Context, client LLMClient, existing, next []Snippet) (*SessionDistillationResponse, error) {
	if len(existing) == 0 {
		return &SessionDistillationResponse{Snippets: next}, nil
	}
	if len(next) == 0 {
		return &SessionDistillationResponse{Snippets: existing}, nil
	}

	existingJSON, _ := json.Marshal(existing)
	nextJSON, _ := json.Marshal(next)

	prompt := fmt.Sprintf(`You are maintaining a long-term memory index. You have an existing set of knowledge snippets and a new set produced from the same session with additional messages. Merge them into a single canonical set.

Rules:
- Preserve all unique knowledge from both sets.
- Where snippets cover the same topic, merge into one: prefer the more specific or up-to-date information; update stale facts with newer ones.
- Do not duplicate information.
- Keep entity and entity_type where present; omit if not applicable.

Existing snippets:
%s

New snippets:
%s

Return JSON: {"snippets": [{"topic": "...", "snippet": "...", "entity": "...", "entity_type": "..."}]}
entity and entity_type are optional — omit if not entity-specific.`, existingJSON, nextJSON)

	var resp SessionDistillationResponse
	if err := client.GenerateJSON(ctx, prompt, &resp); err != nil {
		return nil, fmt.Errorf("llm consolidation failed: %w", err)
	}
	return &resp, nil
}
