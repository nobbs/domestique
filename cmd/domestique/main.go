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

	"github.com/nobbs/domestique/internal/cfaccess"
	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/elevation"
	"github.com/nobbs/domestique/internal/fit"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/oauth"
	"github.com/nobbs/domestique/internal/pushover"
	"github.com/nobbs/domestique/internal/schedule"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/surface"
	syncservice "github.com/nobbs/domestique/internal/sync"
	"github.com/nobbs/domestique/internal/veloplanner"
	"github.com/nobbs/domestique/internal/wahoo"
	"github.com/nobbs/domestique/internal/webui"
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
	// Surface enrichment is optional. An operator who clears the endpoint keeps
	// stage shapes off a third-party server, and the annotator stays nil, which
	// synchronization supports as a normal state.
	var annotator syncservice.Annotator
	if settings.Surface.OverpassURL != "" {
		overpass, overpassErr := surface.NewOverpass(&surface.Options{Endpoint: settings.Surface.OverpassURL})
		if overpassErr != nil {
			return fmt.Errorf("creating Overpass client: %w", overpassErr)
		}
		annotator = surface.NewAnnotator(overpass, store)
	}
	reconciler, err := syncservice.New(&syncservice.Options{TargetIDs: targetIDs, MaxDeletionsPerTarget: settings.Sync.MaxDeletionsPerTarget, AllowEmptySourceDeletion: settings.Sync.EmptySourceDeletion == config.EmptySourceDeletionAllow}, store, source, elevation.New(), fit.New(), destination, annotator)
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
	assets, err := webui.New()
	if err != nil {
		return fmt.Errorf("loading browser UI: %w", err)
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	// Cloudflare Access is the only gate, so this is not conditional: a service
	// that cannot verify an assertion has no way to authenticate anyone.
	verifier, err := cfaccess.New(&cfaccess.Options{
		TeamDomain: settings.Access.Cloudflare.TeamDomain,
		Audience:   settings.Access.Cloudflare.ApplicationAUD,
	})
	if err != nil {
		return fmt.Errorf("configuring Cloudflare Access verification: %w", err)
	}
	accessVerifier := httpapi.AccessVerifierFunc(
		func(ctx context.Context, assertion string) (string, error) {
			identity, identityErr := verifier.Verify(ctx, assertion)
			if identityErr != nil {
				return "", identityErr //nolint:wrapcheck // the gate discards the detail rather than reflecting it
			}

			return identity.Email, nil
		},
	)

	handler, err := httpapi.New(
		&httpapi.Options{
			TargetIDs:        targetIDs,
			TileStyleURL:     settings.WebUI.TileStyleURL,
			TileStyleURLDark: settings.WebUI.TileStyleURLDark,
			// The page links a stage back to the source route it was made from,
			// which is on the provider the library is read from.
			SourceBaseURL:  settings.VeloPlanner.BaseURL,
			AccessVerifier: accessVerifier,
			AccessEmail:    settings.Access.Cloudflare.AllowedEmail,
			// The Wahoo redirect URL is on the hostname a browser reaches this
			// service at, which is what makes it the origin a state-changing
			// request has to come from.
			BrowserOriginURL: settings.Wahoo.RedirectURL,
		},
		oauthService,
		store,
		// The HTTP surface names a phase; the reporter decides what running one
		// means. Manual triggers deliberately ignore the schedule switches.
		httpapi.SyncTriggerFunc(func(phase httpapi.SyncPhase) bool {
			switch phase {
			case httpapi.SyncPhaseSource:
				return reporter.TriggerPhase(runCtx, syncservice.PhaseSource)
			case httpapi.SyncPhaseTargets:
				return reporter.TriggerPhase(runCtx, syncservice.PhaseTargets)
			case httpapi.SyncPhaseAll:
				return reporter.Trigger(runCtx)
			}

			return false
		}),
		assets,
	)
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
