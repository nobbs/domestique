package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
	"github.com/nobbs/domestique/internal/trainingload"
)

// StoredMetrics is one ride's derived numbers as they come back out.
type StoredMetrics struct {
	Metrics trainingload.Metrics
	ID      int64
}

// ActivitiesAwaitingDerivation lists the target's rides whose stored samples
// could yield something these profile values allow: those never derived, and
// those derived against different values. Newest first, so a rider watching a
// long recompute sees the rides they care about settle first.
func (s *Store) ActivitiesAwaitingDerivation(
	ctx context.Context, targetID string, inputs trainingload.Inputs,
) ([]int64, error) {
	ids, err := s.queries.ListActivitiesAwaitingDerivation(ctx, sqlcgen.ListActivitiesAwaitingDerivationParams{
		TargetSlot:         targetID,
		MaxHeartRate:       inputs.MaxHeartRateBPM,
		RestingHeartRate:   inputs.RestingHeartRateBPM,
		ThresholdHeartRate: inputs.ThresholdHeartRateBPM,
		ThresholdPower:     inputs.FunctionalThresholdPowerWatts,
	})
	if err != nil {
		return nil, fmt.Errorf("listing activities awaiting derivation: %w", err)
	}

	return ids, nil
}

// ActivitySensorSamples reads one ride's heart-rate and power series. A record
// that carried no reading for a sensor is left out of that sensor's series
// rather than read as a zero, which would be rest the rider never took.
func (s *Store) ActivitySensorSamples(
	ctx context.Context, targetID string, id int64,
) (heartRate, power []trainingload.Sample, err error) {
	rows, err := s.queries.ListActivitySensorRecords(ctx, sqlcgen.ListActivitySensorRecordsParams{
		TargetSlot: targetID,
		WorkoutID:  id,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("reading the recorded samples: %w", err)
	}
	for _, row := range rows {
		at := time.Unix(row.RecordedAtUnix, 0).UTC()
		if row.HeartRateBpm.Valid {
			heartRate = append(heartRate, trainingload.Sample{At: at, Value: row.HeartRateBpm.Float64})
		}
		if row.PowerWatts.Valid {
			power = append(power, trainingload.Sample{At: at, Value: row.PowerWatts.Float64})
		}
	}

	return heartRate, power, nil
}

// StoreActivityMetrics replaces one ride's derived numbers. A derivation that
// yielded nothing removes the row instead of writing one full of nulls: no row
// is what "this ride has nothing to say" already means everywhere else.
//
//nolint:gocritic // value param: metrics are plain numbers, copied as cheaply as a pointer.
func (s *Store) StoreActivityMetrics(
	ctx context.Context, targetID string, id int64, metrics trainingload.Metrics,
) error {
	if !metrics.Derived() {
		if err := s.queries.DeleteActivityMetrics(ctx, sqlcgen.DeleteActivityMetricsParams{
			TargetSlot: targetID, WorkoutID: id,
		}); err != nil {
			return fmt.Errorf("clearing the activity metrics: %w", err)
		}

		return nil
	}
	zones := [5]sql.NullFloat64{}
	for index := range zones {
		zones[index] = nullFloat(metrics.Zones[index], metrics.HasZones)
	}
	if err := s.queries.UpsertActivityMetrics(ctx, sqlcgen.UpsertActivityMetricsParams{
		TargetSlot: targetID, WorkoutID: id,
		Zone1Seconds: zones[0], Zone2Seconds: zones[1], Zone3Seconds: zones[2],
		Zone4Seconds: zones[3], Zone5Seconds: zones[4],
		Trimp:                   nullFloat(metrics.TRIMP, metrics.HasTRIMP),
		HeartRateTss:            nullFloat(metrics.HeartRateTSS, metrics.HasHeartRateTSS),
		NormalizedPowerWatts:    nullFloat(metrics.Power.NormalizedWatts, metrics.HasPower),
		IntensityFactor:         nullFloat(metrics.Power.IntensityFactor, metrics.HasPower),
		PowerTss:                nullFloat(metrics.Power.TSS, metrics.HasPower),
		InputMaxHeartRate:       metrics.Inputs.MaxHeartRateBPM,
		InputRestingHeartRate:   metrics.Inputs.RestingHeartRateBPM,
		InputThresholdHeartRate: metrics.Inputs.ThresholdHeartRateBPM,
		InputThresholdPower:     metrics.Inputs.FunctionalThresholdPowerWatts,
		ComputedAtUnix:          time.Now().Unix(),
	}); err != nil {
		return fmt.Errorf("storing the activity metrics: %w", err)
	}

	return nil
}

// ActivityMetrics reads every derived row one target holds, keyed by ride.
func (s *Store) ActivityMetrics(ctx context.Context, targetID string) (map[int64]trainingload.Metrics, error) {
	rows, err := s.queries.ListActivityMetrics(ctx, targetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("reading the activity metrics: %w", err)
	}
	metrics := make(map[int64]trainingload.Metrics, len(rows))
	for index := range rows {
		row := &rows[index]
		one := trainingload.Metrics{
			HasZones:        row.Zone1Seconds.Valid,
			TRIMP:           row.Trimp.Float64,
			HasTRIMP:        row.Trimp.Valid,
			HeartRateTSS:    row.HeartRateTss.Float64,
			HasHeartRateTSS: row.HeartRateTss.Valid,
			HasPower:        row.NormalizedPowerWatts.Valid,
			Power: trainingload.Power{
				NormalizedWatts: row.NormalizedPowerWatts.Float64,
				IntensityFactor: row.IntensityFactor.Float64,
				TSS:             row.PowerTss.Float64,
			},
		}
		one.Zones = trainingload.Zones{
			row.Zone1Seconds.Float64, row.Zone2Seconds.Float64, row.Zone3Seconds.Float64,
			row.Zone4Seconds.Float64, row.Zone5Seconds.Float64,
		}
		metrics[row.WorkoutID] = one
	}

	return metrics, nil
}

// TargetOwner is the subject whose target this is, empty for a slot that
// predates ownership. A slot this deployment does not have is not an error:
// a target can go while work about it is still queued.
func (s *Store) TargetOwner(ctx context.Context, targetID string) (string, error) {
	subject, err := s.queries.GetTargetOwner(ctx, targetID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("reading the target owner: %w", err)
	}

	return subject, nil
}
