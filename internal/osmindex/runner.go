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
// failureNotificationSuppression is how long one silences the next.
const (
	failureNotificationCategory    = "surface_index:build"
	failureNotificationSuppression = 7 * 24 * time.Hour
)

// generationName matches an index filename this package wrote. Exact, because it
// decides whether a file in the index directory may be removed.
var generationName = regexp.MustCompile(`^` + indexPrefix + `[0-9a-f]{12}\` + indexSuffix + `$`)

// State remembers what the last build produced, so the schedule survives a
// restart and the annotator knows which index its classifications used.
type State interface {
	// SurfaceIndexBuild reports when the last build finished and which generation
	// it produced. Never built reports the zero time and an empty generation.
	SurfaceIndexBuild(ctx context.Context) (builtAt time.Time, generation string, err error)
	RecordSurfaceIndexBuild(ctx context.Context, builtAt time.Time, generation string) error
	LastFailureNotification(ctx context.Context, category string) (sentAt time.Time, found bool, err error)
	RecordFailureNotification(ctx context.Context, category string, sentAt time.Time) error
}

// Notifier delivers already-safe notification text.
type Notifier interface {
	Send(ctx context.Context, title, message string) error
}

// Runner rebuilds the index on a schedule and installs what it builds. It
// reports nothing to its scheduler: a build is preprocessing, so a failed one
// changes what the service knows rather than whether it works.
type Runner struct {
	current  *Current
	state    State
	notifier Notifier
	regions  func() []string
	now      func() time.Time
	options  Options
	running  atomic.Bool
}

// NewRunner creates a runner over an index holder, durable state, and a
// notifier. Regions are a function because they are an editable setting;
// Options.Regions is ignored. An empty list builds nothing and is supported.
func NewRunner(
	options Options, regions func() []string, current *Current, state State, notifier Notifier,
) (*Runner, error) {
	if current == nil || state == nil || notifier == nil || regions == nil {
		return nil, errors.New("osmindex: index holder, state, notifier, and regions are required")
	}
	if options.Directory == "" {
		return nil, errors.New("osmindex: an index directory is required")
	}

	return &Runner{
		options: options, regions: regions, current: current, state: state, notifier: notifier, now: time.Now,
	}, nil
}

// Run performs one scheduled rebuild. Concurrent runs are refused rather than
// queued: a build reads a region's whole road network into memory.
func (r *Runner) Run(ctx context.Context) {
	if !r.running.CompareAndSwap(false, true) {
		slog.Warn("surface index build skipped", "reason", "a build is already running")

		return
	}
	defer r.running.Store(false)

	// No region is the operator's switch for leaving stages unclassified. The
	// schedule still runs, so turning regions on needs no restart.
	options := r.options
	options.Regions = r.regions()
	if len(options.Regions) == 0 {
		slog.Info("surface index build skipped", "reason", "no regions are configured")

		return
	}

	startedAt := r.now().UTC()
	result, err := Build(ctx, options, r.current.Generation())
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
		"regions", len(options.Regions),
		"built_in", finishedAt.Sub(startedAt).Round(time.Second),
	)
}

// record writes down what the run established, for an unchanged check as well as
// a rebuild, so the next delay counts from the last upstream look.
func (r *Runner) record(ctx context.Context, finishedAt time.Time, generation string) {
	if err := r.state.RecordSurfaceIndexBuild(ctx, finishedAt, generation); err != nil {
		slog.Error("recording the surface index build", "error", err)
	}
}

// notifyFailure announces a failed build, subject to the suppression window. The
// message carries no error detail: those name upstream URLs and local paths,
// and the log already has them.
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

// prune removes indexes from earlier builds. Swap deletes the file it replaces,
// so this only finds what a crash left behind — each hundreds of megabytes. Only
// names this package writes, and only after a build has succeeded.
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

// InitialBuildDelay is the floor InitialDelay never goes below, so a restart puts
// the service on its feet before a memory-hungry job behind it.
const InitialBuildDelay = 5 * time.Minute

// InitialDelay is how long to wait before the first build of a process. Reading
// when the last build finished turns the interval into time between builds
// rather than time since this process started. A build already overdue still
// waits the floor.
func InitialDelay(lastBuiltAt time.Time, interval, floor time.Duration, now time.Time) time.Duration {
	if lastBuiltAt.IsZero() {
		return floor
	}
	if delay := interval - now.Sub(lastBuiltAt); delay > floor {
		return delay
	}

	return floor
}
