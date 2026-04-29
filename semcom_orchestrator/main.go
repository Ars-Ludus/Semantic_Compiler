package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	semanticstore "github.com/ars/semantic_store"
	semcomretrieve "github.com/ars/semcom_retrieve"
	distill "semcom_distill"
	semindex "semcom_embed"
	llmclient "semcom_llm"
	personal "semcom_personal"
	seminternal "semcom_internal"

	_ "modernc.org/sqlite"
)

func main() {
	indexPath := seminternal.EnvOr("INDEX_PATH", "index.bin")
	dbPath := seminternal.EnvOr("DB_PATH", "memory.db")
	personalDBPath := seminternal.EnvOr("PERSONAL_DB_PATH", "personal.db")
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
	personalDB, err := openSharedDB(personalDBPath, personal.Schema, distill.Schema)
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
		mux := http.NewServeMux()
		mux.HandleFunc("/chat", orch.handleChat)

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
	model := seminternal.EnvOr("GEMINI_MODEL", "gemini-2.5-flash-preview-04-17")
	client, err := llmclient.New(apiKey, model)
	if err != nil {
		log.Fatalf("create llm client: %v", err)
	}
	return client
}
