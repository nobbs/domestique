package schedule

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerRunsAfterDelayAndOnEachTick(t *testing.T) {
	initial := make(chan time.Time, 1)
	ticks := make(chan time.Time, 2)
	runner := newFakeRunner()
	scheduler, err := New(Options{InitialDelay: time.Second, Interval: time.Hour}, runner)
	require.NoError(t, err)
	fakeTicker := &fakeTicker{ticks: ticks}
	scheduler.after = func(time.Duration) <-chan time.Time { return initial }
	scheduler.ticker = func(time.Duration) ticker { return fakeTicker }

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()
	initial <- time.Now()
	waitForRun(t, runner.called)
	ticks <- time.Now()
	waitForRun(t, runner.called)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Scheduler.Run() did not stop after cancellation")
	}
	assert.True(t, fakeTicker.stopped, "Scheduler.Run() did not stop the ticker")
}

// A gap an operator edits is honoured after the next run rather than at the
// next restart, which is the whole reason it is a function.
func TestSchedulerPicksUpAnEditedInterval(t *testing.T) {
	initial := make(chan time.Time, 1)
	ticks := make(chan time.Time, 1)
	interval := &atomic.Int64{}
	interval.Store(int64(time.Hour))
	runner := newFakeRunner()
	scheduler, err := New(Options{
		InitialDelay: time.Second,
		IntervalFunc: func() time.Duration { return time.Duration(interval.Load()) },
	}, runner)
	require.NoError(t, err)

	created := make(chan time.Duration, 2)
	scheduler.after = func(time.Duration) <-chan time.Time { return initial }
	scheduler.ticker = func(period time.Duration) ticker {
		created <- period

		return &fakeTicker{ticks: ticks}
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go scheduler.Run(ctx)

	initial <- time.Now()
	waitForRun(t, runner.called)
	require.Equal(t, time.Hour, <-created, "the first ticker's period")

	interval.Store(int64(15 * time.Minute))
	ticks <- time.Now()
	waitForRun(t, runner.called)
	assert.Equal(t, 15*time.Minute, <-created, "the ticker created after the edit")
}

func TestSchedulerStopsBeforeInitialRunWhenCancelled(t *testing.T) {
	initial := make(chan time.Time)
	runner := newFakeRunner()
	scheduler, err := New(Options{InitialDelay: time.Second, Interval: time.Hour}, runner)
	require.NoError(t, err)
	scheduler.after = func(time.Duration) <-chan time.Time { return initial }

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Scheduler.Run() did not stop after cancellation")
	}
	assert.Zero(t, runner.calls.Load(), "the runner ran despite the cancellation arriving first")
}

// A first run held back by the initial delay is work waiting rather than a
// service with nothing to do, and nothing else can tell the two apart.
func TestSchedulerReportsTheHeldBackFirstRun(t *testing.T) {
	initial := make(chan time.Time, 1)
	waiting := make(chan struct{})
	runner := newFakeRunner()
	scheduler, err := New(Options{InitialDelay: time.Minute, Interval: time.Hour}, runner)
	require.NoError(t, err)
	startedAt := time.Date(2026, time.August, 20, 6, 0, 0, 0, time.UTC)
	scheduler.now = func() time.Time { return startedAt }
	scheduler.after = func(time.Duration) <-chan time.Time {
		close(waiting)

		return initial
	}
	scheduler.ticker = func(time.Duration) ticker { return &fakeTicker{ticks: make(chan time.Time)} }
	_, held := scheduler.NextRunAt()
	assert.False(t, held, "NextRunAt() held a run before Run() started")

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go scheduler.Run(ctx)

	<-waiting
	nextRunAt, held := scheduler.NextRunAt()
	assert.True(t, held, "NextRunAt() reported nothing while the first run was waiting")
	assert.Equal(t, startedAt.Add(time.Minute), nextRunAt, "NextRunAt()")

	initial <- startedAt
	waitForRun(t, runner.called)
	// The interval between runs is this service's cadence, not work held back.
	_, held = scheduler.NextRunAt()
	assert.False(t, held, "NextRunAt() still held a run once the first one had started")
}

type fakeRunner struct {
	called chan struct{}
	calls  atomic.Int32
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{called: make(chan struct{}, 4)}
}

func (r *fakeRunner) Run(context.Context) {
	r.calls.Add(1)
	r.called <- struct{}{}
}

type fakeTicker struct {
	ticks   <-chan time.Time
	stopped bool
}

func (t *fakeTicker) Chan() <-chan time.Time {
	return t.ticks
}

func (t *fakeTicker) Stop() {
	t.stopped = true
}

func waitForRun(t *testing.T, called <-chan struct{}) {
	t.Helper()
	select {
	case <-called:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not invoke runner")
	}
}
