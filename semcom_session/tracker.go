// Create: semcom_session/tracker.go
package semcom_session

import (
	"database/sql"
	_ "embed"
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
