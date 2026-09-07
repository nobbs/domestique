package activity

import (
	"context"
	"math"
	"slices"
	"time"
)

// weatherRidesPerRun bounds how many rides one run asks about. A backfill of a
// whole stored history is a few hundred requests; spreading them over runs
// keeps one from contending with the course forecasts a rider is waiting on.
const weatherRidesPerRun = 20

// maximumWeatherPoints bounds the coordinates one ride is asked about, however
// long it lasted and however fine its step. Beyond it the ride is asked about
// at a coarser spacing rather than with a request nobody's quota can afford: a
// quarter-hourly ride over six hours is sampled more coarsely in space than in
// time, which is the right way round — the weather moves faster than the rider.
const maximumWeatherPoints = 24

// weatherStepFallback is the step assumed for a provider that named none. Only
// a hand-built source reaches this; both real endpoints say what they answered.
const weatherStepFallback = time.Hour

// WeatherStore is what recording a ride's weather needs of stored state.
type WeatherStore interface {
	// ActivitiesAwaitingWeather lists the rides nobody has asked the weather
	// about, newest first, at most limit of them.
	ActivitiesAwaitingWeather(ctx context.Context, targetID string, limit int) ([]PendingWeather, error)
	// ActivityTrack lists the positioned samples of one ride, in the order they
	// were recorded.
	ActivityTrack(ctx context.Context, targetID string, id int64) ([]TrackPoint, error)
	// StoreActivityWeather replaces one ride's weather and records that it was
	// asked about. An empty set of steps records the asking and nothing else.
	StoreActivityWeather(ctx context.Context, targetID string, id int64,
		steps []WeatherStep, readAt time.Time) error
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
	// Asked before the request rather than read off the answer: the coordinates
	// have to be chosen to match the step, and they travel in that same request.
	points := weatherPoints(track, from, to, d.weather.StepFor(from))
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

	return d.recordWeather(ctx, targetID, ride.ID, stepsOf(series, points, from, to))
}

func (d *Deriver) recordWeather(ctx context.Context, targetID string, id int64, steps []WeatherStep) Result {
	if err := d.weatherStore.StoreActivityWeather(ctx, targetID, id, steps, d.now()); err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}

	return Result{Outcome: Polled}
}

// weatherPoints picks where along the ride to ask: one point per step of it,
// the first and the last always among them, so a long ride crossing a front is
// not described by where it happened to start.
func weatherPoints(track []TrackPoint, from, to time.Time, step time.Duration) []TrackPoint {
	if len(track) == 0 {
		return nil
	}
	if step <= 0 {
		step = weatherStepFallback
	}
	steps := int(math.Ceil(float64(to.Sub(from)) / float64(step)))
	wanted := min(max(steps, 1)+1, maximumWeatherPoints, len(track))
	if wanted <= 1 {
		return track[:1]
	}
	points := make([]TrackPoint, 0, wanted)
	last := len(track) - 1
	cursor := 0
	for index := range wanted {
		// Evenly spaced in time, not in record index: a recorder that paused, or
		// that samples unevenly, would otherwise cluster every point in whichever
		// stretch it recorded most densely — and the whole reason to ask at more
		// than one place is to cross the ground the ride crossed.
		//
		// The ends are the first and last samples themselves rather than whatever
		// lies nearest the window's edges, so a ride is always asked about where
		// it started and where it finished.
		switch index {
		case 0:
			points = append(points, track[0])
		case wanted - 1:
			points = append(points, track[last])
		default:
			at := from.Add(time.Duration(float64(to.Sub(from)) * float64(index) / float64(wanted-1)))
			cursor = nearestInTime(track, at, cursor)
			points = append(points, track[cursor])
		}
	}

	return points
}

// nearestInTime is the sample closest to a moment, searched forward from where
// the last one was found: the track is in recorded order and the moments asked
// for only ever move forward, so one pass covers all of them.
func nearestInTime(track []TrackPoint, at time.Time, from int) int {
	nearest := from
	for index := from; index < len(track); index++ {
		if absDuration(track[index].Time.Sub(at)) <= absDuration(track[nearest].Time.Sub(at)) {
			nearest = index

			continue
		}
		// Ordered by time, so once a sample is further away than the one before
		// it, every sample after it is further still.
		break
	}

	return nearest
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}

	return d
}

// stepDistance is how far a moment is from a step the provider answered for.
// The label names the step it opens, so a moment ridden during that step is no
// distance from it, and only one outside is measured to the nearer edge.
func stepDistance(at, start time.Time, step time.Duration) time.Duration {
	switch {
	case at.Before(start):
		return start.Sub(at)
	case at.Sub(start) < step:
		return 0
	default:
		return at.Sub(start.Add(step))
	}
}

// readingAt is one coordinate's answer for one step, as the provider gave it.
func readingAt(one *WeatherSeries, index int, at time.Time, step time.Duration) WeatherStep {
	hour := WeatherStep{
		At:                         at.UTC(),
		Step:                       step,
		TemperatureCelsius:         one.TemperatureCelsius[index],
		ApparentTemperatureCelsius: one.ApparentTemperatureCelsius[index],
		PrecipitationMillimetres:   one.PrecipitationMillimetres[index],
		WindSpeedKMH:               one.WindSpeedKMH[index],
		WindDirectionDegrees:       one.WindDirectionDegrees[index],
		CloudCoverPercent:          one.CloudCoverPercent[index],
		WeatherCode:                one.WeatherCode[index],
	}
	if index < len(one.PrecipitationProbabilityPercent) {
		hour.PrecipitationProbabilityPercent = one.PrecipitationProbabilityPercent[index]
		hour.HasPrecipitationProbability = true
	}

	return hour
}

// stepsOf reduces the provider's per-coordinate series to one row per step of
// the ride. Series i answers for the coordinate weatherPoints chose for step i,
// so each step keeps the reading of the coordinate nearest it in time: a mean
// across the whole route says what the weather did along the ride, not what its
// rider rode through, and erases a headwind that became a tailwind halfway.
func stepsOf(series []WeatherSeries, points []TrackPoint, from, to time.Time) []WeatherStep {
	type candidate struct {
		hour WeatherStep
		// How far the winning coordinate's own moment is from this hour, and the
		// worst code of every coordinate that calls this hour its nearest: a code
		// is not a quantity, and half a ride in rain was ridden in rain.
		distance time.Duration
		worst    int
	}
	byHour := map[int64]*candidate{}
	order := []int64{}
	for seriesIndex := range series {
		one := &series[seriesIndex]
		// A series the sampling has no coordinate for answers as if from the
		// ride's start, which no ride's own hours are further from than its span.
		at := from
		if seriesIndex < len(points) {
			at = points[seriesIndex].Time
		}
		var nearest *candidate
		nearestDistance, nearestCode := time.Duration(0), 0
		step := one.Step
		if step <= 0 {
			step = weatherStepFallback
		}
		for index, hourAt := range one.Time {
			if hourAt.Before(from.Truncate(step)) || hourAt.After(to) {
				continue
			}
			key := hourAt.Unix()
			into, seen := byHour[key]
			if !seen {
				into = &candidate{}
				byHour[key], order = into, append(order, key)
			}
			distance := stepDistance(at, hourAt, step)
			if !seen || distance < into.distance {
				into.hour, into.distance = readingAt(one, index, hourAt, step), distance
			}
			if nearest == nil || distance < nearestDistance {
				nearest, nearestDistance, nearestCode = into, distance, one.WeatherCode[index]
			}
		}
		if nearest != nil {
			nearest.worst = max(nearest.worst, nearestCode)
		}
	}
	// Sorted rather than first seen: a coordinate the provider held no reading
	// for at one step contributes its later steps first, so the order keys were
	// met in is not the order the ride was ridden in.
	slices.Sort(order)
	hours := make([]WeatherStep, 0, len(order))
	for _, key := range order {
		into := byHour[key]
		hour := into.hour
		hour.WeatherCode = max(hour.WeatherCode, into.worst)
		hours = append(hours, hour)
	}

	return hours
}
