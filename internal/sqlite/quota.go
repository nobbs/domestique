package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// WahooQuota is one reading of Wahoo's advertised request quota, kept so a
// restart resumes with what the last process observed. ResetAt and NotBefore
// are zero when that reading named neither.
type WahooQuota struct {
	ObservedAt time.Time
	ExpiresAt  time.Time
	ResetAt    time.Time
	NotBefore  time.Time
	Remaining  int
}

// WahooQuota reads the stored reading. A service that has never had a response
// carry a quota header reports none, which is not a failure.
func (s *Store) WahooQuota(ctx context.Context) (WahooQuota, bool, error) {
	row, err := s.queries.GetWahooQuota(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return WahooQuota{}, false, nil
	}
	if err != nil {
		return WahooQuota{}, false, fmt.Errorf("reading the wahoo quota: %w", err)
	}

	return WahooQuota{
		ObservedAt: instantAt(row.ObservedAtUnix),
		ExpiresAt:  instantAt(row.ExpiresAtUnix),
		ResetAt:    instantAt(row.ResetAtUnix),
		NotBefore:  instantAt(row.NotBeforeUnix),
		Remaining:  int(row.Remaining),
	}, true, nil
}

// StoreWahooQuota replaces the stored reading with a newer one.
func (s *Store) StoreWahooQuota(ctx context.Context, quota *WahooQuota) error {
	if quota.ObservedAt.IsZero() || quota.ExpiresAt.IsZero() {
		return errors.New("a stored wahoo quota needs an observation and an expiry instant")
	}
	if err := s.queries.UpsertWahooQuota(ctx, sqlcgen.UpsertWahooQuotaParams{
		Remaining:      int64(quota.Remaining),
		ResetAtUnix:    unixOf(quota.ResetAt),
		NotBeforeUnix:  unixOf(quota.NotBefore),
		ObservedAtUnix: unixOf(quota.ObservedAt),
		ExpiresAtUnix:  unixOf(quota.ExpiresAt),
	}); err != nil {
		return fmt.Errorf("storing the wahoo quota: %w", err)
	}

	return nil
}

// unixOf stores a zero instant as zero rather than as 1970, so instantAt can
// give the same absent instant back.
func unixOf(instant time.Time) int64 {
	if instant.IsZero() {
		return 0
	}

	return instant.UTC().Unix()
}

func instantAt(seconds int64) time.Time {
	if seconds == 0 {
		return time.Time{}
	}

	return time.Unix(seconds, 0).UTC()
}
