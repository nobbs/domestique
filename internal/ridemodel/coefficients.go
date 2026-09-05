// Package ridemodel is the forward model that turns a stage's geometry and a
// calibrated coefficient pair into a predicted moving time. It is a pure
// function of its inputs, and dev/fitter's benchmark runs exactly this model.
package ridemodel

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strconv"
	"time"
)

// Coefficients are the values the model can legitimately vary: the two fitted
// terms, and the metadata describing the fit that produced them. They live as
// one row in the state database, replaced by a calibration.
type Coefficients struct {
	Fingerprint       string
	CalibrationCutoff string // "2025-08-01"; the last calibration ride's date, recorded as-is rather than computed from anything at load time
	SecondsPerKM      float64
	SecondsPerAscentM float64
	// EvaluatedRides, BiasPercent, MAEPercent, and P90Percent are the profile's
	// measured unseen-route error, pooled across rolling-origin folds. Optional:
	// EvaluatedRides == 0 is the sentinel for "not measured", which a real
	// evaluation can never produce.
	EvaluatedRides int
	BiasPercent    float64
	MAEPercent     float64
	P90Percent     float64
	// TrainingWindowMonths is how far back the fit that produced SecondsPerKM and
	// SecondsPerAscentM reached. Nothing here reads it, but without it a profile
	// cannot be reproduced from its own metadata. Zero means all history.
	TrainingWindowMonths int
}

// Default is the built-in pair every deployment predicts with until a
// calibration replaces it, carrying no measured error of its own.
func Default() Coefficients {
	return Coefficients{SecondsPerKM: 145.3578, SecondsPerAscentM: 3.2190}.WithFingerprint()
}

// HasValidation reports whether these coefficients carry a measured
// unseen-route benchmark result.
//
//nolint:gocritic // value receiver: Coefficients is immutable once loaded, and a pointer would let a caller mutate the shared instance mid-prediction.
func (c Coefficients) HasValidation() bool {
	return c.EvaluatedRides > 0
}

// Validate rejects a pair that could not have come from a real fit. The metrics
// are optional, but a value present must be one that could mean something.
//
//nolint:gocritic // value receiver: see HasValidation.
func (c Coefficients) Validate() error {
	if math.IsNaN(c.SecondsPerKM) || math.IsInf(c.SecondsPerKM, 0) ||
		math.IsNaN(c.SecondsPerAscentM) || math.IsInf(c.SecondsPerAscentM, 0) {
		return errors.New("ridemodel: the coefficient terms must be finite")
	}
	if c.SecondsPerKM <= 0 {
		return errors.New("ridemodel: seconds_per_km must be positive")
	}
	if c.SecondsPerAscentM <= 0 {
		return errors.New("ridemodel: seconds_per_ascent_m must be positive")
	}
	if c.CalibrationCutoff != "" {
		if _, err := time.Parse(time.DateOnly, c.CalibrationCutoff); err != nil {
			return fmt.Errorf("ridemodel: calibration_cutoff must be a date in %s form: %w", time.DateOnly, err)
		}
	}
	if c.EvaluatedRides < 0 {
		return errors.New("ridemodel: evaluated_rides must not be negative")
	}
	if c.EvaluatedRides == 0 && (c.BiasPercent != 0 || c.MAEPercent != 0 || c.P90Percent != 0) {
		return errors.New("ridemodel: a measured error needs the rides it was measured over")
	}
	if c.TrainingWindowMonths < 0 {
		return errors.New("ridemodel: training_window_months must not be negative")
	}
	for _, percent := range []float64{c.BiasPercent, c.MAEPercent, c.P90Percent} {
		if math.IsNaN(percent) || math.IsInf(percent, 0) {
			return errors.New("ridemodel: bias_percent, mae_percent, and p90_percent must be finite")
		}
	}
	if c.MAEPercent < 0 || c.P90Percent < 0 {
		return errors.New("ridemodel: mae_percent and p90_percent must not be negative")
	}

	return nil
}

// WithFingerprint returns the same coefficients stamped with the fingerprint a
// cached prediction is keyed by. Only the two terms enter it: the metrics
// describe a fit rather than a prediction, so re-measuring one must not
// invalidate a stored duration.
//
//nolint:gocritic // value receiver: see HasValidation.
func (c Coefficients) WithFingerprint() Coefficients {
	canonical := strconv.FormatFloat(c.SecondsPerKM, 'g', -1, 64) + "\n" +
		strconv.FormatFloat(c.SecondsPerAscentM, 'g', -1, 64)
	c.Fingerprint = fingerprintOf(modelVersion, []byte(canonical))

	return c
}

// fingerprintOf hashes version and data with version's length written ahead of
// it, so the two can never be reinterpreted as a different (version, data) pair
// that concatenates to the same bytes.
func fingerprintOf(version string, data []byte) string {
	digest := sha256.New()
	var versionLength [8]byte
	binary.BigEndian.PutUint64(versionLength[:], uint64(len(version)))
	// hash.Hash.Write never returns an error — the interface carries one only
	// because it embeds io.Writer.
	_, _ = digest.Write(versionLength[:])
	_, _ = digest.Write([]byte(version))
	_, _ = digest.Write(data)

	return hex.EncodeToString(digest.Sum(nil))
}
