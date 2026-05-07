package adapter

import "context"

type Op string

const (
	OpChat   Op = "chat"
	OpIngest Op = "ingest"
)

type CanonicalRequest struct {
	Op        Op
	Prompt    string
	SessionID string
	By        string // "user" | "model"
	TopK      int    // 0 = use default
	Benchmark string // "ignore" | "total" | "verbose"
}

type HitType string

const (
	HitDistilled HitType = "distilled"
	HitWiki      HitType = "wiki"
)

type ContextHit struct {
	Type    HitType
	ID      int32
	Score   int
	Topic   string
	Content string
}

type Benchmark struct {
	EmbedUs    *int64
	RetrieveUs *int64
	StoreUs    *int64
	TotalUs    int64
}

type CanonicalResponse struct {
	Context   []ContextHit
	Benchmark *Benchmark
}

// Harness translates between a harness-specific wire format and the canonical types.
type Harness interface {
	Decode(raw []byte) (CanonicalRequest, error)
	Encode(CanonicalResponse) ([]byte, error)
}

// Dispatcher is provided by the orchestrator: it maps a CanonicalRequest to a CanonicalResponse.
type Dispatcher func(ctx context.Context, req CanonicalRequest) (CanonicalResponse, error)
