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
// It needs no credentials. The public instance is rate-limited and is never
// what a deployment routes with; the coordinates below are round numbers on
// public roads, not anyone's route.
package brouter_test

import (
	"testing"

	"github.com/nobbs/domestique/internal/brouter"
	"github.com/nobbs/domestique/internal/plan"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAcceptancePublicInstanceRoutesEveryProfile(t *testing.T) {
	client, err := brouter.New(&brouter.Options{BaseURL: "https://brouter.de"})
	require.NoError(t, err)

	waypoints := []plan.Waypoint{{Longitude: 8.68, Latitude: 50.11}, {Longitude: 8.70, Latitude: 50.12}}
	for _, profile := range []plan.Profile{plan.Trekking, plan.Fastbike, plan.Gravel} {
		points, err := client.Route(t.Context(), waypoints, profile)
		require.NoError(t, err, "profile %s", profile)
		require.GreaterOrEqual(t, len(points), 2, "profile %s", profile)
		assert.InDelta(t, waypoints[0].Longitude, points[0].Longitude, 0.01, "start longitude, profile %s", profile)
		assert.InDelta(t, waypoints[0].Latitude, points[0].Latitude, 0.01, "start latitude, profile %s", profile)
		assert.NotNil(t, points[0].Elevation, "elevation, profile %s", profile)
	}
}

func TestAcceptancePublicInstanceRefusesAnUncoveredPoint(t *testing.T) {
	client, err := brouter.New(&brouter.Options{BaseURL: "https://brouter.de"})
	require.NoError(t, err)

	_, err = client.Route(t.Context(), []plan.Waypoint{{Longitude: 0, Latitude: 0}, {Longitude: 0.1, Latitude: 0.1}}, plan.Trekking)
	var failure *brouter.Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, brouter.FailureRefused, failure.Category)
}
