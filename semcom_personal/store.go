package personal

import (
	"database/sql"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
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

func (s *Store) AddIgnore(word string) error {
	word = strings.ToLower(word)
	_, err := s.db.Exec(`INSERT OR IGNORE INTO personal_ignore (word) VALUES (?)`, word)
	return err
}

func (s *Store) IsIgnored(word string) (bool, error) {
	word = strings.ToLower(word)
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM personal_ignore WHERE word = ?)`, word).Scan(&exists)
	return exists, err
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

func (s *Store) GetAllIgnore() (map[string]struct{}, error) {
	rows, err := s.db.Query(`SELECT word FROM personal_ignore`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ignore := make(map[string]struct{})
	for rows.Next() {
		var word string
		if err := rows.Scan(&word); err != nil {
			return nil, err
		}
		ignore[strings.ToLower(word)] = struct{}{}
	}
	return ignore, nil
}
