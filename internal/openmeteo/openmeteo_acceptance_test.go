//go:build openmeteo_acceptance

// This file is not compiled into the normal suite. It contacts Open-Meteo, and
// it exists because the alternative did not catch a real fault: a fixture that
// asserts the parameters this client sends agrees with whatever the client
// sends, and Open-Meteo refused the combination for months of stored rides.
//
// Invoke it on its own:
//
//	go test -tags openmeteo_acceptance ./internal/openmeteo/ -run Acceptance -v
//
// It needs no credentials. The free endpoints this exercises take none.
//
// What it does not check is the handling of a null hour. The provider returns
// none at the ages asked for below, so an assertion about them here passes
// whether the handling is right or wrong — verified by putting the old
// behaviour back and watching this file still pass. The deterministic cases in
// client_test.go are what guard that, and they fail without it.
package openmeteo_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/openmeteo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pastWindowStart is where to begin a two-hour window that has already
// happened, the given number of days back.
//
// A day back or more it is mid-morning on that date, which no two-hour window
// can carry across midnight into the neighbouring date and so into the other
// endpoint. Today it is simply the last two whole hours, since mid-morning has
// not necessarily happened yet: run before two in the morning that window sits
// on yesterday's date, which the forecast endpoint answers for exactly as it
// answers for today's.
func pastWindowStart(today time.Time, daysAgo int) time.Time {
	if daysAgo == 0 {
		return today.Truncate(time.Hour).Add(-2 * time.Hour)
	}
	date := today.AddDate(0, 0, -daysAgo)

	return time.Date(date.Year(), date.Month(), date.Day(), 10, 0, 0, 0, date.Location())
}

// TestOpenMeteoHistoryAcceptance asks the real provider for a ride's weather at
// each age the split has to cover, and checks the answer is weather rather than
// an error or a column of nulls.
func TestOpenMeteoHistoryAcceptance(t *testing.T) {
	client, err := openmeteo.New(&openmeteo.Options{
		Timezone: func() string { return "Europe/Berlin" },
	})
	require.NoError(t, err, "New()")

	// The client asks in this zone and splits the endpoints on the ride's date in
	// it, so the windows below are counted in it too. Counted in the runner's own
	// zone instead, a ride two days back could fall on a different Berlin date
	// and be asked of the other endpoint, which is a test that fails by
	// geography rather than by fault.
	berlin, err := time.LoadLocation("Europe/Berlin")
	require.NoError(t, err, "LoadLocation()")
	today := time.Now().In(berlin)

	at := []openmeteo.Coordinate{{Latitude: 49.0, Longitude: 8.4}, {Latitude: 49.2, Longitude: 8.6}}
	// wantStep is what each window should come back at: the forecast endpoint
	// answers the recent past every quarter hour, the reanalysis only ever by
	// the hour. A fixture cannot say which the provider will actually honour.
	for name, test := range map[string]struct {
		wantStep        time.Duration
		daysAgo         int
		wantProbability bool
	}{
		"today":                     {daysAgo: 0, wantProbability: true, wantStep: 15 * time.Minute},
		"yesterday":                 {daysAgo: 1, wantProbability: true, wantStep: 15 * time.Minute},
		"the day the archive takes": {daysAgo: 2, wantStep: time.Hour},
		"a month back":              {daysAgo: 30, wantStep: time.Hour},
		// Past where the forecast endpoint stops holding values, which is the
		// boundary this split exists to stay clear of.
		"four months back": {daysAgo: 120, wantStep: time.Hour},
		"last year":        {daysAgo: 400, wantStep: time.Hour},
	} {
		t.Run(name, func(t *testing.T) {
			from := pastWindowStart(today, test.daysAgo)
			hourlies, historyErr := client.History(t.Context(), at, from, from.Add(2*time.Hour))
			require.NoError(t, historyErr, "History()")
			require.Len(t, hourlies, len(at), "one series per coordinate")

			for index, one := range hourlies {
				require.NotEmpty(t, one.Time, "coordinate %d: the provider held steps", index)
				require.Len(t, one.TemperatureCelsius, len(one.Time), "aligned with its timestamps")
				assert.Equal(t, test.wantStep, one.Step, "coordinate %d: the step asked for", index)
			}

			// Two hours at the quarter hour is nine steps, at the hour three: the
			// finer request is worth nothing if the provider answers it coarsely.
			if test.wantStep < time.Hour {
				assert.Len(t, hourlies[0].Time, 9, "a two-hour window, every quarter of an hour")
			}

			probability := hourlies[0].PrecipitationProbabilityPercent
			if test.wantProbability {
				assert.Len(t, probability, len(hourlies[0].Time),
					"the forecast endpoint carries a chance of rain")
			} else {
				assert.Empty(t, probability, "the reanalysis carries none, and none is invented")
			}
		})
	}
}
