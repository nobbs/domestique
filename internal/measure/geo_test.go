package measure_test

import (
	"testing"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHaversineMetresMatchesTheRouteReferenceForKnownPoints(t *testing.T) {
	t.Parallel()
	munich := measure.Coordinate{Latitude: 48.1372, Longitude: 11.5756}
	berlin := measure.Coordinate{Latitude: 52.5186, Longitude: 13.4083}

	got := measure.HaversineMetres(munich, berlin)
	want := referenceHaversineMetres(referencePoint{Latitude: munich.Latitude, Longitude: munich.Longitude},
		referencePoint{Latitude: berlin.Latitude, Longitude: berlin.Longitude})

	assert.InDelta(t, want, got, 1e-9)
	assert.InDelta(t, 504_000, got, 5_000, "roughly the known road-map distance")
}

func TestHaversineMetresIsZeroForIdenticalPoints(t *testing.T) {
	t.Parallel()
	point := measure.Coordinate{Latitude: 12.3, Longitude: 45.6}

	assert.InDelta(t, 0, measure.HaversineMetres(point, point), 1e-9)
}

func TestHaversineMetresMatchesTheRouteReferenceOverARandomCorpus(t *testing.T) {
	t.Parallel()
	coordinates := randomCoordinates(500)
	for index := 1; index < len(coordinates); index++ {
		got := measure.HaversineMetres(coordinates[index-1], coordinates[index])
		want := referenceHaversineMetres(
			referencePoint{Latitude: coordinates[index-1].Latitude, Longitude: coordinates[index-1].Longitude},
			referencePoint{Latitude: coordinates[index].Latitude, Longitude: coordinates[index].Longitude},
		)
		assert.InDelta(t, want, got, 1e-9)
	}
}

func TestCumulativeMetresIsNilForEmptyInput(t *testing.T) {
	t.Parallel()
	assert.Nil(t, measure.CumulativeMetres(nil))
	assert.Nil(t, measure.CumulativeMetres([]measure.Coordinate{}))
}

func TestCumulativeMetresStartsAtZeroForOnePoint(t *testing.T) {
	t.Parallel()
	got := measure.CumulativeMetres([]measure.Coordinate{{Latitude: 1, Longitude: 1}})

	require.Len(t, got, 1)
	assert.InDelta(t, 0, got[0], 1e-9)
}

func TestCumulativeMetresMatchesTheRouteReferenceCumulativeLoop(t *testing.T) {
	t.Parallel()
	coordinates := randomCoordinates(300)
	points := make([]referencePoint, len(coordinates))
	for index, coordinate := range coordinates {
		points[index] = referencePoint{Latitude: coordinate.Latitude, Longitude: coordinate.Longitude}
	}
	wantDistances := make([]float64, len(points))
	for index := 1; index < len(points); index++ {
		wantDistances[index] = wantDistances[index-1] + referenceHaversineMetres(points[index-1], points[index])
	}

	got := measure.CumulativeMetres(coordinates)
	require.Len(t, got, len(wantDistances))
	for index, want := range wantDistances {
		assert.InDelta(t, want, got[index], 1e-6)
	}
}

func TestCumulativeMetresIsTheRunningSum(t *testing.T) {
	t.Parallel()
	coordinates := []measure.Coordinate{
		{Latitude: 0, Longitude: 0},
		{Latitude: 0, Longitude: 1},
		{Latitude: 1, Longitude: 1},
	}

	got := measure.CumulativeMetres(coordinates)

	require.Len(t, got, 3)
	assert.InDelta(t, 0, got[0], 1e-9)
	step1 := measure.HaversineMetres(coordinates[0], coordinates[1])
	step2 := measure.HaversineMetres(coordinates[1], coordinates[2])
	assert.InDelta(t, step1, got[1], 1e-9)
	assert.InDelta(t, step1+step2, got[2], 1e-9)
}
