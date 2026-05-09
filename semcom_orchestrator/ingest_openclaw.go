package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	semanticstore "github.com/ars/semantic_store"
	distill "semcom_distill"
)

var (
	reSemcomMemory = regexp.MustCompile(`(?s)<semcom_memory>.*?</semcom_memory>`)
	reTimestamp    = regexp.MustCompile(`^\[[A-Za-z]{3} \d{4}-\d{2}-\d{2} \d{2}:\d{2} [A-Z]{2,4}\]\s*`)
	reFinalTag     = regexp.MustCompile(`(?s)<final>(.*?)</final>`)
)

type openClawLine struct {
	Type    string         `json:"type"`
	Message *openClawTurn `json:"message,omitempty"`
}

type openClawTurn struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type openClawPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// RunOpenClawIngest reads all session files under openClawDir, ingests clean
// user/assistant turns into the memory store, then distils each session.
func RunOpenClawIngest(ctx context.Context, o *Orchestrator, llm distill.LLMClient, userLabel, modelLabel, openClawDir string, force bool) error {
	paths, err := findOpenClawSessions(openClawDir)
	if err != nil {
		return fmt.Errorf("find sessions: %w", err)
	}
	log.Printf("found %d openclaw session files", len(paths))

	for _, path := range paths {
		sessionID := sessionIDFromPath(path)
		if err := ingestOpenClawSession(ctx, o, path, sessionID, force); err != nil {
			log.Printf("ingest session %s: %v", sessionID, err)
			continue
		}
		if llm != nil {
			if err := distillOneSession(ctx, o, llm, sessionID, userLabel, modelLabel, false); err != nil {
				log.Printf("distill session %s: %v", sessionID, err)
			}
		}
	}
	return nil
}

func findOpenClawSessions(openClawDir string) ([]string, error) {
	pattern := filepath.Join(openClawDir, "agents", "*", "sessions", "*.jsonl")
	all, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, p := range all {
		base := filepath.Base(p)
		if strings.Contains(base, ".trajectory") ||
			strings.Contains(base, ".checkpoint") ||
			strings.Contains(base, ".deleted") {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

func sessionIDFromPath(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".jsonl")
}

func ingestOpenClawSession(ctx context.Context, o *Orchestrator, path, sessionID string, force bool) error {
	metaKey := "openclaw_ingested:" + sessionID
	if !force {
		val, err := o.distillStore.GetMetadata(metaKey)
		if err == nil && val != "" {
			log.Printf("session %s: already ingested (%s turns), skipping", sessionID, val)
			return nil
		}
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	var count int
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024)
	for scanner.Scan() {
		var line openClawLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue
		}
		if line.Type != "message" || line.Message == nil {
			continue
		}
		turn := line.Message
		var text string
		var source semanticstore.Source
		switch turn.Role {
		case "user":
			text = extractOpenClawUserText(turn.Content)
			source = semanticstore.SourceUser
		case "assistant":
			text = extractOpenClawAssistantText(turn.Content)
			source = semanticstore.SourceModel
		default:
			continue
		}
		if text == "" {
			continue
		}
		if _, err := o.Ingest(ctx, IngestRequest{
			Text:      text,
			SessionID: sessionID,
			Source:    source,
		}); err != nil {
			log.Printf("ingest turn in session %s: %v", sessionID, err)
			continue
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan %s: %w", path, err)
	}

	log.Printf("session %s: ingested %d turns", sessionID, count)
	if err := o.distillStore.SetMetadata(metaKey, fmt.Sprintf("%d", count)); err != nil {
		log.Printf("set metadata for session %s: %v", sessionID, err)
	}
	return nil
}

func extractOpenClawUserText(raw json.RawMessage) string {
	text := openClawTextFromContent(raw)
	text = reSemcomMemory.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)
	text = reTimestamp.ReplaceAllString(text, "")
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "[Bootstrap pending]") {
		return ""
	}
	return text
}

func extractOpenClawAssistantText(raw json.RawMessage) string {
	text := openClawTextFromContent(raw)
	// Strip non-closed <think> prefix that appears before <final>
	if strings.HasPrefix(text, "<think>") {
		if fi := strings.Index(text, "<final>"); fi > 0 {
			text = text[fi:]
		} else {
			text = strings.TrimPrefix(text, "<think>")
		}
	}
	// Unwrap <final>...</final>
	if m := reFinalTag.FindStringSubmatch(text); m != nil {
		text = m[1]
	}
	return strings.TrimSpace(text)
}

func openClawTextFromContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []json.RawMessage
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		var part openClawPart
		if err := json.Unmarshal(p, &part); err != nil {
			continue
		}
		if part.Type == "text" && part.Text != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}
