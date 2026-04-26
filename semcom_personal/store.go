package personal

import (
	"database/sql"
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
	row := s.db.QueryRow(`SELECT id, word, type FROM personal_tokens WHERE word = ?`, word)
	var t Token
	if err := row.Scan(&t.ID, &t.Word, &t.Type); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) AddIgnore(word string) error {
	_, err := s.db.Exec(`INSERT OR IGNORE INTO personal_ignore (word) VALUES (?)`, word)
	return err
}

func (s *Store) IsIgnored(word string) (bool, error) {
	var exists bool
	err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM personal_ignore WHERE word = ?)`, word).Scan(&exists)
	return exists, err
}
