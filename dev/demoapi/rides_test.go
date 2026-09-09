package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/sqlite"
)

func demoStore(t *testing.T) *sqlite.Store {
	t.Helper()

	var key [32]byte
	for index := range key {
		key[index] = byte(index)
	}
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"), key)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })

	return store
}

// Every per-rider view resolves its target from the signed-in subject, so a
// demo whose slots are owned by nobody who can sign in shows an empty activity
// history however much is seeded.
func TestTheDemoRiderOwnsTheFirstSlot(t *testing.T) {
	t.Parallel()

	slots, err := slotsFor([]string{demoSubject, "rider-b"}, "current,unauthorized")
	require.NoError(t, err)
	store := demoStore(t)
	require.NoError(t, seed(t.Context(), store, slots))

	owners := map[string]string{}
	require.NoError(t, store.ForEachTarget(t.Context(), func(id, _, ownerSubject string) error {
		owners[id] = ownerSubject

		return nil
	}))
	assert.Equal(t, demoSubject, owners[demoSubject], "the signed-in demo rider owns a target of their own")
}

// The demo is meant to show the whole activity surface, and most of it is
// derived rather than seeded: without a derivation pass the ride page has a
// track and nothing else on it.
func TestSeedLeavesTheDemoRiderARideWithEverythingDerived(t *testing.T) {
	t.Parallel()

	slots, err := slotsFor([]string{demoSubject}, "current")
	require.NoError(t, err)
	store := demoStore(t)
	require.NoError(t, seed(t.Context(), store, slots))

	rides, err := store.ActivitiesBetween(
		t.Context(), demoSubject, time.Now().UTC().AddDate(0, 0, -30), time.Now().UTC(), 50,
	)
	require.NoError(t, err)
	require.NotEmpty(t, rides, "the demo rider has recorded rides")

	metrics, err := store.ActivityMetrics(t.Context(), demoSubject)
	require.NoError(t, err)
	assert.Len(t, metrics, len(rides), "every ride carries the numbers the load panel reads")

	summaries, err := store.ActivityWeatherSummaries(t.Context(), demoSubject)
	require.NoError(t, err)
	assert.NotEmpty(t, summaries, "and the weather strip has something to draw")

	steps, err := store.ActivityWeatherSteps(t.Context(), demoSubject, rides[0].ID)
	require.NoError(t, err)
	assert.NotEmpty(t, steps, "a positioned ride is asked about along its own track")
}

// Seeding runs again on every manual synchronisation, so it has to be safe to
// repeat over a database that already holds the last one's rides.
func TestSeedIsRepeatable(t *testing.T) {
	t.Parallel()

	slots, err := slotsFor([]string{demoSubject}, "current")
	require.NoError(t, err)
	store := demoStore(t)
	require.NoError(t, seed(t.Context(), store, slots))
	require.NoError(t, seed(t.Context(), store, slots))

	metrics, err := store.ActivityMetrics(t.Context(), demoSubject)
	require.NoError(t, err)
	assert.Len(t, metrics, 5, "a second seeding replaces the rides rather than doubling them")
}

// The derivation asks its provider for a window and reads back a step; both
// sides of that have to line up with what syntheticWeather actually returns,
// or a ride records no weather at all.
func TestRideWeatherAnswersAWholeRideWindowHourly(t *testing.T) {
	t.Parallel()

	source := rideWeather()
	from := time.Date(2026, time.August, 24, 6, 20, 0, 0, time.UTC)
	to := from.Add(3*time.Hour + 10*time.Minute)

	assert.Equal(t, time.Hour, source.StepFor(from), "an hour is the only step the synthetic series has")

	series, err := source.History(t.Context(), []float64{48.40, 48.55}, []float64{8.10, 8.35}, from, to)
	require.NoError(t, err)
	require.Len(t, series, 2, "one series per coordinate, in the order they were asked about")
	for index := range series {
		one := &series[index]
		assert.Equal(t, time.Hour, one.Step, "the step travels with the answer")
		require.NotEmpty(t, one.Time)
		assert.False(t, one.Time[0].After(from), "series %d misses the start of the ride", index)
		assert.False(t, one.Time[len(one.Time)-1].Before(to), "series %d misses the end of it", index)
		assert.Len(t, one.TemperatureCelsius, len(one.Time), "series %d column length mismatch", index)
		assert.Len(t, one.WindDirectionDegrees, len(one.Time), "series %d column length mismatch", index)
		assert.Len(t, one.WeatherCode, len(one.Time), "series %d column length mismatch", index)
	}
}
