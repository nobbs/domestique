package demo_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/demo"
	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/route"
)

func TestStraightLineRouterReturnsWaypointGeometry(t *testing.T) {
	t.Parallel()

	waypoints := []plan.Waypoint{
		{Longitude: 8.40, Latitude: 49.00},
		{Longitude: 8.41, Latitude: 49.01},
		{Longitude: 8.42, Latitude: 49.00},
	}

	points, err := (demo.StraightLineRouter{}).Route(t.Context(), waypoints, plan.Gravel)
	require.NoError(t, err)
	assert.Equal(t, []route.Point{
		{Longitude: 8.40, Latitude: 49.00},
		{Longitude: 8.41, Latitude: 49.01},
		{Longitude: 8.42, Latitude: 49.00},
	}, points)

	waypoints[0].Longitude = 0
	assert.InDelta(t, 8.40, points[0].Longitude, 0, "router must return independent geometry")
}
