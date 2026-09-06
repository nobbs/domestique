package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"
)

// syntheticWeatherGrid stands in for internal/openmeteogrid.Client. Every
// provider below this handler is unroutable on purpose, and unlike
// syntheticWeather the map overlay's own .om binary format is not
// something worth reproducing in Go for a demo: it answers unavailable
// rather than fabricate bytes the browser's real reader would reject anyway.
//
// The weather-grid overlay has nothing to show in demo mode as a result. If
// that is ever worth fixing, bundle one small captured .om fixture and serve
// its bytes verbatim instead.
type syntheticWeatherGrid struct{}

func (syntheticWeatherGrid) Latest(context.Context) (*http.Response, error) {
	return unavailableWeatherGridResponse(), nil
}

func (syntheticWeatherGrid) Object(context.Context, time.Time, time.Time, string, string) (*http.Response, error) {
	return unavailableWeatherGridResponse(), nil
}

func unavailableWeatherGridResponse() *http.Response {
	recorder := httptest.NewRecorder()
	// relayWeatherGrid maps any non-2xx upstream status to 502; matching that
	// here keeps the demo honest about what a real client actually sees.
	recorder.WriteHeader(http.StatusBadGateway)

	return recorder.Result()
}
