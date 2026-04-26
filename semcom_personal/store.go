package personal

import (
	"database/sql"
	"strings"
)

// Store persists personal tokens and their memory associations.
// It operates on a *sql.DB provided by the caller — no lifecycle ownership.
type Store struct {
	db *sql.DB
}

// NewStore wraps an existing database connection.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

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

func (s *Store) LinkMemory(memoryID int64, personalIDs []uint32) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, pid := range personalIDs {
		if _, err := tx.Exec(
			`INSERT OR IGNORE INTO personal_semkeys (personal_id, memory_id) VALUES (?, ?)`,
			pid, memoryID,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
