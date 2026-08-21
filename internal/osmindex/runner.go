package osmindex

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync/atomic"
	"time"
)

// failureNotificationCategory names this runner's notifications, and
// failureNotificationSuppression is how long one of them silences the next. A
// weekly job that has started failing is worth one message; the same message
// every week afterwards is noise an operator learns to ignore.
const (
	failureNotificationCategory    = "surface_index:build"
	failureNotificationSuppression = 7 * 24 * time.Hour
)

// generationName matches an index filename this package wrote. It is deliberately
// exact — twelve lowercase hex characters between the fixed prefix and suffix —
// because it is what decides whether a file in the index directory may be
// removed.
var generationName = regexp.MustCompile(`^` + indexPrefix + `[0-9a-f]{12}\` + indexSuffix + `$`)

// State remembers what the last build produced, so the schedule survives a
// restart and the annotator knows which index its cached classifications were
// measured against.
type State interface {
	// SurfaceIndexBuild reports when the last build finished and which
	// generation it produced. A service that has never built reports the zero
	// time and an empty generation.
	SurfaceIndexBuild(ctx context.Context) (builtAt time.Time, generation string, err error)
	RecordSurfaceIndexBuild(ctx context.Context, builtAt time.Time, generation string) error
	LastFailureNotification(ctx context.Context, category string) (sentAt time.Time, found bool, err error)
	RecordFailureNotification(ctx context.Context, category string, sentAt time.Time) error
}

// Notifier delivers already-safe notification text.
type Notifier interface {
	Send(ctx context.Context, title, message string) error
}

// Runner rebuilds the index on a schedule and installs what it builds.
//
// It reports nothing to its scheduler. A build is preprocessing: a route whose
// surface is not yet known is served without one, so a build that fails changes
// what the service knows rather than whether it works, and there is no run
// outcome for a caller to act on.
type Runner struct {
	current  *Current
	state    State
	notifier Notifier
	now      func() time.Time
	options  Options
	running  atomic.Bool
}

// NewRunner creates a runner over an index holder, durable state, and a
// notifier.
func NewRunner(options Options, current *Current, state State, notifier Notifier) (*Runner, error) {
	if current == nil || state == nil || notifier == nil {
		return nil, errors.New("osmindex: index holder, state, and notifier are required")
	}
	if len(options.Regions) == 0 {
		return nil, errors.New("osmindex: at least one region is required")
	}
	if options.Directory == "" {
		return nil, errors.New("osmindex: an index directory is required")
	}

	return &Runner{options: options, current: current, state: state, notifier: notifier, now: time.Now}, nil
}

// Run performs one scheduled rebuild.
//
// Concurrent runs are refused rather than queued. A build reads a region's whole
// road network into memory, and two at once is the one way this service could
// exhaust its host.
func (r *Runner) Run(ctx context.Context) {
	if !r.running.CompareAndSwap(false, true) {
		slog.Warn("surface index build skipped", "reason", "a build is already running")

		return
	}
	defer r.running.Store(false)

	startedAt := r.now().UTC()
	result, err := Build(ctx, r.options, r.current.Generation())
	if err != nil {
		// A cancelled build is a shutdown, not a fault. Announcing it would send
		// a notification every time the service is restarted.
		if ctx.Err() != nil {
			return
		}
		slog.Error("surface index build failed", "error", err)
		r.notifyFailure(ctx)

		return
	}

	finishedAt := r.now().UTC()
	if result.Unchanged {
		slog.Info("surface index is current",
			"generation", result.Generation,
			"checked_in", finishedAt.Sub(startedAt).Round(time.Millisecond),
		)
		r.record(ctx, finishedAt, result.Generation)

		return
	}

	index, err := Open(ctx, result.Path)
	if err != nil {
		slog.Error("surface index build produced an unreadable index", "error", err)
		removeFile(result.Path)
		r.notifyFailure(ctx)

		return
	}
	r.current.Swap(index)
	r.record(ctx, finishedAt, result.Generation)
	r.prune(result.Generation)

	slog.Info("surface index rebuilt",
		"generation", result.Generation,
		"regions", len(r.options.Regions),
		"built_in", finishedAt.Sub(startedAt).Round(time.Second),
	)
}

// record writes down what the run established. It runs for an unchanged check as
// well as for a rebuild, so the next start counts its delay from the last time
// the upstream was actually looked at.
func (r *Runner) record(ctx context.Context, finishedAt time.Time, generation string) {
	if err := r.state.RecordSurfaceIndexBuild(ctx, finishedAt, generation); err != nil {
		slog.Error("recording the surface index build", "error", err)
	}
}

// notifyFailure announces a failed build, subject to the suppression window.
//
// The message carries no error detail. A failure reaches a phone through a
// third-party service, and the errors here name upstream URLs and local paths;
// the log already has them, and the log stays on the host.
func (r *Runner) notifyFailure(ctx context.Context) {
	now := r.now().UTC()
	lastSentAt, found, err := r.state.LastFailureNotification(ctx, failureNotificationCategory)
	if err != nil || (found && now.Sub(lastSentAt) < failureNotificationSuppression) {
		return
	}
	if err := r.notifier.Send(ctx, "Domestique surface index failed",
		"the scheduled surface index rebuild did not complete; routes keep their last known surfaces",
	); err != nil {
		return
	}
	if err := r.state.RecordFailureNotification(ctx, failureNotificationCategory, now); err != nil {
		slog.Error("recording the surface index failure notification", "error", err)
	}
}

// prune removes indexes from earlier builds.
//
// Swap deletes the file it replaces, so this only ever finds what a crash left
// behind — but each one is hundreds of megabytes on a host with a few gigabytes
// free, so nothing may accumulate. Only names this package writes are considered,
// and only after a build has succeeded.
func (r *Runner) prune(keep string) {
	entries, err := os.ReadDir(r.options.Directory)
	if err != nil {
		slog.Error("reading the surface index directory", "error", err)

		return
	}
	keepName := filepath.Base(IndexPath(r.options.Directory, keep))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || name == keepName || !generationName.MatchString(name) {
			continue
		}
		if err := os.Remove(filepath.Join(r.options.Directory, name)); err != nil {
			slog.Error("removing a superseded surface index", "error", err)
		}
	}
}

// InitialBuildDelay is the floor InitialDelay never goes below. A build that is
// already overdue when the process starts still waits this long, so a restart
// puts the service on its feet before it puts a memory-hungry job behind it.
const InitialBuildDelay = 5 * time.Minute

// InitialDelay is how long to wait before the first build of a process.
//
// The scheduler counts from process start, which on its own means a service
// deployed several times a day never reaches a weekly interval — it would
// rebuild on every deploy or, with a delay long enough to prevent that, never
// rebuild at all. Reading when the last build finished turns the interval into
// what it reads as: time between builds, not time since this process happened to
// start.
//
// A build that is already overdue still waits the floor rather than running
// immediately, so a restart does not put a memory-hungry job in front of the
// service coming up.
func InitialDelay(lastBuiltAt time.Time, interval, floor time.Duration, now time.Time) time.Duration {
	if lastBuiltAt.IsZero() {
		return floor
	}
	if delay := interval - now.Sub(lastBuiltAt); delay > floor {
		return delay
	}

	return floor
}
