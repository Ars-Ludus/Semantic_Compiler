package personal

import (
	"context"
	"encoding/json"
	"testing"
)

type mockLLM struct {
	response DiscoveryResponse
}

func (m *mockLLM) GenerateJSON(ctx context.Context, prompt string, target interface{}) error {
	b, _ := json.Marshal(m.response)
	return json.Unmarshal(b, target)
}

func TestDiscover(t *testing.T) {
	ctx := context.Background()
	llm := &mockLLM{
		response: DiscoveryResponse{
			Topics: []string{"Alice", "Wonderland"},
		},
	}

	t.Run("successful_discovery", func(t *testing.T) {
		resp, err := Discover(ctx, llm, "Alice in Wonderland")
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if len(resp.Topics) != 2 {
			t.Errorf("expected 2 topics, got %d", len(resp.Topics))
		}
	})

	t.Run("empty_message", func(t *testing.T) {
		resp, err := Discover(ctx, llm, "")
		if err != nil {
			t.Fatalf("Discover: %v", err)
		}
		if len(resp.Topics) != 0 {
			t.Errorf("expected 0 topics, got %d", len(resp.Topics))
		}
	})
}
