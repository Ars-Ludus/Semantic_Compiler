package semanticstore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schema string

type sqliteStore struct {
	db *sql.DB
}

func openSQLite(path string) (*sqliteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	if _, err := db.Exec(`ALTER TABLE memories ADD COLUMN session_id TEXT`); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			db.Close()
			return nil, fmt.Errorf("migrate memories.session_id: %w", err)
		}
	}
	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) Insert(ctx context.Context, m *Memory) (int32, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO memories (turn_id, source, raw_message, session_id) VALUES (?, ?, ?, ?)`,
		m.TurnID, string(m.Source), m.Raw, m.SessionID,
	)
	if err != nil {
		return 0, err
	}
	id64, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for _, v := range m.SemKey {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO memory_semkeys (memory_id, semkey_value) VALUES (?, ?)`,
			id64, v,
		); err != nil {
			return 0, err
		}
	}

	return int32(id64), tx.Commit() //nolint:gosec // IDs stay well within int32 range
}

func (s *sqliteStore) scanMemory(rows *sql.Rows) (*Memory, error) {
	m := &Memory{}
	var createdAt string
	if err := rows.Scan(&m.ID, &m.TurnID, &m.Source, &m.Raw, &createdAt); err != nil {
		return nil, err
	}
	var err error
	m.CreatedAt, err = time.Parse("2006-01-02T15:04:05.999Z", createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return m, nil
}

func (s *sqliteStore) Get(ctx context.Context, id int32) (*Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, turn_id, source, raw_message, created_at FROM memories WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	m, err := s.scanMemory(rows)
	if err != nil {
		return nil, err
	}

	skRows, err := s.db.QueryContext(ctx,
		`SELECT semkey_value FROM memory_semkeys WHERE memory_id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer skRows.Close()
	for skRows.Next() {
		var v uint32
		if err := skRows.Scan(&v); err != nil {
			return nil, err
		}
		m.SemKey = append(m.SemKey, v)
	}
	return m, skRows.Err()
}

func (s *sqliteStore) GetIDsBySessionID(ctx context.Context, sessionID string) ([]int32, error) {
	if sessionID == "" {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM memories WHERE session_id = ?`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []int32
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *sqliteStore) GetDistinctSessionIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT session_id FROM memories
		 WHERE session_id IS NOT NULL AND session_id != ''
		 GROUP BY session_id
		 ORDER BY MIN(id) ASC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *sqliteStore) GetMemoriesBySessionID(ctx context.Context, sessionID string) ([]*Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, turn_id, source, raw_message, created_at FROM memories
		 WHERE session_id = ? ORDER BY id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		m, err := s.scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *sqliteStore) GetChunk(ctx context.Context, startID, endID int32) ([]*Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, turn_id, source, raw_message, created_at FROM memories WHERE id >= ? AND id <= ? ORDER BY id ASC`,
		startID, endID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		m, err := s.scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *sqliteStore) MemoriesContainingWord(ctx context.Context, word string) ([]*Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, turn_id, source, raw_message, created_at FROM memories WHERE raw_message LIKE ?`,
		"%"+word+"%",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var memories []*Memory
	for rows.Next() {
		m, err := s.scanMemory(rows)
		if err != nil {
			return nil, err
		}
		memories = append(memories, m)
	}
	return memories, rows.Err()
}

func (s *sqliteStore) AllSemKeys(ctx context.Context) ([]SemKeyRow, error) {
	return s.querySemKeys(ctx, `SELECT semkey_value, memory_id FROM memory_semkeys`)
}

func (s *sqliteStore) SemKeysSince(ctx context.Context, afterID int32) ([]SemKeyRow, error) {
	return s.querySemKeys(ctx,
		`SELECT semkey_value, memory_id FROM memory_semkeys WHERE memory_id > ?`, afterID,
	)
}

func (s *sqliteStore) querySemKeys(ctx context.Context, query string, args ...any) ([]SemKeyRow, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SemKeyRow
	for rows.Next() {
		var r SemKeyRow
		if err := rows.Scan(&r.Value, &r.MemoryID); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *sqliteStore) GetRaw(ctx context.Context, id int32) (string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT raw_message FROM memories WHERE id = ?`, id).Scan(&raw)
	return raw, err
}

func (s *sqliteStore) MaxTurnID(ctx context.Context) (int32, error) {
	var max int32
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(turn_id), 0) FROM memories`).Scan(&max)
	return max, err
}

func (s *sqliteStore) MaxID(ctx context.Context) (int32, error) {
	var max int32
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM memories`).Scan(&max)
	return max, err
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}
