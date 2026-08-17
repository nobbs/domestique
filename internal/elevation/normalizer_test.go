package elevation

import (
	"math"
	"testing"

	"github.com/nobbs/domestique/internal/route"
)

func TestNormalizerProcessResamplesAndRemovesIsolatedSpike(t *testing.T) {
	stage := elevatedStage(t, []float64{100, 100, 130, 100, 100})

	processed, err := New().Process(&stage)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	geometry := processed.Geometry()
	if got, want := len(geometry), 5; got != want {
		t.Fatalf("normalized point count = %d, want %d", got, want)
	}
	for index, point := range geometry {
		original := stage.Geometry()[index]
		if point.Latitude != original.Latitude || point.Longitude != original.Longitude {
			t.Errorf("normalized point[%d] coordinates = (%v, %v), want (%v, %v)", index, point.Latitude, point.Longitude, original.Latitude, original.Longitude)
		}
		if point.Elevation == nil {
			t.Fatalf("normalized point %d has no elevation", index)
		}
		if got := *point.Elevation; math.Abs(got-100) > 0.1 {
			t.Errorf("normalized elevation[%d] = %v, want approximately 100", index, got)
		}
	}
	if got, want := processed.ContentHash(), stage.ContentHash(); got != want {
		t.Errorf("content hash = %q, want %q", got, want)
	}
}

func TestNormalizerProcessPreservesIncompleteElevation(t *testing.T) {
	elevation := 100.0
	stage, err := route.NewStage(1, 1, "revision", "Route", "", []route.Point{
		{Latitude: 49, Longitude: 8, Elevation: &elevation},
		{Latitude: 49.001, Longitude: 8.001},
	}, "hash")
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	processed, err := New().Process(&stage)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}
	if got, want := processed.Geometry(), stage.Geometry(); !equalGeometry(got, want) {
		t.Errorf("incomplete geometry = %#v, want %#v", got, want)
	}
}

func elevatedStage(t *testing.T, elevations []float64) route.Stage {
	t.Helper()
	points := make([]route.Point, 0, len(elevations))
	for index, elevation := range elevations {
		points = append(points, route.Point{Latitude: 49 + float64(index)*25/earthRadiusMetres*180/math.Pi, Longitude: 8, Elevation: &elevation})
	}
	stage, err := route.NewStage(1, 1, "revision", "Route", "", points, "hash")
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}
	return stage
}

func equalGeometry(left, right []route.Point) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Longitude != right[index].Longitude || left[index].Latitude != right[index].Latitude || (left[index].Elevation == nil) != (right[index].Elevation == nil) {
			return false
		}
		if left[index].Elevation != nil && *left[index].Elevation != *right[index].Elevation {
			return false
		}
	}
	return true
}
