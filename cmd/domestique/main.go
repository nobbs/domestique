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
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/nobbs/domestique/internal/build"
	"github.com/nobbs/domestique/internal/cfaccess"
	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/elevation"
	"github.com/nobbs/domestique/internal/fit"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/oauth"
	"github.com/nobbs/domestique/internal/osmindex"
	"github.com/nobbs/domestique/internal/pushover"
	"github.com/nobbs/domestique/internal/readiness"
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
	notifier, err := pushover.New(&pushover.Options{BaseURL: settings.Notifications.Pushover.BaseURL, ApplicationToken: settings.Notifications.Pushover.ApplicationToken().Bytes(), UserKey: settings.Notifications.Pushover.UserKey().Bytes()})
	if err != nil {
		return fmt.Errorf("creating Pushover client: %w", err)
	}

	// Surface enrichment is optional. An operator who configures no region keeps
	// this host from downloading map extracts at all, and the annotator stays
	// nil, which synchronization supports as a normal state.
	var (
		annotator      syncservice.Annotator
		surfaceIndex   *osmindex.Current
		indexScheduler *schedule.Scheduler
	)
	if len(settings.Surface.Regions) > 0 {
		surfaceIndex, indexScheduler, err = startSurfaceIndex(ctx, settings, store, notifier)
		if err != nil {
			return err
		}
		defer func() {
			if closeErr := surfaceIndex.Close(); closeErr != nil {
				slog.Error("closing the surface index", "error", closeErr)
			}
		}()
		annotator = surface.NewAnnotator(surfaceIndex, store)
	}
	reconciler, err := syncservice.New(&syncservice.Options{TargetIDs: targetIDs, MaxDeletionsPerTarget: settings.Sync.MaxDeletionsPerTarget, AllowEmptySourceDeletion: settings.Sync.EmptySourceDeletion == config.EmptySourceDeletionAllow}, store, []syncservice.Source{source}, elevation.New(), fit.New(), destination, annotator)
	if err != nil {
		return fmt.Errorf("creating sync service: %w", err)
	}
	reporter, err := syncservice.NewReporter(reconciler, store, notifier, syncservice.SuccessNotification{
		Policy:   syncservice.SuccessPolicy(settings.Notifications.Success.Policy),
		Interval: settings.Notifications.Success.DigestInterval,
	}, settings.Sync.StaleAfter)
	if err != nil {
		return fmt.Errorf("creating sync reporter: %w", err)
	}
	// The reporter answers with a result nobody on this path consumes; the
	// scheduler wants a runner that answers with nothing.
	scheduler, err := schedule.New(
		schedule.Options{InitialDelay: settings.Sync.InitialDelay, Interval: settings.Sync.Interval},
		schedule.RunnerFunc(func(ctx context.Context) { _ = reporter.Run(ctx) }),
	)
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

	// Which source produced this binary, and which image is carrying it. The
	// revision is compiled in by CI; the image reference is what the deploying
	// host pinned, handed in through the environment because nothing inside an
	// image can know its own digest — the configuration layer takes it out of
	// that environment on the way past, since the prefix is its own. Only the
	// digest survives the read: the registry and repository in front of it stay
	// on the host.
	buildInfo := build.Current(settings.ImageReference)

	handler, err := httpapi.New(
		&httpapi.Options{
			TargetIDs: targetIDs,
			Basemaps:  basemapOptions(settings.WebUI.Basemaps),
			// The page links a stage back to the source route it was made from,
			// which is on the provider the library is read from.
			SourceBaseURL:    settings.VeloPlanner.BaseURL,
			BuildRevision:    buildInfo.Revision,
			BuildImageDigest: buildInfo.ImageDigest,
			AccessVerifier:   accessVerifier,
			AccessEmail:      settings.Access.Cloudflare.AllowedEmail,
			// The Wahoo redirect URL is on the hostname a browser reaches this
			// service at, which is what makes it the origin a state-changing
			// request has to come from.
			BrowserOriginURL: settings.Wahoo.RedirectURL,
			// What the page reports is the map build classifications are
			// actually being read from, not the one the state database last
			// wrote down — those differ exactly when a recorded build's file did
			// not survive to this start, which is the case worth seeing.
			SurfaceIndexFunc: func() (string, time.Time, bool) {
				if surfaceIndex == nil {
					return "", time.Time{}, false
				}
				metadata, ok := surfaceIndex.Metadata()

				return metadata.Generation, metadata.BuiltAt, ok
			},
			SourceStaleAfter: settings.Sync.StaleAfter,
		},
		oauthService,
		store,
		// The HTTP surface names a phase; the reporter decides what running one
		// means. Manual triggers deliberately ignore the schedule switches.
		httpapi.SyncFuncs{
			TriggerFunc: func(phase httpapi.SyncPhase) bool {
				switch phase {
				case httpapi.SyncPhaseSource:
					return reporter.TriggerPhase(runCtx, syncservice.PhaseSource)
				case httpapi.SyncPhaseTargets:
					return reporter.TriggerPhase(runCtx, syncservice.PhaseTargets)
				case httpapi.SyncPhaseAll:
					return reporter.Trigger(runCtx)
				}

				return false
			},
			TriggerTargetFunc: func(targetID string) bool {
				return reporter.TriggerTarget(runCtx, targetID)
			},
			// Two halves of one answer: the reporter knows what is running now,
			// and the scheduler knows what it is still holding back.
			ActivityFunc: func() httpapi.SyncActivityState {
				phase, running := reporter.Running()
				startsAt, _ := scheduler.NextRunAt()

				return httpapi.SyncActivityState{
					StartsAt: startsAt,
					Phase:    httpapi.SyncPhase(phase),
					Running:  running,
				}
			},
		},
		assets,
	)
	if err != nil {
		return fmt.Errorf("creating HTTP handler: %w", err)
	}

	// Readiness answers on its own listener, with only local state behind it.
	// The served listener is what Tailscale Serve and the tunnel front, so a
	// probe on a second port is how readiness stays off the authenticated public
	// surface without a second gate to get wrong.
	readinessHandler, err := readiness.New(targetIDs, store)
	if err != nil {
		return fmt.Errorf("creating readiness handler: %w", err)
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
	readinessServer := &http.Server{
		Addr:              settings.HTTP.ReadinessAddress,
		Handler:           readinessHandler,
		IdleTimeout:       httpIdleTimeout,
		MaxHeaderBytes:    httpMaximumHeaderBytes,
		ReadHeaderTimeout: httpReadHeaderTimeout,
		ReadTimeout:       httpReadTimeout,
		WriteTimeout:      httpWriteTimeout,
	}
	schedulers := []schedulerRunner{scheduler}
	if indexScheduler != nil {
		schedulers = append(schedulers, indexScheduler)
	}

	return serve(runCtx, cancel, server, readinessServer, schedulers, reporter)
}

// startSurfaceIndex prepares the surface index and the schedule that rebuilds
// it.
//
// The last build's file is opened before anything else runs, so a restart serves
// classifications immediately instead of going blind until the next rebuild
// lands. A file the state database remembers but that is no longer on disk is
// not an error: the holder simply starts empty and the first build fills it.
//
// The index lives beside the state database because that is the one directory a
// deployment is guaranteed to have made durable. Giving it a setting of its own
// would only invite an operator to point it at the container's /tmp, which is a
// tmpfs — the exact memory this build works to stay out of.
func startSurfaceIndex(
	ctx context.Context,
	settings *config.Settings,
	store *sqlite.Store,
	notifier *pushover.Client,
) (*osmindex.Current, *schedule.Scheduler, error) {
	directory := filepath.Dir(settings.State.DatabasePath)
	lastBuiltAt, generation, err := store.SurfaceIndexBuild(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("reading the last surface index build: %w", err)
	}

	current := osmindex.NewCurrent()
	switch index, found, loadErr := osmindex.Load(ctx, directory, generation); {
	case loadErr != nil:
		slog.Warn("the last surface index could not be reopened", "error", loadErr)
	case found:
		current.Swap(index)
		slog.Info("surface index loaded",
			"generation", index.Metadata().Generation,
			"built_at", index.Metadata().BuiltAt.Format(time.RFC3339),
		)
	}

	runner, err := osmindex.NewRunner(osmindex.Options{
		Regions:     settings.Surface.Regions,
		Directory:   directory,
		MemoryLimit: osmindex.DefaultMemoryLimit,
	}, current, store, notifier)
	if err != nil {
		return nil, nil, fmt.Errorf("creating the surface index builder: %w", err)
	}

	scheduler, err := schedule.New(schedule.Options{
		InitialDelay: osmindex.InitialDelay(
			lastBuiltAt, settings.Surface.RebuildInterval, osmindex.InitialBuildDelay, time.Now().UTC(),
		),
		Interval: settings.Surface.RebuildInterval,
	}, runner)
	if err != nil {
		return nil, nil, fmt.Errorf("creating the surface index scheduler: %w", err)
	}

	return current, scheduler, nil
}

type schedulerRunner interface {
	Run(context.Context)
}

type manualSyncWaiter interface {
	Wait()
}

// serve runs both HTTP listeners and every scheduled job under one cancellation
// scope. It waits for cancelled runs to finish before its caller can close the
// durable state and the index files those runs may still be using.
//
// Either listener failing stops the process. A readiness probe that cannot bind
// is a deployment whose health checking silently answers nothing, which is worse
// than a service that refuses to start and says why.
func serve(
	ctx context.Context,
	cancel context.CancelFunc,
	server, readinessServer *http.Server,
	schedulers []schedulerRunner,
	manualSync manualSyncWaiter,
) error {
	defer cancel()
	serverErrors := make(chan error, 2)
	go func() { serverErrors <- server.ListenAndServe() }()
	go func() { serverErrors <- readinessServer.ListenAndServe() }()
	var scheduled sync.WaitGroup
	for _, scheduler := range schedulers {
		scheduled.Go(func() { scheduler.Run(ctx) })
	}

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
	shutdownErr := errors.Join(server.Shutdown(shutdownCtx), readinessServer.Shutdown(shutdownCtx))
	scheduled.Wait()
	manualSync.Wait()
	if shutdownErr != nil {
		shutdownErr = fmt.Errorf("shutting down HTTP: %w", shutdownErr)
	}

	return errors.Join(servingErr, shutdownErr)
}

// basemapOptions restates the configured basemaps in the HTTP surface's own
// type, so that package keeps depending on nothing but its options.
func basemapOptions(basemaps []config.Basemap) []httpapi.Basemap {
	options := make([]httpapi.Basemap, len(basemaps))
	for index, basemap := range basemaps {
		options[index] = httpapi.Basemap{
			Name:            basemap.Name,
			StyleURL:        basemap.StyleURL,
			StyleURLDark:    basemap.StyleURLDark,
			DarkCartography: basemap.DarkCartography,
		}
	}

	return options
}
