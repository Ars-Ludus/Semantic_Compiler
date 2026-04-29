package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	semanticstore "github.com/ars/semantic_store"
)

type ocSessionRecord struct {
	Type    string     `json:"type"`
	Message *ocMessage `json:"message,omitempty"`
}

type ocMessage struct {
	Role    string      `json:"role"`
	Content []ocContent `json:"content"`
}

type ocContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// stripUserPrefix removes the openclaw "Sender (untrusted metadata):" block and
// the leading timestamp bracket (e.g. "[Wed 2026-04-29 06:06 CDT] ") from user messages.
func stripUserPrefix(s string) string {
	// Remove metadata block: everything up to and including the closing ```\n\n
	const marker = "```\n\n"
	if i := strings.Index(s, marker); i >= 0 {
		s = s[i+len(marker):]
	}
	// Remove leading timestamp bracket "[...] "
	if len(s) > 0 && s[0] == '[' {
		if i := strings.Index(s, "] "); i >= 0 {
			s = s[i+2:]
		}
	}
	return strings.TrimSpace(s)
}

// RunIngestSessions reads all session JSONL files from sessionsDir and ingests
// unprocessed user/assistant turns. Already-ingested sessions are skipped.
func RunIngestSessions(ctx context.Context, o *Orchestrator, sessionsDir string) error {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return fmt.Errorf("read sessions dir: %w", err)
	}

	total := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		// Skip reset/archived files (e.g. foo.jsonl.reset.2026-04-29T11-06-17.439Z)
		if strings.Contains(name, ".jsonl.") {
			continue
		}

		sessionID := strings.TrimSuffix(name, ".jsonl")
		key := "ingested_session:" + sessionID

		val, err := o.distillStore.GetMetadata(key)
		if err != nil {
			return fmt.Errorf("check metadata for %s: %w", sessionID, err)
		}
		if val != "" {
			log.Printf("skipping already-ingested session %s", sessionID)
			continue
		}

		path := filepath.Join(sessionsDir, name)
		n, err := ingestSessionFile(ctx, o, path)
		if err != nil {
			log.Printf("error ingesting session %s: %v", sessionID, err)
			continue
		}

		if err := o.distillStore.SetMetadata(key, "1"); err != nil {
			return fmt.Errorf("mark ingested for %s: %w", sessionID, err)
		}
		log.Printf("ingested session %s: %d turns", sessionID, n)
		total += n
	}

	log.Printf("ingest-sessions complete: %d total turns ingested", total)
	return nil
}

func ingestSessionFile(ctx context.Context, o *Orchestrator, path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	count := 0
	for scanner.Scan() {
		var rec ocSessionRecord
		if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
			continue
		}
		if rec.Type != "message" || rec.Message == nil {
			continue
		}

		role := rec.Message.Role
		if role != "user" && role != "assistant" {
			continue
		}

		var sb strings.Builder
		for _, c := range rec.Message.Content {
			if c.Type == "text" {
				sb.WriteString(c.Text)
			}
		}
		text := sb.String()

		if role == "user" {
			text = stripUserPrefix(text)
		}
		if text == "" {
			continue
		}

		source := semanticstore.SourceUser
		if role == "assistant" {
			source = semanticstore.SourceModel
		}

		if _, err := o.Ingest(ctx, IngestRequest{Text: text, Source: source}); err != nil {
			return count, fmt.Errorf("ingest turn: %w", err)
		}
		count++
	}
	return count, scanner.Err()
}
