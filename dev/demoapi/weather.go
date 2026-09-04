package main

import (
	"context"
	"math"
	"time"

	"github.com/nobbs/domestique/internal/httpapi"
)

// syntheticWeather stands in for internal/openmeteo.Client.Forecast. Every
// provider below this handler is unroutable on purpose, and this keeps that true
// for weather: the reading at a coordinate and hour is pure arithmetic, identical
// on every run, so a browser test needs no recorded fixture. Two sine waves drive
// temperature and cloud, with a faster one layered on for rain.
func syntheticWeather(
	_ context.Context, latitudes, longitudes []float64, from, to time.Time,
) ([]httpapi.WeatherSeries, error) {
	hours := weatherHours(from, to)
	series := make([]httpapi.WeatherSeries, len(latitudes))
	for i := range latitudes {
		series[i] = syntheticSeriesAt(latitudes[i], longitudes[i], hours)
	}

	return series, nil
}

// weatherHours is every hour from..to resolves to, floor(from) through
// ceil(to) inclusive — the same rounding internal/openmeteo.Client.Forecast
// applies to the real provider's own hourly window, so a point at either edge
// of the requested span still has an hour to match against.
func weatherHours(from, to time.Time) []time.Time {
	start := from.Truncate(time.Hour)
	end := to.Truncate(time.Hour)
	if end.Before(to) {
		end = end.Add(time.Hour)
	}

	hours := make([]time.Time, 0, int(end.Sub(start)/time.Hour)+1)
	for hour := start; !hour.After(end); hour = hour.Add(time.Hour) {
		hours = append(hours, hour)
	}

	return hours
}

// weatherPhase is the one deterministic input every reading is a function of:
// a value that walks steadily with longitude and latitude — so two points a
// few kilometres apart along a stage read differently — and with the hour, so
// a multi-hour ride sees the sky change too.
func weatherPhase(latitude, longitude float64, hour time.Time) float64 {
	const (
		latitudeStep  = 1.0 / 9
		longitudeStep = 1.0 / 14
		hourStep      = 0.35
	)

	hourIndex := float64(hour.Unix()) / float64(time.Hour/time.Second)

	return latitude*latitudeStep + longitude*longitudeStep + hourIndex*hourStep
}

// syntheticSeriesAt builds one coordinate's hourly series. Every column has
// exactly len(hours) entries, which is what makes the series consistent for
// the httpapi handler's own weatherSeriesConsistent check.
func syntheticSeriesAt(latitude, longitude float64, hours []time.Time) httpapi.WeatherSeries {
	series := httpapi.WeatherSeries{
		Time:                            hours,
		TemperatureCelsius:              make([]float64, len(hours)),
		ApparentTemperatureCelsius:      make([]float64, len(hours)),
		PrecipitationMillimetres:        make([]float64, len(hours)),
		PrecipitationProbabilityPercent: make([]float64, len(hours)),
		WindSpeedKMH:                    make([]float64, len(hours)),
		WindDirectionDegrees:            make([]float64, len(hours)),
		WeatherCode:                     make([]int, len(hours)),
		CloudCoverPercent:               make([]float64, len(hours)),
	}

	for i, hour := range hours {
		phase := weatherPhase(latitude, longitude, hour)
		cloud := math.Sin(phase)
		// A faster, phase-shifted wave of its own, so rain does not simply
		// track cloud cover one-for-one along the route.
		rain := math.Sin(phase*2.7 + 1.1)

		precipitation, probability, code := weatherPrecipitation(cloud, rain)
		temperature := 14 + 9*cloud - 3*math.Max(rain, 0)
		windDirection := math.Mod(phase*(180/math.Pi), 360)
		if windDirection < 0 {
			windDirection += 360
		}

		series.TemperatureCelsius[i] = temperature
		series.ApparentTemperatureCelsius[i] = temperature - 1.5 + 1.5*math.Cos(phase)
		series.PrecipitationMillimetres[i] = precipitation
		series.PrecipitationProbabilityPercent[i] = probability
		series.WindSpeedKMH[i] = 12 + 9*math.Abs(math.Sin(phase*1.4))
		series.WindDirectionDegrees[i] = windDirection
		series.WeatherCode[i] = code
		// cloud is already the same sine wave the sky follows; rescaled from
		// [-1,1] into the 0-100 percent range the field reports.
		series.CloudCoverPercent[i] = 50 + 50*cloud
	}

	return series
}

// weatherPrecipitation turns the cloud and rain signals into a millimetre amount,
// a probability, and a WMO weather code from Open-Meteo's own vocabulary, so the
// strip renders exactly the icons a live forecast would ask for.
func weatherPrecipitation(cloud, rain float64) (millimetres, probabilityPercent float64, code int) {
	switch {
	case rain > 0.6:
		fraction := (rain - 0.6) / 0.4

		return 6 * fraction, 60 + 40*fraction, 63
	case rain > 0.2:
		fraction := (rain - 0.2) / 0.4

		return 1.5 * fraction, 35 + 25*fraction, 61
	case cloud > 0.5:
		return 0, 15, 3
	case cloud > 0:
		return 0, 8, 2
	case cloud > -0.5:
		return 0, 3, 1
	default:
		return 0, 0, 0
	}
}
