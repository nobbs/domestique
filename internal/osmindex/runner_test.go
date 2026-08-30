package osmindex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The scheduler counts from process start, so a service deployed several times a
// day would either rebuild on every deploy or, with a delay long enough to stop
// that, never rebuild at all. Counting from the last build is what turns the
// interval into time between builds.
func TestInitialDelay(t *testing.T) {
	const (
		interval = 7 * 24 * time.Hour
		floor    = 5 * time.Minute
	)
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		lastBuiltAt time.Time
		name        string
		want        time.Duration
	}{
		{name: "never built", want: floor},
		{name: "built just now", lastBuiltAt: now, want: interval},
		{name: "built two days ago", lastBuiltAt: now.Add(-48 * time.Hour), want: interval - 48*time.Hour},
		{name: "already overdue", lastBuiltAt: now.Add(-8 * 24 * time.Hour), want: floor},
		{name: "due within the floor", lastBuiltAt: now.Add(-interval + time.Minute), want: floor},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, InitialDelay(test.lastBuiltAt, interval, floor, now))
		})
	}
}

func TestNewRunnerRequiresItsCollaborators(t *testing.T) {
	options := Options{Directory: t.TempDir()}
	regions := staticRegions("europe/germany")

	_, err := NewRunner(options, regions, nil, &fakeState{})
	require.Error(t, err, "NewRunner() without an index holder")

	_, err = NewRunner(options, regions, NewCurrent(), nil)
	require.Error(t, err, "NewRunner() without state")

	_, err = NewRunner(options, nil, NewCurrent(), &fakeState{})
	require.Error(t, err, "NewRunner() without a region list")

	_, err = NewRunner(Options{}, regions, NewCurrent(), &fakeState{})
	require.Error(t, err, "NewRunner() without a directory")
}

// staticRegions pins the list one test builds from, standing in for the live
// read a running service does.
func staticRegions(regions ...string) func() []string {
	return func() []string { return regions }
}

// No region is how classification is switched off, and it is a state the
// schedule keeps running in: an operator naming their first region must not
// have to restart the service for the build to become possible.
func TestRunBuildsNothingWhileNoRegionIsConfigured(t *testing.T) {
	state := &fakeState{}
	runner, err := NewRunner(Options{Directory: t.TempDir()}, staticRegions(), NewCurrent(), state)
	require.NoError(t, err, "NewRunner()")

	outcome, err := runner.Run(t.Context())
	require.NoError(t, err, "Run()")
	assert.Equal(t, NoRegions, outcome, "Run() outcome")
	assert.True(t, state.builtAt.IsZero(), "a run with no regions recorded a build")
}

// A cancelled build is a shutdown, not a fault. Announcing it would send a
// notification every time the service is restarted.
func TestRunStaysSilentWhenTheBuildIsCancelled(t *testing.T) {
	server := failingUpstream(t)
	runner := testRunner(t, server, NewCurrent(), &fakeState{})

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := runner.Run(ctx)

	require.Error(t, err, "a cancelled build reported no error")
}

// The next start counts its delay from the last time the upstream was actually
// looked at, so a check that found nothing changed still has to be written down.
func TestRunRecordsAnUnchangedBuild(t *testing.T) {
	digest := digestOf([]byte("an extract"))
	server := checksumOnlyUpstream(t, digest)
	generation := generationOf(map[string]string{"europe/germany": digest})

	directory := t.TempDir()
	current := NewCurrent()
	index, err := Open(t.Context(), writeTestIndex(t, directory, generation))
	require.NoError(t, err)
	current.Swap(index)
	t.Cleanup(func() { assert.NoError(t, current.Close()) })

	state := &fakeState{}
	runner := testRunnerIn(t, directory, server, current, state)
	outcome, err := runner.Run(t.Context())

	require.NoError(t, err, "Run()")
	assert.Equal(t, Unchanged, outcome, "Run() outcome")
	assert.Equal(t, generation, state.generation, "the recorded generation")
	assert.False(t, state.builtAt.IsZero(), "the check was not written down")
	assert.Equal(t, generation, current.Generation(), "the live index was replaced by an unchanged check")
}

// Swap deletes the file it replaces, so this only ever finds what a crash left
// behind — but each one is hundreds of megabytes on a host with a few gigabytes
// free, so nothing may accumulate.
func TestPruneRemovesOnlyRetiredIndexes(t *testing.T) {
	directory := t.TempDir()
	keep := writeTestIndex(t, directory, "aaaaaaaaaaaa")
	stale := writeTestIndex(t, directory, "bbbbbbbbbbbb")

	// Nothing this package wrote, and so nothing it may remove.
	bystanders := []string{
		"state.db",
		"europe_germany-latest.osm.pbf",
		"surface-.sqlite",
		"surface-not-hex-here.sqlite",
		"surface-aaaaaaaaaaaa.sqlite.bak",
		"backup-surface-cccccccccccc.sqlite",
	}
	for _, name := range bystanders {
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), []byte("x"), 0o600))
	}

	runner := &Runner{options: Options{Directory: directory}}
	runner.prune("aaaaaaaaaaaa")

	assert.FileExists(t, keep, "the live index was pruned")
	assert.NoFileExists(t, stale, "a retired index was left on disk")
	for _, name := range bystanders {
		assert.FileExists(t, filepath.Join(directory, name), "prune removed a file it did not write")
	}
}

func testRunner(t *testing.T, server *httptest.Server, current *Current, state State) *Runner {
	t.Helper()

	return testRunnerIn(t, t.TempDir(), server, current, state)
}

func testRunnerIn(
	t *testing.T, directory string, server *httptest.Server,
	current *Current, state State,
) *Runner {
	t.Helper()

	runner, err := NewRunner(Options{
		Directory: directory,
		BaseURL:   server.URL,
		Client:    server.Client(),
	}, staticRegions("europe/germany"), current, state)
	require.NoError(t, err, "NewRunner()")

	return runner
}

func failingUpstream(t *testing.T) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	return server
}

// checksumOnlyUpstream publishes a digest and refuses the extract itself, so a
// test that downloads anything fails rather than hanging on a body it has no
// fixture for.
func checksumOnlyUpstream(t *testing.T, digest string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, checksumSuffix) {
			writer.WriteHeader(http.StatusInternalServerError)

			return
		}
		_, err := fmt.Fprintf(writer, "%s  germany-latest.osm.pbf\n", digest)
		assert.NoError(t, err, "writing the checksum")
	}))
	t.Cleanup(server.Close)

	return server
}

type fakeState struct {
	recordErr  error
	builtAt    time.Time
	notifiedAt time.Time
	generation string
}

func (s *fakeState) SurfaceIndexBuild(context.Context) (time.Time, string, error) {
	return s.builtAt, s.generation, nil
}

func (s *fakeState) RecordSurfaceIndexBuild(_ context.Context, builtAt time.Time, generation string) error {
	if s.recordErr != nil {
		return s.recordErr
	}
	s.builtAt, s.generation = builtAt, generation

	return nil
}

func (s *fakeState) LastFailureNotification(context.Context, string) (time.Time, bool, error) {
	return s.notifiedAt, !s.notifiedAt.IsZero(), nil
}

func (s *fakeState) RecordFailureNotification(_ context.Context, _ string, sentAt time.Time) error {
	s.notifiedAt = sentAt

	return nil
}

// A build that succeeds has to leave the service using what it built. Every other
// test here stops short: a failure never opens an index, and an unchanged check
// keeps the one already live. This proves Build's file is one Open accepts, that
// the generation written down is the one being served, and that leftovers go.
func TestRunInstallsWhatItBuilds(t *testing.T) {
	extract := testExtract(t)
	digest := digestOf(extract)
	server := extractServer(t, map[string]string{
		"/europe/germany-latest.osm.pbf":     string(extract),
		"/europe/germany-latest.osm.pbf.md5": digest + "  germany-latest.osm.pbf\n",
	})

	// An index no Current holds, which is what a build interrupted partway
	// leaves behind: Swap deletes what it replaces, so only a crash gets here.
	directory := t.TempDir()
	orphan := writeTestIndex(t, directory, "cccccccccccc")

	current := NewCurrent()
	t.Cleanup(func() { assert.NoError(t, current.Close(), "Close()") })
	state := &fakeState{}

	outcome, err := testRunnerIn(t, directory, server, current, state).Run(t.Context())

	require.NoError(t, err, "Run()")
	assert.Equal(t, Rebuilt, outcome, "Run() outcome")
	generation := generationOf(map[string]string{"europe/germany": digest})
	assert.Equal(t, generation, current.Generation(), "the generation now being served")
	assert.Equal(t, generation, state.generation, "the generation written down")
	assert.False(t, state.builtAt.IsZero(), "the build was not written down")

	ways, err := current.Ways(t.Context(), []route.Point{{Longitude: 8.3015, Latitude: 49.9015}})
	require.NoError(t, err, "Ways()")
	assert.NotEmpty(t, ways, "the installed index answers for the region it was built from")

	assert.NoFileExists(t, orphan, "an index from an earlier build was left on disk")
	assert.FileExists(t, IndexPath(directory, generation), "the index the runner installed")
}
