package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/brouter"
	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/surface"
)

type planningSurfaceSource struct {
	generation string
}

func (planningSurfaceSource) Ways(context.Context, []route.Point) ([]surface.Way, error) {
	return nil, nil
}

func (s planningSurfaceSource) Generation() string { return s.generation }

// [planning] absent switches the planner off: newLocalSource returns nil
// without an error rather than a source with nothing to route.
func TestNewLocalSourceIsNilWithoutPlanning(t *testing.T) {
	t.Parallel()

	source, configured, err := newLocalSource(&config.Settings{}, testStore(t, t.TempDir()), nil)
	require.NoError(t, err)
	assert.False(t, configured, "newLocalSource() without [planning]")
	assert.Nil(t, source, "newLocalSource() without [planning]")
}

func TestSurfaceClassifierOmitsMissingMapAndKeepsUnknownClassification(t *testing.T) {
	points := []route.Point{{Longitude: 8, Latitude: 49}, {Longitude: 8.1, Latitude: 49.1}}

	missing, err := newSurfaceClassifier(planningSurfaceSource{}).Classify(t.Context(), points)
	require.NoError(t, err)
	assert.Nil(t, missing)

	unknown, err := newSurfaceClassifier(planningSurfaceSource{generation: "current"}).Classify(t.Context(), points)
	require.NoError(t, err)
	require.NotNil(t, unknown)
	require.Len(t, unknown.Ranges, 1)
	assert.Equal(t, "unknown", unknown.Ranges[0].Kind)
}

func TestNewLocalSourceBuildsThePlanServiceWhenConfigured(t *testing.T) {
	settings := testPlanningSettings(t)

	source, configured, err := newLocalSource(settings, testStore(t, t.TempDir()), nil)
	require.NoError(t, err)
	require.True(t, configured, "newLocalSource() with [planning] configured")
	require.NotNil(t, source, "newLocalSource() with [planning] configured")
	assert.Equal(t, route.ProviderLocal, source.Provider(), "Provider()")
}

// newLocalSource forwards a BRouter client construction failure rather than
// registering a source that could never route.
func TestNewLocalSourceForwardsABRouterConstructionFailure(t *testing.T) {
	settings := testPlanningSettings(t)
	settings.Planning.BRouterURL = "not a url"

	_, configured, err := newLocalSource(settings, testStore(t, t.TempDir()), nil)
	require.Error(t, err)
	assert.False(t, configured, "newLocalSource() on a construction failure")
}

func TestWireLocalSourceLeavesTheCacheEmptyWithoutPlanning(t *testing.T) {
	t.Parallel()

	cache := newSourceCache()
	service, wired, err := wireLocalSource(&config.Settings{}, testStore(t, t.TempDir()), cache, nil)
	require.NoError(t, err)
	assert.False(t, wired, "wireLocalSource() without [planning]")
	assert.Nil(t, service, "wireLocalSource() without [planning]")

	_, configured, err := cache.sourceFor(testSettings(t, testStore(t, t.TempDir())), route.ProviderLocal)
	require.NoError(t, err)
	assert.False(t, configured, "sourceFor(local) after wireLocalSource without [planning]")
}

func TestWireLocalSourceRegistersThePlanServiceWhenConfigured(t *testing.T) {
	settings := testPlanningSettings(t)
	cache := newSourceCache()
	service, wired, err := wireLocalSource(settings, testStore(t, t.TempDir()), cache, nil)
	require.NoError(t, err)
	require.True(t, wired, "wireLocalSource() with [planning] configured")
	require.NotNil(t, service, "wireLocalSource() with [planning] configured")

	source, configured, sourceErr := cache.sourceFor(testSettings(t, testStore(t, t.TempDir())), route.ProviderLocal)
	require.NoError(t, sourceErr)
	require.True(t, configured, "sourceFor(local) after wireLocalSource with [planning]")
	assert.Equal(t, route.ProviderLocal, source.Provider(), "Provider()")
}

// The segments warning is logged once at startup and changes no behaviour;
// this only exercises the branch, since slog output is not asserted here.
func TestWireLocalSourceWarnsAboutUnusedSegments(t *testing.T) {
	settings := testPlanningSettings(t)
	settings.Planning.Segments = []string{"E5_N45"}
	cache := newSourceCache()

	_, _, err := wireLocalSource(settings, testStore(t, t.TempDir()), cache, nil)
	require.NoError(t, err)
}

// wireLocalSource forwards newLocalSource's own construction failure rather
// than swallowing it.
func TestWireLocalSourceForwardsAConstructionFailure(t *testing.T) {
	settings := testPlanningSettings(t)
	settings.Planning.BRouterURL = "not a url"
	cache := newSourceCache()

	service, configured, err := wireLocalSource(settings, testStore(t, t.TempDir()), cache, nil)
	require.Error(t, err)
	assert.False(t, configured, "wireLocalSource() on a construction failure")
	assert.Nil(t, service, "wireLocalSource() on a construction failure")
}

// httpapiPlans must not hand main's Options literal a typed nil pointer
// behind the Plans interface, which Go would treat as non-nil.
func TestHTTPAPIPlansAvoidsATypedNilInterface(t *testing.T) {
	var unconfigured *plan.Service
	assert.Nil(t, httpapiPlans(unconfigured), "httpapiPlans(nil)")

	settings := testPlanningSettings(t)
	service, configured, err := newLocalSource(settings, testStore(t, t.TempDir()), nil)
	require.NoError(t, err)
	require.True(t, configured)
	assert.NotNil(t, httpapiPlans(service), "httpapiPlans(configured)")
}

// brouterRouter converts plan types to the adapter's own before asking the
// engine, and the points it returns travel back unchanged.
func TestBrouterRouterConvertsWaypointsAndProfile(t *testing.T) {
	var gotProfile string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProfile = r.URL.Query().Get("profile")
		_, writeErr := w.Write([]byte(`{"type":"FeatureCollection","features":[{"type":"Feature",` +
			`"geometry":{"type":"LineString","coordinates":[[8.68,50.11],[8.70,50.12]]}}]}`))
		assert.NoError(t, writeErr)
	}))
	defer server.Close()

	client, err := brouter.New(&brouter.Options{BaseURL: server.URL})
	require.NoError(t, err)
	router := brouterRouter{client: client}

	points, err := router.Route(
		t.Context(), []plan.Waypoint{{Longitude: 8.68, Latitude: 50.11}, {Longitude: 8.70, Latitude: 50.12}}, plan.Gravel)
	require.NoError(t, err)
	assert.Equal(t, "gravel", gotProfile, "profile")
	require.Len(t, points, 2, "points")
}

// A routing failure is wrapped rather than passed through bare, but the
// caller must still be able to recover the adapter's own category.
func TestBrouterRouterWrapsARoutingFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client, err := brouter.New(&brouter.Options{BaseURL: server.URL})
	require.NoError(t, err)
	router := brouterRouter{client: client}

	_, routeErr := router.Route(
		t.Context(), []plan.Waypoint{{Longitude: 8.68, Latitude: 50.11}, {Longitude: 8.70, Latitude: 50.12}}, plan.Gravel)
	require.Error(t, routeErr)
	var brouterErr *brouter.Error
	require.ErrorAs(t, routeErr, &brouterErr)
	assert.Equal(t, brouter.FailureEngine, brouterErr.Category, "Category")
}

// testPlanningSettings loads a valid configuration file with [planning] set,
// exercising the same Settings.Planning.Enabled() signal build() sets.
func testPlanningSettings(t *testing.T) *config.Settings {
	t.Helper()

	directory := t.TempDir()
	keyPath := filepath.Join(directory, "state-key")
	secretPath := filepath.Join(directory, "client-secret")
	require.NoError(t, os.WriteFile(keyPath, []byte(strings.Repeat("A", 43)+"\n"), 0o600))
	require.NoError(t, os.WriteFile(secretPath, []byte("client-secret\n"), 0o600))
	configPath := filepath.Join(directory, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(fmt.Sprintf(`
[http]
listen_address = ":8080"
browser_origin_url = "https://domestique.example.test"
[auth.auth0]
domain = "tenant.eu.auth0.com"
client_id = "client-id"
client_secret_file = %q
[state]
database_path = %q
encryption_key_file = %q
[planning]
brouter_url = "http://brouter:17777"
`, secretPath, filepath.Join(directory, "state.db"), keyPath)), 0o600))
	t.Setenv("DOMESTIQUE_CONFIG_FILE", configPath)

	settings, err := config.Load()
	require.NoError(t, err, "config.Load()")

	return settings
}

// The adapter's job is exactly this round trip: a plan written through it and
// read back must carry the same waypoints, profile, and geometry.
func TestPlanStoreRoundTripsAPlan(t *testing.T) {
	t.Parallel()

	store := planStore{store: testStore(t, t.TempDir())}
	elevation := 42.5
	original := plan.Plan{
		ID: 7, Name: "Round Trip", Profile: plan.Gravel,
		Waypoints: []plan.Waypoint{{Longitude: 8.68, Latitude: 50.11}, {Longitude: 8.70, Latitude: 50.12}},
		Geometry: []route.Point{
			{Longitude: 8.68, Latitude: 50.11, Elevation: &elevation},
			{Longitude: 8.70, Latitude: 50.12},
		},
		DistanceMetres: 1234.5, AscentMetres: 56.7, Published: true, Version: 1,
		CreatedAt: time.Now().UTC().Truncate(time.Second), UpdatedAt: time.Now().UTC().Truncate(time.Second),
	}

	require.NoError(t, store.InsertPlan(t.Context(), &original), "InsertPlan()")

	read, found, err := store.GetPlan(t.Context(), original.ID)
	require.NoError(t, err, "GetPlan()")
	require.True(t, found, "GetPlan() found")
	assert.Equal(t, original.Name, read.Name, "Name")
	assert.Equal(t, original.Profile, read.Profile, "Profile")
	assert.Equal(t, original.Waypoints, read.Waypoints, "Waypoints")
	assert.Equal(t, original.Published, read.Published, "Published")
	require.Len(t, read.Geometry, 2, "Geometry")
	require.NotNil(t, read.Geometry[0].Elevation, "Geometry[0].Elevation")
	assert.InDelta(t, elevation, *read.Geometry[0].Elevation, 0, "Geometry[0].Elevation")
	assert.Nil(t, read.Geometry[1].Elevation, "Geometry[1].Elevation")

	listed, err := store.ListPlans(t.Context())
	require.NoError(t, err, "ListPlans()")
	require.Len(t, listed, 1, "ListPlans()")

	published, err := store.ListPublishedPlans(t.Context())
	require.NoError(t, err, "ListPublishedPlans()")
	require.Len(t, published, 1, "ListPublishedPlans()")

	expectedVersion := read.Version
	read.Name = "Renamed"
	read.Version = expectedVersion + 1
	ok, err := store.ReplacePlan(t.Context(), &read, expectedVersion)
	require.NoError(t, err, "ReplacePlan()")
	assert.True(t, ok, "ReplacePlan()")

	deleted, err := store.DeletePlan(t.Context(), read.ID, read.Version)
	require.NoError(t, err, "DeletePlan()")
	assert.True(t, deleted, "DeletePlan()")
}

// A record whose stored profile this build no longer recognises must surface
// as an error, not a plan silently missing its routing profile. The database
// schema itself refuses an unknown profile on write, so this goes through
// planOf directly rather than round-tripping a corrupt row through SQLite.
func TestPlanOfRejectsAnUnrecognisedStoredProfile(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC().Truncate(time.Second)
	record := sqlite.PlanRecord{
		ID: 1, Name: "Corrupt", Profile: "not-a-real-profile",
		Waypoints: [][2]float64{{8.68, 50.11}, {8.70, 50.12}},
		Geometry:  []route.Point{{Longitude: 8.68, Latitude: 50.11}, {Longitude: 8.70, Latitude: 50.12}},
		Version:   1, CreatedAt: now, UpdatedAt: now,
	}

	_, err := planOf(&record)
	require.Error(t, err)
}

// A store that cannot be reached must surface as an error on every method,
// never a silently empty answer.
func TestPlanStoreForwardsUnderlyingStoreFailures(t *testing.T) {
	t.Parallel()

	underlying := testStore(t, t.TempDir())
	require.NoError(t, underlying.Close())
	store := planStore{store: underlying}
	now := time.Now().UTC()
	unreachable := plan.Plan{ID: 1, Name: "Unreachable", Profile: "gravel", Version: 1, CreatedAt: now, UpdatedAt: now}

	require.Error(t, store.InsertPlan(t.Context(), &unreachable), "InsertPlan()")
	_, _, err := store.GetPlan(t.Context(), 1)
	require.Error(t, err, "GetPlan()")
	_, err = store.ListPlans(t.Context())
	require.Error(t, err, "ListPlans()")
	_, err = store.ListPublishedPlans(t.Context())
	require.Error(t, err, "ListPublishedPlans()")
	_, err = store.ReplacePlan(t.Context(), &unreachable, 1)
	require.Error(t, err, "ReplacePlan()")
	_, err = store.DeletePlan(t.Context(), 1, 1)
	require.Error(t, err, "DeletePlan()")
}
