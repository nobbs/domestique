package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/httpapi"
)

// A Playwright assertion on the weather strip has to see the same reading
// every time it runs, so the whole point of standing this in for the real
// Open-Meteo client is that it is a pure function of its inputs.
func TestSyntheticWeatherIsDeterministic(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, time.August, 24, 6, 0, 0, 0, time.UTC)
	to := from.Add(5 * time.Hour)
	latitudes := []float64{48.40, 48.55, 48.90}
	longitudes := []float64{8.10, 8.35, 8.90}

	first, err := syntheticWeather(t.Context(), latitudes, longitudes, from, to)
	require.NoError(t, err)
	second, err := syntheticWeather(t.Context(), latitudes, longitudes, from, to)
	require.NoError(t, err)

	assert.Equal(t, first, second, "the same coordinates and window must always resolve to the same reading")
}

// The handler indexes the result by coordinate position, and the routes_weather
// consistency check rejects a series whose columns disagree in length, so both
// have to hold for every request this ever answers.
func TestSyntheticWeatherShapeMatchesTheRequest(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, time.August, 24, 13, 20, 0, 0, time.UTC)
	to := from.Add(7*time.Hour + 40*time.Minute)
	latitudes := []float64{48.40, 48.55, 48.62, 48.90}
	longitudes := []float64{8.10, 8.35, 8.50, 8.90}

	series, err := syntheticWeather(t.Context(), latitudes, longitudes, from, to)
	require.NoError(t, err)
	require.Len(t, series, len(latitudes), "one series per requested coordinate")

	for i := range series {
		count := len(series[i].Time)
		assert.Positive(t, count, "series %d has no hours", i)
		assert.Len(t, series[i].TemperatureCelsius, count, "series %d column length mismatch", i)
		assert.Len(t, series[i].ApparentTemperatureCelsius, count, "series %d column length mismatch", i)
		assert.Len(t, series[i].PrecipitationMillimetres, count, "series %d column length mismatch", i)
		assert.Len(t, series[i].PrecipitationProbabilityPercent, count, "series %d column length mismatch", i)
		assert.Len(t, series[i].WindSpeedKMH, count, "series %d column length mismatch", i)
		assert.Len(t, series[i].WindDirectionDegrees, count, "series %d column length mismatch", i)
		assert.Len(t, series[i].WeatherCode, count, "series %d column length mismatch", i)

		require.NotEmpty(t, series[i].Time)
		assert.False(t, series[i].Time[0].After(from), "series %d does not cover the start of the window", i)
		assert.False(t, series[i].Time[count-1].Before(to), "series %d does not cover the end of the window", i)
		for h := 1; h < count; h++ {
			assert.Equal(t, time.Hour, series[i].Time[h].Sub(series[i].Time[h-1]),
				"series %d hour %d is not one hour after the last", i, h)
		}
	}
}

func TestSyntheticWeatherValuesArePlausible(t *testing.T) {
	t.Parallel()

	// The codes this demo is allowed to emit — Open-Meteo's own WMO
	// vocabulary for the sky conditions this function models.
	validCodes := []int{0, 1, 2, 3, 61, 63}

	from := time.Date(2026, time.August, 24, 0, 0, 0, 0, time.UTC)
	to := from.Add(48 * time.Hour) // two full days, so every phase this function can reach shows up somewhere.
	latitudes := []float64{48.40, 48.62, 48.90, 48.30}
	longitudes := []float64{8.10, 8.50, 8.90, 7.70}

	series, err := syntheticWeather(t.Context(), latitudes, longitudes, from, to)
	require.NoError(t, err)

	rainSeen := false
	for i := range series {
		for h := range series[i].Time {
			temperature := series[i].TemperatureCelsius[h]
			assert.Greater(t, temperature, -20.0, "implausible temperature")
			assert.Less(t, temperature, 40.0, "implausible temperature")
			assert.InDelta(t, temperature, series[i].ApparentTemperatureCelsius[h], 6,
				"apparent temperature strayed far from the actual reading")

			precipitation := series[i].PrecipitationMillimetres[h]
			assert.GreaterOrEqual(t, precipitation, 0.0, "precipitation must not be negative")
			assert.LessOrEqual(t, precipitation, 10.0, "implausible precipitation")

			probability := series[i].PrecipitationProbabilityPercent[h]
			assert.GreaterOrEqual(t, probability, 0.0, "probability must not be negative")
			assert.LessOrEqual(t, probability, 100.0, "probability must not exceed 100")

			wind := series[i].WindSpeedKMH[h]
			assert.GreaterOrEqual(t, wind, 0.0, "wind speed must not be negative")
			assert.Less(t, wind, 100.0, "implausible wind speed")

			direction := series[i].WindDirectionDegrees[h]
			assert.GreaterOrEqual(t, direction, 0.0, "wind direction must be within 0..360")
			assert.Less(t, direction, 360.0, "wind direction must be within 0..360")

			code := series[i].WeatherCode[h]
			assert.Contains(t, validCodes, code, "weather code %d is not one of the WMO codes this demo emits", code)
			if precipitation > 0 {
				rainSeen = true
				assert.Contains(t, []int{61, 63}, code, "a positive precipitation reading needs a rain code")
			}
		}
	}
	assert.True(t, rainSeen,
		"the weather strip is meant to show rain somewhere along a route; this window and coordinate spread produced none")
}

// Two coordinates on the same stage, at the same hour, must not read
// identically at every hour of a whole day — a flat strip would look broken,
// the same way an all-zeros one would.
func TestSyntheticWeatherVariesAlongARoute(t *testing.T) {
	t.Parallel()

	from := time.Date(2026, time.August, 24, 6, 0, 0, 0, time.UTC)
	to := from.Add(6 * time.Hour)
	// Two points spread across a long stage, the way the demo's own longest
	// route (58 km) would present them.
	latitudes := []float64{48.40, 48.90}
	longitudes := []float64{8.10, 8.90}

	series, err := syntheticWeather(t.Context(), latitudes, longitudes, from, to)
	require.NoError(t, err)
	require.Len(t, series, 2)

	differed := false
	for h := range series[0].Time {
		if series[0].TemperatureCelsius[h] != series[1].TemperatureCelsius[h] ||
			series[0].WeatherCode[h] != series[1].WeatherCode[h] {
			differed = true

			break
		}
	}
	assert.True(t, differed, "two coordinates apart along a route must not read identically at every hour")
}

// httpapi.WeatherFunc is the seam syntheticWeather is wired in through; this
// just confirms the function still satisfies it after any signature drift.
var _ httpapi.Weather = httpapi.WeatherFunc(syntheticWeather)
