package elevation

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func TestNormalizerProcessResamplesAndRemovesIsolatedSpike(t *testing.T) {
	stage := elevatedStage(t, []float64{100, 100, 130, 100, 100})

	processed, err := New().Process(&stage)
	require.NoError(t, err)

	geometry := processed.Geometry()
	require.Len(t, geometry, 5, "normalization changed the point count")
	for index, point := range geometry {
		original := stage.Geometry()[index]
		assert.InDelta(t, original.Latitude, point.Latitude, 0, "normalized point[%d] moved in latitude", index)
		assert.InDelta(t, original.Longitude, point.Longitude, 0, "normalized point[%d] moved in longitude", index)
		require.NotNilf(t, point.Elevation, "normalized point %d has no elevation", index)
		assert.InDelta(t, 100, *point.Elevation, 0.1, "the spike survived at point %d", index)
	}
	assert.Equal(t, stage.ContentHash(), processed.ContentHash(), "normalization must not restate the source content")
}

func TestNormalizerProcessPreservesIncompleteElevation(t *testing.T) {
	elevation := 100.0
	stage, err := route.NewRoute(route.ProviderVeloPlanner, 1, 1, "revision", "Route", "", []route.Point{
		{Latitude: 49, Longitude: 8, Elevation: &elevation},
		{Latitude: 49.001, Longitude: 8.001},
	}, "hash")
	require.NoError(t, err)

	processed, err := New().Process(&stage)
	require.NoError(t, err)
	assert.Equal(t, stage.Geometry(), processed.Geometry(),
		"a stage missing an elevation must come back untouched")
}

func elevatedStage(t *testing.T, elevations []float64) route.Route {
	t.Helper()
	points := make([]route.Point, 0, len(elevations))
	for index, elevation := range elevations {
		points = append(points, route.Point{Latitude: 49 + float64(index)*25/route.EarthRadiusMetres*180/math.Pi, Longitude: 8, Elevation: &elevation})
	}
	stage, err := route.NewRoute(route.ProviderVeloPlanner, 1, 1, "revision", "Route", "", points, "hash")
	require.NoError(t, err)

	return stage
}
