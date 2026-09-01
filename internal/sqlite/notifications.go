package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// LastFailureNotification returns the previous delivery time for one safe
// failure category. The caller decides whether the configured suppression
// interval has elapsed.
func (s *Store) LastFailureNotification(ctx context.Context, category string) (time.Time, bool, error) {
	if category == "" {
		return time.Time{}, false, errors.New("failure category is required")
	}
	sentAt, err := s.queries.GetNotificationSentAt(ctx, "failure:"+category)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("reading failure notification state: %w", err)
	}

	return time.Unix(sentAt, 0).UTC(), true, nil
}

// RecordFailureNotification records a delivered notification after Pushover
// accepted it, so failed deliveries are retried next run. A zero sentAt clears
// the category's suppression record instead; a category never notified is a no-op.
func (s *Store) RecordFailureNotification(ctx context.Context, category string, sentAt time.Time) error {
	if category == "" {
		return errors.New("failure category is required")
	}
	if sentAt.IsZero() {
		if err := s.queries.DeleteNotification(ctx, "failure:"+category); err != nil {
			return fmt.Errorf("clearing failure notification: %w", err)
		}

		return nil
	}
	if err := s.queries.UpsertNotification(ctx, sqlcgen.UpsertNotificationParams{
		Kind: "failure:" + category, LastSentAtUnix: sentAt.Unix(),
	}); err != nil {
		return fmt.Errorf("recording failure notification: %w", err)
	}

	return nil
}
