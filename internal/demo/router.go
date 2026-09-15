package demo

import (
	"context"

	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/route"
)

// StraightLineRouter turns each waypoint into a route point. Adjacent points
// form the straight segments used by the demo without contacting a routing
// engine.
type StraightLineRouter struct{}

var _ plan.Router = StraightLineRouter{}

// Route returns a copy of the waypoints as a deterministic demo geometry.
func (StraightLineRouter) Route(_ context.Context, waypoints []plan.Waypoint, _ plan.Profile) ([]route.Point, error) {
	points := make([]route.Point, len(waypoints))
	for index, waypoint := range waypoints {
		points[index] = route.Point{Longitude: waypoint.Longitude, Latitude: waypoint.Latitude}
	}

	return points, nil
}
