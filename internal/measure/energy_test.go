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

func TestEstimatedCaloriesScalesLinearlyWithEnergy(t *testing.T) {
	t.Parallel()
	for name, watts := range map[string]float64{
		"no power":  0,
		"endurance": 150,
		"threshold": 280,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			const seconds = 3600.0

			doubled := measure.EstimatedCalories(2*watts, seconds)
			single := measure.EstimatedCalories(watts, seconds)

			assert.InDelta(t, doubled, 2*single, 1e-9)
		})
	}
}
