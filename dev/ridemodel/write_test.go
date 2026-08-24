package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// samples.csv is dev/fitter's own read contract (see dev/fitter/corpus.go),
// so its header and a heart-rate value round-tripping through it are worth
// a test on their own — a column rename or reorder here would otherwise
// only surface as a confusing failure on the other side of that contract.
func TestWriteSamplesIncludesHeartRateColumnsAndRoundTripsAValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "samples.csv")
	samples := []sample{
		{
			RideID: "r1", Time: time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC),
			HeartRateBPM: 142, HasHeartRate: true,
		},
	}

	require.NoError(t, writeSamples(path, samples))

	file, err := os.Open(path) //nolint:gosec // fixture path under the test's own temp directory
	require.NoError(t, err)
	defer closeFile(file)

	records, err := csv.NewReader(file).ReadAll()
	require.NoError(t, err)
	require.Len(t, records, 2)

	header := records[0]
	assert.Contains(t, header, "heart_rate_bpm")
	assert.Contains(t, header, "has_heart_rate")

	columns := indexByColumnName(t, header)
	row := records[1]
	assert.Equal(t, "142", row[columns["heart_rate_bpm"]])
	assert.Equal(t, "true", row[columns["has_heart_rate"]])
}

func indexByColumnName(t *testing.T, header []string) map[string]int {
	t.Helper()
	columns := make(map[string]int, len(header))
	for i, name := range header {
		columns[name] = i
	}

	return columns
}
