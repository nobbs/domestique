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
