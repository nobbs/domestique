package plan

import (
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

// cuesOf places the engine's turns along the plan's own line: each turn's
// distance along the engine's line, scaled onto the normalised length, as
// the walking windows are.
func cuesOf(points []route.Point, turns []RoutedTurn, totalMetres float64) []route.Cue {
	if len(turns) == 0 || len(points) < 2 || totalMetres <= 0 {
		return nil
	}
	along := make([]float64, len(points))
	for index := 1; index < len(points); index++ {
		along[index] = along[index-1] + measure.HaversineMetres(points[index-1].Coordinate(), points[index].Coordinate())
	}
	engineMetres := along[len(along)-1]
	if engineMetres <= 0 {
		return nil
	}
	scale := totalMetres / engineMetres
	cues := make([]route.Cue, 0, len(turns))
	for _, turn := range turns {
		// The last vertex is the finish, which the measurement contract never makes a cue.
		if turn.Index < 0 || turn.Index >= len(points)-1 {
			continue
		}
		cues = append(cues, route.Cue{Turn: turn.Turn, Metres: along[turn.Index] * scale, Exit: turn.Exit})
	}

	return cues
}
