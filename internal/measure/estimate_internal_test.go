package measure

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// The barometric-pressure and ideal-gas formula against hand-computed values:
// density falls with altitude and rises with cold.
func TestAirDensityFallsWithAltitudeAndRisesWithCold(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 1.225, airDensity(0, 15), 0.001, "sea level, fifteen degrees")
	// The handover's own "≈1.112" for this case assumes the standard
	// atmosphere's falling temperature at altitude; the formula it actually
	// specifies holds temperature at the given 15°C and comes to ~1.087.
	assert.InDelta(t, 1.087, airDensity(1000, 15), 0.001, "a thousand metres up, thinner air")
	assert.InDelta(t, 1.164, airDensity(0, 30), 0.001, "sea level, hot air is less dense")
}
