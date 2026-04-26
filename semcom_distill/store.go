package distill

import (
	"database/sql"
	_ "embed"
	"encoding/json"
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
	ID          int64
	Topic       string
	Snippet     string
	PersonalIDs []uint32
	SemKeys     []uint32
}

// NewStore wraps an existing database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// InsertDistillation stores a distillation and its semkey associations atomically.
func (s *Store) InsertDistillation(d *Distillation) (int64, error) {
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
		`INSERT INTO distillations (topic, snippet, personal_tokens) VALUES (?, ?, ?)`,
		d.Topic, d.Snippet, string(pIDsJSON),
	)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

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
