package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// graph is the smallest task set that satisfies both rules: `quick` runs two
// checks, `check` runs those two plus exactly the deferred set, and the ci-*
// groups decompose it the way CI does. Every test starts from this and breaks
// one thing, so what a case asserts is the edit it makes.
//
// `build-check` depends on `ui-build` here for the same reason it does in
// mise-tasks.toml: it is how that check arranges to run, not a further check,
// and the walk must not count it as a step.
func graph() []task {
	return []task{
		{Name: "quick", Depends: []string{"ui-install", "hygiene", "vet"}},
		{Name: "check", Depends: []string{
			"ci-lint", "ci-test", "ci-security", "ci-ui", "build-check",
		}},
		{Name: "ci-lint", Depends: []string{"hygiene"}},
		{Name: "ci-test", Depends: []string{"vet"}},
		{Name: "ci-security", Depends: []string{"vulncheck", "ui-audit"}},
		{Name: "ci-ui", Depends: []string{
			"ui-install", "ui-browser-install", "ui-browser-test",
		}},
		{Name: "build-check", Depends: []string{"ui-build"}, Run: []string{"go build ./..."}},
		{Name: "hygiene", Run: []string{"prek run --all-files"}},
		{Name: "vet", Run: []string{"go vet ./..."}},
		{Name: "vulncheck", Run: []string{"govulncheck ./..."}},
		{Name: "ui-audit", Run: []string{"npm audit"}},
		{Name: "ui-browser-install", Run: []string{"npm run test:browser:install"}},
		{Name: "ui-browser-test", Run: []string{"npm run test:browser"}},
		{Name: "ui-build", Run: []string{"npm run build"}},
		{Name: "ui-install", Run: []string{"npm ci"}},
		{Name: "ui-install", Run: []string{"npm ci"}},

		// Defined but wired into neither root, so a case can add a step
		// without also having to define it.
		{Name: "test", Run: []string{"go test ./..."}},
		{Name: "secret-scan", Run: []string{"gitleaks dir ."}},
	}
}

// with returns the graph with one task replaced, so a case names only what it
// changes.
func with(replacement *task) []task {
	tasks := graph()
	for i, t := range tasks {
		if t.Name == replacement.Name {
			tasks[i] = *replacement

			return tasks
		}
	}

	return append(tasks, *replacement)
}

func TestAnalyseAcceptsASubsetGraph(t *testing.T) {
	t.Parallel()

	problems, err := analyse(graph(), nil)
	require.NoError(t, err)
	assert.Empty(t, problems)
}

// The rule that makes the two comparisons worth anything: a gate task that runs
// a command of its own hides that work from a dependency query.
func TestAnalyseRejectsAnInlineRunOnAGateTask(t *testing.T) {
	t.Parallel()

	problems, err := analyse(with(&task{
		Name:    "ci-test",
		Depends: []string{"vet"},
		Run:     []string{"go test ./..."},
	}), nil)
	require.NoError(t, err)

	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], `gate task "ci-test" runs a command of its own`)
	assert.Contains(t, problems[0], "go test ./...")
}

func TestAnalyseRejectsACheckOnlyTheRoutineLoopRuns(t *testing.T) {
	t.Parallel()

	problems, err := analyse(with(&task{
		Name:    "quick",
		Depends: []string{"ui-install", "hygiene", "vet", "secret-scan"},
	}), nil)
	require.NoError(t, err)

	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "'quick' runs tasks that 'check' does not")
	assert.Contains(t, problems[0], "secret-scan")
}

// A check added to the full gate and forgotten in the routine loop is the
// failure this program exists to catch.
func TestAnalyseRejectsAnUndeclaredDeferral(t *testing.T) {
	t.Parallel()

	problems, err := analyse(with(&task{
		Name:    "ci-test",
		Depends: []string{"vet", "test"},
	}), nil)
	require.NoError(t, err)

	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "'check' runs tasks that 'quick' skips silently")
	assert.Contains(t, problems[0], "test")
}

func TestAnalyseRejectsAStaleDeferral(t *testing.T) {
	t.Parallel()

	problems, err := analyse(with(&task{
		Name:    "ci-security",
		Depends: []string{"ui-audit"},
	}), nil)
	require.NoError(t, err)

	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "the deferred set names tasks 'check' no longer runs")
	assert.Contains(t, problems[0], "vulncheck")
}

// Both directions can break at once, and a run that reports only the first
// costs a second round trip.
func TestAnalyseReportsEveryBrokenRuleAtOnce(t *testing.T) {
	t.Parallel()

	tasks := with(&task{
		Name:    "quick",
		Depends: []string{"ui-install", "hygiene", "vet", "secret-scan"},
	})

	for i, t2 := range tasks {
		if t2.Name == "ci-lint" {
			tasks[i] = task{Name: "ci-lint", Depends: []string{"hygiene"}, Run: []string{"true"}}
		}
	}

	problems, err := analyse(tasks, nil)
	require.NoError(t, err)
	assert.Len(t, problems, 2)
}

func TestStepsStopsAtACheckSOwnDependencies(t *testing.T) {
	t.Parallel()

	byName := map[string]task{}
	for _, t2 := range graph() {
		byName[t2.Name] = t2
	}

	found, err := gateRules().steps(byName, "check")
	require.NoError(t, err)

	assert.Contains(t, found, "build-check")
	assert.NotContains(t, found, "ui-build",
		"a check's own dependency is how it arranges to run, not a further check")
}

func TestStepsDropsPreparation(t *testing.T) {
	t.Parallel()

	byName := map[string]task{}
	for _, t2 := range graph() {
		byName[t2.Name] = t2
	}

	quick, err := gateRules().steps(byName, "quick")
	require.NoError(t, err)
	assert.Equal(t, []string{"hygiene", "vet"}, quick)

	check, err := gateRules().steps(byName, "check")
	require.NoError(t, err)
	assert.NotContains(t, check, "ui-install")
}

func TestAnalyseErrorsOnATaskThatDoesNotExist(t *testing.T) {
	t.Parallel()

	_, err := analyse(with(&task{
		Name:    "ci-test",
		Depends: []string{"vet", "typo-check"},
	}), nil)
	require.ErrorContains(t, err, `task "ci-test" depends on "typo-check", which does not exist`)
}

func TestAnalyseErrorsOnAMissingGateTask(t *testing.T) {
	t.Parallel()

	var tasks []task

	for _, t2 := range graph() {
		if t2.Name != "ci-ui" {
			tasks = append(tasks, t2)
		}
	}

	_, err := analyse(tasks, nil)
	require.ErrorContains(t, err, `no task named "ci-ui"`)
}

// A cycle is mise's to reject — `mise tasks validate` runs in the same gate —
// but this walk must terminate rather than hang if one reaches it.
func TestStepsTerminatesOnACycle(t *testing.T) {
	t.Parallel()

	byName := map[string]task{
		"quick":   {Name: "quick", Depends: []string{"ci-test"}},
		"ci-test": {Name: "ci-test", Depends: []string{"quick", "vet"}},
		"vet":     {Name: "vet", Run: []string{"go vet ./..."}},
	}

	found, err := gateRules().steps(byName, "quick")
	require.NoError(t, err)
	assert.Equal(t, []string{"vet"}, found)
}

// A glob that matches nothing is what mise reads as "nothing to do", so it is
// the one way caching can retire a check without anyone noticing.
func TestAnalyseRejectsASourceGlobThatMatchesNothing(t *testing.T) {
	t.Parallel()

	tracked := []string{"internal/app/main.go", "docs/specs/delivery.md"}

	problems, err := analyse(with(&task{
		Name:    "vet",
		Sources: []string{"**/*.go", "cmd/**/*.rs"},
		Run:     []string{"go vet ./..."},
	}), tracked)
	require.NoError(t, err)

	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], `task "vet" declares the source "cmd/**/*.rs"`)
	assert.NotContains(t, problems[0], "**/*.go")
}

func TestAnalyseAcceptsSourceGlobsThatMatch(t *testing.T) {
	t.Parallel()

	problems, err := analyse(with(&task{
		Name:    "vet",
		Sources: []string{"**/*.go", "docs/**", "go.mod"},
		Run:     []string{"go vet ./..."},
	}), []string{"internal/app/main.go", "docs/specs/delivery.md", "go.mod"})
	require.NoError(t, err)
	assert.Empty(t, problems)
}

func TestMatchGlob(t *testing.T) {
	t.Parallel()

	cases := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"**/*.go", "main.go", true},
		{"**/*.go", "internal/app/main.go", true},
		{"**/*.go", "internal/app/main.ts", false},
		{"**/testdata/**", "internal/sqlite/testdata/schema.sha256", true},
		{"**/testdata/**", "internal/sqlite/schema.sha256", false},
		{"internal/webui/app/src/**", "internal/webui/app/src/map/view.tsx", true},
		{"internal/webui/app/src/**", "internal/webui/app/e2e/library.spec.ts", false},
		{"internal/webui/app/*.ts", "internal/webui/app/vite.config.ts", true},
		{"internal/webui/app/*.ts", "internal/webui/app/src/main.ts", false},
		{"deploy/*.sh", "deploy/deploy.sh", true},
		{"go.mod", "go.mod", true},
		{"go.mod", "dev/go.mod", false},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, matchGlob(c.pattern, c.name), "%q against %q", c.pattern, c.name)
	}
}

func TestDecodeReadsTheFieldsTheRulesUse(t *testing.T) {
	t.Parallel()

	tasks, err := decode(strings.NewReader(`[
		{"name": "check", "depends": ["ci-test"], "depends_post": [], "run": []},
		{"name": "ci-test", "depends": [], "depends_post": ["report"], "run": ["go test ./..."]}
	]`))
	require.NoError(t, err)
	require.Len(t, tasks, 2)

	assert.Equal(t, "check", tasks[0].Name)
	assert.Equal(t, []string{"ci-test"}, tasks[0].Depends)
	assert.Empty(t, tasks[0].Run)
	assert.Equal(t, []string{"report"}, tasks[1].DependsPost)
	assert.Equal(t, []string{"go test ./..."}, tasks[1].Run)
}
