// Create: semcom_session/tracker.go
package semcom_session

import (
	"context"
	"database/sql"
	_ "embed"
	"log/slog"

	"github.com/RoaringBitmap/roaring"
)

//go:embed schema.sql
var Schema string

// Tracker manages the persistence of retrieved memory IDs for active sessions.
type Tracker struct {
	db *sql.DB
}

// NewTracker creates a new Tracker instance.
func NewTracker(db *sql.DB) *Tracker {
	return &Tracker{db: db}
}

// GetRetrievedIDs returns a bitmap of document IDs already retrieved for the session.
// Returns an empty bitmap and logs error if the query fails.
func (t *Tracker) GetRetrievedIDs(ctx context.Context, sessionID string) (*roaring.Bitmap, error) {
	bm := roaring.New()
	if t == nil || t.db == nil {
		return bm, nil
	}

	rows, err := t.db.QueryContext(ctx, "SELECT memory_id FROM session_retrievals WHERE session_id = ?", sessionID)
	if err != nil {
		slog.Error("failed to query retrieved IDs", "session_id", sessionID, "error", err)
		return bm, err
	}
	defer rows.Close()

	for rows.Next() {
		var docID uint32
		if err := rows.Scan(&docID); err != nil {
			slog.Error("failed to scan memory ID", "session_id", sessionID, "error", err)
			continue
		}
		bm.Add(docID)
	}

	if err := rows.Err(); err != nil {
		slog.Error("error during rows iteration", "session_id", sessionID, "error", err)
	}

	return bm, nil
}
