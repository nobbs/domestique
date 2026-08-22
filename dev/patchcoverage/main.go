// Command patchcoverage decides, locally, the coverage question that decides a
// merge.
//
// codecov/patch/go is the only merge-blocking status this repository has with no
// local equivalent, and it is the one that fails: the shortfall is discovered
// five minutes and one push after the change was written, and then again after
// the fix. This reads the same two reports CI uploads — .coverage/go.out and
// .coverage/ui/lcov.info, as written by `mise run coverage` — and grades them by
// the rules in codecov.yml, before any of that.
//
// # Why no base measurement is needed
//
// codecov.yml leaves the patch statuses' target unset, so each compares the
// patch against the base commit's project coverage for the same flag. This
// compares it against the head's instead, and for the bare comparison that is
// not an approximation: project coverage is the weighted average of the lines
// the patch did not touch and the patch itself, so the head's number always
// lies between the base's and the patch's. A patch that clears the head's
// number therefore clears the base's, and one that falls short of either falls
// short of both. That is why nothing here needs a second report, and so why
// nothing here goes to the network.
//
// The threshold is where that stops holding, and the failure would be a false
// pass. codecov.yml allows the patch to sit 1% under the base, and the head's
// number is not the base's — a large patch pulls it toward the patch's own,
// which would let this report a pass where the base is more than a point
// higher and the status fails. So the slack is not spent blindly. Three
// verdicts come out of the one measurement:
//
//   - patch at or above the head's project coverage: the status passes, with
//     the threshold untouched and nothing resting on it.
//   - patch more than a point below it: the base is higher still, so the status
//     fails.
//   - patch inside that point: undecidable from this report alone, because the
//     answer turns on how far the base sits above the head. It is reported as
//     such and counted as a shortfall, which makes this stricter than the
//     status within a band under a point wide and never looser.
//
// # Partials count against
//
// Codecov's ratio is hits over hits plus misses plus partials, and a partial is
// a line reached one way and not another: in a Go profile, a line held by both a
// covered block and an uncovered one — `if err != nil {` whose error path never
// ran; in LCOV, a line whose BRDA records a branch nobody took. Counting those as
// covered is what makes a hand-rolled measurement report 95% where the gate says
// 77%.
//
// # What the numbers mean
//
// Line coverage, as Codecov computes it. It is not what dev/coveragesummary
// prints — that is statement coverage over the same profile, and it reads
// several points higher. Neither is wrong; only one of them gates.
//
// The UI half reproduces Codecov exactly, because LCOV already describes lines:
// against this repository's own report it agrees on all four figures — lines,
// hits, misses and partials.
//
// The Go half does not, because a profile describes blocks rather than lines,
// and going from one to the other takes a judgement call. A block covers every
// line it spans, minus the lines Codecov's uploader marks ignorable from the
// source — blank ones, comment-only ones, and ones holding nothing but a closing
// brace. That last rule is the uploader's rather than a guess, but the exact set
// it computes is not published and is reimplemented here approximately: measured
// against Codecov's own figures for this repository, the Go project percentage
// lands a couple of tenths of a point low. That is well inside the threshold
// below, and it moves both percentages the same way, which is the only thing the
// comparison reads.
//
// The diff runs against the working tree, so this is usable while writing rather
// than only before a push. A file git does not track yet appears in no diff at
// all: `git add -N` puts it in one, and untracked files are named in the output
// so that a missing one is visible rather than silently uncounted.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

func main() {
	name := flag.String("flag", "go", "the Codecov flag to judge: go or ui")
	base := flag.String("base", "", "the commit to diff against (default: the merge base with origin/main)")
	flag.Parse()

	lang, err := languageNamed(*name)
	if err != nil {
		fail(err)
	}

	measured, err := readReport(lang)
	if err != nil {
		fail(err)
	}

	against := *base
	if against == "" {
		if against, err = mergeBase(); err != nil {
			fail(err)
		}
	}

	diff, err := git("diff", "--unified=0", "--no-color", "--no-ext-diff", against)
	if err != nil {
		fail(err)
	}

	changed, err := changedLines(strings.NewReader(diff))
	if err != nil {
		fail(fmt.Errorf("reading the diff against %s: %w", against, err))
	}

	untracked, err := untrackedFiles()
	if err != nil {
		fail(err)
	}

	result := judge(lang, measured, changed, untracked)

	fmt.Print(render(lang, against, &result))

	if lang.enforced && result.stands != holds {
		os.Exit(1)
	}
}

// readReport opens the report a flag is measured from and closes it again, so
// that nothing this program defers is still outstanding when it exits.
func readReport(lang language) (lines, error) {
	report, err := os.Open(lang.report)
	if err != nil {
		return nil, fmt.Errorf("%w; run `mise run coverage` first", err)
	}

	measured, err := lang.parse(report)

	if closeErr := report.Close(); err == nil && closeErr != nil {
		err = closeErr
	}

	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", lang.report, err)
	}

	return measured, nil
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "patchcoverage: %v\n", err)
	os.Exit(1)
}

// verdict is what this makes of a patch: the status passes, the status fails,
// or the answer turns on the base's report and this cannot see it.
type verdict int

const (
	holds verdict = iota
	undecided
	breaks
)

// threshold is codecov.yml's 1%, in tenths of a point.
const threshold = 10

// status is what Codecov calls one measured line. A partial is its own thing
// rather than a covered line, because the ratio counts it as a miss.
type status int

const (
	missed status = iota
	partial
	covered
)

// lines is a coverage report, reduced to what a verdict reads: for every file in
// it, the status of every line it measured. A line absent from the map was not
// measured and is not part of any ratio.
type lines map[string]map[int]status

// counts is a set of lines and how many of them were fully covered.
type counts struct {
	covered int
	total   int
}

// language is one Codecov flag: which report answers for it, how that report
// names files, which changed files it is expected to measure, and whether its
// status blocks a merge.
type language struct {
	parse      func(io.Reader) (lines, error)
	measurable func(path string) bool
	name       string
	report     string
	enforced   bool
}

func languageNamed(name string) (language, error) {
	switch name {
	case "go":
		return language{
			name:       "go",
			report:     ".coverage/go.out",
			enforced:   true,
			parse:      parseProfile,
			measurable: measurableGo,
		}, nil
	case "ui":
		// Informational, as codecov.yml explains: the browser suite reaches code
		// the LCOV file cannot see, so this half reports and does not judge.
		return language{
			name:       "ui",
			report:     ".coverage/ui/lcov.info",
			enforced:   false,
			parse:      parseLCOV,
			measurable: measurableUI,
		}, nil
	}

	return language{}, fmt.Errorf("unknown flag %q: expected go or ui", name)
}

// measurableGo reports whether a path is one the Go profile should hold. Test
// files are the measurement rather than the subject and are instrumented into no
// profile, so a changed one is not a gap.
func measurableGo(path string) bool {
	if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
		return false
	}

	// The -coverpkg set in mise-tasks.toml, which is also the go flag's paths in
	// codecov.yml. dev/ is repository tooling and deliberately outside both.
	return strings.HasPrefix(path, "cmd/") || strings.HasPrefix(path, "internal/")
}

// measurableUI reports whether a path is one the LCOV file should hold, mirroring
// the include and exclude lists in internal/webui/app/vite.config.ts.
func measurableUI(path string) bool {
	const root = "internal/webui/app/src/"

	if !strings.HasPrefix(path, root) {
		return false
	}
	if !strings.HasSuffix(path, ".ts") && !strings.HasSuffix(path, ".tsx") {
		return false
	}
	if strings.HasSuffix(path, ".d.ts") || strings.Contains(path, ".test.") {
		return false
	}

	return !strings.HasPrefix(path, root+"test/") && path != root+"main.tsx"
}

// parseProfile reads a Go coverage profile into the status of every line the
// instrumented packages hold, skipping the lines the working tree shows to carry
// no code.
func parseProfile(in io.Reader) (lines, error) {
	return readProfile(in, (&workingTree{}).ignorable)
}

// ignorer reports whether a line carries no code, and so is measured by nobody.
type ignorer func(file string, line int) bool

// readProfile reads a profile into line statuses.
//
// A block is reported once per test binary that instrumented its package, so the
// same position arrives several times and is covered if any of them reached it.
// Only then are blocks projected onto lines: a block spanning several lines
// covers all of them, and a line held by both a covered block and an uncovered
// one is a partial. Doing it in the other order would make a block one binary
// never reached partial every line of a block another binary covered outright.
func readProfile(in io.Reader, skip ignorer) (lines, error) {
	blocks := make(map[string]bool)

	scanner := bufio.NewScanner(in)
	for number := 1; scanner.Scan(); number++ {
		text := scanner.Text()
		if text == "" || strings.HasPrefix(text, "mode:") {
			continue
		}

		fields := strings.Fields(text)
		if len(fields) != 3 {
			return nil, fmt.Errorf("line %d: expected 3 fields, got %d", number, len(fields))
		}

		executions, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: execution count: %w", number, err)
		}

		blocks[fields[0]] = blocks[fields[0]] || executions > 0
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading profile: %w", err)
	}

	measured := make(lines)

	for position, reached := range blocks {
		file, first, last, err := parsePosition(position)
		if err != nil {
			return nil, err
		}

		if measured[file] == nil {
			measured[file] = make(map[int]status)
		}

		for line := first; line <= last; line++ {
			if skip(file, line) {
				continue
			}

			measured[file][line] = merge(measured[file], line, reached)
		}
	}

	// A file whose every line was skipped is in no report and must not read as
	// one that exists but measures nothing.
	maps.DeleteFunc(measured, func(_ string, held map[int]status) bool { return len(held) == 0 })

	return measured, nil
}

// workingTree answers which lines carry no code, by reading the files as they
// are on disk. A file it cannot read — deleted since the profile was written —
// contributes no skips, so the block projection stays complete rather than
// silently losing a file.
type workingTree struct {
	read map[string][]string
}

// ignorable reports whether a source line is blank, a comment, or nothing but a
// closing brace. The rule is the one Codecov's uploader applies when it tells
// the service which lines of a file to leave out of a report.
func (tree *workingTree) ignorable(file string, line int) bool {
	if tree.read == nil {
		tree.read = make(map[string][]string)
	}

	text, cached := tree.read[file]
	if !cached {
		content, err := os.ReadFile(file) //nolint:gosec // a repository file, named by a coverage report
		if err == nil {
			text = strings.Split(string(content), "\n")
		}

		tree.read[file] = text
	}

	if line < 1 || line > len(text) {
		return false
	}

	trimmed := strings.TrimSpace(text[line-1])

	return trimmed == "" || trimmed == "}" || strings.HasPrefix(trimmed, "//")
}

// merge folds one block's verdict into what other blocks already said about the
// same line: agreement keeps the verdict, disagreement makes it a partial.
func merge(file map[int]status, line int, reached bool) status {
	reported := missed
	if reached {
		reported = covered
	}

	existing, seen := file[line]
	if !seen || existing == reported {
		return reported
	}

	return partial
}

// parsePosition splits a profile position — an import path, then
// ":line.column,line.column" — into the repository-relative file and the first
// and last line the block spans. The prefix it strips is codecov.yml's `fixes`
// entry for Go, which exists because a profile names files by import path.
func parsePosition(position string) (file string, first, last int, err error) {
	const module = "github.com/nobbs/domestique/"

	colon := strings.LastIndex(position, ":")
	if colon < 0 {
		return "", 0, 0, fmt.Errorf("block position %q names no file", position)
	}

	file = strings.TrimPrefix(position[:colon], module)

	span := strings.Split(position[colon+1:], ",")
	if len(span) != 2 {
		return "", 0, 0, fmt.Errorf("block position %q is not a span", position)
	}

	if first, err = lineOf(span[0]); err != nil {
		return "", 0, 0, fmt.Errorf("block position %q: %w", position, err)
	}

	if last, err = lineOf(span[1]); err != nil {
		return "", 0, 0, fmt.Errorf("block position %q: %w", position, err)
	}

	return file, first, last, nil
}

// lineOf takes the line out of a profile's "line.column" pair.
func lineOf(pair string) (int, error) {
	line, _, found := strings.Cut(pair, ".")
	if !found {
		return 0, fmt.Errorf("%q is not a line and column", pair)
	}

	number, err := strconv.Atoi(line)
	if err != nil {
		return 0, fmt.Errorf("line of %q: %w", pair, err)
	}

	return number, nil
}

// parseLCOV reads Vitest's LCOV file into the status of every line it measured.
//
// A DA record carries a line and how often it ran; a BRDA record carries one
// branch on a line and how often that branch was taken, with "-" for a branch
// under an expression that never evaluated. A line that ran but holds a branch
// nobody took is a partial, which is exactly how Codecov reads the same file.
//
// A BRDA line with no DA record beside it is measured all the same, which is
// easy to miss and worth 160 of this repository's 1739 UI lines: Codecov reads
// its status from the branches alone — every one untaken is a miss, some taken
// is a partial, all taken is a hit.
func parseLCOV(in io.Reader) (lines, error) {
	const root = "internal/webui/app/"

	measured := make(lines)

	var (
		file      string
		hits      = make(map[int]int)
		branches  = make(map[int]*branchCounts)
		endRecord = func() {
			if file == "" {
				return
			}

			measured[file] = make(map[int]status, len(hits))
			for line, count := range hits {
				measured[file][line] = ranLine(count, branches[line])
			}

			for line, taken := range branches {
				if _, held := hits[line]; !held {
					measured[file][line] = branchOnlyLine(taken)
				}
			}

			file, hits, branches = "", make(map[int]int), make(map[int]*branchCounts)
		}
	)

	scanner := bufio.NewScanner(in)
	for number := 1; scanner.Scan(); number++ {
		text := scanner.Text()

		switch {
		case strings.HasPrefix(text, "SF:"):
			endRecord()
			// codecov.yml's `fixes` entry for the UI: Vitest writes LCOV paths
			// relative to the UI project rather than to the repository.
			file = strings.TrimPrefix(text, "SF:")
			if strings.HasPrefix(file, "src/") {
				file = root + file
			}
		case strings.HasPrefix(text, "DA:"):
			line, count, err := parseDA(strings.TrimPrefix(text, "DA:"))
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", number, err)
			}

			hits[line] += count
		case strings.HasPrefix(text, "BRDA:"):
			line, taken, err := parseBRDA(strings.TrimPrefix(text, "BRDA:"))
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", number, err)
			}

			if branches[line] == nil {
				branches[line] = &branchCounts{}
			}

			if taken {
				branches[line].taken++
			} else {
				branches[line].untaken++
			}
		case text == "end_of_record":
			endRecord()
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading LCOV: %w", err)
	}

	endRecord()

	return measured, nil
}

// branchCounts is how the branches on one line came out.
type branchCounts struct {
	taken   int
	untaken int
}

// ranLine reads the status of a line a DA record measured.
func ranLine(count int, taken *branchCounts) status {
	switch {
	case count == 0:
		return missed
	case taken != nil && taken.untaken > 0:
		return partial
	default:
		return covered
	}
}

// branchOnlyLine reads the status of a line that carries branches and no DA
// record. Codecov measures it, so leaving it out understates the denominator.
func branchOnlyLine(taken *branchCounts) status {
	switch {
	case taken.taken == 0:
		return missed
	case taken.untaken > 0:
		return partial
	default:
		return covered
	}
}

// parseDA reads one "line,hits" record.
func parseDA(record string) (line, count int, err error) {
	fields := strings.Split(record, ",")
	if len(fields) < 2 {
		return 0, 0, fmt.Errorf("DA record %q is not a line and a count", record)
	}

	if line, err = strconv.Atoi(fields[0]); err != nil {
		return 0, 0, fmt.Errorf("DA record %q: %w", record, err)
	}

	if count, err = strconv.Atoi(fields[1]); err != nil {
		return 0, 0, fmt.Errorf("DA record %q: %w", record, err)
	}

	return line, count, nil
}

// parseBRDA reads one "line,block,branch,taken" record, where taken is a count
// or "-" for a branch whose expression never evaluated at all.
func parseBRDA(record string) (line int, taken bool, err error) {
	const fieldCount = 4

	fields := strings.Split(record, ",")
	if len(fields) != fieldCount {
		return 0, false, fmt.Errorf("BRDA record %q: expected %d fields, got %d", record, fieldCount, len(fields))
	}

	if line, err = strconv.Atoi(fields[0]); err != nil {
		return 0, false, fmt.Errorf("BRDA record %q: %w", record, err)
	}

	if fields[3] == "-" {
		return line, false, nil
	}

	count, err := strconv.Atoi(fields[3])
	if err != nil {
		return 0, false, fmt.Errorf("BRDA record %q: %w", record, err)
	}

	return line, count > 0, nil
}

// changedLines reads a unified diff into the lines each file gained. Only added
// lines are the patch: a line a change deleted is in no file to cover, and a line
// it left alone is the base's business.
func changedLines(diff io.Reader) (map[string][]int, error) {
	added := make(map[string][]int)

	var file string

	scanner := bufio.NewScanner(diff)
	// A hunk header is short, but a context line in a minified bundle is not, and
	// the scanner's default limit would stop the whole diff on one of them.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for number := 1; scanner.Scan(); number++ {
		text := scanner.Text()

		switch {
		case strings.HasPrefix(text, "+++ "):
			// /dev/null is a deleted file, which adds no line anywhere.
			file = ""
			if name := strings.TrimPrefix(text, "+++ "); name != "/dev/null" {
				file = strings.TrimPrefix(name, "b/")
			}
		case strings.HasPrefix(text, "@@ ") && file != "":
			first, count, err := parseHunk(text)
			if err != nil {
				return nil, fmt.Errorf("line %d: %w", number, err)
			}

			for line := first; line < first+count; line++ {
				added[file] = append(added[file], line)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading diff: %w", err)
	}

	return added, nil
}

// parseHunk reads the destination range out of "@@ -a,b +c,d @@". The count is
// omitted when the hunk is one line long, and is zero when the hunk only deletes.
func parseHunk(header string) (first, count int, err error) {
	_, rest, found := strings.Cut(header, "+")
	if !found {
		return 0, 0, fmt.Errorf("hunk header %q names no destination", header)
	}

	span, _, _ := strings.Cut(rest, " ")

	start, length, hasCount := strings.Cut(span, ",")

	if first, err = strconv.Atoi(start); err != nil {
		return 0, 0, fmt.Errorf("hunk header %q: %w", header, err)
	}

	if !hasCount {
		return first, 1, nil
	}

	if count, err = strconv.Atoi(length); err != nil {
		return 0, 0, fmt.Errorf("hunk header %q: %w", header, err)
	}

	return first, count, nil
}

// location is one uncovered added line, as a place to go and look.
type location struct {
	file string
	line int
}

// result is everything a verdict is read from.
type result struct {
	uncovered []location
	// Changed files the report was expected to hold and does not, and changed
	// files git does not track yet. Either is a line this did not count, and
	// saying so is what keeps a silent gap from reading as a pass.
	unmeasured []string
	untracked  []string
	patch      counts
	project    counts
	stands     verdict
}

// judge measures the patch against the report, and the report against itself.
func judge(lang language, measured lines, changed map[string][]int, untracked []string) result {
	var found result

	for _, file := range slices.Sorted(maps.Keys(measured)) {
		for _, reported := range measured[file] {
			found.project.total++
			if reported == covered {
				found.project.covered++
			}
		}
	}

	for _, file := range slices.Sorted(maps.Keys(changed)) {
		lines, reported := measured[file]
		if !reported {
			if lang.measurable(file) {
				found.unmeasured = append(found.unmeasured, file)
			}

			continue
		}

		for _, line := range changed[file] {
			reached, held := lines[line]
			if !held {
				continue
			}

			found.patch.total++
			if reached == covered {
				found.patch.covered++

				continue
			}

			found.uncovered = append(found.uncovered, location{file: file, line: line})
		}
	}

	for _, file := range untracked {
		if lang.measurable(file) {
			found.untracked = append(found.untracked, file)
		}
	}

	found.stands = stands(found.patch, found.project)

	return found
}

// stands reads the three-way verdict off the one measurement. See the package
// comment for why the middle case cannot be decided without the base's report.
func stands(patch, project counts) verdict {
	// A patch that measures nothing clears the gate, which is what Codecov does
	// with one too.
	if patch.total == 0 {
		return holds
	}

	switch measured := tenths(patch); {
	case measured >= tenths(project):
		return holds
	case measured < tenths(project)-threshold:
		return breaks
	default:
		return undecided
	}
}

// tenths is a coverage ratio in tenths of a percent, truncated — codecov.yml's
// `precision: 1` with `round: down`. Integers rather than a rounded float,
// because the verdict is a comparison and a comparison of floats that were each
// rounded on their way in is a comparison of something else.
func tenths(of counts) int {
	if of.total == 0 {
		return 0
	}

	return of.covered * 1000 / of.total
}

func render(lang language, base string, found *result) string {
	var out strings.Builder

	fmt.Fprintf(&out, "patch coverage (%s), against %s\n\n", lang.name, base)

	if found.patch.total == 0 {
		fmt.Fprintf(&out, "  no changed line is measured by this report\n")
	} else {
		fmt.Fprintf(&out, "  patch    %s  (%d/%d lines)\n", percentage(found.patch), found.patch.covered, found.patch.total)
	}

	fmt.Fprintf(&out, "  project  %s  (%d/%d lines)\n", percentage(found.project), found.project.covered, found.project.total)
	fmt.Fprintf(&out, "  needed   %s, or %s to be certain\n",
		tenthsString(tenths(found.project)-threshold), tenthsString(tenths(found.project)))
	fmt.Fprintf(&out, "  verdict  %s\n", verdictOf(lang, found))

	if len(found.uncovered) > 0 {
		fmt.Fprintf(&out, "\nuncovered added lines:\n")

		for _, at := range found.uncovered {
			fmt.Fprintf(&out, "  %s:%d\n", at.file, at.line)
		}
	}

	if len(found.unmeasured) > 0 {
		fmt.Fprintf(&out, "\nchanged and absent from the report, so counted nowhere\n(expected of a file holding only declarations, and a gap otherwise):\n")

		for _, file := range found.unmeasured {
			fmt.Fprintf(&out, "  %s\n", file)
		}
	}

	if len(found.untracked) > 0 {
		fmt.Fprintf(&out, "\nuntracked, so in no diff — `git add -N` to count them:\n")

		for _, file := range found.untracked {
			fmt.Fprintf(&out, "  %s\n", file)
		}
	}

	return out.String()
}

func verdictOf(lang language, found *result) string {
	switch {
	case found.stands == holds:
		return "pass"
	case found.stands == undecided && lang.enforced:
		return "too close to call — under the base by less than the threshold, so\n           this fails where codecov/patch/go may not"
	case found.stands == undecided:
		return "too close to call — informational, and blocks nothing"
	case lang.enforced:
		return "fail — this is what codecov/patch/go will say"
	default:
		return "short — informational, and blocks nothing"
	}
}

func percentage(of counts) string {
	if of.total == 0 {
		return "n/a"
	}

	return tenthsString(tenths(of))
}

func tenthsString(value int) string {
	return fmt.Sprintf("%d.%d%%", value/10, value%10)
}

func mergeBase() (string, error) {
	base, err := git("merge-base", "origin/main", "HEAD")
	if err != nil {
		return "", fmt.Errorf("%w; pass -base to name one yourself", err)
	}

	return strings.TrimSpace(base), nil
}

func untrackedFiles() ([]string, error) {
	out, err := git("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}

	return strings.FieldsFunc(out, func(r rune) bool { return r == '\n' }), nil
}

func git(args ...string) (string, error) {
	//nolint:gosec // the arguments are this program's own literals
	command := exec.CommandContext(context.Background(), "git", args...)

	var stderr strings.Builder

	command.Stderr = &stderr

	out, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}

	return string(out), nil
}
