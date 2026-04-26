package semanticstore

import (
	"context"
	"database/sql"
	"encoding/json"
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

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &sqliteStore{db: db}, nil
}

func migrate(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(memories)")
	if err != nil {
		return err
	}
	defer rows.Close()

	hasPersonalTokens := false
	hasDiscovered := false
	for rows.Next() {
		var cid int
		var name string
		var dtype string
		var notnull int
		var dfltValue any
		var pk int
		if err := rows.Scan(&cid, &name, &dtype, &notnull, &dfltValue, &pk); err != nil {
			return err
		}
		if name == "personal_tokens" {
			hasPersonalTokens = true
		}
		if name == "discovered" {
			hasDiscovered = true
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	if !hasPersonalTokens {
		if _, err := db.Exec("ALTER TABLE memories ADD COLUMN personal_tokens TEXT"); err != nil {
			return err
		}
	}
	if !hasDiscovered {
		if _, err := db.Exec("ALTER TABLE memories ADD COLUMN discovered INTEGER NOT NULL DEFAULT 0"); err != nil {
			return err
		}
	}
	return nil
}

func (s *sqliteStore) Insert(ctx context.Context, m *Memory) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var personalIDsJSON sql.NullString
	if m.PersonalIDs != nil {
		b, err := json.Marshal(m.PersonalIDs)
		if err != nil {
			return 0, fmt.Errorf("marshal personal IDs: %w", err)
		}
		personalIDsJSON = sql.NullString{String: string(b), Valid: true}
	}

	disc := 0
	if m.Discovered {
		disc = 1
	}

	res, err := tx.ExecContext(ctx,
		`INSERT INTO memories (turn_id, source, raw_message, personal_tokens, discovered) VALUES (?, ?, ?, ?, ?)`,
		m.TurnID, string(m.Source), m.Raw, personalIDsJSON, disc,
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

func (s *sqliteStore) scanMemory(rows *sql.Rows) (*Memory, error) {
	m := &Memory{}
	var createdAt string
	var personalIDsJSON sql.NullString
	var disc int

	err := rows.Scan(&m.ID, &m.TurnID, &m.Source, &m.Raw, &personalIDsJSON, &disc, &createdAt)
	if err != nil {
		return nil, err
	}

	if personalIDsJSON.Valid && personalIDsJSON.String != "" {
		if err := json.Unmarshal([]byte(personalIDsJSON.String), &m.PersonalIDs); err != nil {
			return nil, fmt.Errorf("unmarshal personal IDs: %w", err)
		}
	}

	m.Discovered = disc == 1

	m.CreatedAt, err = time.Parse("2006-01-02T15:04:05.999Z", createdAt)
	if err != nil {
		return nil, fmt.Errorf("parse created_at: %w", err)
	}
	return m, nil
}

func (s *sqliteStore) UnprocessedMemories(ctx context.Context) ([]*Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, turn_id, source, raw_message, personal_tokens, discovered, created_at FROM memories WHERE discovered = 0`,
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

func (s *sqliteStore) MarkMemoryDiscovered(ctx context.Context, memoryID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE memories SET discovered = 1 WHERE id = ?`, memoryID)
	return err
}

func (s *sqliteStore) GetChunk(ctx context.Context, startID, endID int64) ([]*Memory, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, turn_id, source, raw_message, personal_tokens, discovered, created_at FROM memories WHERE id >= ? AND id <= ? ORDER BY id ASC`,
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
		`SELECT id, turn_id, source, raw_message, personal_tokens, discovered, created_at FROM memories WHERE raw_message LIKE ?`,
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

func (s *sqliteStore) UpdateMemoryPersonalTokens(ctx context.Context, memoryID int64, personalIDs []uint32) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	b, err := json.Marshal(personalIDs)
	if err != nil {
		return err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE memories SET personal_tokens = ? WHERE id = ?`,
		string(b), memoryID,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (s *sqliteStore) Get(ctx context.Context, id int64) (*Memory, error) {
	m := &Memory{}
	var createdAt string
	var personalIDsJSON sql.NullString
	var disc int

	err := s.db.QueryRowContext(ctx,
		`SELECT id, turn_id, source, raw_message, personal_tokens, discovered, created_at FROM memories WHERE id = ?`, id,
	).Scan(&m.ID, &m.TurnID, &m.Source, &m.Raw, &personalIDsJSON, &disc, &createdAt)
	if err != nil {
		return nil, err
	}

	if personalIDsJSON.Valid && personalIDsJSON.String != "" {
		if err := json.Unmarshal([]byte(personalIDsJSON.String), &m.PersonalIDs); err != nil {
			return nil, fmt.Errorf("unmarshal personal IDs: %w", err)
		}
	}

	m.Discovered = disc == 1

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
