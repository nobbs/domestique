package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nothingSkipped keeps a projection test about the profile rather than about
// whatever the working tree happens to hold at those line numbers.
func nothingSkipped(string, int) bool { return false }

func TestReadProfileSpreadsABlockOverEveryLineItHolds(t *testing.T) {
	t.Parallel()

	profile := `mode: set
github.com/nobbs/domestique/internal/route/stage.go:10.2,12.3 2 1
github.com/nobbs/domestique/internal/route/stage.go:20.2,21.3 1 0
`

	measured, err := readProfile(strings.NewReader(profile), nothingSkipped)
	require.NoError(t, err)

	// The import path is gone: codecov.yml's `fixes` entry says a Go profile
	// names files by import path, and everything else here is a repository path.
	assert.Equal(t, lines{"internal/route/stage.go": {
		10: covered, 11: covered, 12: covered,
		20: missed, 21: missed,
	}}, measured)
}

// Every test binary that instruments a package reports every one of its blocks,
// so the same position arrives once per binary. Folding those together before
// projecting them is what keeps the binary that never reached a block from
// making a covered block's lines look partial.
func TestReadProfileFoldsARepeatedBlockBeforeProjectingIt(t *testing.T) {
	t.Parallel()

	profile := `mode: set
github.com/nobbs/domestique/internal/route/stage.go:10.2,11.3 1 0
github.com/nobbs/domestique/internal/route/stage.go:10.2,11.3 1 4
github.com/nobbs/domestique/internal/route/stage.go:10.2,11.3 1 0
`

	measured, err := readProfile(strings.NewReader(profile), nothingSkipped)
	require.NoError(t, err)

	assert.Equal(t, lines{"internal/route/stage.go": {10: covered, 11: covered}}, measured)
}

// The rule the whole program exists for. `if err != nil {` is the last line of
// the block that evaluated the condition and the first line of the block that
// handles the error, so a never-taken error path leaves it reached one way and
// not the other. Codecov counts that against the patch, and a measurement that
// calls it covered reports several points more than the gate will.
func TestReadProfileMarksALineTwoBlocksDisagreeOnPartial(t *testing.T) {
	t.Parallel()

	profile := `mode: set
github.com/nobbs/domestique/internal/route/stage.go:8.20,10.16 2 1
github.com/nobbs/domestique/internal/route/stage.go:10.16,12.3 1 0
`

	measured, err := readProfile(strings.NewReader(profile), nothingSkipped)
	require.NoError(t, err)

	assert.Equal(t, lines{"internal/route/stage.go": {
		8: covered, 9: covered, 10: partial, 11: missed, 12: missed,
	}}, measured)
}

func TestReadProfileDropsAFileWhoseEveryLineIsSkipped(t *testing.T) {
	t.Parallel()

	profile := `mode: set
github.com/nobbs/domestique/internal/route/stage.go:10.2,11.3 1 1
`

	measured, err := readProfile(strings.NewReader(profile), func(string, int) bool { return true })
	require.NoError(t, err)

	assert.Empty(t, measured)
}

func TestReadProfileRejectsAMalformedProfile(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		profile string
		message string
	}{
		"missing field": {
			profile: "mode: set\ninternal/a.go:1.1,2.2 3\n",
			message: "expected 3 fields",
		},
		"execution count is not a number": {
			profile: "mode: set\ninternal/a.go:1.1,2.2 3 often\n",
			message: "execution count",
		},
		"position names no file": {
			profile: "mode: set\n1.1,2.2 3 1\n",
			message: "names no file",
		},
		"position is not a span": {
			profile: "mode: set\ninternal/a.go:1.1 3 1\n",
			message: "is not a span",
		},
		"position has no column": {
			profile: "mode: set\ninternal/a.go:1,2 3 1\n",
			message: "is not a line and column",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := readProfile(strings.NewReader(test.profile), nothingSkipped)
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestWorkingTreeIgnoresLinesThatCarryNoCode(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "stage.go")
	require.NoError(t, os.WriteFile(file, []byte(strings.Join([]string{
		"func f() int {",
		"\t// A comment nobody executes.",
		"",
		"\treturn 1",
		"}",
	}, "\n")), 0o600))

	tree := &workingTree{}

	assert.False(t, tree.ignorable(file, 1), "a line of code")
	assert.True(t, tree.ignorable(file, 2), "a comment")
	assert.True(t, tree.ignorable(file, 3), "a blank line")
	assert.False(t, tree.ignorable(file, 4), "a statement")
	assert.True(t, tree.ignorable(file, 5), "a closing brace")
	assert.False(t, tree.ignorable(file, 99), "past the end of the file")
}

// A file the profile names and the working tree no longer holds must not have
// every line quietly dropped, which is what an unreadable file would otherwise
// mean.
func TestWorkingTreeSkipsNothingInAFileItCannotRead(t *testing.T) {
	t.Parallel()

	tree := &workingTree{}

	assert.False(t, tree.ignorable(filepath.Join(t.TempDir(), "absent.go"), 1))
}

func TestParseLCOVReadsEachRecord(t *testing.T) {
	t.Parallel()

	report := `TN:
SF:src/lib/surface.ts
DA:4,2
DA:7,0
LF:2
LH:1
end_of_record
`

	measured, err := parseLCOV(strings.NewReader(report))
	require.NoError(t, err)

	// The UI half of codecov.yml's `fixes`: Vitest writes paths relative to the
	// UI project rather than to the repository.
	assert.Equal(t, lines{"internal/webui/app/src/lib/surface.ts": {4: covered, 7: missed}}, measured)
}

// The LCOV half of the same rule: a line that ran but holds a branch nobody
// took is a partial, and Codecov counts a partial against the patch. "-" is a
// branch under an expression that never evaluated at all.
func TestParseLCOVMarksALineWithAnUntakenBranchPartial(t *testing.T) {
	t.Parallel()

	report := `TN:
SF:src/lib/surface.ts
DA:4,2
DA:7,3
DA:9,5
BRDA:4,0,0,2
BRDA:4,0,1,0
BRDA:7,1,0,-
BRDA:9,2,0,5
end_of_record
`

	measured, err := parseLCOV(strings.NewReader(report))
	require.NoError(t, err)

	assert.Equal(t, lines{"internal/webui/app/src/lib/surface.ts": {
		4: partial, 7: partial, 9: covered,
	}}, measured)
}

// A BRDA line with no DA record beside it is still a measured line, and reading
// it from the branches alone is worth about a point of this repository's UI
// total. Dropping it would understate the denominator and flatter the number.
func TestParseLCOVMeasuresALineThatOnlyBranchesReport(t *testing.T) {
	t.Parallel()

	report := `TN:
SF:src/lib/surface.ts
DA:4,2
BRDA:4,0,0,2
BRDA:11,0,0,-
BRDA:11,0,1,-
BRDA:14,0,0,3
BRDA:14,0,1,0
BRDA:17,0,0,3
BRDA:17,0,1,1
end_of_record
`

	measured, err := parseLCOV(strings.NewReader(report))
	require.NoError(t, err)

	assert.Equal(t, lines{"internal/webui/app/src/lib/surface.ts": {
		4:  covered,
		11: missed,
		14: partial,
		17: covered,
	}}, measured)
}

func TestParseLCOVRejectsAMalformedReport(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		report  string
		message string
	}{
		"DA is not a pair":          {report: "SF:src/a.ts\nDA:4\n", message: "DA record"},
		"DA line is not a number":   {report: "SF:src/a.ts\nDA:four,1\n", message: "DA record"},
		"BRDA has too few fields":   {report: "SF:src/a.ts\nBRDA:4,0,1\n", message: "expected 4 fields"},
		"BRDA taken is not a count": {report: "SF:src/a.ts\nBRDA:4,0,1,some\n", message: "BRDA record"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := parseLCOV(strings.NewReader(test.report))
			require.ErrorContains(t, err, test.message)
		})
	}
}

func TestChangedLinesReadsTheAddedLinesOfEachFile(t *testing.T) {
	t.Parallel()

	diff := `diff --git a/internal/route/stage.go b/internal/route/stage.go
--- a/internal/route/stage.go
+++ b/internal/route/stage.go
@@ -9,0 +10,2 @@ func Stage() {
+	first := 1
+	second := 2
@@ -20 +22 @@ func Other() {
+	replaced := 3
diff --git a/internal/route/gone.go b/internal/route/gone.go
--- a/internal/route/gone.go
+++ /dev/null
@@ -1,4 +0,0 @@
-	deleted := 4
diff --git a/internal/route/trimmed.go b/internal/route/trimmed.go
--- a/internal/route/trimmed.go
+++ b/internal/route/trimmed.go
@@ -30,2 +29,0 @@ func Trimmed() {
-	removed := 5
-	alsoRemoved := 6
`

	changed, err := changedLines(strings.NewReader(diff))
	require.NoError(t, err)

	// A deleted file adds nothing anywhere, and a hunk that only removes lines
	// has a destination length of zero.
	assert.Equal(t, map[string][]int{"internal/route/stage.go": {10, 11, 22}}, changed)
}

func TestChangedLinesRejectsAMalformedHunk(t *testing.T) {
	t.Parallel()

	diff := "+++ b/internal/route/stage.go\n@@ -9,0 10,2 @@\n"

	_, err := changedLines(strings.NewReader(diff))
	require.ErrorContains(t, err, "names no destination")
}

func TestJudgeCountsAPartialAgainstThePatch(t *testing.T) {
	t.Parallel()

	measured := lines{"internal/route/stage.go": {10: covered, 11: partial, 12: missed}}
	changed := map[string][]int{"internal/route/stage.go": {10, 11, 12}}

	found := judge(goLanguage(t), measured, changed, nil)

	assert.Equal(t, counts{covered: 1, total: 3}, found.patch)
	assert.Equal(t, []location{
		{file: "internal/route/stage.go", line: 11},
		{file: "internal/route/stage.go", line: 12},
	}, found.uncovered)
}

// A changed line the report says nothing about is not a line anyone can cover —
// a comment, a blank line, a declaration — and Codecov leaves it out of the
// ratio as well.
func TestJudgeIgnoresAChangedLineTheReportDoesNotHold(t *testing.T) {
	t.Parallel()

	measured := lines{"internal/route/stage.go": {10: covered}}
	changed := map[string][]int{"internal/route/stage.go": {9, 10, 11}}

	found := judge(goLanguage(t), measured, changed, nil)

	assert.Equal(t, counts{covered: 1, total: 1}, found.patch)
}

// The head's project coverage decides the bare comparison outright, and the
// threshold is the part it cannot decide: inside that band the answer turns on
// how far the base sits above the head, which this report does not hold.
func TestStandsSpendsTheThresholdOnlyWhereItIsSafe(t *testing.T) {
	t.Parallel()

	project := counts{covered: 800, total: 1000}

	tests := map[string]struct {
		patch counts
		want  verdict
	}{
		"above the project's own coverage":    {patch: counts{covered: 90, total: 100}, want: holds},
		"exactly the project's own coverage":  {patch: counts{covered: 80, total: 100}, want: holds},
		"inside the threshold below it":       {patch: counts{covered: 795, total: 1000}, want: undecided},
		"exactly the threshold below it":      {patch: counts{covered: 790, total: 1000}, want: undecided},
		"further below than the threshold":    {patch: counts{covered: 789, total: 1000}, want: breaks},
		"nothing measured, so nothing to owe": {patch: counts{}, want: holds},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, test.want, stands(test.patch, project))
		})
	}
}

func TestJudgeReadsTheVerdictOffWhatItMeasured(t *testing.T) {
	t.Parallel()

	measured := lines{"internal/route/patch.go": {
		1: covered, 2: missed, 3: missed, 4: missed,
	}, "internal/route/rest.go": {}}
	for line := range 10 {
		measured["internal/route/rest.go"][line+1] = covered
	}

	changed := map[string][]int{"internal/route/patch.go": {1, 2, 3, 4}}

	assert.Equal(t, breaks, judge(goLanguage(t), measured, changed, nil).stands)
}

// Nothing measured is nothing to fall short of. Codecov passes a patch with no
// coverable line in it, and a documentation change must not need an override.
func TestJudgePassesAPatchTheReportMeasuresNothingOf(t *testing.T) {
	t.Parallel()

	measured := lines{"internal/route/stage.go": {10: missed}}
	changed := map[string][]int{"README.md": {1, 2}}

	found := judge(goLanguage(t), measured, changed, nil)

	assert.Equal(t, counts{}, found.patch)
	assert.Equal(t, holds, found.stands)
}

// An accidental gap in the measured set would otherwise read as a pass, because
// a file nothing measures contributes no uncovered line.
func TestJudgeNamesChangedFilesTheReportShouldHoldAndDoesNot(t *testing.T) {
	t.Parallel()

	measured := lines{"internal/route/stage.go": {10: covered}}
	changed := map[string][]int{
		"internal/route/stage.go":      {10},
		"internal/route/unmeasured.go": {4},
		"internal/route/stage_test.go": {8},
		"docs/specs/delivery.md":       {2},
	}

	found := judge(goLanguage(t), measured, changed, []string{"internal/route/new.go", "notes.txt"})

	assert.Equal(t, []string{"internal/route/unmeasured.go"}, found.unmeasured)
	assert.Equal(t, []string{"internal/route/new.go"}, found.untracked)
}

func TestMeasurableGoMatchesCodecovPaths(t *testing.T) {
	t.Parallel()

	assert.True(t, measurableGo("internal/httpapi/handler.go"))
	assert.False(t, measurableGo("internal/httpapi/contract/openapi.gen.go"))
	assert.False(t, measurableGo("internal/httpapi/handler_test.go"))
	assert.False(t, measurableGo("dev/patchcoverage/main.go"))
}

func TestMeasurableUISkipsWhatVitestExcludes(t *testing.T) {
	t.Parallel()

	const root = "internal/webui/app/src/"

	assert.True(t, measurableUI(root+"lib/surface.ts"))
	assert.True(t, measurableUI(root+"components/RouteOverlay.tsx"))
	assert.False(t, measurableUI(root+"lib/surface.test.ts"))
	assert.False(t, measurableUI(root+"api/types.d.ts"))
	assert.False(t, measurableUI(root+"test/setup.ts"))
	assert.False(t, measurableUI(root+"main.tsx"))
	assert.False(t, measurableUI("internal/webui/app/e2e/library.spec.ts"))
}

func TestTenthsTruncatesRatherThanRounds(t *testing.T) {
	t.Parallel()

	// codecov.yml sets `precision: 1` with `round: down`, so 77.19% is 77.1%.
	assert.Equal(t, 771, tenths(counts{covered: 7719, total: 10000}))
	assert.Equal(t, 1000, tenths(counts{covered: 3, total: 3}))
	assert.Equal(t, 0, tenths(counts{}))
}

func TestRenderNamesTheUncoveredLinesAndTheVerdict(t *testing.T) {
	t.Parallel()

	found := result{
		patch:      counts{covered: 1, total: 2},
		project:    counts{covered: 9, total: 10},
		stands:     breaks,
		uncovered:  []location{{file: "internal/route/stage.go", line: 11}},
		unmeasured: []string{"internal/httpapi/views.go"},
		untracked:  []string{"internal/route/new.go"},
	}

	out := render(goLanguage(t), "abc1234", &found)

	assert.Contains(t, out, "against abc1234")
	assert.Contains(t, out, "patch    50.0%  (1/2 lines)")
	assert.Contains(t, out, "project  90.0%  (9/10 lines)")
	assert.Contains(t, out, "needed   89.0%")
	assert.Contains(t, out, "codecov/patch/go")
	assert.Contains(t, out, "internal/route/stage.go:11")
	assert.Contains(t, out, "internal/httpapi/views.go")
	assert.Contains(t, out, "internal/route/new.go")
}

// The UI status reports and does not judge, so a shortfall there says so rather
// than claiming a merge is blocked by it.
func TestRenderSaysAShortfallBlocksNothingWhereTheStatusIsInformational(t *testing.T) {
	t.Parallel()

	ui, err := languageNamed("ui")
	require.NoError(t, err)

	out := render(ui, "abc1234", &result{
		patch:   counts{total: 2},
		project: counts{covered: 1, total: 1},
		stands:  breaks,
	})

	assert.Contains(t, out, "blocks nothing")
}

// Spending the threshold on a project percentage below it would leave a negative
// number, which reads as nonsense rather than as the floor it stands for.
func TestRenderNeverAsksForANegativePercentage(t *testing.T) {
	t.Parallel()

	for name, project := range map[string]counts{
		"an empty report":    {},
		"a report under one": {covered: 5, total: 1000},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			out := render(goLanguage(t), "abc1234", &result{
				patch:   counts{covered: 1, total: 2},
				project: project,
				stands:  breaks,
			})

			assert.Contains(t, out, "needed   0.0%")
			assert.NotContains(t, out, "-")
		})
	}
}

func TestLanguageNamedRejectsAnUnknownFlag(t *testing.T) {
	t.Parallel()

	_, err := languageNamed("rust")
	require.ErrorContains(t, err, "expected go or ui")
}

func goLanguage(t *testing.T) language {
	t.Helper()

	lang, err := languageNamed("go")
	require.NoError(t, err)

	return lang
}
