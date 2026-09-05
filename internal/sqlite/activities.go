package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/ridemodel"
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
// recorded it, overwriting the row a prior poll stored for the same workout and
// forgetting any skip recorded for it.
func (s *Store) StoreActivity(
	ctx context.Context, targetID string, listing activity.Listing, summary activity.Summary, now time.Time,
) error {
	if targetID == "" || listing.ID <= 0 {
		return errors.New("a target and an activity id are required")
	}

	return s.withTx(ctx, "activity", func(queries *sqlcgen.Queries) error {
		if err := queries.UpsertActivity(ctx, sqlcgen.UpsertActivityParams{
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
		if err := queries.DeleteActivitySkip(ctx, sqlcgen.DeleteActivitySkipParams{TargetSlot: targetID, WorkoutID: listing.ID}); err != nil {
			return fmt.Errorf("forgetting an activity skip: %w", err)
		}

		return nil
	})
}

// ActivitySkips are the activities a poll set aside for one target, with how
// often and how recently each was tried.
func (s *Store) ActivitySkips(ctx context.Context, targetID string) ([]activity.Skip, error) {
	rows, err := s.queries.ListActivitySkips(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("reading activity skips: %w", err)
	}
	skips := make([]activity.Skip, 0, len(rows))
	for _, row := range rows {
		skips = append(skips, activity.Skip{
			ID:          row.WorkoutID,
			Attempts:    int(row.Attempts),
			LastAttempt: time.Unix(row.LastAttemptUnix, 0).UTC(),
		})
	}

	return skips, nil
}

// RecordActivitySkip counts one more failed read of an activity, keeping what
// the source answered so the next occurrence can be told from the last.
func (s *Store) RecordActivitySkip(ctx context.Context, targetID string, id int64, observed string, now time.Time) error {
	if targetID == "" || id <= 0 {
		return errors.New("a target and an activity id are required")
	}
	if err := s.queries.UpsertActivitySkip(ctx, sqlcgen.UpsertActivitySkipParams{
		TargetSlot:      targetID,
		WorkoutID:       id,
		LastAttemptUnix: now.Unix(),
		Observed:        observed,
	}); err != nil {
		return fmt.Errorf("recording an activity skip: %w", err)
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

// ActivitiesAwaitingRecords are one target's stored activities whose FIT
// samples are still absent, oldest first, at most limit of them.
func (s *Store) ActivitiesAwaitingRecords(
	ctx context.Context, targetID string, limit int,
) ([]activity.PendingActivity, error) {
	if targetID == "" || limit <= 0 {
		return nil, errors.New("a target and a positive limit are required")
	}
	rows, err := s.queries.ListActivitiesAwaitingRecords(ctx, sqlcgen.ListActivitiesAwaitingRecordsParams{
		TargetSlot: targetID,
		RowLimit:   int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("reading activities awaiting records: %w", err)
	}
	pending := make([]activity.PendingActivity, 0, len(rows))
	for _, row := range rows {
		pending = append(pending, activity.PendingActivity{
			ID:      row.WorkoutID,
			Summary: activity.Summary{Raw: row.RawSummaryJson},
		})
	}

	return pending, nil
}

// insertActivityRecordSQL is prepared once per ride rather than generated by
// sqlc: a FIT holds thousands of samples, and compiling the insert per row is
// most of the cost of storing one.
const insertActivityRecordSQL = `INSERT INTO activity_records (
  target_slot, workout_id, record_index, recorded_at_unix,
  distance_metres, latitude, longitude, altitude_metres,
  cadence_rpm, heart_rate_bpm, power_watts, temperature_celsius
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// StoreActivityRecords replaces one activity's samples and marks it stored, in
// one transaction so a partial rewrite is never left behind as complete.
func (s *Store) StoreActivityRecords(ctx context.Context, targetID string, id int64, fit activity.FIT) error {
	if targetID == "" || id <= 0 {
		return errors.New("a target and an activity id are required")
	}
	transaction, beginErr := s.database.BeginTx(ctx, nil)
	if beginErr != nil {
		return fmt.Errorf("starting the activity records write: %w", beginErr)
	}
	defer rollback(transaction)
	queries := s.queries.WithTx(transaction)
	if deleteErr := queries.DeleteActivityRecords(ctx, sqlcgen.DeleteActivityRecordsParams{
		TargetSlot: targetID, WorkoutID: id,
	}); deleteErr != nil {
		return fmt.Errorf("clearing prior activity records: %w", deleteErr)
	}
	insert, prepareErr := transaction.PrepareContext(ctx, insertActivityRecordSQL)
	if prepareErr != nil {
		return fmt.Errorf("preparing the activity sample insert: %w", prepareErr)
	}
	defer closeStatement(insert)
	for index, record := range fit.Records {
		if _, execErr := insert.ExecContext(ctx,
			targetID, id, int64(index), record.Time.Unix(),
			nullFloat(record.DistanceMetres, record.HasDistance),
			nullFloat(record.Latitude, record.HasPosition),
			nullFloat(record.Longitude, record.HasPosition),
			nullFloat(record.AltitudeMetres, record.HasAltitude),
			nullFloat(record.CadenceRPM, record.HasCadence),
			nullFloat(record.HeartRateBPM, record.HasHeartRate),
			nullFloat(record.PowerWatts, record.HasPower),
			nullFloat(record.TemperatureCelsius, record.HasTemperatureCelsius),
		); execErr != nil {
			return fmt.Errorf("recording an activity sample: %w", execErr)
		}
	}
	if markErr := queries.MarkActivityRecordsStored(ctx, sqlcgen.MarkActivityRecordsStoredParams{
		TargetSlot: targetID, WorkoutID: id, FitChecksumFailed: boolInteger(fit.ChecksumFailed),
	}); markErr != nil {
		return fmt.Errorf("marking an activity's records stored: %w", markErr)
	}
	if commitErr := transaction.Commit(); commitErr != nil {
		return fmt.Errorf("committing the activity records: %w", commitErr)
	}

	return nil
}

// MarkActivityUnreadable records that an activity's FIT file did not decode, so
// no later poll spends a download on it again.
func (s *Store) MarkActivityUnreadable(ctx context.Context, targetID string, id int64) error {
	if targetID == "" || id <= 0 {
		return errors.New("a target and an activity id are required")
	}
	if err := s.queries.MarkActivityRecordsUnreadable(ctx, sqlcgen.MarkActivityRecordsUnreadableParams{
		TargetSlot: targetID, WorkoutID: id,
	}); err != nil {
		return fmt.Errorf("marking an activity unreadable: %w", err)
	}

	return nil
}

func nullFloat(value float64, valid bool) sql.NullFloat64 {
	return sql.NullFloat64{Float64: value, Valid: valid}
}

// ActivityRides is every target's recorded activity as a calibration reads it:
// one rider's corpus is one target's, and the fit pools them all.
func (s *Store) ActivityRides(ctx context.Context) ([]ridemodel.Ride, error) {
	rows, err := s.queries.ListActivityRides(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading activities for calibration: %w", err)
	}
	rides := make([]ridemodel.Ride, 0, len(rows))
	for _, row := range rows {
		rides = append(rides, ridemodel.Ride{
			StartedAt:      time.Unix(row.StartedAtUnix, 0).UTC(),
			DistanceMetres: row.DistanceMetres,
			MovingSeconds:  row.MovingSeconds,
			AscentMetres:   row.AscentMetres,
		})
	}

	return rides, nil
}
