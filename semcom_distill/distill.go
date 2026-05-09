package distill

import "context"

// LLMClient is satisfied by any type that can generate JSON from a prompt.
type LLMClient interface {
	GenerateJSON(ctx context.Context, prompt string, target interface{}) error
}
