package activity

import (
	"context"
	"time"
)

// WeatherHour is one hour of what a ride was actually ridden through.
//
// PrecipitationProbabilityPercent is the one value that can be absent: an older
// ride is answered by reanalysis, which records what fell rather than what
// might have.
type WeatherHour struct {
	Hour                            time.Time
	TemperatureCelsius              float64
	ApparentTemperatureCelsius      float64
	PrecipitationMillimetres        float64
	PrecipitationProbabilityPercent float64
	WindSpeedKMH                    float64
	WindDirectionDegrees            float64
	CloudCoverPercent               float64
	WeatherCode                     int
	HasPrecipitationProbability     bool
}

// PendingWeather is one ride still owed a weather read: when it started and how
// long it lasted, which together are the window to ask about.
type PendingWeather struct {
	StartedAt      time.Time
	ID             int64
	ElapsedSeconds float64
}

// WeatherSource asks a provider what the weather was over one window at a list
// of coordinates. Satisfied by *openmeteo.Client through its History method.
type WeatherSource interface {
	History(ctx context.Context, latitudes, longitudes []float64, from, to time.Time) ([]WeatherSeries, error)
}

// WeatherSeries is one coordinate's hourly answer, column-oriented as the
// provider returns it: index i across every slice describes the same hour. An
// empty PrecipitationProbabilityPercent is a source that does not carry it.
type WeatherSeries struct {
	Time                            []time.Time
	TemperatureCelsius              []float64
	ApparentTemperatureCelsius      []float64
	PrecipitationMillimetres        []float64
	PrecipitationProbabilityPercent []float64
	WindSpeedKMH                    []float64
	WindDirectionDegrees            []float64
	CloudCoverPercent               []float64
	WeatherCode                     []int
}

// WeatherFunc adapts a function to WeatherSource, so the composition root can
// hand over openmeteo's own method without this package importing it.
type WeatherFunc func(ctx context.Context, latitudes, longitudes []float64, from, to time.Time) ([]WeatherSeries, error)

// History calls f.
func (f WeatherFunc) History(
	ctx context.Context, latitudes, longitudes []float64, from, to time.Time,
) ([]WeatherSeries, error) {
	return f(ctx, latitudes, longitudes, from, to)
}
