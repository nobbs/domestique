package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

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
	legacySchema, err := os.ReadFile(filepath.Join("testdata", "legacy-v27.sql"))
	require.NoError(t, err)
	database, err := sql.Open(driverName, databaseDSN(databasePath))
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), string(legacySchema))
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `INSERT INTO targets (slot, authorization_state, updated_at_unix) VALUES ('rider-a', 'not_authorized', 0)`)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	promoted, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, promoted.Close()) })
	var version int
	require.NoError(t, promoted.database.QueryRowContext(t.Context(), `SELECT version FROM domestique_migrations`).Scan(&version))
	assert.Equal(t, currentSchemaVersion, version)
	var targets int
	require.NoError(t, promoted.database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM targets WHERE slot = 'rider-a'`).Scan(&targets))
	assert.Equal(t, 1, targets)
}

func compatibleFutureMigration(version int) string {
	return fmt.Sprintf(`
		ALTER TABLE targets ADD COLUMN migration_note TEXT;
		CREATE INDEX targets_updated_at_index ON targets(updated_at_unix);
		INSERT INTO schema_migrations (version, applied_at_unix) VALUES (%d, 0);
	`, version)
}

func TestStoreAppliesPendingTrackedMigrations(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })

	futureVersion := currentSchemaVersion + 1
	files := futureMigrationFiles(t, futureVersion, compatibleFutureMigration(futureVersion))
	require.NoError(t, store.migrateTo(t.Context(), databasePath, futureVersion, files, "migrations"))
	require.NoError(t, store.migrateTo(t.Context(), databasePath, futureVersion, files, "migrations"))

	var legacy, tracked, column int
	require.NoError(t, store.database.QueryRowContext(t.Context(), `SELECT MAX(version) FROM schema_migrations`).Scan(&legacy))
	require.NoError(t, store.database.QueryRowContext(t.Context(), `SELECT version FROM domestique_migrations`).Scan(&tracked))
	require.NoError(t, store.database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM pragma_table_info('targets') WHERE name = 'migration_note'`).Scan(&column))
	assert.Equal(t, futureVersion, legacy)
	assert.Equal(t, futureVersion, tracked)
	assert.Equal(t, 1, column)
}

func TestStoreRejectsMigrationWithoutLegacyWatermark(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })

	futureVersion := currentSchemaVersion + 1
	files := futureMigrationFiles(t, futureVersion, `ALTER TABLE targets ADD COLUMN migration_note TEXT;`)
	err = store.migrateTo(t.Context(), databasePath, futureVersion, files, "migrations")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration history is not current")
}

func TestStoreReportsPendingMigrationFailure(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })

	futureVersion := currentSchemaVersion + 1
	files := futureMigrationFiles(t, futureVersion, `THIS IS NOT SQL;`)
	err = store.migrateTo(t.Context(), databasePath, futureVersion, files, "migrations")
	require.ErrorContains(t, err, "applying state migrations")
}

func futureMigrationFiles(t *testing.T, version int, future string) fstest.MapFS {
	t.Helper()
	files := fstest.MapFS{}
	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	require.NoError(t, err)
	for _, name := range names {
		contents, err := migrationFiles.ReadFile(name)
		require.NoError(t, err)
		files[name] = &fstest.MapFile{Data: contents}
	}
	files[fmt.Sprintf("migrations/%06d_test.up.sql", version)] = &fstest.MapFile{Data: []byte(future)}
	files[fmt.Sprintf("migrations/%06d_test.down.sql", version)] = &fstest.MapFile{Data: []byte("-- Forward only.")}
	return files
}

func TestStoreRefusesInvalidMigrationState(t *testing.T) {
	for name, mutate := range map[string]func(context.Context, *sql.DB) error{
		"dirty": func(ctx context.Context, database *sql.DB) error {
			if _, err := database.ExecContext(ctx, `UPDATE domestique_migrations SET dirty = 1`); err != nil {
				return fmt.Errorf("marking state schema dirty: %w", err)
			}
			return nil
		},
		"watermark mismatch": func(ctx context.Context, database *sql.DB) error {
			if _, err := database.ExecContext(ctx, fmt.Sprintf(`INSERT INTO schema_migrations (version, applied_at_unix) VALUES (%d, 0)`, currentSchemaVersion+1)); err != nil {
				return fmt.Errorf("advancing legacy watermark: %w", err)
			}
			return nil
		},
		"before baseline": func(ctx context.Context, database *sql.DB) error {
			if _, err := database.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version = 27`); err != nil {
				return fmt.Errorf("rewinding legacy watermark: %w", err)
			}
			if _, err := database.ExecContext(ctx, `UPDATE domestique_migrations SET version = 26`); err != nil {
				return fmt.Errorf("rewinding tracked watermark: %w", err)
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

func TestNewMigrationsStayReadableByThePreviousRelease(t *testing.T) {
	for version := baselineSchemaVersion + 1; version <= currentSchemaVersion; version++ {
		t.Run(fmt.Sprintf("migration %d", version), func(t *testing.T) {
			assertMigrationCompatible(t, migrationFiles, "migrations", version-1, version)
		})
	}
}

func TestAdjacentMigrationCompatibilityCheck(t *testing.T) {
	futureVersion := currentSchemaVersion + 1
	assertMigrationCompatible(
		t,
		futureMigrationFiles(t, futureVersion, compatibleFutureMigration(futureVersion)),
		"migrations",
		currentSchemaVersion,
		futureVersion,
	)
}

type migrationColumn struct {
	declaredType string
	nullable     bool
	hasDefault   bool
	primaryKey   bool
}

type migrationIndex struct {
	table   string
	origin  string
	shape   string
	unique  bool
	partial bool
}

type migrationSchema struct {
	tables  map[string]map[string]migrationColumn
	checks  map[string][]string
	indexes map[string]migrationIndex
}

func assertMigrationCompatible(t *testing.T, files fs.FS, directory string, beforeVersion, afterVersion int) {
	t.Helper()
	before := readMigrationSchema(t, files, directory, beforeVersion)
	after := readMigrationSchema(t, files, directory, afterVersion)

	for table, columns := range before.tables {
		updated, found := after.tables[table]
		require.True(t, found, "migration %d drops or renames table %q", afterVersion, table)
		for name, column := range columns {
			changed, stillThere := updated[name]
			require.True(t, stillThere, "migration %d drops or renames %s.%s", afterVersion, table, name)
			assert.Equal(t, column, changed, "migration %d redefines %s.%s", afterVersion, table, name)
		}
		for name, column := range updated {
			if _, existed := columns[name]; existed {
				continue
			}
			assert.True(t, column.nullable || column.hasDefault,
				"migration %d adds NOT NULL column %s.%s without a default", afterVersion, table, name)
		}
		assert.Equal(t, before.checks[table], after.checks[table],
			"migration %d changes the CHECK constraints on %q", afterVersion, table)
	}
	for name, index := range before.indexes {
		updated, found := after.indexes[name]
		require.True(t, found, "migration %d drops index %q", afterVersion, name)
		assert.Equal(t, index, updated, "migration %d redefines index %q", afterVersion, name)
	}
	for name, index := range after.indexes {
		if _, existed := before.indexes[name]; existed || !index.unique {
			continue
		}
		_, onExistingTable := before.tables[index.table]
		assert.False(t, onExistingTable,
			"migration %d adds UNIQUE index %q to existing table %q", afterVersion, name, index.table)
	}
}

func readMigrationSchema(t *testing.T, files fs.FS, directory string, version int) migrationSchema {
	t.Helper()
	databasePath := filepath.Join(t.TempDir(), "state.db")
	migration, closeMigration, err := openMigrator(databasePath, files, directory)
	require.NoError(t, err)
	require.NoError(t, migration.Migrate(uint(version)))
	closeMigration()

	database, err := openDatabase(databasePath)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, database.Close()) })

	schema := migrationSchema{
		tables:  make(map[string]map[string]migrationColumn),
		checks:  make(map[string][]string),
		indexes: make(map[string]migrationIndex),
	}
	for name, definition := range readMigrationTableDefinitions(t, database) {
		schema.tables[name] = readMigrationColumns(t, database, name)
		schema.checks[name] = checkConstraints(definition)
		maps.Copy(schema.indexes, readMigrationIndexes(t, database, name))
	}
	return schema
}

func readMigrationTableDefinitions(t *testing.T, database *sql.DB) map[string]string {
	t.Helper()
	rows, err := database.QueryContext(t.Context(), `
		SELECT name, COALESCE(sql, '') FROM sqlite_master
		WHERE type = 'table' AND name NOT LIKE 'sqlite_%' AND name NOT IN (?, ?)
	`, "schema_migrations", migrationsTable)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()

	definitions := make(map[string]string)
	for rows.Next() {
		var name, definition string
		require.NoError(t, rows.Scan(&name, &definition))
		definitions[name] = definition
	}
	require.NoError(t, rows.Err())
	return definitions
}

func readMigrationColumns(t *testing.T, database *sql.DB, table string) map[string]migrationColumn {
	t.Helper()
	rows, err := database.QueryContext(t.Context(), `SELECT name, type, "notnull", dflt_value, pk FROM pragma_table_info(?)`, table)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()

	columns := make(map[string]migrationColumn)
	for rows.Next() {
		var name, declaredType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		require.NoError(t, rows.Scan(&name, &declaredType, &notNull, &defaultValue, &primaryKey))
		columns[name] = migrationColumn{
			declaredType: declaredType,
			nullable:     notNull == 0,
			hasDefault:   defaultValue.Valid && !strings.EqualFold(defaultValue.String, "NULL"),
			primaryKey:   primaryKey > 0,
		}
	}
	require.NoError(t, rows.Err())
	return columns
}

func readMigrationIndexes(t *testing.T, database *sql.DB, table string) map[string]migrationIndex {
	t.Helper()
	indexes := readMigrationIndexHeaders(t, database, table)
	for name, index := range indexes {
		shape, err := indexFingerprint(t.Context(), database, name)
		require.NoError(t, err)
		index.shape = shape
		indexes[name] = index
	}
	return indexes
}

func readMigrationIndexHeaders(t *testing.T, database *sql.DB, table string) map[string]migrationIndex {
	t.Helper()
	rows, err := database.QueryContext(t.Context(), `SELECT name, "unique", origin, partial FROM pragma_index_list(?)`, table)
	require.NoError(t, err)
	defer func() { assert.NoError(t, rows.Close()) }()

	indexes := make(map[string]migrationIndex)
	for rows.Next() {
		var name, origin string
		var unique, partial int
		require.NoError(t, rows.Scan(&name, &unique, &origin, &partial))
		indexes[name] = migrationIndex{table: table, unique: unique == 1, origin: origin, partial: partial == 1}
	}
	require.NoError(t, rows.Err())
	return indexes
}

func TestStorePreservesPreviousReleaseRollbackWindow(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	require.NoError(t, store.Close())
	database, err := sql.Open(driverName, databaseDSN(databasePath))
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `UPDATE domestique_migrations SET version = ?; INSERT INTO schema_migrations (version, applied_at_unix) VALUES (?, 0)`, currentSchemaVersion+1, currentSchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	rolledBack, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	require.NoError(t, rolledBack.Close())
	database, err = sql.Open(driverName, databaseDSN(databasePath))
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `UPDATE domestique_migrations SET version = ?; INSERT INTO schema_migrations (version, applied_at_unix) VALUES (?, 0)`, currentSchemaVersion+2, currentSchemaVersion+2)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	_, err = Open(t.Context(), databasePath, testKey(1))
	require.Error(t, err)
	assert.Contains(t, err.Error(), schemaAheadMessage)
}

func TestStoreChecksForeignKeysDuringRollbackWindow(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, err := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, err)
	require.NoError(t, store.Close())
	database, err := sql.Open(driverName, databasePath+"?_foreign_keys=off")
	require.NoError(t, err)
	_, err = database.ExecContext(t.Context(), `
		UPDATE domestique_migrations SET version = ?;
		INSERT INTO schema_migrations (version, applied_at_unix) VALUES (?, 0);
		INSERT INTO oauth_transactions (id, target_slot, state_digest, code_verifier, expires_at_unix) VALUES ('broken', 'missing', X'00', X'00', 0);
	`, currentSchemaVersion+1, currentSchemaVersion+1)
	require.NoError(t, err)
	require.NoError(t, database.Close())
	_, err = Open(t.Context(), databasePath, testKey(1))
	require.ErrorContains(t, err, "state foreign key check failed")
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
