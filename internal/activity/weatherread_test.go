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
	stored     map[int64][]activity.WeatherHour
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
	_ context.Context, _ string, id int64, hours []activity.WeatherHour, _ time.Time,
) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	if s.stored == nil {
		s.stored = map[int64][]activity.WeatherHour{}
	}
	s.stored[id] = hours
	s.recorded = append(s.recorded, id)

	return nil
}

// fakeWeatherSource answers with one hour per coordinate asked about.
type fakeWeatherSource struct {
	err        error
	from       time.Time
	to         time.Time
	series     []activity.WeatherSeries
	latitudes  []float64
	calls      int
	omitChance bool
}

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

// The hour is one thing across the coordinates it was asked at, because the
// rider was somewhere along the ride rather than at all of it at once.
func TestDeriveAveragesTheCoordinatesOfOneHour(t *testing.T) {
	t.Parallel()
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: weatherNow(), ElapsedSeconds: 3600}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(60)},
	}
	source := &fakeWeatherSource{}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	require.Len(t, store.stored[7], 1)
	hour := store.stored[7][0]
	assert.InDelta(t, 18.5, hour.TemperatureCelsius, 1e-9, "18 at one end and 19 at the other")
	// A code is not a quantity: half a ride in rain was ridden in rain.
	assert.Equal(t, 1, hour.WeatherCode, "the worst of them, not their mean")
	assert.True(t, hour.HasPrecipitationProbability)
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

// A bearing wraps. Averaged as plain numbers, a north wind read at 350 degrees
// at one end of the ride and 10 at the other comes out as 180 — a south wind,
// the exact opposite of the one that blew.
func TestDeriveAveragesWindDirectionAroundTheWrap(t *testing.T) {
	t.Parallel()
	hour := weatherNow().Truncate(time.Hour)
	store := &fakeWeatherStore{
		pending: []activity.PendingWeather{{ID: 7, StartedAt: hour, ElapsedSeconds: 1800}},
		tracks:  map[int64][]activity.TrackPoint{7: weatherTrack(30)},
	}
	one := func(direction float64) activity.WeatherSeries {
		return activity.WeatherSeries{
			Time: []time.Time{hour}, TemperatureCelsius: []float64{18},
			ApparentTemperatureCelsius: []float64{17}, PrecipitationMillimetres: []float64{0},
			WindSpeedKMH: []float64{12}, WindDirectionDegrees: []float64{direction},
			CloudCoverPercent: []float64{50}, WeatherCode: []int{1},
		}
	}
	source := &fakeWeatherSource{series: []activity.WeatherSeries{one(350), one(10)}}

	weatherDeriver(t, store, source).Derive(t.Context(), "rider-a")
	require.Len(t, store.stored[7], 1)
	assert.InDelta(t, 0.0, store.stored[7][0].WindDirectionDegrees, 0.001,
		"due north, not the south the arithmetic mean would invent")
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
