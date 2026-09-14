package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/route"
)

// [planning] absent switches the planner off: newLocalSource returns nil
// without an error rather than a source with nothing to route.
func TestNewLocalSourceIsNilWithoutPlanning(t *testing.T) {
	t.Parallel()

	source, configured, err := newLocalSource(&config.Settings{}, testStore(t, t.TempDir()))
	require.NoError(t, err)
	assert.False(t, configured, "newLocalSource() without [planning]")
	assert.Nil(t, source, "newLocalSource() without [planning]")
}

func TestNewLocalSourceBuildsThePlanServiceWhenConfigured(t *testing.T) {
	settings := testPlanningSettings(t)

	source, configured, err := newLocalSource(settings, testStore(t, t.TempDir()))
	require.NoError(t, err)
	require.True(t, configured, "newLocalSource() with [planning] configured")
	require.NotNil(t, source, "newLocalSource() with [planning] configured")
	assert.Equal(t, route.ProviderLocal, source.Provider(), "Provider()")
}

func TestWireLocalSourceLeavesTheCacheEmptyWithoutPlanning(t *testing.T) {
	t.Parallel()

	cache := newSourceCache()
	require.NoError(t, wireLocalSource(&config.Settings{}, testStore(t, t.TempDir()), cache))

	_, configured, err := cache.sourceFor(testSettings(t, testStore(t, t.TempDir())), route.ProviderLocal)
	require.NoError(t, err)
	assert.False(t, configured, "sourceFor(local) after wireLocalSource without [planning]")
}

func TestWireLocalSourceRegistersThePlanServiceWhenConfigured(t *testing.T) {
	settings := testPlanningSettings(t)
	cache := newSourceCache()
	require.NoError(t, wireLocalSource(settings, testStore(t, t.TempDir()), cache))

	source, configured, err := cache.sourceFor(testSettings(t, testStore(t, t.TempDir())), route.ProviderLocal)
	require.NoError(t, err)
	require.True(t, configured, "sourceFor(local) after wireLocalSource with [planning]")
	assert.Equal(t, route.ProviderLocal, source.Provider(), "Provider()")
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
