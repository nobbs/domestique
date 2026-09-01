package wahoo

import (
	"strconv"

	"github.com/nobbs/domestique/internal/route"
)

func calculateMetrics(geometry []route.Point) routeMetrics {
	var metrics routeMetrics
	for index := 1; index < len(geometry); index++ {
		metrics.distance += route.HaversineMetres(geometry[index-1], geometry[index])
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

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}
