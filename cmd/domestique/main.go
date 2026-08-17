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

const shutdownTimeout = 15 * time.Second

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
	targetIDs := make([]string, 0, 2)
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
	reconciler, err := syncservice.New(&syncservice.Options{TargetIDs: targetIDs, MaxDeletionsPerTarget: settings.Sync.MaxDeletionsPerTarget, AllowEmptySourceDeletion: settings.Sync.EmptySourceDeletion == config.EmptySourceDeletionAllow}, store, source, fit.New(), destination)
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
	handler, err := httpapi.New(settings.Access.TailnetUserLogin, oauthService, store)
	if err != nil {
		return fmt.Errorf("creating HTTP handler: %w", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	server := &http.Server{Addr: settings.HTTP.ListenAddress, Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	serverErrors := make(chan error, 1)
	go func() { serverErrors <- server.ListenAndServe() }()
	go scheduler.Run(runCtx)
	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serving HTTP: %w", err)
		}
	case <-ctx.Done():
	}
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutting down HTTP: %w", err)
	}

	return nil
}
