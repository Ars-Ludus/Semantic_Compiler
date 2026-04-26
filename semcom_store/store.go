package semanticstore

import (
	"context"
	"time"
)

type Source string

const (
	SourceUser  Source = "user"
	SourceModel Source = "model"
)

type Memory struct {
	ID          int64
	TurnID      int64
	Source      Source
	Raw         string
	CreatedAt   time.Time
	SemKey      []uint32
	PersonalIDs []uint32
	Discovered  bool
}

// SemKeyRow is a (semkey value, memory ID) pair used by the retrieval layer
// to build or update a roaring bitmap reverse index.
type SemKeyRow struct {
	Value    uint32
	MemoryID int64
}

type Store interface {
	Insert(ctx context.Context, m *Memory) (int64, error)
	Get(ctx context.Context, id int64) (*Memory, error)
	// GetRaw returns only the raw_message for the given id.
	GetRaw(ctx context.Context, id int64) (string, error)

	// AllSemKeys returns every (value, memory_id) pair for a full index rebuild.
	AllSemKeys(ctx context.Context) ([]SemKeyRow, error)

	// SemKeysSince returns pairs where memory_id > afterID for incremental append.
	SemKeysSince(ctx context.Context, afterID int64) ([]SemKeyRow, error)

	// MaxTurnID returns the highest turn_id stored, or 0 if no rows exist.
	MaxTurnID(ctx context.Context) (int64, error)

	// UnprocessedMemories returns all memories that haven't been processed by discovery.
	UnprocessedMemories(ctx context.Context) ([]*Memory, error)

	// MarkMemoryDiscovered sets the discovered flag for a memory.
	MarkMemoryDiscovered(ctx context.Context, memoryID int64) error

	// GetChunk returns memories between startID and endID (inclusive).
	GetChunk(ctx context.Context, startID, endID int64) ([]*Memory, error)

	// MemoriesContainingWord returns memories where raw_message contains the word (case-insensitive LIKE).
	MemoriesContainingWord(ctx context.Context, word string) ([]*Memory, error)

	// UpdateMemoryPersonalTokens updates the personal_tokens column and associated semkeys index.
	UpdateMemoryPersonalTokens(ctx context.Context, memoryID int64, personalIDs []uint32) error

	Close() error
}

func Open(path string) (Store, error) {
	return openSQLite(path)
}
