package ridemodel_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fitNow() time.Time {
	return time.Date(2026, time.September, 5, 12, 0, 0, 0, time.UTC)
}

// syntheticRides are rides whose moving time is exactly the pair below, so a
// correct fit recovers it rather than merely landing near it.
func syntheticRides(count int, secondsPerKM, secondsPerAscentM float64) []ridemodel.Ride {
	rides := make([]ridemodel.Ride, 0, count)
	for index := range count {
		distance := float64(20_000 + 3_000*index)
		ascent := float64(100 + 90*(count-index))
		rides = append(rides, ridemodel.Ride{
			StartedAt:      fitNow().AddDate(0, 0, -count+index),
			DistanceMetres: distance,
			AscentMetres:   ascent,
			MovingSeconds:  secondsPerKM*(distance/1000) + secondsPerAscentM*ascent,
		})
	}

	return rides
}

// ridesEndingAt is syntheticRides moved to end on end, so one corpus can hold
// two eras of the same rider.
func ridesEndingAt(count int, secondsPerKM, secondsPerAscentM float64, end time.Time) []ridemodel.Ride {
	rides := syntheticRides(count, secondsPerKM, secondsPerAscentM)
	shift := end.Sub(fitNow())
	for index := range rides {
		rides[index].StartedAt = rides[index].StartedAt.Add(shift)
	}

	return rides
}

func TestFitRecoversThePairAnExactCorpusWasBuiltFrom(t *testing.T) {
	t.Parallel()

	fitted, err := ridemodel.Fit(syntheticRides(14, 150, 4), fitNow())
	require.NoError(t, err, "Fit()")
	assert.InDelta(t, 150.0, fitted.SecondsPerKM, 1e-6, "seconds per km")
	assert.InDelta(t, 4.0, fitted.SecondsPerAscentM, 1e-6, "seconds per ascent metre")
	assert.Equal(t, 14, fitted.EvaluatedRides, "evaluated rides")
	assert.Equal(t, "2026-09-04", fitted.CalibrationCutoff, "calibration cutoff")
	assert.Equal(t, ridemodel.TrainingWindowMonths, fitted.TrainingWindowMonths, "training window")
	assert.NotEmpty(t, fitted.Fingerprint, "fingerprint")
	assert.InDelta(t, 0.0, fitted.MAEPercent, 1e-6, "in-sample error of an exact corpus")
	require.NoError(t, fitted.Validate(), "Validate()")
}

// The Huber weighting is the whole point of the robust fit: one ride recorded
// at twice its real moving time must not drag the pair with it.
func TestFitBarelyMovesForOneOutlyingRide(t *testing.T) {
	t.Parallel()

	rides := syntheticRides(14, 150, 4)
	rides[3].MovingSeconds *= 2

	fitted, err := ridemodel.Fit(rides, fitNow())
	require.NoError(t, err, "Fit()")
	assert.InDelta(t, 150.0, fitted.SecondsPerKM, 3.0, "seconds per km")
	assert.InDelta(t, 4.0, fitted.SecondsPerAscentM, 0.5, "seconds per ascent metre")
}

func TestFitRefusesBelowTheRideFloor(t *testing.T) {
	t.Parallel()

	_, err := ridemodel.Fit(syntheticRides(ridemodel.MinRides-1, 150, 4), fitNow())
	require.ErrorIs(t, err, ridemodel.ErrTooFewRides)
}

// Flat rides price kilometres and say nothing about climbing, so the fit is
// refused rather than reported as a calibration of both terms.
func TestFitRefusesACorpusThatNeverClimbs(t *testing.T) {
	t.Parallel()

	rides := syntheticRides(14, 150, 4)
	for index := range rides {
		rides[index].MovingSeconds -= 4 * rides[index].AscentMetres
		rides[index].AscentMetres = 0
	}

	_, err := ridemodel.Fit(rides, fitNow())
	require.ErrorIs(t, err, ridemodel.ErrDegenerate)
}

// Climbing in strict proportion to distance leaves the two terms with one
// degree of freedom between them: the corpus cannot say which is which.
func TestFitRefusesACorpusWhoseAscentTracksDistance(t *testing.T) {
	t.Parallel()

	rides := syntheticRides(14, 150, 4)
	for index := range rides {
		rides[index].AscentMetres = rides[index].DistanceMetres / 100
		rides[index].MovingSeconds = 150*(rides[index].DistanceMetres/1000) + 4*rides[index].AscentMetres
	}

	_, err := ridemodel.Fit(rides, fitNow())
	require.ErrorIs(t, err, ridemodel.ErrDegenerate)
}

// A ride dated past the fit's own clock is a recording fault, not history: it
// is dropped, which here takes the corpus under the floor.
func TestFitDropsARideDatedAfterNow(t *testing.T) {
	t.Parallel()

	rides := syntheticRides(ridemodel.MinRides, 150, 4)
	rides[0].StartedAt = fitNow().Add(time.Hour)

	_, err := ridemodel.Fit(rides, fitNow())
	require.ErrorIs(t, err, ridemodel.ErrTooFewRides)
}

func TestFitDropsFragmentsAndImpossibleTotals(t *testing.T) {
	t.Parallel()

	rides := append(syntheticRides(ridemodel.MinRides, 150, 4),
		ridemodel.Ride{StartedAt: fitNow(), DistanceMetres: 900, MovingSeconds: 300, AscentMetres: 5},
		ridemodel.Ride{StartedAt: fitNow(), DistanceMetres: 5000, MovingSeconds: 30, AscentMetres: 5},
		ridemodel.Ride{StartedAt: fitNow(), DistanceMetres: 5000, MovingSeconds: 900, AscentMetres: -1},
	)

	fitted, err := ridemodel.Fit(rides, fitNow())
	require.NoError(t, err, "Fit()")
	assert.Equal(t, ridemodel.MinRides, fitted.EvaluatedRides, "the fragments were dropped")
}

// Every ride is recorded ten percent slower than the pair predicts, so the fit
// absorbs the ten percent and the reported error is what is left.
func TestFitReportsTheErrorOfThePairItFound(t *testing.T) {
	t.Parallel()

	rides := syntheticRides(14, 150, 4)
	for index := range rides {
		rides[index].MovingSeconds *= 1.1
	}

	fitted, err := ridemodel.Fit(rides, fitNow())
	require.NoError(t, err, "Fit()")
	assert.InDelta(t, 165.0, fitted.SecondsPerKM, 1e-6, "seconds per km")
	assert.InDelta(t, 0.0, fitted.BiasPercent, 1e-6, "bias percent")
	assert.InDelta(t, 0.0, fitted.P90Percent, 1e-6, "p90 percent")
}

// A rider whose form has durably moved must be priced as they ride now, not
// against every season pooled together: the rides behind the window are not
// weighted down, they are out.
func TestFitReadsOnlyTheWindowWhenTheRiderHasChanged(t *testing.T) {
	t.Parallel()

	corpus := append(
		ridesEndingAt(14, 260, 9, fitNow().AddDate(-2, 0, 0)),
		ridesEndingAt(14, 150, 4, fitNow())...,
	)

	fitted, err := ridemodel.Fit(corpus, fitNow())
	require.NoError(t, err, "Fit()")
	assert.InDelta(t, 150.0, fitted.SecondsPerKM, 1e-6, "seconds per km")
	assert.InDelta(t, 4.0, fitted.SecondsPerAscentM, 1e-6, "seconds per ascent metre")
	assert.Equal(t, 14, fitted.EvaluatedRides, "only the window's rides are fitted")
	assert.Equal(t, ridemodel.TrainingWindowMonths, fitted.TrainingWindowMonths, "training window")
}

// The window is a bound rather than a guillotine: a rider who has ridden for
// less than a year, or only occasionally, still gets a fit.
func TestFitReachesPastTheWindowRatherThanRefuse(t *testing.T) {
	t.Parallel()

	corpus := append(
		ridesEndingAt(14, 150, 4, fitNow().AddDate(-2, 0, 0)),
		ridesEndingAt(4, 150, 4, fitNow())...,
	)

	fitted, err := ridemodel.Fit(corpus, fitNow())
	require.NoError(t, err, "Fit()")
	assert.InDelta(t, 150.0, fitted.SecondsPerKM, 1e-6, "seconds per km")
	assert.InDelta(t, 4.0, fitted.SecondsPerAscentM, 1e-6, "seconds per ascent metre")
	assert.Equal(t, ridemodel.MinRides, fitted.EvaluatedRides,
		"an extended window reaches back for exactly the minimum")
	assert.Greater(t, fitted.TrainingWindowMonths, ridemodel.TrainingWindowMonths,
		"the stored window states the reach the fit really needed")
	require.NoError(t, fitted.Validate(), "Validate()")
}

// Extending the window is not the same as abandoning the ride floor.
func TestFitRefusesWhenEvenAllHistoryIsTooThin(t *testing.T) {
	t.Parallel()

	corpus := append(
		ridesEndingAt(4, 150, 4, fitNow().AddDate(-3, 0, 0)),
		ridesEndingAt(4, 150, 4, fitNow())...,
	)

	_, err := ridemodel.Fit(corpus, fitNow())
	require.ErrorIs(t, err, ridemodel.ErrTooFewRides)
}
