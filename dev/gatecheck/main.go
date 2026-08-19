// Command gatecheck guards the property that makes a fast local loop safe to
// offer: `mise run quick` is a strict subset of `mise run check`, and the work
// it leaves out is exactly the set that was deliberately deferred.
//
// GitHub Actions is the authoritative gate, so `quick` is allowed to be smaller
// than `check`. What it must never become is *different* — a loop that checks
// something the full gate does not is a loop that can pass work the merge gate
// then rejects, and a check added to `check` alone silently stops being part of
// the routine loop. Both directions are asserted here:
//
//   - every task `quick` runs is also run by `check`; and
//   - every task `check` runs is also run by `quick`, except the deferred set
//     below, which is named here so that adding to it is a deliberate edit.
//
// The comparison reads `mise tasks ls --json`, which dumps every task with its
// dependencies without running any of them, so this costs a few milliseconds
// and needs no network.
//
// That comparison can only see a step declared as a dependency, and mise lets a
// task carry an inline `run` alongside its `depends`. A command written there
// would still run as part of the gate while being invisible to a dependency
// query, which is precisely the hole the comparison must not have. So the
// structure it relies on is asserted first: a gate task declares its steps and
// runs nothing itself. Without that check this program would assert rather less
// than it appears to.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
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
	// delegate to. The groups are the decomposition CI uses to name a failing
	// area, not checks in their own right, so the walk descends through them
	// rather than counting them. Every one of them must declare its steps and
	// run nothing itself.
	gate []string

	// preparation is the only dependency of a gate task that is not a check.
	// Both entries install the browser UI dependency tree, which is a
	// precondition of running the UI checks rather than a check that could go
	// unnoticed, so neither counts towards the comparison.
	preparation []string
}

// gateRules is the repository's gate, as mise-tasks.toml declares it.
//
// build-check rebuilds the UI bundle and compiles the published release target,
// which is the slowest check in the gate whenever the build cache is cold;
// vulncheck and ui-audit each need the network and a current advisory database;
// ui-browser-install downloads a browser, and ui-browser-test then drives it
// over the demo stack for minutes. That is why each is deferred.
func gateRules() rules {
	return rules{
		deferred: []string{
			"build-check",
			"ui-audit",
			"ui-browser-install",
			"ui-browser-test",
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
			"ui-ensure",
			"ui-install",
		},
	}
}

// task is one entry of `mise tasks ls --json`, narrowed to the fields the two
// assertions read.
type task struct {
	Name    string   `json:"name"`
	Depends []string `json:"depends"`
	//nolint:tagliatelle // mise's task JSON uses snake_case.
	DependsPost []string `json:"depends_post"`
	Run         []string `json:"run"`
}

func main() {
	tasks, err := load(context.Background())
	if err != nil {
		fmt.Fprintf(os.Stderr, "gatecheck: %v\n", err)
		os.Exit(1)
	}

	problems, err := analyse(tasks)
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
func analyse(tasks []task) ([]string, error) {
	r := gateRules()

	byName := make(map[string]task, len(tasks))
	for _, t := range tasks {
		byName[t.Name] = t
	}

	var problems []string

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

// steps returns the checks a gate task runs, sorted. The walk descends through
// the gate tasks, because a group is a way of naming a failing area rather than
// a check, and stops at anything else: a check's own dependencies are how it
// arranges to run, not further checks. That is why `ui-build` does not count as
// a step of `build-check`: bundling the UI is how that check compiles at all,
// and neither entry point is claiming to run it as a check.
//
// Preparation tasks are dropped here rather than filtered by the caller, so that
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
