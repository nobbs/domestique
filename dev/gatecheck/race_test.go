package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The race check is spread across two files, and the properties that make it
// worth having are properties of the pair. The task-graph comparison in main.go
// sees neither: it knows that `check` runs `test-race` and that `quick` defers
// it deliberately, and nothing about what the task does or how long the job that
// runs it is given. These tests read the files themselves, the way
// internal/readiness reads the deployment files it holds to the code.

// A wedged package has to be given up on by the toolchain rather than by the
// runner. `go test` prints every goroutine's stack when its own -timeout
// expires, which is the only artefact that explains a hang; GitHub Actions
// prints nothing at all when `timeout-minutes` does. Both default to ten
// minutes, so left alone they expire together and the dump is lost.
//
// The comparison is against the sum of both suites, not the race one alone. The
// Test job runs `test` and then `test-race`, and a -timeout bounds each test
// binary rather than the invocation, so neither suite's total runtime is capped
// by its own number: a job budget that cleared only the race timeout could still
// expire while the second suite was inside its first ten minutes.
//
// A sum is a floor rather than a guarantee — a tree slow enough that the
// packages that do finish outlast the job on their own would still be cut off —
// but it is the strongest bound available without timing the suite, and it fails
// on the edit that would otherwise reintroduce the collision quietly: lowering
// the job budget, or raising a suite's timeout into it.
func TestTheToolchainGivesUpBeforeTheRunnerDoes(t *testing.T) {
	t.Parallel()

	require.Contains(t, taskRun(t, "test-race"), " -race ",
		"the race task must actually run the detector")

	plain := suiteTimeoutMinutes(t, "test")
	race := suiteTimeoutMinutes(t, "test-race")

	job := jobTimeoutMinutes(t, "test")
	assert.Greater(t, job, plain+race,
		"the Test job's timeout-minutes (%d) must exceed both suites' timeouts together "+
			"(test %dm + test-race %dm), because it runs them back to back",
		job, plain, race)
}

// suiteTimeoutMinutes returns the -timeout one Go suite gives the toolchain.
// Both suites state it rather than inheriting the default, so that the sum above
// is a number the gate can read.
func suiteTimeoutMinutes(t *testing.T, name string) int {
	t.Helper()

	run := taskRun(t, name)

	match := regexp.MustCompile(`-timeout=(\d+)m\b`).FindStringSubmatch(run)
	require.Len(t, match, 2, "task %q must set an explicit -timeout in minutes: %q", name, run)

	minutes, err := strconv.Atoi(match[1])
	require.NoError(t, err)

	return minutes
}

// The detector is the one thing in this repository that needs cgo, and the one
// thing allowed to have it. What ships is statically linked and built with the
// detector nowhere near it, so a `CGO_ENABLED=1` that spread to a build task
// would change the published artefact rather than only a test binary.
func TestOnlyTheRaceTaskEnablesCgo(t *testing.T) {
	t.Parallel()

	tasks := repositoryFile(t, "mise-tasks.toml")

	assert.Equal(t, 1, strings.Count(tasks, "CGO_ENABLED=1"),
		"only the race task may enable cgo")
	assert.Contains(t, taskRun(t, "test-race"), "CGO_ENABLED=1")

	for _, name := range []string{"build", "build-check"} {
		assert.Contains(t, taskRun(t, name), "CGO_ENABLED=0",
			"task %q builds what ships and must stay cgo-free", name)
	}
}

// taskRun returns the `run` block of one mise task, from its table header to the
// next one. mise-tasks.toml writes a run as a string, a list, or a heredoc, so
// this returns the text rather than the command.
func taskRun(t *testing.T, name string) string {
	t.Helper()

	section := taskSection(t, name)

	_, run, found := strings.Cut(section, "\nrun =")
	require.True(t, found, "task %q declares no run", name)

	return run
}

// taskSection returns one table of mise-tasks.toml, header excluded.
func taskSection(t *testing.T, name string) string {
	t.Helper()

	_, rest, found := strings.Cut(repositoryFile(t, "mise-tasks.toml"), "\n["+name+"]\n")
	require.True(t, found, "mise-tasks.toml declares no task %q", name)

	if end := strings.Index(rest, "\n["); end >= 0 {
		rest = rest[:end]
	}

	return rest
}

// jobTimeoutMinutes returns the wall-clock budget one CI job is given.
func jobTimeoutMinutes(t *testing.T, job string) int {
	t.Helper()

	_, rest, found := strings.Cut(repositoryFile(t, ".github/workflows/ci.yml"), "\n  "+job+":\n")
	require.True(t, found, "ci.yml declares no job %q", job)

	if end := regexp.MustCompile(`(?m)^ {2}[a-z-]+:$`).FindStringIndex(rest); end != nil {
		rest = rest[:end[0]]
	}

	match := regexp.MustCompile(`(?m)^ {4}timeout-minutes: (\d+)$`).FindStringSubmatch(rest)
	require.Len(t, match, 2, "job %q declares no timeout-minutes", job)

	minutes, err := strconv.Atoi(match[1])
	require.NoError(t, err)

	return minutes
}

func repositoryFile(t *testing.T, name string) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	contents, err := os.ReadFile(filepath.Join(root, name)) //nolint:gosec // a repository file, named by this test
	require.NoError(t, err)

	return string(contents)
}
