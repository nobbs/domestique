package openmeteo

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
		assert.Contains(t, query.Get("hourly"), "cloud_cover")
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
				"weather_code":[1,2],
				"cloud_cover":[80,90]}},
			{"hourly":{"time":["2026-08-24T06:00","2026-08-24T07:00"],
				"temperature_2m":[17.0,17.5],
				"apparent_temperature":[16.0,16.5],
				"precipitation":[0,0],
				"precipitation_probability":[5,5],
				"wind_speed_10m":[10.0,10.5],
				"wind_direction_10m":[200,205],
				"weather_code":[0,0],
				"cloud_cover":[20,25]}}
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
	assert.Equal(t, []float64{80, 90}, result[0].CloudCoverPercent)
	assert.Equal(t, []float64{20, 25}, result[1].CloudCoverPercent)
	require.Len(t, result[0].Time, 2)
	// "2026-08-24T06:00" is Berlin local time (CEST, UTC+2 in August), so the
	// parsed instant is 04:00 UTC.
	assert.True(t, result[0].Time[0].Equal(time.Date(2026, 8, 24, 4, 0, 0, 0, time.UTC)))
}

// A window that does not itself land on the hour must still ask for every
// hour a point inside it could resolve to: from rounds down, to rounds up.
func TestClientRoundsTheRequestWindowOutToWholeHours(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		assert.Equal(t, "2026-08-24T08:00", query.Get("start_hour"))
		assert.Equal(t, "2026-08-24T09:00", query.Get("end_hour"))

		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"hourly":{"time":["2026-08-24T08:00"],
			"temperature_2m":[18.4],
			"apparent_temperature":[17.1],
			"precipitation":[0],
			"precipitation_probability":[10],
			"wind_speed_10m":[12.3],
			"wind_direction_10m":[240],
			"weather_code":[1],
			"cloud_cover":[50]}}`)
	}))
	defer server.Close()

	// 06:05 and 06:40 UTC land on 08:05 and 08:40 CEST — neither on the hour.
	from := time.Date(2026, 8, 24, 6, 5, 0, 0, time.UTC)
	to := time.Date(2026, 8, 24, 6, 40, 0, 0, time.UTC)
	_, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, to)
	require.NoError(t, err)
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
			"weather_code":[1],
			"cloud_cover":[50]}}`)
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

func TestClientRejectsAnUndecodableResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `not json at all`)
	}))
	defer server.Close()

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	_, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from)
	require.Error(t, err)
}

func TestClientRejectsAnOversizedResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"hourly":{"time":`+strings.Repeat(" ", maximumBodyBytes)+`}}`)
	}))
	defer server.Close()

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	_, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from)
	require.Error(t, err)
}

func TestClientRejectsMismatchedHourlySeriesLengths(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"hourly":{"time":["2026-08-24T06:00","2026-08-24T07:00"],
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
	_, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from)
	require.Error(t, err)
}

func TestClientRejectsAMismatchedWeatherCodeLength(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"hourly":{"time":["2026-08-24T06:00"],
			"temperature_2m":[18.4],
			"apparent_temperature":[17.1],
			"precipitation":[0],
			"precipitation_probability":[10],
			"wind_speed_10m":[12.3],
			"wind_direction_10m":[240],
			"weather_code":[],
			"cloud_cover":[50]}}`)
	}))
	defer server.Close()

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	_, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from)
	require.Error(t, err)
}

func TestClientRejectsAMalformedHourlyTimestamp(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"hourly":{"time":["not-a-timestamp"],
			"temperature_2m":[18.4],
			"apparent_temperature":[17.1],
			"precipitation":[0],
			"precipitation_probability":[10],
			"wind_speed_10m":[12.3],
			"wind_direction_10m":[240],
			"weather_code":[1],
			"cloud_cover":[50]}}`)
	}))
	defer server.Close()

	from := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	_, err := newTestClient(t, server).Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from)
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

func TestNewRejectsNilOptions(t *testing.T) {
	_, err := New(nil)
	require.Error(t, err)
}

func TestNewUsesTheDefaultBaseURLWhenNoneIsGiven(t *testing.T) {
	client, err := New(&Options{})
	require.NoError(t, err)
	assert.Equal(t, defaultBaseURL, client.baseURL.String())
}

func TestNewRejectsANegativeTimeout(t *testing.T) {
	_, err := New(&Options{BaseURL: "https://example.test", Timeout: -time.Second})
	require.Error(t, err)
}

//nolint:gosec // A rejection fixture for URL userinfo, not a real credential.
func TestParseOriginRejectsEveryMalformedShape(t *testing.T) {
	for name, value := range map[string]string{
		"not a URL at all":  "://not-a-url",
		"non-https scheme":  "http://example.test",
		"no host":           "https://",
		"embedded userinfo": "https://user:pass@example.test",
		"query string":      "https://example.test?key=value",
		"fragment":          "https://example.test#section",
		"non-root path":     "https://example.test/v1",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := parseOrigin(value)
			assert.Error(t, err)
		})
	}
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

func TestClientAsksInTheConfiguredZone(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "Europe/Lisbon", request.URL.Query().Get("timezone"), "the zone asked for")
		// 06:05 and 06:40 UTC are 07:05 and 07:40 in Lisbon, an hour behind Berlin.
		assert.Equal(t, "2026-08-24T07:00", request.URL.Query().Get("start_hour"), "start hour")

		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"hourly":{"time":["2026-08-24T07:00"],
			"temperature_2m":[18.4],
			"apparent_temperature":[17.1],
			"precipitation":[0],
			"precipitation_probability":[10],
			"wind_speed_10m":[12.3],
			"wind_direction_10m":[240],
			"weather_code":[1],
			"cloud_cover":[50]}}`)
	}))
	defer server.Close()

	client, err := New(&Options{
		BaseURL:   server.URL,
		Timeout:   time.Second,
		Transport: server.Client().Transport,
		Timezone:  func() string { return "Europe/Lisbon" },
	})
	require.NoError(t, err, "New()")

	from := time.Date(2026, 8, 24, 6, 5, 0, 0, time.UTC)
	to := time.Date(2026, 8, 24, 6, 40, 0, 0, time.UTC)
	_, err = client.Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, to)
	require.NoError(t, err, "Forecast()")
}

func TestClientReadsAnEditedZoneOnTheNextForecastRatherThanTheNextRestart(t *testing.T) {
	seen := make(chan string, 2)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		seen <- request.URL.Query().Get("timezone")
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"hourly":{"time":["2026-08-24T07:00"],
			"temperature_2m":[18.4],"apparent_temperature":[17.1],"precipitation":[0],
			"precipitation_probability":[10],"wind_speed_10m":[12.3],
			"wind_direction_10m":[240],"weather_code":[1],"cloud_cover":[50]}}`)
	}))
	defer server.Close()

	zone := "Europe/Berlin"
	client, err := New(&Options{
		BaseURL:   server.URL,
		Timeout:   time.Second,
		Transport: server.Client().Transport,
		Timezone:  func() string { return zone },
	})
	require.NoError(t, err, "New()")

	from := time.Date(2026, 8, 24, 6, 5, 0, 0, time.UTC)
	to := time.Date(2026, 8, 24, 6, 40, 0, 0, time.UTC)
	_, err = client.Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, to)
	require.NoError(t, err, "first Forecast()")

	zone = "Europe/Lisbon"
	_, err = client.Forecast(t.Context(), []Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, to)
	require.NoError(t, err, "second Forecast()")

	assert.Equal(t, "Europe/Berlin", <-seen, "the first request")
	assert.Equal(t, "Europe/Lisbon", <-seen, "the edited zone never reached the second request")
}

func TestNewRefusesAZoneItCannotLoad(t *testing.T) {
	_, err := New(&Options{Timezone: func() string { return "Middle/Earth" }})

	require.Error(t, err, "New() accepted a zone it cannot load")
	assert.Contains(t, err.Error(), "Middle/Earth", "the message names the zone")
}

// historyNow is the clock every choice between the two endpoints below is made
// against.
func historyNow() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) }

// newHistoryClient points the forecast and archive hosts at the same test
// server, so one handler can assert which of the two paths was asked.
func newHistoryClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		BaseURL:        server.URL,
		ArchiveBaseURL: server.URL,
		Timeout:        time.Second,
		Transport:      server.Client().Transport,
		Now:            historyNow,
	})
	require.NoError(t, err)

	return client
}

const historyBody = `{"minutely_15":{"time":["2026-08-24T08:15"],
	"temperature_2m":[18.4],
	"apparent_temperature":[17.1],
	"precipitation":[0],
	"precipitation_probability":[10],
	"wind_speed_10m":[12.3],
	"wind_direction_10m":[240],
	"weather_code":[1],
	"cloud_cover":[50]}}`

// A quarter-hourly block with no probability of precipitation, which only the
// reanalysis may answer with: shaped like a forecast, so refusing it is about
// the missing series and not the missing block.
const quarterBodyWithoutProbability = `{"minutely_15":{"time":["2026-08-24T08:15"],
	"temperature_2m":[18.4],
	"apparent_temperature":[17.1],
	"precipitation":[0],
	"wind_speed_10m":[12.3],
	"wind_direction_10m":[240],
	"weather_code":[1],
	"cloud_cover":[50]}}`

// The archive carries no probability of precipitation: it records what fell,
// not what might have.
const archiveBody = `{"hourly":{"time":["2025-08-24T08:00"],
	"temperature_2m":[18.4],
	"apparent_temperature":[17.1],
	"precipitation":[0],
	"wind_speed_10m":[12.3],
	"wind_direction_10m":[240],
	"weather_code":[1],
	"cloud_cover":[50]}}`

// A ride of the last day or two is asked of the forecast endpoint, by hour
// bounds alone. past_days must not be among them: the provider treats an hour
// range and a count of past days as two ways of saying the same thing and
// refuses a request carrying both, which is a 400 for every ride.
func TestHistoryAsksTheForecastEndpointForARecentRide(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/v1/forecast", request.URL.Path)
		query := request.URL.Query()
		assert.Empty(t, query.Get("past_days"), "refused beside a range")
		assert.Empty(t, query.Get("forecast_days"), "and so is its companion")
		assert.Equal(t, "2026-08-23T10:00", query.Get("start_minutely_15"))
		assert.Equal(t, "2026-08-23T12:00", query.Get("end_minutely_15"))
		assert.Contains(t, query.Get("minutely_15"), "precipitation_probability")
		assert.Empty(t, query.Get("hourly"), "the hourly block is not asked for")
		assert.Empty(t, query.Get("start_hour"), "and its bounds address only that one")
		assert.Empty(t, query.Get("start_date"), "the archive's parameters are not this endpoint's")

		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, historyBody)
	}))
	defer server.Close()

	from := historyNow().AddDate(0, 0, -1).Add(-4 * time.Hour)
	result, err := newHistoryClient(t, server).History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(2*time.Hour))
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, []float64{10}, result[0].PrecipitationProbabilityPercent)
	assert.Equal(t, quarterHourStep, result[0].Step, "answered by the quarter hour")
}

// A ride older than the forecast endpoint's reach is asked of the reanalysis
// archive instead, by the same hour bounds and without the probability series.
func TestHistoryAsksTheArchiveForAnOlderRide(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/v1/archive", request.URL.Path)
		query := request.URL.Query()
		assert.Equal(t, "2025-08-24T10:00", query.Get("start_hour"))
		assert.Equal(t, "2025-08-24T12:00", query.Get("end_hour"))
		assert.Empty(t, query.Get("start_date"), "asked by hour, as the forecast endpoint is")
		assert.NotContains(t, query.Get("hourly"), "precipitation_probability",
			"the reanalysis refuses a series it does not carry")
		assert.Empty(t, query.Get("past_days"), "the forecast's parameters are not this endpoint's")
		assert.Empty(t, query.Get("minutely_15"), "there is no sub-hourly reanalysis to ask for")

		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, archiveBody)
	}))
	defer server.Close()

	from := time.Date(2025, 8, 24, 8, 0, 0, 0, time.UTC)
	result, err := newHistoryClient(t, server).History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(2*time.Hour))
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, []float64{18.4}, result[0].TemperatureCelsius)
	assert.Empty(t, result[0].PrecipitationProbabilityPercent,
		"absent, rather than a column of invented numbers")
	assert.Equal(t, time.Hour, result[0].Step, "the reanalysis is hourly and only hourly")
}

// The boundary itself: the last day the forecast endpoint reaches, and the
// first the archive has to answer for.
func TestHistoryChoosesTheEndpointAtTheBoundary(t *testing.T) {
	for name, test := range map[string]struct {
		path    string
		body    string
		daysAgo int
	}{
		"today":                     {path: "/v1/forecast", body: historyBody, daysAgo: 0},
		"yesterday":                 {path: "/v1/forecast", body: historyBody, daysAgo: 1},
		"the day the archive takes": {path: "/v1/archive", body: archiveBody, daysAgo: 2},
		"and everything older":      {path: "/v1/archive", body: archiveBody, daysAgo: 400},
	} {
		t.Run(name, func(t *testing.T) {
			var asked string
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				asked = request.URL.Path
				writer.Header().Set("Content-Type", "application/json")
				writeResponse(t, writer, http.StatusOK, test.body)
			}))
			defer server.Close()

			from := historyNow().AddDate(0, 0, -test.daysAgo)
			_, err := newHistoryClient(t, server).History(t.Context(),
				[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(time.Hour))
			require.NoError(t, err)
			assert.Equal(t, test.path, asked)
		})
	}
}

// A device with a wrong clock can date a ride in the future. It is asked about
// as if it were today's rather than with a negative reach.
func TestHistoryAsksAboutAFutureRideAsIfItWereTodays(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/v1/forecast", request.URL.Path)
		assert.Empty(t, request.URL.Query().Get("past_days"))

		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, historyBody)
	}))
	defer server.Close()

	from := historyNow().AddDate(0, 0, 3)
	_, err := newHistoryClient(t, server).History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(time.Hour))
	require.NoError(t, err)
}

func TestNewRefusesAnArchiveHostThatIsNotAnOrigin(t *testing.T) {
	t.Parallel()
	_, err := New(&Options{ArchiveBaseURL: "http://archive.example.test/with/a/path"})
	require.ErrorContains(t, err, "archive base url")
}

// A series the response is short of is refused rather than read past the end of.
func TestHistoryRefusesAResponseMissingAColumn(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"minutely_15":{"time":["2026-08-24T08:00","2026-08-24T09:00"],
			"temperature_2m":[18.4],
			"apparent_temperature":[17.1],
			"precipitation":[0],
			"wind_speed_10m":[12.3],
			"wind_direction_10m":[240],
			"weather_code":[1],
			"cloud_cover":[50]}}`)
	}))
	defer server.Close()

	from := historyNow().AddDate(0, 0, -1)
	_, err := newHistoryClient(t, server).History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(time.Hour))
	require.ErrorContains(t, err, "series lengths did not match")
}

// The forecast endpoint is asked for a probability of precipitation, so one
// that comes back without it is a response to refuse rather than to read as
// "none was recorded". Only the reanalysis, which is never asked, may omit it.
func TestHistoryRefusesAForecastWithNoProbabilityAtAll(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, quarterBodyWithoutProbability)
	}))
	defer server.Close()

	from := historyNow().AddDate(0, 0, -1)
	_, err := newHistoryClient(t, server).History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(time.Hour))
	require.ErrorContains(t, err, "series lengths did not match")
}

// The probability of precipitation is the one series that may be absent; a
// short one is still a mismatch.
func TestHistoryRefusesAShortProbabilitySeries(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"minutely_15":{"time":["2026-08-24T08:00","2026-08-24T09:00"],
			"temperature_2m":[18.4,18.5],
			"apparent_temperature":[17.1,17.2],
			"precipitation":[0,0],
			"precipitation_probability":[10],
			"wind_speed_10m":[12.3,12.4],
			"wind_direction_10m":[240,241],
			"weather_code":[1,1],
			"cloud_cover":[50,51]}}`)
	}))
	defer server.Close()

	from := historyNow().AddDate(0, 0, -1)
	_, err := newHistoryClient(t, server).History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(time.Hour))
	require.ErrorContains(t, err, "series lengths did not match")
}

// The split is by local date, so a ride is asked of the endpoint that holds its
// day rather than of whichever the elapsed hours happen to round to.
func TestHistorySplitsByTheDateNotTheElapsedHours(t *testing.T) {
	var asked string
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		asked = request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, archiveBody)
	}))
	defer server.Close()

	client, err := New(&Options{
		BaseURL: server.URL, ArchiveBaseURL: server.URL, Timeout: time.Second,
		Transport: server.Client().Transport,
		// Just after midnight, Berlin time.
		Now: func() time.Time { return time.Date(2026, 8, 24, 0, 30, 0, 0, time.UTC) },
	})
	require.NoError(t, err)

	// 27.5 hours old, which elapsed hours round down to one day. In the
	// service's own zone the ride is on the 22nd and today is the 24th, so it
	// belongs to the archive rather than to the forecast endpoint.
	from := time.Date(2026, 8, 22, 21, 0, 0, 0, time.UTC)
	_, err = client.History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(time.Hour))
	require.NoError(t, err)
	assert.Equal(t, "/v1/archive", asked)
}

// The provider answers inside its own allowed range with nulls where it holds
// no value. Read as numbers those become a ride at nought degrees in still air,
// stored and drawn as though they were measured.
func TestHistoryLeavesOutAnHourTheProviderHadNoReadingFor(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"minutely_15":{
			"time":["2026-08-24T08:00","2026-08-24T09:00","2026-08-24T10:00"],
			"temperature_2m":[18.4,null,19.2],
			"apparent_temperature":[17.1,null,18.0],
			"precipitation":[0,null,0.2],
			"precipitation_probability":[10,null,20],
			"wind_speed_10m":[12.3,null,13.0],
			"wind_direction_10m":[240,null,245],
			"weather_code":[1,null,2],
			"cloud_cover":[50,null,55]}}`)
	}))
	defer server.Close()

	from := historyNow().Add(-2 * time.Hour)
	result, err := newHistoryClient(t, server).History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(2*time.Hour))
	require.NoError(t, err)
	require.Len(t, result, 1)

	assert.Equal(t, []float64{18.4, 19.2}, result[0].TemperatureCelsius,
		"the hour with no reading is gone, not zero")
	assert.Len(t, result[0].Time, 2, "and every series stays aligned with the times")
	assert.Equal(t, []float64{12.3, 13.0}, result[0].WindSpeedKMH)
	assert.Equal(t, []int{1, 2}, result[0].WeatherCode)
	assert.Equal(t, []float64{10, 20}, result[0].PrecipitationProbabilityPercent)
}

// A hole in the chance of rain alone is not a hole in the weather: the hour
// still says what it was like, and no chance recorded is no chance given.
func TestHistoryKeepsAnHourMissingOnlyTheChanceOfRain(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"minutely_15":{
			"time":["2026-08-24T08:00"],
			"temperature_2m":[18.4],
			"apparent_temperature":[17.1],
			"precipitation":[0],
			"precipitation_probability":[null],
			"wind_speed_10m":[12.3],
			"wind_direction_10m":[240],
			"weather_code":[1],
			"cloud_cover":[50]}}`)
	}))
	defer server.Close()

	from := historyNow().Add(-time.Hour)
	result, err := newHistoryClient(t, server).History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, result[0].Time, 1, "the hour is kept")
	assert.Equal(t, []float64{0}, result[0].PrecipitationProbabilityPercent)
}

// A window the provider holds nothing at all for yields no hours rather than a
// column of zeroes for every one of them.
func TestHistoryYieldsNoHoursWhenTheProviderHeldNone(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"minutely_15":{
			"time":["2026-08-24T08:00","2026-08-24T09:00"],
			"temperature_2m":[null,null],
			"apparent_temperature":[null,null],
			"precipitation":[null,null],
			"precipitation_probability":[null,null],
			"wind_speed_10m":[null,null],
			"wind_direction_10m":[null,null],
			"weather_code":[null,null],
			"cloud_cover":[null,null]}}`)
	}))
	defer server.Close()

	from := historyNow().Add(-time.Hour)
	result, err := newHistoryClient(t, server).History(t.Context(),
		[]Coordinate{{Latitude: 50.11, Longitude: 8.68}}, from, from.Add(time.Hour))
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Empty(t, result[0].Time, "nothing was recorded, so nothing is reported")
}

// StepFor is asked before the request, so it has to agree with the endpoint
// History will actually choose: a caller that samples once per step and is told
// the wrong one asks about the wrong number of places.
func TestStepForAgreesWithTheEndpointHistoryChooses(t *testing.T) {
	t.Parallel()
	client, err := New(&Options{Now: historyNow, Timezone: func() string { return "Europe/Berlin" }})
	require.NoError(t, err, "New()")

	assert.Equal(t, quarterHourStep, client.StepFor(historyNow()), "today")
	assert.Equal(t, quarterHourStep, client.StepFor(historyNow().AddDate(0, 0, -1)), "yesterday")
	assert.Equal(t, time.Hour, client.StepFor(historyNow().AddDate(0, 0, -2)),
		"the day the archive takes, which has no sub-hourly reanalysis behind it")
	assert.Equal(t, time.Hour, client.StepFor(historyNow().AddDate(0, 0, -400)), "last year")
}

// The zone is a runtime setting, so it can turn unloadable after construction
// validated it. The split is still made, against the fallback zone.
func TestStepForFallsBackWhenTheZoneStopsLoading(t *testing.T) {
	t.Parallel()
	asked := 0
	client, err := New(&Options{Now: historyNow, Timezone: func() string {
		asked++
		if asked > 1 {
			return "Not/AZone"
		}

		return "Europe/Berlin"
	}})
	require.NoError(t, err, "New()")

	assert.Equal(t, quarterHourStep, client.StepFor(historyNow()))
}

func TestHistoryRefusesAnEmptyRequest(t *testing.T) {
	client, err := New(&Options{Now: historyNow})
	require.NoError(t, err)

	_, err = client.History(t.Context(), nil, historyNow(), historyNow())
	require.ErrorContains(t, err, "at least one coordinate is required")

	_, err = client.History(t.Context(), []Coordinate{{}}, historyNow(), historyNow().Add(-time.Hour))
	require.ErrorContains(t, err, "to must not be before from")
}
