package komoot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func exampleTour(id int64) *tourDetail {
	tour := &tourDetail{
		ID:        id,
		Type:      tourTypePlanned,
		Name:      "  Weekend loop  ",
		ChangedAt: "2026-08-17T07:00:00.000Z",
	}
	tour.Embedded.Coordinates.Items = []coordinate{
		{Latitude: 49.0, Longitude: 8.4, Elevation: 100},
		{Latitude: 49.1, Longitude: 8.5, Elevation: 110},
	}

	return tour
}

func TestConvertTourProducesOneStageWithTrimmedTitle(t *testing.T) {
	stage, err := convertTour(exampleTour(45))
	require.NoError(t, err)

	assert.Equal(t, "domestique:komoot:45:stage:1", stage.Key().ExternalID())
	assert.Equal(t, "Weekend loop", stage.Title(), "no stage name is ever appended: a tour has no subdivision")
	assert.Len(t, stage.ContentHash(), 64)

	geometry := stage.Geometry()
	require.Len(t, geometry, 2)
	for index, point := range geometry {
		assert.NotNilf(t, point.Elevation, "geometry[%d].Elevation", index)
	}
}

func TestConvertTourUsesStableFallbackName(t *testing.T) {
	tour := exampleTour(46)
	tour.Name = "   "

	stage, err := convertTour(tour)
	require.NoError(t, err)
	assert.Equal(t, "Komoot 46", stage.Title())
}

func TestConvertTourIsDeterministic(t *testing.T) {
	first, err := convertTour(exampleTour(45))
	require.NoError(t, err)
	second, err := convertTour(exampleTour(45))
	require.NoError(t, err)

	assert.Equal(t, first.ContentHash(), second.ContentHash(),
		"reading the same unchanged tour twice must produce identical stages")
}

func TestConvertTourDiffersOnChangedGeometry(t *testing.T) {
	first, err := convertTour(exampleTour(45))
	require.NoError(t, err)

	changed := exampleTour(45)
	changed.Embedded.Coordinates.Items[0].Elevation = 999

	second, err := convertTour(changed)
	require.NoError(t, err)

	assert.NotEqual(t, first.ContentHash(), second.ContentHash())
}

func TestConvertTourRejectsInvalidID(t *testing.T) {
	_, err := convertTour(exampleTour(0))
	require.Error(t, err)
}

func TestConvertTourRejectsTooFewPoints(t *testing.T) {
	tour := exampleTour(45)
	tour.Embedded.Coordinates.Items = tour.Embedded.Coordinates.Items[:1]

	_, err := convertTour(tour)
	require.Error(t, err)
}
