package wahoo

import (
	"math"
	"strconv"

	"github.com/nobbs/domestique/internal/route"
)

func calculateMetrics(geometry []route.Point) routeMetrics {
	var metrics routeMetrics
	for index := 1; index < len(geometry); index++ {
		metrics.distance += haversine(geometry[index-1], geometry[index])
		if geometry[index-1].Elevation != nil && geometry[index].Elevation != nil {
			delta := *geometry[index].Elevation - *geometry[index-1].Elevation
			if delta > 0 {
				metrics.ascent += delta
			} else {
				metrics.descent -= delta
			}
		}
	}

	return metrics
}

type routeMetrics struct {
	distance float64
	ascent   float64
	descent  float64
}

func haversine(left, right route.Point) float64 {
	latitudeDelta := (right.Latitude - left.Latitude) * math.Pi / 180
	longitudeDelta := (right.Longitude - left.Longitude) * math.Pi / 180
	leftLatitude := left.Latitude * math.Pi / 180
	rightLatitude := right.Latitude * math.Pi / 180
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)

	return earthRadiusMetre * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
