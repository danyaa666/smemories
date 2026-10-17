package printfeedback

import (
	"context"
	"database/sql"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Store keeps the counters in print_feedback_tab: one row per (browser_family, verdict).
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// increment adds one to the counter in a single statement, so parallel answers never lose a count.
func (s *Store) increment(ctx context.Context, family, verdict string, nowMs int64) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO print_feedback_tab (browser_family, verdict, hits, created_at, updated_at) VALUES (?,?,1,?,?)
		 ON DUPLICATE KEY UPDATE hits = hits + 1, updated_at = VALUES(updated_at)`, family, verdict, nowMs, nowMs)
	return apperr.WithCode(apperr.Internal, err)
}
