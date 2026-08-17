package route

import (
	"math"
	"strings"
	"testing"
)

func TestNewStageCreatesImmutableStage(t *testing.T) {
	elevation := 42.5
	geometry := []Point{
		{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
		{Longitude: 8.5, Latitude: 49.1},
	}

	stage, err := NewStage(17, 2, "2026-08-16", "Morning ride", "Climb", geometry, "hash")
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	if got, want := stage.Key().ExternalID(), "domestique:veloplanner:17:stage:2"; got != want {
		t.Errorf("Key().ExternalID() = %q, want %q", got, want)
	}
	if got, want := stage.Title(), "Morning ride — Climb"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
	geometry[0].Longitude = 0
	*geometry[0].Elevation = 0
	returned := stage.Geometry()
	if got, want := returned[0].Longitude, 8.4; got != want {
		t.Errorf("Geometry()[0].Longitude = %v, want %v", got, want)
	}
	if got, want := *returned[0].Elevation, 42.5; got != want {
		t.Errorf("Geometry()[0].Elevation = %v, want %v", got, want)
	}
	*returned[0].Elevation = 0
	if got, want := *stage.Geometry()[0].Elevation, 42.5; got != want {
		t.Errorf("Geometry() leaked elevation pointer: got %v, want %v", got, want)
	}
}

func TestStageDistanceMetres(t *testing.T) {
	tests := []struct {
		name      string
		geometry  []Point
		want      float64
		tolerance float64
	}{
		{
			name:      "one degree of latitude",
			geometry:  []Point{{Longitude: 8, Latitude: 49}, {Longitude: 8, Latitude: 50}},
			want:      111_195,
			tolerance: 50,
		},
		{
			name: "accumulates across segments",
			geometry: []Point{
				{Longitude: 8, Latitude: 49},
				{Longitude: 8, Latitude: 49.5},
				{Longitude: 8, Latitude: 50},
			},
			want:      111_195,
			tolerance: 50,
		},
		{
			name:      "repeated point contributes nothing",
			geometry:  []Point{{Longitude: 8, Latitude: 49}, {Longitude: 8, Latitude: 49}},
			want:      0,
			tolerance: 0.001,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stage, err := NewStage(1, 1, "revision", "route", "", test.geometry, "hash")
			if err != nil {
				t.Fatalf("NewStage() error = %v", err)
			}
			if got := stage.DistanceMetres(); math.Abs(got-test.want) > test.tolerance {
				t.Errorf("DistanceMetres() = %v, want %v within %v", got, test.want, test.tolerance)
			}
		})
	}
}

func TestStageBounds(t *testing.T) {
	stage, err := NewStage(1, 1, "revision", "route", "", []Point{
		{Longitude: 8.5, Latitude: 49.2},
		{Longitude: 8.1, Latitude: 49.9},
		{Longitude: 8.9, Latitude: 49.0},
	}, "hash")
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	want := Bounds{MinLongitude: 8.1, MinLatitude: 49.0, MaxLongitude: 8.9, MaxLatitude: 49.9}
	if got := stage.Bounds(); got != want {
		t.Errorf("Bounds() = %+v, want %+v", got, want)
	}
}

// A profile that climbs a known amount over a known distance, so the expected
// gain and gradient can be reasoned about rather than recorded from output.
func elevationTestStage(t *testing.T, elevations []float64, latitudeStep float64) Stage {
	t.Helper()

	geometry := make([]Point, 0, len(elevations))
	for index, elevation := range elevations {
		metres := elevation
		geometry = append(geometry, Point{
			Longitude: 8.0,
			Latitude:  49.0 + float64(index)*latitudeStep,
			Elevation: &metres,
		})
	}
	stage, err := NewStage(1, 1, "revision", "route", "", geometry, "hash")
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	return stage
}

func TestStageElevationGainMetres(t *testing.T) {
	// Roughly 111 m of northing per 0.001 degrees of latitude.
	const step = 0.001

	tests := []struct {
		name       string
		elevations []float64
		want       float64
	}{
		{name: "flat", elevations: []float64{100, 100, 100}, want: 0},
		{name: "monotonic climb", elevations: []float64{100, 150, 200}, want: 100},
		{name: "descent is not counted", elevations: []float64{200, 150, 100}, want: 0},
		{name: "only the climbing parts", elevations: []float64{100, 150, 120, 170}, want: 100},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stage := elevationTestStage(t, test.elevations, step)
			if got := stage.ElevationGainMetres(); math.Abs(got-test.want) > 0.001 {
				t.Errorf("ElevationGainMetres() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestStageMaxGradientPercent(t *testing.T) {
	// 0.0018 degrees of latitude is about 200 m, so each step spans two
	// gradient windows and a gradient is measurable.
	const step = 0.0018

	t.Run("flat ground has no gradient", func(t *testing.T) {
		stage := elevationTestStage(t, []float64{100, 100, 100}, step)
		if got := stage.MaxGradientPercent(); got != 0 {
			t.Errorf("MaxGradientPercent() = %v, want 0", got)
		}
	})

	t.Run("reports the steepest section", func(t *testing.T) {
		// Second step climbs 20 m over ~200 m, roughly 10%; the first is gentler.
		stage := elevationTestStage(t, []float64{100, 102, 122}, step)
		got := stage.MaxGradientPercent()
		if got < 8 || got > 12 {
			t.Errorf("MaxGradientPercent() = %v, want roughly 10", got)
		}
	})

	t.Run("a descent is as steep as a climb", func(t *testing.T) {
		climbing := elevationTestStage(t, []float64{100, 120}, step)
		descending := elevationTestStage(t, []float64{120, 100}, step)
		climb := climbing.MaxGradientPercent()
		descent := descending.MaxGradientPercent()
		if math.Abs(climb-descent) > 0.001 {
			t.Errorf("descent gradient %v, want the same magnitude as the climb %v", descent, climb)
		}
	})

	t.Run("a single altitude spike cannot dominate", func(t *testing.T) {
		// Points 2 m apart: a 5 m spike between them would read as 250% if
		// gradient were measured between adjacent points.
		spiked := elevationTestStage(t, []float64{100, 105, 100, 100, 100}, 0.00002)
		if got := spiked.MaxGradientPercent(); got > 50 {
			t.Errorf("MaxGradientPercent() = %v, want a spike not to dominate", got)
		}
	})
}

func TestStageElevationStatisticsNeedACompleteProfile(t *testing.T) {
	partial, err := NewStage(1, 1, "revision", "route", "", []Point{
		{Longitude: 8.0, Latitude: 49.0},
		{Longitude: 8.0, Latitude: 49.01, Elevation: func() *float64 { v := 200.0; return &v }()},
	}, "hash")
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	if got := partial.ElevationGainMetres(); got != 0 {
		t.Errorf("ElevationGainMetres() = %v, want 0 without a complete profile", got)
	}
	if got := partial.MaxGradientPercent(); got != 0 {
		t.Errorf("MaxGradientPercent() = %v, want 0 without a complete profile", got)
	}
}

func TestZeroStageGeometryAccessorsAreSafe(t *testing.T) {
	var stage Stage

	if got := stage.DistanceMetres(); got != 0 {
		t.Errorf("DistanceMetres() = %v, want 0", got)
	}
	if got := (stage.Bounds()); got != (Bounds{}) {
		t.Errorf("Bounds() = %+v, want zero value", got)
	}
}

func TestNewStageRejectsInvalidIdentityAndGeometry(t *testing.T) {
	validGeometry := []Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.5, Latitude: 49.1}}
	tests := []struct {
		name     string
		want     string
		geometry []Point
		routeID  int64
		order    int
	}{
		{name: "route ID", routeID: 0, order: 1, geometry: validGeometry, want: "route ID"},
		{name: "stage order", routeID: 1, order: 0, geometry: validGeometry, want: "stage order"},
		{name: "short geometry", routeID: 1, order: 1, geometry: validGeometry[:1], want: "at least two"},
		{name: "longitude", routeID: 1, order: 1, geometry: []Point{{Longitude: 181, Latitude: 49}, {Longitude: 8, Latitude: 49}}, want: "longitude"},
		{name: "latitude", routeID: 1, order: 1, geometry: []Point{{Longitude: 8, Latitude: math.NaN()}, {Longitude: 8, Latitude: 49}}, want: "latitude"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewStage(test.routeID, test.order, "revision", "route", "stage", test.geometry, "hash")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("NewStage() error = %v, want substring %q", err, test.want)
			}
		})
	}
}
