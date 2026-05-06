package openclaw

import (
	"encoding/json"
	"fmt"

	adapter "semcom_adapter"
)

// Harness implements adapter.Harness for the openClaw Plugin SDK wire format.
type Harness struct{}

type wireRequest struct {
	Operation string `json:"operation"`
	Prompt    string `json:"prompt"`
	SessionID string `json:"session_id"`
	By        string `json:"by"`
	Source    string `json:"source"`    // accepted, reserved for future document tagging
	TopK      int    `json:"top_k"`
	Benchmark string `json:"benchmark"` // "ignore" | "total" | "verbose"
}

type wireContextHit struct {
	Type    adapter.HitType `json:"type"`
	ID      int32           `json:"id"`
	Score   int             `json:"score"`
	Topic   string          `json:"topic,omitempty"` // distilled only
	Content string          `json:"content"`
}

type wireBenchmark struct {
	EmbedUs    *int64 `json:"embed_us,omitempty"`
	RetrieveUs *int64 `json:"retrieve_us,omitempty"`
	StoreUs    *int64 `json:"store_us,omitempty"`
	TotalUs    int64  `json:"total_us"`
}

type wireResponse struct {
	Context   []wireContextHit `json:"context,omitempty"`
	Benchmark *wireBenchmark   `json:"benchmark,omitempty"`
}

func (Harness) Decode(raw []byte) (adapter.CanonicalRequest, error) {
	var wr wireRequest
	if err := json.Unmarshal(raw, &wr); err != nil {
		return adapter.CanonicalRequest{}, fmt.Errorf("invalid JSON: %w", err)
	}
	return adapter.CanonicalRequest{
		Op:        adapter.Op(wr.Operation),
		Prompt:    wr.Prompt,
		SessionID: wr.SessionID,
		By:        wr.By,
		TopK:      wr.TopK,
		Benchmark: wr.Benchmark,
	}, nil
}

func (Harness) Encode(resp adapter.CanonicalResponse) ([]byte, error) {
	wr := wireResponse{}

	if len(resp.Context) > 0 {
		wr.Context = make([]wireContextHit, len(resp.Context))
		for i, h := range resp.Context {
			wr.Context[i] = wireContextHit{
				Type:    h.Type,
				ID:      h.ID,
				Score:   h.Score,
				Topic:   h.Topic,
				Content: h.Content,
			}
		}
	}

	if resp.Benchmark != nil {
		wr.Benchmark = &wireBenchmark{
			EmbedUs:    resp.Benchmark.EmbedUs,
			RetrieveUs: resp.Benchmark.RetrieveUs,
			StoreUs:    resp.Benchmark.StoreUs,
			TotalUs:    resp.Benchmark.TotalUs,
		}
	}

	return json.Marshal(wr)
}
