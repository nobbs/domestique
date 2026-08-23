package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// fakeWayFetcher returns one fixed classification for every point it is
// asked about, so labelSurfaces can be tested without a real on-disk
// osmindex.Index.
type fakeWayFetcher struct {
	err  error
	kind surface.Kind
}

func (f fakeWayFetcher) Ways(context.Context, []route.Point) ([]surface.Way, error) {
	if f.err != nil {
		return nil, f.err
	}

	return []surface.Way{{
		Kind: f.kind,
		Line: []surface.Coordinate{{Latitude: 50.0, Longitude: 8.0}, {Latitude: 50.001, Longitude: 8.0}},
	}}, nil
}

func TestLabelSurfacesAssignsTheMatchedKindToEachPositionedSample(t *testing.T) {
	samplesByRide := map[string][]sampleRow{
		"r1": {
			{RideID: "r1", Latitude: 50.0002, Longitude: 8.0, HasPosition: true},
			{RideID: "r1", HasPosition: false}, // no position: left at KindUnknown
		},
	}

	labelSurfaces(context.Background(), fakeWayFetcher{kind: surface.KindGravel}, samplesByRide)

	assert.Equal(t, surface.KindGravel, samplesByRide["r1"][0].Surface)
	assert.Equal(t, surface.KindUnknown, samplesByRide["r1"][1].Surface)
}

func TestLabelSurfacesLeavesARideUnlabelledWhenTheLookupFails(t *testing.T) {
	samplesByRide := map[string][]sampleRow{
		"r1": {{RideID: "r1", Latitude: 50.0, Longitude: 8.0, HasPosition: true}},
	}

	labelSurfaces(context.Background(), fakeWayFetcher{err: errWaysLookup}, samplesByRide)

	assert.Equal(t, surface.KindUnknown, samplesByRide["r1"][0].Surface)
}

var errWaysLookup = errors.New("ways lookup failed")
