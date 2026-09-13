package measure_test

import (
	"testing"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
)

// 200 W over two hours is 1440 kJ, which at the handover document's 22% gross
// efficiency cross-check comes to ~1564 kcal.
func TestEstimatedCaloriesAtTheHandoverEfficiency(t *testing.T) {
	t.Parallel()
	const meanWatts, seconds = 200.0, 2 * 3600.0

	assert.InDelta(t, 1564.4, measure.EstimatedCalories(meanWatts, seconds), 0.1)
}

// 280 W over ninety minutes is 1512 kJ, ~1642.6 kcal at the same efficiency —
// a second worked example at different inputs, so a wrong constant cannot
// pass both by cancelling itself out the way a pure scaling check would.
func TestEstimatedCaloriesAtASecondWorkedExample(t *testing.T) {
	t.Parallel()
	const meanWatts, seconds = 280.0, 90 * 60.0

	assert.InDelta(t, 1642.6, measure.EstimatedCalories(meanWatts, seconds), 0.1)
}

func TestEstimatedCaloriesIsZeroWithNoPower(t *testing.T) {
	t.Parallel()

	assert.InDelta(t, 0, measure.EstimatedCalories(0, 3600), 1e-9)
}
