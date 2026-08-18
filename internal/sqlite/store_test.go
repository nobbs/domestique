package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

func TestStoreAuthorizesAndEncryptsRefreshToken(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"); err != nil {
		t.Fatalf("AuthorizeTarget() error = %v", err)
	}

	target, err := store.Target(t.Context(), "rider-a")
	if err != nil {
		t.Fatalf("Target() error = %v", err)
	}
	if got, want := target.AuthorizationState, AuthorizationAuthorized; got != want {
		t.Errorf("Target().AuthorizationState = %q, want %q", got, want)
	}
	if got, want := target.WahooUserID, "wahoo-user"; got != want {
		t.Errorf("Target().WahooUserID = %q, want %q", got, want)
	}

	var encrypted []byte
	if queryErr := store.database.QueryRowContext(t.Context(), "SELECT refresh_token FROM targets WHERE slot = ?", "rider-a").Scan(&encrypted); queryErr != nil {
		t.Fatalf("query encrypted token: %v", queryErr)
	}
	if bytes.Contains(encrypted, []byte("refresh-token")) {
		t.Error("database stores refresh token in plaintext")
	}

	got, err := store.RefreshToken(t.Context(), "rider-a")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if want := "refresh-token"; got != want {
		t.Errorf("RefreshToken() = %q, want %q", got, want)
	}
}

func TestStoreRejectsDuplicateWahooUser(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "token-a"); err != nil {
		t.Fatalf("AuthorizeTarget(rider-a) error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-b", "wahoo-user", "token-b"); !errors.Is(err, ErrWahooUserAlreadyAuthorized) {
		t.Errorf("AuthorizeTarget(rider-b) error = %v, want %v", err, ErrWahooUserAlreadyAuthorized)
	}
}

func TestStoreBindsTokenToTarget(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a", "rider-b"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user-a", "token-a"); err != nil {
		t.Fatalf("AuthorizeTarget() error = %v", err)
	}

	var encrypted []byte
	if err := store.database.QueryRowContext(t.Context(), "SELECT refresh_token FROM targets WHERE slot = ?", "rider-a").Scan(&encrypted); err != nil {
		t.Fatalf("query encrypted token: %v", err)
	}
	if _, err := store.database.ExecContext(t.Context(), "UPDATE targets SET refresh_token = ? WHERE slot = ?", encrypted, "rider-b"); err != nil {
		t.Fatalf("copy encrypted token: %v", err)
	}

	_, err := store.RefreshToken(t.Context(), "rider-b")
	if !errors.Is(err, ErrStateUnreadable) {
		t.Errorf("RefreshToken() error = %v, want %v", err, ErrStateUnreadable)
	}
}

func TestStoreRejectsDifferentEncryptionKey(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, openErr := Open(t.Context(), databasePath, testKey(1))
	if openErr != nil {
		t.Fatalf("Open() error = %v", openErr)
	}
	if ensureErr := store.EnsureTargets(t.Context(), []string{"rider-a"}); ensureErr != nil {
		t.Fatalf("EnsureTargets() error = %v", ensureErr)
	}
	if authorizeErr := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"); authorizeErr != nil {
		t.Fatalf("AuthorizeTarget() error = %v", authorizeErr)
	}
	if closeErr := store.Close(); closeErr != nil {
		t.Fatalf("Close() error = %v", closeErr)
	}

	reopened, err := Open(t.Context(), databasePath, testKey(2))
	if err != nil {
		t.Fatalf("Open() with different key error = %v", err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if _, err := reopened.RefreshToken(t.Context(), "rider-a"); !errors.Is(err, ErrStateUnreadable) {
		t.Errorf("RefreshToken() error = %v, want %v", err, ErrStateUnreadable)
	}
}

func TestStoreMarksTargetForReauthorization(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "refresh-token"); err != nil {
		t.Fatalf("AuthorizeTarget() error = %v", err)
	}
	if err := store.MarkNeedsReauthorization(t.Context(), "rider-a"); err != nil {
		t.Fatalf("MarkNeedsReauthorization() error = %v", err)
	}

	target, err := store.Target(t.Context(), "rider-a")
	if err != nil {
		t.Fatalf("Target() error = %v", err)
	}
	if got, want := target.AuthorizationState, AuthorizationNeedsReauthorization; got != want {
		t.Errorf("Target().AuthorizationState = %q, want %q", got, want)
	}
	if _, err := store.RefreshToken(t.Context(), "rider-a"); !errors.Is(err, ErrRefreshTokenUnavailable) {
		t.Errorf("RefreshToken() error = %v, want %v", err, ErrRefreshTokenUnavailable)
	}
}

func TestStoreReplacesRefreshToken(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.AuthorizeTarget(t.Context(), "rider-a", "wahoo-user", "old-refresh-token"); err != nil {
		t.Fatalf("AuthorizeTarget() error = %v", err)
	}
	if err := store.ReplaceRefreshToken(t.Context(), "rider-a", "new-refresh-token"); err != nil {
		t.Fatalf("ReplaceRefreshToken() error = %v", err)
	}

	got, err := store.RefreshToken(t.Context(), "rider-a")
	if err != nil {
		t.Fatalf("RefreshToken() error = %v", err)
	}
	if want := "new-refresh-token"; got != want {
		t.Errorf("RefreshToken() = %q, want %q", got, want)
	}
}

func TestStoreConsumesCallerBoundOAuthAuthorization(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	digest := bytes.Repeat([]byte{1}, 32)
	if err := store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		digest,
		time.Now().Add(time.Minute),
	); err != nil {
		t.Fatalf("BeginAuthorization() error = %v", err)
	}

	if _, err := store.ConsumeAuthorization(t.Context(), "other@example.ts.net", digest); !errors.Is(err, ErrOAuthTransactionIdentityMismatch) {
		t.Fatalf("ConsumeAuthorization() with another caller error = %v, want %v", err, ErrOAuthTransactionIdentityMismatch)
	}
	targetID, err := store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", digest)
	if err != nil {
		t.Fatalf("ConsumeAuthorization() error = %v", err)
	}
	if want := "rider-a"; targetID != want {
		t.Errorf("ConsumeAuthorization() target = %q, want %q", targetID, want)
	}
	if _, err := store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", digest); !errors.Is(err, ErrOAuthTransactionUsed) {
		t.Errorf("ConsumeAuthorization() after use error = %v, want %v", err, ErrOAuthTransactionUsed)
	}
}

func TestStoreRejectsExpiredOAuthAuthorization(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	digest := bytes.Repeat([]byte{2}, 32)
	if err := store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		digest,
		time.Now().Add(time.Minute),
	); err != nil {
		t.Fatalf("BeginAuthorization() error = %v", err)
	}
	if _, err := store.database.ExecContext(
		t.Context(),
		"UPDATE oauth_transactions SET expires_at_unix = ? WHERE state_digest = ?",
		time.Now().Add(-time.Second).Unix(),
		digest,
	); err != nil {
		t.Fatalf("expiring OAuth authorization: %v", err)
	}

	if _, err := store.ConsumeAuthorization(t.Context(), "rider@example.ts.net", digest); !errors.Is(err, ErrOAuthTransactionExpired) {
		t.Errorf("ConsumeAuthorization() error = %v, want %v", err, ErrOAuthTransactionExpired)
	}
}

func TestStorePersistsTrustedInventoryAndTargetStages(t *testing.T) {
	store := openTestStore(t, testKey(1))
	if err := store.EnsureTargets(t.Context(), []string{"rider-a"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}
	count, err := store.TrustedInventoryCount(t.Context())
	if err != nil {
		t.Fatalf("TrustedInventoryCount() error = %v", err)
	}
	if got, want := count, 1; got != want {
		t.Errorf("TrustedInventoryCount() = %d, want %d", got, want)
	}
	if err := store.UpsertTargetStage(t.Context(), "rider-a", 1, 1, "revision", "content-hash", 42); err != nil {
		t.Fatalf("UpsertTargetStage() error = %v", err)
	}

	var got []string
	if err := store.ForEachTargetStage(
		t.Context(),
		"rider-a",
		func(routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error {
			got = append(got, fmt.Sprintf("%d/%d/%s/%s/%d", routeID, stageOrder, sourceRevision, contentHash, wahooRouteID))

			return nil
		},
	); err != nil {
		t.Fatalf("ForEachTargetStage() error = %v", err)
	}
	if want := []string{"1/1/revision/content-hash/42"}; !equalStrings(got, want) {
		t.Errorf("target mappings = %v, want %v", got, want)
	}
	if err := store.DeleteTargetStage(t.Context(), "rider-a", 1, 1); err != nil {
		t.Fatalf("DeleteTargetStage() error = %v", err)
	}
	if err := store.ForEachTargetStage(t.Context(), "rider-a", func(int64, int, string, string, int64) error {
		t.Error("ForEachTargetStage() invoked visitor after deletion")

		return nil
	}); err != nil {
		t.Fatalf("ForEachTargetStage() after deletion error = %v", err)
	}
}

func TestStoreRecordsRunsAndFailureNotificationState(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	finishedAt := startedAt.Add(time.Minute)
	if err := store.RecordSyncRun(
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
	); err != nil {
		t.Fatalf("RecordSyncRun() error = %v", err)
	}

	var (
		outcome      string
		detail       string
		sourceStages int
		created      int
		updated      int
		deleted      int
	)
	if err := store.database.QueryRowContext(t.Context(), `
		SELECT outcome, detail, source_stages, created, updated, deleted FROM sync_runs
	`).Scan(&outcome, &detail, &sourceStages, &created, &updated, &deleted); err != nil {
		t.Fatalf("querying sync run: %v", err)
	}
	if got, want := fmt.Sprintf("%s/%s/%d/%d/%d/%d", outcome, detail, sourceStages, created, updated, deleted), "succeeded//3/2/1/0"; got != want {
		t.Errorf("stored sync run = %q, want %q", got, want)
	}
	if _, found, err := store.LastFailureNotification(t.Context(), "destination"); err != nil || found {
		t.Errorf("LastFailureNotification() = found %t, error %v, want no state", found, err)
	}
	if err := store.RecordFailureNotification(t.Context(), "destination", finishedAt); err != nil {
		t.Fatalf("RecordFailureNotification() error = %v", err)
	}
	sentAt, found, err := store.LastFailureNotification(t.Context(), "destination")
	if err != nil {
		t.Fatalf("LastFailureNotification() error = %v", err)
	}
	if !found {
		t.Fatal("LastFailureNotification() found = false, want true")
	}
	if got, want := sentAt, finishedAt; !got.Equal(want) {
		t.Errorf("LastFailureNotification() = %s, want %s", got, want)
	}
}

func TestStoreMigrationsAreIdempotent(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	first, firstOpenErr := Open(t.Context(), databasePath, testKey(1))
	if firstOpenErr != nil {
		t.Fatalf("first Open() error = %v", firstOpenErr)
	}
	if firstCloseErr := first.Close(); firstCloseErr != nil {
		t.Fatalf("first Close() error = %v", firstCloseErr)
	}

	second, secondOpenErr := Open(t.Context(), databasePath, testKey(1))
	if secondOpenErr != nil {
		t.Fatalf("second Open() error = %v", secondOpenErr)
	}
	t.Cleanup(func() {
		if err := second.Close(); err != nil {
			t.Errorf("second Close() error = %v", err)
		}
	})

	var version int
	if err := second.database.QueryRowContext(t.Context(), "SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil {
		t.Fatalf("query migration version: %v", err)
	}
	if got, want := version, len(schemaMigrations()); got != want {
		t.Errorf("schema version = %d, want %d", got, want)
	}
}

func TestStoreMigratesExistingOAuthTransactions(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	database, openErr := sql.Open(driverName, databasePath)
	if openErr != nil {
		t.Fatalf("opening version one database: %v", openErr)
	}
	if _, registryErr := database.ExecContext(t.Context(), `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at_unix INTEGER NOT NULL
		)
	`); registryErr != nil {
		t.Fatalf("creating migration registry: %v", registryErr)
	}
	for _, statement := range schemaMigrations()[0] {
		if _, executeErr := database.ExecContext(t.Context(), statement); executeErr != nil {
			t.Fatalf("creating version one schema: %v", executeErr)
		}
	}
	if _, insertErr := database.ExecContext(
		t.Context(),
		"INSERT INTO schema_migrations (version, applied_at_unix) VALUES (1, ?)",
		time.Now().Unix(),
	); insertErr != nil {
		t.Fatalf("recording version one migration: %v", insertErr)
	}
	if closeErr := database.Close(); closeErr != nil {
		t.Fatalf("closing version one database: %v", closeErr)
	}

	store, err := Open(t.Context(), databasePath, testKey(1))
	if err != nil {
		t.Fatalf("Open() after version one error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	})
	if err := store.EnsureTargets(t.Context(), []string{"rider-a"}); err != nil {
		t.Fatalf("EnsureTargets() error = %v", err)
	}
	if err := store.BeginAuthorization(
		t.Context(),
		"rider-a",
		"rider@example.ts.net",
		bytes.Repeat([]byte{3}, 32),
		time.Now().Add(time.Minute),
	); err != nil {
		t.Fatalf("BeginAuthorization() after migration error = %v", err)
	}
	if err := store.UpsertTargetStage(t.Context(), "rider-a", 1, 1, "revision", "content-hash", 42); err != nil {
		t.Fatalf("UpsertTargetStage() after migration error = %v", err)
	}
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
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}

	stages, err := store.TrustedInventory(t.Context())
	if err != nil {
		t.Fatalf("TrustedInventory() error = %v", err)
	}
	if got, want := len(stages), 1; got != want {
		t.Fatalf("stages = %d, want %d", got, want)
	}
	restored := stages[0]
	if got, want := restored.Key(), stage.Key(); got != want {
		t.Errorf("key = %v, want %v", got, want)
	}
	if got, want := restored.ContentHash(), "content-hash"; got != want {
		t.Errorf("content hash = %q, want %q", got, want)
	}
	if got, want := restored.Revision(), "revision"; got != want {
		t.Errorf("revision = %q, want %q", got, want)
	}
	if got, want := restored.Title(), "Alpine loop — Descent"; got != want {
		t.Errorf("title = %q, want %q", got, want)
	}
	points := restored.Geometry()
	if got, want := len(points), 2; got != want {
		t.Fatalf("points = %d, want %d", got, want)
	}
	if points[0].Elevation == nil || *points[0].Elevation != elevation {
		t.Errorf("first elevation = %v, want %v", points[0].Elevation, elevation)
	}
	if points[1].Elevation != nil {
		t.Errorf("second elevation = %v, want none", *points[1].Elevation)
	}
}

// A partial library reads as a library whose missing stages should be deleted,
// so a stage without geometry for its current hash fails the whole read.
func TestStoreRefusesATrustedInventoryMissingGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.2},
	})
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}
	if _, err := store.database.ExecContext(t.Context(), "DELETE FROM stage_geometry"); err != nil {
		t.Fatalf("clearing geometry cache error = %v", err)
	}

	if _, err := store.TrustedInventory(t.Context()); err == nil {
		t.Error("TrustedInventory() error = nil, want a refusal to describe a partial library")
	}
}

// Both halves are on until an operator says otherwise, which is what every
// deployment did before the switches existed.
func TestStoreSchedulesBothPhasesUntilChanged(t *testing.T) {
	store := openTestStore(t, testKey(1))

	source, targets, err := store.SyncSchedule(t.Context())
	if err != nil {
		t.Fatalf("SyncSchedule() error = %v", err)
	}
	if !source || !targets {
		t.Errorf("SyncSchedule() = %v, %v, want both enabled", source, targets)
	}

	if setErr := store.SetSyncSchedule(t.Context(), false, true); setErr != nil {
		t.Fatalf("SetSyncSchedule() error = %v", setErr)
	}
	source, targets, err = store.SyncSchedule(t.Context())
	if err != nil {
		t.Fatalf("SyncSchedule() after change error = %v", err)
	}
	if source || !targets {
		t.Errorf("SyncSchedule() = %v, %v, want the source half off and the target half on", source, targets)
	}
}

// Each phase's own last run is what an operator reads; the newest run of the
// other phase answers a different question.
func TestStoreReportsTheLastRunOfEachPhase(t *testing.T) {
	store := openTestStore(t, testKey(1))
	startedAt := time.Date(2026, time.August, 17, 8, 0, 0, 0, time.UTC)
	record := func(phase, outcome string, minute int, sourceStages, created int) {
		t.Helper()
		began := startedAt.Add(time.Duration(minute) * time.Minute)
		if err := store.RecordSyncRun(
			t.Context(), phase, began, began.Add(time.Second), outcome, "", sourceStages, created, 0, 0,
		); err != nil {
			t.Fatalf("RecordSyncRun() error = %v", err)
		}
	}
	record("source", "failed", 0, 0, 0)
	record("source", "succeeded", 1, 12, 0)
	record("targets", "succeeded", 2, 12, 3)

	outcomes := make(map[string]string)
	counts := make(map[string]int)
	if err := store.ForEachPhaseRun(t.Context(), func(
		phase string, _ time.Time, outcome, _ string, sourceStages, created, _, _ int,
	) error {
		outcomes[phase] = outcome
		counts[phase] = sourceStages + created

		return nil
	}); err != nil {
		t.Fatalf("ForEachPhaseRun() error = %v", err)
	}
	if got, want := outcomes["source"], "succeeded"; got != want {
		t.Errorf("source outcome = %q, want %q", got, want)
	}
	if got, want := outcomes["targets"], "succeeded"; got != want {
		t.Errorf("targets outcome = %q, want %q", got, want)
	}
	if got, want := counts["targets"], 15; got != want {
		t.Errorf("target run counts = %d, want %d", got, want)
	}
}

func TestStoreCachesStageGeometryForTheMapView(t *testing.T) {
	store := openTestStore(t, testKey(1))
	elevation := 128.5
	stage := storeTestStageWithGeometry(t, 7, 2, "revision", "content-hash", "Alpine loop", "Descent", []route.Point{
		{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
		{Longitude: 8.5, Latitude: 49.2},
	})
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}

	summary, coordinates, found, err := store.StageGeometry(t.Context(), 7, 2)
	if err != nil {
		t.Fatalf("StageGeometry() error = %v", err)
	}
	if !found {
		t.Fatal("StageGeometry() found = false, want true")
	}
	if got, want := summary.Title(), "Alpine loop — Descent"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
	if got, want := summary.PointCount, 2; got != want {
		t.Errorf("PointCount = %d, want %d", got, want)
	}
	if summary.DistanceMetres <= 0 {
		t.Errorf("DistanceMetres = %v, want a positive length", summary.DistanceMetres)
	}
	wantBounds := route.Bounds{MinLongitude: 8.4, MinLatitude: 49.0, MaxLongitude: 8.5, MaxLatitude: 49.2}
	if summary.Bounds != wantBounds {
		t.Errorf("Bounds = %+v, want %+v", summary.Bounds, wantBounds)
	}
	if got, want := string(coordinates), `[[8.4,49,128.5],[8.5,49.2]]`; got != want {
		t.Errorf("coordinates = %s, want %s", got, want)
	}
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
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}

	summary, _, found, err := store.StageGeometry(t.Context(), 5, 1)
	if err != nil || !found {
		t.Fatalf("StageGeometry() found = %v, error = %v", found, err)
	}
	if got, want := summary.AscentMetres, 40.0; math.Abs(got-want) > 0.001 {
		t.Errorf("AscentMetres = %v, want %v", got, want)
	}
	if summary.MaxGradientPercent <= 0 {
		t.Errorf("MaxGradientPercent = %v, want a positive gradient", summary.MaxGradientPercent)
	}

	var listed route.Summary
	if err := store.ForEachStageSummary(t.Context(), func(summary route.Summary) error {
		listed = summary

		return nil
	}); err != nil {
		t.Fatalf("ForEachStageSummary() error = %v", err)
	}
	if got, want := listed.AscentMetres, summary.AscentMetres; got != want {
		t.Errorf("listed AscentMetres = %v, want %v", got, want)
	}
}

// A stage cached before the statistics existed must still be readable; the
// columns default to zero until a content change refills them.
func TestStoreReadsGeometryCachedBeforeElevationStatistics(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "hash")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}
	if _, err := store.database.ExecContext(t.Context(),
		`UPDATE stage_geometry SET ascent_metres = 0, max_gradient_percent = 0`); err != nil {
		t.Fatalf("clearing statistics error = %v", err)
	}

	summary, _, found, err := store.StageGeometry(t.Context(), 1, 1)
	if err != nil || !found {
		t.Fatalf("StageGeometry() found = %v, error = %v", found, err)
	}
	if summary.AscentMetres != 0 || summary.MaxGradientPercent != 0 {
		t.Errorf("expected zeroed statistics, got %+v", summary)
	}
}

func TestStoreDoesNotRewriteUnchangedStageGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "content-hash")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}
	// A sentinel that a rewrite would necessarily overwrite. This is the
	// write-amplification guarantee: an unchanged library must not rewrite the
	// geometry cache on every scheduled run.
	const sentinel = 1
	if _, err := store.database.ExecContext(t.Context(),
		`UPDATE stage_geometry SET updated_at_unix = ?`, sentinel); err != nil {
		t.Fatalf("seeding sentinel error = %v", err)
	}

	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("second StoreTrustedInventory() error = %v", err)
	}
	if got := stageGeometryUpdatedAt(t, store, 1, 1); got != sentinel {
		t.Errorf("updated_at_unix = %d after an unchanged sync, want the sentinel %d", got, sentinel)
	}

	changed := storeTestStage(t, 1, 1, "revision", "different-content-hash")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{changed}); err != nil {
		t.Fatalf("changed StoreTrustedInventory() error = %v", err)
	}
	if got := stageGeometryUpdatedAt(t, store, 1, 1); got == sentinel {
		t.Error("updated_at_unix was not refreshed after the content hash changed")
	}
}

func TestStorePrunesGeometryForStagesLeavingTheInventory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "hash-one")
	second := storeTestStage(t, 2, 1, "revision", "hash-two")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{first, second}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{first}); err != nil {
		t.Fatalf("second StoreTrustedInventory() error = %v", err)
	}

	if _, _, found, err := store.StageGeometry(t.Context(), 2, 1); err != nil || found {
		t.Errorf("StageGeometry() for a removed stage = found %v, error %v; want not found", found, err)
	}
	if _, _, found, err := store.StageGeometry(t.Context(), 1, 1); err != nil || !found {
		t.Errorf("StageGeometry() for a retained stage = found %v, error %v; want found", found, err)
	}
}

func TestStoreListsStageSummaries(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStageWithGeometry(t, 3, 1, "revision", "hash", "Sunday", "", []route.Point{
		{Longitude: 8.4, Latitude: 49.0},
		{Longitude: 8.5, Latitude: 49.1},
	})
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}

	var summaries []route.Summary
	if err := store.ForEachStageSummary(t.Context(), func(summary route.Summary) error {
		summaries = append(summaries, summary)

		return nil
	}); err != nil {
		t.Fatalf("ForEachStageSummary() error = %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("ForEachStageSummary() returned %d summaries, want 1", len(summaries))
	}
	if got, want := summaries[0].Title(), "Sunday"; got != want {
		t.Errorf("Title() = %q, want %q", got, want)
	}
	if got, want := summaries[0].SourceRevision, "revision"; got != want {
		t.Errorf("SourceRevision = %q, want %q", got, want)
	}
	if got, want := summaries[0].PointCount, 2; got != want {
		t.Errorf("PointCount = %d, want %d", got, want)
	}
}

func TestStoreCachesStageSurfaceAgainstTheGeometryItDescribes(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 7, 2, "revision", "content-hash")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}

	if _, found, err := store.StageSurfaceHash(t.Context(), 7, 2); err != nil || found {
		t.Errorf("StageSurfaceHash() before enrichment = found %v, error %v; want not found", found, err)
	}

	if err := store.StoreStageSurface(
		t.Context(), 7, 2, "content-hash", []byte(testSurfaceRanges), 1234.5,
	); err != nil {
		t.Fatalf("StoreStageSurface() error = %v", err)
	}

	ranges, matchedMetres, found, err := store.StageSurface(t.Context(), 7, 2, "content-hash")
	if err != nil {
		t.Fatalf("StageSurface() error = %v", err)
	}
	if !found {
		t.Fatal("StageSurface() found = false, want true")
	}
	// Byte-identical, because the endpoint serves the stored ranges as they are.
	if got, want := string(ranges), testSurfaceRanges; got != want {
		t.Errorf("ranges = %s, want %s", got, want)
	}
	if got, want := matchedMetres, 1234.5; got != want {
		t.Errorf("matchedMetres = %v, want %v", got, want)
	}
	if hash, found, err := store.StageSurfaceHash(t.Context(), 7, 2); err != nil || !found ||
		hash != "content-hash" {
		t.Errorf("StageSurfaceHash() = %q, found %v, error %v; want %q", hash, found, err, "content-hash")
	}
}

func TestStoreHidesASurfaceMeasuredAgainstOtherGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "current-hash")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}
	if err := store.StoreStageSurface(
		t.Context(), 1, 1, "earlier-hash", []byte(testSurfaceRanges), 10,
	); err != nil {
		t.Fatalf("StoreStageSurface() error = %v", err)
	}

	// The ranges index the coordinates of the geometry they were measured
	// against, so beside a re-planned stage they are absent, never approximate.
	if _, _, found, err := store.StageSurface(t.Context(), 1, 1, "current-hash"); err != nil || found {
		t.Errorf("StageSurface() for other geometry = found %v, error %v; want not found", found, err)
	}
	// The hash is still readable, which is how the enrichment pass knows the
	// stage needs asking about again.
	if hash, found, err := store.StageSurfaceHash(t.Context(), 1, 1); err != nil || !found ||
		hash != "earlier-hash" {
		t.Errorf("StageSurfaceHash() = %q, found %v, error %v; want %q", hash, found, err, "earlier-hash")
	}
}

func TestStoreReplacesAStageSurfaceRatherThanAccumulatingOne(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "second-hash")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}
	if err := store.StoreStageSurface(t.Context(), 1, 1, "first-hash", []byte(`[]`), 1); err != nil {
		t.Fatalf("first StoreStageSurface() error = %v", err)
	}
	if err := store.StoreStageSurface(
		t.Context(), 1, 1, "second-hash", []byte(testSurfaceRanges), 2,
	); err != nil {
		t.Fatalf("second StoreStageSurface() error = %v", err)
	}

	ranges, matchedMetres, found, err := store.StageSurface(t.Context(), 1, 1, "second-hash")
	if err != nil || !found {
		t.Fatalf("StageSurface() found = %v, error = %v", found, err)
	}
	if got, want := string(ranges), testSurfaceRanges; got != want {
		t.Errorf("ranges = %s, want %s", got, want)
	}
	if got, want := matchedMetres, 2.0; got != want {
		t.Errorf("matchedMetres = %v, want %v", got, want)
	}
	if got := countStageSurfaceRows(t, store); got != 1 {
		t.Errorf("stage_surface holds %d rows for one stage, want 1", got)
	}
}

func TestStorePrunesSurfaceForStagesLeavingTheInventory(t *testing.T) {
	store := openTestStore(t, testKey(1))
	first := storeTestStage(t, 1, 1, "revision", "hash-one")
	second := storeTestStage(t, 2, 1, "revision", "hash-two")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{first, second}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}
	if err := store.StoreStageSurface(
		t.Context(), 1, 1, "hash-one", []byte(testSurfaceRanges), 10,
	); err != nil {
		t.Fatalf("StoreStageSurface() error = %v", err)
	}
	if err := store.StoreStageSurface(
		t.Context(), 2, 1, "hash-two", []byte(testSurfaceRanges), 10,
	); err != nil {
		t.Fatalf("StoreStageSurface() error = %v", err)
	}

	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{first}); err != nil {
		t.Fatalf("second StoreTrustedInventory() error = %v", err)
	}

	if _, found, err := store.StageSurfaceHash(t.Context(), 2, 1); err != nil || found {
		t.Errorf("StageSurfaceHash() for a removed stage = found %v, error %v; want not found", found, err)
	}
	if _, _, found, err := store.StageSurface(t.Context(), 1, 1, "hash-one"); err != nil || !found {
		t.Errorf("StageSurface() for a retained stage = found %v, error %v; want found", found, err)
	}
}

func TestStorePrunesSurfaceMeasuredAgainstReplacedGeometry(t *testing.T) {
	store := openTestStore(t, testKey(1))
	stage := storeTestStage(t, 1, 1, "revision", "hash-one")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{stage}); err != nil {
		t.Fatalf("StoreTrustedInventory() error = %v", err)
	}
	if err := store.StoreStageSurface(
		t.Context(), 1, 1, "hash-one", []byte(testSurfaceRanges), 10,
	); err != nil {
		t.Fatalf("StoreStageSurface() error = %v", err)
	}

	replanned := storeTestStage(t, 1, 1, "revision", "hash-two")
	if err := store.StoreTrustedInventory(t.Context(), []route.Stage{replanned}); err != nil {
		t.Fatalf("second StoreTrustedInventory() error = %v", err)
	}

	// The row goes rather than lingering as something to be matched around: the
	// coordinate array its ranges address has been replaced.
	if got := countStageSurfaceRows(t, store); got != 0 {
		t.Errorf("stage_surface holds %d rows after re-planning, want 0", got)
	}
}

// testSurfaceRanges is one stored classification, in the shape the annotator
// writes and the geometry endpoint serves.
const testSurfaceRanges = `[{"kind":"asphalt","start_index":0,"end_index":1}]`

func countStageSurfaceRows(t *testing.T, store *Store) int {
	t.Helper()

	var rows int
	if err := store.database.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM stage_surface`,
	).Scan(&rows); err != nil {
		t.Fatalf("counting stage_surface rows error = %v", err)
	}

	return rows
}

func stageGeometryUpdatedAt(t *testing.T, store *Store, routeID int64, stageOrder int) int64 {
	t.Helper()

	var updatedAt int64
	if err := store.database.QueryRowContext(t.Context(),
		`SELECT updated_at_unix FROM stage_geometry WHERE route_id = ? AND stage_order = ?`,
		routeID, stageOrder,
	).Scan(&updatedAt); err != nil {
		t.Fatalf("reading updated_at_unix error = %v", err)
	}

	return updatedAt
}

func openTestStore(t *testing.T, key [32]byte) *Store {
	t.Helper()

	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "state.db"), key)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			t.Errorf("Close() error = %v", err)
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
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

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
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	return stage
}

func equalStrings(left, right []string) bool {
	return slices.Equal(left, right)
}
