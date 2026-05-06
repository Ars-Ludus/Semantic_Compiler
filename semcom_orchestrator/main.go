package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	semanticstore "github.com/ars/semantic_store"
	semcomretrieve "github.com/ars/semcom_retrieve"
	adapter "semcom_adapter"
	"semcom_adapter/claudecode"
	"semcom_adapter/openclaw"
	distill "semcom_distill"
	semindex "semcom_embed"
	llmclient "semcom_llm"
	personal "semcom_personal"
	session "semcom_session"
	seminternal "semcom_internal"

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

	subcommand := "serve"
	if len(os.Args) > 1 {
		subcommand = os.Args[1]
	}

	switch subcommand {
	case "serve", "distill", "ingest-sessions":
	default:
		log.Fatalf("unknown subcommand %q; use: serve, distill, ingest-sessions", subcommand)
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
	personalDB, err := openSharedDB(personalDBPath, personal.Schema, distill.Schema, session.Schema)
	if err != nil {
		log.Fatalf("open personal db: %v", err)
	}
	defer personalDB.Close()

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

	maxTurn, err := store.MaxTurnID(context.Background())
	if err != nil {
		log.Fatalf("read max turn_id: %v", err)
	}

	orch := &Orchestrator{
		embed:             idx,
		personal:          pMatcher,
		personalStore:     pStore,
		personalRetriever: pRetriever,
		distillStore:      dStore,
		distillRetriever:  dRetriever,
		thresholds:        semindex.Thresholds{L2: 0.25, L1: 0.20, L0: 0.15},
		store:             store,
		retriever:         retriever,
	}
	orch.turnSeq.Store(maxTurn)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch subcommand {
	case "distill":
		client := newLLMClient()
		if err := RunDistillationPass(ctx, orch, client); err != nil {
			log.Fatalf("distillation: %v", err)
		}
		log.Println("distillation complete.")

	case "ingest-sessions":
		defaultDir := os.ExpandEnv("$HOME/.openclaw/agents/main/sessions")
		fs := flag.NewFlagSet("ingest-sessions", flag.ExitOnError)
		sessionsDir := fs.String("sessions-dir", defaultDir, "path to openclaw sessions directory")
		fs.Parse(os.Args[2:])
		if err := RunIngestSessions(ctx, orch, *sessionsDir); err != nil {
			log.Fatalf("ingest-sessions: %v", err)
		}

	case "serve":
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
	}
}

func openSharedDB(path string, schemas ...string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
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
	return db, nil
}

func newLLMClient() *llmclient.Client {
	apiKey := seminternal.EnvOr("GOOGLE_API_KEY", seminternal.EnvOr("GEMINI_API_KEY", ""))
	if apiKey == "" {
		log.Fatal("GOOGLE_API_KEY or GEMINI_API_KEY required for LLM passes")
	}
	model := seminternal.EnvOr("GEMINI_MODEL", "gemini-3.1-flash-lite-preview")
	client, err := llmclient.New(apiKey, model)
	if err != nil {
		log.Fatalf("create llm client: %v", err)
	}
	return client
}
