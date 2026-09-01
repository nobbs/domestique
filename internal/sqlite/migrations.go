package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

const (
	currentSchemaVersion = 27
	migrationsTable      = "domestique_migrations"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

func (s *Store) migrate(ctx context.Context, databasePath string) error {
	legacyVersion, legacyExists, err := legacySchemaVersion(ctx, s.database)
	if err != nil {
		return err
	}
	trackedVersion, tracked, dirty, err := migrationVersion(ctx, s.database)
	if err != nil {
		return err
	}
	if tracked {
		if dirty {
			return errors.New("state schema migration is dirty")
		}
		if trackedVersion > currentSchemaVersion+forwardCompatibleMigrations {
			return fmt.Errorf("%s: the state file is at version %d and this service knows %d", schemaAheadMessage, trackedVersion, currentSchemaVersion)
		}
		if trackedVersion < currentSchemaVersion || !legacyExists || legacyVersion != trackedVersion {
			return errors.New("state schema migration history is not current")
		}
		if trackedVersion == currentSchemaVersion {
			return validateCurrentSchema(ctx, s.database)
		}
		return nil
	}
	if legacyExists {
		if legacyVersion != currentSchemaVersion {
			return fmt.Errorf("state schema version %d is not supported", legacyVersion)
		}
		if err := validateCurrentSchema(ctx, s.database); err != nil {
			return err
		}
		migration, closeMigration, err := openMigrator(databasePath)
		if err != nil {
			return err
		}
		defer closeMigration()
		if err := migration.Force(currentSchemaVersion); err != nil {
			return fmt.Errorf("recording current state baseline: %w", err)
		}
		return nil
	}
	if empty, err := databaseIsEmpty(ctx, s.database); err != nil {
		return err
	} else if !empty {
		return errors.New("state schema is unrecognised")
	}
	migration, closeMigration, err := openMigrator(databasePath)
	if err != nil {
		return err
	}
	defer closeMigration()
	if err := migration.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying state baseline: %w", err)
	}
	return validateCurrentSchema(ctx, s.database)
}

func openMigrator(databasePath string) (*migrate.Migrate, func(), error) {
	database, err := openDatabase(databasePath)
	if err != nil {
		return nil, nil, fmt.Errorf("opening state migration database: %w", err)
	}
	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		closeDatabase(database)
		return nil, nil, fmt.Errorf("opening state migrations: %w", err)
	}
	driver, err := sqlite.WithInstance(database, &sqlite.Config{MigrationsTable: migrationsTable})
	if err != nil {
		closeDatabase(database)
		return nil, nil, fmt.Errorf("opening state migration registry: %w", err)
	}
	migration, err := migrate.NewWithInstance("iofs", source, migrationsTable, driver)
	if err != nil {
		closeDatabase(database)
		return nil, nil, fmt.Errorf("starting state migrations: %w", err)
	}
	return migration, func() { _, _ = migration.Close() }, nil
}

func migrationVersion(ctx context.Context, database *sql.DB) (version int, exists, dirty bool, err error) {
	exists, err = hasTable(ctx, database, migrationsTable)
	if err != nil || !exists {
		return 0, exists, false, err
	}
	var storedDirty bool
	err = database.QueryRowContext(ctx, `SELECT version, dirty FROM domestique_migrations LIMIT 1`).Scan(&version, &storedDirty)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, true, false, errors.New("state schema migration history is empty")
	}
	if err != nil {
		return 0, true, false, fmt.Errorf("reading state schema migration version: %w", err)
	}
	return version, true, storedDirty, nil
}

func legacySchemaVersion(ctx context.Context, database *sql.DB) (int, bool, error) {
	exists, err := hasTable(ctx, database, "schema_migrations")
	if err != nil || !exists {
		return 0, exists, err
	}
	var count, minimum, maximum int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MIN(version), 0), COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&count, &minimum, &maximum); err != nil {
		return 0, true, fmt.Errorf("reading legacy state schema version: %w", err)
	}
	if minimum != 1 || count != maximum {
		return 0, true, errors.New("legacy state schema migration history is not contiguous")
	}
	return maximum, true, nil
}

func hasTable(ctx context.Context, database *sql.DB, table string) (bool, error) {
	var found int
	err := database.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = ?)`, table).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("reading state schema: %w", err)
	}
	return found == 1, nil
}

func databaseIsEmpty(ctx context.Context, database *sql.DB) (bool, error) {
	var objects int
	if err := database.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE name NOT LIKE 'sqlite_%'`).Scan(&objects); err != nil {
		return false, fmt.Errorf("reading state schema: %w", err)
	}
	return objects == 0, nil
}

func validateCurrentSchema(ctx context.Context, actual *sql.DB) error {
	if err := databaseHealthy(ctx, actual); err != nil {
		return err
	}
	reference, err := openDatabase("file:domestique-schema-reference?mode=memory&cache=private")
	if err != nil {
		return err
	}
	defer closeDatabase(reference)
	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return err
	}
	driver, err := sqlite.WithInstance(reference, &sqlite.Config{MigrationsTable: migrationsTable})
	if err != nil {
		return err
	}
	migration, err := migrate.NewWithInstance("iofs", source, migrationsTable, driver)
	if err != nil {
		return err
	}
	defer func() { _, _ = migration.Close() }()
	if err := migration.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("building state schema reference: %w", err)
	}
	expected, err := schemaFingerprint(ctx, reference)
	if err != nil {
		return err
	}
	got, err := schemaFingerprint(ctx, actual)
	if err != nil {
		return err
	}
	if strings.Join(expected, "\n") != strings.Join(got, "\n") {
		return errors.New("state schema differs from the current baseline")
	}
	return nil
}

func databaseHealthy(ctx context.Context, database *sql.DB) error {
	var integrity string
	if err := database.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil {
		return fmt.Errorf("checking state integrity: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("state integrity check failed: %s", integrity)
	}
	rows, err := database.QueryContext(ctx, `PRAGMA foreign_key_check`)
	if err != nil {
		return fmt.Errorf("checking state foreign keys: %w", err)
	}
	defer closeRows(rows)
	if rows.Next() {
		return errors.New("state foreign key check failed")
	}
	return rows.Err()
}

func schemaFingerprint(ctx context.Context, database *sql.DB) ([]string, error) {
	rows, err := database.QueryContext(ctx, `SELECT type, name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND name NOT IN (?, ?) ORDER BY type, name`, "schema_migrations", migrationsTable)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	var objects []string
	for rows.Next() {
		var kind, name string
		if err := rows.Scan(&kind, &name); err != nil {
			return nil, err
		}
		switch kind {
		case "table":
			shape, err := tableFingerprint(ctx, database, name)
			if err != nil {
				return nil, err
			}
			objects = append(objects, "table "+name+" "+shape)
		case "index":
			shape, err := indexFingerprint(ctx, database, name)
			if err != nil {
				return nil, err
			}
			objects = append(objects, "index "+name+" "+shape)
		default:
			return nil, fmt.Errorf("unexpected state schema object %s %q", kind, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(objects)
	return objects, nil
}

func tableFingerprint(ctx context.Context, database *sql.DB, table string) (string, error) {
	rows, err := database.QueryContext(ctx, `SELECT cid, name, type, "notnull", dflt_value, pk FROM pragma_table_info(?)`, table)
	if err != nil {
		return "", err
	}
	defer closeRows(rows)
	var columns []string
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey); err != nil {
			return "", err
		}
		columns = append(columns, fmt.Sprintf("%d:%s:%s:%d:%s:%d", cid, name, kind, notNull, defaultValue.String, primaryKey))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	foreignKeys, err := pragmaRows(ctx, database, `SELECT id, seq, "table", "from", "to", on_update, on_delete, match FROM pragma_foreign_key_list(?)`, table)
	if err != nil {
		return "", err
	}
	indexes, err := pragmaRows(ctx, database, `SELECT "unique", origin, partial, group_concat(name, ',') FROM pragma_index_list(?) JOIN pragma_index_info(name) GROUP BY name ORDER BY name`, table)
	if err != nil {
		return "", err
	}
	var definition string
	if err := database.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&definition); err != nil {
		return "", err
	}
	checks := regexp.MustCompile(`(?i)CHECK\\s*\\(([^()]*)\\)`).FindAllStringSubmatch(definition, -1)
	var checkParts []string
	for _, check := range checks {
		checkParts = append(checkParts, strings.Join(strings.Fields(strings.ToLower(check[1])), " "))
	}
	return strings.Join(columns, ",") + "|" + strings.Join(foreignKeys, ",") + "|" + strings.Join(indexes, ",") + "|" + strings.Join(checkParts, ","), nil
}

func pragmaRows(ctx context.Context, database *sql.DB, query, table string) ([]string, error) {
	rows, err := database.QueryContext(ctx, query, table)
	if err != nil {
		return nil, err
	}
	defer closeRows(rows)
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var values []string
	for rows.Next() {
		items := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range items {
			pointers[index] = &items[index]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, err
		}
		parts := make([]string, len(items))
		for index, item := range items {
			parts[index] = fmt.Sprint(item)
		}
		values = append(values, strings.Join(parts, ":"))
	}
	return values, rows.Err()
}

func indexFingerprint(ctx context.Context, database *sql.DB, index string) (string, error) {
	rows, err := database.QueryContext(ctx, `SELECT seqno, cid, name FROM pragma_index_info(?) ORDER BY seqno`, index)
	if err != nil {
		return "", err
	}
	defer closeRows(rows)
	var columns []string
	for rows.Next() {
		var sequence, column int
		var name string
		if err := rows.Scan(&sequence, &column, &name); err != nil {
			return "", err
		}
		columns = append(columns, fmt.Sprintf("%d:%d:%s", sequence, column, name))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	return strings.Join(columns, ","), nil
}
