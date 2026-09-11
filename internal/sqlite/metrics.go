package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
	"github.com/nobbs/domestique/internal/trainingload"
)

// derivationVersion is which derivation wrote a stored row. Bumped whenever the
// rows before it have gone stale: a figure they cannot hold, or a method or an
// input that changed under them. Those rows are listed again.
//
// 3: rows before it cannot hold the estimate's quality diagnostics.
// 4: the estimate follows a cadence gate and a per-sample air density, and
// the heart-rate figures are worked out from the capped series -- both
// change every figure a row before it holds.
// 5: the estimate's grade-and-speed window is derived per ride from the
// altimeter's own resolution.
// 6: rows before it cannot hold a ride's decoupling or its heat-drift reading.
// 7: rows before it cannot hold the ride's power-duration bests.
// 8: rows before it cannot hold the ride's maximum speed.
// 9: rows before it were worked out over samples the version 2 re-read has
// since replaced, at a time when a re-read left the metrics row in place.
// 10: the estimate carries the inertial term and the drivetrain loss, which
// moves every estimated figure a row before it holds.
// 11: rows before it hold no pedalling share and were worked out at another
// bicycle.
const derivationVersion = 11

// ActivitiesAwaitingDerivation lists the target's rides whose stored samples
// could yield something this derivation now allows: those never derived, those
// derived against different profile values, and those an earlier derivation
// wrote. Newest first, so a rider watching a long recompute sees the rides they
// care about settle first.
func (s *Store) ActivitiesAwaitingDerivation(
	ctx context.Context, targetID string, inputs trainingload.Inputs, coefficients measure.Coefficients,
) ([]int64, error) {
	ids, err := s.queries.ListActivitiesAwaitingDerivation(ctx, sqlcgen.ListActivitiesAwaitingDerivationParams{
		TargetSlot:         targetID,
		MaxHeartRate:       inputs.MaxHeartRateBPM,
		RestingHeartRate:   inputs.RestingHeartRateBPM,
		ThresholdHeartRate: inputs.ThresholdHeartRateBPM,
		ThresholdPower:     inputs.FunctionalThresholdPowerWatts,
		TotalMass:          inputs.TotalMassKG,
		DragArea:           coefficients.DragArea,
		RollingResistance:  coefficients.RollingResistance,
		DerivationVersion:  derivationVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("listing activities awaiting derivation: %w", err)
	}

	return ids, nil
}

// ActivityRideSamples reads one ride's recorded series, split by what each of
// them is for. A record that carried no reading for a sensor is left out of
// that sensor's series rather than read as a zero, which would be rest the
// rider never took.
func (s *Store) ActivityRideSamples(
	ctx context.Context, targetID string, id int64,
) (activity.RideSamples, error) {
	rows, err := s.queries.ListActivitySensorRecords(ctx, sqlcgen.ListActivitySensorRecordsParams{
		TargetSlot: targetID,
		WorkoutID:  id,
	})
	if err != nil {
		return activity.RideSamples{}, fmt.Errorf("reading the recorded samples: %w", err)
	}
	samples := activity.RideSamples{}
	for index := range rows {
		row := &rows[index]
		at := time.Unix(row.RecordedAtUnix, 0).UTC()
		if row.HeartRateBpm.Valid {
			samples.HeartRate = append(samples.HeartRate, trainingload.Sample{At: at, Value: row.HeartRateBpm.Float64})
		}
		if row.CadenceRpm.Valid {
			samples.Cadence = append(samples.Cadence, trainingload.Sample{At: at, Value: row.CadenceRpm.Float64})
		}
		if row.PowerWatts.Valid {
			samples.Power = append(samples.Power, trainingload.Sample{At: at, Value: row.PowerWatts.Float64})
		}
		if row.TemperatureCelsius.Valid {
			samples.Temperature = append(samples.Temperature,
				trainingload.Sample{At: at, Value: row.TemperatureCelsius.Float64})
		}
		// A whole positioned sample — latitude and longitude both, as the track
		// endpoint defines one — plus an altitude and a distance. Without all of
		// them the model has no track, and a trainer ride's "grade" means
		// nothing; with fewer than the track itself needs, the estimate would be
		// shaped by samples nobody is ever shown.
		if row.Latitude.Valid && row.Longitude.Valid && row.AltitudeMetres.Valid && row.DistanceMetres.Valid {
			samples.Track = append(samples.Track, measure.Sample{
				At:                 at,
				DistanceMetres:     row.DistanceMetres.Float64,
				AltitudeMetres:     row.AltitudeMetres.Float64,
				CadenceRPM:         row.CadenceRpm.Float64,
				HasCadence:         row.CadenceRpm.Valid,
				TemperatureCelsius: row.TemperatureCelsius.Float64,
				HasTemperature:     row.TemperatureCelsius.Valid,
			})
			samples.TrackRecords = append(samples.TrackRecords, row.RecordIndex)
		}
	}
	samples.Speed, samples.MovingIntervals = speedFromRows(rows)

	return samples, nil
}

// speedFromRows is the device's own speed where any row carried one, else the
// odometer's distance over time; never a mix of the two within one ride.
// moving is the same ride's moving intervals, read the way each source
// requires: a device speed is an instantaneous reading, so a step only
// counts as moving where both readings that bracket it are positive; the
// odometer's own speed already names the rate of the step it ends. Nil
// rather than a known-empty list where a device speed field was present but
// never once positive throughout -- a real quirk of some trainers -- since
// that says the field cannot be trusted to name when the ride moved at all,
// not that it never did.
func speedFromRows(rows []sqlcgen.ListActivitySensorRecordsRow) (speed []trainingload.Sample, moving []measure.Interval) {
	hasDeviceSpeed := false
	for index := range rows {
		if rows[index].SpeedMs.Valid {
			hasDeviceSpeed = true

			break
		}
	}
	var samples []trainingload.Sample
	if hasDeviceSpeed {
		everPositive := false
		for index := range rows {
			row := &rows[index]
			if !row.SpeedMs.Valid {
				continue
			}
			kmh := row.SpeedMs.Float64 * 3.6
			everPositive = everPositive || kmh > 0
			samples = append(samples, trainingload.Sample{At: time.Unix(row.RecordedAtUnix, 0).UTC(), Value: kmh})
		}
		if everPositive {
			moving = measure.MovingIntervalsFromInstantaneous(samples)
		}
	} else {
		var previous activity.DistanceStep
		anchored := false
		for index := range rows {
			row := &rows[index]
			current := activity.DistanceStep{
				At:       time.Unix(row.RecordedAtUnix, 0).UTC(),
				Distance: row.DistanceMetres.Float64, Known: row.DistanceMetres.Valid,
			}
			if index > 0 {
				if kmh, ok := activity.DistanceSpeedKmh(previous, current); ok {
					if !anchored {
						// A speed reading names the step it ends, not the one
						// it starts: without a reading at the step's own
						// start, a consumer pairing consecutive readings
						// (measure.MovingIntervals) never sees this first
						// step at all. One anchor, at the step's start with
						// its own rate, gives it a start to pair from.
						samples = append(samples, trainingload.Sample{At: previous.At, Value: kmh})
						anchored = true
					}
					samples = append(samples, trainingload.Sample{At: current.At, Value: kmh})
				}
			}
			previous = current
		}
		moving = measure.MovingIntervals(samples)
	}

	return capSpeedSamples(samples), moving
}

// capSpeedSamples drops every reading above measure.MaxPlausibleSpeedKmh: a
// data fault, not the ride's own peak.
func capSpeedSamples(samples []trainingload.Sample) []trainingload.Sample {
	var capped []trainingload.Sample
	for _, sample := range samples {
		if sample.Value <= measure.MaxPlausibleSpeedKmh {
			capped = append(capped, sample)
		}
	}

	return capped
}

// setEstimatedPowerSQL is prepared once per ride rather than generated by
// sqlc, for the reason insertActivityRecordSQL is: a ride holds thousands of
// samples, and compiling the update per row is most of the cost of storing one.
const setEstimatedPowerSQL = `UPDATE activity_records SET estimated_power_watts = ?
WHERE target_slot = ? AND workout_id = ? AND record_index = ?`

// StoreEstimatedPower replaces one ride's estimated power series, in one
// transaction so a partial rewrite is never left behind as complete. An empty
// series clears whatever was there, which is what a ride that has stopped
// yielding an estimate needs. The ride's route match goes with it: its climb
// attempts were read from the series being replaced, so the match is redone.
func (s *Store) StoreEstimatedPower(
	ctx context.Context, targetID string, id int64, recordIndices []int64, estimates []measure.Estimate,
) error {
	transaction, beginErr := s.database.BeginTx(ctx, nil)
	if beginErr != nil {
		return fmt.Errorf("starting the estimated power write: %w", beginErr)
	}
	defer rollback(transaction)
	if err := storeEstimatedPower(ctx, transaction, s.queries.WithTx(transaction), targetID, id, recordIndices, estimates); err != nil {
		return err
	}
	if commitErr := transaction.Commit(); commitErr != nil {
		return fmt.Errorf("committing the estimated power: %w", commitErr)
	}

	return nil
}

// storeEstimatedPower is StoreEstimatedPower's body, run against a
// transaction and queries the caller already opened, so StoreRideDerivation
// can share one transaction with storeActivityMetrics.
func storeEstimatedPower(
	ctx context.Context, transaction *sql.Tx, queries *sqlcgen.Queries,
	targetID string, id int64, recordIndices []int64, estimates []measure.Estimate,
) error {
	if len(recordIndices) != len(estimates) {
		return errors.New("an estimate per record or none")
	}
	// Cleared first, so a record that no longer yields an estimate does not keep
	// the one it had from a mass the rider has since changed.
	cleared, clearErr := queries.ClearEstimatedPower(ctx, sqlcgen.ClearEstimatedPowerParams{
		TargetSlot: targetID, WorkoutID: id,
	})
	if clearErr != nil {
		return fmt.Errorf("clearing the estimated power: %w", clearErr)
	}
	writesAnEstimate := false
	for _, estimate := range estimates {
		if estimate.Known {
			writesAnEstimate = true

			break
		}
	}
	// A ride whose series neither was nor becomes anything keeps its match:
	// a metered ride's climbs owe the estimate nothing. len(estimates) alone
	// is not that test -- it is nonzero whenever the track has samples, known
	// or not, so a track that yielded no known estimate at all must not
	// count as one that did.
	if cleared > 0 || writesAnEstimate {
		if err := queries.DeleteActivityClimbAttempts(ctx, sqlcgen.DeleteActivityClimbAttemptsParams{
			TargetSlot: targetID, WorkoutID: id,
		}); err != nil {
			return fmt.Errorf("forgetting the climb attempts: %w", err)
		}
		if err := queries.DeleteActivityRouteMatch(ctx, sqlcgen.DeleteActivityRouteMatchParams{
			TargetSlot: targetID, WorkoutID: id,
		}); err != nil {
			return fmt.Errorf("forgetting the route match: %w", err)
		}
	}
	// Prepared once for the whole ride, as the sample insert is: a long ride is
	// thousands of these, and preparing each one costs more than running it.
	update, prepareErr := transaction.PrepareContext(ctx, setEstimatedPowerSQL)
	if prepareErr != nil {
		return fmt.Errorf("preparing the estimated power update: %w", prepareErr)
	}
	defer closeStatement(update)
	for index, estimate := range estimates {
		if !estimate.Known {
			continue
		}
		if _, execErr := update.ExecContext(ctx, estimate.Watts, targetID, id, recordIndices[index]); execErr != nil {
			return fmt.Errorf("recording an estimated power: %w", execErr)
		}
	}

	return nil
}

// StoreActivityMetrics replaces one ride's derived numbers. A derivation that
// yielded nothing removes the row instead of writing one full of nulls: no row
// is what "this ride has nothing to say" already means everywhere else.
//
//nolint:gocritic // value param: metrics are plain numbers, copied as cheaply as a pointer.
func (s *Store) StoreActivityMetrics(
	ctx context.Context, targetID string, id int64, stored activity.RideMetrics,
) error {
	return storeActivityMetrics(ctx, s.queries, targetID, id, stored)
}

// storeActivityMetrics is StoreActivityMetrics' body, run against the
// queries the caller already opened, so StoreRideDerivation can share one
// transaction with storeEstimatedPower.
//
//nolint:gocritic // value param: metrics are plain numbers, copied as cheaply as a pointer.
func storeActivityMetrics(
	ctx context.Context, queries *sqlcgen.Queries, targetID string, id int64, stored activity.RideMetrics,
) error {
	metrics, averages := stored.Load, stored.Averages
	if !stored.Derived() {
		if err := queries.DeleteActivityMetrics(ctx, sqlcgen.DeleteActivityMetricsParams{
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
	bests := stored.PowerBests
	if err := queries.UpsertActivityMetrics(ctx, sqlcgen.UpsertActivityMetricsParams{
		TargetSlot: targetID, WorkoutID: id,
		Zone1Seconds: zones[0], Zone2Seconds: zones[1], Zone3Seconds: zones[2],
		Zone4Seconds: zones[3], Zone5Seconds: zones[4],
		Trimp:                   nullFloat(metrics.TRIMP, metrics.HasTRIMP),
		HeartRateTss:            nullFloat(metrics.HeartRateTSS, metrics.HasHeartRateTSS),
		NormalizedPowerWatts:    nullFloat(metrics.Power.NormalizedWatts, metrics.HasPower),
		IntensityFactor:         nullFloat(metrics.Power.IntensityFactor, metrics.HasPower),
		PowerTss:                nullFloat(metrics.Power.TSS, metrics.HasPower),
		EstimatedPowerWatts:     nullFloat(metrics.EstimatedPowerWatts, metrics.HasEstimatedPower),
		EstimatedPedallingShare: nullFloat(stored.EstimatedPedallingShare, stored.HasEstimatedPedallingShare),
		AverageHeartRateBpm:     nullFloat(averages.HeartRateBPM, averages.HasHeartRate),
		MaxHeartRateBpm:         nullFloat(averages.MaxHeartRateBPM, averages.HasHeartRate),
		AverageCadenceRpm:       nullFloat(averages.CadenceRPM, averages.HasCadence),
		AveragePowerWatts:       nullFloat(averages.PowerWatts, averages.HasPower),
		MaxSpeedKmh:             nullFloat(averages.MaxSpeedKmh, averages.HasSpeed),
		DecouplingPercent:       nullFloat(stored.Decoupling.Percent, stored.Decoupling.Known),
		HeatDriftHeartRateBpm:   nullFloat(stored.HeatDrift.HeartRateBPM, stored.HeatDrift.Known),
		HeatDriftTemperatureCelsius: nullFloat(
			stored.HeatDrift.TemperatureCelsius, stored.HeatDrift.Known),
		HeatDriftSamples:        nullInt(int64(stored.HeatDrift.Samples), stored.HeatDrift.Known),
		BestPower5s:             nullFloat(bests.Watts[0], bests.Held[0]),
		BestPower30s:            nullFloat(bests.Watts[1], bests.Held[1]),
		BestPower60s:            nullFloat(bests.Watts[2], bests.Held[2]),
		BestPower300s:           nullFloat(bests.Watts[3], bests.Held[3]),
		BestPower1200s:          nullFloat(bests.Watts[4], bests.Held[4]),
		BestPower3600s:          nullFloat(bests.Watts[5], bests.Held[5]),
		InputMaxHeartRate:       metrics.Inputs.MaxHeartRateBPM,
		InputRestingHeartRate:   metrics.Inputs.RestingHeartRateBPM,
		InputThresholdHeartRate: metrics.Inputs.ThresholdHeartRateBPM,
		InputThresholdPower:     metrics.Inputs.FunctionalThresholdPowerWatts,
		InputTotalMass:          metrics.Inputs.TotalMassKG,
		InputDragArea:           stored.Coefficients.DragArea,
		InputRollingResistance:  stored.Coefficients.RollingResistance,
		DerivationVersion:       derivationVersion,
		ComputedAtUnix:          time.Now().Unix(),
	}); err != nil {
		return fmt.Errorf("storing the activity metrics: %w", err)
	}

	return nil
}

// StoreRideDerivation replaces one ride's estimated power series and its
// derived metrics row together, in one transaction: written separately, a
// failure between the two could leave the track endpoint serving an estimate
// series no metrics row stands behind any longer, or the reverse.
//
//nolint:gocritic // value param: metrics are plain numbers, copied as cheaply as a pointer.
func (s *Store) StoreRideDerivation(
	ctx context.Context, targetID string, id int64,
	recordIndices []int64, estimates []measure.Estimate, metrics activity.RideMetrics,
) error {
	transaction, beginErr := s.database.BeginTx(ctx, nil)
	if beginErr != nil {
		return fmt.Errorf("starting the ride derivation write: %w", beginErr)
	}
	defer rollback(transaction)
	queries := s.queries.WithTx(transaction)
	// The series first: a metrics row is what says a ride has been derived,
	// so it must not appear before the samples it describes are in place.
	if err := storeEstimatedPower(ctx, transaction, queries, targetID, id, recordIndices, estimates); err != nil {
		return err
	}
	if err := storeActivityMetrics(ctx, queries, targetID, id, metrics); err != nil {
		return err
	}
	if commitErr := transaction.Commit(); commitErr != nil {
		return fmt.Errorf("committing the ride derivation write: %w", commitErr)
	}

	return nil
}

// ActivityMetrics reads every derived row one target holds, keyed by ride.
func (s *Store) ActivityMetrics(ctx context.Context, targetID string) (map[int64]activity.RideMetrics, error) {
	rows, err := s.queries.ListActivityMetrics(ctx, targetID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("reading the activity metrics: %w", err)
	}
	metrics := make(map[int64]activity.RideMetrics, len(rows))
	for index := range rows {
		row := &rows[index]
		one := trainingload.Metrics{
			HasZones:            row.Zone1Seconds.Valid,
			TRIMP:               row.Trimp.Float64,
			HasTRIMP:            row.Trimp.Valid,
			HeartRateTSS:        row.HeartRateTss.Float64,
			HasHeartRateTSS:     row.HeartRateTss.Valid,
			HasPower:            row.NormalizedPowerWatts.Valid,
			EstimatedPowerWatts: row.EstimatedPowerWatts.Float64,
			HasEstimatedPower:   row.EstimatedPowerWatts.Valid,
			Power: trainingload.Power{
				NormalizedWatts: row.NormalizedPowerWatts.Float64,
				IntensityFactor: row.IntensityFactor.Float64,
				TSS:             row.PowerTss.Float64,
			},
			// Only the two rates the zones were cut at: the rest of the inputs
			// are read back by the staleness check, from its own query.
			Inputs: trainingload.Inputs{
				MaxHeartRateBPM:       row.InputMaxHeartRate,
				ThresholdHeartRateBPM: row.InputThresholdHeartRate,
			},
		}
		one.Zones = trainingload.Zones{
			row.Zone1Seconds.Float64, row.Zone2Seconds.Float64, row.Zone3Seconds.Float64,
			row.Zone4Seconds.Float64, row.Zone5Seconds.Float64,
		}
		metrics[row.WorkoutID] = activity.RideMetrics{
			Load: one,
			Averages: activity.RideAverages{
				HeartRateBPM:    row.AverageHeartRateBpm.Float64,
				MaxHeartRateBPM: row.MaxHeartRateBpm.Float64,
				CadenceRPM:      row.AverageCadenceRpm.Float64,
				PowerWatts:      row.AveragePowerWatts.Float64,
				MaxSpeedKmh:     row.MaxSpeedKmh.Float64,
				HasHeartRate:    row.AverageHeartRateBpm.Valid,
				HasCadence:      row.AverageCadenceRpm.Valid,
				HasPower:        row.AveragePowerWatts.Valid,
				HasSpeed:        row.MaxSpeedKmh.Valid,
			},
			EstimatedPedallingShare:    row.EstimatedPedallingShare.Float64,
			HasEstimatedPedallingShare: row.EstimatedPedallingShare.Valid,
			Coefficients: measure.Coefficients{
				DragArea: row.InputDragArea, RollingResistance: row.InputRollingResistance,
			},
			Decoupling: activity.Decoupling{
				Percent: row.DecouplingPercent.Float64,
				Known:   row.DecouplingPercent.Valid,
			},
			HeatDrift: activity.HeatDrift{
				HeartRateBPM:       row.HeatDriftHeartRateBpm.Float64,
				TemperatureCelsius: row.HeatDriftTemperatureCelsius.Float64,
				Samples:            int(row.HeatDriftSamples.Int64),
				// A reading is the whole of what was written: a heart rate with
				// no temperature beside it is not a point on a season's drift,
				// and one with no count is a row half a derivation wrote.
				Known: row.HeatDriftHeartRateBpm.Valid && row.HeatDriftTemperatureCelsius.Valid &&
					row.HeatDriftSamples.Valid,
			},
			PowerBests: powerCurveOf(&[...]sql.NullFloat64{
				row.BestPower5s, row.BestPower30s, row.BestPower60s,
				row.BestPower300s, row.BestPower1200s, row.BestPower3600s,
			}),
		}
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

// ClearActivityMetrics removes every derived row one target holds and reports
// how many went. It is what a rider clearing their whole profile leaves
// behind: numbers worked out from parameters nobody holds any more must not
// go on being served. The estimate series this same profile fed goes with
// it, and the route matches and climb attempts an estimate shaped -- otherwise
// the track and library endpoints would keep serving an estimate, and the
// climbs it shaped, after the derivation that produced it has nothing left to
// stand on. A metered ride's own match owes the profile nothing and is left
// standing: only the estimate-dependent history is cleared, not the whole
// target's. One transaction: a partial clear must never leave an estimate
// outliving the metrics row it was derived beside.
func (s *Store) ClearActivityMetrics(ctx context.Context, targetID string) (int, error) {
	transaction, beginErr := s.database.BeginTx(ctx, nil)
	if beginErr != nil {
		return 0, fmt.Errorf("starting the activity metrics clear: %w", beginErr)
	}
	defer rollback(transaction)
	queries := s.queries.WithTx(transaction)
	removed, err := queries.ClearActivityMetrics(ctx, targetID)
	if err != nil {
		return 0, fmt.Errorf("clearing the activity metrics: %w", err)
	}
	// Cleared before the series it was read from, the same order
	// StoreEstimatedPower keeps for one ride: a climb attempt naming an
	// estimate that has since been cleared is never left standing.
	if _, err := queries.ClearEstimatedActivityClimbAttemptsForTarget(ctx, targetID); err != nil {
		return 0, fmt.Errorf("clearing activity climb attempts: %w", err)
	}
	if _, err := queries.ClearEstimatedActivityRouteMatchesForTarget(ctx, targetID); err != nil {
		return 0, fmt.Errorf("clearing activity route matches: %w", err)
	}
	if _, err := queries.ClearEstimatedPowerForTarget(ctx, targetID); err != nil {
		return 0, fmt.Errorf("clearing the estimated power: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return 0, fmt.Errorf("committing the activity metrics clear: %w", err)
	}

	return int(removed), nil
}

// ActivityRideLoads is every derived ride of one target, with the moment it was
// ridden, oldest first. It is the whole of what the fitness timeline folds: one
// row per ride rather than per hour or per sample, so a rider's whole history
// is a few hundred rows.
func (s *Store) ActivityRideLoads(ctx context.Context, targetID string) ([]trainingload.RideLoad, error) {
	rows, err := s.queries.ListActivityRideLoads(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("reading the activity ride loads: %w", err)
	}
	loads := make([]trainingload.RideLoad, 0, len(rows))
	for index := range rows {
		row := &rows[index]
		metrics := trainingload.Metrics{
			TRIMP: row.Trimp.Float64, HasTRIMP: row.Trimp.Valid,
			HeartRateTSS: row.HeartRateTss.Float64, HasHeartRateTSS: row.HeartRateTss.Valid,
			Power:    trainingload.Power{TSS: row.PowerTss.Float64},
			HasPower: row.PowerTss.Valid,
			Zones: trainingload.Zones{
				row.Zone1Seconds.Float64, row.Zone2Seconds.Float64, row.Zone3Seconds.Float64,
				row.Zone4Seconds.Float64, row.Zone5Seconds.Float64,
			},
		}
		loads = append(loads, trainingload.LoadOf(time.Unix(row.StartedAtUnix, 0).UTC(), &metrics))
	}

	return loads, nil
}

// nullInt renders a count the way a nullable column wants it: absent rather
// than nought where the figure was never worked out.
func nullInt(value int64, valid bool) sql.NullInt64 {
	return sql.NullInt64{Int64: value, Valid: valid}
}

// PowerCurve folds the rider's own stored per-ride bests into one curve: the
// best each duration reached over the rides ridden in the half-open window.
// Read over the caller's own targets, never pooled across riders.
func (s *Store) PowerCurve(
	ctx context.Context, targetIDs []string, from, to time.Time,
) (rider.PowerCurve, error) {
	// sqlc expands the slice into the IN list, and an empty one is not SQL.
	if len(targetIDs) == 0 {
		return rider.PowerCurve{}, nil
	}
	rows, err := s.queries.ListPowerBests(ctx, sqlcgen.ListPowerBestsParams{
		FromUnix:    from.Truncate(time.Second).Unix(),
		ToUnix:      to.Truncate(time.Second).Unix(),
		TargetSlots: targetIDs,
	})
	if err != nil {
		return rider.PowerCurve{}, fmt.Errorf("reading the stored power bests: %w", err)
	}
	curve := rider.PowerCurve{}
	for index := range rows {
		row := &rows[index]
		ride := powerCurveOf(&[...]sql.NullFloat64{
			row.BestPower5s, row.BestPower30s, row.BestPower60s,
			row.BestPower300s, row.BestPower1200s, row.BestPower3600s,
		})
		for point, held := range ride.Held {
			if held && (!curve.Held[point] || ride.Watts[point] > curve.Watts[point]) {
				curve.Watts[point], curve.Held[point] = ride.Watts[point], true
			}
		}
	}

	return curve, nil
}

// powerCurveOf reads one row's stored bests, in the order the durations are
// declared in. A duration the ride was never long enough for stays absent.
func powerCurveOf(columns *[rider.PowerCurvePoints]sql.NullFloat64) rider.PowerCurve {
	curve := rider.PowerCurve{}
	for point, column := range columns {
		curve.Watts[point], curve.Held[point] = column.Float64, column.Valid
	}

	return curve
}
