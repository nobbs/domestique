package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummarizeReportsCoveragePerPackage(t *testing.T) {
	t.Parallel()

	profile := `mode: set
example.com/svc/internal/route/stage.go:10.2,12.3 2 1
example.com/svc/internal/route/stage.go:14.2,15.3 1 0
example.com/svc/internal/route/name.go:4.2,6.3 1 1
example.com/svc/cmd/svc/main.go:20.2,24.3 4 0
`

	var out strings.Builder
	require.NoError(t, summarize(strings.NewReader(profile), &out))

	assert.Equal(t, strings.Join([]string{
		"package                         coverage  statements",
		"example.com/svc/cmd/svc             0.0%         0/4",
		"example.com/svc/internal/route     75.0%         3/4",
		"total                              37.5%         3/8",
		"",
	}, "\n"), out.String())
}

// Every test binary that instruments a package reports every one of its blocks,
// so the merged profile holds one line per binary for the same position. Adding
// them up would count the same statements several times and would report a
// block as uncovered because the binary that did not reach it was listed last.
func TestSummarizeCountsARepeatedBlockOnce(t *testing.T) {
	t.Parallel()

	profile := `mode: set
example.com/svc/internal/route/stage.go:10.2,12.3 2 0
example.com/svc/internal/route/stage.go:10.2,12.3 2 7
example.com/svc/internal/route/stage.go:10.2,12.3 2 0
`

	var out strings.Builder
	require.NoError(t, summarize(strings.NewReader(profile), &out))

	assert.Contains(t, out.String(), "total                             100.0%         2/2")
}

func TestSummarizeReportsAPackageWithoutStatements(t *testing.T) {
	t.Parallel()

	profile := `mode: set
example.com/svc/internal/empty/doc.go:1.1,1.1 0 0
`

	var out strings.Builder
	require.NoError(t, summarize(strings.NewReader(profile), &out))

	assert.Contains(t, out.String(), "n/a")
}

func TestSummarizeRejectsAMalformedProfile(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		profile string
		message string
	}{
		"missing field": {
			profile: "mode: set\nexample.com/svc/a.go:1.1,2.2 3\n",
			message: "expected 3 fields, got 2",
		},
		"statement count is not a number": {
			profile: "mode: set\nexample.com/svc/a.go:1.1,2.2 many 1\n",
			message: "statement count",
		},
		"execution count is not a number": {
			profile: "mode: set\nexample.com/svc/a.go:1.1,2.2 3 often\n",
			message: "execution count",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			var out strings.Builder
			err := summarize(strings.NewReader(test.profile), &out)

			require.ErrorContains(t, err, test.message)
			assert.Contains(t, err.Error(), "line 2")
		})
	}
}

func TestPackageOfTakesTheImportPathFromAPosition(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "example.com/svc/internal/route",
		packageOf("example.com/svc/internal/route/stage.go:10.2,12.3"))
}
