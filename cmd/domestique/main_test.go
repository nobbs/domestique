package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/openmeteo"
	"github.com/nobbs/domestique/internal/pushover"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/sqlite"
)

func TestSourceBaseURLsIncludesOnlyConfiguredSources(t *testing.T) {
	t.Parallel()

	both := sourceBaseURLs(&config.Settings{
		VeloPlanner: &config.VeloPlanner{BaseURL: "https://veloplanner.example.test"},
		Komoot:      &config.Komoot{BaseURL: "https://komoot.example.test"},
	})
	assert.Equal(t, map[route.Provider]string{
		route.ProviderVeloPlanner: "https://veloplanner.example.test",
		route.ProviderKomoot:      "https://komoot.example.test",
	}, both)

	veloPlannerOnly := sourceBaseURLs(&config.Settings{
		VeloPlanner: &config.VeloPlanner{BaseURL: "https://veloplanner.example.test"},
	})
	assert.Equal(t, map[route.Provider]string{route.ProviderVeloPlanner: "https://veloplanner.example.test"}, veloPlannerOnly)

	komootOnly := sourceBaseURLs(&config.Settings{
		Komoot: &config.Komoot{BaseURL: "https://komoot.example.test"},
	})
	assert.Equal(t, map[route.Provider]string{route.ProviderKomoot: "https://komoot.example.test"}, komootOnly)
}

func TestWeatherCoordinatesPairsParallelSlices(t *testing.T) {
	t.Parallel()

	at := weatherCoordinates([]float64{50.11, 50.25}, []float64{8.68, 8.51})
	assert.Equal(t, []openmeteo.Coordinate{
		{Latitude: 50.11, Longitude: 8.68},
		{Latitude: 50.25, Longitude: 8.51},
	}, at)
}

func TestWeatherSeriesOfConvertsEveryField(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	series := weatherSeriesOf([]openmeteo.Hourly{
		{
			Time:                            []time.Time{now},
			TemperatureCelsius:              []float64{18.4},
			ApparentTemperatureCelsius:      []float64{17.1},
			PrecipitationMillimetres:        []float64{0.2},
			PrecipitationProbabilityPercent: []float64{10},
			WindSpeedKMH:                    []float64{12.3},
			WindDirectionDegrees:            []float64{240},
			WeatherCode:                     []int{1},
		},
	})
	require.Len(t, series, 1)
	assert.Equal(t, httpapi.WeatherSeries{
		Time:                            []time.Time{now},
		TemperatureCelsius:              []float64{18.4},
		ApparentTemperatureCelsius:      []float64{17.1},
		PrecipitationMillimetres:        []float64{0.2},
		PrecipitationProbabilityPercent: []float64{10},
		WindSpeedKMH:                    []float64{12.3},
		WindDirectionDegrees:            []float64{240},
		WeatherCode:                     []int{1},
	}, series[0])
}

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
	go func() {
		result <- serve(ctx, cancel, server, readinessServer, []schedulerRunner{scheduler}, &blockingManualSync{})
	}()

	<-scheduler.started
	cancel()
	<-scheduler.cancelled

	select {
	case err := <-result:
		t.Fatalf("serve returned before scheduler finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(scheduler.release)
	require.NoError(t, <-result)
}

// Synchronization and the surface index rebuild are two independent schedules,
// and serve owns the shutdown of both. A serve that waited for only the first
// would close the state database and the index files while the other was still
// reading them.
func TestServeWaitsForEverySchedulerBeforeReturning(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	first := &blockingScheduler{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
	}
	second := &blockingScheduler{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
	}
	result := make(chan error, 1)
	go func() {
		result <- serve(
			ctx, cancel, newTestServer(), newTestServer(),
			[]schedulerRunner{first, second}, &blockingManualSync{},
		)
	}()

	<-first.started
	<-second.started
	cancel()
	<-first.cancelled
	<-second.cancelled

	// Releasing only one of them must not be enough to let serve return.
	close(first.release)
	select {
	case err := <-result:
		t.Fatalf("serve returned while a scheduler was still running: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(second.release)
	require.NoError(t, <-result)
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
	go func() {
		result <- serve(ctx, cancel, server, readinessServer, []schedulerRunner{scheduler}, &blockingManualSync{})
	}()

	<-scheduler.started
	cancel()
	<-scheduler.cancelled
	close(scheduler.release)
	require.NoError(t, <-result)

	for name, server := range map[string]*http.Server{"served": server, "readiness": readinessServer} {
		assert.ErrorIsf(t, server.ListenAndServe(), http.ErrServerClosed, "the %s listener is still open", name)
	}
}

// A readiness listener that cannot bind stops the process, and the error it
// stops with is the bind failure itself. Shutdown contributes nothing here: it
// returns a listener-close error or the context's, never ErrServerClosed, so
// there is no second error to join onto the real one.
func TestServeStopsWhenTheReadinessListenerCannotBind(t *testing.T) {
	t.Parallel()

	var listenConfig net.ListenConfig
	occupied, err := listenConfig.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err, "reserving a port")
	t.Cleanup(func() {
		if closeErr := occupied.Close(); closeErr != nil {
			t.Logf("closing the reserved port: %v", closeErr)
		}
	})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	scheduler := &blockingScheduler{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
	}
	readinessServer := newTestServer()
	readinessServer.Addr = occupied.Addr().String()
	result := make(chan error, 1)
	go func() {
		result <- serve(ctx, cancel, newTestServer(), readinessServer, []schedulerRunner{scheduler}, &blockingManualSync{})
	}()

	<-scheduler.started
	<-scheduler.cancelled
	close(scheduler.release)

	err = <-result
	require.Error(t, err, "serve returned no error for a readiness listener that cannot bind")
	require.ErrorContains(t, err, "serving HTTP")
	require.ErrorContains(t, err, "address already in use")
	assert.NotErrorIs(t, err, http.ErrServerClosed, "ErrServerClosed is not a failure")
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

// A restart has to serve classifications immediately rather than going blind
// until the next rebuild lands, which is the whole reason the last build's file
// is reopened here. These are the states a host can restart into: one that has
// never built, and one whose state database remembers a build whose file is no
// longer beside it. Neither is an error, and both have to end with a schedule.
func TestStartSurfaceIndexOnAHostThatHasNeverBuilt(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)

	current, scheduler, err := startSurfaceIndex(
		t.Context(), stateSettings(directory), surfaceSettings(t, store), store, testNotifier(t),
	)
	require.NoError(t, err, "startSurfaceIndex()")
	t.Cleanup(func() { assert.NoError(t, current.Close(), "Close()") })

	assert.NotNil(t, scheduler, "no rebuild was scheduled")
	assert.Empty(t, current.Generation(), "an index was served by a host that has never built")
}

func TestStartSurfaceIndexWhenTheRememberedIndexIsGone(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)
	require.NoError(t, store.RecordSurfaceIndexBuild(t.Context(), time.Now().UTC(), "abcdef012345"))

	current, scheduler, err := startSurfaceIndex(
		t.Context(), stateSettings(directory), surfaceSettings(t, store), store, testNotifier(t),
	)
	require.NoError(t, err, "a remembered index that is no longer on disk is not an error")
	t.Cleanup(func() { assert.NoError(t, current.Close(), "Close()") })

	assert.NotNil(t, scheduler, "no rebuild was scheduled")
	assert.Empty(t, current.Generation(), "a generation was served from a file that does not exist")
}

func stateSettings(directory string) *config.Settings {
	return &config.Settings{State: config.State{DatabasePath: filepath.Join(directory, "state.db")}}
}

// surfaceSettings publishes a live snapshot naming one region, which is the
// state a host that builds an index restarts into. An interval the scheduler
// could not keep is not among the states reachable here: it is refused where the
// settings are written and again where they are read back.
func surfaceSettings(t *testing.T, store *sqlite.Store) *runtimeconfig.Current {
	t.Helper()
	current, err := runtimeconfig.Load(t.Context(), store)
	require.NoError(t, err, "runtimeconfig.Load()")

	values := current.Values()
	values.Surface.Regions = []string{"europe/germany"}
	require.NoError(t, current.Set(t.Context(), values), "Set()")

	return current
}

func testStore(t *testing.T, directory string) *sqlite.Store {
	t.Helper()

	var key [32]byte
	for index := range key {
		key[index] = byte(index)
	}
	store, err := sqlite.Open(t.Context(), filepath.Join(directory, "state.db"), key)
	require.NoError(t, err, "sqlite.Open()")
	t.Cleanup(func() { assert.NoError(t, store.Close(), "Close()") })

	return store
}

func testNotifier(t *testing.T) *pushover.Client {
	t.Helper()

	notifier, err := pushover.New(&pushover.Options{
		ApplicationToken: []byte("token"),
		UserKey:          []byte("user"),
	})
	require.NoError(t, err, "pushover.New()")

	return notifier
}

// An operator who configures no coefficients file keeps every stage exactly
// as it is today: no rider figure is ever guessed.
func TestLoadRidePredictorReturnsNilWithNoCoefficientsFileConfigured(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	predictor, validation, err := loadRidePredictor(t.Context(), &config.Settings{}, testStore(t, directory))
	require.NoError(t, err, "loadRidePredictor()")
	assert.Nil(t, predictor, "a predictor was built with no coefficients file configured")
	assert.Nil(t, validation, "validation metadata was built with no coefficients file configured")
}

func TestLoadRidePredictorBuildsAPredictorFromAValidFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	settings := &config.Settings{RideModel: config.RideModel{CoefficientsFile: writeTestCoefficients(t, directory)}}

	predictor, validation, err := loadRidePredictor(t.Context(), settings, testStore(t, directory))
	require.NoError(t, err, "loadRidePredictor()")
	assert.NotNil(t, predictor, "no predictor was built from a valid coefficients file")
	assert.Nil(t, validation, "validation metadata was built from a file with no measured benchmark result")
}

// A file that does carry the optional benchmark fields makes its measured
// unseen-route error available to the HTTP layer.
func TestLoadRidePredictorSurfacesValidationFromAFileThatHasIt(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "ridemodel.toml")
	document := `
calibration_cutoff = "2025-08-01"
mass_kg = 90.0
power_watts = 180.0
cda_m2 = 0.45
crr = 0.012
seconds_per_km = 145.3578
seconds_per_ascent_m = 3.2190
evaluated_rides = 42
bias_percent = -1.20
mae_percent = 6.80
p90_percent = 14.10
`
	require.NoError(t, os.WriteFile(path, []byte(document), 0o600), "writing coefficient file")
	settings := &config.Settings{RideModel: config.RideModel{CoefficientsFile: path}}

	predictor, validation, err := loadRidePredictor(t.Context(), settings, testStore(t, directory))
	require.NoError(t, err, "loadRidePredictor()")
	assert.NotNil(t, predictor, "no predictor was built from a valid coefficients file")
	require.NotNil(t, validation, "no validation metadata was built from a file that carries it")
	assert.Equal(t, 42, validation.EvaluatedRides, "EvaluatedRides")
	assert.InDelta(t, -1.20, validation.BiasPercent, 1e-9, "BiasPercent")
	assert.InDelta(t, 6.80, validation.MAEPercent, 1e-9, "MAEPercent")
	assert.InDelta(t, 14.10, validation.P90Percent, 1e-9, "P90Percent")
}

// A malformed or physically implausible file is a startup failure: the
// service refuses to serve a prediction it cannot stand behind.
func TestLoadRidePredictorRefusesAnImplausibleFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "ridemodel.toml")
	require.NoError(t, os.WriteFile(path, []byte("mass_kg = 1.0\n"), 0o600), "writing coefficient file")
	settings := &config.Settings{RideModel: config.RideModel{CoefficientsFile: path}}

	_, _, err := loadRidePredictor(t.Context(), settings, testStore(t, directory))
	require.Error(t, err, "loadRidePredictor() with an implausible coefficient file")
}

// A coefficient file edited or removed since the last restart must not leave
// the previous file's predictions being served as current: nothing else
// would ever notice they no longer match what is loaded now, since they
// still address the same, unchanged geometry.
func TestLoadRidePredictorPrunesPredictionsFromADifferentCoefficientFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)
	seconds := 123.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash", "", "an-earlier-coefficient-file", &seconds, nil,
	), "seeding a stale prediction")

	settings := &config.Settings{RideModel: config.RideModel{CoefficientsFile: writeTestCoefficients(t, directory)}}
	_, _, err := loadRidePredictor(t.Context(), settings, store)
	require.NoError(t, err, "loadRidePredictor()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint()")
	assert.False(t, found, "a prediction from a different coefficient file was still being served")
}

// The mirror case: switching ride model prediction off entirely must not
// leave a previous configuration's predictions being served either.
func TestLoadRidePredictorPrunesEverythingWhenUnconfigured(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)
	seconds := 123.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash", "", "a-since-removed-coefficient-file", &seconds, nil,
	), "seeding a stale prediction")

	_, _, err := loadRidePredictor(t.Context(), &config.Settings{}, store)
	require.NoError(t, err, "loadRidePredictor()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint()")
	assert.False(t, found, "a prediction survived switching ride model prediction off")
}

func writeTestCoefficients(t *testing.T, directory string) string {
	t.Helper()

	path := filepath.Join(directory, "ridemodel.toml")
	const document = `
calibration_cutoff = "2025-08-01"
mass_kg = 90.0
power_watts = 180.0
cda_m2 = 0.45
crr = 0.012
seconds_per_km = 145.3578
seconds_per_ascent_m = 3.2190
`
	require.NoError(t, os.WriteFile(path, []byte(document), 0o600), "writing coefficient file")

	return path
}
