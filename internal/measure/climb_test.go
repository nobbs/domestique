package measure_test

import (
	"testing"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These vectors are mirrored in internal/webui/app/src/lib/climbs.test.ts; a
// change here changes both.

// climbFineSpacingMetres is one ten-thousandth of a degree of latitude, in metres.
const climbFineSpacingMetres = 11.119

// climbRamp builds a profile whose segments run at the given gradients, in percent, on
// points spaced climbFineSpacingMetres apart — the same construction climbs.test.ts uses,
// so a run of ten segments covers about the hundred-metre window a climb is measured over.
func climbRamp(t *testing.T, percents []float64) measure.Profile {
	t.Helper()
	coordinates := []measure.Coordinate{{Latitude: 49, Longitude: 8}}
	altitudeMetres := []float64{100}
	for index, percent := range percents {
		coordinates = append(coordinates, measure.Coordinate{
			Latitude:  49 + float64(index+1)*0.0001,
			Longitude: 8,
		})
		altitudeMetres = append(altitudeMetres, altitudeMetres[index]+climbFineSpacingMetres*percent/100)
	}
	distanceMetres := measure.CumulativeMetres(coordinates)
	profile, ok := measure.ProfileOf(distanceMetres, altitudeMetres)
	require.True(t, ok)

	return profile
}

// climbSteady is count segments at a steady percent gradient.
func climbSteady(percent float64, count int) []float64 {
	percents := make([]float64, count)
	for index := range percents {
		percents[index] = percent
	}

	return percents
}

func climbConcat(runs ...[]float64) []float64 {
	var all []float64
	for _, run := range runs {
		all = append(all, run...)
	}

	return all
}

func TestClimbsFindsNothingOnFlatGround(t *testing.T) {
	t.Parallel()
	profile := climbRamp(t, climbSteady(0, 20))

	assert.Empty(t, measure.Climbs(profile, 100, 3))
}

func TestClimbsFindsNothingOnADescentHoweverSteep(t *testing.T) {
	t.Parallel()
	profile := climbRamp(t, climbSteady(-12, 20))

	assert.Empty(t, measure.Climbs(profile, 100, 3))
}

func TestClimbsReportsASteadyClimbHeldOverTheWindow(t *testing.T) {
	t.Parallel()
	profile := climbRamp(t, climbSteady(10, 20))

	climbs := measure.Climbs(profile, 100, 3)

	require.Len(t, climbs, 1)
	assert.InDelta(t, 0, climbs[0].StartMetres, 1)
	assert.InDelta(t, 10, climbs[0].AverageGradePercent, 1)
	assert.InDelta(t, 10, climbs[0].MaxGradePercent, 1)
	assert.Positive(t, climbs[0].AscentMetres)
}

func TestClimbsFindsNothingInABumpTooSlightToReachAClimbingGradient(t *testing.T) {
	t.Parallel()
	// Three segments at five percent lift the ground under a metre — never
	// enough, even smeared across the whole look-back window, to reach the
	// gradient a climb opens at.
	profile := climbRamp(t, climbConcat(climbSteady(0, 10), climbSteady(5, 3), climbSteady(0, 10)))

	assert.Empty(t, measure.Climbs(profile, 100, 3))
}

func TestClimbsHoldsAtAGradientJustOverTheOneThatOpensTheFirstNonFlatBand(t *testing.T) {
	t.Parallel()
	profile := climbRamp(t, climbSteady(3.1, 20))

	assert.Len(t, measure.Climbs(profile, 100, 3), 1)
}

func TestClimbsRefusesTheSameRunJustUnderThatGradient(t *testing.T) {
	t.Parallel()
	profile := climbRamp(t, climbSteady(2.9, 20))

	assert.Empty(t, measure.Climbs(profile, 100, 3))
}

func TestClimbsMergesTwoClimbsAShortFlatGapApartIntoOne(t *testing.T) {
	t.Parallel()
	// ~56 m of flat between two climbs, under the 100 m floor that separates them.
	profile := climbRamp(t, climbConcat(climbSteady(10, 15), climbSteady(0, 5), climbSteady(10, 15)))

	climbs := measure.Climbs(profile, 100, 3)

	require.Len(t, climbs, 1)
	assert.Greater(t, climbs[0].EndMetres, climbs[0].StartMetres)
}

func TestClimbsKeepsTwoClimbsApartAcrossAFlatLongEnoughToMatter(t *testing.T) {
	t.Parallel()
	// ~145 m of flat between two climbs, over the 100 m floor.
	profile := climbRamp(t, climbConcat(climbSteady(10, 15), climbSteady(0, 13), climbSteady(10, 15)))

	assert.Len(t, measure.Climbs(profile, 100, 3), 2)
}

func TestClimbsKeepsADipInsideAClimbAsOneClimb(t *testing.T) {
	t.Parallel()
	// ~44 m dip (four segments at -10%) inside an otherwise steady climb: too
	// short for the 100 m look-back window to pull the gradient below 3%.
	profile := climbRamp(t, climbConcat(climbSteady(10, 15), climbSteady(-10, 4), climbSteady(10, 15)))

	climbs := measure.Climbs(profile, 100, 3)

	require.Len(t, climbs, 1)
	assert.InDelta(t, profile.LengthMetres(), climbs[0].DistanceMetres, 1)
}

func TestClimbsReportsAClimbReachingTheLastPoint(t *testing.T) {
	t.Parallel()
	profile := climbRamp(t, climbSteady(10, 12))

	climbs := measure.Climbs(profile, 100, 3)

	require.Len(t, climbs, 1)
	assert.InDelta(t, profile.LengthMetres(), climbs[0].EndMetres, 1e-6)
}

func TestClimbsIsEmptyForTheZeroValueProfile(t *testing.T) {
	t.Parallel()
	var profile measure.Profile

	assert.Nil(t, measure.Climbs(profile, 100, 3))
}

func TestClimbsIsEmptyForAProfileOfZeroLength(t *testing.T) {
	t.Parallel()
	profile, ok := measure.ProfileOf([]float64{5, 5}, []float64{100, 140})
	require.True(t, ok)

	assert.Nil(t, measure.Climbs(profile, 100, 3))
}

func TestClimbsIsEmptyForANonPositiveWindow(t *testing.T) {
	t.Parallel()
	profile := climbRamp(t, climbSteady(10, 20))

	for _, window := range []float64{0, -100} {
		assert.Nil(t, measure.Climbs(profile, window, 3))
	}
}
