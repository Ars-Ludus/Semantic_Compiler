package claudecode

import (
	"encoding/json"
	"fmt"
	"strings"

	adapter "semcom_adapter"
)

// Harness implements adapter.Harness for Claude Code's UserPromptSubmit hook event.
type Harness struct{}

type wireInput struct {
	HookEventName string `json:"hook_event_name"`
	Prompt        string `json:"prompt"`
	SessionID     string `json:"session_id"`
	Cwd           string `json:"cwd"`
}

type wireOutput struct {
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}

type hookSpecificOutput struct {
	HookEventName     string `json:"hookEventName"`
	AdditionalContext string `json:"additionalContext"`
}

func (Harness) Decode(raw []byte) (adapter.CanonicalRequest, error) {
	var in wireInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return adapter.CanonicalRequest{}, fmt.Errorf("invalid JSON: %w", err)
	}
	if in.HookEventName != "UserPromptSubmit" {
		return adapter.CanonicalRequest{}, fmt.Errorf("unsupported hook_event_name %q: only UserPromptSubmit is handled at this endpoint", in.HookEventName)
	}
	return adapter.CanonicalRequest{
		Op:     adapter.OpChat,
		Prompt: in.Prompt,
		By:     "user",
	}, nil
}

func (Harness) Encode(resp adapter.CanonicalResponse) ([]byte, error) {
	if len(resp.Context) == 0 {
		return []byte("{}"), nil
	}

	lines := make([]string, 0, len(resp.Context))
	for _, h := range resp.Context {
		var label string
		if h.Topic != "" {
			label = fmt.Sprintf("[%s | %s]", h.Type, h.Topic)
		} else {
			label = fmt.Sprintf("[%s]", h.Type)
		}
		lines = append(lines, label+" "+h.Content)
	}
	block := "<semcom_memory>\n" + strings.Join(lines, "\n") + "\n</semcom_memory>"

	out := wireOutput{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:     "UserPromptSubmit",
			AdditionalContext: block,
		},
	}
	return json.Marshal(out)
}
