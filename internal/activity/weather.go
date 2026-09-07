package activity

import (
	"context"
	"time"
)

// WeatherStep is one step of what a ride was actually ridden through.
//
// Step is how long it covers. A ride recent enough for the forecast endpoint is
// answered by the quarter hour; an older one by the hour, which is all the
// reanalysis behind it has. A ride is asked about once, so the step it was
// given is the step it keeps.
//
// PrecipitationProbabilityPercent is the one value that can be absent: an older
// ride is answered by reanalysis, which records what fell rather than what
// might have.
type WeatherStep struct {
	At                              time.Time
	Step                            time.Duration
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

// WeatherSummary is what a ride's steps come to, in one line: the range the
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
// of coordinates, and says beforehand how fine an answer it will give.
//
// StepFor is asked first because the ride is sampled once per step, and the
// coordinates have to be chosen before the request that carries them.
type WeatherSource interface {
	History(ctx context.Context, latitudes, longitudes []float64, from, to time.Time) ([]WeatherSeries, error)
	StepFor(at time.Time) time.Duration
}

// WeatherSeries is one coordinate's answer, column-oriented as the provider
// returns it: index i across every slice describes the same step. Step is how
// long each covers. An empty PrecipitationProbabilityPercent is a source that
// does not carry it.
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
	Step                            time.Duration
}

// WeatherAdapter adapts a pair of functions to WeatherSource, so the
// composition root can hand over openmeteo's own methods without this package
// importing it.
type WeatherAdapter struct {
	Read func(ctx context.Context, latitudes, longitudes []float64, from, to time.Time) ([]WeatherSeries, error)
	Step func(at time.Time) time.Duration
}

// History calls Read.
func (a WeatherAdapter) History(
	ctx context.Context, latitudes, longitudes []float64, from, to time.Time,
) ([]WeatherSeries, error) {
	return a.Read(ctx, latitudes, longitudes, from, to)
}

// StepFor calls Step.
func (a WeatherAdapter) StepFor(at time.Time) time.Duration { return a.Step(at) }
