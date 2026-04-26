package personal

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestSchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(Schema); err != nil {
		t.Fatalf("failed to execute schema: %v", err)
	}
}
