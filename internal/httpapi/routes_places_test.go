package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reversePath = "/v1/places/reverse?latitude=49.0094&longitude=8.4044"

type fakePlaces struct {
	err  error
	name string
	// asked records the last coordinate, so a test can see what was passed on.
	asked [2]float64
}

func (p *fakePlaces) Reverse(_ context.Context, latitude, longitude float64) (string, error) {
	p.asked = [2]float64{latitude, longitude}
	if p.err != nil {
		return "", p.err
	}

	return p.name, nil
}

// placesHandler builds a handler over one session identity and Places port;
// places nil is a build with no geocoder configured.
func placesHandler(t *testing.T, sessions Sessions, places Places) *Handler {
	t.Helper()
	handler, err := New(
		&Options{
			schemaCache:      testSchemaCache,
			Alerts:           &fakeAlerts{},
			Tasks:            &fakeTasks{},
			Settings:         settingsWith(testBasemaps()),
			Sessions:         sessions,
			BrowserOriginURL: testBrowserOriginURL,
			Plans:            &fakePlans{},
			Places:           places,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{}, &fakeWeatherGrid{},
	)
	require.NoError(t, err, "New()")

	return handler
}

func TestReversePlaceNamesTheCoordinate(t *testing.T) {
	places := &fakePlaces{name: "Kaiserstraße 12, Karlsruhe"}
	handler := placesHandler(t, newFakeSessions(), places)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, reversePath))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.JSONEq(t, `{"name":"Kaiserstraße 12, Karlsruhe"}`, recorder.Body.String())
	assert.InDelta(t, 49.0094, places.asked[0], 0.00001, "latitude")
	assert.InDelta(t, 8.4044, places.asked[1], 0.00001, "longitude")
}

func TestReversePlaceOmitsANameThereIsNone(t *testing.T) {
	handler := placesHandler(t, newFakeSessions(), &fakePlaces{})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, reversePath))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.JSONEq(t, `{}`, recorder.Body.String())
}

func TestReversePlaceRefusesACoordinateItCannotRead(t *testing.T) {
	handler := placesHandler(t, newFakeSessions(), &fakePlaces{name: "Somewhere"})

	for name, target := range map[string]string{
		"no latitude":      "/v1/places/reverse?longitude=8",
		"no longitude":     "/v1/places/reverse?latitude=49",
		"not a number":     "/v1/places/reverse?latitude=north&longitude=8",
		"off the earth":    "/v1/places/reverse?latitude=91&longitude=8",
		"past the equator": "/v1/places/reverse?latitude=49&longitude=181",
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, target))

			assert.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		})
	}
}

func TestReversePlaceAnswersAGeocoderFailureAsBadGateway(t *testing.T) {
	handler := placesHandler(t, newFakeSessions(), &fakePlaces{err: errors.New("unreachable")})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, reversePath))

	require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), codeGeocodingFailed)
	assert.NotContains(t, recorder.Body.String(), "unreachable", "the geocoder's own error stays in the log")
}

func TestReversePlaceIsAdminOnly(t *testing.T) {
	handler := placesHandler(t, nonAdminSessions("rider-a"), &fakePlaces{name: "Somewhere"})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, reversePath))

	assert.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
}

func TestReversePlaceIsUnregisteredWithNoGeocoder(t *testing.T) {
	handler := placesHandler(t, newFakeSessions(), nil)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, reversePath))

	assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
}
