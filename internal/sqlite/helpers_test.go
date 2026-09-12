package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// templateState is one fully migrated state file; each test writes a copy
// instead of replaying every migration, which the race detector makes slow.
var templateState []byte //nolint:gochecknoglobals // Set once by TestMain, read-only afterwards.

func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "domestique-sqlite-test")
	if err != nil {
		panic(err)
	}
	databasePath := filepath.Join(directory, "state.db")
	store, err := Open(context.Background(), databasePath, testKey(1))
	if err != nil {
		panic(err)
	}
	if err = store.Close(); err != nil {
		panic(err)
	}
	templateState, err = os.ReadFile(databasePath) //nolint:gosec // A path this process just created under the OS temp directory.
	if err != nil {
		panic(err)
	}
	if err = os.RemoveAll(directory); err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func openTestStore(t *testing.T, key [32]byte) *Store {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "state.db")
	require.NoError(t, os.WriteFile(databasePath, templateState, 0o600), "WriteFile(state)")
	store, err := Open(context.Background(), databasePath, key)
	require.NoError(t, err, "Open()")
	t.Cleanup(func() {
		if err := store.Close(); !errors.Is(err, sql.ErrConnDone) {
			assert.NoError(t, err, "Close()")
		}
	})

	return store
}

func testKey(value byte) [32]byte {
	var key [32]byte
	for index := range key {
		key[index] = value
	}

	return key
}

func storeTestStage(t *testing.T, routeID int64, stageOrder int, revision, contentHash string) route.Route {
	t.Helper()
	stage, err := route.NewRoute(
		route.ProviderVeloPlanner,
		routeID,
		stageOrder,
		revision,
		"Route",
		"",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.401, Latitude: 49.001}},
		contentHash,
	)
	require.NoError(t, err, "NewRoute()")

	return stage
}

func storeTestStageWithGeometry(
	t *testing.T,
	routeID int64,
	stageOrder int,
	revision, contentHash, routeName, stageName string,
	geometry []route.Point,
) route.Route {
	t.Helper()
	stage, err := route.NewRoute(route.ProviderVeloPlanner, routeID, stageOrder, revision, routeName, stageName, geometry, contentHash)
	require.NoError(t, err, "NewRoute()")

	return stage
}
