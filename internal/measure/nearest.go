package measure

import "math"

// NearestPoint is the point on any of the lines closest to at, and how far it
// lies from at in metres. It is false where no line has two coordinates.
//
// It scans every segment, so it suits a single question over the few ways near
// one point; a matcher asking of every point on a route wants SnapIndex.
func NearestPoint(lines [][]Coordinate, at Coordinate) (nearest Coordinate, metres float64, found bool) {
	frame := newProjection(at.Longitude, at.Latitude)
	metres = math.Inf(1)
	var bestEast, bestNorth float64
	for _, line := range lines {
		for index := 1; index < len(line); index++ {
			startEast, startNorth := frame.project(line[index-1].Longitude, line[index-1].Latitude)
			endEast, endNorth := frame.project(line[index].Longitude, line[index].Latitude)
			// The query point is the frame's origin, so the foot of the
			// perpendicular is the start moved along the run by its projection.
			runEast, runNorth := endEast-startEast, endNorth-startNorth
			lengthSquared := runEast*runEast + runNorth*runNorth
			fraction := 0.0
			if lengthSquared > 0 {
				fraction = math.Max(0, math.Min(1, -(startEast*runEast+startNorth*runNorth)/lengthSquared))
			}
			footEast, footNorth := startEast+fraction*runEast, startNorth+fraction*runNorth
			if distance := math.Hypot(footEast, footNorth); distance < metres {
				metres, bestEast, bestNorth, found = distance, footEast, footNorth, true
			}
		}
	}
	if !found {
		return Coordinate{}, 0, false
	}

	return frame.unproject(bestEast, bestNorth), metres, true
}

func (p projection) unproject(east, north float64) Coordinate {
	metresPerDegree := EarthRadiusMetres * math.Pi / 180

	return Coordinate{
		Longitude: p.referenceLongitude + east/(metresPerDegree*p.longitudeScale),
		Latitude:  p.referenceLatitude + north/metresPerDegree,
	}
}
