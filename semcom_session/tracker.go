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

// GetRetrievedIDs loads the bitmap of previously retrieved IDs.
// Returns an empty bitmap if none exist or if a database error occurs.
func (t *Tracker) GetRetrievedIDs(ctx context.Context, sessionID string) *roaring.Bitmap {
	bm := roaring.New()
	if sessionID == "" {
		return bm
	}

	rows, err := t.db.QueryContext(ctx, `SELECT memory_id FROM session_retrievals WHERE session_id = ?`, sessionID)
	if err != nil {
		slog.Error("failed to query session retrievals", "session_id", sessionID, "error", err)
		return bm
	}
	defer rows.Close()

	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			slog.Error("failed to scan session retrieval row", "session_id", sessionID, "error", err)
			continue
		}
		bm.Add(uint32(id))
	}
	if err := rows.Err(); err != nil {
		slog.Error("error iterating session retrievals", "session_id", sessionID, "error", err)
	}
	return bm
}

// MarkRetrieved appends newly retrieved memory IDs to the session's record.
func (t *Tracker) MarkRetrieved(ctx context.Context, sessionID string, memoryIDs []int32) error {
	if sessionID == "" || len(memoryIDs) == 0 {
		return nil
	}

	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO session_retrievals (session_id, memory_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range memoryIDs {
		if _, err := stmt.ExecContext(ctx, sessionID, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}
