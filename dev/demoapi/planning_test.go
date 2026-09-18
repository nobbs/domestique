package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/brouter"
	"github.com/nobbs/domestique/internal/demo"
	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/route"
)

type testRouter struct{}

func (testRouter) Route(context.Context, []plan.Waypoint, plan.Profile) (plan.Routed, error) {
	return plan.Routed{Points: []route.Point{{Longitude: 8.4, Latitude: 49}, {Longitude: 8.5, Latitude: 49.1}}}, nil
}

type noPlans struct{}

func (noPlans) Inventory(context.Context) ([]route.Route, error) { return []route.Route{}, nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestBrouterRouterConvertsWaypointsAndProfile(t *testing.T) {
	client, err := brouter.New(&brouter.Options{
		BaseURL: "https://brouter.de",
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			assert.Equal(t, "gravel", request.URL.Query().Get("profile"))

			return &http.Response{
				StatusCode: http.StatusOK,
				Body: io.NopCloser(strings.NewReader(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
					`"geometry":{"type":"LineString","coordinates":[[8.68,50.11],[8.70,50.12]]}}]}`)),
			}, nil
		}),
	})
	require.NoError(t, err)
	routed, err := (brouterRouter{client: client}).Route(
		t.Context(), []plan.Waypoint{{Longitude: 8.68, Latitude: 50.11}, {Longitude: 8.70, Latitude: 50.12}}, plan.Gravel,
	)
	require.NoError(t, err)
	assert.Len(t, routed.Points, 2)
}

func TestSeedAddsPublishedPlansToTheDemoInventory(t *testing.T) {
	t.Parallel()

	store := demoStore(t)
	planService := plan.NewService(
		planStore{store: store},
		testRouter{},
		demoPace{},
		func() time.Time { return time.Date(2026, time.September, 15, 12, 0, 0, 0, time.UTC) },
		func() (int64, error) { return 42, nil },
	)
	created, err := planService.Create(
		t.Context(), "Demo plan", plan.Gravel,
		[]plan.Waypoint{{Longitude: 8.4, Latitude: 49}, {Longitude: 8.5, Latitude: 49.1}},
	)
	require.NoError(t, err)
	_, err = planService.Replace(
		t.Context(), created.ID, created.Version, created.Name, created.Profile, created.Waypoints, true,
	)
	require.NoError(t, err)

	require.NoError(t, seed(t.Context(), store, []demo.Slot{{ID: demoSubject, State: demo.SlotCurrent}}, planService))
	stages, err := store.TrustedInventory(t.Context())
	require.NoError(t, err)

	var local []route.Route
	for index := range stages {
		if stages[index].Key().Provider() == route.ProviderLocal {
			local = append(local, stages[index])
		}
	}
	require.Len(t, local, 1)
	assert.Equal(t, created.ID, local[0].Key().SourceRouteID())
}

func TestDemoSurfaceClassifierProvidesDeterministicBands(t *testing.T) {
	points := make([]route.Point, 12)
	for index := range points {
		points[index] = route.Point{Longitude: 8 + float64(index)/1000, Latitude: 49}
	}

	first, err := (demoSurfaceClassifier{}).Classify(t.Context(), points)
	require.NoError(t, err)
	second, err := (demoSurfaceClassifier{}).Classify(t.Context(), points)
	require.NoError(t, err)

	assert.Equal(t, first, second)
	require.Len(t, first.Ranges, 4)
	assert.Equal(t, "asphalt", first.Ranges[0].Kind)
	assert.Equal(t, "gravel", first.Ranges[1].Kind)
	assert.Equal(t, "ground", first.Ranges[2].Kind)
	assert.Equal(t, "paving", first.Ranges[3].Kind)
	assert.Greater(t, first.MatchedMetres, 0.0)
}
