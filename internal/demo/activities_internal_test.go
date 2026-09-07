package demo

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func fixtureTime() time.Time {
	return time.Date(2026, time.April, 12, 9, 30, 0, 0, time.UTC)
}

// A ride names its stage by identity, and the library is edited far more often
// than these fixtures are. Both ways of naming one that cannot be ridden have
// to fail loudly: a silent fallback would leave a demo ride with no track and
// nothing to say why.
func TestARideRefusesAStageItCannotBeRecordedOver(t *testing.T) {
	t.Parallel()

	stages, err := Routes()
	require.NoError(t, err)

	for name, test := range map[string]struct {
		wants   string
		because string
		spec    rideSpec
	}{
		"a stage the library has dropped": {
			spec:    rideSpec{workoutID: 90_900, routeID: 4_999, stageOrder: 1},
			wants:   "no longer holds",
			because: "a fixture pointing at nothing must name what it was pointing at",
		},
		"a stage stored with no profile": {
			spec:    rideSpec{workoutID: 90_901, routeID: 4103, stageOrder: 1},
			wants:   "no complete profile",
			because: "timing comes from the profile, so a stage without one cannot be ridden",
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := test.spec.records(stages, fixtureTime())

			require.Error(t, err, test.because)
			assert.ErrorContains(t, err, test.wants)
		})
	}
}

// The same refusal has to survive being reached through the exported entry
// point, which is the only way anything outside this package builds a ride.
func TestRideCarriesTheRefusalOut(t *testing.T) {
	t.Parallel()

	stages, err := Routes()
	require.NoError(t, err)
	spec := rideSpec{workoutID: 90_902, routeID: 4_999, stageOrder: 1}

	_, err = spec.ride(stages, fixtureTime())

	require.Error(t, err)
	assert.ErrorContains(t, err, "90902", "the ride that failed is named")
}

func TestStageForReportsAKeyTheLibraryDoesNotHold(t *testing.T) {
	t.Parallel()

	stages, err := Routes()
	require.NoError(t, err)

	_, found := stageFor(stages, 4_999, 1)
	assert.False(t, found, "an unknown route id matches nothing")

	_, found = stageFor(stages, 4101, 99)
	assert.False(t, found, "and neither does a stage order the route does not have")
}

// A height beside a hole is not a slope, and two samples in the same place are
// not a climb. Either would otherwise drive the effort — and so the recorded
// heart rate and power — off a difference that means nothing.
func TestGradientIsNothingWithoutTwoHeightsAndGroundBetweenThem(t *testing.T) {
	t.Parallel()

	height := 210.0
	somewhere := route.Point{Latitude: 48.40, Longitude: 8.10, Elevation: &height}
	elsewhere := route.Point{Latitude: 48.41, Longitude: 8.10, Elevation: &height}
	unprofiled := route.Point{Latitude: 48.41, Longitude: 8.10}

	assert.InDelta(t, 0.0, gradientAt([]route.Point{somewhere}, 0), 1e-12,
		"the first sample has nothing to be a rise from")
	assert.InDelta(t, 0.0, gradientAt([]route.Point{somewhere, unprofiled}, 1), 1e-12,
		"a sample with no height yields no gradient")
	assert.InDelta(t, 0.0, gradientAt([]route.Point{unprofiled, somewhere}, 1), 1e-12,
		"and neither does one measured from a sample that had none")
	assert.InDelta(t, 0.0, gradientAt([]route.Point{somewhere, somewhere}, 1), 1e-12,
		"a repeated point covers no ground to rise over")
	assert.InDelta(t, 0.0, gradientAt([]route.Point{somewhere, elsewhere}, 1), 1e-12,
		"two equal heights a kilometre apart are level")
}
