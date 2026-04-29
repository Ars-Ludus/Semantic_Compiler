package semcomretrieve

import (
	"context"
	"sort"
	"sync"

	"github.com/RoaringBitmap/roaring"
	semanticstore "github.com/ars/semantic_store"
)

// Result is a single ranked match.
type Result struct {
	MemoryID int32
	Score    int // count of shared l0_ids with query
}

// Retriever holds the in-memory reverse index over memory semkeys.
// It is safe for concurrent use.
type Retriever struct {
	store  semanticstore.Store
	mu     sync.RWMutex
	index  map[uint32]*roaring.Bitmap // l0_id → set of memory_ids
	lastID int32
}

// Open builds an initial full index from store.
func Open(store semanticstore.Store) (*Retriever, error) {
	r := &Retriever{
		store: store,
		index: make(map[uint32]*roaring.Bitmap),
	}

	rows, err := store.AllSemKeys(context.Background())
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		bm, ok := r.index[row.Value]
		if !ok {
			bm = roaring.New()
			r.index[row.Value] = bm
		}
		bm.Add(uint32(row.MemoryID))
		if row.MemoryID > r.lastID {
			r.lastID = row.MemoryID
		}
	}

	return r, nil
}

// Refresh pulls rows added since the last refresh and patches the index.
// It is safe to call concurrently with Query.
func (r *Retriever) Refresh(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rows, err := r.store.SemKeysSince(ctx, r.lastID)
	if err != nil {
		return err
	}
	for _, row := range rows {
		bm, ok := r.index[row.Value]
		if !ok {
			bm = roaring.New()
			r.index[row.Value] = bm
		}
		bm.Add(uint32(row.MemoryID))
		if row.MemoryID > r.lastID {
			r.lastID = row.MemoryID
		}
	}
	return nil
}

// Query scores all indexed memories against queryL0IDs and returns the top-k
// results sorted descending by score. Pass k=0 to return all matches.
func (r *Retriever) Query(queryL0IDs []uint32, k int) []Result {
	r.mu.RLock()
	defer r.mu.RUnlock()

	scores := make(map[uint32]uint8, 64)
	for _, l0id := range queryL0IDs {
		bm, ok := r.index[l0id]
		if !ok {
			continue
		}
		it := bm.Iterator()
		for it.HasNext() {
			scores[it.Next()]++
		}
	}

	if len(scores) == 0 {
		return nil
	}

	results := make([]Result, 0, len(scores))
	for memID, score := range scores {
		results = append(results, Result{MemoryID: int32(memID), Score: int(score)}) //nolint:gosec
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	if k > 0 && k < len(results) {
		results = results[:k]
	}
	return results
}
