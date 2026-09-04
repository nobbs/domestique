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

// ActivitiesBetween is one target's recorded activities that started within
// the half-open window [from, to), newest first, at most limit of them.
func (s *Store) ActivitiesBetween(
	ctx context.Context, targetID string, from, to time.Time, limit int,
) ([]activity.Stored, error) {
	// Start times are stored as whole seconds; a sub-second edge must not widen
	// the window onto the second before it.
	rows, err := s.queries.ListActivitiesBetween(ctx, sqlcgen.ListActivitiesBetweenParams{
		TargetSlot: targetID,
		FromUnix:   from.Truncate(time.Second).Unix(),
		ToUnix:     to.Truncate(time.Second).Unix(),
		RowLimit:   int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("reading stored activities: %w", err)
	}
	stored := make([]activity.Stored, 0, len(rows))
	for _, row := range rows {
		stored = append(stored, activity.Stored{
			ID:             row.WorkoutID,
			StartedAt:      time.Unix(row.StartedAtUnix, 0).UTC(),
			DistanceMetres: row.DistanceMetres,
			MovingSeconds:  row.MovingSeconds,
			ElapsedSeconds: row.ElapsedSeconds,
			AscentMetres:   row.AscentMetres,
			TypeID:         int(row.WorkoutTypeID),
			LocationID:     int(row.WorkoutTypeLocationID),
		})
	}

	return stored, nil
}
