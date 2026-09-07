package sqlite

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func weatherHour(at time.Time, temperature float64, chance bool) activity.WeatherHour {
	return activity.WeatherHour{
		Hour:                            at,
		TemperatureCelsius:              temperature,
		ApparentTemperatureCelsius:      temperature - 1,
		PrecipitationMillimetres:        0.4,
		PrecipitationProbabilityPercent: 30,
		WindSpeedKMH:                    12,
		WindDirectionDegrees:            240,
		CloudCoverPercent:               55,
		WeatherCode:                     61,
		HasPrecipitationProbability:     chance,
	}
}

// A ride is asked about once and the answer kept whole.
func TestActivityWeatherRoundTrips(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	hours := []activity.WeatherHour{
		weatherHour(activityNow(), 18, true),
		weatherHour(activityNow().Add(time.Hour), 20, true),
	}

	require.NoError(t, store.StoreActivityWeather(t.Context(), "rider-a", 1, hours, activityNow()),
		"StoreActivityWeather()")

	read, err := store.ActivityWeatherHours(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityWeatherHours()")
	require.Len(t, read, 2)
	assert.Equal(t, hours[0], read[0])
	assert.Equal(t, hours[1], read[1])

	byRide, err := store.ActivityWeather(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityWeather()")
	assert.Equal(t, hours, byRide[1])
}

// The reanalysis that answers for an older ride carries no probability of
// precipitation, and an absent value must come back absent rather than zero.
func TestActivityWeatherKeepsAnAbsentProbabilityAbsent(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	require.NoError(t, store.StoreActivityWeather(t.Context(), "rider-a", 1,
		[]activity.WeatherHour{weatherHour(activityNow(), 18, false)}, activityNow()),
		"StoreActivityWeather()")

	read, err := store.ActivityWeatherHours(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityWeatherHours()")
	require.Len(t, read, 1)
	assert.False(t, read[0].HasPrecipitationProbability)
	assert.Zero(t, read[0].PrecipitationProbabilityPercent)
}

// The acceptance criterion: a second run requests nothing for a ride that
// already has rows, and nothing for one whose read found nothing either.
func TestActivitiesAwaitingWeatherSkipsWhatWasAlreadyAsked(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1, 2, 3)
	for _, id := range []int64{1, 2, 3} {
		require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", id, activity.FIT{
			Records: []activity.Record{{Time: activityNow()}},
		}), "StoreActivityRecords()")
	}
	require.NoError(t, store.StoreActivityWeather(t.Context(), "rider-a", 1,
		[]activity.WeatherHour{weatherHour(activityNow(), 18, true)}, activityNow()),
		"StoreActivityWeather() with rows")
	// A read that found nothing still records the asking, which is what keeps a
	// ride the provider has nothing for from being asked again on every run.
	require.NoError(t, store.StoreActivityWeather(t.Context(), "rider-a", 2, nil, activityNow()),
		"StoreActivityWeather() with nothing")

	pending, err := store.ActivitiesAwaitingWeather(t.Context(), "rider-a", 10)
	require.NoError(t, err, "ActivitiesAwaitingWeather()")
	require.Len(t, pending, 1, "only the ride nobody has asked about")
	assert.Equal(t, int64(3), pending[0].ID)
	assert.Equal(t, activityNow(), pending[0].StartedAt)
	assert.InDelta(t, 3900.0, pending[0].ElapsedSeconds, 1e-9, "the window to ask about")
}

// A ride still waiting for its samples has no track to ask about a place along.
func TestActivitiesAwaitingWeatherSkipsARideWithNoStoredRecords(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)

	pending, err := store.ActivitiesAwaitingWeather(t.Context(), "rider-a", 10)
	require.NoError(t, err, "ActivitiesAwaitingWeather()")
	assert.Empty(t, pending)
}

func TestActivitiesAwaitingWeatherHonoursTheLimit(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1, 2, 3)
	for _, id := range []int64{1, 2, 3} {
		require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", id, activity.FIT{
			Records: []activity.Record{{Time: activityNow()}},
		}), "StoreActivityRecords()")
	}

	pending, err := store.ActivitiesAwaitingWeather(t.Context(), "rider-a", 2)
	require.NoError(t, err, "ActivitiesAwaitingWeather()")
	assert.Len(t, pending, 2)
}

// A second read replaces the hours whole rather than adding to them.
func TestStoreActivityWeatherReplacesWhatWasThere(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.StoreActivityWeather(t.Context(), "rider-a", 1,
		[]activity.WeatherHour{
			weatherHour(activityNow(), 18, true),
			weatherHour(activityNow().Add(time.Hour), 20, true),
		}, activityNow()), "StoreActivityWeather()")

	require.NoError(t, store.StoreActivityWeather(t.Context(), "rider-a", 1,
		[]activity.WeatherHour{weatherHour(activityNow(), 5, true)}, activityNow()),
		"StoreActivityWeather() again")

	read, err := store.ActivityWeatherHours(t.Context(), "rider-a", 1)
	require.NoError(t, err, "ActivityWeatherHours()")
	require.Len(t, read, 1)
	assert.InDelta(t, 5.0, read[0].TemperatureCelsius, 1e-9)
}

func TestActivityWeatherReportsAnUnreadableStore(t *testing.T) {
	t.Parallel()
	store := metricsStore(t, 1)
	require.NoError(t, store.Close(), "Close()")

	_, err := store.ActivityWeather(t.Context(), "rider-a")
	require.ErrorContains(t, err, "reading the activity weather")
	_, err = store.ActivityWeatherHours(t.Context(), "rider-a", 1)
	require.ErrorContains(t, err, "reading the activity weather")
	_, err = store.ActivitiesAwaitingWeather(t.Context(), "rider-a", 10)
	require.ErrorContains(t, err, "listing activities awaiting weather")
	require.ErrorContains(t, store.StoreActivityWeather(t.Context(), "rider-a", 1, nil, activityNow()),
		"starting the activity weather write")
}
