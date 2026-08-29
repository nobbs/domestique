package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestStore(t *testing.T, key [32]byte) *Store {
	t.Helper()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "state.db"), key)
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
