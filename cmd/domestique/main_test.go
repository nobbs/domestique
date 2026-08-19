package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestServeWaitsForCancelledSchedulerBeforeReturning(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	scheduler := &blockingScheduler{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
	}
	server := newTestServer()
	readinessServer := newTestServer()
	result := make(chan error, 1)
	go func() { result <- serve(ctx, cancel, server, readinessServer, scheduler, &blockingManualSync{}) }()

	<-scheduler.started
	cancel()
	<-scheduler.cancelled

	select {
	case err := <-result:
		t.Fatalf("serve returned before scheduler finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(scheduler.release)
	if err := <-result; err != nil {
		t.Fatalf("serve returned error: %v", err)
	}
}

// Both listeners are shut down together, so a serve that returns has stopped
// answering the probe as well as the served surface.
func TestServeStopsBothListeners(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	scheduler := &blockingScheduler{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
	}
	server, readinessServer := newTestServer(), newTestServer()
	result := make(chan error, 1)
	go func() { result <- serve(ctx, cancel, server, readinessServer, scheduler, &blockingManualSync{}) }()

	<-scheduler.started
	cancel()
	<-scheduler.cancelled
	close(scheduler.release)
	if err := <-result; err != nil {
		t.Fatalf("serve returned error: %v", err)
	}

	for name, server := range map[string]*http.Server{"served": server, "readiness": readinessServer} {
		if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			t.Errorf("%s listener error = %v, want %v", name, err, http.ErrServerClosed)
		}
	}
}

func newTestServer() *http.Server {
	return &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		ReadHeaderTimeout: time.Second,
	}
}

type blockingScheduler struct {
	started   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
}

type blockingManualSync struct{}

func (*blockingManualSync) Wait() {}

func (s *blockingScheduler) Run(ctx context.Context) {
	close(s.started)
	<-ctx.Done()
	close(s.cancelled)
	<-s.release
}
