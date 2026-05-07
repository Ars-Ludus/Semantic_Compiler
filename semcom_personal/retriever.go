package personal

import (
	"sync"

	"github.com/RoaringBitmap/roaring"
)

// PersonalRetriever holds an in-memory reverse index from personal token IDs to memory IDs.
// It is kept separate from the L0 semkey index — personal IDs and L0 IDs share the same
// integer space and must never be combined in a single bitmap map.
// It is safe for concurrent use.
type PersonalRetriever struct {
	mu    sync.RWMutex
	index map[uint32]*roaring.Bitmap // personal_id → set of memory_ids
}

// NewPersonalRetriever loads all existing memory→personal links from the store.
func NewPersonalRetriever(s *Store) (*PersonalRetriever, error) {
	r := &PersonalRetriever{
		index: make(map[uint32]*roaring.Bitmap),
	}
	links, err := s.GetAllLinks()
	if err != nil {
		return nil, err
	}
	for _, l := range links {
		addToBitmap(r.index, l.PersonalID, uint32(l.MemoryID))
	}
	return r, nil
}

// AddLink records that memoryID matched personalID.
func (r *PersonalRetriever) AddLink(personalID uint32, memoryID int32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	addToBitmap(r.index, personalID, uint32(memoryID))
}

// MemoryTokenCounts returns a map of memory_id → personal token hit count for the given personal IDs.
// excludeIDs is an optional bitmap of memory IDs to ignore.
func (r *PersonalRetriever) MemoryTokenCounts(personalIDs []uint32, excludeIDs *roaring.Bitmap) map[int32]int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	counts := make(map[int32]int)
	useExclusion := excludeIDs != nil && !excludeIDs.IsEmpty()
	for _, pid := range personalIDs {
		bm, ok := r.index[pid]
		if !ok {
			continue
		}

		var finalBM *roaring.Bitmap
		if useExclusion {
			finalBM = roaring.AndNot(bm, excludeIDs)
		} else {
			finalBM = bm
		}

		it := finalBM.Iterator()
		for it.HasNext() {
			counts[int32(it.Next())]++ //nolint:gosec
		}
	}
	return counts
}

func addToBitmap(index map[uint32]*roaring.Bitmap, key, value uint32) {
	bm, ok := index[key]
	if !ok {
		bm = roaring.New()
		index[key] = bm
	}
	bm.Add(value)
}
