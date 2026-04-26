package personal

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

type mockLLMClient struct {
	response *DiscoveryResponse
	err      error
}

func (m *mockLLMClient) GenerateJSON(ctx context.Context, prompt string, target interface{}) error {
	if m.err != nil {
		return m.err
	}
	
	// We simulate the JSON unmarshaling that the real client would do
	data, _ := json.Marshal(m.response)
	return json.Unmarshal(data, target)
}

func TestDiscover(t *testing.T) {
	ctx := context.Background()

	t.Run("successful discovery", func(t *testing.T) {
		mockResp := &DiscoveryResponse{
			New: []Entity{
				{Word: "Ars", Type: "PERSON"},
				{Word: "SemCom", Type: "PROJECT"},
			},
			Ignore: []string{"the", "is", "a"},
		}
		client := &mockLLMClient{response: mockResp}

		words := []string{"Ars", "is", "working", "on", "SemCom"}
		contextStr := "Ars is working on SemCom."

		resp, err := Discover(ctx, client, words, contextStr)
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}

		if !reflect.DeepEqual(resp, mockResp) {
			t.Errorf("expected %+v, got %+v", mockResp, resp)
		}
	})

	t.Run("empty words", func(t *testing.T) {
		client := &mockLLMClient{}
		resp, err := Discover(ctx, client, []string{}, "any context")
		if err != nil {
			t.Fatalf("Discover failed: %v", err)
		}
		if len(resp.New) != 0 || len(resp.Ignore) != 0 {
			t.Errorf("expected empty response, got %+v", resp)
		}
	})

	t.Run("llm failure", func(t *testing.T) {
		expectedErr := fmt.Errorf("api error")
		client := &mockLLMClient{err: expectedErr}
		_, err := Discover(ctx, client, []string{"word"}, "context")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), expectedErr.Error()) {
			t.Errorf("expected error to contain %q, got %q", expectedErr.Error(), err.Error())
		}
	})
}
