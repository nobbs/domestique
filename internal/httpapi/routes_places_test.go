package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const reversePath = "/v1/places/reverse?latitude=49.0094&longitude=8.4044"

type fakePlaces struct {
	err     error
	near    *PlaceNear
	name    string
	query   string
	matches []PlaceMatch
	// asked records the last coordinate, so a test can see what was passed on.
	asked [2]float64
}

func (p *fakePlaces) Search(_ context.Context, query string, near *PlaceNear) ([]PlaceMatch, error) {
	p.query, p.near = query, near
	if p.err != nil {
		return nil, p.err
	}

	return p.matches, nil
}

func (p *fakePlaces) Reverse(_ context.Context, latitude, longitude float64) (string, error) {
	p.asked = [2]float64{latitude, longitude}
	if p.err != nil {
		return "", p.err
	}

	return p.name, nil
}

type fakeSnapper struct {
	err   error
	moved bool
	// to is where a moved waypoint lands.
	to [2]float64
}

func (s *fakeSnapper) Snap(
	_ context.Context, latitude, longitude float64,
) (snapLatitude, snapLongitude float64, moved bool, err error) {
	if s.err != nil {
		return latitude, longitude, false, s.err
	}
	if !s.moved {
		return latitude, longitude, false, nil
	}

	return s.to[0], s.to[1], true, nil
}

// snapHandler builds a handler over one session identity and Snapper port.
func snapHandler(t *testing.T, sessions Sessions, snapper Snapper) *Handler {
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
			Snapper:          snapper,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{}, &fakeWeatherGrid{},
	)
	require.NoError(t, err, "New()")

	return handler
}

const snapPath = "/v1/places/snap?latitude=49.0094&longitude=8.4044"

func TestSnapPlaceMovesTheWaypointOntoAWay(t *testing.T) {
	handler := snapHandler(t, newFakeSessions(), &fakeSnapper{moved: true, to: [2]float64{49.0096, 8.4045}})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, snapPath))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.JSONEq(t, `{"latitude":49.0096,"longitude":8.4045,"snapped":true}`, recorder.Body.String())
}

func TestSnapPlaceAnswersAWaypointWithNoWayUnmoved(t *testing.T) {
	handler := snapHandler(t, newFakeSessions(), &fakeSnapper{})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, snapPath))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.JSONEq(t, `{"latitude":49.0094,"longitude":8.4044,"snapped":false}`, recorder.Body.String())
}

func TestSnapPlaceAnswersAnUnreadableMapAsUnavailable(t *testing.T) {
	handler := snapHandler(t, newFakeSessions(), &fakeSnapper{err: errors.New("disk")})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, snapPath))

	assert.Equal(t, http.StatusServiceUnavailable, recorder.Code, recorder.Body.String())
}

func TestSnapPlaceIsAdminOnly(t *testing.T) {
	handler := snapHandler(t, nonAdminSessions("rider-a"), &fakeSnapper{})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, snapPath))

	assert.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
}

func TestSnapPlaceIsUnregisteredWithNoPlanner(t *testing.T) {
	handler := snapHandler(t, newFakeSessions(), nil)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, snapPath))

	assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
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
		"not a coordinate": "/v1/places/reverse?latitude=NaN&longitude=8",
		"endless":          "/v1/places/reverse?latitude=49&longitude=Inf",
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

const searchPath = "/v1/places/search?query=Turmberg&latitude=49&longitude=8.4"

func TestSearchPlacesAnswersWhatTheGeocoderFound(t *testing.T) {
	places := &fakePlaces{matches: []PlaceMatch{
		{Name: "Turmberg", Context: "Durlach, Karlsruhe", Kind: "peak", Latitude: 49.0006, Longitude: 8.4868},
		{Name: "Turmbergbahn", Kind: "place", Latitude: 48.9992, Longitude: 8.4781},
	}}
	handler := placesHandler(t, newFakeSessions(), places)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, searchPath))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.JSONEq(t, `{"places":[
		{"name":"Turmberg","context":"Durlach, Karlsruhe","kind":"peak","latitude":49.0006,"longitude":8.4868},
		{"name":"Turmbergbahn","kind":"place","latitude":48.9992,"longitude":8.4781}
	]}`, recorder.Body.String())
	assert.Equal(t, "Turmberg", places.query)
	require.NotNil(t, places.near)
	assert.Equal(t, PlaceNear{Latitude: 49, Longitude: 8.4}, *places.near)
}

func TestSearchPlacesAnswersNoMatchAsAnEmptyList(t *testing.T) {
	handler := placesHandler(t, newFakeSessions(), &fakePlaces{})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, searchPath))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.JSONEq(t, `{"places":[]}`, recorder.Body.String())
}

func TestSearchPlacesAsksUnbiasedWithoutAPoint(t *testing.T) {
	places := &fakePlaces{}
	handler := placesHandler(t, newFakeSessions(), places)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/v1/places/search?query=%20Turmberg%20"))

	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
	assert.Nil(t, places.near)
	assert.Equal(t, "Turmberg", places.query, "the query is trimmed")
}

func TestSearchPlacesRefusesWhatItCannotAsk(t *testing.T) {
	handler := placesHandler(t, newFakeSessions(), &fakePlaces{})

	for name, target := range map[string]string{
		"no query":           "/v1/places/search",
		"too short":          "/v1/places/search?query=ab",
		"short once trimmed": "/v1/places/search?query=%20ab%20%20",
		"too long":           "/v1/places/search?query=" + strings.Repeat("a", 201),
		"latitude alone":     "/v1/places/search?query=Turmberg&latitude=49",
		"longitude alone":    "/v1/places/search?query=Turmberg&longitude=8",
		"off the earth":      "/v1/places/search?query=Turmberg&latitude=91&longitude=8",
	} {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, target))

			assert.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
		})
	}
}

func TestSearchPlacesCountsCharactersNotBytes(t *testing.T) {
	handler := placesHandler(t, newFakeSessions(), &fakePlaces{})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, "/v1/places/search?query=%C3%96d%C3%A9"))

	assert.Equal(t, http.StatusOK, recorder.Code, "three characters, five bytes")
}

func TestSearchPlacesAnswersAGeocoderFailureAsBadGateway(t *testing.T) {
	handler := placesHandler(t, newFakeSessions(), &fakePlaces{err: errors.New("unreachable")})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, searchPath))

	require.Equal(t, http.StatusBadGateway, recorder.Code, recorder.Body.String())
	assert.Contains(t, recorder.Body.String(), codeGeocodingFailed)
	assert.NotContains(t, recorder.Body.String(), "unreachable", "the geocoder's own error stays in the log")
}

func TestSearchPlacesIsAdminOnly(t *testing.T) {
	handler := placesHandler(t, nonAdminSessions("rider-a"), &fakePlaces{})

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, searchPath))

	assert.Equal(t, http.StatusForbidden, recorder.Code, recorder.Body.String())
}

func TestSearchPlacesIsUnregisteredWithNoGeocoder(t *testing.T) {
	handler := placesHandler(t, newFakeSessions(), nil)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, authenticatedRequest(http.MethodGet, searchPath))

	assert.Equal(t, http.StatusNotFound, recorder.Code, recorder.Body.String())
}
