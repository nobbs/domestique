package osmindex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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
	options := Options{Regions: []string{"europe/germany"}, Directory: t.TempDir()}

	_, err := NewRunner(options, nil, &fakeState{}, &fakeNotifier{})
	require.Error(t, err, "NewRunner() without an index holder")

	_, err = NewRunner(options, NewCurrent(), nil, &fakeNotifier{})
	require.Error(t, err, "NewRunner() without state")

	_, err = NewRunner(options, NewCurrent(), &fakeState{}, nil)
	require.Error(t, err, "NewRunner() without a notifier")

	_, err = NewRunner(Options{Directory: t.TempDir()}, NewCurrent(), &fakeState{}, &fakeNotifier{})
	require.Error(t, err, "NewRunner() with no regions; the caller decides not to build at all")

	_, err = NewRunner(Options{Regions: []string{"europe/germany"}}, NewCurrent(), &fakeState{}, &fakeNotifier{})
	require.Error(t, err, "NewRunner() without a directory")
}

// A failed build is worth one message. The same message every week afterwards is
// noise an operator learns to ignore, which is how the message that mattered
// gets missed.
func TestRunNotifiesOnceForARunOfFailures(t *testing.T) {
	server := failingUpstream(t)
	state := &fakeState{}
	notifier := &fakeNotifier{}
	runner := testRunner(t, server, NewCurrent(), state, notifier)

	runner.Run(t.Context())
	require.Len(t, notifier.sent, 1, "the first failure was not announced")
	assert.Contains(t, notifier.sent[0], "surface index", "the message does not say what failed")
	assert.NotContains(t, notifier.sent[0], server.URL, "the message carries an upstream address")

	runner.Run(t.Context())
	assert.Len(t, notifier.sent, 1, "a second failure inside the window was announced again")

	// A failure after the window has passed is worth saying again.
	state.notifiedAt = state.notifiedAt.Add(-failureNotificationSuppression - time.Hour)
	runner.Run(t.Context())
	assert.Len(t, notifier.sent, 2, "a failure after the suppression window stayed silent")
}

// A cancelled build is a shutdown, not a fault. Announcing it would send a
// notification every time the service is restarted.
func TestRunStaysSilentWhenTheBuildIsCancelled(t *testing.T) {
	server := failingUpstream(t)
	notifier := &fakeNotifier{}
	runner := testRunner(t, server, NewCurrent(), &fakeState{}, notifier)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	runner.Run(ctx)

	assert.Empty(t, notifier.sent, "a shutdown was announced as a failure")
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
	notifier := &fakeNotifier{}
	runner := testRunnerIn(t, directory, server, current, state, notifier)
	runner.Run(t.Context())

	assert.Empty(t, notifier.sent, "an unchanged check was announced as a failure")
	assert.Equal(t, generation, state.generation, "the recorded generation")
	assert.False(t, state.builtAt.IsZero(), "the check was not written down")
	assert.Equal(t, generation, current.Generation(), "the live index was replaced by an unchanged check")
}

// Two builds at once is the one way this service could exhaust its host: each
// one reads a region's whole road network.
func TestRunRefusesToBuildTwiceAtOnce(t *testing.T) {
	release := make(chan struct{})
	var concurrent, started sync.WaitGroup
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if requests.Add(1) == 1 {
			started.Done()
			<-release
		}
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	runner := testRunner(t, server, NewCurrent(), &fakeState{}, &fakeNotifier{})

	started.Add(1)
	concurrent.Go(func() { runner.Run(t.Context()) })
	started.Wait()

	// The second call returns rather than waiting on the first, and reaches
	// nothing while it is held.
	runner.Run(t.Context())
	assert.Equal(t, int64(1), requests.Load(), "a second build started while the first was still running")

	close(release)
	concurrent.Wait()
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

func testRunner(t *testing.T, server *httptest.Server, current *Current, state State, notifier Notifier) *Runner {
	t.Helper()

	return testRunnerIn(t, t.TempDir(), server, current, state, notifier)
}

func testRunnerIn(
	t *testing.T, directory string, server *httptest.Server,
	current *Current, state State, notifier Notifier,
) *Runner {
	t.Helper()

	runner, err := NewRunner(Options{
		Regions:   []string{"europe/germany"},
		Directory: directory,
		BaseURL:   server.URL,
		Client:    server.Client(),
	}, current, state, notifier)
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

type fakeNotifier struct {
	err  error
	sent []string
}

func (n *fakeNotifier) Send(_ context.Context, title, message string) error {
	if n.err != nil {
		return n.err
	}
	n.sent = append(n.sent, title+": "+message)

	return nil
}

// A build that succeeds has to leave the service using what it built. Every
// other test here stops short of that: a failure never opens an index, and an
// unchanged check deliberately keeps the one already live. This is the run that
// installs, and it is the only one that proves the pieces fit — that Build's
// file is one Open accepts, that the generation written down is the generation
// now being served, and that a crash's leftovers go with it.
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
	notifier := &fakeNotifier{}

	testRunnerIn(t, directory, server, current, state, notifier).Run(t.Context())

	require.Empty(t, notifier.sent, "a successful build was announced as a failure")
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
