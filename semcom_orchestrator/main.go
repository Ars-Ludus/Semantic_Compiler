package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	providertron "github.com/Ars-Ludus/providertron"
	"github.com/Ars-Ludus/providertron/capability"
	"github.com/Ars-Ludus/providertron/provider"
	semanticstore "github.com/ars/semantic_store"
	semcomretrieve "github.com/ars/semcom_retrieve"
	adapter "semcom_adapter"
	"semcom_adapter/claudecode"
	"semcom_adapter/openclaw"
	distill "semcom_distill"
	semindex "semcom_embed"
	seminternal "semcom_internal"
	personal "semcom_personal"
	session "semcom_session"

	_ "modernc.org/sqlite"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("resolve executable path: %v", err)
	}
	exeDir := filepath.Dir(exe)

	indexPath := seminternal.EnvOr("INDEX_PATH", filepath.Join(exeDir, "index.bin"))
	dbPath := seminternal.EnvOr("DB_PATH", filepath.Join(exeDir, "memory.db"))
	personalDBPath := seminternal.EnvOr("PERSONAL_DB_PATH", filepath.Join(exeDir, "personal.db"))
	port := seminternal.EnvOr("PORT", "8080")
	userLabel := seminternal.EnvOr("SEMCOM_USER_NAME", "User")
	modelLabel := seminternal.EnvOr("SEMCOM_MODEL_NAME", "Assistant")

	subcommand := "serve"
	if len(os.Args) > 1 {
		subcommand = os.Args[1]
	}

	switch subcommand {
	case "serve", "distill-sessions", "ingest-openclaw":
	default:
		log.Fatalf("unknown subcommand %q; use: serve, distill-sessions, ingest-openclaw", subcommand)
	}

	idx, err := semindex.Load(indexPath)
	if err != nil {
		log.Fatalf("load index: %v", err)
	}

	store, err := semanticstore.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

	// One shared SQLite connection for all personalization modules.
	personalDB, err := openSharedDB(personalDBPath, personal.Schema, distill.Schema)
	if err != nil {
		log.Fatalf("open personal db: %v", err)
	}
	defer personalDB.Close()

	// Session tracking lives in memory.db alongside the memories it references.
	memoryDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open memory db for session tracker: %v", err)
	}
	if _, err := memoryDB.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		memoryDB.Close()
		log.Fatalf("set busy_timeout on memory db: %v", err)
	}
	if _, err := memoryDB.Exec(session.Schema); err != nil {
		memoryDB.Close()
		log.Fatalf("apply session schema: %v", err)
	}
	defer memoryDB.Close()

	pStore := personal.NewStore(personalDB)
	dStore := distill.NewStore(personalDB)

	pMatcher, err := personal.NewMatcher(pStore)
	if err != nil {
		log.Fatalf("create personal matcher: %v", err)
	}

	retriever, err := semcomretrieve.Open(store)
	if err != nil {
		log.Fatalf("open retriever: %v", err)
	}

	dRetriever, err := distill.NewDistillationRetriever(dStore)
	if err != nil {
		log.Fatalf("create distillation retriever: %v", err)
	}

	pRetriever, err := personal.NewPersonalRetriever(pStore)
	if err != nil {
		log.Fatalf("create personal retriever: %v", err)
	}

	sessionTracker := session.NewTracker(memoryDB)

	maxTurn, err := store.MaxTurnID(context.Background())
	if err != nil {
		log.Fatalf("read max turn_id: %v", err)
	}

	orch := &Orchestrator{
		embed:             idx,
		personal:          pMatcher,
		personalStore:     pStore,
		personalRetriever: pRetriever,
		sessionTracker:    sessionTracker,
		distillStore:      dStore,
		distillRetriever:  dRetriever,
		thresholds:        semindex.Thresholds{L2: 0.25, L1: 0.20, L0: 0.15},
		store:             store,
		retriever:         retriever,
	}
	orch.turnSeq.Store(maxTurn)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	orch.shutdownCtx = ctx

	switch subcommand {
	case "distill-sessions":
		fs := flag.NewFlagSet("distill-sessions", flag.ExitOnError)
		force := fs.Bool("force", false, "re-distill sessions that were already processed")
		fs.Parse(os.Args[2:])
		client, err := newLLMClient()
		if err != nil {
			log.Fatalf("%v", err)
		}
		if err := RunSessionDistillationPass(ctx, orch, client, userLabel, modelLabel, *force); err != nil {
			log.Fatalf("session distillation: %v", err)
		}
		log.Println("session distillation complete.")

	case "ingest-openclaw":
		fs := flag.NewFlagSet("ingest-openclaw", flag.ExitOnError)
		openClawDir := fs.String("dir", filepath.Join(os.Getenv("HOME"), ".openclaw"), "path to .openclaw directory")
		force := fs.Bool("force", false, "re-ingest sessions already processed")
		fs.Parse(os.Args[2:])
		client, err := newLLMClient()
		if err != nil {
			log.Fatalf("%v", err)
		}
		if err := RunOpenClawIngest(ctx, orch, client, userLabel, modelLabel, *openClawDir, *force); err != nil {
			log.Fatalf("openclaw ingest: %v", err)
		}
		log.Println("openclaw ingest complete.")

	case "serve":
		// Enable auto-distillation if credentials are configured; non-fatal if absent.
		if c, err := newLLMClient(); err == nil {
			orch.llmClient = c
			orch.userLabel = userLabel
			orch.modelLabel = modelLabel
			log.Println("auto-distill enabled: previous session will be distilled on session change")
		} else {
			log.Printf("auto-distill disabled: %v", err)
		}

		dispatcher := func(ctx context.Context, req adapter.CanonicalRequest) (adapter.CanonicalResponse, error) {
			if req.Op == adapter.OpIngest {
				result, err := orch.Ingest(ctx, IngestRequest{
					Text:      req.Prompt,
					SessionID: req.SessionID,
					Source:    semanticstore.Source(req.By),
				})
				if err != nil {
					return adapter.CanonicalResponse{}, err
				}
				resp := adapter.CanonicalResponse{}
				totalUs := result.EmbedUs + result.StoreUs
				switch req.Benchmark {
				case "total":
					resp.Benchmark = &adapter.Benchmark{TotalUs: totalUs}
				case "verbose":
					resp.Benchmark = &adapter.Benchmark{
						EmbedUs: &result.EmbedUs,
						StoreUs: &result.StoreUs,
						TotalUs: totalUs,
					}
				}
				return resp, nil
			}

			result, err := orch.Chat(ctx, ChatRequest{
				Prompt:    req.Prompt,
				SessionID: req.SessionID,
				By:        semanticstore.Source(req.By),
				TopK:      req.TopK,
			})
			if err != nil {
				return adapter.CanonicalResponse{}, err
			}
			resp := adapter.CanonicalResponse{}
			if len(result.Context) > 0 {
				hits := make([]adapter.ContextHit, len(result.Context))
				for i, h := range result.Context {
					hits[i] = adapter.ContextHit{
						Type:    adapter.HitType(h.Type),
						ID:      h.ID,
						Score:   h.Score,
						Topic:   h.Topic,
						Content: h.Content,
					}
				}
				resp.Context = hits
			}
			switch req.Benchmark {
			case "total":
				resp.Benchmark = &adapter.Benchmark{TotalUs: result.Benchmark.TotalUs}
			case "verbose":
				resp.Benchmark = &adapter.Benchmark{
					EmbedUs:    &result.Benchmark.EmbedUs,
					RetrieveUs: &result.Benchmark.RetrieveUs,
					StoreUs:    &result.Benchmark.StoreUs,
					TotalUs:    result.Benchmark.TotalUs,
				}
			}
			return resp, nil
		}

		mux := http.NewServeMux()
		mux.Handle("/chat", adapter.NewHandler(openclaw.Harness{}, dispatcher))
		mux.Handle("/hooks/claude", adapter.NewHandler(claudecode.Harness{}, dispatcher))

		srv := &http.Server{Addr: ":" + port, Handler: mux}
		go func() {
			log.Printf("listening on :%s (index=%s db=%s)", port, indexPath, dbPath)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				log.Fatalf("serve: %v", err)
			}
		}()

		<-ctx.Done()
		log.Println("shutting down")
		srv.Shutdown(context.Background())
		orch.bgWg.Wait()
	}
}

func openSharedDB(path string, schemas ...string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, err
	}
	for _, schema := range schemas {
		if _, err := db.Exec(schema); err != nil {
			db.Close()
			return nil, err
		}
	}
	for _, stmt := range []string{
		`ALTER TABLE distillations ADD COLUMN session_id TEXT`,
		`ALTER TABLE distillations ADD COLUMN entity TEXT`,
		`ALTER TABLE distillations ADD COLUMN entity_type TEXT`,
		`CREATE INDEX IF NOT EXISTS idx_distillations_session ON distillations(session_id)`,
	} {
		if _, err := db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			db.Close()
			return nil, fmt.Errorf("migrate: %w", err)
		}
	}
	return db, nil
}

func newLLMClient() (distill.LLMClient, error) {
	p, err := providertron.Get("gemini")
	if err != nil {
		return nil, err
	}
	return &providerLLMClient{p: p}, nil
}

// providerLLMClient adapts a providertron Provider to distill.LLMClient.
type providerLLMClient struct {
	p *provider.Provider
}

func (c *providerLLMClient) GenerateJSON(ctx context.Context, prompt string, target interface{}) error {
	req := capability.GenerateRequest{
		Messages: []capability.Message{
			{Role: "user", Content: prompt},
		},
		Temperature: 0.1,
	}
	resp, err := c.p.Generate(ctx, req)
	if err != nil {
		return fmt.Errorf("providertron generate: %w", err)
	}
	s := strings.TrimSpace(resp.Content)
	if i := strings.Index(s, "```"); i >= 0 {
		s = s[i+3:]
		if strings.HasPrefix(s, "json") {
			s = s[4:]
		}
		if j := strings.Index(s, "```"); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
	}
	if err := json.Unmarshal([]byte(s), target); err != nil {
		return fmt.Errorf("parse JSON: %w\nresponse: %s", err, resp.Content)
	}
	return nil
}
