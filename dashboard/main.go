package main

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	semanticstore "github.com/ars/semantic_store"
	semcomretrieve "github.com/ars/semcom_retrieve"
	semindex "semcom_embed"
	seminternal "semcom_internal"
	_ "modernc.org/sqlite"
)

//go:embed static
var staticFiles embed.FS

// Server holds all shared state.
type Server struct {
	idx        *semindex.Index
	thresholds semindex.Thresholds
	store      semanticstore.Store
	retriever  *semcomretrieve.Retriever
	db         *sql.DB
	benchmarks *BenchmarkRing
}

// BenchmarkEntry records the timing for a single operation.
type BenchmarkEntry struct {
	Op         string    `json:"op"`
	EmbedUs    int64     `json:"embed_us"`
	RetrieveUs int64     `json:"retrieve_us"`
	L0Count    int       `json:"l0_count"`
	Tokens     int       `json:"tokens"`
	Timestamp  time.Time `json:"ts"`
}

const ringSize = 200

// BenchmarkRing is a bounded buffer of recent benchmark entries.
type BenchmarkRing struct {
	mu  sync.Mutex
	buf []BenchmarkEntry
}

func (r *BenchmarkRing) add(e BenchmarkEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf = append(r.buf, e)
	if len(r.buf) > ringSize {
		r.buf = r.buf[len(r.buf)-ringSize:]
	}
}

// recent returns up to n entries in newest-first order.
func (r *BenchmarkRing) recent(n int) []BenchmarkEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	if n > len(r.buf) {
		n = len(r.buf)
	}
	out := make([]BenchmarkEntry, n)
	for i := range n {
		out[i] = r.buf[len(r.buf)-1-i]
	}
	return out
}

// Memory is the API shape for a stored memory row.
type Memory struct {
	ID          int64     `json:"id"`
	TurnID      int64     `json:"turn_id"`
	Source      string    `json:"source"`
	Raw         string    `json:"raw"`
	CreatedAt   time.Time `json:"created_at"`
	SemKeyCount int       `json:"semkey_count"`
}

// QueryRequest is the POST body for /api/query.
type QueryRequest struct {
	Text string `json:"text"`
	TopK int    `json:"top_k"`
}

// QueryResult is the response from /api/query.
type QueryResult struct {
	EmbedUs    int64 `json:"embed_us"`
	RetrieveUs int64 `json:"retrieve_us"`
	L0Count    int   `json:"l0_count"`
	Tokens     int   `json:"tokens"`
	Results    []Hit `json:"results"`
}

// Hit is a single retrieved memory with its score.
type Hit struct {
	MemoryID int64  `json:"memory_id"`
	Score    int    `json:"score"`
	Raw      string `json:"raw"`
}

func main() {
	indexPath := seminternal.EnvOr("INDEX_PATH", "../semcom_embed/index.bin")
	dbPath := seminternal.EnvOr("DB_PATH", "../semcom_orchestrator/memory.db")
	port := seminternal.EnvOr("PORT", "8081")

	log.Printf("loading index from %s", indexPath)
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

	// Second connection for raw SQL queries (memory listing).
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("open raw db: %v", err)
	}
	defer db.Close()

	srv := &Server{
		idx:        idx,
		thresholds: semindex.Thresholds{L2: 0.25, L1: 0.20, L0: 0.15},
		store:      store,
		retriever:  retriever,
		db:         db,
		benchmarks: &BenchmarkRing{},
	}

	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		log.Fatalf("static fs: %v", err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/api/memories", srv.handleMemories)
	mux.HandleFunc("/api/benchmarks", srv.handleBenchmarks)
	mux.HandleFunc("/api/query", srv.handleQuery)

	httpSrv := &http.Server{Addr: ":" + port, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("dashboard on :%s  (index=%s  db=%s)", port, indexPath, dbPath)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("serve: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down")
	httpSrv.Shutdown(context.Background()) //nolint:errcheck
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

func (s *Server) handleMemories(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT m.id, m.turn_id, m.source, m.raw_message, m.created_at,
		       COUNT(sk.semkey_value) AS semkey_count
		FROM memories m
		LEFT JOIN memory_semkeys sk ON sk.memory_id = m.id
		GROUP BY m.id
		ORDER BY m.id DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	memories := make([]Memory, 0, limit)
	for rows.Next() {
		var m Memory
		var createdAt string
		if err := rows.Scan(&m.ID, &m.TurnID, &m.Source, &m.Raw, &createdAt, &m.SemKeyCount); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		m.CreatedAt, _ = time.Parse("2006-01-02T15:04:05.999Z", createdAt)
		memories = append(memories, m)
	}
	writeJSON(w, memories)
}

func (s *Server) handleBenchmarks(w http.ResponseWriter, r *http.Request) {
	n := 50
	if nStr := r.URL.Query().Get("n"); nStr != "" {
		if v, err := strconv.Atoi(nStr); err == nil && v > 0 && v <= ringSize {
			n = v
		}
	}
	writeJSON(w, s.benchmarks.recent(n))
}

func (s *Server) handleQuery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}

	var req QueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
		http.Error(w, "text required", http.StatusBadRequest)
		return
	}
	if req.TopK <= 0 || req.TopK > 50 {
		req.TopK = 5
	}

	t0 := time.Now()
	stats := s.idx.Query(req.Text, s.thresholds)
	embedUs := time.Since(t0).Microseconds()

	l0IDs := make([]uint32, len(stats.L0IDs))
	for i, id := range stats.L0IDs {
		l0IDs[i] = uint32(id)
	}

	t1 := time.Now()
	retrieved := s.retriever.Query(l0IDs, req.TopK)
	retrieveUs := time.Since(t1).Microseconds()

	hits := make([]Hit, 0, len(retrieved))
	for _, res := range retrieved {
		raw, err := s.store.GetRaw(r.Context(), res.MemoryID)
		if err != nil {
			slog.Error("GetRaw", "memory_id", res.MemoryID, "err", err)
			continue
		}
		hits = append(hits, Hit{MemoryID: res.MemoryID, Score: res.Score, Raw: raw})
	}

	s.benchmarks.add(BenchmarkEntry{
		Op:         "query",
		EmbedUs:    embedUs,
		RetrieveUs: retrieveUs,
		L0Count:    len(stats.L0IDs),
		Tokens:     stats.QueryTokens,
		Timestamp:  time.Now(),
	})

	writeJSON(w, QueryResult{
		EmbedUs:    embedUs,
		RetrieveUs: retrieveUs,
		L0Count:    len(stats.L0IDs),
		Tokens:     stats.QueryTokens,
		Results:    hits,
	})
}
