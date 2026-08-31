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
	"github.com/nobbs/domestique/internal/openmeteo"
	"github.com/nobbs/domestique/internal/osmindex"
	"github.com/nobbs/domestique/internal/pushover"
	"github.com/nobbs/domestique/internal/readiness"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/surface"
	syncservice "github.com/nobbs/domestique/internal/sync"
	"github.com/nobbs/domestique/internal/task"
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
	// Everything an operator edits from the web UI. Only the listeners, the
	// identity gate and the state file come from the configuration file; each
	// component below reads the rest through a function, so an edit reaches the
	// next run or the next request instead of the next restart.
	runtimeSettings, err := runtimeconfig.Load(ctx, store)
	if err != nil {
		return fmt.Errorf("loading runtime settings: %w", err)
	}
	if missing := runtimeSettings.Missing(); len(missing) > 0 {
		slog.Warn("settings are still needed before anything will run", "settings", missing)
	}

	destination := newWahooProvider(runtimeSettings, settings.HTTP.BrowserOriginURL)
	oauthService, err := oauth.New(store, destination)
	if err != nil {
		return fmt.Errorf("creating oauth service: %w", err)
	}
	notifier, err := pushover.New(&pushover.Options{
		BaseURL: func() string { return runtimeSettings.Values().Notifications.PushoverBaseURL },
		ApplicationToken: func() []byte {
			return runtimeSettings.Secret(runtimeconfig.SecretPushoverApplicationToken).Bytes()
		},
		UserKey: func() []byte { return runtimeSettings.Secret(runtimeconfig.SecretPushoverUserKey).Bytes() },
	})
	if err != nil {
		return fmt.Errorf("creating Pushover client: %w", err)
	}
	// There is no API key to configure: the free forecast endpoint needs none.
	weatherProvider, err := openmeteo.New(&openmeteo.Options{})
	if err != nil {
		return fmt.Errorf("creating Open-Meteo client: %w", err)
	}
	// Adapted rather than passed directly: httpapi.Weather is kept to
	// primitive types so that package never imports this adapter.
	weather := httpapi.WeatherFunc(func(
		ctx context.Context, latitudes, longitudes []float64, from, to time.Time,
	) ([]httpapi.WeatherSeries, error) {
		hourlies, forecastErr := weatherProvider.Forecast(ctx, weatherCoordinates(latitudes, longitudes), from, to)
		if forecastErr != nil {
			return nil, forecastErr //nolint:wrapcheck // the httpapi boundary discards the detail rather than reflecting it
		}

		return weatherSeriesOf(hourlies), nil
	})

	// Surface enrichment is built whether or not a region is named, since naming
	// the first one is an edit rather than a restart. With no regions the holder
	// stays empty, every build returns without work, and stages are served without
	// a surface.
	surfaceIndex, indexTask, err := startSurfaceIndex(ctx, settings, runtimeSettings, store, notifier)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := surfaceIndex.Close(); closeErr != nil {
			slog.Error("closing the surface index", "error", closeErr)
		}
	}()
	annotator := surface.NewAnnotator(surfaceIndex, store)

	// Ride model prediction is equally optional; no rider figure is ever guessed.
	// A file that will not load is reported here and again by the run that needs
	// it, rather than keeping the service from starting: the setting naming it is
	// edited through a page this process must be up to serve.
	rideModel := newRideModelProvider(store)
	if reloadErr := rideModel.reload(ctx, runtimeSettings); reloadErr != nil {
		slog.Error("the ride model could not be loaded", "error", reloadErr)
	}
	reconciler, err := syncservice.New(&syncservice.Options{
		TargetIDs: destination.targetIDs,
		Sources:   func() ([]syncservice.Source, error) { return sources(runtimeSettings) },
		AllowEmptySourceDeletion: func() bool {
			return runtimeSettings.Values().Sync.AllowEmptySourceDeletion
		},
	}, store, elevation.New(), fit.New(), destination, annotator, predictorFor(rideModel, runtimeSettings))
	if err != nil {
		return fmt.Errorf("creating sync service: %w", err)
	}
	reporter, err := syncservice.NewReporter(reconciler, store, notifier, func() syncservice.Notifications {
		values := runtimeSettings.Values()

		return syncservice.Notifications{
			Enabled: values.Notifications.Enabled,
			Success: syncservice.SuccessNotification{
				Policy:   syncservice.SuccessPolicy(values.Notifications.Policy),
				Interval: values.Notifications.DigestInterval,
			},
			StaleAfter: values.Sync.StaleAfter,
		}
	})
	if err != nil {
		return fmt.Errorf("creating sync reporter: %w", err)
	}
	tasks := task.NewManager()
	for _, definition := range append(inventoryTasks(reporter, runtimeSettings), indexTask) {
		if registerErr := tasks.Register(definition); registerErr != nil {
			return fmt.Errorf("registering background tasks: %w", registerErr)
		}
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

	// Which source produced this binary, and which image carries it. The revision
	// is compiled in by CI; the image reference is what the deploying host pinned,
	// handed in through the environment because nothing inside an image knows its
	// own digest. Only the digest survives the read.
	buildInfo := build.Current(settings.ImageReference)

	handler, err := httpapi.New(
		&httpapi.Options{
			Settings:         runtimeSettings,
			BuildRevision:    buildInfo.Revision,
			BuildImageDigest: buildInfo.ImageDigest,
			AccessVerifier:   accessVerifier,
			AccessEmail:      settings.Access.Cloudflare.AllowedEmail,
			// Cloudflare Access serves this path on the hostname it fronts, and
			// this binary is the one that is only ever deployed behind it — so
			// this is where knowing that belongs. It is relative because the
			// session being ended is the one on the origin the page came from.
			AccessSignOutURL: "/cdn-cgi/access/logout",
			BrowserOriginURL: settings.HTTP.BrowserOriginURL,
			// What the page reports is the map build classifications are
			// actually being read from, not the one the state database last
			// wrote down — those differ exactly when a recorded build's file did
			// not survive to this start, which is the case worth seeing.
			SurfaceIndexFunc: func() (string, time.Time, bool) {
				metadata, ok := surfaceIndex.Metadata()

				return metadata.Generation, metadata.BuiltAt, ok
			},
			RideModelValidationFunc: rideModel.validationView,
		},
		oauthService,
		store,
		syncSurface(runCtx, tasks, reporter, destination.RateLimit),
		assets,
		weather,
	)
	if err != nil {
		return fmt.Errorf("creating HTTP handler: %w", err)
	}

	// Readiness answers on its own listener, with only local state behind it.
	// The served listener is what Tailscale Serve and the tunnel front, so a
	// probe on a second port is how readiness stays off the authenticated public
	// surface without a second gate to get wrong.
	readinessHandler, err := readiness.New(destination.targetIDs, store)
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
	return serve(runCtx, cancel, server, readinessServer, tasks)
}

// startSurfaceIndex prepares the surface index and the schedule that rebuilds it.
// The last build's file is opened first, so a restart serves classifications
// immediately. A file the state database remembers but that is gone from disk is
// not an error. The index lives beside the state database, the one directory a
// deployment is guaranteed to have made durable.
func startSurfaceIndex(
	ctx context.Context,
	settings *config.Settings,
	runtimeSettings *runtimeconfig.Current,
	store *sqlite.Store,
	notifier *pushover.Client,
) (*osmindex.Current, task.Definition, error) {
	directory := filepath.Dir(settings.State.DatabasePath)
	lastBuiltAt, generation, err := store.SurfaceIndexBuild(ctx)
	if err != nil {
		return nil, task.Definition{}, fmt.Errorf("reading the last surface index build: %w", err)
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
		Directory:   directory,
		MemoryLimit: osmindex.DefaultMemoryLimit,
	}, func() []string {
		return runtimeSettings.Values().Surface.Regions
	}, current, store, notifier)
	if err != nil {
		return nil, task.Definition{}, fmt.Errorf("creating the surface index builder: %w", err)
	}

	return current, surfaceIndexTask(runner, runtimeSettings, lastBuiltAt), nil
}

// backgroundTasks is the task layer as this file uses it: run everything
// scheduled, and wait for everything triggered.
type backgroundTasks interface {
	Run(context.Context)
	Wait()
}

// serve runs both HTTP listeners and every scheduled job under one cancellation
// scope, waiting for cancelled runs to finish before its caller closes the state
// and index files they may still use. Either listener failing stops the process.
func serve(
	ctx context.Context,
	cancel context.CancelFunc,
	server, readinessServer *http.Server,
	tasks backgroundTasks,
) error {
	defer cancel()
	serverErrors := make(chan error, 2)
	go func() { serverErrors <- server.ListenAndServe() }()
	go func() { serverErrors <- readinessServer.ListenAndServe() }()
	var scheduled sync.WaitGroup
	scheduled.Go(func() { tasks.Run(ctx) })

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
	tasks.Wait()
	if shutdownErr != nil {
		shutdownErr = fmt.Errorf("shutting down HTTP: %w", shutdownErr)
	}

	return errors.Join(servingErr, shutdownErr)
}

// weatherCoordinates pairs the httpapi boundary's parallel latitude/longitude
// slices into the coordinate type openmeteo.Client.Forecast expects.
func weatherCoordinates(latitudes, longitudes []float64) []openmeteo.Coordinate {
	at := make([]openmeteo.Coordinate, len(latitudes))
	for i := range latitudes {
		at[i] = openmeteo.Coordinate{Latitude: latitudes[i], Longitude: longitudes[i]}
	}

	return at
}

// weatherSeriesOf converts openmeteo's hourly series into httpapi's own
// vocabulary, so that package never imports this adapter.
func weatherSeriesOf(hourlies []openmeteo.Hourly) []httpapi.WeatherSeries {
	series := make([]httpapi.WeatherSeries, len(hourlies))
	for i := range hourlies {
		series[i] = httpapi.WeatherSeries{
			Time:                            hourlies[i].Time,
			TemperatureCelsius:              hourlies[i].TemperatureCelsius,
			ApparentTemperatureCelsius:      hourlies[i].ApparentTemperatureCelsius,
			PrecipitationMillimetres:        hourlies[i].PrecipitationMillimetres,
			PrecipitationProbabilityPercent: hourlies[i].PrecipitationProbabilityPercent,
			WindSpeedKMH:                    hourlies[i].WindSpeedKMH,
			WindDirectionDegrees:            hourlies[i].WindDirectionDegrees,
			WeatherCode:                     hourlies[i].WeatherCode,
		}
	}

	return series
}
