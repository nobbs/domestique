package measure

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNearestPointDropsAPerpendicularOntoASegment(t *testing.T) {
	t.Parallel()
	origin := Coordinate{Longitude: 8.4, Latitude: 49}
	// A road running east, thirty metres north of the point.
	road := []Coordinate{metresNorth(metresEast(origin, -100), 30), metresNorth(metresEast(origin, 100), 30)}

	nearest, metres, found := NearestPoint([][]Coordinate{road}, origin)

	require.True(t, found)
	assert.InDelta(t, 30, metres, 0.5)
	assert.InDelta(t, origin.Longitude, nearest.Longitude, 1e-6, "straight up, not sideways")
	assert.InDelta(t, 30, HaversineMetres(origin, nearest), 0.5)
}

func TestNearestPointStopsAtASegmentsEnd(t *testing.T) {
	t.Parallel()
	origin := Coordinate{Longitude: 8.4, Latitude: 49}
	// A road ending forty metres east of the point: the nearest place is its end.
	end := metresEast(origin, 40)
	road := []Coordinate{metresEast(origin, 200), end}

	nearest, metres, found := NearestPoint([][]Coordinate{road}, origin)

	require.True(t, found)
	assert.InDelta(t, 40, metres, 0.5)
	assert.InDelta(t, end.Longitude, nearest.Longitude, 1e-7)
	assert.InDelta(t, end.Latitude, nearest.Latitude, 1e-7)
}

func TestNearestPointPrefersTheCloserOfTwoLines(t *testing.T) {
	t.Parallel()
	origin := Coordinate{Longitude: 8.4, Latitude: 49}
	far := []Coordinate{metresNorth(metresEast(origin, -50), 80), metresNorth(metresEast(origin, 50), 80)}
	near := []Coordinate{metresNorth(metresEast(origin, -50), -12), metresNorth(metresEast(origin, 50), -12)}

	_, metres, found := NearestPoint([][]Coordinate{far, near}, origin)

	require.True(t, found)
	assert.InDelta(t, 12, metres, 0.5)
}

func TestNearestPointFindsNothingWithoutASegment(t *testing.T) {
	t.Parallel()
	origin := Coordinate{Longitude: 8.4, Latitude: 49}

	_, _, found := NearestPoint([][]Coordinate{{origin}, nil}, origin)

	assert.False(t, found)
}
