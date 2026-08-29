// Command gatecheck asserts three properties of the local gate.
//
//   - `mise run quick` is a strict subset of `mise run check`, and the
//     difference is exactly the deferred set below. Both directions are checked.
//   - Every declared `sources` glob matches a tracked file: one matching nothing
//     counts as "nothing moved" forever.
//   - A gate task declares its steps and runs no command itself, since an inline
//     `run` would be invisible to the dependency query the first check reads.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"slices"
	"strings"
)

// rules names the three sets the assertions are written against.
type rules struct {
	// deferred is what `check` runs and `quick` does not, and it is the whole
	// of that difference: a task here is out of the routine local loop, so
	// adding one needs a reason.
	deferred []string

	// gate is the gate tasks: the two entry points and the ci-* groups they
	// delegate to. The walk descends through the groups rather than counting them,
	// and each must declare its steps and run nothing itself.
	gate []string

	// preparation is the only dependency of a gate task that is not a check.
	// It installs the browser UI dependency tree, which is a precondition of
	// running the UI checks rather than a check that could go unnoticed, so it
	// does not count towards the comparison.
	preparation []string
}

// gateRules is the repository's gate, as mise-tasks.toml declares it. Each
// deferred check is slow or needs the network: build-check recompiles the release
// target, test-race needs cgo, vulncheck and ui-audit need advisory databases,
// and ui-browser-* download and drive a browser.
func gateRules() rules {
	return rules{
		deferred: []string{
			"build-check",
			"test-race",
			"ui-audit",
			"ui-browser-install",
			"ui-browser-test",
			"ui-storybook-test",
			"ui-storybook-sweep",
			"vulncheck",
		},
		gate: []string{
			"check",
			"ci-lint",
			"ci-security",
			"ci-test",
			"ci-ui",
			"quick",
		},
		preparation: []string{
			"ui-install",
		},
	}
}

// task is one entry of `mise tasks ls --json`, narrowed to the fields the two
// assertions read.
type task struct {
	Name    string   `json:"name"`
	Depends []string `json:"depends"`
	Sources []string `json:"sources"`
	//nolint:tagliatelle // mise's task JSON uses snake_case.
	DependsPost []string `json:"depends_post"`
	Run         []string `json:"run"`
}

func main() {
	ctx := context.Background()

	tasks, err := load(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gatecheck: %v\n", err)
		os.Exit(1)
	}

	tracked, err := trackedFiles(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gatecheck: %v\n", err)
		os.Exit(1)
	}

	problems, err := analyse(tasks, tracked)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gatecheck: %v\n", err)
		os.Exit(1)
	}

	if len(problems) > 0 {
		for _, problem := range problems {
			fmt.Fprintf(os.Stderr, "gatecheck: %s\n", problem)
		}

		os.Exit(1)
	}

	fmt.Printf("gatecheck: 'quick' is a strict subset of 'check', deferring: %s\n",
		strings.Join(gateRules().deferred, " "))
}

// load reads the task graph from mise. `tasks ls` resolves the whole graph
// without running anything.
func load(ctx context.Context) ([]task, error) {
	cmd := exec.CommandContext(ctx, "mise", "tasks", "ls", "--json")
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading the task graph: %w", err)
	}

	return decode(bytes.NewReader(out))
}

// trackedFiles lists what Git holds, which is the set a source glob is allowed
// to name: a file Git does not track cannot be a dependable input to a check.
func trackedFiles(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "-z")
	cmd.Stderr = os.Stderr

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing the tracked files: %w", err)
	}

	names := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")

	return slices.DeleteFunc(names, func(name string) bool { return name == "" }), nil
}

func decode(r io.Reader) ([]task, error) {
	var tasks []task
	if err := json.NewDecoder(r).Decode(&tasks); err != nil {
		return nil, fmt.Errorf("parsing the task graph: %w", err)
	}

	return tasks, nil
}

// analyse returns every way the two entry points fail the rules above, as
// messages ready to print. An error is returned only when the graph cannot be
// walked at all, which is a different kind of failure from a rule being broken.
func analyse(tasks []task, tracked []string) ([]string, error) {
	r := gateRules()

	byName := make(map[string]task, len(tasks))
	for _, t := range tasks {
		byName[t.Name] = t
	}

	problems := staleSources(tasks, tracked)

	// Rule zero: the structure the two comparisons below can actually see.
	for _, name := range r.gate {
		t, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("no task named %q", name)
		}

		if len(t.Run) > 0 {
			problems = append(problems, fmt.Sprintf(
				"gate task %q runs a command of its own: %q.\n"+
					"  Every gate step must be its own task, named in depends, or the subset\n"+
					"  comparison silently ignores it.", name, t.Run[0]))
		}
	}

	quick, err := r.steps(byName, "quick")
	if err != nil {
		return nil, err
	}

	check, err := r.steps(byName, "check")
	if err != nil {
		return nil, err
	}

	// Direction one: nothing is in the routine loop that the full gate skips.
	if extra := missing(quick, check); len(extra) > 0 {
		problems = append(problems, fmt.Sprintf(
			"'quick' runs tasks that 'check' does not:\n%s\n"+
				"  Add them to 'check' as well, or drop them from 'quick'.", indent(extra)))
	}

	// Direction two: what the full gate runs and the routine loop skips is
	// exactly the deferred set — no more, and no less.
	skipped := missing(check, quick)

	if undeclared := missing(skipped, r.deferred); len(undeclared) > 0 {
		problems = append(problems, fmt.Sprintf(
			"'check' runs tasks that 'quick' skips silently:\n%s\n"+
				"  Add them to 'quick', or defer them deliberately in gatecheck's\n"+
				"  deferred set.", indent(undeclared)))
	}

	if stale := missing(r.deferred, skipped); len(stale) > 0 {
		problems = append(problems, fmt.Sprintf(
			"the deferred set names tasks 'check' no longer runs:\n%s\n"+
				"  Remove them from gatecheck's deferred set.", indent(stale)))
	}

	return problems, nil
}

// staleSources reports every source glob that matches nothing, because mise
// treats one as permanently up to date and would skip the check that declares
// it for good.
func staleSources(tasks []task, tracked []string) []string {
	var problems []string

	for _, t := range tasks {
		for _, pattern := range t.Sources {
			if slices.ContainsFunc(tracked, func(name string) bool {
				return matchGlob(pattern, name)
			}) {
				continue
			}

			problems = append(problems, fmt.Sprintf(
				"task %q declares the source %q, which matches no tracked file.\n"+
					"  mise reads that as nothing to do and skips the task from now on,\n"+
					"  so correct the glob or drop it.", t.Name, pattern))
		}
	}

	return problems
}

// matchGlob reports whether a mise source glob matches a slash-separated path.
// `**` stands for any number of path segments, including none; within a segment
// the usual shell metacharacters apply.
func matchGlob(pattern, name string) bool {
	return matchSegments(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegments(pattern, name []string) bool {
	for len(pattern) > 0 {
		if pattern[0] == "**" {
			for i := range len(name) + 1 {
				if matchSegments(pattern[1:], name[i:]) {
					return true
				}
			}

			return false
		}

		if len(name) == 0 {
			return false
		}

		if ok, err := path.Match(pattern[0], name[0]); err != nil || !ok {
			return false
		}

		pattern, name = pattern[1:], name[1:]
	}

	return len(name) == 0
}

// steps returns the checks a gate task runs, sorted. The walk descends through
// the gate tasks and stops at anything else: a check's own dependencies are how
// it arranges to run, not further checks. Preparation tasks are dropped here so
// neither comparison has to know they exist.
func (r rules) steps(byName map[string]task, root string) ([]string, error) {
	found := map[string]bool{}
	seen := map[string]bool{}

	var walk func(name string) error

	walk = func(name string) error {
		if seen[name] {
			return nil
		}

		seen[name] = true

		t, ok := byName[name]
		if !ok {
			return fmt.Errorf("no task named %q", name)
		}

		for _, dep := range slices.Concat(t.Depends, t.DependsPost) {
			if slices.Contains(r.gate, dep) {
				if err := walk(dep); err != nil {
					return err
				}

				continue
			}

			if _, ok := byName[dep]; !ok {
				return fmt.Errorf("task %q depends on %q, which does not exist", name, dep)
			}

			if slices.Contains(r.preparation, dep) {
				continue
			}

			found[dep] = true
		}

		return nil
	}

	if err := walk(root); err != nil {
		return nil, err
	}

	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}

	slices.Sort(names)

	return names, nil
}

// missing returns the members of a that b does not hold, sorted.
func missing(a, b []string) []string {
	var out []string

	for _, name := range a {
		if !slices.Contains(b, name) {
			out = append(out, name)
		}
	}

	slices.Sort(out)

	return out
}

// indent renders a set for a diagnostic, one name per line.
func indent(names []string) string {
	lines := make([]string, 0, len(names))
	for _, name := range names {
		lines = append(lines, "  "+name)
	}

	return strings.Join(lines, "\n")
}
