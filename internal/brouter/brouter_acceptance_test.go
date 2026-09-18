//go:build brouter_acceptance

// This file is not compiled into the normal suite. It contacts the BRouter
// project's public instance, because the fixture in client_test.go asserts
// the request this client sends and always agrees with it; only an engine can
// say whether it accepts that request and answers in the shape parsed here.
//
// Invoke it on its own:
//
//	go test -tags brouter_acceptance ./internal/brouter/ -run Acceptance -v
//
// It needs no credentials. The public instance is rate-limited, and it is
// also a valid engine for a deployment that accepts sending waypoints there;
// the coordinates below are round numbers on public roads, not anyone's route.
package brouter_test

import (
	"testing"

	"github.com/nobbs/domestique/internal/brouter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcceptancePublicInstanceRoutesEveryProfile(t *testing.T) {
	client, err := brouter.New(&brouter.Options{BaseURL: "https://brouter.de"})
	require.NoError(t, err)

	waypoints := []brouter.Waypoint{{Longitude: 8.68, Latitude: 50.11}, {Longitude: 8.70, Latitude: 50.12}}
	for _, profile := range []string{"trekking", "fastbike", "gravel"} {
		answer, err := client.Route(t.Context(), waypoints, profile, nil)
		require.NoError(t, err, "profile %s", profile)
		points := answer.Points
		assert.NotEmpty(t, answer.Ways, "the ways under the line, profile %s", profile)
		require.GreaterOrEqual(t, len(points), 2, "profile %s", profile)
		assert.InDelta(t, waypoints[0].Longitude, points[0].Longitude, 0.01, "start longitude, profile %s", profile)
		assert.InDelta(t, waypoints[0].Latitude, points[0].Latitude, 0.01, "start latitude, profile %s", profile)
		assert.NotNil(t, points[0].Elevation, "elevation, profile %s", profile)
	}
}

func TestAcceptancePublicInstanceRefusesAnUncoveredPoint(t *testing.T) {
	client, err := brouter.New(&brouter.Options{BaseURL: "https://brouter.de"})
	require.NoError(t, err)

	_, err = client.Route(
		t.Context(), []brouter.Waypoint{{Longitude: 0, Latitude: 0}, {Longitude: 0.1, Latitude: 0.1}}, "trekking", nil)
	var failure *brouter.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, brouter.FailureRefused, failure.Category)
}

// A straight leg runs from one waypoint to the next as a bare line, and an
// avoided circle on the routed line moves it: both as this adapter sends them.
func TestAcceptancePublicInstanceHonoursStraightLegsAndAvoidedAreas(t *testing.T) {
	client, err := brouter.New(&brouter.Options{BaseURL: "https://brouter.de"})
	require.NoError(t, err)
	start := brouter.Waypoint{Longitude: 8.3985, Latitude: 49.0095}
	finish := brouter.Waypoint{Longitude: 8.4350, Latitude: 49.0020}

	routed, err := client.Route(t.Context(), []brouter.Waypoint{start, finish}, "trekking", nil)
	require.NoError(t, err)
	straight, err := client.Route(t.Context(), []brouter.Waypoint{start, {Longitude: finish.Longitude, Latitude: finish.Latitude, Straight: true}}, "trekking", nil)
	require.NoError(t, err)
	avoided, err := client.Route(t.Context(), []brouter.Waypoint{start, {Longitude: finish.Longitude, Latitude: finish.Latitude}}, "trekking",
		[]brouter.Nogo{{Longitude: 8.4170, Latitude: 49.0060, RadiusMetres: 300}})
	require.NoError(t, err)

	assert.Len(t, straight.Points, 2, "a straight leg is the two waypoints alone")
	assert.Greater(t, len(routed.Points), 2, "a routed leg follows the ways")
	assert.NotEqual(t, routed.Points, avoided.Points, "an avoided area moves the line")
}
