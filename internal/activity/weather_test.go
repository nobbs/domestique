package activity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// WeatherAdapter is what lets the composition root hand over a provider's own
// methods without this package importing the adapter.
func TestWeatherAdapterAdaptsAPairOfFunctions(t *testing.T) {
	t.Parallel()
	var asked []float64
	source := activity.WeatherAdapter{
		Read: func(
			_ context.Context, latitudes, _ []float64, _, _ time.Time,
		) ([]activity.WeatherSeries, error) {
			asked = latitudes

			return []activity.WeatherSeries{{TemperatureCelsius: []float64{18}}}, nil
		},
		Step: func(time.Time) time.Duration { return 15 * time.Minute },
	}

	series, err := source.History(t.Context(), []float64{49}, []float64{8}, time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, []float64{49}, asked)
	require.Len(t, series, 1)
	assert.Equal(t, []float64{18}, series[0].TemperatureCelsius)
	assert.Equal(t, 15*time.Minute, source.StepFor(time.Time{}))

	failing := activity.WeatherAdapter{
		Read: func(
			context.Context, []float64, []float64, time.Time, time.Time,
		) ([]activity.WeatherSeries, error) {
			return nil, errors.New("upstream")
		},
	}
	_, err = failing.History(t.Context(), nil, nil, time.Time{}, time.Time{})
	require.ErrorContains(t, err, "upstream")

	// A half-built adapter is a wiring fault. It must reach the caller as one
	// rather than as a panic inside the run that asked for the weather.
	_, err = activity.WeatherAdapter{}.History(t.Context(), nil, nil, time.Time{}, time.Time{})
	require.ErrorContains(t, err, "no read function")
	assert.Zero(t, activity.WeatherAdapter{}.StepFor(time.Time{}), "and names no step")
}
