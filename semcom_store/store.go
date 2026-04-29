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
	ID        int32
	TurnID    int32
	Source    Source
	Raw       string
	CreatedAt time.Time
	SemKey    []uint32
}

// SemKeyRow is a (semkey value, memory ID) pair used by the retrieval layer
// to build or update a roaring bitmap reverse index.
type SemKeyRow struct {
	Value    uint32
	MemoryID int32
}

type Store interface {
	Insert(ctx context.Context, m *Memory) (int32, error)
	Get(ctx context.Context, id int32) (*Memory, error)
	GetRaw(ctx context.Context, id int32) (string, error)

	AllSemKeys(ctx context.Context) ([]SemKeyRow, error)
	SemKeysSince(ctx context.Context, afterID int32) ([]SemKeyRow, error)

	MaxTurnID(ctx context.Context) (int32, error)
	MaxID(ctx context.Context) (int32, error)

	GetChunk(ctx context.Context, startID, endID int32) ([]*Memory, error)
	MemoriesContainingWord(ctx context.Context, word string) ([]*Memory, error)

	Close() error
}

func Open(path string) (Store, error) {
	return openSQLite(path)
}
