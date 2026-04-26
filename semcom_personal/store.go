package personal

import (
	"database/sql"
	"encoding/json"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type Distillation struct {
	ID          int64
	Topic       string
	Snippet     string
	PersonalIDs []uint32
	SemKeys     []uint32
}

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(Schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// Token Methods

func (s *Store) InsertToken(word, t string) (uint32, error) {
	word = strings.ToLower(word)
	res, err := s.db.Exec(`INSERT INTO personal_tokens (word, type) VALUES (?, ?)`, word, t)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return uint32(id), nil
}

func (s *Store) GetToken(word string) (*Token, error) {
	word = strings.ToLower(word)
	row := s.db.QueryRow(`SELECT id, word, type FROM personal_tokens WHERE word = ?`, word)
	var t Token
	if err := row.Scan(&t.ID, &t.Word, &t.Type); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) GetAllTokens() (map[string]uint32, error) {
	rows, err := s.db.Query(`SELECT word, id FROM personal_tokens`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tokens := make(map[string]uint32)
	for rows.Next() {
		var word string
		var id uint32
		if err := rows.Scan(&word, &id); err != nil {
			return nil, err
		}
		tokens[strings.ToLower(word)] = id
	}
	return tokens, nil
}

// Link Methods

func (s *Store) LinkMemory(memoryID int64, personalIDs []uint32) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, pid := range personalIDs {
		_, err := tx.Exec(`INSERT OR IGNORE INTO personal_semkeys (personal_id, memory_id) VALUES (?, ?)`, pid, memoryID)
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Distillation Methods

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

	res, err := tx.Exec(`INSERT INTO distillations (topic, snippet, personal_tokens) VALUES (?, ?, ?)`,
		d.Topic, d.Snippet, string(pIDsJSON))
	if err != nil {
		return 0, err
	}

	distillID, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	for _, sk := range d.SemKeys {
		_, err := tx.Exec(`INSERT INTO distillation_semkeys (distillation_id, semkey_value) VALUES (?, ?)`,
			distillID, sk)
		if err != nil {
			return 0, err
		}
	}

	return distillID, tx.Commit()
}

// Metadata Methods

func (s *Store) GetMetadata(key string) (string, error) {
	var val string
	err := s.db.QueryRow(`SELECT value FROM metadata WHERE key = ?`, key).Scan(&val)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return val, err
}

func (s *Store) SetMetadata(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO metadata (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value)
	return err
}
