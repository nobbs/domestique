package rider_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRampThresholdPowerAcceptsARideShapedLikeARampTest(t *testing.T) {
	t.Parallel()

	watts, ok := rider.RampThresholdPower(28*time.Minute, 330, 330/1.14)

	require.True(t, ok, "20-40 minutes moving with a ratio at 1.14 is a ramp test")
	assert.InDelta(t, 247.5, watts, 0.01, "75% of the best minute")
}

func TestRampThresholdPowerRejectsAnHourLongIntervalSession(t *testing.T) {
	t.Parallel()

	_, ok := rider.RampThresholdPower(60*time.Minute, 340, 180) // ratio 1.89, moving time also out of range.

	assert.False(t, ok, "an hour is well outside a ramp test's own length")
}

func TestRampThresholdPowerRejectsAShorterSessionsHardMinute(t *testing.T) {
	t.Parallel()

	_, ok := rider.RampThresholdPower(25*time.Minute, 300, 200) // ratio 1.5: the moving time is right, the shape is not.

	assert.False(t, ok, "an interval session's hard minute, not a ramp test's")
}
