package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ars-Ludus/providertron/capability"
	"github.com/Ars-Ludus/providertron/provider"
)

// ProvidertronClient implements personal.LLMClient using the providertron library.
type ProvidertronClient struct {
	provider *provider.Provider
	model    string
}

func NewProvidertronClient(p *provider.Provider, model string) *ProvidertronClient {
	return &ProvidertronClient{
		provider: p,
		model:    model,
	}
}

func (c *ProvidertronClient) GenerateJSON(ctx context.Context, prompt string, target interface{}) error {
	req := capability.GenerateRequest{
		Model: c.model,
		Messages: []capability.Message{
			{
				Role:    "user",
				Content: prompt,
			},
		},
		Temperature: 0.1, // Low temperature for deterministic entity identification
	}

	resp, err := c.provider.Generate(ctx, req)
	if err != nil {
		return fmt.Errorf("providertron generate: %w", err)
	}

	// Extract JSON from response. LLMs sometimes wrap JSON in markdown blocks.
	cleanJSON := extractJSON(resp.Content)

	if err := json.Unmarshal([]byte(cleanJSON), target); err != nil {
		return fmt.Errorf("failed to parse discovery JSON: %w\nResponse was: %s", err, resp.Content)
	}

	return nil
}

func extractJSON(s string) string {
	// If it starts with ```json and ends with ```, strip them
	if strings.Contains(s, "```json") {
		parts := strings.Split(s, "```json")
		if len(parts) > 1 {
			s = parts[1]
			parts = strings.Split(s, "```")
			if len(parts) > 0 {
				s = parts[0]
			}
		}
	} else if strings.Contains(s, "```") {
		parts := strings.Split(s, "```")
		if len(parts) > 1 {
			s = parts[1]
			parts = strings.Split(s, "```")
			if len(parts) > 0 {
				s = parts[0]
			}
		}
	}
	return strings.TrimSpace(s)
}
