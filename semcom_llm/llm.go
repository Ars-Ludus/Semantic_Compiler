package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Ars-Ludus/providertron/capability"
	"github.com/Ars-Ludus/providertron/provider"
	"github.com/Ars-Ludus/providertron/providers/gemini"
)

// Client wraps a providertron provider and satisfies any LLMClient interface
// that requires GenerateJSON.
type Client struct {
	provider *provider.Provider
	model    string
}

// New creates a Gemini-backed Client.
func New(apiKey, model string) (*Client, error) {
	cfg := &gemini.Config{APIKey: apiKey, Model: model}
	backend, err := gemini.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("create gemini backend: %w", err)
	}
	p, err := provider.New(cfg, backend)
	if err != nil {
		return nil, fmt.Errorf("create provider: %w", err)
	}
	return &Client{provider: p, model: model}, nil
}

// GenerateJSON sends prompt to the model and unmarshals the JSON response into target.
func (c *Client) GenerateJSON(ctx context.Context, prompt string, target interface{}) error {
	req := capability.GenerateRequest{
		Model: c.model,
		Messages: []capability.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
	}
	resp, err := c.provider.Generate(ctx, req)
	if err != nil {
		return fmt.Errorf("providertron generate: %w", err)
	}
	if err := json.Unmarshal([]byte(stripFence(resp.Content)), target); err != nil {
		return fmt.Errorf("parse JSON: %w\nresponse: %s", err, resp.Content)
	}
	return nil
}

// stripFence removes optional ```json ... ``` or ``` ... ``` markdown fences.
func stripFence(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		if strings.HasPrefix(s, "json") {
			s = s[4:]
		}
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
	}
	return strings.TrimSpace(s)
}
