package surface

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

type classifySource struct {
	generation string
	ways       []Way
	called     bool
}

func (s *classifySource) Ways(context.Context, []route.Point) ([]Way, error) {
	s.called = true

	return s.ways, nil
}

func (s *classifySource) Generation() string { return s.generation }

func TestClassifyGeometryUsesTheCurrentSource(t *testing.T) {
	points := []route.Point{
		{Longitude: 8, Latitude: 49},
		{Longitude: 8.001, Latitude: 49},
		{Longitude: 8.002, Latitude: 49},
	}
	source := &classifySource{
		generation: "demo",
		ways: []Way{{ID: 1, Kind: KindGravel, Line: []measure.Coordinate{
			{Longitude: 8, Latitude: 49},
			{Longitude: 8.001, Latitude: 49},
			{Longitude: 8.002, Latitude: 49},
		}}},
	}

	ranges, matchedMetres, err := ClassifyGeometry(t.Context(), source, points)
	require.NoError(t, err)
	require.Len(t, ranges, 1)
	assert.Equal(t, KindGravel, ranges[0].Kind)
	assert.Equal(t, 0, ranges[0].StartIndex)
	assert.Equal(t, 2, ranges[0].EndIndex)
	assert.Greater(t, matchedMetres, 0.0)
	assert.True(t, source.called)
}

func TestClassifyGeometryLeavesGeometryUnclassifiedWithoutAMap(t *testing.T) {
	source := &classifySource{}

	ranges, matchedMetres, err := ClassifyGeometry(t.Context(), source, []route.Point{{Longitude: 8, Latitude: 49}})
	require.NoError(t, err)
	assert.Empty(t, ranges)
	assert.Zero(t, matchedMetres)
	assert.False(t, source.called)
}
