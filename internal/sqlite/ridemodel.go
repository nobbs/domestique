package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// RideModelCoefficients reads the calibrated pair in force. Absent is the
// ordinary state of a service nothing has calibrated yet, not a failure.
func (s *Store) RideModelCoefficients(ctx context.Context) (ridemodel.Coefficients, bool, error) {
	row, err := s.queries.GetRideModelCoefficients(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return ridemodel.Coefficients{}, false, nil
	}
	if err != nil {
		return ridemodel.Coefficients{}, false, fmt.Errorf("reading the ride model coefficients: %w", err)
	}
	cutoff := ""
	if row.CalibrationCutoffUnix.Valid {
		cutoff = time.Unix(row.CalibrationCutoffUnix.Int64, 0).UTC().Format(time.DateOnly)
	}

	return ridemodel.Coefficients{
		CalibrationCutoff:    cutoff,
		SecondsPerKM:         row.SecondsPerKm,
		SecondsPerAscentM:    row.SecondsPerAscentM,
		EvaluatedRides:       int(row.EvaluatedRides),
		BiasPercent:          row.BiasPercent,
		MAEPercent:           row.MaePercent,
		P90Percent:           row.P90Percent,
		TrainingWindowMonths: int(row.TrainingWindowMonths),
	}.WithFingerprint(), true, nil
}

// StoreRideModelCoefficients replaces the pair in force with the one a
// calibration produced.
//
//nolint:gocritic // value param: ridemodel.Coefficients is immutable by contract.
func (s *Store) StoreRideModelCoefficients(
	ctx context.Context, coefficients ridemodel.Coefficients, now time.Time,
) error {
	if err := coefficients.Validate(); err != nil {
		return fmt.Errorf("refusing to store ride model coefficients: %w", err)
	}
	cutoff := sql.NullInt64{}
	if coefficients.CalibrationCutoff != "" {
		day, err := time.ParseInLocation(time.DateOnly, coefficients.CalibrationCutoff, time.UTC)
		if err != nil {
			return fmt.Errorf("storing the ride model coefficients: %w", err)
		}
		cutoff = sql.NullInt64{Int64: day.Unix(), Valid: true}
	}

	if err := s.queries.UpsertRideModelCoefficients(ctx, sqlcgen.UpsertRideModelCoefficientsParams{
		SecondsPerKm:          coefficients.SecondsPerKM,
		SecondsPerAscentM:     coefficients.SecondsPerAscentM,
		CalibrationCutoffUnix: cutoff,
		EvaluatedRides:        int64(coefficients.EvaluatedRides),
		BiasPercent:           coefficients.BiasPercent,
		MaePercent:            coefficients.MAEPercent,
		P90Percent:            coefficients.P90Percent,
		TrainingWindowMonths:  int64(coefficients.TrainingWindowMonths),
		UpdatedAtUnix:         now.Unix(),
	}); err != nil {
		return fmt.Errorf("storing the ride model coefficients: %w", err)
	}

	return nil
}
