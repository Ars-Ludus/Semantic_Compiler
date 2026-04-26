package semindex

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"log/slog"
	"os"
	"sort"

	"github.com/RoaringBitmap/roaring"
)

// Index holds the in-memory representation ready for querying.
type Index struct {
	words    map[string]int32
	l0       map[int32]*roaring.Bitmap
	l1       map[int32]*roaring.Bitmap
	l2       map[int32]*roaring.Bitmap
	l3       map[int32]*roaring.Bitmap
	l3toL2   map[int32][]int32
	l2toL1   map[int32][]int32
	l1toL0   map[int32][]int32
	l0Tokens map[int32][]int32
}

// Thresholds controls the minimum match ratio (matched / query tokens) at each level.
type Thresholds struct {
	L2 float64
	L1 float64
	L0 float64
}

// Load reads an index.bin produced by Build and returns a query-ready Index.
func Load(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var si SerializedIndex
	if err = gob.NewDecoder(f).Decode(&si); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	idx := &Index{words: si.Words, l3toL2: si.L3toL2, l2toL1: si.L2toL1, l1toL0: si.L1toL0, l0Tokens: si.L0Tokens}
	if idx.l0, err = deserializeBitmaps(si.L0Bitmaps); err != nil {
		return nil, err
	}
	if idx.l1, err = deserializeBitmaps(si.L1Bitmaps); err != nil {
		return nil, err
	}
	if idx.l2, err = deserializeBitmaps(si.L2Bitmaps); err != nil {
		return nil, err
	}
	if idx.l3, err = deserializeBitmaps(si.L3Bitmaps); err != nil {
		return nil, err
	}
	return idx, nil
}

// QueryStats holds results and per-level counts from a query.
type QueryStats struct {
	QueryTokens int
	L3          int
	L2          int
	L1          int
	L0IDs    []int32
	TokenIDs []int32
	OOVWords []string
}

// Query tokenizes text and runs the hierarchical threshold search.
// At each level, only clusters whose match ratio exceeds the threshold pass.
// The query narrows at each level to only the tokens that matched in the
// parent cluster, pruning irrelevant branches early.
func (idx *Index) Query(text string, th Thresholds) QueryStats {
	q, oov := tokenize(text, idx.words)
	if q.IsEmpty() {
		return QueryStats{OOVWords: oov}
	}

	// L3: take top 5 clusters by raw match count
	l3pass := topNByOverlap(q, idx.l3, 5)

	// L2: for each passing L3, narrow query and filter its L2 children
	l2pass := drillDown(l3pass, idx.l3toL2, idx.l2, th.L2)

	// L1
	l1pass := drillDown(l2pass, idx.l2toL1, idx.l1, th.L1)

	// L0
	l0pass := drillDown(l1pass, idx.l1toL0, idx.l0, th.L0)

	result := roaring.New()
	for _, c := range l0pass {
		for _, t := range idx.l0Tokens[c.id] {
			result.Add(uint32(t))
		}
	}

	tokenIDs := make([]int32, 0, result.GetCardinality())
	it := result.Iterator()
	for it.HasNext() {
		tokenIDs = append(tokenIDs, int32(it.Next()))
	}

	l0IDs := make([]int32, len(l0pass))
	for i, c := range l0pass {
		l0IDs[i] = c.id
	}

	return QueryStats{
		QueryTokens: int(q.GetCardinality()),
		L3:          len(l3pass),
		L2:          len(l2pass),
		L1:          len(l1pass),
		L0IDs:    l0IDs,
		TokenIDs: tokenIDs,
		OOVWords: oov,
	}
}

// candidate is a cluster that passed its level's threshold, carrying the
// narrowed query (only the tokens that matched in this branch).
type candidate struct {
	id int32
	q  *roaring.Bitmap // query narrowed to tokens present in this cluster
}

// topNByOverlap returns the top n clusters ranked by raw match count against q.
func topNByOverlap(q *roaring.Bitmap, bitmaps map[int32]*roaring.Bitmap, n int) []candidate {
	type scored struct {
		id      int32
		matched uint64
	}
	scores := make([]scored, 0, len(bitmaps))
	for id, bm := range bitmaps {
		if m := q.AndCardinality(bm); m > 0 {
			scores = append(scores, scored{id, m})
		}
	}
	sort.Slice(scores, func(i, j int) bool { return scores[i].matched > scores[j].matched })
	if n > len(scores) {
		n = len(scores)
	}
	out := make([]candidate, n)
	for i := range out {
		out[i] = candidate{scores[i].id, roaring.And(q, bitmaps[scores[i].id])}
	}
	return out
}

// drillDown takes passing parents, expands to their children via the
// membership map, and filters children using each parent's narrowed query.
// If a child appears under multiple parents, their narrowed queries are
// unioned (giving the child the best chance to pass).
func drillDown(parents []candidate, members map[int32][]int32, childBitmaps map[int32]*roaring.Bitmap, threshold float64) []candidate {
	// Collect per-child: union of narrowed queries from all parents that reach it
	childQueries := make(map[int32]*roaring.Bitmap)
	for _, p := range parents {
		for _, cid := range members[p.id] {
			if existing, ok := childQueries[cid]; ok {
				existing.Or(p.q)
			} else {
				childQueries[cid] = p.q.Clone()
			}
		}
	}

	// Filter children by threshold using their narrowed query
	var out []candidate
	for cid, q := range childQueries {
		bm, ok := childBitmaps[cid]
		if !ok {
			continue
		}
		qCard := float64(q.GetCardinality())
		if qCard == 0 {
			continue
		}
		matched := q.AndCardinality(bm)
		if float64(matched)/qCard >= threshold {
			narrowed := roaring.And(q, bm)
			out = append(out, candidate{cid, narrowed})
		}
	}
	return out
}

func deserializeBitmaps(m map[int32][]byte) (map[int32]*roaring.Bitmap, error) {
	out := make(map[int32]*roaring.Bitmap, len(m))
	for id, b := range m {
		bm := roaring.New()
		if _, err := bm.ReadFrom(bytes.NewReader(b)); err != nil {
			slog.Error("deserialize bitmap", "id", id, "err", err)
			return nil, fmt.Errorf("deserialize bitmap %d: %w", id, err)
		}
		out[id] = bm
	}
	return out, nil
}
