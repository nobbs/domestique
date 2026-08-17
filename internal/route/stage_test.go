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
