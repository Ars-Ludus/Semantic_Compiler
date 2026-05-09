package distill

import (
	"encoding/json"
	"sort"
	"sync"

	"github.com/RoaringBitmap/roaring"
)

// DistillationResult is a scored distillation candidate.
type DistillationResult struct {
	ID    int32
	Score int
}

// DistillationRetriever holds two in-memory reverse indexes over distillations:
// one for global L0 IDs and one for personal token IDs. Both map to distillation IDs.
// The two indexes are kept separate because L0 IDs and personal token IDs share
// the same integer space and cannot be combined without collision.
// It is safe for concurrent use.
type DistillationRetriever struct {
	mu            sync.RWMutex
	l0Index       map[uint32]*roaring.Bitmap // l0_id → set of distillation_ids
	personalIndex map[uint32]*roaring.Bitmap // personal_id → set of distillation_ids
}

// NewDistillationRetriever builds the initial indexes from all existing distillations.
func NewDistillationRetriever(s *Store) (*DistillationRetriever, error) {
	r := &DistillationRetriever{
		l0Index:       make(map[uint32]*roaring.Bitmap),
		personalIndex: make(map[uint32]*roaring.Bitmap),
	}

	skRows, err := s.db.Query(`SELECT distillation_id, semkey_value FROM distillation_semkeys`)
	if err != nil {
		return nil, err
	}
	defer skRows.Close()
	for skRows.Next() {
		var did int32
		var sk uint32
		if err := skRows.Scan(&did, &sk); err != nil {
			return nil, err
		}
		addToBitmap(r.l0Index, sk, uint32(did))
	}
	if err := skRows.Err(); err != nil {
		return nil, err
	}

	pRows, err := s.db.Query(
		`SELECT id, personal_tokens FROM distillations
		 WHERE personal_tokens IS NOT NULL AND personal_tokens != '' AND personal_tokens != 'null'`)
	if err != nil {
		return nil, err
	}
	defer pRows.Close()
	for pRows.Next() {
		var did int32
		var pJSON string
		if err := pRows.Scan(&did, &pJSON); err != nil {
			return nil, err
		}
		var pIDs []uint32
		if err := json.Unmarshal([]byte(pJSON), &pIDs); err != nil {
			continue
		}
		for _, pid := range pIDs {
			addToBitmap(r.personalIndex, pid, uint32(did))
		}
	}
	return r, pRows.Err()
}

// Add indexes a newly inserted distillation. Call after InsertDistillation.
func (r *DistillationRetriever) Add(id int32, semKeys []uint32, personalIDs []uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, sk := range semKeys {
		addToBitmap(r.l0Index, sk, uint32(id))
	}
	for _, pid := range personalIDs {
		addToBitmap(r.personalIndex, pid, uint32(id))
	}
}

// Query scores all candidate distillations against l0IDs and personalIDs.
// Personal token matches add personalWeight to the score per matched token.
// Distillation IDs present in excludeIDs are skipped; pass nil to disable exclusion.
// Results are returned sorted descending by score.
func (r *DistillationRetriever) Query(l0IDs []uint32, personalIDs []uint32, personalWeight int, excludeIDs *roaring.Bitmap) []DistillationResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	useExclusion := excludeIDs != nil && !excludeIDs.IsEmpty()

	scores := make(map[uint32]int)
	for _, id := range l0IDs {
		bm, ok := r.l0Index[id]
		if !ok {
			continue
		}
		it := bm.Iterator()
		for it.HasNext() {
			did := it.Next()
			if useExclusion && excludeIDs.Contains(did) {
				continue
			}
			scores[did]++
		}
	}
	for _, id := range personalIDs {
		bm, ok := r.personalIndex[id]
		if !ok {
			continue
		}
		it := bm.Iterator()
		for it.HasNext() {
			did := it.Next()
			if useExclusion && excludeIDs.Contains(did) {
				continue
			}
			scores[did] += personalWeight
		}
	}

	results := make([]DistillationResult, 0, len(scores))
	for did, score := range scores {
		results = append(results, DistillationResult{ID: int32(did), Score: score}) //nolint:gosec
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

// GetByPersonalID returns distillation IDs directly associated with the given personal token,
// ordered by ID descending (most recent first).
func (r *DistillationRetriever) GetByPersonalID(id uint32) []int32 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	bm, ok := r.personalIndex[id]
	if !ok {
		return nil
	}
	arr := bm.ToArray() // ascending
	for i, j := 0, len(arr)-1; i < j; i, j = i+1, j-1 {
		arr[i], arr[j] = arr[j], arr[i]
	}
	result := make([]int32, len(arr))
	for i, v := range arr {
		result[i] = int32(v) //nolint:gosec
	}
	return result
}

// Rebuild atomically replaces both indexes by re-scanning the store from scratch.
// Call after DeleteDistillationsBySessionID to evict stale entries from deleted rows.
func (r *DistillationRetriever) Rebuild(s *Store) error {
	newL0 := make(map[uint32]*roaring.Bitmap)
	newPersonal := make(map[uint32]*roaring.Bitmap)

	skRows, err := s.db.Query(`SELECT distillation_id, semkey_value FROM distillation_semkeys`)
	if err != nil {
		return err
	}
	defer skRows.Close()
	for skRows.Next() {
		var did int32
		var sk uint32
		if err := skRows.Scan(&did, &sk); err != nil {
			return err
		}
		addToBitmap(newL0, sk, uint32(did))
	}
	if err := skRows.Err(); err != nil {
		return err
	}

	pRows, err := s.db.Query(
		`SELECT id, personal_tokens FROM distillations
		 WHERE personal_tokens IS NOT NULL AND personal_tokens != '' AND personal_tokens != 'null'`)
	if err != nil {
		return err
	}
	defer pRows.Close()
	for pRows.Next() {
		var did int32
		var pJSON string
		if err := pRows.Scan(&did, &pJSON); err != nil {
			return err
		}
		var pIDs []uint32
		if err := json.Unmarshal([]byte(pJSON), &pIDs); err != nil {
			continue
		}
		for _, pid := range pIDs {
			addToBitmap(newPersonal, pid, uint32(did))
		}
	}
	if err := pRows.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	r.l0Index = newL0
	r.personalIndex = newPersonal
	r.mu.Unlock()
	return nil
}

func addToBitmap(index map[uint32]*roaring.Bitmap, key, value uint32) {
	bm, ok := index[key]
	if !ok {
		bm = roaring.New()
		index[key] = bm
	}
	bm.Add(value)
}
