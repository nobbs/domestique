package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreCreatesCurrentSchemaBaseline(t *testing.T) {
	store := openTestStore(t, testKey(1))
	var legacy, current int
	var dirty bool
	require.NoError(t, store.database.QueryRowContext(t.Context(), `SELECT MAX(version) FROM schema_migrations`).Scan(&legacy))
	require.NoError(t, store.database.QueryRowContext(t.Context(), `SELECT version, dirty FROM domestique_migrations`).Scan(&current, &dirty))
	assert.Equal(t, currentSchemaVersion, legacy)
	assert.Equal(t, currentSchemaVersion, current)
	assert.False(t, dirty)
}

func TestStoreMarksValidatedLegacyState(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	first, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	require.NoError(t, first.Close())
	database, err := sql.Open(driverName, databaseDSN(databasePath))
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `DROP TABLE domestique_migrations`)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	promoted, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, promoted.Close()) })
	var version int
	require.NoError(t, promoted.database.QueryRowContext(t.Context(), `SELECT version FROM domestique_migrations`).Scan(&version))
	assert.Equal(t, currentSchemaVersion, version)
}

func TestStoreRefusesInvalidMigrationState(t *testing.T) {
	for name, mutate := range map[string]func(context.Context, *sql.DB) error{
		"dirty": func(ctx context.Context, database *sql.DB) error {
			if _, err := database.ExecContext(ctx, `UPDATE domestique_migrations SET dirty = 1`); err != nil {
				return fmt.Errorf("marking state schema dirty: %w", err)
			}
			return nil
		},
		"older legacy": func(ctx context.Context, database *sql.DB) error {
			if _, err := database.ExecContext(ctx, `DROP TABLE domestique_migrations`); err != nil {
				return fmt.Errorf("removing state migration tracker: %w", err)
			}
			if _, err := database.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 27`); err != nil {
				return fmt.Errorf("rewinding legacy state migration: %w", err)
			}
			return nil
		},
		"schema drift": func(ctx context.Context, database *sql.DB) error {
			if _, err := database.ExecContext(ctx, `DROP TABLE domestique_migrations`); err != nil {
				return fmt.Errorf("removing state migration tracker: %w", err)
			}
			if _, err := database.ExecContext(ctx, `ALTER TABLE targets ADD COLUMN unexpected TEXT`); err != nil {
				return fmt.Errorf("changing state schema: %w", err)
			}
			return nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			databasePath := filepath.Join(t.TempDir(), "state.db")
			store, err := Open(t.Context(), databasePath, testKey(1))
			require.NoError(t, err)
			require.NoError(t, store.Close())
			database, err := sql.Open(driverName, databaseDSN(databasePath))
			require.NoError(t, err)
			require.NoError(t, mutate(t.Context(), database))
			require.NoError(t, database.Close())
			_, err = Open(t.Context(), databasePath, testKey(1))
			require.Error(t, err)
		})
	}
}

func TestCheckConstraintsCapturesNestedParentheses(t *testing.T) {
	checks := checkConstraints(`CREATE TABLE test (surface TEXT CHECK (surface IN ('asphalt', 'gravel')))`)
	assert.Equal(t, []string{"surface in ('asphalt', 'gravel')"}, checks)
}

func TestStorePreservesPreviousReleaseRollbackWindow(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	require.NoError(t, store.Close())
	database, err := sql.Open(driverName, databaseDSN(databasePath))
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `UPDATE domestique_migrations SET version = 28; INSERT INTO schema_migrations (version, applied_at_unix) VALUES (28, 0)`)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	rolledBack, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	require.NoError(t, rolledBack.Close())
	database, err = sql.Open(driverName, databaseDSN(databasePath))
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `UPDATE domestique_migrations SET version = 29; INSERT INTO schema_migrations (version, applied_at_unix) VALUES (29, 0)`)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	_, err = Open(t.Context(), databasePath, testKey(1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), schemaAheadMessage)
}

func TestDatabaseDSNConfiguresEveryConnection(t *testing.T) {
	database, err := sql.Open(driverName, databaseDSN(filepath.Join(t.TempDir(), "state.db")))
	require.NoError(t, err)
	database.SetMaxOpenConns(2)
	t.Cleanup(func() { assert.NoError(t, database.Close()) })
	first, err := database.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, first.Close()) })
	second, err := database.Conn(t.Context())
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	for _, connection := range []*sql.Conn{first, second} {
		var foreignKeys, busyTimeout int
		require.NoError(t, connection.QueryRowContext(t.Context(), `PRAGMA foreign_keys`).Scan(&foreignKeys))
		require.NoError(t, connection.QueryRowContext(t.Context(), `PRAGMA busy_timeout`).Scan(&busyTimeout))
		assert.Equal(t, 1, foreignKeys)
		assert.Equal(t, 5000, busyTimeout)
	}
}
