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

// WeatherSummary is what a ride's hours come to, in one line: the range the
// temperature moved over, the mean wind speed, the whole of what fell, and the
// worst of its weather codes. It carries no wind direction — a bearing does not
// average into a summary, and one line has no room to say why.
type WeatherSummary struct {
	TemperatureMinCelsius    float64
	TemperatureMaxCelsius    float64
	WindSpeedKMH             float64
	PrecipitationMillimetres float64
	WeatherCode              int
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
