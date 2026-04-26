package semanticstore

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
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
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &sqliteStore{db: db}, nil
}

func (s *sqliteStore) Insert(ctx context.Context, m *Memory) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.ExecContext(ctx,
		`INSERT INTO memories (turn_id, source, raw_message) VALUES (?, ?, ?)`,
		m.TurnID, string(m.Source), m.Raw,
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for _, v := range m.SemKey {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO memory_semkeys (memory_id, semkey_value) VALUES (?, ?)`,
			id, v,
		); err != nil {
			return 0, err
		}
	}

	return id, tx.Commit()
}

func (s *sqliteStore) Get(ctx context.Context, id int64) (*Memory, error) {
	m := &Memory{}
	var createdAt string

	err := s.db.QueryRowContext(ctx,
		`SELECT id, turn_id, source, raw_message, created_at FROM memories WHERE id = ?`, id,
	).Scan(&m.ID, &m.TurnID, &m.Source, &m.Raw, &createdAt)
	if err != nil {
		return nil, err
	}

	m.CreatedAt, err = time.Parse("2006-01-02T15:04:05.999Z", createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT semkey_value FROM memory_semkeys WHERE memory_id = ?`, id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var v uint32
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		m.SemKey = append(m.SemKey, v)
	}
	return m, rows.Err()
}

func (s *sqliteStore) AllSemKeys(ctx context.Context) ([]SemKeyRow, error) {
	return s.querySemKeys(ctx, `SELECT semkey_value, memory_id FROM memory_semkeys`)
}

func (s *sqliteStore) SemKeysSince(ctx context.Context, afterID int64) ([]SemKeyRow, error) {
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

func (s *sqliteStore) GetRaw(ctx context.Context, id int64) (string, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, `SELECT raw_message FROM memories WHERE id = ?`, id).Scan(&raw)
	return raw, err
}

func (s *sqliteStore) MaxTurnID(ctx context.Context) (int64, error) {
	var max int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(turn_id), 0) FROM memories`).Scan(&max)
	return max, err
}

func (s *sqliteStore) Close() error {
	return s.db.Close()
}
