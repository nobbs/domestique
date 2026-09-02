package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWeatherReturnsOneSampleFromEachPointsSeries(t *testing.T) {
	var gotLatitudes, gotLongitudes []float64
	var gotFrom, gotTo time.Time
	weather := &fakeWeather{
		ForecastFunc: func(_ context.Context, latitudes, longitudes []float64, from, to time.Time) ([]WeatherSeries, error) {
			gotLatitudes, gotLongitudes, gotFrom, gotTo = latitudes, longitudes, from, to

			return []WeatherSeries{
				{
					Time:                            []time.Time{from, from.Add(time.Hour)},
					TemperatureCelsius:              []float64{18.4, 19.1},
					ApparentTemperatureCelsius:      []float64{17.1, 18.0},
					PrecipitationMillimetres:        []float64{0, 0.2},
					PrecipitationProbabilityPercent: []float64{10, 20},
					WindSpeedKMH:                    []float64{12.3, 13.0},
					WindDirectionDegrees:            []float64{240, 245},
					WeatherCode:                     []int{1, 2},
				},
				{
					Time:                            []time.Time{from, from.Add(time.Hour)},
					TemperatureCelsius:              []float64{17.0, 17.5},
					ApparentTemperatureCelsius:      []float64{16.0, 16.5},
					PrecipitationMillimetres:        []float64{0, 0},
					PrecipitationProbabilityPercent: []float64{5, 5},
					WindSpeedKMH:                    []float64{10.0, 10.5},
					WindDirectionDegrees:            []float64{200, 205},
					WeatherCode:                     []int{0, 0},
				},
			}, nil
		},
	}
	handler := newHandlerWithWeather(t, weather)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet,
		"/v1/weather?point=50.11,8.68,2026-08-24T06:00:00Z&point=50.25,8.51,2026-08-24T07:05:00Z"))

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.Equal(t, []float64{50.11, 50.25}, gotLatitudes)
	assert.Equal(t, []float64{8.68, 8.51}, gotLongitudes)
	assert.True(t, gotFrom.Equal(time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)), "from")
	assert.True(t, gotTo.Equal(time.Date(2026, 8, 24, 7, 5, 0, 0, time.UTC)), "to")

	var body openapi.WeatherForecast
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Points, 2)
	// The first point asks for exactly the series' first hour; the second asks
	// for 07:05, five minutes past the series' second hour and fifty-five past
	// its first, so it resolves to the second sample.
	assert.InDelta(t, 18.4, body.Points[0].TemperatureCelsius, 0)
	assert.InDelta(t, 17.5, body.Points[1].TemperatureCelsius, 0)
	assert.Equal(t, 1, body.Points[0].WeatherCode)
	assert.Equal(t, 0, body.Points[1].WeatherCode)
}

func TestWeatherDecodesBareObjectSeriesForOnePoint(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet,
		"/v1/weather?point=50.11,8.68,2026-08-24T06:00:00Z"))

	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var body openapi.WeatherForecast
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
	require.Len(t, body.Points, 1)
	assert.InDelta(t, 18.4, body.Points[0].TemperatureCelsius, 0)
}

func TestWeatherRejectsNoPoints(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather"))

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestWeatherRejectsMoreThanFortyEightPoints(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{})

	points := make([]string, maximumWeatherPoints+1)
	for i := range points {
		points[i] = "point=50.11,8.68,2026-08-24T06:00:00Z"
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather?"+strings.Join(points, "&")))

	assert.Equal(t, http.StatusBadRequest, response.Code)
}

func TestWeatherRejectsMalformedPoints(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{})

	for _, query := range []string{
		"point=not-a-coordinate",
		"point=91,8.68,2026-08-24T06:00:00Z",
		"point=50.11,181,2026-08-24T06:00:00Z",
		"point=50.11,8.68,not-a-time",
		"point=50.11,8.68,2026-08-24T06:00",
	} {
		t.Run(query, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather?"+query))
			assert.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}

func TestWeatherRejectsAWindowOutsideTheProviderRange(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{})
	handler.now = func() time.Time { return time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC) }

	for _, at := range []string{
		"2026-08-01T06:00:00Z", // more than a day in the past
		"2026-09-30T06:00:00Z", // beyond the forecast horizon
	} {
		t.Run(at, func(t *testing.T) {
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather?point=50.11,8.68,"+at))
			assert.Equal(t, http.StatusBadRequest, response.Code)
		})
	}
}

func TestWeatherReturnsBadGatewayOnProviderFailure(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{
		ForecastFunc: func(context.Context, []float64, []float64, time.Time, time.Time) ([]WeatherSeries, error) {
			return nil, errors.New("provider says private things")
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather?point=50.11,8.68,2026-08-24T06:00:00Z"))

	require.Equal(t, http.StatusBadGateway, response.Code)
	assert.NotContains(t, response.Body.String(), "private things")
}

func TestWeatherReturnsBadGatewayOnCoordinateCountMismatch(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{
		ForecastFunc: func(context.Context, []float64, []float64, time.Time, time.Time) ([]WeatherSeries, error) {
			return []WeatherSeries{}, nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather?point=50.11,8.68,2026-08-24T06:00:00Z"))

	assert.Equal(t, http.StatusBadGateway, response.Code)
}

func TestWeatherReturnsBadGatewayWhenAPointsSeriesHasNoHours(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{
		ForecastFunc: func(_ context.Context, latitudes, _ []float64, _, _ time.Time) ([]WeatherSeries, error) {
			return make([]WeatherSeries, len(latitudes)), nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather?point=50.11,8.68,2026-08-24T06:00:00Z"))

	assert.Equal(t, http.StatusBadGateway, response.Code)
}

// A Weather implementation could return a series whose fields disagree in
// length — this must become a 502, not an index panic.
func TestWeatherReturnsBadGatewayOnAnInconsistentSeries(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{
		ForecastFunc: func(_ context.Context, latitudes, _ []float64, from, _ time.Time) ([]WeatherSeries, error) {
			series := make([]WeatherSeries, len(latitudes))
			for i := range series {
				series[i] = WeatherSeries{
					Time:               []time.Time{from},
					TemperatureCelsius: []float64{},
				}
			}

			return series, nil
		},
	})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/weather?point=50.11,8.68,2026-08-24T06:00:00Z"))

	assert.Equal(t, http.StatusBadGateway, response.Code)
}

// A later point naming an earlier time than an already-seen one must still
// widen the window backwards, not just forwards.
func TestWeatherDerivesFromAsTheEarliestPointEvenOutOfOrder(t *testing.T) {
	var gotFrom time.Time
	weather := &fakeWeather{
		ForecastFunc: func(_ context.Context, latitudes, _ []float64, from, _ time.Time) ([]WeatherSeries, error) {
			gotFrom = from

			return make([]WeatherSeries, len(latitudes)), nil
		},
	}
	handler := newHandlerWithWeather(t, weather)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet,
		"/v1/weather?point=50.11,8.68,2026-08-24T07:00:00Z&point=50.25,8.51,2026-08-24T06:00:00Z"))

	// The response is a 502 because the fake returns no hours; what this test
	// checks is the window the fake was called with.
	require.Equal(t, http.StatusBadGateway, response.Code)
	assert.True(t, gotFrom.Equal(time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)), "from")
}

func TestWeatherIsNotSameOriginWrapped(t *testing.T) {
	handler := newHandlerWithWeather(t, &fakeWeather{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/v1/weather?point=50.11,8.68,2026-08-24T06:00:00Z", http.NoBody)
	withSession(request)
	// Deliberately no Origin header: a GET never carries one from a browser,
	// and this route must not require it the way a state-changing one does.
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	assert.Equal(t, http.StatusOK, response.Code, response.Body.String())
}

func TestParseWeatherPointCapsCoordinateRange(t *testing.T) {
	_, ok := parseWeatherPoint("90.0000001,0,2026-08-24T06:00:00Z")
	assert.False(t, ok)
	point, ok := parseWeatherPoint("90,-180,2026-08-24T06:00:00Z")
	assert.True(t, ok)
	assert.InDelta(t, 90.0, point.Latitude, 0)
	assert.InDelta(t, -180.0, point.Longitude, 0)
}

func TestWeatherFuncAdaptsToWeather(t *testing.T) {
	var gotFrom time.Time
	fn := WeatherFunc(func(_ context.Context, _, _ []float64, from, _ time.Time) ([]WeatherSeries, error) {
		gotFrom = from

		return []WeatherSeries{{Time: []time.Time{from}}}, nil
	})

	now := time.Now()
	series, err := fn.Forecast(t.Context(), []float64{50.11}, []float64{8.68}, now, now)
	require.NoError(t, err)
	require.Len(t, series, 1)
	assert.True(t, gotFrom.Equal(now))
}

func TestNearestHourIndexPicksClosestSample(t *testing.T) {
	hours := []time.Time{
		time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 24, 7, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC),
	}
	index, found := nearestHourIndex(hours, time.Date(2026, 8, 24, 7, 40, 0, 0, time.UTC))
	require.True(t, found)
	assert.Equal(t, 2, index)

	_, found = nearestHourIndex(nil, time.Now())
	assert.False(t, found)
}
