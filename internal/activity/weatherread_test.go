package activity_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func weatherNow() time.Time { return time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC) }

// fakeWeatherStore is the rides owed a weather read and what was recorded about
// them.
type fakeWeatherStore struct {
	pendingErr error
	trackErr   error
	storeErr   error
	tracks     map[int64][]activity.TrackPoint
	stored     map[int64][]activity.WeatherStep
	pending    []activity.PendingWeather
	recorded   []int64
	askedLimit int
}

func (s *fakeWeatherStore) ActivitiesAwaitingWeather(
	_ context.Context, _ string, limit int,
) ([]activity.PendingWeather, error) {
	s.askedLimit = limit

	return s.pending, s.pendingErr
}

func (s *fakeWeatherStore) ActivityTrack(_ context.Context, _ string, id int64) ([]activity.TrackPoint, error) {
	return s.tracks[id], s.trackErr
}

func (s *fakeWeatherStore) StoreActivityWeather(
	_ context.Context, _ string, id int64, steps []activity.WeatherStep, _ time.Time,
) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	if s.stored == nil {
		s.stored = map[int64][]activity.WeatherStep{}
	}
	s.stored[id] = steps
	s.recorded = append(s.recorded, id)

	return nil
}

// fakeWeatherSource answers with one step per coordinate asked about, hourly
// unless a test says otherwise.
type fakeWeatherSource struct {
	err        error
	from       time.Time
	to         time.Time
	series     []activity.WeatherSeries
	latitudes  []float64
	step       time.Duration
	calls      int
	omitChance bool
}

// StepFor is the step this source was built to answer with, named as bluntly as
// a provider would: a zero here is a source that says nothing about its step,
// which the caller has to decide for itself.
func (s *fakeWeatherSource) StepFor(time.Time) time.Duration { return s.step }

func (s *fakeWeatherSource) History(
	_ context.Context, latitudes, _ []float64, from, to time.Time,
) ([]activity.WeatherSeries, error) {
	s.calls++
	s.latitudes, s.from, s.to = latitudes, from, to
	if s.err != nil {
		return nil, s.err
	}
	if s.series != nil {
		return s.series, nil
	}
	hour := weatherNow().Truncate(time.Hour)
	series := make([]activity.WeatherSeries, len(latitudes))
	for index := range series {
		series[index] = activity.WeatherSeries{
			Time:                       []time.Time{hour},
			TemperatureCelsius:         []float64{18 + float64(index)},
			ApparentTemperatureCelsius: []float64{17},
			PrecipitationMillimetres:   []float64{0},
			WindSpeedKMH:               []float64{12},
			WindDirectionDegrees:       []float64{240},
			CloudCoverPercent:          []float64{50},
			WeatherCode:                []int{index},
		}
		if !s.omitChance {
			series[index].PrecipitationProbabilityPercent = []float64{10}
		}
	}

	return series, nil
}

func weatherTrack(points int) []activity.TrackPoint {
	track := make([]activity.TrackPoint, points)
	for index := range track {
		track[index] = activity.TrackPoint{
			Time:      weatherNow().Add(time.Duration(index) * time.Minute),
			Latitude:  49 + float64(index)/1000,
			Longitude: 8,
		}
	}

	return track
}

func weatherDeriver(t *testing.T, store *fakeWeatherStore, source *fakeWeatherSource) *activity.Deriver {
	t.Helper()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{}, store, source, weatherNow)
	require.NoError(t, err, "NewDeriver()")

	return deriver
}

func TestDeriveAsksTheWeatherOfEveryRideOwedOne(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{
			{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 3600},
			{ID: 8, StartedAt: weatherNow(), ElapsedSeconds: 3600},
		},
		tracks: map[int64][]activity.TrackPoint{7: weatherTrack(60), 8: weatherTrack(60)},
	}
	source := &fakeWeatherSource{}

	result := weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Polled, result.Outcome)
	assert.Equal(t, []int64{7, 8}, store.recorded, "both recorded as asked")
	assert.Equal(t, 2, source.calls, "one request per ride")
	assert.Len(t, store.stored[7], 1, "the one hour the ride covered")
}

// A backfill of a whole history must not contend with the course forecasts a
// rider is waiting on, so one run asks about a bounded few.
func TestDeriveBoundsHowManyRidesOneRunAsksAbout(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{}

	weatherDeriver(t, store, &fakeWeatherSource{}).Derive(t.Context(), "rider-a")
	assert.Positive(t, store.askedLimit, "a limit was asked for")
	assert.LessOrEqual(t, store.askedLimit, 50, "and it is a small one")
}

// A ride is asked about once, whatever comes back. A provider with nothing to
// say about a place and time must cost one request rather than one per run.
func TestDeriveRecordsARideWithNoTrackAsAsked(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 3600}},
	}
	source := &fakeWeatherSource{}

	result := weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Polled, result.Outcome)
	assert.Equal(t, []int64{7}, store.recorded, "recorded as asked")
	assert.Empty(t, store.stored[7], "with nothing to record about it")
	assert.Zero(t, source.calls, "and no request spent on a ride with no place to ask about")
}

// A provider failure is a run to try again, not an answer: nothing is recorded,
// so the ride is still owed a read.
func TestDeriveDoesNotRecordARideTheProviderRefused(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(60)},
	}

	result := weatherDeriver(t, store, &fakeWeatherSource{err: errors.New("upstream")}).
		Derive(t.Context(), "rider-a")
	assert.Equal(t, activity.Failed, result.Outcome)
	assert.Equal(t, activity.FailureUpstream, result.Failure)
	assert.Empty(t, store.recorded, "the ride is still owed a read")
}

// The ride's own window, and points along the whole of it: a long ride crossing
// a front is not described by where it happened to start.
func TestDeriveAsksAlongTheWholeRideOverItsOwnWindow(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 4 * 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(240)},
	}
	source := &fakeWeatherSource{}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	assert.Len(t, source.latitudes, 5, "one point per hour, both ends included")
	assert.InDelta(t, 49.0, source.latitudes[0], 1e-9, "the first sample")
	assert.InDelta(t, 49.239, source.latitudes[4], 1e-9, "and the last")
	assert.Equal(t, weatherNow(), source.from)
	assert.Equal(t, weatherNow().Add(4*time.Hour), source.to)
}

// However long the ride, the request stays inside a quota: a coarser spacing is
// better than a request nobody can afford.
func TestDeriveBoundsHowManyPointsOneRideIsAskedAt(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 100 * 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(500)},
	}
	source := &fakeWeatherSource{}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	assert.Len(t, source.latitudes, 24, "a day on the bicycle is the ceiling")
}

// Each point is asked about for the step it was chosen for, so each stored hour
// must be what the rider rode through then — not a mean across the whole route,
// which flattens the temperature and outright misleads about the wind.
func TestDeriveGivesEachHourThePlaceTheRiderWas(t *testing.T) {
	t.Parallel()
	hour := weatherNow().Truncate(time.Hour)
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: hour, ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(60)},
	}
	// Two ends 70 km apart, each carrying its own deliberately different series
	// over the two hours the ride touches.
	end := func(temperature, direction float64, codes []int) activity.WeatherSeries {
		return activity.WeatherSeries{
			Time:                            []time.Time{hour, hour.Add(time.Hour)},
			TemperatureCelsius:              []float64{temperature, temperature + 1},
			ApparentTemperatureCelsius:      []float64{temperature - 1, temperature},
			PrecipitationMillimetres:        []float64{0, 0},
			WindSpeedKMH:                    []float64{12, 13},
			WindDirectionDegrees:            []float64{direction, direction},
			CloudCoverPercent:               []float64{50, 55},
			PrecipitationProbabilityPercent: []float64{10, 20},
			WeatherCode:                     codes,
		}
	}
	source := &fakeWeatherSource{series: []activity.WeatherSeries{
		end(18, 350, []int{1, 0}), end(28, 10, []int{0, 2}),
	}}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	require.Len(t, store.stored[7], 2, "the hour it started in and the one it ended in")
	assert.InDelta(t, 18.0, store.stored[7][0].TemperatureCelsius, 1e-9, "where it started")
	assert.InDelta(t, 29.0, store.stored[7][1].TemperatureCelsius, 1e-9, "where it finished")
	// A route-wide mean would report 350 and 10 as one bearing for both hours,
	// and no arithmetic could say which end the rider was at.
	assert.InDelta(t, 350.0, store.stored[7][0].WindDirectionDegrees, 1e-9, "the wind at the start")
	assert.InDelta(t, 10.0, store.stored[7][1].WindDirectionDegrees, 1e-9, "and the one at the finish")
	assert.True(t, store.stored[7][0].HasPrecipitationProbability)
}

// An hour is labelled by the hour it opens, not centred on that label. A
// coordinate ridden thirty minutes into an hour was ridden during it; one
// thirty minutes before it was not, however close its label looks.
func TestDeriveKeepsTheCoordinateRiddenDuringTheHour(t *testing.T) {
	t.Parallel()
	start := time.Date(2026, 8, 24, 11, 45, 0, 0, time.UTC)
	track := make([]activity.TrackPoint, 60)
	for index := range track {
		track[index] = activity.TrackPoint{
			Time:      start.Add(time.Duration(index) * time.Minute),
			Latitude:  49 + float64(index)/1000,
			Longitude: 8,
		}
	}
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: start, ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: track},
	}
	end := func(temperature float64) activity.WeatherSeries {
		return activity.WeatherSeries{
			Time: []time.Time{
				start.Truncate(time.Hour), start.Truncate(time.Hour).Add(time.Hour),
			},
			TemperatureCelsius:         []float64{temperature, temperature + 1},
			ApparentTemperatureCelsius: []float64{temperature - 1, temperature},
			PrecipitationMillimetres:   []float64{0, 0},
			WindSpeedKMH:               []float64{12, 13},
			WindDirectionDegrees:       []float64{240, 250},
			CloudCoverPercent:          []float64{50, 55},
			WeatherCode:                []int{0, 0},
		}
	}
	source := &fakeWeatherSource{series: []activity.WeatherSeries{end(18), end(28)}}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	require.Len(t, store.stored[7], 2)
	assert.InDelta(t, 18.0, store.stored[7][0].TemperatureCelsius, 1e-9,
		"11:45 was ridden during the 11:00 hour")
	// Measured to the label alone, 11:45 would look fifteen minutes from 12:00
	// and beat the coordinate at 12:44 that was actually out in that hour.
	assert.InDelta(t, 29.0, store.stored[7][1].TemperatureCelsius, 1e-9,
		"and 12:44 during the 12:00 one")
}

// A ride recent enough for the forecast endpoint is answered by the quarter
// hour, and is sampled once per step so each of those still names where the
// rider was. Series i answers for point i, so the temperatures come back in the
// order the ride passed through them.
func TestDeriveRecordsARecentRideByTheQuarterHour(t *testing.T) {
	t.Parallel()
	hour := weatherNow().Truncate(time.Hour)
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: hour, ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(60)},
	}
	quarters := []time.Time{
		hour, hour.Add(15 * time.Minute), hour.Add(30 * time.Minute),
		hour.Add(45 * time.Minute), hour.Add(time.Hour),
	}
	at := func(temperature float64) activity.WeatherSeries {
		one := activity.WeatherSeries{Step: 15 * time.Minute, Time: quarters}
		for range quarters {
			one.TemperatureCelsius = append(one.TemperatureCelsius, temperature)
			one.ApparentTemperatureCelsius = append(one.ApparentTemperatureCelsius, temperature-1)
			one.PrecipitationMillimetres = append(one.PrecipitationMillimetres, 0)
			one.PrecipitationProbabilityPercent = append(one.PrecipitationProbabilityPercent, 10)
			one.WindSpeedKMH = append(one.WindSpeedKMH, 12)
			one.WindDirectionDegrees = append(one.WindDirectionDegrees, 240)
			one.CloudCoverPercent = append(one.CloudCoverPercent, 50)
			one.WeatherCode = append(one.WeatherCode, 1)
		}

		return one
	}
	source := &fakeWeatherSource{step: 15 * time.Minute, series: []activity.WeatherSeries{
		at(10), at(11), at(12), at(13), at(14),
	}}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	assert.Len(t, source.latitudes, 5, "one coordinate per quarter hour, both ends included")
	require.Len(t, store.stored[7], 5, "four rows an hour, and the one it ended on")
	for index, step := range store.stored[7] {
		assert.InDelta(t, float64(10+index), step.TemperatureCelsius, 1e-9,
			"the coordinate the rider was at that quarter")
		assert.Equal(t, 15*time.Minute, step.Step, "the step is stored, not inferred")
	}
}

// Everything older is answered by the reanalysis, which is hourly and only
// hourly. Such a ride keeps the hour it was given however it ages.
func TestDeriveRecordsAnOlderRideByTheHour(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(60)},
	}
	// An actually-hourly provider, not one that names no step and is read as
	// hourly by the fallback — that is TestDeriveReadsASourceWithNoStepAsHourly.
	hour := weatherNow().Truncate(time.Hour)
	source := &fakeWeatherSource{step: time.Hour, series: []activity.WeatherSeries{{
		Step: time.Hour, Time: []time.Time{hour},
		TemperatureCelsius: []float64{18}, ApparentTemperatureCelsius: []float64{17},
		PrecipitationMillimetres: []float64{0}, WindSpeedKMH: []float64{12},
		WindDirectionDegrees: []float64{240}, CloudCoverPercent: []float64{50},
		WeatherCode: []int{1},
	}}}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	assert.Len(t, source.latitudes, 2, "one coordinate per hour, both ends included")
	require.Len(t, store.stored[7], 1)
	assert.Equal(t, time.Hour, store.stored[7][0].Step)
	assert.InDelta(t, 18.0, store.stored[7][0].TemperatureCelsius, 1e-9)
}

// A finer step must not multiply the coordinates a long ride is asked at: the
// cap is the same one, and a ride past it is sampled more coarsely in space
// than in time.
func TestDeriveBoundsTheCoordinatesOfALongQuarterHourlyRide(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 24 * 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(500)},
	}
	source := &fakeWeatherSource{step: 15 * time.Minute}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	assert.Len(t, source.latitudes, 24, "the same ceiling, whatever the step")
}

// A source that names no step at all is read as hourly rather than dividing the
// ride by nothing. Only a hand-built one reaches this; both real endpoints say
// what they answered.
func TestDeriveReadsASourceWithNoStepAsHourly(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(60)},
	}
	// Names no step, either before the request or on the answer.
	source := &fakeWeatherSource{}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	assert.Zero(t, source.StepFor(weatherNow()), "the source really does name none")
	assert.Len(t, source.latitudes, 2, "sampled as if hourly")
	require.Len(t, store.stored[7], 1)
	assert.Equal(t, time.Hour, store.stored[7][0].Step, "and stored as an hour")
}

// The provider drops a step it held no reading for, per coordinate. A ride
// whose first coordinate has such a hole meets its later steps first, and the
// order the steps were met in is not the order the ride was ridden in.
func TestDeriveStoresTheStepsInTheOrderTheRideWasRidden(t *testing.T) {
	t.Parallel()
	hour := weatherNow().Truncate(time.Hour)
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: hour, ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(60)},
	}
	one := func(times []time.Time, temperature float64) activity.WeatherSeries {
		series := activity.WeatherSeries{Step: time.Hour, Time: times}
		for range times {
			series.TemperatureCelsius = append(series.TemperatureCelsius, temperature)
			series.ApparentTemperatureCelsius = append(series.ApparentTemperatureCelsius, temperature-1)
			series.PrecipitationMillimetres = append(series.PrecipitationMillimetres, 0)
			series.WindSpeedKMH = append(series.WindSpeedKMH, 12)
			series.WindDirectionDegrees = append(series.WindDirectionDegrees, 240)
			series.CloudCoverPercent = append(series.CloudCoverPercent, 50)
			series.WeatherCode = append(series.WeatherCode, 1)
		}

		return series
	}
	// The first coordinate has no reading for the hour the ride began in.
	source := &fakeWeatherSource{step: time.Hour, series: []activity.WeatherSeries{
		one([]time.Time{hour.Add(time.Hour)}, 19),
		one([]time.Time{hour, hour.Add(time.Hour)}, 28),
	}}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	require.Len(t, store.stored[7], 2)
	assert.Equal(t, hour, store.stored[7][0].At, "the hour it started in comes first")
	assert.Equal(t, hour.Add(time.Hour), store.stored[7][1].At, "and the one it ended in second")
}

// A ride asked about at one coordinate has one answer per hour, and reports it
// unchanged.
func TestDeriveKeepsASinglePointRideAsItStands(t *testing.T) {
	t.Parallel()
	hour := weatherNow().Truncate(time.Hour)
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: hour, ElapsedSeconds: 0}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(1)},
	}
	source := &fakeWeatherSource{}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	require.Len(t, source.latitudes, 1, "one coordinate is all there is")
	require.Len(t, store.stored[7], 1)
	assert.InDelta(t, 18.0, store.stored[7][0].TemperatureCelsius, 1e-9, "the one reading, as it stands")
}

// Where a whole ride falls inside one hour, both its ends are asked about for
// that one step. The nearest in time wins the reading, but a code is not a
// quantity to be picked: rain at either end was rain the ride was ridden in.
func TestDeriveKeepsTheWorstCodeOfAStepAskedAtSeveralPoints(t *testing.T) {
	t.Parallel()
	hour := weatherNow().Truncate(time.Hour)
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: hour, ElapsedSeconds: 1800}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(30)},
	}
	source := &fakeWeatherSource{}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	require.Len(t, store.stored[7], 1)
	assert.InDelta(t, 18.0, store.stored[7][0].TemperatureCelsius, 1e-9, "the nearer end's reading")
	assert.Equal(t, 1, store.stored[7][0].WeatherCode, "the worst of the step's own points")
}

// The reanalysis that answers for an older ride carries no probability of
// precipitation, and an absent series must not become a column of zeroes.
func TestDeriveKeepsAnAbsentProbabilityAbsent(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(60)},
	}

	weatherDeriver(t, store, &fakeWeatherSource{omitChance: true}).Derive(t.Context(), "rider-a")
	require.Len(t, store.stored[7], 1)
	assert.False(t, store.stored[7][0].HasPrecipitationProbability)
	assert.Zero(t, store.stored[7][0].PrecipitationProbabilityPercent)
}

// A build wired without a weather source still derives the training numbers.
func TestDeriveWithoutAWeatherSourceStillDerivesTheRest(t *testing.T) {
	t.Parallel()
	deriver, err := activity.NewDeriver(&fakeDeriveStore{owner: "rider-a"}, nil, nil, weatherNow)
	require.NoError(t, err, "NewDeriver()")

	assert.Equal(t, activity.NotReady, deriver.Derive(t.Context(), "rider-a").Outcome)
}

// A ride short enough to fit in one hour is asked about at one point.
func TestDeriveAsksAShortRideAtOnePoint(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 0}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(3)},
	}
	source := &fakeWeatherSource{}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	assert.Len(t, source.latitudes, 2, "the ends of a ride that lasted no time")
}

// An hour outside the ride's own window is not the ride's weather, however
// much of the day the provider chose to answer with — which the archive, asked
// by date, always does.
func TestDeriveKeepsOnlyTheHoursTheRideCovered(t *testing.T) {
	t.Parallel()
	hour := weatherNow().Truncate(time.Hour)
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: hour, ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(60)},
	}
	source := &fakeWeatherSource{series: []activity.WeatherSeries{{
		Time: []time.Time{
			hour.Add(-3 * time.Hour), hour, hour.Add(time.Hour), hour.Add(9 * time.Hour),
		},
		TemperatureCelsius:         []float64{2, 18, 19, 25},
		ApparentTemperatureCelsius: []float64{1, 17, 18, 24},
		PrecipitationMillimetres:   []float64{0, 0, 0, 0},
		WindSpeedKMH:               []float64{5, 12, 13, 20},
		WindDirectionDegrees:       []float64{10, 240, 245, 300},
		CloudCoverPercent:          []float64{10, 50, 55, 5},
		WeatherCode:                []int{0, 1, 2, 0},
	}}}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	require.Len(t, store.stored[7], 2, "the hour it started in and the one it ended in")
	assert.InDelta(t, 18.0, store.stored[7][0].TemperatureCelsius, 1e-9)
	assert.InDelta(t, 19.0, store.stored[7][1].TemperatureCelsius, 1e-9)
}

// The whole reason to ask at more than one place is to cross the ground the
// ride crossed. A recorder that sampled densely for the first few minutes and
// sparsely after would, spaced by record index, put every point in that first
// stretch and describe a four-hour ride by its first ten minutes.
func TestDeriveSpacesTheAskedPointsByTimeNotByRecordIndex(t *testing.T) {
	t.Parallel()
	// Four hundred samples in the first ten minutes, then one a minute for the
	// rest of a four-hour ride.
	track := make([]activity.TrackPoint, 0, 630)
	for index := range 400 {
		track = append(track, activity.TrackPoint{
			Time:      weatherNow().Add(time.Duration(index) * 1500 * time.Millisecond),
			Latitude:  49 + float64(index)/100000,
			Longitude: 8,
		})
	}
	for index := range 230 {
		track = append(track, activity.TrackPoint{
			Time:      weatherNow().Add(10*time.Minute + time.Duration(index)*time.Minute),
			Latitude:  49.004 + float64(index)/1000,
			Longitude: 8,
		})
	}
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 4 * 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: track},
	}
	source := &fakeWeatherSource{}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	require.Len(t, source.latitudes, 5, "one point per hour, both ends included")
	// Spaced by index, the middle three would all sit inside the dense opening
	// stretch, within a thousandth of a degree of the start.
	assert.Greater(t, source.latitudes[1], 49.05, "an hour in, not ten minutes in")
	assert.Greater(t, source.latitudes[2], 49.1, "and two hours in")
	assert.InDelta(t, 49.233, source.latitudes[4], 0.001, "the last sample either way")
}

func TestDeriveReportsAWeatherStoreItCannotRead(t *testing.T) {
	t.Parallel()
	for name, store := range map[string]*fakeWeatherStore{
		"the rides cannot be listed": {pendingErr: errors.New("unreadable")},
		"the track cannot be read": {
			pending:  []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 3600}},
			trackErr: errors.New("unreadable"),
		},
		"the weather cannot be written": {
			pending:  []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 3600}},
			tracks:   map[int64][]activity.TrackPoint{7: weatherTrack(60)},
			storeErr: errors.New("unwritable"),
		},
	} {
		t.Run(name, func(t *testing.T) {
			result := weatherDeriver(t, store, &fakeWeatherSource{}).Derive(t.Context(), "rider-a")
			assert.Equal(t, activity.Failed, result.Outcome)
			assert.Equal(t, activity.FailureState, result.Failure)
		})
	}
}
