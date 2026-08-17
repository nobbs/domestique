package schedule

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/sync"
)

func TestSchedulerRunsAfterDelayAndOnEachTick(t *testing.T) {
	initial := make(chan time.Time, 1)
	ticks := make(chan time.Time, 2)
	runner := newFakeRunner()
	scheduler, err := New(Options{InitialDelay: time.Second, Interval: time.Hour}, runner)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
	if !fakeTicker.stopped {
		t.Error("Scheduler.Run() did not stop the ticker")
	}
}

func TestSchedulerStopsBeforeInitialRunWhenCancelled(t *testing.T) {
	initial := make(chan time.Time)
	runner := newFakeRunner()
	scheduler, err := New(Options{InitialDelay: time.Second, Interval: time.Hour}, runner)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
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
	if got := runner.calls.Load(); got != 0 {
		t.Errorf("runner calls = %d, want 0", got)
	}
}

type fakeRunner struct {
	called chan struct{}
	calls  atomic.Int32
}

func newFakeRunner() *fakeRunner {
	return &fakeRunner{called: make(chan struct{}, 4)}
}

func (r *fakeRunner) Run(context.Context) sync.Result {
	r.calls.Add(1)
	r.called <- struct{}{}

	return sync.Result{}
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
