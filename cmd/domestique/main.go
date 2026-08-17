// Package main hosts the Domestique service binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/elevation"
	"github.com/nobbs/domestique/internal/fit"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/oauth"
	"github.com/nobbs/domestique/internal/pushover"
	"github.com/nobbs/domestique/internal/schedule"
	"github.com/nobbs/domestique/internal/sqlite"
	syncservice "github.com/nobbs/domestique/internal/sync"
	"github.com/nobbs/domestique/internal/veloplanner"
	"github.com/nobbs/domestique/internal/wahoo"
)

const (
	shutdownTimeout        = 15 * time.Second
	httpIdleTimeout        = 60 * time.Second
	httpReadHeaderTimeout  = 10 * time.Second
	httpReadTimeout        = 15 * time.Second
	httpWriteTimeout       = 75 * time.Second
	httpMaximumHeaderBytes = 8 << 10
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	if err := run(ctx); err != nil {
		stop()
		slog.Error("domestique stopped", "error", err)
		os.Exit(1)
	}
	stop()
}

func run(ctx context.Context) error {
	settings, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading configuration: %w", err)
	}
	store, err := sqlite.Open(ctx, settings.State.DatabasePath, settings.State.EncryptionKey())
	if err != nil {
		return fmt.Errorf("opening state: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			slog.Error("closing state store", "error", closeErr)
		}
	}()
	targetIDs := make([]string, 0, len(settings.Wahoo.Targets()))
	for _, target := range settings.Wahoo.Targets() {
		targetIDs = append(targetIDs, target.ID)
	}
	if ensureErr := store.EnsureTargets(ctx, targetIDs); ensureErr != nil {
		return fmt.Errorf("initializing targets: %w", ensureErr)
	}

	source, err := veloplanner.New(&veloplanner.Options{BaseURL: settings.VeloPlanner.BaseURL, Email: settings.VeloPlanner.Email().Bytes(), Password: settings.VeloPlanner.Password().Bytes()})
	if err != nil {
		return fmt.Errorf("creating VeloPlanner client: %w", err)
	}
	destination, err := wahoo.New(&wahoo.Options{APIBaseURL: settings.Wahoo.APIBaseURL, OAuthBaseURL: settings.Wahoo.OAuthBaseURL, ClientID: settings.Wahoo.ClientID, RedirectURL: settings.Wahoo.RedirectURL, ClientSecret: settings.Wahoo.ClientSecret().Bytes()})
	if err != nil {
		return fmt.Errorf("creating Wahoo client: %w", err)
	}
	oauthService, err := oauth.New(store, destination)
	if err != nil {
		return fmt.Errorf("creating oauth service: %w", err)
	}
	reconciler, err := syncservice.New(&syncservice.Options{TargetIDs: targetIDs, MaxDeletionsPerTarget: settings.Sync.MaxDeletionsPerTarget, AllowEmptySourceDeletion: settings.Sync.EmptySourceDeletion == config.EmptySourceDeletionAllow}, store, source, elevation.New(), fit.New(), destination)
	if err != nil {
		return fmt.Errorf("creating sync service: %w", err)
	}
	notifier, err := pushover.New(&pushover.Options{ApplicationToken: settings.Notifications.Pushover.ApplicationToken().Bytes(), UserKey: settings.Notifications.Pushover.UserKey().Bytes()})
	if err != nil {
		return fmt.Errorf("creating Pushover client: %w", err)
	}
	reporter, err := syncservice.NewReporter(reconciler, store, notifier)
	if err != nil {
		return fmt.Errorf("creating sync reporter: %w", err)
	}
	scheduler, err := schedule.New(schedule.Options{InitialDelay: settings.Sync.InitialDelay, Interval: settings.Sync.Interval}, reporter)
	if err != nil {
		return fmt.Errorf("creating scheduler: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	handler, err := httpapi.New(settings.Access.TailnetUserLogin, targetIDs, oauthService, store, httpapi.SyncTriggerFunc(func() bool {
		return reporter.Trigger(runCtx)
	}))
	if err != nil {
		return fmt.Errorf("creating HTTP handler: %w", err)
	}

	server := &http.Server{
		Addr:              settings.HTTP.ListenAddress,
		Handler:           handler,
		IdleTimeout:       httpIdleTimeout,
		MaxHeaderBytes:    httpMaximumHeaderBytes,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
	}
	return serve(runCtx, cancel, server, scheduler, reporter)
}

type schedulerRunner interface {
	Run(context.Context)
}

type manualSyncWaiter interface {
	Wait()
}

// serve runs HTTP and scheduled synchronization under one cancellation scope.
// It waits for a cancelled synchronization run before its caller can close the
// durable state that the run may still be using.
func serve(ctx context.Context, cancel context.CancelFunc, server *http.Server, scheduler schedulerRunner, manualSync manualSyncWaiter) error {
	defer cancel()
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	schedulerDone := make(chan struct{})
	go func() {
		defer close(schedulerDone)
		scheduler.Run(ctx)
	}()

	var servingErr error
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			servingErr = fmt.Errorf("serving HTTP: %w", err)
		}
	case <-ctx.Done():
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	shutdownErr := server.Shutdown(shutdownCtx)
	<-schedulerDone
	manualSync.Wait()
	if shutdownErr != nil {
		shutdownErr = fmt.Errorf("shutting down HTTP: %w", shutdownErr)
	}

	return errors.Join(servingErr, shutdownErr)
}
