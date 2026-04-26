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
	SummaryID   *int64
	Source      Source
	Raw         string
	CreatedAt   time.Time
	SemKey      []uint32
	PersonalIDs []uint32
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

	Close() error
}

func Open(path string) (Store, error) {
	return openSQLite(path)
}
