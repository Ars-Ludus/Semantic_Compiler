package semcom_session

import (
	"context"
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func openTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	if _, err := db.Exec(Schema); err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}
	return db
}

func TestGetRetrievedIDs(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

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

func TestMarkRetrieved(t *testing.T) {
	db := openTestDB(t)
	tracker := NewTracker(db)
	ctx := context.Background()

	ids := []int32{5, 15, 25}
	if err := tracker.MarkRetrieved(ctx, "sess3", ids); err != nil {
		t.Fatalf("MarkRetrieved failed: %v", err)
	}

	bm := tracker.GetRetrievedIDs(ctx, "sess3")
	if bm.GetCardinality() != 3 {
		t.Errorf("expected 3 items, got %d", bm.GetCardinality())
	}
	for _, id := range ids {
		if !bm.Contains(uint32(id)) {
			t.Errorf("bitmap missing id %d", id)
		}
	}

	// Test idempotency (inserting same ID again shouldn't error due to primary key conflict)
	if err := tracker.MarkRetrieved(ctx, "sess3", []int32{15}); err != nil {
		t.Fatalf("MarkRetrieved duplicate failed: %v", err)
	}
}
