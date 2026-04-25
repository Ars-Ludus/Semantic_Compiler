package semindex

import (
	"bytes"
	"context"
	"encoding/gob"
	"fmt"
	"os"
	"github.com/RoaringBitmap/roaring"
	"github.com/jackc/pgx/v5"
)

// SerializedIndex is the on-disk representation of the index.
type SerializedIndex struct {
	Words map[string]int32

	L0Bitmaps map[int32][]byte
	L1Bitmaps map[int32][]byte
	L2Bitmaps map[int32][]byte
	L3Bitmaps map[int32][]byte

	L3toL2   map[int32][]int32
	L2toL1   map[int32][]int32
	L1toL0   map[int32][]int32
	L0Tokens map[int32][]int32
}

// Build connects to PostgreSQL, loads all tables, computes bitmaps, and
// writes the serialized index to outPath.
func Build(dsn, outPath string) error {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer conn.Close(ctx)

	fmt.Println("loading tokenizer...")
	words, err := loadWords(ctx, conn)
	if err != nil {
		return err
	}
	fmt.Printf("  %d words\n", len(words))

	fmt.Println("loading l0...")
	l0Tokens, err := loadL0(ctx, conn)
	if err != nil {
		return err
	}
	fmt.Printf("  %d clusters\n", len(l0Tokens))

	fmt.Println("loading l1...")
	l1toL0, err := loadMembers(ctx, conn, "l1", "l0_members")
	if err != nil {
		return err
	}
	fmt.Printf("  %d clusters\n", len(l1toL0))

	fmt.Println("loading l2...")
	l2toL1, err := loadMembers(ctx, conn, "l2", "l1_members")
	if err != nil {
		return err
	}
	fmt.Printf("  %d clusters\n", len(l2toL1))

	fmt.Println("loading l3...")
	l3toL2, err := loadMembers(ctx, conn, "l3", "l2_members")
	if err != nil {
		return err
	}
	fmt.Printf("  %d clusters\n", len(l3toL2))

	fmt.Println("building bitmaps...")
	l0bm := buildL0Bitmaps(l0Tokens)
	l1bm := buildUpperBitmaps(l1toL0, l0bm)
	l2bm := buildUpperBitmaps(l2toL1, l1bm)
	l3bm := buildUpperBitmaps(l3toL2, l2bm)

	fmt.Println("serializing...")
	si := SerializedIndex{
		Words:    words,
		L0Bitmaps: serializeBitmaps(l0bm),
		L1Bitmaps: serializeBitmaps(l1bm),
		L2Bitmaps: serializeBitmaps(l2bm),
		L3Bitmaps: serializeBitmaps(l3bm),
		L3toL2:   l3toL2,
		L2toL1:   l2toL1,
		L1toL0:   l1toL0,
		L0Tokens: l0Tokens,
	}

	f, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("create %s: %w", outPath, err)
	}
	defer f.Close()

	if err := gob.NewEncoder(f).Encode(si); err != nil {
		return fmt.Errorf("encode: %w", err)
	}
	fmt.Printf("wrote %s\n", outPath)
	return nil
}

func loadWords(ctx context.Context, conn *pgx.Conn) (map[string]int32, error) {
	rows, err := conn.Query(ctx, "SELECT token_id, lower(word) FROM tokenizer")
	if err != nil {
		return nil, fmt.Errorf("query tokenizer: %w", err)
	}
	defer rows.Close()

	m := make(map[string]int32)
	for rows.Next() {
		var id int32
		var word string
		if err := rows.Scan(&id, &word); err != nil {
			return nil, err
		}
		m[word] = id
	}
	return m, rows.Err()
}

func loadL0(ctx context.Context, conn *pgx.Conn) (map[int32][]int32, error) {
	rows, err := conn.Query(ctx, "SELECT l0_id, tokens FROM l0")
	if err != nil {
		return nil, fmt.Errorf("query l0: %w", err)
	}
	defer rows.Close()

	m := make(map[int32][]int32)
	for rows.Next() {
		var id int32
		var tokens []int32
		if err := rows.Scan(&id, &tokens); err != nil {
			return nil, err
		}
		m[id] = tokens
	}
	return m, rows.Err()
}

func loadMembers(ctx context.Context, conn *pgx.Conn, table, col string) (map[int32][]int32, error) {
	// e.g. "SELECT l1_id, l0_members FROM l1"
	q := fmt.Sprintf("SELECT %s_id, %s FROM %s", table, col, table)
	rows, err := conn.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query %s: %w", table, err)
	}
	defer rows.Close()

	m := make(map[int32][]int32)
	for rows.Next() {
		var id int32
		var members []int32
		if err := rows.Scan(&id, &members); err != nil {
			return nil, err
		}
		m[id] = members
	}
	return m, rows.Err()
}

// buildL0Bitmaps creates one bitmap per l0 cluster from its token list.
func buildL0Bitmaps(l0Tokens map[int32][]int32) map[int32]*roaring.Bitmap {
	out := make(map[int32]*roaring.Bitmap, len(l0Tokens))
	for id, tokens := range l0Tokens {
		bm := roaring.New()
		for _, t := range tokens {
			bm.Add(uint32(t))
		}
		out[id] = bm
	}
	return out
}

// buildUpperBitmaps creates one bitmap per cluster at a higher level by
// unioning the bitmaps of all child members.
func buildUpperBitmaps(members map[int32][]int32, childBitmaps map[int32]*roaring.Bitmap) map[int32]*roaring.Bitmap {
	out := make(map[int32]*roaring.Bitmap, len(members))
	for id, childIDs := range members {
		bm := roaring.New()
		for _, cid := range childIDs {
			if child, ok := childBitmaps[cid]; ok {
				bm.Or(child)
			}
		}
		out[id] = bm
	}
	return out
}

func serializeBitmaps(m map[int32]*roaring.Bitmap) map[int32][]byte {
	out := make(map[int32][]byte, len(m))
	for id, bm := range m {
		var buf bytes.Buffer
		if _, err := bm.WriteTo(&buf); err != nil {
			panic(err) // bitmaps always serialize cleanly
		}
		out[id] = buf.Bytes()
	}
	return out
}
