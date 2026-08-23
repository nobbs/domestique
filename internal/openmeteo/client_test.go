package openmeteo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientDecodesArrayResponseForSeveralCoordinates(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/v1/forecast", request.URL.Path)
		query := request.URL.Query()
		assert.Equal(t, "50.11,50.25", query.Get("latitude"))
		assert.Equal(t, "8.68,8.51", query.Get("longitude"))
		assert.Equal(t, "Europe/Berlin", query.Get("timezone"))
		assert.NotContains(t, query, "models")
		assert.NotContains(t, query, "temperature_unit")
		// The request carries Europe/Berlin local time: UTC 06:00 and 08:00 land
		// on 08:00 and 10:00 CEST.
		assert.Equal(t, "2026-08-24T08:00", query.Get("start_hour"))
		assert.Equal(t, "2026-08-24T10:00", query.Get("end_hour"))

		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `[
			{"hourly":{"time":["2026-08-24T06:00","2026-08-24T07:00"],
				"temperature_2m":[18.4,19.1],
				"apparent_temperature":[17.1,18.0],
				"precipitation":[0,0.2],
				"precipitation_probability":[10,20],
				"wind_speed_10m":[12.3,13.0],
				"wind_direction_10m":[240,245],
				"weather_code":[1,2]}},
			{"hourly":{"time":["2026-08-24T06:00","2026-08-24T07:00"],
				"temperature_2m":[17.0,17.5],
				"apparent_temperature":[16.0,16.5],
				"precipitation":[0,0],
				"precipitation_probability":[5,5],
				"wind_speed_10m":[10.0,10.5],
				"wind_direction_10m":[200,205],
				"weather_code":[0,0]}}
		]`)
	}))
	defer server.Close()

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	to := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	result, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{
		{Latitude: 50.11, Longitude: 8.68},
		{Latitude: 50.25, Longitude: 8.51},
	}, from, to)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, []float64{18.4, 19.1}, result[0].TemperatureCelsius)
	assert.Equal(t, []float64{17.0, 17.5}, result[1].TemperatureCelsius)
	require.Len(t, result[0].Time, 2)
	assert.True(t, result[0].Time[0].Equal(time.Date(2026, 8, 24, 6, 0, 0, 0, berlin)))
}

func TestClientDecodesBareObjectResponseForOneCoordinate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "50.11", request.URL.Query().Get("latitude"))
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"hourly":{"time":["2026-08-24T06:00"],
			"temperature_2m":[18.4],
			"apparent_temperature":[17.1],
			"precipitation":[0],
			"precipitation_probability":[10],
			"wind_speed_10m":[12.3],
			"wind_direction_10m":[240],
			"weather_code":[1]}}`)
	}))
	defer server.Close()

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	result, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, []float64{18.4}, result[0].TemperatureCelsius)
}

func TestClientRejectsResponseCoordinateCountMismatch(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `[{"hourly":{"time":[],"temperature_2m":[],"apparent_temperature":[],"precipitation":[],"precipitation_probability":[],"wind_speed_10m":[],"wind_direction_10m":[],"weather_code":[]}}]`)
	}))
	defer server.Close()

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	_, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{
		{Latitude: 50.11, Longitude: 8.68},
		{Latitude: 50.25, Longitude: 8.51},
	}, from, from)
	require.Error(t, err)
}

func TestClientHidesProviderFailureDetails(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeResponse(t, writer, http.StatusBadRequest, "private provider failure detail")
	}))
	defer server.Close()

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	_, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from)
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private provider failure detail")
}

func TestClientAbortsOnCancelledContext(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeResponse(t, writer, http.StatusOK, `{"hourly":{"time":[]}}`)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	_, err := newTestClient(t, server).Forecast(ctx, []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from)
	require.Error(t, err)
}

func TestClientAbortsOnExceededTimeout(t *testing.T) {
	blockUntilDone := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		select {
		case <-blockUntilDone:
		case <-request.Context().Done():
		}
	}))
	defer close(blockUntilDone)
	defer server.Close()

	client, err := New(&Options{
		BaseURL:   server.URL,
		Timeout:   10 * time.Millisecond,
		Transport: server.Client().Transport,
	})
	require.NoError(t, err)

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	_, err = client.Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from)
	require.Error(t, err)
}

func TestClientRejectsEmptyCoordinateList(t *testing.T) {
	client, err := New(&Options{BaseURL: "https://example.test"})
	require.NoError(t, err)

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	_, err = client.Forecast(t.Context(), nil, from, from)
	require.Error(t, err)
}

func TestClientRejectsToBeforeFrom(t *testing.T) {
	client, err := New(&Options{BaseURL: "https://example.test"})
	require.NoError(t, err)

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	to := from.Add(-time.Hour)
	_, err = client.Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, to)
	require.Error(t, err)
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		BaseURL:   server.URL,
		Timeout:   time.Second,
		Transport: server.Client().Transport,
	})
	require.NoError(t, err)

	return client
}

func writeResponse(t *testing.T, writer http.ResponseWriter, status int, body string) {
	t.Helper()
	writer.WriteHeader(status)
	_, err := writer.Write([]byte(body))
	assert.NoError(t, err, "writing the response")
}
