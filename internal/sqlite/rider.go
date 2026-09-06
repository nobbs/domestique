package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// RiderProfile reads one subject's own parameters. A subject who has entered
// nothing has an empty profile rather than a missing one: there is no state to
// distinguish, and every field is optional anyway.
func (s *Store) RiderProfile(ctx context.Context, subject string) (rider.Profile, error) {
	row, err := s.queries.GetRiderProfile(ctx, subject)
	if errors.Is(err, sql.ErrNoRows) {
		return rider.Profile{}, nil
	}
	if err != nil {
		return rider.Profile{}, fmt.Errorf("reading the rider profile: %w", err)
	}

	return rider.Profile{
		MaxHeartRateBPM:               riderValue(row.MaxHeartRateBpm),
		RestingHeartRateBPM:           riderValue(row.RestingHeartRateBpm),
		ThresholdHeartRateBPM:         riderValue(row.ThresholdHeartRateBpm),
		FunctionalThresholdPowerWatts: riderValue(row.FunctionalThresholdPowerWatts),
		RiderMassKG:                   riderValue(row.RiderMassKg),
		BikeMassKG:                    riderValue(row.BikeMassKg),
	}, nil
}

// SetRiderProfile replaces one subject's parameters whole, the way every
// settings section is replaced whole.
//
//nolint:gocritic // value param: a profile is six optional numbers, copied as cheaply as a pointer.
func (s *Store) SetRiderProfile(ctx context.Context, subject string, profile rider.Profile) error {
	if err := s.queries.UpsertRiderProfile(ctx, sqlcgen.UpsertRiderProfileParams{
		Subject:                       subject,
		MaxHeartRateBpm:               nullRiderValue(profile.MaxHeartRateBPM),
		RestingHeartRateBpm:           nullRiderValue(profile.RestingHeartRateBPM),
		ThresholdHeartRateBpm:         nullRiderValue(profile.ThresholdHeartRateBPM),
		FunctionalThresholdPowerWatts: nullRiderValue(profile.FunctionalThresholdPowerWatts),
		RiderMassKg:                   nullRiderValue(profile.RiderMassKG),
		BikeMassKg:                    nullRiderValue(profile.BikeMassKG),
		UpdatedAtUnix:                 time.Now().Unix(),
	}); err != nil {
		return fmt.Errorf("storing the rider profile: %w", err)
	}

	return nil
}

// RiderSuggestions reads the best efforts the given targets' recent rides hold,
// as the numbers those efforts imply. Rides that carry no such sensor yield no
// suggestion, which is what lets the page offer one field a figure and not the
// next.
//
// A query per target: the caller's own targets are at most one, because a
// target's identity is its owning subject's own value.
func (s *Store) RiderSuggestions(ctx context.Context, targetIDs []string, since time.Time) (rider.Suggestions, error) {
	suggestions := rider.Suggestions{}
	for _, targetID := range targetIDs {
		rows, err := s.queries.ListActivitySensorSamples(ctx, sqlcgen.ListActivitySensorSamplesParams{
			TargetSlot: targetID,
			SinceUnix:  since.Unix(),
		})
		if err != nil {
			return rider.Suggestions{}, fmt.Errorf("reading the recorded samples: %w", err)
		}
		accumulateSuggestions(rows, &suggestions)
	}

	return suggestions, nil
}

// accumulateSuggestions folds one target's samples in, a ride at a time: a best
// effort is held within one ride, so each ride's series is closed before the
// next one opens.
func accumulateSuggestions(rows []sqlcgen.ListActivitySensorSamplesRow, into *rider.Suggestions) {
	var heartRate, power sensorSeries
	var workoutID int64
	for index, row := range rows {
		if index == 0 || row.WorkoutID != workoutID {
			heartRate.best(rider.MaxHeartRateWindow, &into.MaxHeartRateBPM, nil)
			power.best(rider.ThresholdPowerWindow, &into.FunctionalThresholdPowerWatts, rider.ThresholdPower)
			heartRate, power, workoutID = sensorSeries{}, sensorSeries{}, row.WorkoutID
		}
		at := time.Unix(row.RecordedAtUnix, 0).UTC()
		heartRate.add(at, row.HeartRateBpm)
		power.add(at, row.PowerWatts)
	}
	heartRate.best(rider.MaxHeartRateWindow, &into.MaxHeartRateBPM, nil)
	power.best(rider.ThresholdPowerWindow, &into.FunctionalThresholdPowerWatts, rider.ThresholdPower)
}

// sensorSeries is one ride's samples from one sensor. A record without that
// sensor is left out rather than read as a zero, which would read as rest the
// rider never took.
type sensorSeries struct {
	times  []time.Time
	values []float64
}

func (s *sensorSeries) add(at time.Time, value sql.NullFloat64) {
	if !value.Valid {
		return
	}
	s.times = append(s.times, at)
	s.values = append(s.values, value.Float64)
}

// best keeps the series' own best window if it beats what is already held,
// scaled by derive when the suggestion is not the effort itself.
func (s *sensorSeries) best(window time.Duration, into *rider.Value, derive func(float64) float64) {
	mean, ok := rider.BestAverage(s.times, s.values, window)
	if !ok {
		return
	}
	if derive != nil {
		mean = derive(mean)
	}
	if !into.Set || mean > into.Number {
		*into = rider.Set(mean)
	}
}

func riderValue(value sql.NullFloat64) rider.Value {
	if !value.Valid {
		return rider.Value{}
	}

	return rider.Set(value.Float64)
}

func nullRiderValue(value rider.Value) sql.NullFloat64 {
	return sql.NullFloat64{Float64: value.Number, Valid: value.Set}
}
