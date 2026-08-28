package sqlite

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
