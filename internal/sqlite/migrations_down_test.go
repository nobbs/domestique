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
