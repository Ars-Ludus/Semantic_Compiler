package personal

import (
	"database/sql"
	"strings"
)

// Store persists personal tokens.
// It operates on a *sql.DB provided by the caller — no lifecycle ownership.
type Store struct {
	db *sql.DB
}

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

// MemoryLink is a single row from memory_personal_tokens.
type MemoryLink struct {
	MemoryID   int32
	PersonalID uint32
}

// LinkMemory records that a memory matched a set of personal tokens.
func (s *Store) LinkMemory(memoryID int32, personalIDs []uint32) error {
	if len(personalIDs) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO memory_personal_tokens (memory_id, personal_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, pid := range personalIDs {
		if _, err := stmt.Exec(memoryID, pid); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetAllLinks loads all memory→personal token associations for retriever initialization.
func (s *Store) GetAllLinks() ([]MemoryLink, error) {
	rows, err := s.db.Query(`SELECT memory_id, personal_id FROM memory_personal_tokens`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var links []MemoryLink
	for rows.Next() {
		var l MemoryLink
		if err := rows.Scan(&l.MemoryID, &l.PersonalID); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	return links, rows.Err()
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
