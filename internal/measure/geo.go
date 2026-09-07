package measure

import "math"

// EarthRadiusMetres is the one spherical Earth model distances are computed
// on, so every package's lengths agree with the ones shown beside them.
const EarthRadiusMetres = 6_371_000.0

// Coordinate is one point on the Earth's surface, in decimal degrees.
type Coordinate struct {
	Latitude, Longitude float64
}

// HaversineMetres returns the great-circle distance between two coordinates
// on the EarthRadiusMetres sphere.
//
// See docs/specs/measurement.md §Spherical distance.
func HaversineMetres(a, b Coordinate) float64 {
	latitudeDelta := (b.Latitude - a.Latitude) * math.Pi / 180
	longitudeDelta := (b.Longitude - a.Longitude) * math.Pi / 180
	leftLatitude := a.Latitude * math.Pi / 180
	rightLatitude := b.Latitude * math.Pi / 180
	chord := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*
			math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)

	return EarthRadiusMetres * 2 * math.Atan2(math.Sqrt(chord), math.Sqrt(1-chord))
}

// CumulativeMetres is the running haversine distance along coordinates, one
// entry per coordinate with the first at zero. Nil for empty input.
//
// See docs/specs/measurement.md §Spherical distance.
func CumulativeMetres(coordinates []Coordinate) []float64 {
	if len(coordinates) == 0 {
		return nil
	}
	distances := make([]float64, len(coordinates))
	for index := 1; index < len(coordinates); index++ {
		distances[index] = distances[index-1] + HaversineMetres(coordinates[index-1], coordinates[index])
	}

	return distances
}
