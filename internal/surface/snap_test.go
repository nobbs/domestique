package surface

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

// waysSource hands back the same ways whatever it is asked about.
type waysSource struct {
	err        error
	generation string
	ways       []Way
}

func (s waysSource) Generation() string { return s.generation }

func (s waysSource) Ways(context.Context, []route.Point) ([]Way, error) { return s.ways, s.err }

// roadNorthOf is an east-west way the given metres north of a point.
func roadNorthOf(at measure.Coordinate, metres float64) Way {
	degreesNorth := metres / (measure.EarthRadiusMetres * math.Pi / 180)

	return Way{Line: []measure.Coordinate{
		{Longitude: at.Longitude - 0.002, Latitude: at.Latitude + degreesNorth},
		{Longitude: at.Longitude + 0.002, Latitude: at.Latitude + degreesNorth},
	}}
}

func TestSnapMovesAPointOntoANearbyWay(t *testing.T) {
	t.Parallel()
	at := measure.Coordinate{Longitude: 8.4, Latitude: 49}
	source := waysSource{generation: "g1", ways: []Way{roadNorthOf(at, 30)}}

	snapped, moved, err := Snap(t.Context(), source, at, WaypointRadiusMetres)

	require.NoError(t, err)
	require.True(t, moved)
	assert.InDelta(t, 30, measure.HaversineMetres(at, snapped), 0.5)
	assert.InDelta(t, at.Longitude, snapped.Longitude, 1e-6, "straight onto the road")
}

func TestSnapLeavesAPointFarFromAnyWay(t *testing.T) {
	t.Parallel()
	at := measure.Coordinate{Longitude: 8.4, Latitude: 49}
	source := waysSource{generation: "g1", ways: []Way{roadNorthOf(at, 120)}}

	snapped, moved, err := Snap(t.Context(), source, at, WaypointRadiusMetres)

	require.NoError(t, err)
	assert.False(t, moved)
	assert.Equal(t, at, snapped)
}

func TestSnapLeavesAPointWhereNoMapIsBuilt(t *testing.T) {
	t.Parallel()
	at := measure.Coordinate{Longitude: 8.4, Latitude: 49}
	// Ways it would snap to, but a source with no generation has built nothing.
	source := waysSource{ways: []Way{roadNorthOf(at, 10)}}

	snapped, moved, err := Snap(t.Context(), source, at, WaypointRadiusMetres)

	require.NoError(t, err)
	assert.False(t, moved)
	assert.Equal(t, at, snapped)
}

func TestSnapReportsAMapThatCannotBeRead(t *testing.T) {
	t.Parallel()
	at := measure.Coordinate{Longitude: 8.4, Latitude: 49}
	source := waysSource{generation: "g1", err: errors.New("disk")}

	snapped, moved, err := Snap(t.Context(), source, at, WaypointRadiusMetres)

	require.Error(t, err)
	assert.False(t, moved)
	assert.Equal(t, at, snapped)
}
