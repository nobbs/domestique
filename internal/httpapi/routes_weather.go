package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

const (
	// maximumWeatherPoints bounds one request to roughly double the realistic
	// worst case: a 200 km audax samples at about 25 points. The cap exists so
	// a malformed page cannot turn one request into an unbounded outbound one.
	maximumWeatherPoints = 48

	// weatherPastAllowance and weatherForecastHorizon bound the window this
	// service will ask Open-Meteo for, so a malformed window is refused here
	// rather than passed through to be rejected remotely.
	weatherPastAllowance   = 24 * time.Hour
	weatherForecastHorizon = 16 * 24 * time.Hour
)

// GetWeather answers the browser's request for a forecast at each point of a
// planned ride, without the page reaching Open-Meteo itself. It derives from and
// to as the earliest and latest point time, calls Forecast once for every point
// together, and resolves each to the hour nearest its own time.
func (h *Handler) GetWeather(writer http.ResponseWriter, request *http.Request) {
	raw := request.URL.Query()["point"]
	if len(raw) == 0 {
		h.error(writer, http.StatusBadRequest, "invalid_request", "at least one point is required")

		return
	}
	if len(raw) > maximumWeatherPoints {
		h.error(writer, http.StatusBadRequest, "invalid_request",
			"no more than "+strconv.Itoa(maximumWeatherPoints)+" points are allowed")

		return
	}

	points := make([]weatherPoint, len(raw))
	for i, value := range raw {
		point, ok := parseWeatherPoint(value)
		if !ok {
			h.error(writer, http.StatusBadRequest, "invalid_request", "point "+strconv.Itoa(i)+" is malformed")

			return
		}
		points[i] = point
	}

	from, to := points[0].At, points[0].At
	latitudes := make([]float64, len(points))
	longitudes := make([]float64, len(points))
	for i, point := range points {
		latitudes[i] = point.Latitude
		longitudes[i] = point.Longitude
		if point.At.Before(from) {
			from = point.At
		}
		if point.At.After(to) {
			to = point.At
		}
	}
	if !h.weatherWindowFits(from, to) {
		h.error(writer, http.StatusBadRequest, "invalid_request", "the time window is outside what the provider can forecast")

		return
	}

	series, err := h.weather.Forecast(request.Context(), latitudes, longitudes, from, to)
	if err != nil {
		h.error(writer, http.StatusBadGateway, "provider_unavailable", "the weather provider could not be reached")

		return
	}
	if len(series) != len(points) {
		h.error(writer, http.StatusBadGateway, "provider_unavailable", "the weather provider returned an unexpected response")

		return
	}

	view := openapi.WeatherForecast{Points: make([]openapi.WeatherPoint, len(points))}
	for i, point := range points {
		if !weatherSeriesConsistent(&series[i]) {
			h.error(writer, http.StatusBadGateway, "provider_unavailable",
				"the weather provider returned an inconsistent forecast series")

			return
		}
		index, found := nearestHourIndex(series[i].Time, point.At)
		if !found {
			h.error(writer, http.StatusBadGateway, "provider_unavailable",
				"the weather provider returned no forecast for one of the requested points")

			return
		}
		view.Points[i] = newWeatherPointView(&series[i], index)
	}
	h.writeJSON(writer, http.StatusOK, view)
}

// weatherWindowFits rejects a window Open-Meteo's forecast endpoint could
// never answer: more than a day in the past, or beyond its forecast horizon.
func (h *Handler) weatherWindowFits(from, to time.Time) bool {
	now := h.now()

	return !from.Before(now.Add(-weatherPastAllowance)) && !to.After(now.Add(weatherForecastHorizon))
}

// weatherPoint is one coordinate and time the browser asked a forecast for.
type weatherPoint struct {
	At                  time.Time
	Latitude, Longitude float64
}

// parseWeatherPoint reads "latitude,longitude,time", where time is a full
// RFC3339 timestamp with an offset (or Z) and seconds, the way the service
// specification defines the query parameter.
func parseWeatherPoint(raw string) (point weatherPoint, ok bool) {
	parts := strings.SplitN(raw, ",", 3)
	if len(parts) != 3 {
		return weatherPoint{}, false
	}
	latitude, latErr := strconv.ParseFloat(parts[0], 64)
	longitude, lonErr := strconv.ParseFloat(parts[1], 64)
	at, timeErr := time.Parse(time.RFC3339, parts[2])
	if latErr != nil || lonErr != nil || timeErr != nil ||
		latitude < -90 || latitude > 90 || longitude < -180 || longitude > 180 {
		return weatherPoint{}, false
	}

	return weatherPoint{Latitude: latitude, Longitude: longitude, At: at}, true
}

// weatherSeriesConsistent reports whether every field of series names as many
// hours as its Time slice does. Weather is an interface a future
// implementation, or a test double, could satisfy with mismatched slices; this
// is what turns that into a 502 rather than an index panic below.
func weatherSeriesConsistent(series *WeatherSeries) bool {
	count := len(series.Time)

	return len(series.TemperatureCelsius) == count &&
		len(series.ApparentTemperatureCelsius) == count &&
		len(series.PrecipitationMillimetres) == count &&
		len(series.PrecipitationProbabilityPercent) == count &&
		len(series.WindSpeedKMH) == count &&
		len(series.WindDirectionDegrees) == count &&
		len(series.WeatherCode) == count
}

// nearestHourIndex finds the hour in an hourly series closest to at. The
// requested window always covers every point's own time, so this is an exact
// match in practice; nearest is the defensive answer to a provider series
// that turns out not to align to the hour.
func nearestHourIndex(hours []time.Time, at time.Time) (index int, found bool) {
	if len(hours) == 0 {
		return 0, false
	}
	best := 0
	bestDelta := hours[0].Sub(at).Abs()
	for i := 1; i < len(hours); i++ {
		if delta := hours[i].Sub(at).Abs(); delta < bestDelta {
			best, bestDelta = i, delta
		}
	}

	return best, true
}

func newWeatherPointView(series *WeatherSeries, index int) openapi.WeatherPoint {
	return openapi.WeatherPoint{
		Time:                            wireTime(series.Time[index]),
		TemperatureCelsius:              series.TemperatureCelsius[index],
		ApparentTemperatureCelsius:      series.ApparentTemperatureCelsius[index],
		PrecipitationMillimetres:        series.PrecipitationMillimetres[index],
		PrecipitationProbabilityPercent: series.PrecipitationProbabilityPercent[index],
		WindSpeedKmh:                    series.WindSpeedKMH[index],
		WindDirectionDegrees:            series.WindDirectionDegrees[index],
		WeatherCode:                     series.WeatherCode[index],
	}
}
