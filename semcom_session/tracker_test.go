package semcom_session

import (
	"context"
	"database/sql"
	"os"
	"testing"

	"github.com/RoaringBitmap/roaring"
	_ "modernc.org/sqlite"
)

func TestGetRetrievedIDs(t *testing.T) {
	dbPath := "test_tracker.db"
	os.Remove(dbPath)
	defer os.Remove(dbPath)

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Initialize schema
	if _, err := db.Exec(Schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	tracker := &Tracker{db: db}
	ctx := context.Background()

	t.Run("ValidSession", func(t *testing.T) {
		sessionID := "test-session"
		docIDs := []uint32{1, 5, 10}
		for _, id := range docIDs {
			_, err := db.Exec("INSERT INTO session_retrievals (session_id, memory_id) VALUES (?, ?)", sessionID, id)
			if err != nil {
				t.Fatalf("failed to insert test data: %v", err)
			}
		}

		bitmap := tracker.GetRetrievedIDs(ctx, sessionID)
		var _ *roaring.Bitmap = bitmap // Ensure roaring is used

		if bitmap.GetCardinality() != uint64(len(docIDs)) {
			t.Errorf("expected cardinality %d, got %d", len(docIDs), bitmap.GetCardinality())
		}

		for _, id := range docIDs {
			if !bitmap.Contains(id) {
				t.Errorf("expected bitmap to contain %d", id)
			}
		}
	})

	t.Run("EmptySessionID", func(t *testing.T) {
		bitmap := tracker.GetRetrievedIDs(ctx, "")
		if !bitmap.IsEmpty() {
			t.Error("expected empty bitmap for empty sessionID")
		}
	})

	t.Run("NonExistentSession", func(t *testing.T) {
		bitmap := tracker.GetRetrievedIDs(ctx, "non-existent")
		if !bitmap.IsEmpty() {
			t.Error("expected empty bitmap for non-existent sessionID")
		}
	})
}
