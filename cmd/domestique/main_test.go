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
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/wahoo"
)

// A run reads the libraries that are configured now, with the credentials that
// are stored now, so adding a library is an edit rather than a restart.
func TestSourcesFollowTheConfiguredLibraries(t *testing.T) {
	t.Parallel()

	current := testSettings(t, testStore(t, t.TempDir()))
	built, err := sources(current)
	require.NoError(t, err, "sources() with nothing configured")
	assert.Empty(t, built, "a client was built for a library that has not been configured")

	values := current.Values()
	values.Sources = []runtimeconfig.Source{
		{Provider: route.ProviderVeloPlanner, BaseURL: "https://veloplanner.example.test"},
		{Provider: route.ProviderKomoot, BaseURL: "https://komoot.example.test"},
	}
	storeSettings(t, current, values)

	// Reading part of a library and calling it the whole inventory is what the
	// deletion gate exists to prevent, so a source missing its credentials
	// fails the lot rather than quietly dropping itself.
	_, err = sources(current)
	require.Error(t, err, "sources() with no credentials stored")

	require.NoError(t, current.SetSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretVeloPlannerEmail:    runtimeconfig.NewSecret([]byte("rider@example.test")),
		runtimeconfig.SecretVeloPlannerPassword: runtimeconfig.NewSecret([]byte("secret")),
		runtimeconfig.SecretKomootEmail:         runtimeconfig.NewSecret([]byte("rider@example.test")),
		runtimeconfig.SecretKomootPassword:      runtimeconfig.NewSecret([]byte("secret")),
	}), "SetSecrets()")

	built, err = sources(current)
	require.NoError(t, err, "sources()")
	require.Len(t, built, 2, "sources()")
	assert.Equal(t, route.ProviderVeloPlanner, built[0].Provider(), "first source")
	assert.Equal(t, route.ProviderKomoot, built[1].Provider(), "second source")
}

func TestSourceForBuildsOneLibraryWhateverTheOthersAreMissing(t *testing.T) {
	t.Parallel()

	current := testSettings(t, testStore(t, t.TempDir()))
	values := current.Values()
	values.Sources = []runtimeconfig.Source{
		{Provider: route.ProviderVeloPlanner, BaseURL: "https://veloplanner.example.test"},
		{Provider: route.ProviderKomoot, BaseURL: "https://komoot.example.test"},
	}
	storeSettings(t, current, values)
	require.NoError(t, current.SetSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretKomootEmail:    runtimeconfig.NewSecret([]byte("rider@example.test")),
		runtimeconfig.SecretKomootPassword: runtimeconfig.NewSecret([]byte("secret")),
	}), "SetSecrets()")

	_, err := sources(current)
	require.Error(t, err, "sources() with one library missing its credentials")

	built, configured, err := sourceFor(current, route.ProviderKomoot)
	require.NoError(t, err, "sourceFor(komoot)")
	require.True(t, configured, "a configured library reported itself unconfigured")
	require.NotNil(t, built, "no client was built for a configured library")
	assert.Equal(t, route.ProviderKomoot, built.Provider(), "the library that was built")

	half, configured, err := sourceFor(current, route.ProviderVeloPlanner)
	require.NoError(t, err, "sourceFor() with the credentials not entered")
	assert.False(t, configured, "a library without credentials reported itself configured")
	assert.Nil(t, half, "a client was built without credentials")
}

// Until the Wahoo application is entered there is nothing to reconcile against,
// so a run is told it has no targets instead of failing against an application
// that does not exist.
func TestWahooProviderOffersNoTargetsUntilItsApplicationIsConfigured(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	current := testSettings(t, store)
	require.NoError(t, store.EnsureTargetOwner(t.Context(), "rider"), "EnsureTargetOwner()")

	provider := newWahooProvider(current, store, "https://domestique.example.test")
	assert.Empty(t, provider.targetIDs(), "a target was offered before the Wahoo application was configured")
	_, err := provider.current()
	require.ErrorIs(t, err, errNotConfigured, "current()")

	configureWahoo(t, current)

	assert.Equal(t, []string{"rider"}, provider.targetIDs(), "targetIDs()")
	client, err := provider.current()
	require.NoError(t, err, "current()")
	assert.NotNil(t, client, "no client was built from a configured application")
}

// The client carries the request budget observed from Wahoo's own responses, so
// it survives every use that did not change the settings it was built from.
func TestWahooProviderRebuildsOnlyWhenItsSettingsChange(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	current := testSettings(t, store)
	configureWahoo(t, current)
	provider := newWahooProvider(current, store, "https://domestique.example.test")

	first, err := provider.current()
	require.NoError(t, err, "current()")
	again, err := provider.current()
	require.NoError(t, err, "current()")
	assert.Same(t, first, again, "the client was rebuilt without a settings change")

	// An edit anywhere else on the settings page is not a reason to throw the
	// observed budget away.
	values := current.Values()
	values.Surface.Regions = []string{"europe/germany"}
	storeSettings(t, current, values)

	kept, err := provider.current()
	require.NoError(t, err, "current()")
	assert.Same(t, first, kept, "an unrelated settings edit discarded the client")

	values.Wahoo.ClientID = "another-application"
	storeSettings(t, current, values)

	rebuilt, err := provider.current()
	require.NoError(t, err, "current()")
	assert.NotSame(t, first, rebuilt, "an edited application was still served by the old client")

	require.NoError(t, current.SetSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretWahooClientSecret: runtimeconfig.NewSecret([]byte("a-rotated-secret")),
	}), "SetSecrets()")

	rotated, err := provider.current()
	require.NoError(t, err, "current()")
	assert.NotSame(t, rebuilt, rotated, "a rotated client secret was still served by the old client")
}

func configureWahoo(t *testing.T, current *runtimeconfig.Current) {
	t.Helper()

	values := current.Values()
	values.Wahoo.APIBaseURL = "https://api.wahoo.example.test"
	values.Wahoo.OAuthBaseURL = "https://oauth.wahoo.example.test"
	values.Wahoo.ClientID = "an-application"
	storeSettings(t, current, values)
	require.NoError(t, current.SetSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretWahooClientSecret: runtimeconfig.NewSecret([]byte("a-client-secret")),
	}), "SetSecrets()")
}

// A library client is only ever built from settings that describe one: a
// provider nothing reads, and an origin no client would accept, are refused
// here rather than by the first run that tried to read from them.
func TestNoSourceIsBuiltFromSettingsNoClientAccepts(t *testing.T) {
	t.Parallel()

	email, password := []byte("rider@example.test"), []byte("secret")
	_, err := newSource(runtimeconfig.Source{Provider: "nowhere", BaseURL: "https://example.test"}, email, password)
	require.Error(t, err, "newSource() for a provider nothing reads")

	for _, provider := range []route.Provider{route.ProviderVeloPlanner, route.ProviderKomoot} {
		_, err := newSource(runtimeconfig.Source{Provider: provider, BaseURL: "http://example.test"}, email, password)
		require.Error(t, err, string(provider))
	}
}

// Nothing this provider hands out is answered by the provider itself, so every
// call an unconfigured service makes reports that rather than failing against
// an application that does not exist.
func TestWahooProviderRefusesEveryCallUntilItsApplicationIsConfigured(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	provider := newWahooProvider(testSettings(t, store), store, "https://domestique.example.test")
	calls := map[string]func() error{
		"AuthorizationURL":          func() error { _, err := provider.AuthorizationURL("state"); return err },
		"ExchangeAuthorizationCode": func() error { _, _, err := provider.ExchangeAuthorizationCode(t.Context(), "code"); return err },
		"AuthenticatedUser":         func() error { _, err := provider.AuthenticatedUser(t.Context(), "token"); return err },
		"RefreshAccessToken":        func() error { _, _, err := provider.RefreshAccessToken(t.Context(), "token"); return err },
		"ListOwnedRoutes":           func() error { _, err := provider.ListOwnedRoutes(t.Context(), "token"); return err },
		"DeleteOwnedRoutes":         func() error { _, err := provider.DeleteOwnedRoutes(t.Context(), "token"); return err },
		"CreateRoute":               func() error { _, err := provider.CreateRoute(t.Context(), "token", nil, nil); return err },
		"UpdateRoute":               func() error { _, err := provider.UpdateRoute(t.Context(), 1, "token", nil, nil); return err },
		"DeleteRoute":               func() error { return provider.DeleteRoute(t.Context(), 1, "token") },
		"ListActivities":            func() error { _, err := provider.ListActivities(t.Context(), "token"); return err },
		"ActivitySummary":           func() error { _, err := provider.ActivitySummary(t.Context(), "token", 42); return err },
	}
	for name, call := range calls {
		require.ErrorIs(t, call(), errNotConfigured, name)
	}

	_, _, ok := provider.RateLimit()
	assert.False(t, ok, "a request budget was reported for an application that does not exist")
	assert.True(t, provider.IsUnauthorized(wahoo.ErrUnauthorized), "IsUnauthorized()")
	assert.True(t, provider.IsUnreadable(wahoo.ErrWorkoutUnreadable), "IsUnreadable()")
	assert.False(t, provider.IsUnreadable(wahoo.ErrUnauthorized), "IsUnreadable() took a rejected grant for a dead workout")
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
			CloudCoverPercent:               []float64{62},
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
		CloudCoverPercent:               []float64{62},
	}, series[0])
}

func TestServeWaitsForTheCancelledTaskLayerBeforeReturning(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tasks := &blockingTasks{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
	}
	server := newTestServer()
	readinessServer := newTestServer()
	result := make(chan error, 1)
	go func() {
		result <- serve(ctx, cancel, server, readinessServer, tasks, func(context.Context) {})
	}()

	<-tasks.started
	cancel()
	<-tasks.cancelled

	select {
	case err := <-result:
		t.Fatalf("serve returned before the task layer finished: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	close(tasks.release)
	require.NoError(t, <-result)
}

func TestServeStopsBothListeners(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	tasks := &blockingTasks{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
	}
	server, readinessServer := newTestServer(), newTestServer()
	result := make(chan error, 1)
	go func() {
		result <- serve(ctx, cancel, server, readinessServer, tasks, func(context.Context) {})
	}()

	<-tasks.started
	cancel()
	<-tasks.cancelled
	close(tasks.release)
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
	tasks := &blockingTasks{
		started:   make(chan struct{}),
		cancelled: make(chan struct{}),
		release:   make(chan struct{}),
	}
	readinessServer := newTestServer()
	readinessServer.Addr = occupied.Addr().String()
	result := make(chan error, 1)
	go func() {
		result <- serve(ctx, cancel, newTestServer(), readinessServer, tasks, func(context.Context) {})
	}()

	<-tasks.started
	<-tasks.cancelled
	close(tasks.release)

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

type blockingTasks struct {
	started   chan struct{}
	cancelled chan struct{}
	release   chan struct{}
}

func (b *blockingTasks) Run(ctx context.Context) {
	close(b.started)
	<-ctx.Done()
	close(b.cancelled)
	<-b.release
}

func (*blockingTasks) Wait() {}

// A restart has to serve classifications immediately rather than going blind
// until the next rebuild. These are the states a host can restart into: never
// built, and a state database remembering a build whose file is gone. Neither is
// an error, and both end with a schedule.
func TestStartSurfaceIndexOnAHostThatHasNeverBuilt(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)

	current, definition, err := startSurfaceIndex(t.Context(), stateSettings(directory), surfaceSettings(t, store), store, allEnabled)
	require.NoError(t, err, "startSurfaceIndex()")
	t.Cleanup(func() { assert.NoError(t, current.Close(), "Close()") })

	assert.NotNil(t, definition.Schedule, "no rebuild was scheduled")
	assert.Empty(t, current.Generation(), "an index was served by a host that has never built")
}

func TestStartSurfaceIndexWhenTheRememberedIndexIsGone(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)
	require.NoError(t, store.RecordSurfaceIndexBuild(t.Context(), time.Now().UTC(), "abcdef012345"))

	current, definition, err := startSurfaceIndex(t.Context(), stateSettings(directory), surfaceSettings(t, store), store, allEnabled)
	require.NoError(t, err, "a remembered index that is no longer on disk is not an error")
	t.Cleanup(func() { assert.NoError(t, current.Close(), "Close()") })

	assert.NotNil(t, definition.Schedule, "no rebuild was scheduled")
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
	current := testSettings(t, store)

	values := current.Values()
	values.Surface.Regions = []string{"europe/germany"}
	storeSettings(t, current, values)

	return current
}

// testSettings publishes what a service that has never been configured starts
// with. storeSettings replaces the settings whole, which is what a test holding
// a complete set of values wants; the service replaces one section at a time.
//
//nolint:gocritic // value param: mirrors Update's own copy-in.
func storeSettings(t *testing.T, current *runtimeconfig.Current, values runtimeconfig.Values) {
	t.Helper()
	require.NoError(t, current.Update(t.Context(), func(runtimeconfig.Values) runtimeconfig.Values {
		return values
	}), "Update()")
}

func testSettings(t *testing.T, store *sqlite.Store) *runtimeconfig.Current {
	t.Helper()
	current, err := runtimeconfig.Load(t.Context(), store)
	require.NoError(t, err, "runtimeconfig.Load()")

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

// An operator who configures no coefficients file keeps every stage exactly
// as it is today: no rider figure is ever guessed.
func TestRideModelIsAbsentWithNoCoefficientsFileConfigured(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	provider := newRideModelProvider(store)
	require.NoError(t, provider.reload(t.Context(), testSettings(t, store)), "reload()")
	assert.Nil(t, provider.current(), "a predictor was built with no coefficients file configured")
	assert.Nil(t, provider.validationView(), "validation metadata was built with no coefficients file configured")
}

func TestRideModelLoadsAPredictorFromAValidFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)
	provider := newRideModelProvider(store)
	settings := rideModelSettings(t, store, writeTestCoefficients(t, directory))

	require.NoError(t, provider.reload(t.Context(), settings), "reload()")
	assert.NotNil(t, provider.current(), "no predictor was built from a valid coefficients file")
	assert.Nil(t, provider.validationView(), "validation metadata was built from a file with no measured benchmark result")
}

// A file that does carry the optional benchmark fields makes its measured
// unseen-route error available to the HTTP layer.
func TestRideModelSurfacesValidationFromAFileThatHasIt(t *testing.T) {
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
	store := testStore(t, directory)
	provider := newRideModelProvider(store)

	require.NoError(t, provider.reload(t.Context(), rideModelSettings(t, store, path)), "reload()")
	assert.NotNil(t, provider.current(), "no predictor was built from a valid coefficients file")
	validation := provider.validationView()
	require.NotNil(t, validation, "no validation metadata was built from a file that carries it")
	assert.Equal(t, 42, validation.EvaluatedRides, "EvaluatedRides")
	assert.InDelta(t, -1.20, validation.BiasPercent, 1e-9, "BiasPercent")
	assert.InDelta(t, 6.80, validation.MAEPercent, 1e-9, "MAEPercent")
	assert.InDelta(t, 14.10, validation.P90Percent, 1e-9, "P90Percent")
}

// A malformed or physically implausible file leaves nothing loaded: the service
// refuses to serve a prediction it cannot stand behind.
func TestRideModelRefusesAnImplausibleFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	path := filepath.Join(directory, "ridemodel.toml")
	require.NoError(t, os.WriteFile(path, []byte("mass_kg = 1.0\n"), 0o600), "writing coefficient file")
	store := testStore(t, directory)
	provider := newRideModelProvider(store)

	require.Error(t, provider.reload(t.Context(), rideModelSettings(t, store, path)),
		"reload() with an implausible coefficient file")
	assert.Nil(t, provider.current(), "a predictor was built from an implausible coefficients file")
}

// A coefficient file pointed somewhere else must not leave the previous file's
// predictions being served as current: nothing else would ever notice they no
// longer match what is loaded now, since they still address the same,
// unchanged geometry.
func TestRideModelPrunesPredictionsFromADifferentCoefficientFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)
	seconds := 123.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash", "", "an-earlier-coefficient-file", &seconds, nil,
	), "seeding a stale prediction")

	provider := newRideModelProvider(store)
	settings := rideModelSettings(t, store, writeTestCoefficients(t, directory))
	require.NoError(t, provider.reload(t.Context(), settings), "reload()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint()")
	assert.False(t, found, "a prediction from a different coefficient file was still being served")
}

// The mirror case: switching ride model prediction off entirely must not
// leave a previous configuration's predictions being served either.
func TestRideModelPrunesEverythingWhenUnconfigured(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)
	seconds := 123.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash", "", "a-since-removed-coefficient-file", &seconds, nil,
	), "seeding a stale prediction")

	provider := newRideModelProvider(store)
	require.NoError(t, provider.reload(t.Context(), testSettings(t, store)), "reload()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint()")
	assert.False(t, found, "a prediction survived switching ride model prediction off")
}

func rideModelSettings(t *testing.T, store *sqlite.Store, path string) *runtimeconfig.Current {
	t.Helper()

	current := testSettings(t, store)
	values := current.Values()
	values.RideModel.CoefficientsFile = path
	storeSettings(t, current, values)

	return current
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

// Synchronization sees nothing but Predict, and what it predicts with is the
// coefficient file in force at the time of the run rather than the one the
// process started with.
func TestPredictorFollowsTheConfiguredCoefficientFile(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	store := testStore(t, directory)
	current := testSettings(t, store)
	predictor := predictorFor(newRideModelProvider(store), current)

	predicted, failed, err := predictor.Predict(t.Context(), nil)
	require.NoError(t, err, "Predict() with prediction switched off")
	assert.Zero(t, predicted, "a stage was predicted with no coefficient file configured")
	assert.Zero(t, failed, "a prediction failed with no coefficient file configured")

	values := current.Values()
	values.RideModel.CoefficientsFile = writeTestCoefficients(t, directory)
	storeSettings(t, current, values)

	_, _, err = predictor.Predict(t.Context(), nil)
	require.NoError(t, err, "Predict() after a coefficient file was configured")

	// A file that will not load is a failed prediction rather than a
	// substituted one: nothing is served that the loaded model does not
	// stand behind.
	values.RideModel.CoefficientsFile = filepath.Join(directory, "not-a-file.toml")
	storeSettings(t, current, values)

	_, _, err = predictor.Predict(t.Context(), nil)
	require.Error(t, err, "Predict() with a coefficient file that will not load")
}

// Every style a browser may be told to load has to be read, or the policy is
// complete for a reader on one colour scheme and short for a reader on the
// other.
func TestConfiguredStyleURLsCoversBothColourSchemes(t *testing.T) {
	styles := configuredStyleURLs([]runtimeconfig.Basemap{
		{Name: "Streets", StyleURL: "https://tiles.example.test/bright", StyleURLDark: "https://tiles.example.test/dark"},
		{Name: "Satellite", StyleURL: "https://imagery.example.test/satellite", DarkCartography: true},
	})

	assert.Equal(t, []string{
		"https://tiles.example.test/bright",
		"https://tiles.example.test/dark",
		"https://imagery.example.test/satellite",
	}, styles, "the styles a browser may load")
	assert.Empty(t, configuredStyleURLs(nil), "an unconfigured list")
}
