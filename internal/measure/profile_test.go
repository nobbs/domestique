package measure_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func distancesAndAltitudes(samples []measure.Sample) (distanceMetres, altitudeMetres []float64) {
	distanceMetres = make([]float64, len(samples))
	altitudeMetres = make([]float64, len(samples))
	for index, sample := range samples {
		distanceMetres[index] = sample.DistanceMetres
		altitudeMetres[index] = sample.AltitudeMetres
	}

	return distanceMetres, altitudeMetres
}

func TestProfileOfRefusesUnequalLengths(t *testing.T) {
	t.Parallel()
	_, ok := measure.ProfileOf([]float64{0, 1, 2}, []float64{0, 1})
	assert.False(t, ok)
}

func TestProfileOfRefusesNilInput(t *testing.T) {
	t.Parallel()
	_, ok := measure.ProfileOf(nil, nil)
	assert.False(t, ok)
}

func TestProfileOfRefusesFewerThanTwoPoints(t *testing.T) {
	t.Parallel()
	_, ok := measure.ProfileOf([]float64{0}, []float64{100})
	assert.False(t, ok)
}

func TestProfileOfAcceptsTwoPoints(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10}, []float64{100, 110})
	require.True(t, ok)
	assert.Equal(t, 2, profile.Len())
	assert.InDelta(t, 10, profile.LengthMetres(), 1e-9)
}

func TestProfileOfCopiesItsInputsRatherThanAliasingThem(t *testing.T) {
	t.Parallel()
	distanceMetres := []float64{0, 10, 20}
	altitudeMetres := []float64{100, 110, 90}
	profile, ok := measure.ProfileOf(distanceMetres, altitudeMetres)
	require.True(t, ok)

	distanceMetres[0] = 999
	altitudeMetres[0] = 999

	assert.InDelta(t, 0, profile.DistanceMetres()[0], 1e-9)
	assert.InDelta(t, 100, profile.AltitudeMetres()[0], 1e-9)
}

func TestProfileDistanceAndAltitudeMetresReturnCopies(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10, 20}, []float64{100, 110, 90})
	require.True(t, ok)

	distances := profile.DistanceMetres()
	distances[0] = 999
	altitudes := profile.AltitudeMetres()
	altitudes[0] = 999

	assert.InDelta(t, 0, profile.DistanceMetres()[0], 1e-9)
	assert.InDelta(t, 100, profile.AltitudeMetres()[0], 1e-9)
}

func TestProfileLengthMetresIsLastMinusFirstDistance(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{5, 15, 30}, []float64{0, 0, 0})
	require.True(t, ok)
	assert.InDelta(t, 25, profile.LengthMetres(), 1e-9)
}

func buildersForCoverage() map[string][]measure.Sample {
	return map[string][]measure.Sample{
		"flat":       flat(50, 5),
		"ramp":       ramp(50, 5, 0.05),
		"outAndBack": outAndBack(30, 5, 0.08),
		"noisyRamp":  noisy(ramp(50, 5, 0.05)),
		"stationary": stationary(20),
		"paused":     paused(30, 30, 30*time.Second),
		"random":     randomCorpus(),
	}
}

func TestProfileAscentAndDescentMetresMatchTheRouteReference(t *testing.T) {
	t.Parallel()
	for name, samples := range buildersForCoverage() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, altitudeMetres := distancesAndAltitudes(samples)
			distanceMetres, _ := distancesAndAltitudes(samples)
			profile, ok := measure.ProfileOf(distanceMetres, altitudeMetres)
			require.True(t, ok)

			assert.InDelta(t, referenceElevationGainMetres(altitudeMetres), profile.AscentMetres(), 1e-9)
			assert.InDelta(t, referenceElevationLossMetres(altitudeMetres), profile.DescentMetres(), 1e-9)
		})
	}
}

func TestProfileAscentMetresIsZeroForAFlatOrDescendingProfile(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10, 20}, []float64{100, 90, 80})
	require.True(t, ok)
	assert.InDelta(t, 0, profile.AscentMetres(), 1e-9)
	assert.InDelta(t, 20, profile.DescentMetres(), 1e-9)
}

// referenceCoordinateTrack turns a builder's distance series into a straight
// line of coordinates whose cumulative haversine distance matches it, so
// route.go's MaxGradientPercent reference (which measures over coordinates)
// can be compared against Profile's distance-based implementation.
func referenceCoordinateTrack(distanceMetres []float64) []referencePoint {
	const metresPerDegreeLatitude = 111_320.0
	points := make([]referencePoint, len(distanceMetres))
	for index, distance := range distanceMetres {
		points[index] = referencePoint{Latitude: distance / metresPerDegreeLatitude, Longitude: 0}
	}

	return points
}

func TestProfileMaxGradientPercentMatchesTheRouteReference(t *testing.T) {
	t.Parallel()
	const windowMetres = 100.0
	for name, samples := range buildersForCoverage() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			distanceMetres, altitudeMetres := distancesAndAltitudes(samples)
			profile, ok := measure.ProfileOf(distanceMetres, altitudeMetres)
			require.True(t, ok)

			points := referenceCoordinateTrack(distanceMetres)
			want := referenceMaxGradientPercent(points, altitudeMetres, windowMetres)
			got := profile.MaxGradientPercent(windowMetres)

			// referenceCoordinateTrack's cumulative haversine distance only
			// approximates distanceMetres (great-circle over a spherical
			// Earth vs. a flat metres-per-degree conversion); a wide
			// tolerance keeps the comparison about the algorithm, not the
			// projection.
			assert.InDelta(t, want, got, 0.05)
		})
	}
}

func TestProfileGradientsPercentIsZeroAtIndexZero(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10, 20}, []float64{100, 105, 110})
	require.True(t, ok)

	gradientPercent, spanMetres := profile.GradientsPercent(100)
	assert.InDelta(t, 0, gradientPercent[0], 1e-9)
	assert.InDelta(t, 0, spanMetres[0], 1e-9)
}

func TestProfileGradientsPercentWithZeroWindowUsesAdjacentSteps(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10, 30}, []float64{100, 105, 95})
	require.True(t, ok)

	gradientPercent, spanMetres := profile.GradientsPercent(0)
	require.Len(t, gradientPercent, 3)
	assert.InDelta(t, 10, spanMetres[1], 1e-9)
	assert.InDelta(t, 50, gradientPercent[1], 1e-9, "5m rise over 10m run")
	assert.InDelta(t, 20, spanMetres[2], 1e-9)
	assert.InDelta(t, -50, gradientPercent[2], 1e-9, "10m fall over 20m run")
}

func TestProfileMaxGradientPercentIsZeroWhenNoSpanReachesTheWindow(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10, 20}, []float64{100, 110, 120})
	require.True(t, ok)

	assert.InDelta(t, 0, profile.MaxGradientPercent(1000), 1e-9, "the profile never covers a kilometre")
}

func TestProfileMaxGradientPercentSeesADescentAsSteepAsAClimb(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 100, 200}, []float64{200, 100, 0})
	require.True(t, ok)

	assert.InDelta(t, 100, profile.MaxGradientPercent(100), 1e-9)
}

func TestProfileResampleMatchesTheElevationNormalizerReference(t *testing.T) {
	t.Parallel()
	const intervalMetres = 25.0
	for name, samples := range buildersForCoverage() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			distanceMetres, altitudeMetres := distancesAndAltitudes(samples)
			profile, ok := measure.ProfileOf(distanceMetres, altitudeMetres)
			require.True(t, ok)

			stepDistances := make([]float64, len(distanceMetres))
			for index := 1; index < len(distanceMetres); index++ {
				stepDistances[index] = distanceMetres[index] - distanceMetres[index-1]
			}
			want := referenceResampleElevations(stepDistances, altitudeMetres, intervalMetres)

			got := profile.Resample(intervalMetres)
			require.Equal(t, len(want), got.Len())
			gotDistances, gotAltitudes := got.DistanceMetres(), got.AltitudeMetres()
			for index, sample := range want {
				assert.InDelta(t, sample.distance, gotDistances[index], 1e-6)
				assert.InDelta(t, sample.elevation, gotAltitudes[index], 1e-6)
			}
		})
	}
}

func TestProfileResampleWithAnIntervalLargerThanTheLengthKeepsOnlyTheEndpoints(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 5, 10}, []float64{100, 105, 110})
	require.True(t, ok)

	resampled := profile.Resample(1000)

	assert.Equal(t, 2, resampled.Len())
	assert.InDelta(t, 0, resampled.DistanceMetres()[0], 1e-9)
	assert.InDelta(t, 10, resampled.DistanceMetres()[1], 1e-9)
	assert.InDelta(t, 110, resampled.AltitudeMetres()[1], 1e-9)
}

func TestProfileMedianFilteredMatchesTheElevationNormalizerReference(t *testing.T) {
	t.Parallel()
	const intervalMetres, windowMetres = 25.0, 100.0
	for name, samples := range buildersForCoverage() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			distanceMetres, altitudeMetres := distancesAndAltitudes(samples)
			profile, ok := measure.ProfileOf(distanceMetres, altitudeMetres)
			require.True(t, ok)

			stepDistances := make([]float64, len(distanceMetres))
			for index := 1; index < len(distanceMetres); index++ {
				stepDistances[index] = distanceMetres[index] - distanceMetres[index-1]
			}
			resampledReference := referenceResampleElevations(stepDistances, altitudeMetres, intervalMetres)
			referenceApplyMovingMedian(resampledReference, windowMetres, intervalMetres)

			resampled := profile.Resample(intervalMetres)
			filtered := resampled.MedianFiltered(intervalMetres, windowMetres)

			gotAltitudes := filtered.AltitudeMetres()
			require.Len(t, gotAltitudes, len(resampledReference))
			for index, sample := range resampledReference {
				assert.InDelta(t, sample.elevation, gotAltitudes[index], 1e-6)
			}
		})
	}
}

func TestProfileMedianFilteredWithRadiusZeroLeavesEachPointUnchanged(t *testing.T) {
	t.Parallel()
	// window/interval/2 truncates to zero, so each point is its own window.
	profile, ok := measure.ProfileOf([]float64{0, 25, 50, 75}, []float64{100, 999, 105, 110})
	require.True(t, ok)

	filtered := profile.MedianFiltered(25, 10)

	assert.Equal(t, []float64{100, 999, 105, 110}, filtered.AltitudeMetres())
}

func TestProfileAltitudeAtBeforeTheFirstSampleTakesTheFirstAltitude(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{10, 20, 30}, []float64{100, 110, 120})
	require.True(t, ok)

	assert.InDelta(t, 100, profile.AltitudeAt(0), 1e-9)
	assert.InDelta(t, 100, profile.AltitudeAt(-5), 1e-9)
	assert.InDelta(t, 100, profile.AltitudeAt(10), 1e-9, "exactly on the first sample")
}

func TestProfileAltitudeAtOnASampleReturnsItExactly(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10, 20}, []float64{100, 110, 90})
	require.True(t, ok)

	assert.InDelta(t, 110, profile.AltitudeAt(10), 1e-9)
}

func TestProfileAltitudeAtBetweenSamplesInterpolates(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10}, []float64{100, 110})
	require.True(t, ok)

	assert.InDelta(t, 105, profile.AltitudeAt(5), 1e-9)
}

func TestProfileAltitudeAtPastTheEndTakesTheLastAltitude(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10, 20}, []float64{100, 110, 90})
	require.True(t, ok)

	assert.InDelta(t, 90, profile.AltitudeAt(20), 1e-9)
	assert.InDelta(t, 90, profile.AltitudeAt(1000), 1e-9)
}

func TestProfileAltitudeAtMatchesTheFullElevationNormalizerPipeline(t *testing.T) {
	t.Parallel()
	const intervalMetres, windowMetres = 25.0, 100.0
	for name, samples := range buildersForCoverage() {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			distanceMetres, altitudeMetres := distancesAndAltitudes(samples)
			profile, ok := measure.ProfileOf(distanceMetres, altitudeMetres)
			require.True(t, ok)

			stepDistances := make([]float64, len(distanceMetres))
			for index := 1; index < len(distanceMetres); index++ {
				stepDistances[index] = distanceMetres[index] - distanceMetres[index-1]
			}
			resampledReference := referenceResampleElevations(stepDistances, altitudeMetres, intervalMetres)
			referenceApplyMovingMedian(resampledReference, windowMetres, intervalMetres)
			want := referenceApplyElevations(stepDistances, resampledReference)

			filtered := profile.Resample(intervalMetres).MedianFiltered(intervalMetres, windowMetres)
			for index, distance := range distanceMetres {
				assert.InDelta(t, want[index], filtered.AltitudeAt(distance), 1e-6)
			}
		})
	}
}

func TestProfileResampleKeepsTheProfilesOwnDistanceOrigin(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{1000, 1010, 1030}, []float64{5, 6, 8})
	require.True(t, ok)

	resampled := profile.Resample(10)

	assert.Equal(t, []float64{1000, 1010, 1020, 1030}, resampled.DistanceMetres())
	assert.InDeltaSlice(t, []float64{5, 6, 7, 8}, resampled.AltitudeMetres(), 1e-9)
	assert.InDelta(t, 7, resampled.AltitudeAt(1020), 1e-9)
}

func TestProfileOfRefusesADistanceThatGoesBackwards(t *testing.T) {
	t.Parallel()
	_, ok := measure.ProfileOf([]float64{0, 20, 10}, []float64{1, 2, 3})
	assert.False(t, ok)
}

func TestProfileZeroValueIsSafeToMeasure(t *testing.T) {
	t.Parallel()
	var profile measure.Profile

	assert.Zero(t, profile.Len())
	assert.Zero(t, profile.LengthMetres())
	assert.Zero(t, profile.AscentMetres())
	assert.Zero(t, profile.DescentMetres())
	assert.Zero(t, profile.MaxGradientPercent(100))
	assert.Zero(t, profile.AltitudeAt(10))
	assert.Zero(t, profile.Resample(25).Len())
	assert.Zero(t, profile.MedianFiltered(25, 100).Len())
}

func TestProfileResampleAndMedianRefuseANonPositiveInterval(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{0, 10, 30}, []float64{5, 6, 8})
	require.True(t, ok)

	for _, interval := range []float64{0, -25} {
		assert.Equal(t, profile.AltitudeMetres(), profile.Resample(interval).AltitudeMetres())
		assert.Equal(t, profile.DistanceMetres(), profile.Resample(interval).DistanceMetres())
		assert.Equal(t, profile.AltitudeMetres(), profile.MedianFiltered(interval, 100).AltitudeMetres())
	}
}
