package semcom_session

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/RoaringBitmap/roaring"
	_ "github.com/mattn/go-sqlite3"
)

func TestGetRetrievedIDs(t *testing.T) {
	dbPath := "test_tracker.db"
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Initialize schema
	if _, err := db.Exec(Schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	// Insert test data
	sessionID := "test-session"
	docIDs := []uint32{1, 5, 10}
	for _, id := range docIDs {
		_, err := db.Exec("INSERT INTO session_retrievals (session_id, memory_id) VALUES (?, ?)", sessionID, id)
		if err != nil {
			t.Fatalf("failed to insert test data: %v", err)
		}
	}

	tracker := &Tracker{db: db}
	
	ctx := context.Background()
	bitmap, err := tracker.GetRetrievedIDs(ctx, sessionID)
	if err != nil {
		t.Errorf("GetRetrievedIDs failed: %v", err)
	}

	var _ *roaring.Bitmap = bitmap // Explicitly use roaring package

	if bitmap.GetCardinality() != uint64(len(docIDs)) {
		t.Errorf("expected cardinality %d, got %d", len(docIDs), bitmap.GetCardinality())
	}

	for _, id := range docIDs {
		if !bitmap.Contains(id) {
			t.Errorf("expected bitmap to contain %d", id)
		}
	}
}
