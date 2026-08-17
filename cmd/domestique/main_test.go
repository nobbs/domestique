package main

import (
	"context"
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
	server := &http.Server{
		Addr:              "127.0.0.1:0",
		Handler:           http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		ReadHeaderTimeout: time.Second,
	}
	result := make(chan error, 1)
	go func() { result <- serve(ctx, server, scheduler) }()

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

type blockingScheduler struct {
	started   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
}

func (s *blockingScheduler) Run(ctx context.Context) {
	close(s.started)
	<-ctx.Done()
	close(s.cancelled)
	<-s.release
}
