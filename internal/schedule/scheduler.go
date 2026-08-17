// Package schedule owns delayed, periodic execution of synchronization runs.
package schedule

import (
	"context"
	"errors"
	"time"

	"github.com/nobbs/domestique/internal/sync"
)

// Runner executes one synchronization attempt.
type Runner interface {
	Run(ctx context.Context) sync.Result
}

// Options configures delayed startup and periodic execution.
type Options struct {
	InitialDelay time.Duration
	Interval     time.Duration
}

// Scheduler starts one delayed run and then runs on an interval. It never
// starts concurrent work; each Runner invocation finishes before the next tick
// is considered.
type Scheduler struct {
	runner  Runner
	after   func(time.Duration) <-chan time.Time
	ticker  func(time.Duration) ticker
	options Options
}

type ticker interface {
	Chan() <-chan time.Time
	Stop()
}

// New creates an inert scheduler. Run starts its lifecycle under the caller's
// context; the constructor never starts a goroutine.
func New(options Options, runner Runner) (*Scheduler, error) {
	if runner == nil || options.InitialDelay <= 0 || options.Interval <= 0 {
		return nil, errors.New("scheduler runner, initial delay, and interval are required")
	}

	return &Scheduler{
		runner:  runner,
		options: options,
		after:   time.After,
		ticker:  newTicker,
	}, nil
}

// Run waits for the initial delay, invokes the runner, and continues invoking
// it at the configured interval until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	if !wait(ctx, s.after(s.options.InitialDelay)) {
		return
	}
	s.runner.Run(ctx)

	intervalTicker := s.ticker(s.options.Interval)
	defer intervalTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-intervalTicker.Chan():
			s.runner.Run(ctx)
		}
	}
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
