package main

import (
	"encoding/json"
	"net/http"

	semanticstore "github.com/ars/semantic_store"
)

type chatRequest struct {
	Operation string `json:"operation"`
	Prompt    string `json:"prompt"`
	By        string `json:"by"`
	Source    string `json:"source"`    // accepted, reserved for future document tagging
	TopK      int    `json:"top_k"`
	Benchmark string `json:"benchmark"` // "ignore" | "total" | "verbose"
}

type contextHit struct {
	Type    HitType `json:"type"`
	ID      int32   `json:"id"`
	Score   int     `json:"score"`
	Topic   string  `json:"topic,omitempty"` // distilled only
	Content string  `json:"content"`
}

// Pointer fields: nil means not reported. Avoids omitempty silencing a real zero.
type benchmarkResponse struct {
	EmbedUs    *int64 `json:"embed_us,omitempty"`
	RetrieveUs *int64 `json:"retrieve_us,omitempty"`
	StoreUs    *int64 `json:"store_us,omitempty"`
	TotalUs    int64  `json:"total_us"`
}

type chatResponse struct {
	Context   []contextHit       `json:"context,omitempty"`
	Benchmark *benchmarkResponse `json:"benchmark,omitempty"`
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func (o *Orchestrator) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req chatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	if req.Operation != "chat" && req.Operation != "ingest" {
		writeError(w, http.StatusBadRequest, `operation must be "chat" or "ingest"`)
		return
	}
	if req.Prompt == "" {
		writeError(w, http.StatusBadRequest, "prompt is required")
		return
	}
	if req.By != "user" && req.By != "model" {
		writeError(w, http.StatusBadRequest, `by must be "user" or "model"`)
		return
	}

	benchmarkMode := req.Benchmark
	if benchmarkMode == "" {
		benchmarkMode = "ignore"
	}
	if benchmarkMode != "ignore" && benchmarkMode != "total" && benchmarkMode != "verbose" {
		writeError(w, http.StatusBadRequest, `benchmark must be "ignore", "total", or "verbose"`)
		return
	}

	w.Header().Set("Content-Type", "application/json")

	if req.Operation == "ingest" {
		result, err := o.Ingest(r.Context(), IngestRequest{
			Text:   req.Prompt,
			Source: semanticstore.Source(req.By),
		})
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		resp := chatResponse{}
		totalUs := result.EmbedUs + result.StoreUs
		switch benchmarkMode {
		case "total":
			resp.Benchmark = &benchmarkResponse{TotalUs: totalUs}
		case "verbose":
			resp.Benchmark = &benchmarkResponse{
				EmbedUs: &result.EmbedUs,
				StoreUs: &result.StoreUs,
				TotalUs: totalUs,
			}
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	topK := req.TopK
	if topK <= 0 {
		topK = 5
	}

	result, err := o.Chat(r.Context(), ChatRequest{
		Prompt: req.Prompt,
		By:     semanticstore.Source(req.By),
		TopK:   topK,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	resp := chatResponse{}

	if len(result.Context) > 0 {
		hits := make([]contextHit, len(result.Context))
		for i, h := range result.Context {
			hits[i] = contextHit{Type: h.Type, ID: h.ID, Score: h.Score, Topic: h.Topic, Content: h.Content}
		}
		resp.Context = hits
	}

	switch benchmarkMode {
	case "total":
		resp.Benchmark = &benchmarkResponse{TotalUs: result.Benchmark.TotalUs}
	case "verbose":
		resp.Benchmark = &benchmarkResponse{
			EmbedUs:    &result.Benchmark.EmbedUs,
			RetrieveUs: &result.Benchmark.RetrieveUs,
			StoreUs:    &result.Benchmark.StoreUs,
			TotalUs:    result.Benchmark.TotalUs,
		}
	}

	json.NewEncoder(w).Encode(resp)
}
