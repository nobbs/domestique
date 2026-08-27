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
	"github.com/nobbs/domestique/internal/runtimeconfig"
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
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	count, err := store.TrustedInventoryCount(t.Context(), route.ProviderVeloPlanner)
	require.NoError(t, err, "TrustedInventoryCount()")
	assert.Equal(t, 1, count, "TrustedInventoryCount()")
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", route.ProviderVeloPlanner, 1, 1, "revision", "content-hash", 42), "UpsertTargetStage()")

	var got []string
	require.NoError(t, store.ForEachTargetStage(
		t.Context(),
		"rider-a",
		func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error {
			got = append(got, fmt.Sprintf("%s/%d/%d/%s/%s/%d", provider, routeID, stageOrder, sourceRevision, contentHash, wahooRouteID))

			return nil
		},
	), "ForEachTargetStage()")
	assert.Equal(t, []string{"veloplanner/1/1/revision/content-hash/42"}, got, "target mappings")
	require.NoError(t, store.DeleteTargetStage(t.Context(), "rider-a", route.ProviderVeloPlanner, 1, 1), "DeleteTargetStage()")
	require.NoError(t, store.ForEachTargetStage(t.Context(), "rider-a", func(route.Provider, int64, int, string, string, int64) error {
		assert.Fail(t, "ForEachTargetStage() invoked the visitor after deletion")

		return nil
	}), "ForEachTargetStage() after deletion")
}

func TestStoreRecordsRunsAndFailureNotificationState(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	reference, err := store.RecordSyncRun(
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
	)
	require.NoError(t, err, "RecordSyncRun()")
	assert.Len(t, reference, 2*syncRunReferenceBytes, "the reference naming the recorded run")

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

// A zero sentAt clears the record instead of recording one: recording and
// clearing are complements of the same row.
func TestStoreRecordFailureNotificationClearsOnAZeroTime(t *testing.T) {
	store := openTestStore(t, testKey(1))
	sentAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)

	require.NoError(t, store.RecordFailureNotification(t.Context(), "source:stale", sentAt), "RecordFailureNotification()")
	_, found, err := store.LastFailureNotification(t.Context(), "source:stale")
	require.NoError(t, err, "LastFailureNotification()")
	require.True(t, found, "the notification that was recorded is not readable")

	require.NoError(t, store.RecordFailureNotification(t.Context(), "source:stale", time.Time{}), "RecordFailureNotification() clear")
	_, found, err = store.LastFailureNotification(t.Context(), "source:stale")
	require.NoError(t, err, "LastFailureNotification()")
	assert.False(t, found, "the cleared notification is still readable")
}

func TestStoreRecordFailureNotificationRequiresACategory(t *testing.T) {
	store := openTestStore(t, testKey(1))

	require.Error(t, store.RecordFailureNotification(t.Context(), "", time.Time{}), "RecordFailureNotification() accepted an empty category")
}

func TestStoreRecordFailureNotificationReportsAnUnreadableDatabaseWhenClearing(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, store.RecordFailureNotification(t.Context(), "source:stale", time.Time{}), "RecordFailureNotification() clear on a closed database")
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
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", route.ProviderVeloPlanner, 1, 1, "revision", "content-hash", 42), "UpsertTargetStage() after migration")
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
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")

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

// Storing one provider's inventory must never disturb another's: a source
// phase that reads several sources replaces each one's stored stages on its
// own, so a source that failed to read this run keeps what it last had.
func TestStoreTrustedInventoryIsScopedToItsProvider(t *testing.T) {
	const secondProvider route.Provider = "second-provider"
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "content-hash")
	second, err := route.NewStage(
		secondProvider, 1, 1, "revision", "Route", "",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.401, Latitude: 49.001}},
		"content-hash",
	)
	require.NoError(t, err, "NewStage()")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{first}), "StoreTrustedInventory() first provider")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), secondProvider, []route.Stage{second}), "StoreTrustedInventory() second provider")

	firstCount, err := store.TrustedInventoryCount(t.Context(), route.ProviderVeloPlanner)
	require.NoError(t, err, "TrustedInventoryCount() first provider")
	assert.Equal(t, 1, firstCount, "first provider count")
	secondCount, err := store.TrustedInventoryCount(t.Context(), secondProvider)
	require.NoError(t, err, "TrustedInventoryCount() second provider")
	assert.Equal(t, 1, secondCount, "second provider count")

	stages, err := store.TrustedInventory(t.Context())
	require.NoError(t, err, "TrustedInventory()")
	assert.ElementsMatch(t, []route.Key{first.Key(), second.Key()}, []route.Key{stages[0].Key(), stages[1].Key()}, "union of both providers")

	// Replacing the first provider's inventory must not touch the second's.
	replacement := storeTestStage(t, 2, 1, "replacement", "replacement-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{replacement}), "StoreTrustedInventory() replacing first provider")

	secondCountAfter, err := store.TrustedInventoryCount(t.Context(), secondProvider)
	require.NoError(t, err, "TrustedInventoryCount() second provider after replacement")
	assert.Equal(t, 1, secondCountAfter, "second provider count is unaffected by the first provider's replacement")
	_, _, _, secondGeometryFound, err := store.StageGeometry(t.Context(), secondProvider, 1, 1)
	require.NoError(t, err, "StageGeometry() second provider")
	assert.True(t, secondGeometryFound, "the second provider's geometry cache must survive the first provider's replacement")
}

// A stage claiming a provider other than the one it is being stored under
// would let one source's write corrupt another's scoped share.
func TestStoreRefusesATrustedInventoryStageUnderTheWrongProvider(t *testing.T) {
	const secondProvider route.Provider = "second-provider"
	store := openTestStore(t, testKey(1))
	mismatched := storeTestStage(t, 1, 1, "revision", "content-hash")

	err := store.StoreTrustedInventory(t.Context(), secondProvider, []route.Stage{mismatched})
	require.Error(t, err, "StoreTrustedInventory() with a stage under the wrong provider")
}

// A partial library reads as a library whose missing stages should be deleted,
// so a stage without geometry for its current hash fails the whole read.
func TestStoreRefusesATrustedInventoryMissingGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
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
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", route.ProviderVeloPlanner, 7, 2, "revision", "encoded-hash", 4242), "UpsertTargetStage()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 7, 2, "content-hash", "index-gen", []byte(`[{"kind":"asphalt","startIndex":0,"endIndex":1}]`), 100,
	), "StoreStageSurface()")

	found, err := store.RequestStageReprocess(t.Context(), route.ProviderVeloPlanner, 7, 2)
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
	_, _, surfaceFound, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 7, 2, "content-hash")
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
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	_, err := store.database.ExecContext(t.Context(), `
		UPDATE stage_geometry SET route_name = 'stale name' WHERE route_id = 7 AND stage_order = 2
	`)
	require.NoError(t, err, "ageing the cached geometry")

	// Without a request, an unchanged stage is left alone.
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	summary, _, _, _, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageGeometry()")
	// An unchanged stage is not rewritten, so the aged name is still there.
	require.Equal(t, "stale name", summary.RouteName, "route name")

	_, requestErr := store.RequestStageReprocess(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, requestErr, "RequestStageReprocess()")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory() after request")
	summary, _, _, _, err = store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 7, 2)
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

	found, err := store.RequestStageReprocess(t.Context(), route.ProviderVeloPlanner, 99, 1)
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
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{first, second}), "StoreTrustedInventory()")

	classified, total, err := store.SurfaceCoverage(t.Context())
	require.NoError(t, err, "SurfaceCoverage()")
	require.Zero(t, classified, "a stage counted as classified before anything was")
	require.Equal(t, 2, total, "total stages")

	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 7, 1, "hash-a", "index-gen", []byte(`[{"kind":"asphalt","startIndex":0,"endIndex":1}]`), 100,
	), "StoreStageSurface()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 8, 1, "an-earlier-shape", "index-gen", []byte(`[{"kind":"gravel","startIndex":0,"endIndex":1}]`), 100,
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

// The seeded settings are the defaults the configuration file documented for
// the same keys, so an upgraded deployment that changes nothing runs on exactly
// what it ran on before.
func TestStoreSeedsTheDocumentedRuntimeSettings(t *testing.T) {
	store := openTestStore(t, testKey(1))

	values, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings()")

	assert.False(t, values.Sync.AllowEmptySourceDeletion, "final-library deletion is denied until asked for")
	assert.Equal(t, 24*time.Hour, values.Sync.StaleAfter, "Sync.StaleAfter")
	assert.True(t, values.Notifications.Enabled, "notifications are on")
	assert.Equal(t, runtimeconfig.SuccessPolicyEvery, values.Notifications.Policy, "Notifications.Policy")
	assert.Equal(t, 24*time.Hour, values.Notifications.DigestInterval, "Notifications.DigestInterval")
	assert.Equal(t, "https://api.pushover.net", values.Notifications.PushoverBaseURL, "Notifications.PushoverBaseURL")
	require.Len(t, values.Basemaps, 1, "Basemaps")
	assert.Equal(t, "Streets", values.Basemaps[0].Name, "Basemaps[0].Name")
	assert.Equal(t, "https://tiles.openfreemap.org/styles/bright", values.Basemaps[0].StyleURL, "Basemaps[0].StyleURL")
	assert.Empty(t, values.Surface.Regions, "surface classification is off until regions are named")
	assert.Equal(t, 7*24*time.Hour, values.Surface.RebuildInterval, "Surface.RebuildInterval")
}

func TestStoreKeepsTheRuntimeSettingsItWasGiven(t *testing.T) {
	store := openTestStore(t, testKey(1))

	next := runtimeconfig.Values{
		Sync: runtimeconfig.Sync{AllowEmptySourceDeletion: true, StaleAfter: 90 * time.Minute},
		Notifications: runtimeconfig.Notifications{
			Enabled:         false,
			Policy:          runtimeconfig.SuccessPolicyDigest,
			DigestInterval:  6 * time.Hour,
			PushoverBaseURL: "https://pushover.example.test",
		},
		Basemaps: []runtimeconfig.Basemap{
			{Name: "Streets", StyleURL: "https://tiles.example.test/bright"},
			{Name: "Satellite", StyleURL: "https://imagery.example.test/style.json", DarkCartography: true},
		},
		Surface: runtimeconfig.Surface{
			Regions:         []string{"europe/germany/rheinland-pfalz", "europe/france"},
			RebuildInterval: 48 * time.Hour,
		},
	}
	require.NoError(t, store.SetRuntimeSettings(t.Context(), next), "SetRuntimeSettings()")

	stored, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings() after the write")
	assert.Equal(t, next, stored, "what was written is what is read back, in the order it was arranged in")
}

// A shorter list is the whole list. Rewriting it must remove the entries that
// are gone rather than leave them behind at their old positions.
func TestStoreReplacesTheRuntimeListsWhole(t *testing.T) {
	store := openTestStore(t, testKey(1))
	values, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings()")

	values.Basemaps = append(values.Basemaps, runtimeconfig.Basemap{
		Name:     "Satellite",
		StyleURL: "https://imagery.example.test/style.json",
	})
	values.Surface.Regions = []string{"europe/germany", "europe/france"}
	require.NoError(t, store.SetRuntimeSettings(t.Context(), values), "SetRuntimeSettings()")

	values.Basemaps = values.Basemaps[:1]
	values.Surface.Regions = nil
	require.NoError(t, store.SetRuntimeSettings(t.Context(), values), "SetRuntimeSettings() with shorter lists")

	stored, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings() after the second write")
	assert.Len(t, stored.Basemaps, 1, "the removed basemap is gone")
	assert.Empty(t, stored.Surface.Regions, "the removed regions are gone")
}

// Each phase's own last run is what an operator reads; the newest run of the
// other phase answers a different question.
func TestStoreReportsTheLastRunOfEachPhase(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	record := func(phase, outcome string, minute int, sourceStages, created int) {
		t.Helper()
		began := startedAt.Add(time.Duration(minute) * time.Minute)
		_, err := store.RecordSyncRun(
			t.Context(), phase, began, began.Add(time.Second), outcome, "", sourceStages, created, 0, 0,
		)
		require.NoError(t, err, "RecordSyncRun()")
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

// The history is what an operator reads back after a notification, so a run and
// the record of it must be the same run, and the page must come back newest
// first with a cursor that continues where it stopped.
func TestStoreReadsTheRecordedHistoryOnePageAtATime(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	references := make([]string, 0, 5)
	for minute := range 5 {
		began := startedAt.Add(time.Duration(minute) * time.Minute)
		reference, err := store.RecordSyncRun(
			t.Context(), "source", began, began.Add(time.Second), "succeeded", "", minute, 0, 0, 0,
		)
		require.NoError(t, err, "RecordSyncRun()")
		references = append(references, reference)
	}

	page, next := readSyncRunPage(t, store, "", 2)
	assert.Equal(t, []string{references[4], references[3]}, page, "the newest page, newest first")
	require.NotEmpty(t, next, "a cursor for the runs before that page")

	page, next = readSyncRunPage(t, store, next, 2)
	assert.Equal(t, []string{references[2], references[1]}, page, "the page after the cursor")
	require.NotEmpty(t, next, "a cursor for the runs before that page")

	page, next = readSyncRunPage(t, store, next, 2)
	assert.Equal(t, []string{references[0]}, page, "the oldest page")
	assert.Empty(t, next, "a cursor past the oldest recorded run")
}

// A cursor this store did not issue is a client mistake rather than an empty
// history, and it is reported as one so the caller can say so. A number is not
// enough to be one: a position past the newest run would silently serve the
// first page again, which reads as a history that starts over.
func TestStoreRefusesACursorItDidNotIssue(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	_, err := store.RecordSyncRun(
		t.Context(), "source", startedAt, startedAt.Add(time.Second), "succeeded", "", 3, 0, 0, 0,
	)
	require.NoError(t, err, "RecordSyncRun()")

	for _, cursor := range []string{"the-newest-one", "0", "-1", "999999999"} {
		visited := 0
		next, usable, err := store.ForEachSyncRun(t.Context(), cursor, 10, func(
			string, string, time.Time, string, string, int, int, int, int,
		) error {
			visited++

			return nil
		})
		require.NoError(t, err, "ForEachSyncRun(%q)", cursor)
		assert.False(t, usable, "ForEachSyncRun(%q) accepted a cursor it did not issue", cursor)
		assert.Empty(t, next, "a cursor served under %q", cursor)
		assert.Zero(t, visited, "runs visited under %q", cursor)
	}
}

// The visitor is this method's entire output, and the page size decides how much
// of the table it reads, so a caller that supplied neither is answered rather
// than served an empty history it would read as "nothing has run". A visitor
// that fails partway stops the page for the same reason: a swallowed failure
// would serve half a page as a whole one.
func TestStoreStopsReadingTheHistoryOnVisitorFailure(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	_, err := store.RecordSyncRun(
		t.Context(), "source", startedAt, startedAt.Add(time.Second), "succeeded", "", 3, 0, 0, 0,
	)
	require.NoError(t, err, "RecordSyncRun()")

	_, _, err = store.ForEachSyncRun(t.Context(), "", 10, nil)
	require.Error(t, err, "ForEachSyncRun() without a visitor")

	_, _, err = store.ForEachSyncRun(t.Context(), "", 0, func(
		string, string, time.Time, string, string, int, int, int, int,
	) error {
		return nil
	})
	require.Error(t, err, "ForEachSyncRun() without a page size")

	visitErr := errors.New("visiting sync run")
	_, _, err = store.ForEachSyncRun(t.Context(), "", 10, func(
		string, string, time.Time, string, string, int, int, int, int,
	) error {
		return visitErr
	})
	assert.ErrorIs(t, err, visitErr, "ForEachSyncRun() with a failing visitor")
}

// A run is what the history and the status response are both read from, so a
// record that could not describe one is refused rather than stored as a row that
// reports nothing.
func TestStoreRefusesAnIncompleteSyncRun(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Second)

	_, err := store.RecordSyncRun(t.Context(), "", startedAt, finishedAt, "succeeded", "", 0, 0, 0, 0)
	require.Error(t, err, "RecordSyncRun() without a phase")

	_, err = store.RecordSyncRun(t.Context(), "source", startedAt, startedAt.Add(-time.Second), "succeeded", "", 0, 0, 0, 0)
	require.Error(t, err, "RecordSyncRun() finishing before it started")

	_, err = store.RecordSyncRun(t.Context(), "source", startedAt, finishedAt, "", "", 0, 0, 0, 0)
	require.Error(t, err, "RecordSyncRun() without an outcome")

	_, err = store.RecordSyncRun(t.Context(), "source", startedAt, finishedAt, "succeeded", "", 0, -1, 0, 0)
	require.Error(t, err, "RecordSyncRun() with a negative count")
}

// Runs are recorded forever on a service that is deployed forever, so the
// history is bounded. What it must never drop is the newest run of a half: the
// status response reads that as what the half last came to, and a half switched
// off while the other keeps running would otherwise lose its answer.
func TestStoreBoundsTheRecordedHistoryAndKeepsEachPhasesLastRun(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	record := func(phase string, minute int) {
		t.Helper()
		began := startedAt.Add(time.Duration(minute) * time.Minute)
		_, err := store.RecordSyncRun(
			t.Context(), phase, began, began.Add(time.Second), "succeeded", "", 0, 0, 0, 0,
		)
		require.NoError(t, err, "RecordSyncRun()")
	}
	record("targets", 0)
	for minute := 1; minute <= retainedSyncRuns+10; minute++ {
		record("source", minute)
	}

	var runs, targetRuns int
	require.NoError(t, store.database.QueryRowContext(t.Context(), `
		SELECT COUNT(*), COUNT(*) FILTER (WHERE phase = 'targets') FROM sync_runs
	`).Scan(&runs, &targetRuns), "counting retained runs")
	assert.Equal(t, retainedSyncRuns+1, runs, "retained runs, plus the target half's last one")
	assert.Equal(t, 1, targetRuns, "the target half's last run was pruned with the rest")
}

// syncRunReferenceVersion is the migration that named the recorded runs. It is
// pinned rather than counted from the end of the list, because what the test
// below needs is the schema immediately before that one migration, which stops
// being the last one the moment another is appended.
const syncRunReferenceVersion = 11

// A deployment upgrading into this feature has a history already, and it is as
// addressable as anything recorded after the upgrade.
func TestStoreNamesRunsRecordedBeforeReferencesExisted(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	seedSchemaVersion(t, databasePath, syncRunReferenceVersion-1)
	database, err := sql.Open(driverName, databasePath)
	require.NoError(t, err, "opening the seeded database")
	_, err = database.ExecContext(t.Context(), `
		INSERT INTO sync_runs (phase, started_at_unix, finished_at_unix, outcome, detail)
		VALUES ('source', 100, 160, 'succeeded', '')
	`)
	require.NoError(t, err, "recording a run under the earlier schema")
	require.NoError(t, database.Close(), "closing the seeded database")

	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err, "Open()")
	t.Cleanup(func() {
		assert.NoError(t, store.Close(), "Close()")
	})

	page, _ := readSyncRunPage(t, store, "", 10)
	require.Len(t, page, 1, "the run recorded under the earlier schema")
	assert.Len(t, page[0], 2*syncRunReferenceBytes, "the reference the migration gave it")
}

// readSyncRunPage collects one page of the recorded history as the references
// it names, newest first, with the cursor for the page after it.
// Runs recorded before the history was split by phase keep an empty phase, and
// they are not served: the history reports one half or the other, and a run
// that covered both at once cannot be called either without saying something
// untrue. The page they are excluded from still fills to its limit, because the
// exclusion happens where the page is read rather than after it.
func TestStoreLeavesPrePhaseRunsOutOfTheHistory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	legacy, err := store.RecordSyncRun(
		t.Context(), "source", startedAt, startedAt.Add(time.Second), "succeeded", "", 1, 0, 0, 0,
	)
	require.NoError(t, err, "RecordSyncRun()")
	// The state the phase migration leaves a pre-existing row in.
	_, err = store.database.ExecContext(
		t.Context(), "UPDATE sync_runs SET phase = '' WHERE reference = ?", legacy,
	)
	require.NoError(t, err, "ageing a run back to the pre-phase shape")

	recent := make([]string, 0, 2)
	for minute := range 2 {
		began := startedAt.Add(time.Duration(minute+1) * time.Minute)
		reference, recordErr := store.RecordSyncRun(
			t.Context(), "targets", began, began.Add(time.Second), "succeeded", "", 0, minute, 0, 0,
		)
		require.NoError(t, recordErr, "RecordSyncRun()")
		recent = append(recent, reference)
	}

	page, next := readSyncRunPage(t, store, "", 10)
	assert.Equal(t, []string{recent[1], recent[0]}, page, "the phased runs, newest first")
	assert.NotContains(t, page, legacy, "a run from before the history was split by phase")
	assert.Empty(t, next, "a cursor past the oldest servable run")
}

func readSyncRunPage(t *testing.T, store *Store, after string, limit int) (references []string, next string) {
	t.Helper()

	next, usable, err := store.ForEachSyncRun(t.Context(), after, limit, func(
		reference, _ string, _ time.Time, _, _ string, _, _, _, _ int,
	) error {
		references = append(references, reference)

		return nil
	})
	require.NoError(t, err, "ForEachSyncRun()")
	require.True(t, usable, "ForEachSyncRun() rejected a cursor it issued")

	return references, next
}

func TestStoreCachesStageGeometryForTheMapView(t *testing.T) {
	store := openTestStore(t, testKey(1))
	elevation := 128.5
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
		{Longitude: 8.5, Latitude: 49.2},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")

	summary, coordinates, _, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 7, 2)
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
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")

	summary, _, _, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 5, 1)
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
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE stage_geometry SET ascent_metres = 0, max_gradient_percent = 0`)
	require.NoError(t, err, "clearing statistics")

	summary, _, _, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stored stage")
	assert.Zero(t, summary.AscentMetres, "AscentMetres")
	assert.Zero(t, summary.MaxGradientPercent, "MaxGradientPercent")
}

func TestStoreDoesNotRewriteUnchangedStageGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	// A sentinel that a rewrite would necessarily overwrite. This is the
	// write-amplification guarantee: an unchanged library must not rewrite the
	// geometry cache on every scheduled run.
	const sentinel = 1
	_, err := store.database.ExecContext(t.Context(),
		`UPDATE stage_geometry SET updated_at_unix = ?`, sentinel)
	require.NoError(t, err, "seeding sentinel")

	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "second StoreTrustedInventory()")
	assert.EqualValues(t, sentinel, stageGeometryUpdatedAt(t, store, 1, 1),
		"an unchanged sync rewrote the geometry cache")

	changed := storeTestStage(t, 1, 1, "revision", "different-content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{changed}), "changed StoreTrustedInventory()")
	assert.NotEqualValues(t, sentinel, stageGeometryUpdatedAt(t, store, 1, 1),
		"a changed content hash left the geometry cache untouched")
}

func TestStorePrunesGeometryForStagesLeavingTheInventory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "hash-one")
	second := storeTestStage(t, 2, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{first, second}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{first}), "second StoreTrustedInventory()")

	_, _, _, removedFound, removedErr := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 2, 1)
	require.NoError(t, removedErr, "StageGeometry() for a removed stage")
	assert.False(t, removedFound, "the geometry of a removed stage is still stored")

	_, _, _, retainedFound, retainedErr := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, retainedErr, "StageGeometry() for a retained stage")
	assert.True(t, retainedFound, "the geometry of a retained stage was dropped")
}

func TestStoreListsStageSummaries(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 3, 1, "revision", "hash", "Sunday", "", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.1},
	})
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")

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
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")

	_, _, found, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageSurfaceHash() before enrichment")
	assert.False(t, found, "a surface hash was stored before enrichment")

	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 7, 2, "content-hash", "index-gen", []byte(testSurfaceRanges), 1234.5,
	), "StoreStageSurface()")

	ranges, matchedMetres, found, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 7, 2, "content-hash")
	require.NoError(t, err, "StageSurface()")
	require.True(t, found, "StageSurface() did not find the stored surface")
	// Byte-identical, because the endpoint serves the stored ranges as they are.
	wantRanges := []byte(testSurfaceRanges)
	assert.Equal(t, wantRanges, []byte(ranges), "ranges")
	assert.InDelta(t, 1234.5, matchedMetres, 0.001, "matchedMetres")

	hash, generation, found, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageSurfaceHash()")
	require.True(t, found, "StageSurfaceHash() did not find the stored hash")
	assert.Equal(t, "content-hash", hash, "StageSurfaceHash()")
	assert.Equal(t, "index-gen", generation, "StageSurfaceHash() generation")
}

func TestStoreMigratesStoredSurfaceRangesToCamelCase(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	// The state is seeded one short of the migration that rewrites the field
	// names, which is 16 and stays 16 however many migrations are appended after
	// it. Counting back from the end of the list instead would quietly stop
	// testing this migration the moment another one shipped.
	seedSchemaVersion(t, databasePath, 15)

	database, err := sql.Open(driverName, databasePath)
	require.NoError(t, err, "opening state before the range migration")
	_, err = database.ExecContext(t.Context(), `
		INSERT INTO stage_surface (
			provider, route_id, stage_order, content_hash, index_generation, ranges, matched_metres, updated_at_unix
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, route.ProviderVeloPlanner, 7, 2, "content-hash", "index-gen", `[{"kind":"asphalt","start_index":0,"end_index":1}]`, 1234.5, time.Now().Unix())
	require.NoError(t, err, "storing the prior wire shape")
	require.NoError(t, database.Close(), "closing state before migration")

	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err, "opening state applies the range migration")
	t.Cleanup(func() {
		assert.NoError(t, store.Close(), "closing migrated state")
	})

	ranges, _, found, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 7, 2, "content-hash")
	require.NoError(t, err, "reading migrated ranges")
	require.True(t, found, "migrated surface range")
	assert.JSONEq(t, `[{"kind":"asphalt","startIndex":0,"endIndex":1}]`, string(ranges), "migrated range fields")
}

func TestStoreHidesASurfaceMeasuredAgainstOtherGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "current-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "earlier-hash", "index-gen", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	// The ranges index the coordinates of the geometry they were measured
	// against, so beside a re-planned stage they are absent, never approximate.
	_, _, found, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "current-hash")
	require.NoError(t, err, "StageSurface() for other geometry")
	assert.False(t, found, "ranges measured against replaced geometry were served for it")
	// The hash is still readable, which is how the enrichment pass knows the
	// stage needs asking about again.
	hash, _, hashFound, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageSurfaceHash()")
	require.True(t, hashFound, "StageSurfaceHash() did not find the stored hash")
	assert.Equal(t, "earlier-hash", hash, "StageSurfaceHash()")
}

// The mirror of the test above, and deliberately the opposite answer. A stale
// generation is not a stale geometry: those ranges still index the coordinates
// the stage actually has, so they are old rather than wrong. There is one row
// per stage, so withholding it would serve no surface at all — every rebuild
// would blank the library until enrichment had walked it again. The hash read
// is what notices, and the next pass is what corrects it.
func TestStoreKeepsASurfaceMeasuredAgainstAnEarlierIndex(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "content-hash", "earlier-generation", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	ranges, _, found, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "content-hash")
	require.NoError(t, err, "StageSurface() after a rebuild")
	require.True(t, found, "a stage was blanked because the index had been rebuilt")
	// Byte-identical, as everywhere here: the endpoint serves the stored ranges
	// as they are rather than re-encoding them.
	wantRanges := []byte(testSurfaceRanges)
	assert.Equal(t, wantRanges, []byte(ranges), "ranges")

	// What the enrichment pass compares against the live generation, and so what
	// makes the stage be measured again rather than left as it is forever.
	_, generation, hashFound, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageSurfaceHash()")
	require.True(t, hashFound, "StageSurfaceHash() did not find the stored hash")
	assert.Equal(t, "earlier-generation", generation, "the generation the row was measured against")
}

func TestStoreReplacesAStageSurfaceRatherThanAccumulatingOne(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "second-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "first-hash", "index-gen", []byte(`[]`), 1), "first StoreStageSurface()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "second-hash", "index-gen", []byte(testSurfaceRanges), 2,
	), "second StoreStageSurface()")

	ranges, matchedMetres, found, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "second-hash")
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
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{first, second}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one", "index-gen", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 2, 1, "hash-two", "index-gen", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{first}), "second StoreTrustedInventory()")

	_, _, removedFound, err := store.StageSurfaceHash(t.Context(), route.ProviderVeloPlanner, 2, 1)
	require.NoError(t, err, "StageSurfaceHash() for a removed stage")
	assert.False(t, removedFound, "the surface hash of a removed stage is still stored")

	_, _, retainedFound, err := store.StageSurface(t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one")
	require.NoError(t, err, "StageSurface() for a retained stage")
	assert.True(t, retainedFound, "the surface of a retained stage was dropped")
}

func TestStorePrunesSurfaceMeasuredAgainstReplacedGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "hash-one")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	require.NoError(t, store.StoreStageSurface(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one", "index-gen", []byte(testSurfaceRanges), 10,
	), "StoreStageSurface()")

	replanned := storeTestStage(t, 1, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{replanned}), "second StoreTrustedInventory()")

	// The row goes rather than lingering as something to be matched around: the
	// coordinate array its ranges address has been replaced.
	assert.Zero(t, countStageSurfaceRows(t, store), "stage_surface rows after re-planning")
}

func TestStoreForEachStageSummaryReportsAPredictedMovingTime(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	movingSeconds := 555.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "content-hash", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")

	var found *route.Summary
	require.NoError(t, store.ForEachStageSummary(t.Context(), func(summary route.Summary) error {
		found = &summary

		return nil
	}), "ForEachStageSummary()")
	require.NotNil(t, found, "ForEachStageSummary() visited no stage")
	require.NotNil(t, found.MovingSeconds, "ForEachStageSummary() moving seconds")
	assert.InDelta(t, 555.0, *found.MovingSeconds, 0.001, "moving seconds")
}

func TestStoreCachesStageDurationAgainstTheFingerprintItWasComputedFrom(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 7, 2, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageDurationFingerprint() before prediction")
	assert.False(t, found, "a duration fingerprint was stored before prediction")

	movingSeconds := 987.5
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 7, 2,
		"content-hash", "surface-gen", "coefficient-fingerprint",
		&movingSeconds, []byte(`[0,10,20]`),
	), "StoreStageDuration()")

	contentHash, surfaceGeneration, coefficientFingerprint, found, err := store.StageDurationFingerprint(
		t.Context(), route.ProviderVeloPlanner, 7, 2,
	)
	require.NoError(t, err, "StageDurationFingerprint()")
	require.True(t, found, "StageDurationFingerprint() did not find the stored fingerprint")
	assert.Equal(t, "content-hash", contentHash, "content hash")
	assert.Equal(t, "surface-gen", surfaceGeneration, "surface generation")
	assert.Equal(t, "coefficient-fingerprint", coefficientFingerprint, "coefficient fingerprint")

	summary, _, cumulativeSeconds, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 7, 2)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stage")
	require.NotNil(t, summary.MovingSeconds, "StageGeometry() moving seconds")
	assert.InDelta(t, 987.5, *summary.MovingSeconds, 0.001, "moving seconds")
	assert.JSONEq(t, `[0,10,20]`, string(cumulativeSeconds), "StageGeometry() cumulative seconds")
}

// A stage with no usable elevation is still recorded, with a nil prediction,
// so the next pass does not ask about it again every run — the same reasoning
// stage_surface's own "nothing to report" row follows.
func TestStoreStoresTheAbsenceOfAPrediction(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")

	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "content-hash", "", "coefficient-fingerprint", nil, nil,
	), "StoreStageDuration()")

	contentHash, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint()")
	require.True(t, found, "a stage with no usable elevation was not recorded")
	assert.Equal(t, "content-hash", contentHash, "content hash")

	summary, _, cumulativeSeconds, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stage")
	assert.Nil(t, summary.MovingSeconds, "a stage with no usable elevation must not report a moving time")
	assert.Empty(t, cumulativeSeconds, "a stage with no usable elevation must not report a cumulative series")
}

// A prediction measured against an earlier plan of the same stage addresses
// coordinates that no longer exist, so it must not be served for the stage's
// current geometry, on the same terms StageSurface hides a stale
// classification.
func TestStoreHidesADurationMeasuredAgainstOtherGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "current-hash")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "earlier-hash", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")

	summary, _, cumulativeSeconds, found, err := store.StageGeometry(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageGeometry()")
	require.True(t, found, "StageGeometry() did not find the stage")
	assert.Nil(t, summary.MovingSeconds, "a prediction measured against replaced geometry was served for it")
	assert.Empty(t, cumulativeSeconds, "a cumulative series measured against replaced geometry was served for it")
}

func TestStorePrunesDurationForStagesLeavingTheInventory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "hash-one")
	second := storeTestStage(t, 2, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{first, second}), "StoreTrustedInventory()")
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 2, 1, "hash-two", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")

	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{first}), "second StoreTrustedInventory()")

	_, _, _, removedFound, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 2, 1)
	require.NoError(t, err, "StageDurationFingerprint() for a removed stage")
	assert.False(t, removedFound, "the duration of a removed stage is still stored")

	_, _, _, retainedFound, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint() for a retained stage")
	assert.True(t, retainedFound, "the duration of a retained stage was dropped")
}

func TestStorePrunesDurationMeasuredAgainstReplacedGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "hash-one")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{stage}), "StoreTrustedInventory()")
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash-one", "", "fingerprint", &movingSeconds, nil,
	), "StoreStageDuration()")

	replanned := storeTestStage(t, 1, 1, "revision", "hash-two")
	require.NoError(t, store.StoreTrustedInventory(t.Context(), route.ProviderVeloPlanner, []route.Stage{replanned}), "second StoreTrustedInventory()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint() after re-planning")
	assert.False(t, found, "stage_duration row survived re-planning")
}

func TestStorePruneStageDurationsWithDifferentFingerprintKeepsOnlyTheCurrentOne(t *testing.T) {
	store := openTestStore(t, testKey(1))
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash", "", "current", &movingSeconds, nil,
	), "StoreStageDuration() current")
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 2, 1, "hash", "", "earlier", &movingSeconds, nil,
	), "StoreStageDuration() earlier")

	require.NoError(t, store.PruneStageDurationsWithDifferentFingerprint(t.Context(), "current"), "PruneStageDurationsWithDifferentFingerprint()")

	_, _, _, currentFound, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint() current")
	assert.True(t, currentFound, "a prediction matching the current fingerprint was pruned")

	_, _, _, earlierFound, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 2, 1)
	require.NoError(t, err, "StageDurationFingerprint() earlier")
	assert.False(t, earlierFound, "a prediction from an earlier coefficient file survived pruning")
}

// An empty currentFingerprint is what an unconfigured deployment passes: it
// matches no stored row, so every prediction is pruned — the read path must
// serve nothing, not whatever an earlier configuration left behind.
func TestStorePruneStageDurationsWithDifferentFingerprintClearsEverythingWhenUnconfigured(t *testing.T) {
	store := openTestStore(t, testKey(1))
	movingSeconds := 100.0
	require.NoError(t, store.StoreStageDuration(
		t.Context(), route.ProviderVeloPlanner, 1, 1, "hash", "", "a-since-removed-coefficient-file", &movingSeconds, nil,
	), "StoreStageDuration()")

	require.NoError(t, store.PruneStageDurationsWithDifferentFingerprint(t.Context(), ""), "PruneStageDurationsWithDifferentFingerprint()")

	_, _, _, found, err := store.StageDurationFingerprint(t.Context(), route.ProviderVeloPlanner, 1, 1)
	require.NoError(t, err, "StageDurationFingerprint()")
	assert.False(t, found, "a prediction survived pruning with no fingerprint configured")
}

// testSurfaceRanges is one stored classification, in the shape the annotator
// writes and the geometry endpoint serves.
const testSurfaceRanges = `[{"kind":"asphalt","startIndex":0,"endIndex":1}]`

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
		route.ProviderVeloPlanner,
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
	stage, err := route.NewStage(route.ProviderVeloPlanner, routeID, stageOrder, revision, routeName, stageName, geometry, contentHash)
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
	require.NoError(t, store.UpsertTargetStage(t.Context(), "rider-a", route.ProviderVeloPlanner, 1, 1, "revision", "content-hash", 42), "UpsertTargetStage()")
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

func TestStoreReadsTheLastOutcomeOfEachPhase(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	recordRun := func(phase, outcome string, at time.Time) {
		t.Helper()
		_, err := store.RecordSyncRun(t.Context(), phase, at, at.Add(time.Minute), outcome, "", 0, 0, 0, 0)
		require.NoError(t, err, "RecordSyncRun()")
	}

	_, found, err := store.LastPhaseOutcome(t.Context(), "targets")
	require.NoError(t, err, "LastPhaseOutcome()")
	assert.False(t, found, "a phase reported an outcome before it had run")

	recordRun("targets", "failed", startedAt)
	recordRun("source", "succeeded", startedAt.Add(time.Hour))
	recordRun("targets", "succeeded", startedAt.Add(2*time.Hour))

	// The phases are answered independently, and each answers with its own most
	// recent run rather than the most recent run overall.
	outcome, found, err := store.LastPhaseOutcome(t.Context(), "targets")
	require.NoError(t, err, "LastPhaseOutcome()")
	require.True(t, found, "the recorded targets run is not readable")
	assert.Equal(t, "succeeded", outcome, "LastPhaseOutcome(targets)")

	outcome, _, err = store.LastPhaseOutcome(t.Context(), "source")
	require.NoError(t, err, "LastPhaseOutcome()")
	assert.Equal(t, "succeeded", outcome, "LastPhaseOutcome(source)")
}

func TestStoreTotalsSuccessfulRunsForADigest(t *testing.T) {
	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	recordRun := func(phase, outcome string, created, updated, deleted int) {
		t.Helper()
		_, err := store.RecordSyncRun(t.Context(), phase, at, at, outcome, "", 0, created, updated, deleted)
		require.NoError(t, err, "RecordSyncRun()")
	}
	totalAfter := func(runID int64) (phases []string, created, updated, deleted int, latest int64) {
		t.Helper()
		require.NoError(t, store.ForEachSuccessfulRunAfter(t.Context(), runID,
			func(id int64, phase string, runCreated, runUpdated, runDeleted int) error {
				phases = append(phases, phase)
				created += runCreated
				updated += runUpdated
				deleted += runDeleted
				latest = id

				return nil
			}), "ForEachSuccessfulRunAfter()")

		return phases, created, updated, deleted, latest
	}

	// Every run shares one finished-at second, which is the case a timestamp
	// boundary cannot separate: run 1 is behind the window and runs 2 and 3 are
	// inside it, all indistinguishable by clock. The failure is inside it too and
	// must not be counted.
	recordRun("targets", "succeeded", 9, 9, 9)
	recordRun("source", "succeeded", 0, 0, 0)
	recordRun("targets", "succeeded", 2, 1, 1)
	recordRun("targets", "failed", 7, 7, 7)

	phases, created, updated, deleted, latest := totalAfter(1)
	assert.Equal(t, []string{"source", "targets"}, phases, "visited runs")
	assert.Equal(t, "2/1/1", fmt.Sprintf("%d/%d/%d", created, updated, deleted), "digest totals")
	assert.Equal(t, int64(3), latest, "the last run the window saw")

	// Moving the window to that run leaves nothing behind to count twice.
	phases, _, _, _, _ = totalAfter(latest)
	assert.Empty(t, phases, "a run was counted in two windows")
}

func TestStoreLastSuccessfulPhaseCompletionIgnoresFailuresAndOtherPhases(t *testing.T) {
	store := openTestStore(t, testKey(1))

	_, found, err := store.LastSuccessfulPhaseCompletion(t.Context(), "source")
	require.NoError(t, err, "LastSuccessfulPhaseCompletion()")
	assert.False(t, found, "a completion was reported before any run")

	firstSuccess := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	_, err = store.RecordSyncRun(t.Context(), "source", firstSuccess, firstSuccess, "succeeded", "", 3, 0, 0, 0)
	require.NoError(t, err, "RecordSyncRun()")
	_, err = store.RecordSyncRun(t.Context(), "targets", firstSuccess, firstSuccess, "succeeded", "", 3, 0, 1, 0)
	require.NoError(t, err, "RecordSyncRun()")

	laterFailure := firstSuccess.Add(time.Hour)
	_, err = store.RecordSyncRun(t.Context(), "source", laterFailure, laterFailure, "failed", "state", 0, 0, 0, 0)
	require.NoError(t, err, "RecordSyncRun()")

	completedAt, found, err := store.LastSuccessfulPhaseCompletion(t.Context(), "source")
	require.NoError(t, err, "LastSuccessfulPhaseCompletion()")
	require.True(t, found, "the recorded success was not reported")
	assert.WithinDuration(t, firstSuccess, completedAt, 0, "LastSuccessfulPhaseCompletion() kept the failed run's time")
}

func TestStoreLastSuccessfulPhaseCompletionRequiresAPhase(t *testing.T) {
	store := openTestStore(t, testKey(1))

	_, _, err := store.LastSuccessfulPhaseCompletion(t.Context(), "")
	require.Error(t, err, "LastSuccessfulPhaseCompletion() accepted an empty phase")
}

func TestStoreLastSuccessfulPhaseCompletionReportsAnUnreadableDatabase(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	_, _, err := store.LastSuccessfulPhaseCompletion(t.Context(), "source")
	require.Error(t, err, "LastSuccessfulPhaseCompletion() on a closed database")
}

func TestStoreRecordsDigestNotificationState(t *testing.T) {
	store := openTestStore(t, testKey(1))
	sentAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)

	_, _, found, err := store.LastDigestNotification(t.Context())
	require.NoError(t, err, "LastDigestNotification()")
	assert.False(t, found, "a digest was recorded before one was sent")

	require.NoError(t, store.RecordDigestNotification(t.Context(), sentAt, 7), "RecordDigestNotification()")
	readBack, runID, found, err := store.LastDigestNotification(t.Context())
	require.NoError(t, err, "LastDigestNotification()")
	require.True(t, found, "the digest that was recorded is not readable")
	assert.WithinDuration(t, sentAt, readBack, 0, "LastDigestNotification()")
	assert.Equal(t, int64(7), runID, "the run the digest covered up to")

	// The window moves forward in place: a digest keeps one row, not one per send.
	later := sentAt.Add(24 * time.Hour)
	require.NoError(t, store.RecordDigestNotification(t.Context(), later, 19), "second RecordDigestNotification()")
	readBack, runID, _, err = store.LastDigestNotification(t.Context())
	require.NoError(t, err, "LastDigestNotification()")
	assert.WithinDuration(t, later, readBack, 0, "LastDigestNotification() after the second send")
	assert.Equal(t, int64(19), runID, "the run boundary after the second send")
}

// The digest reads are argument-guarded like every other store method, and the
// visitor's own error stops the walk rather than being counted as the end of it.
func TestStoreRejectsUnusableDigestArguments(t *testing.T) {
	store := openTestStore(t, testKey(1))
	at := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	_, err := store.RecordSyncRun(t.Context(), "targets", at, at, "succeeded", "", 0, 1, 0, 0)
	require.NoError(t, err, "RecordSyncRun()")

	_, _, err = store.LastPhaseOutcome(t.Context(), "")
	require.Error(t, err, "LastPhaseOutcome() accepted no phase")

	require.Error(t, store.RecordDigestNotification(t.Context(), time.Time{}, 0),
		"RecordDigestNotification() accepted no time")

	require.Error(t, store.ForEachSuccessfulRunAfter(t.Context(), 0, nil),
		"ForEachSuccessfulRunAfter() accepted no visitor")

	stop := errors.New("stop")
	require.ErrorIs(t, store.ForEachSuccessfulRunAfter(t.Context(), 0,
		func(int64, string, int, int, int) error { return stop }), stop,
		"the visitor's error did not stop the walk")
}
