package adapter

import (
	"encoding/json"
	"io"
	"net/http"
)

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// NewHandler returns an http.HandlerFunc that decodes via h, dispatches via d,
// and encodes the response via h.
func NewHandler(h Harness, d Dispatcher) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeError(w, http.StatusBadRequest, "failed to read request body")
			return
		}

		req, err := h.Decode(body)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		if req.Op != OpChat && req.Op != OpIngest {
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
		if req.TopK <= 0 {
			req.TopK = 5
		}
		if req.Benchmark == "" {
			req.Benchmark = "ignore"
		}
		if req.Benchmark != "ignore" && req.Benchmark != "total" && req.Benchmark != "verbose" {
			writeError(w, http.StatusBadRequest, `benchmark must be "ignore", "total", or "verbose"`)
			return
		}

		resp, err := d(r.Context(), req)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		out, err := h.Encode(resp)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to encode response")
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.Write(out)
	}
}
