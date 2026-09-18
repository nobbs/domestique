package photon_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/photon"
)

func answer(properties string) string {
	return `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{` +
		properties + `}}]}`
}

// write answers a request, failing the test if the body cannot be sent.
func write(t *testing.T, writer http.ResponseWriter, body string) {
	t.Helper()
	_, err := writer.Write([]byte(body))
	assert.NoError(t, err)
}

// serving starts a geocoder for handler, and a client paced as fast as the
// adapter allows so tests do not wait on the courtesy interval.
func serving(t *testing.T, handler http.HandlerFunc) (*photon.Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := photon.New(&photon.Options{BaseURL: server.URL, MinInterval: time.Nanosecond})
	require.NoError(t, err)

	return client, server
}

func TestReverseAsksForTheCoordinate(t *testing.T) {
	t.Parallel()
	var asked string
	client, _ := serving(t, func(writer http.ResponseWriter, request *http.Request) {
		asked = request.URL.Path + "?" + request.URL.RawQuery
		write(t, writer, answer(`"street":"Kaiserstraße","housenumber":"12","city":"Karlsruhe"`))
	})

	name, err := client.Reverse(t.Context(), 49.0094, 8.4044)

	require.NoError(t, err)
	assert.Equal(t, "Kaiserstraße 12, Karlsruhe", name)
	assert.Contains(t, asked, "/reverse?")
	assert.Contains(t, asked, "lat=49.0094")
	assert.Contains(t, asked, "lon=8.4044")
}

func TestReverseNamesAPlaceWithNoAddress(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		write(t, writer, answer(`"name":"Turmberg","city":"Karlsruhe"`))
	})

	name, err := client.Reverse(t.Context(), 48.9988, 8.4761)

	require.NoError(t, err)
	assert.Equal(t, "Turmberg, Karlsruhe", name)
}

func TestReverseReadsNoNameAsAnAnswer(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		write(t, writer, `{"type":"FeatureCollection","features":[]}`)
	})

	name, err := client.Reverse(t.Context(), 49, 8)

	require.NoError(t, err)
	assert.Empty(t, name)
}

func TestReverseAsksOnceForACoordinateItKnows(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		write(t, writer, answer(`"name":"Turmberg"`))
	})

	first, err := client.Reverse(t.Context(), 48.99881, 8.47612)
	require.NoError(t, err)
	// Within eleven metres of the first, so the same place and the same answer.
	second, err := client.Reverse(t.Context(), 48.99883, 8.47614)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	assert.Equal(t, int64(1), calls.Load())
}

func TestReverseCategorisesAFailure(t *testing.T) {
	t.Parallel()
	for name, testCase := range map[string]struct {
		expected photon.Failure
		status   int
	}{
		"refused":  {status: http.StatusBadRequest, expected: photon.FailureRefused},
		"geocoder": {status: http.StatusBadGateway, expected: photon.FailureGeocoder},
		"response": {status: http.StatusNoContent, expected: photon.FailureResponse},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
				writer.WriteHeader(testCase.status)
			})

			_, err := client.Reverse(t.Context(), 49, 8)

			var failure *photon.Error
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, testCase.expected, failure.Category)
			assert.Equal(t, testCase.status, failure.Status)
		})
	}
}

func TestReverseRefusesAnUnparsableAnswer(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		write(t, writer, "not json")
	})

	_, err := client.Reverse(t.Context(), 49, 8)

	var failure *photon.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, photon.FailureResponse, failure.Category)
}

func TestReverseRefusesAnOversizedAnswer(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		write(t, writer, answer(`"name":"`+strings.Repeat("a", 1<<20)+`"`))
	})

	_, err := client.Reverse(t.Context(), 49, 8)

	var failure *photon.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, photon.FailureResponse, failure.Category)
}

func TestReverseRefusesACoordinateOffTheEarth(t *testing.T) {
	t.Parallel()
	var calls atomic.Int64
	client, _ := serving(t, func(http.ResponseWriter, *http.Request) { calls.Add(1) })

	_, err := client.Reverse(t.Context(), 91, 8)

	var failure *photon.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, photon.FailureRefused, failure.Category)
	assert.Equal(t, int64(0), calls.Load(), "nothing is asked of the geocoder")
}

func TestReverseReportsAnUnreachableGeocoder(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	client, err := photon.New(&photon.Options{BaseURL: server.URL})
	require.NoError(t, err)
	server.Close()

	_, err = client.Reverse(t.Context(), 49, 8)

	var failure *photon.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, photon.FailureUnreachable, failure.Category)
}

func TestReversePacesRequests(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		write(t, writer, answer(`"name":"Somewhere"`))
	}))
	t.Cleanup(server.Close)
	client, err := photon.New(&photon.Options{BaseURL: server.URL, MinInterval: 40 * time.Millisecond})
	require.NoError(t, err)

	started := time.Now()
	_, err = client.Reverse(t.Context(), 49, 8)
	require.NoError(t, err)
	_, err = client.Reverse(t.Context(), 50, 9)
	require.NoError(t, err)

	assert.GreaterOrEqual(t, time.Since(started), 40*time.Millisecond)
}

func TestNewRefusesAnUnusableBaseURL(t *testing.T) {
	t.Parallel()
	for name, baseURL := range map[string]string{
		"empty":      "",
		"no scheme":  "photon.example.test",
		"with path":  "https://photon.example.test/api",
		"with query": "https://photon.example.test?lang=de",
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := photon.New(&photon.Options{BaseURL: baseURL})

			require.ErrorContains(t, err, "base url")
		})
	}
}

func TestNewRefusesUnusableOptions(t *testing.T) {
	t.Parallel()
	_, err := photon.New(nil)
	require.ErrorContains(t, err, "options are required")

	_, err = photon.New(&photon.Options{BaseURL: "https://photon.example.test", Timeout: -time.Second})
	require.ErrorContains(t, err, "timeout")

	_, err = photon.New(&photon.Options{
		BaseURL: "https://photon.example.test", MinInterval: -time.Second,
	})
	require.ErrorContains(t, err, "interval")
}

func TestReverseStopsOnACancelledContext(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
		write(t, writer, answer(`"name":"Somewhere"`))
	})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := client.Reverse(ctx, 49, 8)

	require.Error(t, err)
}

func TestReverseLabelsAPlaceByWhatItKnowsOfIt(t *testing.T) {
	t.Parallel()
	for expected, properties := range map[string]string{
		"Kaiserstraße, Karlsruhe": `"street":"Kaiserstraße","city":"Karlsruhe"`,
		"Durlach":                 `"district":"Durlach"`,
	} {
		t.Run(expected, func(t *testing.T) {
			t.Parallel()
			client, _ := serving(t, func(writer http.ResponseWriter, _ *http.Request) {
				write(t, writer, answer(properties))
			})

			name, err := client.Reverse(t.Context(), 49, 8)

			require.NoError(t, err)
			assert.Equal(t, expected, name)
		})
	}
}

func TestReverseFollowsNoRedirect(t *testing.T) {
	t.Parallel()
	client, _ := serving(t, func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, "https://elsewhere.example.test/reverse", http.StatusFound)
	})

	_, err := client.Reverse(t.Context(), 49, 8)

	var failure *photon.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, http.StatusFound, failure.Status)
}

func TestErrorNamesItsCategoryAndStatusOnly(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "photon: refused (HTTP 400)",
		(&photon.Error{Category: photon.FailureRefused, Status: http.StatusBadRequest}).Error())
	assert.Equal(t, "photon: unreachable", (&photon.Error{Category: photon.FailureUnreachable}).Error())
}
