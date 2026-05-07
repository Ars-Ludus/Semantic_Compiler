package distill

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/json"
	"strings"
)

//go:embed schema.sql
var Schema string

// Store persists distillations and tracks pass progress via metadata.
// It operates on a *sql.DB provided by the caller — no lifecycle ownership.
type Store struct {
	db *sql.DB
}

// Distillation is a compressed knowledge snippet extracted from a conversation chunk.
type Distillation struct {
	ID          int32
	Topic       string
	Snippet     string
	SessionID   string   // originating session; empty for legacy chunk distillations
	PersonalIDs []uint32
	SemKeys     []uint32
}

// NewStore wraps an existing database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// InsertDistillation stores a distillation and its semkey associations atomically.
func (s *Store) InsertDistillation(d *Distillation) (int32, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var pIDsJSON []byte
	if d.PersonalIDs != nil {
		pIDsJSON, _ = json.Marshal(d.PersonalIDs)
	}

	res, err := tx.Exec(
		`INSERT INTO distillations (topic, snippet, personal_tokens, session_id) VALUES (?, ?, ?, ?)`,
		d.Topic, d.Snippet, string(pIDsJSON), d.SessionID,
	)
	if err != nil {
		return 0, err
	}
	id64, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	id := int32(id64) //nolint:gosec // IDs stay well within int32 range

	for _, sk := range d.SemKeys {
		if _, err := tx.Exec(
			`INSERT INTO distillation_semkeys (distillation_id, semkey_value) VALUES (?, ?)`,
			id, sk,
		); err != nil {
			return 0, err
		}
	}

	return id, tx.Commit()
}

// GetDistillationsByIDs returns the topic and snippet for each requested ID.
// Used by the budget fill step to fetch text for top-scored candidates.
func (s *Store) GetDistillationsByIDs(ctx context.Context, ids []int32) ([]*Distillation, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, topic, snippet FROM distillations WHERE id IN ("+
			strings.Join(placeholders, ",")+")",
		args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Distillation
	for rows.Next() {
		d := &Distillation{}
		if err := rows.Scan(&d.ID, &d.Topic, &d.Snippet); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// AllDistillations returns every distillation with its semkeys and personal IDs populated.
func (s *Store) AllDistillations(ctx context.Context) ([]*Distillation, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, topic, snippet, personal_tokens FROM distillations ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []*Distillation
	index := make(map[int32]int)
	for rows.Next() {
		d := &Distillation{}
		var pJSON sql.NullString
		if err := rows.Scan(&d.ID, &d.Topic, &d.Snippet, &pJSON); err != nil {
			return nil, err
		}
		if pJSON.Valid && pJSON.String != "" && pJSON.String != "null" {
			json.Unmarshal([]byte(pJSON.String), &d.PersonalIDs)
		}
		index[d.ID] = len(all)
		all = append(all, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	skRows, err := s.db.QueryContext(ctx,
		`SELECT distillation_id, semkey_value FROM distillation_semkeys`)
	if err != nil {
		return nil, err
	}
	defer skRows.Close()

	for skRows.Next() {
		var dID int32
		var sk uint32
		if err := skRows.Scan(&dID, &sk); err != nil {
			return nil, err
		}
		if i, ok := index[dID]; ok {
			all[i].SemKeys = append(all[i].SemKeys, sk)
		}
	}
	return all, skRows.Err()
}

// DeleteDistillationsBySessionID removes all distillations for the given session.
// Semkey rows are removed automatically via ON DELETE CASCADE.
func (s *Store) DeleteDistillationsBySessionID(ctx context.Context, sessionID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM distillations WHERE session_id = ?`, sessionID)
	return err
}

// GetMetadata returns the value for key, or "" if not set.
func (s *Store) GetMetadata(key string) (string, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM distill_metadata WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

// SetMetadata upserts a key/value pair.
func (s *Store) SetMetadata(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO distill_metadata (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}
