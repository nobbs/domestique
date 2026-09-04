package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// KnownActivityIDs are the Wahoo workout IDs already stored for one target,
// which is what a poll compares the account's listing against.
func (s *Store) KnownActivityIDs(ctx context.Context, targetID string) ([]int64, error) {
	ids, err := s.queries.ListActivityIDs(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("reading stored activity ids: %w", err)
	}

	return ids, nil
}

// StoreActivity records one activity summary against the target whose account
// recorded it, overwriting the row a prior poll stored for the same workout.
func (s *Store) StoreActivity(
	ctx context.Context, targetID string, listing activity.Listing, summary activity.Summary, now time.Time,
) error {
	if targetID == "" || listing.ID <= 0 {
		return errors.New("a target and an activity id are required")
	}
	if err := s.queries.UpsertActivity(ctx, sqlcgen.UpsertActivityParams{
		TargetSlot:            targetID,
		WorkoutID:             listing.ID,
		WorkoutTypeID:         int64(listing.TypeID),
		WorkoutTypeLocationID: int64(listing.LocationID),
		StartedAtUnix:         listing.Starts.Unix(),
		DistanceMetres:        summary.DistanceMetres,
		MovingSeconds:         summary.MovingSeconds,
		ElapsedSeconds:        summary.ElapsedSeconds,
		AscentMetres:          summary.AscentMetres,
		RawSummaryJson:        summary.Raw,
		UpdatedAtUnix:         now.Unix(),
	}); err != nil {
		return fmt.Errorf("recording an activity: %w", err)
	}

	return nil
}
