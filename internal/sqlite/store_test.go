package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func TestStoreAuthorizesAndEncryptsRefreshToken(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}), "EnsureTargets()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"), "AuthorizeTarget()")

	target, err := store.Target(t.Context(), "rider-a")
	require.NoError(t, err, "Target()")
	assert.Equal(t, AuthorizationAuthorized, target.AuthorizationState, "Target().AuthorizationState")
	assert.Equal(t, "wahoo-user", target.WahooUserID, "Target().WahooUserID")

	var encrypted []byte
	require.NoError(t, store.database.QueryRowContext(t.Context(), "SELECT refresh_token FROM targets WHERE slot = ?", "rider-a").Scan(&encrypted), "query encrypted token")
	assert.NotContains(t, string(encrypted), "refresh-token", "the database stores the refresh token in plaintext")

	got, err := store.RefreshToken(t.Context(), "rider-a")
	require.NoError(t, err, "RefreshToken()")
	assert.Equal(t, "refresh-token", got, "RefreshToken()")
}

func TestStoreRejectsDuplicateWahooUser(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}), "EnsureTargets()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "token-a"), "AuthorizeTarget(rider-a)")
	require.ErrorIs(t, store.AuthorizeTarget(t.Context(), "rider-b", "wahoo-user", "token-b"), ErrWahooUserAlreadyAuthorized, "AuthorizeTarget(rider-b)")
}

func TestStoreBindsTokenToTarget(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}), "EnsureTargets()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user-a", "token-a"), "AuthorizeTarget()")

	var encrypted []byte
	require.NoError(t, store.database.QueryRowContext(t.Context(), "SELECT refresh_token FROM targets WHERE slot = ?", "rider-a").Scan(&encrypted), "query encrypted token")
	_, err := store.database.ExecContext(t.Context(), "UPDATE targets SET refresh_token = ? WHERE slot = ?", encrypted, "rider-b")
	require.NoError(t, err, "copy encrypted token")

	_, err = store.RefreshToken(t.Context(), "rider-b")
	require.ErrorIs(t, err, ErrStateUnreadable, "RefreshToken()")
}

func TestStoreRejectsDifferentEncryptionKey(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, openErr := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, openErr, "Open()")
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"), "AuthorizeTarget()")
	require.NoError(t, store.Close(), "Close()")

	reopened, err := Open(t.Context(), databasePath, testKey(2))
	require.NoError(t, err, "Open() with different key")
	t.Cleanup(func() {
		assert.NoError(t, reopened.Close(), "Close()")
	})
	_, err = reopened.RefreshToken(t.Context(), "rider-a")
	require.ErrorIs(t, err, ErrStateUnreadable, "RefreshToken()")
}

func TestStoreMarksTargetForReauthorization(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"), "AuthorizeTarget()")
	require.NoError(t, store.MarkNeedsReauthorization(t.Context(), "rider-a"), "MarkNeedsReauthorization()")

	target, err := store.Target(t.Context(), "rider-a")
	require.NoError(t, err, "Target()")
	assert.Equal(t, AuthorizationNeedsReauthorization, target.AuthorizationState, "Target().AuthorizationState")
	_, err = store.RefreshToken(t.Context(), "rider-a")
	require.ErrorIs(t, err, ErrRefreshTokenUnavailable, "RefreshToken()")
}

func TestStoreReplacesRefreshToken(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	require.NoError(t, store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "old-refresh-token"), "AuthorizeTarget()")
	require.NoError(t, store.ReplaceRefreshToken(t.Context(), "rider-a", "new-refresh-token"), "ReplaceRefreshToken()")

	got, err := store.RefreshToken(t.Context(), "rider-a")
	require.NoError(t, err, "RefreshToken()")
	assert.Equal(t, "new-refresh-token", got, "RefreshToken()")
}

func TestStoreConsumesCallerBoundOAuthAuthorization(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	digest := bytes.Repeat([]byte{1}, 32)
	require.NoError(t, store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		digest,
		time.Now().Add(time.Minute),
	), "BeginAuthorization()")

	_, err := store.ConsumeAuthorization(t.Context(), "other@example.ts.net", digest)
	require.ErrorIs(t, err, ErrOAuthTransactionIdentityMismatch, "ConsumeAuthorization() with another caller")
	targetID, err := store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", digest)
	require.NoError(t, err, "ConsumeAuthorization()")
	assert.Equal(t, "rider-a", targetID, "ConsumeAuthorization() target")
	_, err = store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", digest)
	require.ErrorIs(t, err, ErrOAuthTransactionUsed, "ConsumeAuthorization() after use")
}

func TestStoreRejectsExpiredOAuthAuthorization(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	digest := bytes.Repeat([]byte{2}, 32)
	require.NoError(t, store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		digest,
		time.Now().Add(time.Minute),
	), "BeginAuthorization()")
	_, err := store.database.ExecContext(
		t.Context(),
		"UPDATE oauth_transactions SET expires_at_unix = ? WHERE state_digest = ?",
		time.Now().Add(-time.Second).Unix(),
		digest,
	)
	require.NoError(t, err, "expiring OAuth authorization")

	_, err = store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", digest)
	require.ErrorIs(t, err, ErrOAuthTransactionExpired, "ConsumeAuthorization()")
}

func TestStoreReportsOnlyLiveAuthorizationsAsPending(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b", "rider-c"}), "EnsureTargets()")
	begin := func(targetID string, digestByte byte) []byte {
		digest := bytes.Repeat([]byte{digestByte}, 32)
		require.NoError(t, store.BeginAuthorization(
			t.Context(),
			targetID,
			"rider@example.ts.net",
			digest,
			time.Now().Add(time.Minute),
		), "BeginAuthorization()")

		return digest
	}
	begin("rider-a", 3)
	consumed := begin("rider-b", 4)
	expired := begin("rider-c", 5)
	_, err := store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", consumed)
	require.NoError(t, err, "ConsumeAuthorization()")
	_, err = store.database.ExecContext(
		t.Context(),
		"UPDATE oauth_transactions SET expires_at_unix = ? WHERE state_digest = ?",
		time.Now().Add(-time.Second).Unix(),
		expired,
	)
	require.NoError(t, err, "expiring OAuth authorization")

	var pending []string
	require.NoError(t, store.ForEachPendingAuthorization(t.Context(), func(targetID string) error {
		pending = append(pending, targetID)

		return nil
	}), "ForEachPendingAuthorization()")

	// A flow that was completed and one that ran out of time are both over, and
	// a slot waiting on neither must not read as one an operator is midway
	// through connecting.
	assert.Equal(t, []string{"rider-a"}, pending, "pending target slots")
}

// The visitor is this method's entire output, so a caller that supplied none and
// a visitor that fails partway both have to be answered rather than iterated
// past: the status view reads a slot as pending on the strength of a visit, and
// a swallowed failure would report a half-read table as a whole one.
func TestStoreStopsReadingPendingAuthorizationsOnVisitorFailure(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	require.NoError(t, store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		bytes.Repeat([]byte{6}, 32),
		time.Now().Add(time.Minute),
	), "BeginAuthorization()")

	require.Error(
		t,
		store.ForEachPendingAuthorization(t.Context(), nil),
		"ForEachPendingAuthorization() without a visitor",
	)

	visitErr := errors.New("visiting pending authorization")
	assert.ErrorIs(t, store.ForEachPendingAuthorization(t.Context(), func(string) error {
		return visitErr
	}), visitErr, "ForEachPendingAuthorization() with a failing visitor")
}

func TestStorePersistsTrustedInventoryAndTargetStages(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	count, err := store.TrustedInventoryCount(t.Context())
	require.NoError(t, err, "TrustedInventoryCount()")
	assert.Equal(t, 1, count, "TrustedInventoryCount()")
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", 1, 1, "revision", "content-hash", 42), "UpsertTargetStage()")

	var got []string
	require.NoError(t, store.ForEachTargetStage(
		t.Context(),
		"rider-a",
		func(routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error {
			got = append(got, fmt.Sprintf("%d/%d/%s/%s/%d", routeID, stageOrder, sourceRevision, contentHash, wahooRouteID))

			return nil
		},
	), "ForEachTargetStage()")
	assert.Equal(t, []string{"1/1/revision/content-hash/42"}, got, "target mappings")
	require.NoError(t, store.DeleteTargetStage(t.Context(), "rider-a", 1, 1), "DeleteTargetStage()")
	require.NoError(t, store.ForEachTargetStage(t.Context(), "rider-a", func(int64, int, string, string, int64) error {
		assert.Fail(t, "ForEachTargetStage() invoked the visitor after deletion")

		return nil
	}), "ForEachTargetStage() after deletion")
}

func TestStoreRecordsRunsAndFailureNotificationState(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	require.NoError(t, store.RecordSyncRun(
		t.Context(),
		"targets",
		startedAt,
		finishedAt,
		"succeeded",
		"",
		3,
		2,
		1,
		0,
	), "RecordSyncRun()")

	var (
		outcome      string
		detail       string
		sourceStages int
		created      int
		updated      int
		deleted      int
	)
	require.NoError(t, store.database.QueryRowContext(t.Context(), `
		SELECT outcome, detail, source_stages, created, updated, deleted FROM sync_runs
	`).Scan(&outcome, &detail, &sourceStages, &created, &updated, &deleted), "querying sync run")
	assert.Equal(t, "succeeded//3/2/1/0", fmt.Sprintf("%s/%s/%d/%d/%d/%d", outcome, detail, sourceStages, created, updated, deleted), "stored sync run")
	_, found, err := store.LastFailureNotification(t.Context(), "destination")
	require.NoError(t, err, "LastFailureNotification()")
	assert.False(t, found, "a notification was recorded before one was sent")
	require.NoError(t, store.RecordFailureNotification(t.Context(), "destination", finishedAt), "RecordFailureNotification()")
	sentAt, found, err := store.LastFailureNotification(t.Context(), "destination")
	require.NoError(t, err, "LastFailureNotification()")
	require.True(t, found, "the notification that was recorded is not readable")
	assert.WithinDuration(t, finishedAt, sentAt, 0, "LastFailureNotification()")
}

func TestStoreMigrationsAreIdempotent(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	first, firstOpenErr := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, firstOpenErr, "first Open()")
	require.NoError(t, first.Close(), "first Close()")

	second, secondOpenErr := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, secondOpenErr, "second Open()")
	t.Cleanup(func() {
		assert.NoError(t, second.Close(), "second Close()")
	})

	var version int
	require.NoError(t, second.database.QueryRowContext(t.Context(), "SELECT MAX(version) FROM schema_migrations").Scan(&version), "query migration version")
	assert.Equal(t, len(schemaMigrations()), version, "schema version")
}

// The one guard that catches a migration inserted into shipped history.
//
// Every other test here builds its database from the list as it stands now, so a
// list whose elements have been reordered still migrates cleanly from any prefix
// of itself. A deployment cannot: it recorded a count against the old order, so
// element N must still be the migration that shipped as N. The fingerprints in
// testdata are that record. Appending a migration means appending one line;
// changing a line means an already-applied migration has been rewritten, which
// no deployment will ever re-run.
func TestStoreMigrationHistoryIsAppendOnly(t *testing.T) {
	recorded, err := os.ReadFile(filepath.Join("testdata", "schema-migrations.sha256"))
	require.NoError(t, err)

	want := strings.Fields(strings.TrimSpace(string(recorded)))
	got := make([]string, 0, len(want))
	for _, statements := range schemaMigrations() {
		digest := sha256.Sum256([]byte(strings.Join(statements, "\n")))
		got = append(got, hex.EncodeToString(digest[:]))
	}

	require.Len(t, got, len(want),
		"a migration was added or removed; append its fingerprint to testdata/schema-migrations.sha256")
	for index := range want {
		assert.Equal(t, want[index], got[index],
			"migration %d is not the one that shipped as %d", index+1, index+1)
	}
}

// A deployment upgrades from whatever version it is on, and every version this
// service has ever shipped is still out there in somebody's volume. Opening a
// database at each earlier version is what proves the history is still
// append-only: insert a migration rather than append one, and the deployment
// that already applied the old numbering re-runs the migration that took its
// place. That is a startup failure on exactly the databases carrying the
// operator's data, and every other test here migrates an empty file and passes.
func TestStoreUpgradesFromEveryEarlierVersion(t *testing.T) {
	migrations := schemaMigrations()
	for version := 1; version < len(migrations); version++ {
		t.Run(fmt.Sprintf("version %d", version), func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "state.db")
			seedSchemaVersion(t, databasePath, version)

			store, err := Open(t.Context(), databasePath, testKey(1))
			require.NoError(t, err, "opening a database left at version %d", version)
			t.Cleanup(func() {
				assert.NoError(t, store.Close())
			})

			var applied int
			require.NoError(t, store.database.QueryRowContext(
				t.Context(),
				"SELECT MAX(version) FROM schema_migrations",
			).Scan(&applied))
			assert.Equal(t, len(migrations), applied)
		})
	}
}

// seedSchemaVersion builds a database that has applied exactly the first
// `version` migrations, as a deployment last started on that release would have.
func seedSchemaVersion(t *testing.T, databasePath string, version int) {
	t.Helper()

	database, err := sql.Open(driverName, databasePath)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, database.Close())
	}()

	_, err = database.ExecContext(t.Context(), `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at_unix INTEGER NOT NULL
		)
	`)
	require.NoError(t, err)
	for applied, statements := range schemaMigrations()[:version] {
		for _, statement := range statements {
			_, err = database.ExecContext(t.Context(), statement)
			require.NoError(t, err, "applying migration %d", applied+1)
		}
		_, err = database.ExecContext(
			t.Context(),
			"INSERT INTO schema_migrations (version, applied_at_unix) VALUES (?, ?)",
			applied+1,
			time.Now().Unix(),
		)
		require.NoError(t, err)
	}
}

func TestStoreMigratesExistingOAuthTransactions(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	database, openErr := sql.Open(driverName, databasePath)
	require.NoError(t, openErr, "opening version one database")
	_, registryErr := database.ExecContext(t.Context(), `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at_unix INTEGER NOT NULL
		)
	`)
	require.NoError(t, registryErr, "creating migration registry")
	for _, statement := range schemaMigrations()[0] {
		_, executeErr := database.ExecContext(t.Context(), statement)
		require.NoError(t, executeErr, "creating version one schema")
	}
	_, insertErr := database.ExecContext(
		t.Context(),
		"INSERT INTO schema_migrations (version, applied_at_unix) VALUES (1, ?)",
		time.Now().Unix(),
	)
	require.NoError(t, insertErr, "recording version one migration")
	require.NoError(t, database.Close(), "closing version one database")

	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err, "Open() after version one")
	t.Cleanup(func() {
		assert.NoError(t, store.Close(), "Close()")
	})
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	require.NoError(t, store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		bytes.Repeat([]byte{3}, 32),
		time.Now().Add(time.Minute),
	), "BeginAuthorization() after migration")
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", 1, 1, "revision", "content-hash", 42), "UpsertTargetStage() after migration")
}

// The stored inventory is what the target phase reconciles from, so it has to
// come back as the same stages that went in, elevation included.
func TestStoreReadsTheTrustedInventoryBackAsStages(t *testing.T) {
	store := openTestStore(t, testKey(1))
	elevation := 128.5
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")

	stages, err := store.TrustedInventory(t.Context())
	require.NoError(t, err, "TrustedInventory()")
	require.Len(t, stages, 1, "stages")
	restored := stages[0]
	assert.Equal(t, stage.Key(), restored.Key(), "key")
	assert.Equal(t, "content-hash", restored.ContentHash(), "content hash")
	assert.Equal(t, "revision", restored.Revision(), "revision")
	assert.Equal(t, "Alpine loop — Descent", restored.Title(), "title")
	points := restored.Geometry()
	require.Len(t, points, 2, "points")
	require.NotNil(t, points[0].Elevation, "the first point lost its elevation")
	assert.InDelta(t, elevation, *points[0].Elevation, 0.001, "first elevation")
	assert.Nil(t, points[1].Elevation, "the second point gained an elevation")
}

// A partial library reads as a library whose missing stages should be deleted,
// so a stage without geometry for its current hash fails the whole read.
func TestStoreRefusesATrustedInventoryMissingGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	_, err := store.database.ExecContext(t.Context(), "DELETE FROM stage_geometry")
	require.NoError(t, err, "clearing geometry cache")

	_, inventoryErr := store.TrustedInventory(t.Context())
	require.Error(t, inventoryErr, "TrustedInventory() described a library it could not read whole")
}

// A reprocess removes the three answers the service would otherwise reuse, and
// keeps the one that says which Wahoo route it already owns.
func TestStoreReprocessesOneStageWithoutLosingItsRouteIdentity(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", 7, 2, "revision", "encoded-hash", 4242), "UpsertTargetStage()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), 7, 2, "content-hash", []byte(`[{"kind":"asphalt","start_index":0,"end_index":1}]`), 100,
	), "StoreStageSurface()")

	found, err := store.RequestStageReprocess(t.Context(), 7, 2)
	require.NoError(t, err, "RequestStageReprocess()")
	require.True(t, found, "RequestStageReprocess() did not find the stored stage")

	var revision, contentHash string
	var wahooRouteID int64
	require.NoError(t, store.database.QueryRowContext(t.Context(), `
		SELECT source_revision, content_hash, wahoo_route_id FROM target_stages
		WHERE target_slot = 'rider-a' AND route_id = 7 AND stage_order = 2
	`).Scan(&revision, &contentHash, &wahooRouteID), "reading the target mapping")
	// Forgotten, but still a usable row: the reconciler rejects a mapping with an
	// empty field outright, which would fail the whole target phase instead of
	// rewriting this one route.
	assert.NotEmpty(t, revision, "the mapping's revision is not a value the reconciler can read")
	assert.NotEmpty(t, contentHash, "the mapping's content hash is not a value the reconciler can read")
	assert.NotEqual(t, "revision", revision, "the mapping still claims the revision it pushed")
	assert.NotEqual(t, "encoded-hash", contentHash, "the mapping still claims the content it pushed")
	// A reprocess rewrites the owned route; it never recreates it.
	assert.Equal(t, int64(4242), wahooRouteID, "wahoo route id")
	_, _, surfaceFound, err := store.StageSurface(t.Context(), 7, 2, "content-hash")
	require.NoError(t, err, "StageSurface()")
	assert.False(t, surfaceFound, "the surface survived a reprocess instead of being asked for again")
}

// The geometry cache skips a stage whose content has not changed. A reprocess is
// the operator saying the derivation itself should be redone.
func TestStoreRewritesGeometryOfAReprocessedStage(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	_, err := store.database.ExecContext(t.Context(), `
		UPDATE stage_geometry SET route_name = 'stale name' WHERE route_id = 7 AND stage_order = 2
	`)
	require.NoError(t, err, "ageing the cached geometry")

	// Without a request, an unchanged stage is left alone.
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	summary, _, _, err := store.StageGeometry(t.Context(), 7, 2)
	require.NoError(t, err, "StageGeometry()")
	// An unchanged stage is not rewritten, so the aged name is still there.
	require.Equal(t, "stale name", summary.RouteName, "route name")

	_, requestErr := store.RequestStageReprocess(t.Context(), 7, 2)
	require.NoError(t, requestErr, "RequestStageReprocess()")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory() after request")
	summary, _, _, err = store.StageGeometry(t.Context(), 7, 2)
	require.NoError(t, err, "StageGeometry()")
	assert.Equal(t, "Alpine loop", summary.RouteName, "the request did not rewrite the route name")

	// The mark is consumed, so the next pass leaves the stage alone again.
	var marks int
	require.NoError(t, store.database.QueryRowContext(t.Context(), "SELECT COUNT(*) FROM stage_reprocess").Scan(&marks), "counting reprocess requests")
	assert.Zero(t, marks, "the pass that honoured the request left it behind")
}

// A stage that is not in the inventory cannot be redone, and a mark nothing will
// consume is worse than an answer.
func TestStoreRefusesToReprocessAnUnknownStage(t *testing.T) {
	store := openTestStore(t, testKey(1))

	found, err := store.RequestStageReprocess(t.Context(), 99, 1)
	require.NoError(t, err, "RequestStageReprocess()")
	assert.False(t, found, "a stage that is not stored was marked for reprocessing")
}

// A classification measured against a shape the stage no longer has describes a
// line the map cannot draw, so it does not count as classified.
func TestStoreCountsOnlyClassificationsOfTheCurrentGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	geometry := []route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.5, Latitude: 49.2}}
	first := storeTestStageWithGeometry(t, 7, 1, "revision", "hash-a", "Alpine loop", "Descent", geometry)
	second := storeTestStageWithGeometry(t, 8, 1, "revision", "hash-b", "Coast road", "Return", geometry)
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{first, second}), "StoreTrustedInventory()")

	classified, total, err := store.SurfaceCoverage(t.Context())
	require.NoError(t, err, "SurfaceCoverage()")
	require.Zero(t, classified, "a stage counted as classified before anything was")
	require.Equal(t, 2, total, "total stages")

	require.NoError(t, store.StoreStageSurface(
		t.Context(), 7, 1, "hash-a", []byte(`[{"kind":"asphalt","start_index":0,"end_index":1}]`), 100,
	), "StoreStageSurface()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), 8, 1, "an-earlier-shape", []byte(`[{"kind":"gravel","start_index":0,"end_index":1}]`), 100,
	), "StoreStageSurface()")

	classified, total, err = store.SurfaceCoverage(t.Context())
	require.NoError(t, err, "SurfaceCoverage()")
	// The stale classification does not count towards the coverage.
	assert.Equal(t, 1, classified, "classified stages")
	assert.Equal(t, 2, total, "total stages")
}

// Both halves are on until an operator says otherwise, which is what every
// deployment did before the switches existed.
func TestStoreSchedulesBothPhasesUntilChanged(t *testing.T) {
	store := openTestStore(t, testKey(1))

	source, targets, err := store.SyncSchedule(t.Context())
	require.NoError(t, err, "SyncSchedule()")
	assert.True(t, source, "the source half is off by default")
	assert.True(t, targets, "the target half is off by default")

	require.NoError(t, store.SetSyncSchedule(t.Context(), false, true), "SetSyncSchedule()")
	source, targets, err = store.SyncSchedule(t.Context())
	require.NoError(t, err, "SyncSchedule() after change")
	assert.False(t, source, "the source half was left on")
	assert.True(t, targets, "the target half was switched off")
}

// Each phase's own last run is what an operator reads; the newest run of the
// other phase answers a different question.
func TestStoreReportsTheLastRunOfEachPhase(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	record := func(phase, outcome string, minute int, sourceStages, created int) {
		t.Helper()
		began := startedAt.Add(time.Duration(minute) * time.Minute)
		require.NoError(t, store.RecordSyncRun(
			t.Context(), phase, began, began.Add(time.Second), outcome, "", sourceStages, created, 0, 0,
		), "RecordSyncRun()")
	}
	record("source", "failed", 0, 0, 0)
	record("source", "succeeded", 1, 12, 0)
	record("targets", "succeeded", 2, 12, 3)

	outcomes := make(map[string]string)
	counts := make(map[string]int)
	require.NoError(t, store.ForEachPhaseRun(t.Context(), func(
		phase string, _ time.Time, outcome, _ string, sourceStages, created, _, _ int,
	) error {
		outcomes[phase] = outcome
		counts[phase] = sourceStages + created

		return nil
	}), "ForEachPhaseRun()")
	assert.Equal(t, "succeeded", outcomes["source"], "source outcome")
	assert.Equal(t, "succeeded", outcomes["targets"], "targets outcome")
	assert.Equal(t, 15, counts["targets"], "target run counts")
}

func TestStoreCachesStageGeometryForTheMapView(t *testing.T) {
	store := openTestStore(t, testKey(1))
	elevation := 128.5
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")

	summary, coordinates, found, err := store.StageGeometry(t.Context(), 7, 2)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stored stage")
	assert.Equal(t, "Alpine loop — Descent", summary.Title(), "Title()")
	assert.Equal(t, 2, summary.PointCount, "PointCount")
	assert.Positive(t, summary.DistanceMetres, "DistanceMetres")
	wantBounds := route.Bounds{MinLongitude: 8.4, MinLatitude: 49.0, MaxLongitude: 8.5, MaxLatitude: 49.2}
	assert.Equal(t, wantBounds, summary.Bounds, "Bounds")
	assert.Equal(t, `[[8.4,49,128.5],[8.5,49.2]]`, string(coordinates), "coordinates")
}

func TestStoreCachesElevationStatistics(t *testing.T) {
	store := openTestStore(t, testKey(1))
	// A climb of 40 m spread over roughly 400 m of northing.
	geometry := make([]route.Point, 0, 5)
	for index := range 5 {
		elevation := 100 + float64(index)*10
		geometry = append(geometry, route.Point{
			Longitude: 8.4,
			Latitude:  49.0 + float64(index)*0.0009,
			Elevation: &elevation,
		})
	}
	stage := storeTestStageWithGeometry(t, 5, 1, "revision", "hash", "Climb", "", geometry)
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")

	summary, _, found, err := store.StageGeometry(t.Context(), 5, 1)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stored stage")
	assert.InDelta(t, 40.0, summary.AscentMetres, 0.001, "AscentMetres")
	assert.Positive(t, summary.MaxGradientPercent, "MaxGradientPercent")

	var listed route.Summary
	require.NoError(t, store.ForEachStageSummary(t.Context(), func(summary route.Summary) error {
		listed = summary

		return nil
	}), "ForEachStageSummary()")
	assert.InDelta(t, summary.AscentMetres, listed.AscentMetres, 0.001, "listed AscentMetres")
}

// A stage cached before the statistics existed must still be readable; the
// columns default to zero until a content change refills them.
func TestStoreReadsGeometryCachedBeforeElevationStatistics(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE stage_geometry SET ascent_metres = 0, max_gradient_percent = 0`)
	require.NoError(t, err, "clearing statistics")

	summary, _, found, err := store.StageGeometry(t.Context(), 1, 1)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stored stage")
	assert.Zero(t, summary.AscentMetres, "AscentMetres")
	assert.Zero(t, summary.MaxGradientPercent, "MaxGradientPercent")
}

func TestStoreDoesNotRewriteUnchangedStageGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	// A sentinel that a rewrite would necessarily overwrite. This is the
	// write-amplification guarantee: an unchanged library must not rewrite the
	// geometry cache on every scheduled run.
	const sentinel = 1
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE stage_geometry SET updated_at_unix = ?`, sentinel)
	require.NoError(t, err, "seeding sentinel")

	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "second StoreTrustedInventory()")
	assert.EqualValues(t, sentinel, stageGeometryUpdatedAt(t, store, 1, 1),
		"an unchanged sync rewrote the geometry cache")

	changed := storeTestStage(t, 1, 1, "revision", "different-content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{changed}), "changed StoreTrustedInventory()")
	assert.NotEqualValues(t, sentinel, stageGeometryUpdatedAt(t, store, 1, 1),
		"a changed content hash left the geometry cache untouched")
}

func TestStorePrunesGeometryForStagesLeavingTheInventory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "hash-one")
	second := storeTestStage(t, 2, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{first, second}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{first}), "second StoreTrustedInventory()")

	_, _, removedFound, removedErr := store.StageGeometry(t.Context(), 2, 1)
	require.NoError(t, removedErr, "StageGeometry() for a removed stage")
	assert.False(t, removedFound, "the geometry of a removed stage is still stored")

	_, _, retainedFound, retainedErr := store.StageGeometry(t.Context(), 1, 1)
	require.NoError(t, retainedErr, "StageGeometry() for a retained stage")
	assert.True(t, retainedFound, "the geometry of a retained stage was dropped")
}

func TestStoreListsStageSummaries(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 3, 1, "revision", "hash", "Sunday", "", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.1},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")

	var summaries []route.Summary
	require.NoError(t, store.ForEachStageSummary(t.Context(), func(summary route.Summary) error {
		summaries = append(summaries, summary)

		return nil
	}), "ForEachStageSummary()")
	require.Len(t, summaries, 1, "listed summaries")
	assert.Equal(t, "Sunday", summaries[0].Title(), "Title()")
	assert.Equal(t, "revision", summaries[0].SourceRevision, "SourceRevision")
	assert.Equal(t, 2, summaries[0].PointCount, "PointCount")
}

func TestStoreCachesStageSurfaceAgainstTheGeometryItDescribes(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 7, 2, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")

	_, found, err := store.StageSurfaceHash(t.Context(), 7, 2)
	require.NoError(t, err, "StageSurfaceHash() before enrichment")
	assert.False(t, found, "a surface hash was stored before enrichment")

	require.NoError(t, store.StoreStageSurface(
		t.Context(), 7, 2, "content-hash", []byte(testSurfaceRanges), 1234.5,
	), "StoreStageSurface()")

	ranges, matchedMetres, found, err := store.StageSurface(t.Context(), 7, 2, "content-hash")
	require.NoError(t, err, "StageSurface()")
	require.True(t, found, "StageSurface() did not find the stored surface")
	// Byte-identical, because the endpoint serves the stored ranges as they are.
	wantRanges := []byte(testSurfaceRanges)
	assert.Equal(t, wantRanges, []byte(ranges), "ranges")
	assert.InDelta(t, 1234.5, matchedMetres, 0.001, "matchedMetres")

	hash, found, err := store.StageSurfaceHash(t.Context(), 7, 2)
	require.NoError(t, err, "StageSurfaceHash()")
	require.True(t, found, "StageSurfaceHash() did not find the stored hash")
	assert.Equal(t, "content-hash", hash, "StageSurfaceHash()")
}

func TestStoreHidesASurfaceMeasuredAgainstOtherGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "current-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), 1, 1, "earlier-hash", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	// The ranges index the coordinates of the geometry they were measured
	// against, so beside a re-planned stage they are absent, never approximate.
	_, _, found, err := store.StageSurface(t.Context(), 1, 1, "current-hash")
	require.NoError(t, err, "StageSurface() for other geometry")
	assert.False(t, found, "ranges measured against replaced geometry were served for it")
	// The hash is still readable, which is how the enrichment pass knows the
	// stage needs asking about again.
	hash, hashFound, err := store.StageSurfaceHash(t.Context(), 1, 1)
	require.NoError(t, err, "StageSurfaceHash()")
	require.True(t, hashFound, "StageSurfaceHash() did not find the stored hash")
	assert.Equal(t, "earlier-hash", hash, "StageSurfaceHash()")
}

func TestStoreReplacesAStageSurfaceRatherThanAccumulatingOne(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "second-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(t.Context(), 1, 1, "first-hash", []byte(`[]`), 1), "first StoreStageSurface()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), 1, 1, "second-hash", []byte(testSurfaceRanges), 2,
	), "second StoreStageSurface()")

	ranges, matchedMetres, found, err := store.StageSurface(t.Context(), 1, 1, "second-hash")
	require.NoError(t, err, "StageSurface()")
	require.True(t, found, "StageSurface() did not find the stored surface")
	wantRanges := []byte(testSurfaceRanges)
	assert.Equal(t, wantRanges, []byte(ranges), "ranges")
	assert.InDelta(t, 2.0, matchedMetres, 0.001, "matchedMetres")
	assert.Equal(t, 1, countStageSurfaceRows(t, store), "stage_surface rows for one stage")
}

func TestStorePrunesSurfaceForStagesLeavingTheInventory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "hash-one")
	second := storeTestStage(t, 2, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{first, second}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), 1, 1, "hash-one", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), 2, 1, "hash-two", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{first}), "second StoreTrustedInventory()")

	_, removedFound, err := store.StageSurfaceHash(t.Context(), 2, 1)
	require.NoError(t, err, "StageSurfaceHash() for a removed stage")
	assert.False(t, removedFound, "the surface hash of a removed stage is still stored")

	_, _, retainedFound, err := store.StageSurface(t.Context(), 1, 1, "hash-one")
	require.NoError(t, err, "StageSurface() for a retained stage")
	assert.True(t, retainedFound, "the surface of a retained stage was dropped")
}

func TestStorePrunesSurfaceMeasuredAgainstReplacedGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "hash-one")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), 1, 1, "hash-one", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	replanned := storeTestStage(t, 1, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), []route.Stage{replanned}), "second StoreTrustedInventory()")

	// The row goes rather than lingering as something to be matched around: the
	// coordinate array its ranges address has been replaced.
	assert.Zero(t, countStageSurfaceRows(t, store), "stage_surface rows after re-planning")
}

// testSurfaceRanges is one stored classification, in the shape the annotator
// writes and the geometry endpoint serves.
const testSurfaceRanges = `[{"kind":"asphalt","start_index":0,"end_index":1}]`

func countStageSurfaceRows(t *testing.T, store *Store) int {
	t.Helper()

	var rows int
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM stage_surface`,
	).Scan(&rows), "counting stage_surface rows")

	return rows
}

func stageGeometryUpdatedAt(t *testing.T, store *Store, routeID int64, stageOrder int) int64 {
	t.Helper()

	var updatedAt int64
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		`SELECT updated_at_unix FROM stage_geometry WHERE route_id = ? AND stage_order = ?`,
		routeID, stageOrder,
	).Scan(&updatedAt), "reading updated_at_unix")

	return updatedAt
}

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

func storeTestStage(t *testing.T, routeID int64, stageOrder int, revision, contentHash string) route.Stage {
	t.Helper()
	stage, err := route.NewStage(
		routeID,
		stageOrder,
		revision,
		"Route",
		"",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.401, Latitude: 49.001}},
		contentHash,
	)
	require.NoError(t, err, "NewStage()")

	return stage
}

func storeTestStageWithGeometry(
	t *testing.T,
	routeID int64,
	stageOrder int,
	revision, contentHash, routeName, stageName string,
	geometry []route.Point,
) route.Stage {
	t.Helper()
	stage, err := route.NewStage(routeID, stageOrder, revision, routeName, stageName, geometry, contentHash)
	require.NoError(t, err, "NewStage()")

	return stage
}

// Convergence is answered from the last attempt per slot, so a second run
// replaces what the first recorded rather than accumulating a history nobody
// reads.
func TestStoreKeepsOnlyTheLastRunOfEachTarget(t *testing.T) {
	store := openTestStore(t, testKey(9))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}))

	first := time.Date(2026, time.August, 18, 6, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	require.NoError(t, store.RecordTargetRun(t.Context(), "rider-a", first, "failed", "destination"))
	require.NoError(t, store.RecordTargetRun(t.Context(), "rider-b", first, "succeeded", ""))
	require.NoError(t, store.RecordTargetRun(t.Context(), "rider-a", second, "succeeded", ""))

	type recorded struct {
		finishedAt time.Time
		id         string
		outcome    string
		detail     string
	}
	var runs []recorded
	require.NoError(t, store.ForEachTargetRun(
		t.Context(),
		func(targetID string, finishedAt time.Time, outcome, detail string) error {
			runs = append(runs, recorded{finishedAt: finishedAt, id: targetID, outcome: outcome, detail: detail})

			return nil
		},
	))
	assert.Equal(t, []recorded{
		{finishedAt: second, id: "rider-a", outcome: "succeeded"},
		{finishedAt: first, id: "rider-b", outcome: "succeeded"},
	}, runs)
}

// A slot that has never been reconciled is absent rather than reported as a run
// that succeeded with nothing to do.
func TestStoreReportsNoRunForAnUnreconciledTarget(t *testing.T) {
	store := openTestStore(t, testKey(10))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}))

	visits := 0
	require.NoError(t, store.ForEachTargetRun(t.Context(), func(string, time.Time, string, string) error {
		visits++

		return nil
	}))
	assert.Zero(t, visits)
}

func TestStoreRefusesAnIncompleteTargetRun(t *testing.T) {
	store := openTestStore(t, testKey(11))
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}))
	finishedAt := time.Date(2026, time.August, 18, 6, 0, 0, 0, time.UTC)

	require.Error(t, store.RecordTargetRun(t.Context(), " ", finishedAt, "succeeded", ""))
	require.Error(t, store.RecordTargetRun(t.Context(), "rider-a", time.Time{}, "succeeded", ""))
	require.Error(t, store.RecordTargetRun(t.Context(), "rider-a", finishedAt, "", ""))
}

// The rollback case: a deploy migrated the state, failed its health gate, and
// the previous binary is put back in front of a database one migration ahead of
// it. That binary has to open the file and keep working, or the rollback leaves
// the host down. Only the recorded version moves here — a real future migration
// would also change the schema, and the compatibility rule below is what keeps
// that change invisible to these writes.
func TestStoreOpensStateOneMigrationAhead(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	seedSchemaVersion(t, databasePath, len(schemaMigrations()))
	recordSchemaVersion(t, databasePath, len(schemaMigrations())+forwardCompatibleMigrations)

	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err, "Open() against a state file one migration ahead")
	t.Cleanup(func() {
		assert.NoError(t, store.Close(), "Close()")
	})

	// What the readiness probe reads and what a sync writes, which together are
	// the difference between a rolled-back host that serves and one that does not.
	require.NoError(t, store.EnsureTargets(t.Context(), []string{"rider-a"}), "EnsureTargets()")
	require.NoError(t, store.ForEachTarget(t.Context(), func(string, string) error { return nil }), "ForEachTarget()")
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", 1, 1, "revision", "content-hash", 42), "UpsertTargetStage()")
}

// The tolerance is bounded on purpose. A binary far enough behind the schema is
// a deployment mistake, and a clear refusal is better than writes against a
// database whose shape it cannot reason about.
func TestStoreRefusesStateTooFarAhead(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	seedSchemaVersion(t, databasePath, len(schemaMigrations()))
	recordSchemaVersion(t, databasePath, len(schemaMigrations())+forwardCompatibleMigrations+1)

	_, err := Open(t.Context(), databasePath, testKey(1))
	require.Error(t, err, "Open() against a state file beyond the tolerated distance")
	assert.Contains(t, err.Error(), schemaAheadMessage, "the refusal must stay recognisable to the deploy script")
}

// The other half of the rollback guarantee. Tolerating a newer schema is only
// safe while every migration leaves the previous release's binary able to read
// and write what it already did, so each migration is compared against the
// schema it was applied to. This is the structural half of the rule documented
// on schemaMigrations: what a migration means by an existing column's values is
// beyond a schema comparison and stays with the author.
func TestNewMigrationsStayReadableByThePreviousRelease(t *testing.T) {
	migrations := schemaMigrations()
	for version := compatibilityRuleFromMigration; version <= len(migrations); version++ {
		t.Run(fmt.Sprintf("migration %d", version), func(t *testing.T) {
			before := readSchemaShape(t, version-1)
			after := readSchemaShape(t, version)

			for table, columns := range before.tables {
				updated, found := after.tables[table]
				require.True(t, found, "migration %d drops or renames table %q", version, table)
				for name, column := range columns {
					changed, stillThere := updated[name]
					require.True(t, stillThere, "migration %d drops or renames %s.%s", version, table, name)
					assert.Equal(t, column, changed, "migration %d redefines %s.%s", version, table, name)
				}
				for name, column := range updated {
					if _, existed := columns[name]; existed {
						continue
					}
					assert.True(t, column.nullable || column.hasDefault,
						"migration %d adds NOT NULL column %s.%s without a default", version, table, name)
				}
				assert.Equal(t, before.checks[table], after.checks[table],
					"migration %d changes the CHECK constraints on %q, which an earlier binary's writes must still satisfy", version, table)
			}
			for name, index := range before.indexes {
				updated, found := after.indexes[name]
				require.True(t, found, "migration %d drops index %q", version, name)
				assert.Equal(t, index, updated, "migration %d redefines index %q", version, name)
			}
			for name, index := range after.indexes {
				if _, existed := before.indexes[name]; existed || !index.unique {
					continue
				}
				_, onAnOldTable := before.tables[index.table]
				assert.False(t, onAnOldTable,
					"migration %d adds UNIQUE index %q to %q, which an earlier binary already writes", version, name, index.table)
			}
		})
	}
}

// compatibilityRuleFromMigration is the first migration the rule above is
// applied to. Migration 2 predates it and breaks it — it adds a UNIQUE index to
// a table version 1 already wrote — and rewriting shipped history to satisfy a
// rule adopted afterwards is the one thing schemaMigrations forbids. Everything
// appended since already complies, so the rule starts where it can hold.
const compatibilityRuleFromMigration = 3

// checkConstraint matches a CHECK constraint's text, with one level of nesting
// inside it, which is as deep as this schema's constraints go.
var checkConstraint = regexp.MustCompile(`(?is)CHECK\s*\((?:[^()]|\([^()]*\))*\)`)

type schemaColumn struct {
	declaredType string
	nullable     bool
	hasDefault   bool
	primaryKey   bool
}

type schemaIndex struct {
	table  string
	unique bool
}

type schemaShape struct {
	tables  map[string]map[string]schemaColumn
	checks  map[string][]string
	indexes map[string]schemaIndex
}

// readSchemaShape builds a database at the given migration count and reads back
// the shape a binary of that release would expect to find. Indexes come from
// PRAGMA index_list rather than sqlite_master, because a UNIQUE declared inside
// a CREATE TABLE has an implicit index and no sqlite_master row of its own.
func readSchemaShape(t *testing.T, version int) schemaShape {
	t.Helper()

	databasePath := filepath.Join(t.TempDir(), "state.db")
	seedSchemaVersion(t, databasePath, version)
	database, err := sql.Open(driverName, databasePath)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, database.Close())
	}()

	shape := schemaShape{
		tables:  make(map[string]map[string]schemaColumn),
		checks:  make(map[string][]string),
		indexes: make(map[string]schemaIndex),
	}
	for name, definition := range readTableDefinitions(t, database) {
		shape.tables[name] = readTableColumns(t, database, name)
		shape.checks[name] = checkConstraint.FindAllString(definition, -1)
		for indexName, unique := range readTableIndexes(t, database, name) {
			shape.indexes[indexName] = schemaIndex{table: name, unique: unique}
		}
	}

	return shape
}

// readTableDefinitions returns every table this service owns, by name and stored
// definition. The migration registry is left out: it is the versioning mechanism
// rather than state a release reads.
func readTableDefinitions(t *testing.T, database *sql.DB) map[string]string {
	t.Helper()

	rows, err := database.QueryContext(t.Context(), `
		SELECT name, COALESCE(sql, '') FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name <> 'schema_migrations'
	`)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, rows.Close())
	}()

	definitions := make(map[string]string)
	for rows.Next() {
		var name, definition string
		require.NoError(t, rows.Scan(&name, &definition))
		definitions[name] = definition
	}
	require.NoError(t, rows.Err())

	return definitions
}

func readTableColumns(t *testing.T, database *sql.DB, table string) map[string]schemaColumn {
	t.Helper()

	rows, err := database.QueryContext(t.Context(), `SELECT name, type, "notnull", dflt_value, pk FROM pragma_table_info(?)`, table)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, rows.Close())
	}()

	columns := make(map[string]schemaColumn)
	for rows.Next() {
		var (
			name         string
			declaredType string
			notNull      int
			defaultValue sql.NullString
			primaryKey   int
		)
		require.NoError(t, rows.Scan(&name, &declaredType, &notNull, &defaultValue, &primaryKey))
		columns[name] = schemaColumn{
			declaredType: declaredType,
			nullable:     notNull == 0,
			// An explicit DEFAULT NULL is recorded as a default but supplies
			// nothing: an earlier binary's insert omits the column, gets NULL,
			// and fails the NOT NULL constraint. That is the case this check
			// exists for, so it does not count as a default.
			hasDefault: defaultValue.Valid && !strings.EqualFold(defaultValue.String, "NULL"),
			primaryKey: primaryKey > 0,
		}
	}
	require.NoError(t, rows.Err())

	return columns
}

func readTableIndexes(t *testing.T, database *sql.DB, table string) map[string]bool {
	t.Helper()

	rows, err := database.QueryContext(t.Context(), `SELECT name, "unique" FROM pragma_index_list(?)`, table)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, rows.Close())
	}()

	indexes := make(map[string]bool)
	for rows.Next() {
		var (
			name   string
			unique int
		)
		require.NoError(t, rows.Scan(&name, &unique))
		indexes[name] = unique == 1
	}
	require.NoError(t, rows.Err())

	return indexes
}

// recordSchemaVersion writes a migration record this binary does not know, which
// is what a database migrated by a later release looks like to it.
func recordSchemaVersion(t *testing.T, databasePath string, version int) {
	t.Helper()

	database, err := sql.Open(driverName, databasePath)
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, database.Close())
	}()
	_, err = database.ExecContext(
		t.Context(),
		"INSERT INTO schema_migrations (version, applied_at_unix) VALUES (?, ?)",
		version,
		time.Now().Unix(),
	)
	require.NoError(t, err)
}

// The deploy script tells a rollback that cannot read its state from one that is
// unhealthy for any other reason by matching this message in the container log.
// A reworded refusal that left the script matching the old text would silently
// go back to reporting the generic failure.
func TestTheDeployScriptRecognisesTheSchemaAheadRefusal(t *testing.T) {
	script, err := os.ReadFile(filepath.Join("..", "..", "deploy", "domestique-deploy.sh"))
	require.NoError(t, err)

	assert.Contains(t, string(script), schemaAheadMessage)
}
