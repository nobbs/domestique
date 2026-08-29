package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// LastFailureNotification returns the previous delivery time for one safe
// failure category. The caller decides whether the configured suppression
// interval has elapsed.
func (s *Store) LastFailureNotification(ctx context.Context, category string) (time.Time, bool, error) {
	if category == "" {
		return time.Time{}, false, errors.New("failure category is required")
	}
	var sentAt int64
	err := s.database.QueryRowContext(ctx, `
		SELECT last_sent_at_unix FROM notification_state WHERE kind = ?
	`, "failure:"+category).Scan(&sentAt)
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
		if _, err := s.database.ExecContext(ctx, `
			DELETE FROM notification_state WHERE kind = ?
		`, "failure:"+category); err != nil {
			return fmt.Errorf("clearing failure notification: %w", err)
		}

		return nil
	}
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO notification_state (kind, last_sent_at_unix) VALUES (?, ?)
		ON CONFLICT(kind) DO UPDATE SET last_sent_at_unix = excluded.last_sent_at_unix
	`, "failure:"+category, sentAt.Unix()); err != nil {
		return fmt.Errorf("recording failure notification: %w", err)
	}

	return nil
}

// digestNotificationKind is the single row the success digest keeps its clock
// in. Failure notifications key a row per category because each category is its
// own alert; the digest is one periodic message and needs one.
const digestNotificationKind = "digest:success"

// LastDigestNotification returns when the last success digest was delivered and
// the highest run it covered. Absent, the caller has not started its clock yet.
func (s *Store) LastDigestNotification(ctx context.Context) (sentAt time.Time, lastRunID int64, found bool, err error) {
	var sentAtUnix int64
	if err := s.database.QueryRowContext(ctx, `
		SELECT last_sent_at_unix, last_run_id FROM notification_state WHERE kind = ?
	`, digestNotificationKind).Scan(&sentAtUnix, &lastRunID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return time.Time{}, 0, false, nil
		}

		return time.Time{}, 0, false, fmt.Errorf("reading digest notification state: %w", err)
	}

	return time.Unix(sentAtUnix, 0).UTC(), lastRunID, true, nil
}

// RecordDigestNotification moves the digest window forward, after Pushover
// accepted the message or when the window is first anchored.
func (s *Store) RecordDigestNotification(ctx context.Context, sentAt time.Time, lastRunID int64) error {
	if sentAt.IsZero() {
		return errors.New("notification time is required")
	}
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO notification_state (kind, last_sent_at_unix, last_run_id) VALUES (?, ?, ?)
		ON CONFLICT(kind) DO UPDATE SET
			last_sent_at_unix = excluded.last_sent_at_unix,
			last_run_id = excluded.last_run_id
	`, digestNotificationKind, sentAt.Unix(), lastRunID); err != nil {
		return fmt.Errorf("recording digest notification: %w", err)
	}

	return nil
}
