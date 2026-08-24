package main

import (
	"context"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// wayFetcher is the one method this package needs from *osmindex.Index — the
// interface osmindex.Index already satisfies. Depending on the interface
// rather than the concrete type keeps labelSurfaces testable without a real
// on-disk index.
type wayFetcher interface {
	Ways(ctx context.Context, points []route.Point) ([]surface.Way, error)
}

// labelSurfaces classifies every sample's ground with the repository's own
// classifier, one ride at a time — no new taxonomy, no new dependency, the
// same classifier the forward model will consume. A ride whose way lookup
// fails is left at surface.KindUnknown rather than aborting the run:
// crrForSurface's own fallback treats that the same as ground nobody
// surveyed. It reports how many rides it attempted a lookup for and how
// many of those failed, so a caller can tell "a few rides had no nearby
// ways" from "the index itself is broken" rather than the failure being
// invisible either way.
func labelSurfaces(ctx context.Context, index wayFetcher, samplesByRide map[string][]sampleRow) (attempted, failed int) {
	for _, samples := range samplesByRide {
		points := make([]route.Point, 0, len(samples))
		indices := make([]int, 0, len(samples))
		for i := range samples {
			if !samples[i].HasPosition {
				continue
			}
			points = append(points, route.Point{Latitude: samples[i].Latitude, Longitude: samples[i].Longitude})
			indices = append(indices, i)
		}
		if len(points) == 0 {
			continue
		}
		attempted++

		ways, err := index.Ways(ctx, points)
		if err != nil {
			failed++

			continue
		}
		kinds := surface.Match(points, ways)
		for j, kind := range kinds {
			samples[indices[j]].Surface = kind
		}
	}

	return attempted, failed
}
