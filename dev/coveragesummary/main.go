// Command coveragesummary turns a merged Go coverage profile into a per-package
// summary, reading the profile on standard input.
//
// The profile is produced with -coverpkg over the whole service, so that code
// exercised only through another package's tests is not reported as dead. The
// cost is that the percentage `go test` prints for each package becomes the
// fraction of the whole service that that package's tests reached, which is not
// a number anyone wants to read. The merged profile still holds the real one,
// so this reads it from there.
package main

import (
	"bufio"
	"fmt"
	"io"
	"maps"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
)

func main() {
	if err := summarize(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "coveragesummary: %v\n", err)
		os.Exit(1)
	}
}

// block is one instrumented statement block: how many statements it holds, and
// whether a test run reached it.
type block struct {
	statements int
	covered    bool
}

// counts is the statement total of one package and how much of it was reached.
type counts struct {
	statements int
	covered    int
}

func summarize(in io.Reader, out io.Writer) error {
	blocks, err := parse(in)
	if err != nil {
		return err
	}

	packages, total := aggregate(blocks)

	if _, err := io.WriteString(out, render(packages, total)); err != nil {
		return fmt.Errorf("writing summary: %w", err)
	}

	return nil
}

// parse reads a profile into its blocks, keyed by position. Every test binary
// that instruments a package reports every one of its blocks, so the same
// position arrives once per binary; a block is covered if any of them reached
// it.
func parse(in io.Reader) (map[string]block, error) {
	blocks := make(map[string]block)

	scanner := bufio.NewScanner(in)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if text == "" || (line == 1 && strings.HasPrefix(text, "mode:")) {
			continue
		}

		position, parsed, err := parseBlock(text)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}

		if existing, seen := blocks[position]; !seen || parsed.covered {
			parsed.covered = parsed.covered || existing.covered
			blocks[position] = parsed
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading profile: %w", err)
	}

	return blocks, nil
}

// parseBlock reads one profile line: a position, the statements it holds, and
// the number of times a test run executed it.
func parseBlock(line string) (string, block, error) {
	const fieldCount = 3

	fields := strings.Fields(line)
	if len(fields) != fieldCount {
		return "", block{}, fmt.Errorf("expected %d fields, got %d", fieldCount, len(fields))
	}

	statements, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", block{}, fmt.Errorf("statement count: %w", err)
	}

	executions, err := strconv.Atoi(fields[2])
	if err != nil {
		return "", block{}, fmt.Errorf("execution count: %w", err)
	}

	return fields[0], block{statements: statements, covered: executions > 0}, nil
}

func aggregate(blocks map[string]block) (map[string]counts, counts) {
	packages := make(map[string]counts)

	var total counts

	for position, reported := range blocks {
		name := packageOf(position)

		current := packages[name]
		current.statements += reported.statements
		total.statements += reported.statements

		if reported.covered {
			current.covered += reported.statements
			total.covered += reported.statements
		}

		packages[name] = current
	}

	return packages, total
}

// packageOf takes the import path out of a block position, which is a file path
// followed by ":line.column,line.column".
func packageOf(position string) string {
	file := position
	if colon := strings.LastIndex(file, ":"); colon >= 0 {
		file = file[:colon]
	}

	return path.Dir(file)
}

func render(packages map[string]counts, total counts) string {
	const (
		nameHeader       = "package"
		coverageHeader   = "coverage"
		statementsHeader = "statements"
		gap              = "  "
	)

	names := slices.Sorted(maps.Keys(packages))

	nameWidth := max(len(nameHeader), len("total"))
	statementsWidth := len(statementsHeader)

	for _, name := range names {
		nameWidth = max(nameWidth, len(name))
		statementsWidth = max(statementsWidth, len(statementsOf(packages[name])))
	}

	statementsWidth = max(statementsWidth, len(statementsOf(total)))

	var out strings.Builder

	fmt.Fprintf(&out, "%-*s%s%*s%s%*s\n",
		nameWidth, nameHeader,
		gap, len(coverageHeader), coverageHeader,
		gap, statementsWidth, statementsHeader)

	row := func(name string, reported counts) {
		fmt.Fprintf(&out, "%-*s%s%*s%s%*s\n",
			nameWidth, name,
			gap, len(coverageHeader), percentageOf(reported),
			gap, statementsWidth, statementsOf(reported))
	}

	for _, name := range names {
		row(name, packages[name])
	}

	row("total", total)

	return out.String()
}

func percentageOf(reported counts) string {
	if reported.statements == 0 {
		return "n/a"
	}

	return fmt.Sprintf("%.1f%%", 100*float64(reported.covered)/float64(reported.statements))
}

func statementsOf(reported counts) string {
	return fmt.Sprintf("%d/%d", reported.covered, reported.statements)
}
