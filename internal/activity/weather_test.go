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

// WeatherFunc is what lets the composition root hand over a provider's own
// method without this package importing the adapter.
func TestWeatherFuncAdaptsAFunction(t *testing.T) {
	t.Parallel()
	var asked []float64
	source := activity.WeatherFunc(func(
		_ context.Context, latitudes, _ []float64, _, _ time.Time,
	) ([]activity.WeatherSeries, error) {
		asked = latitudes

		return []activity.WeatherSeries{{TemperatureCelsius: []float64{18}}}, nil
	})

	series, err := source.History(t.Context(), []float64{49}, []float64{8}, time.Time{}, time.Time{})
	require.NoError(t, err)
	assert.Equal(t, []float64{49}, asked)
	require.Len(t, series, 1)
	assert.Equal(t, []float64{18}, series[0].TemperatureCelsius)

	failing := activity.WeatherFunc(func(
		context.Context, []float64, []float64, time.Time, time.Time,
	) ([]activity.WeatherSeries, error) {
		return nil, errors.New("upstream")
	})
	_, err = failing.History(t.Context(), nil, nil, time.Time{}, time.Time{})
	require.ErrorContains(t, err, "upstream")
}
