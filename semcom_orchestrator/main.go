package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	semanticstore "github.com/ars/semantic_store"
	semcomretrieve "github.com/ars/semcom_retrieve"
	semindex "semcom_embed"
	personal "semcom_personal"

	"github.com/Ars-Ludus/providertron/provider"
	"github.com/Ars-Ludus/providertron/providers/gemini"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	indexPath := envOr("INDEX_PATH", "index.bin")
	dbPath := envOr("DB_PATH", "memory.db")
	personalDBPath := envOr("PERSONAL_DB_PATH", "personal.db")
	port := envOr("PORT", "8080")

	subcommand := "serve"
	if len(os.Args) > 1 {
		subcommand = os.Args[1]
	}

	switch subcommand {
	case "serve", "discover", "distill":
	default:
		log.Fatalf("unknown subcommand %q; use: serve, discover, distill", subcommand)
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

	pStore, err := personal.Open(personalDBPath)
	if err != nil {
		log.Fatalf("open personal store: %v", err)
	}
	defer pStore.Close()

	pMatcher, err := personal.NewMatcher(pStore)
	if err != nil {
		log.Fatalf("create personal matcher: %v", err)
	}

	retriever, err := semcomretrieve.Open(store, semcomretrieve.Options{AutoRefresh: true})
	if err != nil {
		log.Fatalf("open retriever: %v", err)
	}
	defer retriever.Close()

	maxTurn, err := store.MaxTurnID(context.Background())
	if err != nil {
		log.Fatalf("read max turn_id: %v", err)
	}

	orch := &Orchestrator{
		embed:         idx,
		personal:      pMatcher,
		personalStore: pStore,
		thresholds:    semindex.Thresholds{L2: 0.25, L1: 0.20, L0: 0.15},
		store:         store,
		retriever:     retriever,
	}
	orch.turnSeq.Store(maxTurn)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	switch subcommand {
	case "discover":
		client := newLLMClient()
		if err := RunDiscoveryPass(ctx, orch, client); err != nil {
			log.Fatalf("discovery: %v", err)
		}
		log.Println("discovery complete.")

	case "distill":
		client := newLLMClient()
		log.Println("running discovery pass before distillation...")
		if err := RunDiscoveryPass(ctx, orch, client); err != nil {
			log.Fatalf("discovery: %v", err)
		}
		if err := RunDistillationPass(ctx, orch, client); err != nil {
			log.Fatalf("distillation: %v", err)
		}
		log.Println("distillation complete.")

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

func newLLMClient() *ProvidertronClient {
	apiKey := envOr("GOOGLE_API_KEY", envOr("GEMINI_API_KEY", ""))
	if apiKey == "" {
		log.Fatal("GOOGLE_API_KEY or GEMINI_API_KEY required for LLM passes")
	}
	model := envOr("GEMINI_MODEL", "gemini-2.5-flash-preview-04-17")
	cfg := &gemini.Config{APIKey: apiKey, Model: model}
	backend, err := gemini.New(cfg)
	if err != nil {
		log.Fatalf("create gemini backend: %v", err)
	}
	p, err := provider.New(cfg, backend)
	if err != nil {
		log.Fatalf("create provider: %v", err)
	}
	return NewProvidertronClient(p, model)
}
