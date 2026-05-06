package semcom_session

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestTrackerSchema(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(Schema); err != nil {
		t.Fatalf("execute schema: %v", err)
	}

	// Verify table exists
	var name string
	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='session_retrievals'").Scan(&name)
	if err != nil {
		t.Fatalf("check table: %v", err)
	}
	if name != "session_retrievals" {
		t.Errorf("expected table session_retrievals, got %q", name)
	}
}

func TestNewTracker(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	tracker := NewTracker(db)
	if tracker == nil {
		t.Fatal("expected tracker, got nil")
	}
	if tracker.db != db {
		t.Error("tracker db does not match")
	}
}
