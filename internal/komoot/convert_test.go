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
		{Latitude: floatPtr(49.0), Longitude: floatPtr(8.4), Elevation: floatPtr(100)}, //nolint:modernize // see floatPtr's doc comment
		{Latitude: floatPtr(49.1), Longitude: floatPtr(8.5), Elevation: floatPtr(110)}, //nolint:modernize // see floatPtr's doc comment
	}

	return tour
}

// floatPtr is a plain pointer-to-literal helper rather than new(expr): tooling
// that reviews this code may not recognise the newer form.
//
//nolint:modernize // deliberately not new(expr); see comment above.
func floatPtr(v float64) *float64 {
	return &v
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
	changed.Embedded.Coordinates.Items[0].Elevation = floatPtr(999)

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

func TestConvertTourRejectsMissingCoordinateField(t *testing.T) {
	tests := []struct {
		mutate func(*coordinate)
		name   string
	}{
		{name: "missing latitude", mutate: func(c *coordinate) { c.Latitude = nil }},
		{name: "missing longitude", mutate: func(c *coordinate) { c.Longitude = nil }},
		{name: "missing elevation", mutate: func(c *coordinate) { c.Elevation = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tour := exampleTour(45)
			test.mutate(&tour.Embedded.Coordinates.Items[0])

			_, err := convertTour(tour)
			require.ErrorContains(t, err, "missing a coordinate field")
		})
	}
}
