// Package schedule owns delayed, periodic execution of synchronization runs.
package schedule

import (
	"context"
	"errors"
	"sync/atomic"
	"time"
)

// Runner executes one scheduled attempt at whatever the scheduler drives.
//
// It reports nothing. What a run produced belongs to whoever ran it — the sync
// reporter records and notifies on its own results, and the index builder
// installs its own output — and a timer that took an outcome it could not act on
// would only tie this package to one caller's vocabulary.
type Runner interface {
	Run(ctx context.Context)
}

// RunnerFunc adapts a plain function to Runner, which is what lets a runner that
// does report something be scheduled without changing its signature.
type RunnerFunc func(ctx context.Context)

// Run calls f.
func (f RunnerFunc) Run(ctx context.Context) { f(ctx) }

// Options configures delayed startup and periodic execution.
type Options struct {
	// IntervalFunc supersedes Interval where the gap is a setting an operator
	// edits while the service runs. It is read again after every run, so an
	// edit takes effect at the next gap rather than at the next restart. New
	// wraps a constant Interval into one when it is unset.
	IntervalFunc func() time.Duration
	InitialDelay time.Duration
	// Interval is the gap between runs. It is fixed for the life of the
	// scheduler.
	Interval time.Duration
}

// Scheduler starts one delayed run and then runs on an interval. It never
// starts concurrent work; each Runner invocation finishes before the next tick
// is considered.
type Scheduler struct {
	runner   Runner
	interval func() time.Duration
	after    func(time.Duration) <-chan time.Time
	ticker   func(time.Duration) ticker
	now      func() time.Time
	// startsAt holds the instant the first run is due, and only while it is
	// still due. Nil is the ordinary state of a scheduler that has started.
	startsAt atomic.Pointer[time.Time]
	options  Options
}

type ticker interface {
	Chan() <-chan time.Time
	Stop()
}

// New creates an inert scheduler. Run starts its lifecycle under the caller's
// context; the constructor never starts a goroutine.
func New(options Options, runner Runner) (*Scheduler, error) {
	interval := options.IntervalFunc
	if interval == nil {
		constant := options.Interval
		interval = func() time.Duration { return constant }
	}
	if runner == nil || options.InitialDelay <= 0 || interval() <= 0 {
		return nil, errors.New("scheduler runner, initial delay, and interval are required")
	}

	return &Scheduler{
		runner:   runner,
		interval: interval,
		options:  options,
		after:    time.After,
		ticker:   newTicker,
		now:      time.Now,
	}, nil
}

// Run waits for the initial delay, invokes the runner, and continues invoking
// it at the configured interval until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	startsAt := s.now().UTC().Add(s.options.InitialDelay)
	s.startsAt.Store(&startsAt)
	started := wait(ctx, s.after(s.options.InitialDelay))
	s.startsAt.Store(nil)
	if !started {
		return
	}
	s.runner.Run(ctx)

	interval := s.interval()
	intervalTicker := s.ticker(interval)
	defer func() { intervalTicker.Stop() }()
	for {
		select {
		case <-ctx.Done():
			return
		case <-intervalTicker.Chan():
			s.runner.Run(ctx)
			// A gap that has been edited is honoured from here on. The ticker is
			// replaced rather than reconfigured because a ticker whose period
			// changed under it would still be counting the old one.
			if edited := s.interval(); edited != interval && edited > 0 {
				intervalTicker.Stop()
				interval = edited
				intervalTicker = s.ticker(interval)
			}
		}
	}
}

// NextRunAt reports the instant the first run is being held until, while the
// scheduler is still holding it.
//
// Only the initial delay answers here. The interval between runs is this
// service's cadence rather than work being held back, and a scheduler that
// called every gap between hourly runs "waiting" would leave a status page —
// and anything polling one — with nothing to settle on.
func (s *Scheduler) NextRunAt() (time.Time, bool) {
	startsAt := s.startsAt.Load()
	if startsAt == nil {
		return time.Time{}, false
	}

	return *startsAt, true
}

type timeTicker struct {
	ticker *time.Ticker
}

func newTicker(interval time.Duration) ticker {
	return &timeTicker{ticker: time.NewTicker(interval)}
}

func (t *timeTicker) Chan() <-chan time.Time {
	return t.ticker.C
}

func (t *timeTicker) Stop() {
	t.ticker.Stop()
}

func wait(ctx context.Context, signal <-chan time.Time) bool {
	select {
	case <-ctx.Done():
		return false
	case <-signal:
		return true
	}
}
