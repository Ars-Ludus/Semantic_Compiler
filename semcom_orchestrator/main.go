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
	port := envOr("PORT", "8080")

	idx, err := semindex.Load(indexPath)
	if err != nil {
		log.Fatalf("load index: %v", err)
	}

	store, err := semanticstore.Open(dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer store.Close()

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
		embed:      idx,
		thresholds: semindex.Thresholds{L2: 0.25, L1: 0.20, L0: 0.15},
		store:      store,
		retriever:  retriever,
	}
	orch.turnSeq.Store(maxTurn)

	mux := http.NewServeMux()
	mux.HandleFunc("/chat", orch.handleChat)

	srv := &http.Server{Addr: ":" + port, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

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
