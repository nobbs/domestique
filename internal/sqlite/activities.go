package sqlite

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// ActivityStored reports whether one provider's activity is already stored for
// a target, without reading the rest of the target's activities.
func (s *Store) ActivityStored(ctx context.Context, targetID string, id int64, provider string) (bool, error) {
	stored, err := s.queries.ActivityExists(ctx,
		sqlcgen.ActivityExistsParams{TargetSlot: targetID, WorkoutID: id, Provider: provider})
	if err != nil {
		return false, fmt.Errorf("reading whether an activity is stored: %w", err)
	}

	return stored, nil
}

// KnownActivityIDs are one provider's activity ids already stored for one
// target, which is what that provider's poll compares its listing against.
func (s *Store) KnownActivityIDs(ctx context.Context, targetID, provider string) ([]int64, error) {
	ids, err := s.queries.ListActivityIDs(ctx, sqlcgen.ListActivityIDsParams{TargetSlot: targetID, Provider: provider})
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
		written, err := queries.UpsertActivity(ctx, sqlcgen.UpsertActivityParams{
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
			Provider:              cmp.Or(listing.Provider, activity.ProviderWahoo),
		})
		if err != nil {
			return fmt.Errorf("recording an activity: %w", err)
		}
		if written == 0 {
			return fmt.Errorf("%w: activity %d of %s", ErrActivityProviderConflict, listing.ID, targetID)
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

// ActivityListings are the activities one target's account holds, oldest first,
// as the last full reading of that account left them, and when that reading was
// taken. A target never read has no listings and a zero time.
func (s *Store) ActivityListings(
	ctx context.Context, targetID string,
) (listings []activity.Listing, readAt time.Time, err error) {
	rows, err := s.queries.ListActivityListings(ctx, targetID)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("reading activity listings: %w", err)
	}
	listings = make([]activity.Listing, 0, len(rows))
	for _, row := range rows {
		listings = append(listings, activity.Listing{
			ID:         row.WorkoutID,
			Starts:     time.Unix(row.StartedAtUnix, 0).UTC(),
			TypeID:     int(row.WorkoutTypeID),
			LocationID: int(row.WorkoutTypeLocationID),
		})
		readAt = time.Unix(row.ReadAtUnix, 0).UTC()
	}

	return listings, readAt, nil
}

// ReplaceActivityListings makes the kept listings exactly those a fresh reading
// of the account found. It is one transaction because a poll that read a
// partial listing would treat the account as smaller than it is, and read it
// again on every poll after.
func (s *Store) ReplaceActivityListings(
	ctx context.Context, targetID string, listings []activity.Listing, now time.Time,
) error {
	if targetID == "" {
		return errors.New("a target is required")
	}

	return s.withTx(ctx, "activity listings", func(queries *sqlcgen.Queries) error {
		if err := queries.DeleteActivityListings(ctx, targetID); err != nil {
			return fmt.Errorf("clearing activity listings: %w", err)
		}
		for _, listing := range listings {
			if listing.ID <= 0 {
				return errors.New("an activity id is required")
			}
			if err := queries.InsertActivityListing(ctx, sqlcgen.InsertActivityListingParams{
				TargetSlot:            targetID,
				WorkoutID:             listing.ID,
				StartedAtUnix:         listing.Starts.Unix(),
				WorkoutTypeID:         int64(listing.TypeID),
				WorkoutTypeLocationID: int64(listing.LocationID),
				ReadAtUnix:            now.Unix(),
			}); err != nil {
				return fmt.Errorf("recording an activity listing: %w", err)
			}
		}

		return nil
	})
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
			Provider:       row.Provider,
		})
	}

	return stored, nil
}

// ActivityRecordsState is how far one target's activity has got in storing its
// recorded samples, and the workout type it was recorded as. found is false
// when the target has no such activity, which is what tells a missing ride
// from one whose samples are still awaited.
func (s *Store) ActivityRecordsState(
	ctx context.Context, targetID string, id int64,
) (state activity.RecordsState, typeID int, found bool, err error) {
	stored, err := s.queries.GetActivityRecordsState(ctx, sqlcgen.GetActivityRecordsStateParams{
		TargetSlot: targetID, WorkoutID: id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	if err != nil {
		return "", 0, false, fmt.Errorf("reading an activity records state: %w", err)
	}

	return activity.RecordsState(stored.RecordsState), int(stored.WorkoutTypeID), true, nil
}

// ActivityProviderSummary is which provider recorded one target's activity and
// the summary document that provider's own adapter wrote for it. Interpreting
// the document is the caller's; this store only holds it.
func (s *Store) ActivityProviderSummary(
	ctx context.Context, targetID string, id int64,
) (provider string, summary []byte, err error) {
	row, err := s.queries.GetActivityProviderSummary(ctx, sqlcgen.GetActivityProviderSummaryParams{
		TargetSlot: targetID, WorkoutID: id,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, fmt.Errorf("reading an activity's provider summary: %w", err)
	}

	return row.Provider, row.RawSummaryJson, nil
}

// ActivityTrack is the positioned samples of one target's activity, in the
// order they were recorded. Records without a position are left out, so an
// activity that never recorded one has an empty track.
func (s *Store) ActivityTrack(ctx context.Context, targetID string, id int64) ([]activity.TrackPoint, error) {
	rows, err := s.queries.ListActivityTrack(ctx, sqlcgen.ListActivityTrackParams{TargetSlot: targetID, WorkoutID: id})
	if err != nil {
		return nil, fmt.Errorf("reading an activity track: %w", err)
	}
	track := make([]activity.TrackPoint, 0, len(rows))
	for _, row := range rows {
		track = append(track, activity.TrackPoint{
			Time:                time.Unix(row.RecordedAtUnix, 0).UTC(),
			Latitude:            row.Latitude.Float64,
			Longitude:           row.Longitude.Float64,
			AltitudeMetres:      row.AltitudeMetres.Float64,
			EstimatedPowerWatts: row.EstimatedPowerWatts.Float64,
			HasAltitude:         row.AltitudeMetres.Valid,
			HasEstimatedPower:   row.EstimatedPowerWatts.Valid,
		})
	}

	return track, nil
}

// ActivitySeries is the non-positional part of one target's activity's
// positioned samples, in the order they were recorded — indexed 1:1 with what
// ActivityTrack returns for the same ride.
func (s *Store) ActivitySeries(ctx context.Context, targetID string, id int64) ([]activity.SampleRow, error) {
	rows, err := s.queries.ListActivitySeries(ctx, sqlcgen.ListActivitySeriesParams{TargetSlot: targetID, WorkoutID: id})
	if err != nil {
		return nil, fmt.Errorf("reading an activity series: %w", err)
	}
	samples := make([]activity.SampleRow, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		samples = append(samples, activity.SampleRow{
			Time:               time.Unix(row.RecordedAtUnix, 0).UTC(),
			DistanceMetres:     reading(row.DistanceMetres),
			AltitudeMetres:     reading(row.AltitudeMetres),
			HeartRateBPM:       reading(row.HeartRateBpm),
			CadenceRPM:         reading(row.CadenceRpm),
			PowerWatts:         reading(row.PowerWatts),
			TemperatureCelsius: reading(row.TemperatureCelsius),
			SpeedMS:            reading(row.SpeedMs),
			GradePercent:       reading(row.GradePercent),
			CaloriesKcal:       reading(row.CaloriesKcal),
			AscentMetres:       reading(row.AscentMetres),
			DescentMetres:      reading(row.DescentMetres),
		})
	}

	return samples, nil
}

// reading carries a nullable column across as the optional value it is.
func reading(column sql.NullFloat64) activity.Reading {
	return activity.Reading{Value: column.Float64, Known: column.Valid}
}

// ActivitiesAwaitingRecords are one target's stored activities whose FIT
// samples are still absent, newest first, followed by the activities whose
// samples predate recordsVersion, also newest first, at most limit of them.
func (s *Store) ActivitiesAwaitingRecords(
	ctx context.Context, targetID, provider string, recordsVersion, limit int,
) ([]activity.PendingActivity, error) {
	if targetID == "" || limit <= 0 {
		return nil, errors.New("a target and a positive limit are required")
	}
	rows, err := s.queries.ListActivitiesAwaitingRecords(ctx, sqlcgen.ListActivitiesAwaitingRecordsParams{
		TargetSlot:     targetID,
		Provider:       cmp.Or(provider, activity.ProviderWahoo),
		RecordsVersion: int64(recordsVersion),
		RowLimit:       int64(limit),
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
  cadence_rpm, heart_rate_bpm, power_watts, temperature_celsius,
  speed_ms, grade_percent, calories_kcal, ascent_metres, descent_metres
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// StoreActivityRecords replaces one activity's samples and marks it stored at
// recordsVersion, in one transaction so a partial rewrite is never left behind
// as complete.
//
//nolint:gocritic // value param: this method conforms to the activity poll's store contract.
func (s *Store) StoreActivityRecords(
	ctx context.Context, targetID string, id int64, fit activity.FIT, recordsVersion int,
) error {
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
	// The route match describes the samples being replaced, so it goes with
	// them: the next derivation works it out again from the new track.
	if matchErr := queries.DeleteActivityRouteMatch(ctx, sqlcgen.DeleteActivityRouteMatchParams{
		TargetSlot: targetID, WorkoutID: id,
	}); matchErr != nil {
		return fmt.Errorf("clearing a prior route match: %w", matchErr)
	}
	// So is the metrics row: nothing else lists a ride whose samples changed
	// under a row worked out from the old ones.
	if metricsErr := queries.DeleteActivityMetrics(ctx, sqlcgen.DeleteActivityMetricsParams{
		TargetSlot: targetID, WorkoutID: id,
	}); metricsErr != nil {
		return fmt.Errorf("clearing prior activity metrics: %w", metricsErr)
	}
	insert, prepareErr := transaction.PrepareContext(ctx, insertActivityRecordSQL)
	if prepareErr != nil {
		return fmt.Errorf("preparing the activity sample insert: %w", prepareErr)
	}
	defer closeStatement(insert)
	for index := range fit.Records {
		record := &fit.Records[index]
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
			nullFloat(record.SpeedMS, record.HasSpeed),
			nullFloat(record.GradePercent, record.HasGrade),
			nullFloat(record.CaloriesKcal, record.HasCalories),
			nullFloat(record.AscentMetres, record.HasAscent),
			nullFloat(record.DescentMetres, record.HasDescent),
		); execErr != nil {
			return fmt.Errorf("recording an activity sample: %w", execErr)
		}
	}
	if sessionErr := storeActivitySession(ctx, queries, targetID, id, &fit.Session); sessionErr != nil {
		return sessionErr
	}
	if markErr := queries.MarkActivityRecordsStored(ctx, sqlcgen.MarkActivityRecordsStoredParams{
		TargetSlot: targetID, WorkoutID: id, FitChecksumFailed: boolInteger(fit.ChecksumFailed),
		RecordsVersion: int64(recordsVersion),
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

// storeActivitySession replaces one ride's device-declared session figures, or
// removes the row when the file carried none: no row is what "the file said
// nothing" already means for activity_metrics.
func storeActivitySession(ctx context.Context, queries *sqlcgen.Queries, targetID string, id int64, session *activity.Session) error {
	if !session.Any() {
		if err := queries.DeleteActivitySession(ctx, sqlcgen.DeleteActivitySessionParams{
			TargetSlot: targetID, WorkoutID: id,
		}); err != nil {
			return fmt.Errorf("clearing a prior activity session: %w", err)
		}

		return nil
	}
	if err := queries.UpsertActivitySession(ctx, sqlcgen.UpsertActivitySessionParams{
		TargetSlot: targetID, WorkoutID: id,
		MaxSpeedKmh:               readingNull(session.MaxSpeedKmh),
		AverageSpeedKmh:           readingNull(session.AverageSpeedKmh),
		DistanceMetres:            readingNull(session.DistanceMetres),
		TimerSeconds:              readingNull(session.TimerSeconds),
		ElapsedSeconds:            readingNull(session.ElapsedSeconds),
		AscentMetres:              readingNull(session.AscentMetres),
		DescentMetres:             readingNull(session.DescentMetres),
		CaloriesKcal:              readingNull(session.CaloriesKcal),
		AverageHeartRateBpm:       readingNull(session.AverageHeartRateBPM),
		MaxHeartRateBpm:           readingNull(session.MaxHeartRateBPM),
		MinHeartRateBpm:           readingNull(session.MinHeartRateBPM),
		AverageCadenceRpm:         readingNull(session.AverageCadenceRPM),
		MaxCadenceRpm:             readingNull(session.MaxCadenceRPM),
		AveragePowerWatts:         readingNull(session.AveragePowerWatts),
		MaxPowerWatts:             readingNull(session.MaxPowerWatts),
		NormalizedPowerWatts:      readingNull(session.NormalizedPowerWatts),
		ThresholdPowerWatts:       readingNull(session.ThresholdPowerWatts),
		AverageTemperatureCelsius: readingNull(session.AverageTemperatureCelsius),
		MaxTemperatureCelsius:     readingNull(session.MaxTemperatureCelsius),
		AverageGradePercent:       readingNull(session.AverageGradePercent),
		MaxPositiveGradePercent:   readingNull(session.MaxPositiveGradePercent),
		MaxNegativeGradePercent:   readingNull(session.MaxNegativeGradePercent),
		MinAltitudeMetres:         readingNull(session.MinAltitudeMetres),
		MaxAltitudeMetres:         readingNull(session.MaxAltitudeMetres),
		AverageAltitudeMetres:     readingNull(session.AverageAltitudeMetres),
		Sport:                     session.Sport,
		SubSport:                  session.SubSport,
		HeartRateZoneSecondsJson:  nullJSON(session.HeartRateZoneSeconds),
		HeartRateZoneHighBpmJson:  nullJSON(session.HeartRateZoneHighBPM),
		PowerZoneSecondsJson:      nullJSON(session.PowerZoneSeconds),
		PowerZoneHighWattsJson:    nullJSON(session.PowerZoneHighWatts),
	}); err != nil {
		return fmt.Errorf("recording an activity session: %w", err)
	}
	if err := queries.ApplyActivitySessionTotals(ctx, sqlcgen.ApplyActivitySessionTotalsParams{
		TargetSlot: targetID, WorkoutID: id,
		DistanceMetres: readingNull(session.DistanceMetres),
		MovingSeconds:  readingNull(session.TimerSeconds),
		ElapsedSeconds: readingNull(session.ElapsedSeconds),
		AscentMetres:   readingNull(session.AscentMetres),
	}); err != nil {
		return fmt.Errorf("applying an activity session's totals: %w", err)
	}

	return nil
}

// readingNull carries a Reading across as the nullable column it is.
func readingNull(reading activity.Reading) sql.NullFloat64 {
	return nullFloat(reading.Value, reading.Known)
}

// nullJSON encodes values as JSON, or NULL for an empty slice: a zone table
// with no entries says as little as none.
func nullJSON(values []float64) sql.NullString {
	if len(values) == 0 {
		return sql.NullString{}
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		// Only a NaN or infinity fails here, and a zone table with one is no
		// table at all.
		return sql.NullString{}
	}

	return sql.NullString{String: string(encoded), Valid: true}
}

// ActivitySessions reads every device-declared session row one target holds,
// keyed by ride. A target with none has an empty map.
func (s *Store) ActivitySessions(ctx context.Context, targetID string) (map[int64]activity.Session, error) {
	rows, err := s.queries.ListActivitySessions(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("reading activity sessions: %w", err)
	}
	sessions := make(map[int64]activity.Session, len(rows))
	for index := range rows {
		row := &rows[index]
		session, decodeErr := activitySessionOf(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		sessions[row.WorkoutID] = session
	}

	return sessions, nil
}

// activitySessionOf maps one stored row to the activity.Session it holds.
func activitySessionOf(row *sqlcgen.ListActivitySessionsRow) (activity.Session, error) {
	session := activity.Session{
		MaxSpeedKmh:               reading(row.MaxSpeedKmh),
		AverageSpeedKmh:           reading(row.AverageSpeedKmh),
		DistanceMetres:            reading(row.DistanceMetres),
		TimerSeconds:              reading(row.TimerSeconds),
		ElapsedSeconds:            reading(row.ElapsedSeconds),
		AscentMetres:              reading(row.AscentMetres),
		DescentMetres:             reading(row.DescentMetres),
		CaloriesKcal:              reading(row.CaloriesKcal),
		AverageHeartRateBPM:       reading(row.AverageHeartRateBpm),
		MaxHeartRateBPM:           reading(row.MaxHeartRateBpm),
		MinHeartRateBPM:           reading(row.MinHeartRateBpm),
		AverageCadenceRPM:         reading(row.AverageCadenceRpm),
		MaxCadenceRPM:             reading(row.MaxCadenceRpm),
		AveragePowerWatts:         reading(row.AveragePowerWatts),
		MaxPowerWatts:             reading(row.MaxPowerWatts),
		NormalizedPowerWatts:      reading(row.NormalizedPowerWatts),
		ThresholdPowerWatts:       reading(row.ThresholdPowerWatts),
		AverageTemperatureCelsius: reading(row.AverageTemperatureCelsius),
		MaxTemperatureCelsius:     reading(row.MaxTemperatureCelsius),
		AverageGradePercent:       reading(row.AverageGradePercent),
		MaxPositiveGradePercent:   reading(row.MaxPositiveGradePercent),
		MaxNegativeGradePercent:   reading(row.MaxNegativeGradePercent),
		MinAltitudeMetres:         reading(row.MinAltitudeMetres),
		MaxAltitudeMetres:         reading(row.MaxAltitudeMetres),
		AverageAltitudeMetres:     reading(row.AverageAltitudeMetres),
		Sport:                     row.Sport,
		SubSport:                  row.SubSport,
	}
	zoneFields := []struct {
		target *[]float64
		json   sql.NullString
	}{
		{&session.HeartRateZoneSeconds, row.HeartRateZoneSecondsJson},
		{&session.HeartRateZoneHighBPM, row.HeartRateZoneHighBpmJson},
		{&session.PowerZoneSeconds, row.PowerZoneSecondsJson},
		{&session.PowerZoneHighWatts, row.PowerZoneHighWattsJson},
	}
	for _, field := range zoneFields {
		if !field.json.Valid {
			continue
		}
		if err := json.Unmarshal([]byte(field.json.String), field.target); err != nil {
			return activity.Session{}, fmt.Errorf("decoding an activity session's zone table: %w", err)
		}
	}

	return session, nil
}

// ActivityRides is every target's recorded activity of one of workoutTypeIDs,
// ridden at or after since, as a calibration reads it: one rider's corpus is
// one target's, and the fit pools them all. A zero since reads all history; an
// empty set of types matches no activity, since a corpus of every type is not
// something a caller can ask for by omission.
func (s *Store) ActivityRides(
	ctx context.Context, since time.Time, workoutTypeIDs []int,
) ([]ridemodel.Ride, error) {
	if len(workoutTypeIDs) == 0 {
		return nil, nil
	}
	rows, err := s.queries.ListActivityRides(ctx, sqlcgen.ListActivityRidesParams{
		SinceUnix: since.Unix(), WorkoutTypeIds: int64s(workoutTypeIDs),
	})
	if err != nil {
		return nil, fmt.Errorf("reading activities for calibration: %w", err)
	}
	rides := make([]ridemodel.Ride, 0, len(rows))
	for _, row := range rows {
		rides = append(rides, ridemodel.Ride{
			StartedAt:      time.Unix(row.StartedAtUnix, 0).UTC(),
			TargetID:       row.TargetSlot,
			DistanceMetres: row.DistanceMetres,
			MovingSeconds:  row.MovingSeconds,
			AscentMetres:   row.AscentMetres,
		})
	}

	return rides, nil
}

// RecordedRide identifies one ride whose samples are stored, the ascent its
// device reported for it, and the summary distance and moving time it also
// reported.
type RecordedRide struct {
	TargetID       string
	WorkoutID      int64
	AscentMetres   float64
	DistanceMetres float64
	MovingSeconds  float64
}

// ActivityCaloriesAccum is the kilocalories a ride's device reported in its
// stored summary, and whether the summary carried one at all. Decodes only
// that one field of raw_summary_json; nothing else in it is this store's
// concern to interpret.
func (s *Store) ActivityCaloriesAccum(ctx context.Context, targetID string, id int64) (kcal float64, ok bool, err error) {
	raw, err := s.queries.GetActivityRawSummary(ctx, sqlcgen.GetActivityRawSummaryParams{TargetSlot: targetID, WorkoutID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("reading an activity's raw summary: %w", err)
	}
	// Wahoo writes the accumulators as strings; json.Number reads either.
	var summary struct {
		//nolint:tagliatelle // Wahoo's API uses snake_case.
		CaloriesAccum *json.Number `json:"calories_accum"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if unmarshalErr := decoder.Decode(&summary); unmarshalErr != nil {
		return 0, false, fmt.Errorf("decoding an activity's raw summary: %w", unmarshalErr)
	}
	if summary.CaloriesAccum == nil {
		return 0, false, nil
	}
	kcal, parseErr := strconv.ParseFloat(strings.Trim(summary.CaloriesAccum.String(), `"`), 64)
	if parseErr != nil {
		return 0, false, fmt.Errorf("decoding an activity's calories: %w", parseErr)
	}

	return kcal, true, nil
}

// RecordedRides is every target's ride whose samples are stored, ordered by
// target slot then workout id, for an offline tool that walks every ride's
// own track rather than one target's.
func (s *Store) RecordedRides(ctx context.Context) ([]RecordedRide, error) {
	rows, err := s.queries.ListRecordedActivities(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading recorded activities: %w", err)
	}
	rides := make([]RecordedRide, 0, len(rows))
	for _, row := range rows {
		rides = append(rides, RecordedRide{
			TargetID: row.TargetSlot, WorkoutID: row.WorkoutID, AscentMetres: row.AscentMetres,
			DistanceMetres: row.DistanceMetres, MovingSeconds: row.MovingSeconds,
		})
	}

	return rides, nil
}

// ErrActivityProviderConflict reports a stored activity whose provider is not
// the one writing it. Two id spaces share the activity key, so a collision is
// refused rather than allowed to overwrite another upstream's ride.
var ErrActivityProviderConflict = errors.New("an activity of this id belongs to another provider")

// IndoorRideStarts are the start times of one target's stored Zwift rides,
// which is what tells a Wahoo listing that is the head unit's copy of one.
func (s *Store) IndoorRideStarts(ctx context.Context, targetID string) ([]time.Time, error) {
	rows, err := s.queries.ListIndoorRideStarts(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("reading indoor ride starts: %w", err)
	}
	starts := make([]time.Time, 0, len(rows))
	for _, unix := range rows {
		starts = append(starts, time.Unix(unix, 0).UTC())
	}

	return starts, nil
}

// DeleteTrainerCopy removes the Wahoo activities that started within window of
// at, and reports how many went. Everything derived from them goes with them
// through the schema's cascades; the skip and listing rows do not, because
// those mirror the account rather than what is stored.
func (s *Store) DeleteTrainerCopy(
	ctx context.Context, targetID string, at time.Time, window time.Duration, indoorTypeIDs []int,
) (int, error) {
	removed, err := s.queries.DeleteTrainerCopyActivity(ctx, sqlcgen.DeleteTrainerCopyActivityParams{
		TargetSlot:    targetID,
		IndoorTypeIds: typeIDList(indoorTypeIDs),
		FromUnix:      at.Add(-window).Unix(),
		ToUnix:        at.Add(window).Unix(),
	})
	if err != nil {
		return 0, fmt.Errorf("removing a trainer copy of an indoor ride: %w", err)
	}

	return int(removed), nil
}

// int64s widens workout type ids for a query binding.
func int64s(values []int) []int64 {
	widened := make([]int64, len(values))
	for index, value := range values {
		widened[index] = int64(value)
	}

	return widened
}

// typeIDList renders workout type ids as the JSON array json_each reads. A
// bound list rather than sqlc.slice: SQLite numbers the other placeholders, and
// an expanded slice before a LIMIT would take their indices.
func typeIDList(values []int) string {
	rendered := make([]string, len(values))
	for index, value := range values {
		rendered[index] = strconv.Itoa(value)
	}

	return "[" + strings.Join(rendered, ",") + "]"
}
