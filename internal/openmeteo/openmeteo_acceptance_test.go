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
package openmeteo_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/openmeteo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOpenMeteoHistoryAcceptance asks the real provider for a ride's weather at
// each age the split has to cover, and checks the answer is weather rather than
// an error or a column of nulls.
func TestOpenMeteoHistoryAcceptance(t *testing.T) {
	client, err := openmeteo.New(&openmeteo.Options{
		Timezone: func() string { return "Europe/Berlin" },
	})
	require.NoError(t, err, "New()")

	at := []openmeteo.Coordinate{{Latitude: 49.0, Longitude: 8.4}, {Latitude: 49.2, Longitude: 8.6}}
	for name, test := range map[string]struct {
		daysAgo         int
		wantProbability bool
	}{
		"today":                     {daysAgo: 0, wantProbability: true},
		"yesterday":                 {daysAgo: 1, wantProbability: true},
		"the day the archive takes": {daysAgo: 2},
		"a month back":              {daysAgo: 30},
		// Past where the forecast endpoint stops holding values, which is the
		// boundary this split exists to stay clear of.
		"four months back": {daysAgo: 120},
		"last year":        {daysAgo: 400},
	} {
		t.Run(name, func(t *testing.T) {
			from := time.Now().AddDate(0, 0, -test.daysAgo).Truncate(time.Hour)
			hourlies, historyErr := client.History(t.Context(), at, from, from.Add(2*time.Hour))
			require.NoError(t, historyErr, "History()")
			require.Len(t, hourlies, len(at), "one series per coordinate")

			for index, one := range hourlies {
				require.NotEmpty(t, one.Time, "coordinate %d: the provider held hours", index)
				require.Len(t, one.TemperatureCelsius, len(one.Time), "aligned with its timestamps")
				// Nulls are dropped rather than read as zero, so every value
				// served is one the provider actually recorded. A whole window at
				// exactly nought degrees is what the old reading of a null looked
				// like, and is not weather anywhere this service rides.
				assert.NotEqual(t, make([]float64, len(one.Time)), one.TemperatureCelsius,
					"coordinate %d: a column of zeroes is a null read as a number", index)
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
