package activity

import (
	"context"
	"math"
	"time"
)

// weatherRidesPerRun bounds how many rides one run asks about. A backfill of a
// whole stored history is a few hundred requests; spreading them over runs
// keeps one from contending with the course forecasts a rider is waiting on.
const weatherRidesPerRun = 20

// maximumWeatherPoints bounds the coordinates one ride is asked about, however
// long it lasted. A ride is sampled at one point per hour, so this is a day on
// the bicycle — beyond it the ride is asked about at a coarser spacing rather
// than with a request nobody's quota can afford.
const maximumWeatherPoints = 24

// WeatherStore is what recording a ride's weather needs of stored state.
type WeatherStore interface {
	// ActivitiesAwaitingWeather lists the rides nobody has asked the weather
	// about, newest first, at most limit of them.
	ActivitiesAwaitingWeather(ctx context.Context, targetID string, limit int) ([]PendingWeather, error)
	// ActivityTrack lists the positioned samples of one ride, in the order they
	// were recorded.
	ActivityTrack(ctx context.Context, targetID string, id int64) ([]TrackPoint, error)
	// StoreActivityWeather replaces one ride's weather and records that it was
	// asked about. An empty set of hours records the asking and nothing else.
	StoreActivityWeather(ctx context.Context, targetID string, id int64,
		hours []WeatherHour, readAt time.Time) error
}

// readWeather asks a provider what the rides nobody has asked about were ridden
// through, at most a bounded few per run.
//
// A ride is asked about once. Whatever comes back, including nothing, is
// recorded as having been asked, so a ride the provider has no data for costs
// one request rather than one per run forever. A provider failure is not
// recorded that way — it is a run to try again, not an answer.
func (d *Deriver) readWeather(ctx context.Context, targetID string) Result {
	if d.weather == nil || d.weatherStore == nil {
		return Result{Outcome: NotReady}
	}
	pending, err := d.weatherStore.ActivitiesAwaitingWeather(ctx, targetID, weatherRidesPerRun)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	read := 0
	for _, ride := range pending {
		outcome := d.readOneRidesWeather(ctx, targetID, ride)
		if outcome.Outcome == Failed {
			outcome.Derived = read

			return outcome
		}
		read++
	}
	if read == 0 {
		return Result{Outcome: Unchanged}
	}

	return Result{Outcome: Polled, Derived: read}
}

// readOneRidesWeather asks about one ride and records the answer.
func (d *Deriver) readOneRidesWeather(ctx context.Context, targetID string, ride PendingWeather) Result {
	track, err := d.weatherStore.ActivityTrack(ctx, targetID, ride.ID)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	from, to := ride.StartedAt, ride.StartedAt.Add(time.Duration(ride.ElapsedSeconds)*time.Second)
	points := weatherPoints(track, from, to)
	if len(points) == 0 {
		// A ride with no track has no place to ask about. Recorded as asked all
		// the same: no later run will find it a track it did not record.
		return d.recordWeather(ctx, targetID, ride.ID, nil)
	}
	latitudes := make([]float64, len(points))
	longitudes := make([]float64, len(points))
	for index, point := range points {
		latitudes[index], longitudes[index] = point.Latitude, point.Longitude
	}
	series, err := d.weather.History(ctx, latitudes, longitudes, from, to)
	if err != nil {
		// The provider, not the store: worth trying again rather than recording
		// as an answer.
		return Result{Outcome: Failed, Failure: FailureUpstream}
	}

	return d.recordWeather(ctx, targetID, ride.ID, hoursOf(series, from, to))
}

func (d *Deriver) recordWeather(ctx context.Context, targetID string, id int64, hours []WeatherHour) Result {
	if err := d.weatherStore.StoreActivityWeather(ctx, targetID, id, hours, d.now()); err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}

	return Result{Outcome: Polled}
}

// weatherPoints picks where along the ride to ask: one point per hour of it,
// the first and the last always among them, so a long ride crossing a front is
// not described by where it happened to start.
func weatherPoints(track []TrackPoint, from, to time.Time) []TrackPoint {
	if len(track) == 0 {
		return nil
	}
	hours := int(math.Ceil(to.Sub(from).Hours()))
	wanted := min(max(hours, 1)+1, maximumWeatherPoints, len(track))
	if wanted <= 1 {
		return track[:1]
	}
	points := make([]TrackPoint, 0, wanted)
	last := len(track) - 1
	for index := range wanted {
		// Evenly spaced across the recorded track, ends included: index 0 is the
		// first sample and the last is the last, whatever the spacing between.
		points = append(points, track[last*index/(wanted-1)])
	}

	return points
}

// hoursOf reduces the provider's per-coordinate series to one row per hour of
// the ride, averaging across the coordinates that hour was asked at: a ride is
// one thing, and its rider was somewhere along it rather than at all of them.
func hoursOf(series []WeatherSeries, from, to time.Time) []WeatherHour {
	type accumulator struct {
		hour        WeatherHour
		count       float64
		probability float64
	}
	byHour := map[int64]*accumulator{}
	order := []int64{}
	for seriesIndex := range series {
		one := &series[seriesIndex]
		for index, at := range one.Time {
			if at.Before(from.Truncate(time.Hour)) || at.After(to) {
				continue
			}
			key := at.Unix()
			into, seen := byHour[key]
			if !seen {
				into = &accumulator{hour: WeatherHour{Hour: at.UTC()}}
				byHour[key], order = into, append(order, key)
			}
			into.count++
			into.hour.TemperatureCelsius += one.TemperatureCelsius[index]
			into.hour.ApparentTemperatureCelsius += one.ApparentTemperatureCelsius[index]
			into.hour.PrecipitationMillimetres += one.PrecipitationMillimetres[index]
			into.hour.WindSpeedKMH += one.WindSpeedKMH[index]
			into.hour.WindDirectionDegrees += one.WindDirectionDegrees[index]
			into.hour.CloudCoverPercent += one.CloudCoverPercent[index]
			if index < len(one.PrecipitationProbabilityPercent) {
				into.probability += one.PrecipitationProbabilityPercent[index]
				into.hour.HasPrecipitationProbability = true
			}
			// The code is the worst of them rather than a mean: half a ride in
			// rain was ridden in rain, and averaging a code is meaningless anyway.
			into.hour.WeatherCode = max(into.hour.WeatherCode, one.WeatherCode[index])
		}
	}
	hours := make([]WeatherHour, 0, len(order))
	for _, key := range order {
		into := byHour[key]
		hour := into.hour
		hour.TemperatureCelsius /= into.count
		hour.ApparentTemperatureCelsius /= into.count
		hour.PrecipitationMillimetres /= into.count
		hour.WindSpeedKMH /= into.count
		hour.WindDirectionDegrees /= into.count
		hour.CloudCoverPercent /= into.count
		if hour.HasPrecipitationProbability {
			hour.PrecipitationProbabilityPercent = into.probability / into.count
		}
		hours = append(hours, hour)
	}

	return hours
}
