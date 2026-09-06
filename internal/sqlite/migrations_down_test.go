package sqlite

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// No down migration in this repo is otherwise exercised by anything: the
// compatibility harness only ever migrates forward. 029's down is the one
// that rebuilds a table rather than dropping it outright, so it is the one
// most worth actually running — a mistake in the rebuild loses every session
// row, silently, on whatever operator ever needs it.
func TestMigration029DownRebuildsWebSessions(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "web-sessions-rollback.db")
	migration, closeFn, err := openMigrator(dbPath, migrationFiles, "migrations")
	require.NoError(t, err)
	defer closeFn()

	require.NoError(t, migration.Migrate(29))

	database, err := openDatabase(dbPath)
	require.NoError(t, err)
	defer closeDatabase(database)

	_, err = database.ExecContext(t.Context(), `INSERT INTO web_sessions (token_digest, subject, display, admin, created_at_unix, renewed_at_unix, expires_at_unix) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		loginDigest(99), "github|123", "Rider", 1, 1700000000, 1700000000, 1700086400)
	require.NoError(t, err)

	require.NoError(t, migration.Migrate(28))

	var subject, display string
	var created, renewed, expires int64
	err = database.QueryRowContext(t.Context(), `SELECT subject, display, created_at_unix, renewed_at_unix, expires_at_unix FROM web_sessions`).
		Scan(&subject, &display, &created, &renewed, &expires)
	require.NoError(t, err, "row must survive the rollback")
	require.Equal(t, "github|123", subject)
	require.Equal(t, "Rider", display)
	require.EqualValues(t, 1700086400, expires)

	var adminColumnCount int
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pragma_table_info('web_sessions') WHERE name='admin'`).Scan(&adminColumnCount))
	require.Zero(t, adminColumnCount, "admin column must be gone after rollback")

	var indexCount int
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='web_sessions_expiry_index'`).Scan(&indexCount))
	require.Equal(t, 1, indexCount, "the expiry index must survive the rebuild")

	require.NoError(t, migration.Migrate(29), "must be able to re-migrate up after rolling back")
}

// 030's down uses ALTER TABLE ... DROP COLUMN rather than 029's rebuild
// pattern — targets is referenced by foreign key from oauth_transactions and
// target_stages, and a rebuild's DROP TABLE fails against any row those
// still reference. Exercised with exactly such a referencing row present, so
// a regression here would be the FK violation a rebuild would hit for real.
func TestMigration030DownDropsOwnerSubjectWithReferencingRowsPresent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "target-ownership-rollback.db")
	migration, closeFn, err := openMigrator(dbPath, migrationFiles, "migrations")
	require.NoError(t, err)
	defer closeFn()

	require.NoError(t, migration.Migrate(30))

	database, err := openDatabase(dbPath)
	require.NoError(t, err)
	defer closeDatabase(database)

	_, err = database.ExecContext(t.Context(),
		`INSERT INTO targets (slot, owner_subject, authorization_state, updated_at_unix) VALUES (?, ?, ?, ?)`,
		"github|123", "github|123", "authorized", 1700000000)
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(),
		`INSERT INTO oauth_transactions (id, target_slot, state_digest, code_verifier, expires_at_unix, caller_login) VALUES (?, ?, ?, ?, ?, ?)`,
		"tx-1", "github|123", loginDigest(1), "verifier", 1700086400, "github|123")
	require.NoError(t, err)

	require.NoError(t, migration.Migrate(29), "must roll back cleanly despite the referencing oauth_transactions row")

	var slot, state string
	err = database.QueryRowContext(t.Context(), `SELECT slot, authorization_state FROM targets`).Scan(&slot, &state)
	require.NoError(t, err, "the target row must survive the rollback")
	require.Equal(t, "github|123", slot)
	require.Equal(t, "authorized", state)

	var referencingRows int
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM oauth_transactions WHERE target_slot = ?`, "github|123").Scan(&referencingRows))
	require.Equal(t, 1, referencingRows, "the referencing row must survive the rollback")

	var ownerColumnCount int
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pragma_table_info('targets') WHERE name='owner_subject'`).Scan(&ownerColumnCount))
	require.Zero(t, ownerColumnCount, "owner_subject column must be gone after rollback")

	require.NoError(t, migration.Migrate(30), "must be able to re-migrate up after rolling back")
}

// 031's down drops two tables that reference targets; the referenced target
// row and everything else must be untouched, and the schema must come back.
func TestMigration031DownDropsActivityTablesOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "activities-rollback.db")
	migration, closeFn, err := openMigrator(dbPath, migrationFiles, "migrations")
	require.NoError(t, err)
	defer closeFn()

	require.NoError(t, migration.Migrate(31))

	database, err := openDatabase(dbPath)
	require.NoError(t, err)
	defer closeDatabase(database)

	_, err = database.ExecContext(t.Context(),
		`INSERT INTO targets (slot, authorization_state, updated_at_unix) VALUES ('rider-a', 'authorized', 1700000000)`)
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `
		INSERT INTO activities (target_slot, workout_id, workout_type_id, workout_type_location_id, started_at_unix,
			distance_metres, moving_seconds, elapsed_seconds, ascent_metres, raw_summary_json, updated_at_unix)
		VALUES ('rider-a', 1, 15, 1, 1700000000, 1000, 60, 65, 10, '{}', 1700000000)`)
	require.NoError(t, err)

	require.NoError(t, migration.Migrate(30))

	var targets, tables int
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM targets`).Scan(&targets))
	require.Equal(t, 1, targets, "the target row must survive the rollback")
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name IN ('activities', 'activity_records')`).Scan(&tables))
	require.Zero(t, tables, "both activity tables must be gone")

	require.NoError(t, migration.Migrate(31), "must be able to re-migrate up after rolling back")
}

// 032's down drops the skip table alone; the activities beside it stay.
func TestMigration032DownDropsActivitySkipsOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "activity-skips-rollback.db")
	migration, closeFn, err := openMigrator(dbPath, migrationFiles, "migrations")
	require.NoError(t, err)
	defer closeFn()

	require.NoError(t, migration.Migrate(32))

	database, err := openDatabase(dbPath)
	require.NoError(t, err)
	defer closeDatabase(database)

	_, err = database.ExecContext(t.Context(),
		`INSERT INTO targets (slot, authorization_state, updated_at_unix) VALUES ('rider-a', 'authorized', 1700000000)`)
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `
		INSERT INTO activities (target_slot, workout_id, workout_type_id, workout_type_location_id, started_at_unix,
			distance_metres, moving_seconds, elapsed_seconds, ascent_metres, raw_summary_json, updated_at_unix)
		VALUES ('rider-a', 1, 15, 1, 1700000000, 1000, 60, 65, 10, '{}', 1700000000)`)
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(),
		`INSERT INTO activity_skips (target_slot, workout_id, attempts, last_attempt_unix, observed) VALUES ('rider-a', 2, 1, 1700000000, 'HTTP 404')`)
	require.NoError(t, err)

	require.NoError(t, migration.Migrate(31))

	var activities, tables int
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM activities`).Scan(&activities))
	require.Equal(t, 1, activities, "the activity row must survive the rollback")
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name = 'activity_skips'`).Scan(&tables))
	require.Zero(t, tables, "the skip table must be gone")

	require.NoError(t, migration.Migrate(32), "must be able to re-migrate up after rolling back")
}

// 033's down drops two columns from a table activity_records references, which
// is why it drops columns rather than rebuilding; exercised with such a record
// present so a regression is the foreign key failure a rebuild would hit.
func TestMigration033DownDropsRecordStateColumnsOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "records-state-rollback.db")
	migration, closeFn, err := openMigrator(dbPath, migrationFiles, "migrations")
	require.NoError(t, err)
	defer closeFn()

	require.NoError(t, migration.Migrate(33))

	database, err := openDatabase(dbPath)
	require.NoError(t, err)
	defer closeDatabase(database)

	_, err = database.ExecContext(t.Context(),
		`INSERT INTO targets (slot, authorization_state, updated_at_unix) VALUES ('rider-a', 'authorized', 1700000000)`)
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `
		INSERT INTO activities (target_slot, workout_id, workout_type_id, workout_type_location_id, started_at_unix,
			distance_metres, moving_seconds, elapsed_seconds, ascent_metres, raw_summary_json, updated_at_unix, records_state)
		VALUES ('rider-a', 1, 15, 1, 1700000000, 1000, 60, 65, 10, '{}', 1700000000, 'stored')`)
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(),
		`INSERT INTO activity_records (target_slot, workout_id, record_index, recorded_at_unix, power_watts)
		 VALUES ('rider-a', 1, 0, 1700000001, 240)`)
	require.NoError(t, err)

	require.NoError(t, migration.Migrate(32), "must roll back cleanly despite the referencing record row")

	var records int
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM activity_records`).Scan(&records))
	require.Equal(t, 1, records, "the sample row must survive the rollback")

	var columns int
	require.NoError(t, database.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM pragma_table_info('activities') WHERE name IN ('records_state', 'fit_checksum_failed')`).Scan(&columns))
	require.Zero(t, columns, "both columns must be gone after rollback")

	require.NoError(t, migration.Migrate(33), "must be able to re-migrate up after rolling back")
}

// 034's down drops the singleton coefficient row's table and nothing else.
func TestMigration034DownDropsTheCoefficientTableOnly(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ridemodel-rollback.db")
	migration, closeFn, err := openMigrator(dbPath, migrationFiles, "migrations")
	require.NoError(t, err)
	defer closeFn()

	require.NoError(t, migration.Migrate(34))

	database, err := openDatabase(dbPath)
	require.NoError(t, err)
	defer closeDatabase(database)

	_, err = database.ExecContext(t.Context(), `
		INSERT INTO ridemodel_coefficients (id, seconds_per_km, seconds_per_ascent_m, updated_at_unix)
		VALUES (1, 145.3578, 3.2190, 1700000000)`)
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `
		INSERT INTO ridemodel_coefficients (id, seconds_per_km, seconds_per_ascent_m, updated_at_unix)
		VALUES (2, 1, 1, 1700000000)`)
	require.Error(t, err, "the table holds one row: a second id must be refused")

	require.NoError(t, migration.Migrate(33))

	var tables int
	require.NoError(t, database.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='ridemodel_coefficients'`).Scan(&tables))
	require.Zero(t, tables, "the coefficient table must be gone")
	var settings int
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM runtime_settings`).Scan(&settings))
	require.Equal(t, 1, settings, "the settings row must survive the rollback")

	require.NoError(t, migration.Migrate(34), "must be able to re-migrate up after rolling back")
}

// 036 is the one data migration in this history: it deletes rows rather than
// changing a table, so what it removes and what it spares are both worth
// proving. Its down cannot restore them, which is exactly why the up must not
// reach further than the workouts a poll would now refuse.
func TestMigration036RemovesOnlyNonCyclingActivities(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cycling-only.db")
	migration, closeFn, err := openMigrator(dbPath, migrationFiles, "migrations")
	require.NoError(t, err)
	defer closeFn()

	require.NoError(t, migration.Migrate(35))

	database, err := openDatabase(dbPath)
	require.NoError(t, err)
	defer closeDatabase(database)

	_, err = database.ExecContext(t.Context(),
		`INSERT INTO targets (slot, authorization_state, updated_at_unix) VALUES ('rider-a', 'authorized', 1700000000)`)
	require.NoError(t, err)
	// 15 road, 61 indoor trainer and 64 e-bike are cycling; 1 and 3 are not.
	for _, workoutType := range []int{15, 61, 64, 1, 3} {
		_, err = database.ExecContext(t.Context(), `
			INSERT INTO activities (target_slot, workout_id, workout_type_id, workout_type_location_id, started_at_unix,
				distance_metres, moving_seconds, elapsed_seconds, ascent_metres, raw_summary_json, updated_at_unix)
			VALUES ('rider-a', ?, ?, 1, 1700000000, 1000, 60, 65, 10, '{}', 1700000000)`, workoutType, workoutType)
		require.NoError(t, err)
		_, err = database.ExecContext(t.Context(), `
			INSERT INTO activity_records (target_slot, workout_id, record_index, recorded_at_unix)
			VALUES ('rider-a', ?, 0, 1700000000)`, workoutType)
		require.NoError(t, err)
	}

	require.NoError(t, migration.Migrate(36))

	var kept []int
	rows, err := database.QueryContext(t.Context(), `SELECT workout_type_id FROM activities ORDER BY workout_type_id`)
	require.NoError(t, err)
	defer closeRows(rows)
	for rows.Next() {
		var workoutType int
		require.NoError(t, rows.Scan(&workoutType))
		kept = append(kept, workoutType)
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []int{15, 61, 64}, kept, "every biking type stays, including indoor and e-bike")

	var records int
	require.NoError(t, database.QueryRowContext(t.Context(),
		`SELECT COUNT(*) FROM activity_records`).Scan(&records))
	require.Equal(t, 3, records, "a deleted activity takes its records with it, and no others")

	var targets int
	require.NoError(t, database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM targets`).Scan(&targets))
	require.Equal(t, 1, targets, "the target row is untouched")

	require.NoError(t, migration.Migrate(35), "the down migration must apply")
	require.NoError(t, migration.Migrate(36), "must be able to re-migrate up after rolling back")
}
