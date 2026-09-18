package surface

import (
	"context"
	"fmt"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

// WaypointRadiusMetres is how far a placed waypoint may sit from a way and still
// be moved onto it. Twice the matcher's own radius: a click on a map is coarser
// than a GPS fix, and a point that far off is aiming at the road beside it.
const WaypointRadiusMetres = 50.0

// Snap moves a point onto the nearest way the surface map holds within
// radiusMetres. It reports false, and no error, where no way is that close or no
// map has been built, so a caller keeps the point where it was put.
func Snap(
	ctx context.Context, source Source, at measure.Coordinate, radiusMetres float64,
) (measure.Coordinate, bool, error) {
	if source.Generation() == "" {
		return at, false, nil
	}
	ways, err := source.Ways(ctx, []route.Point{{Longitude: at.Longitude, Latitude: at.Latitude}})
	if err != nil {
		return at, false, fmt.Errorf("surface: reading ways near a point: %w", err)
	}
	nearest, metres, found := measure.NearestPoint(wayLines(ways), at)
	if !found || metres > radiusMetres {
		return at, false, nil
	}

	return nearest, true, nil
}
