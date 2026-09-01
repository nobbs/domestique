package sqlite

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
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

type schemaObject struct {
	kind string
	name string
}

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
		schemaErr := validateCurrentSchema(ctx, s.database)
		if schemaErr != nil {
			return schemaErr
		}
		migration, closeMigration, migrationErr := openMigrator(databasePath)
		if migrationErr != nil {
			return migrationErr
		}
		defer closeMigration()
		forceErr := migration.Force(currentSchemaVersion)
		if forceErr != nil {
			return fmt.Errorf("recording current state baseline: %w", forceErr)
		}
		return nil
	}
	empty, emptyErr := databaseIsEmpty(ctx, s.database)
	if emptyErr != nil {
		return emptyErr
	} else if !empty {
		return errors.New("state schema is unrecognised")
	}
	migration, closeMigration, migrationErr := openMigrator(databasePath)
	if migrationErr != nil {
		return migrationErr
	}
	defer closeMigration()
	upErr := migration.Up()
	if upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		return fmt.Errorf("applying state baseline: %w", upErr)
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
	return migration, func() { closeMigrator(migration) }, nil
}

//nolint:errcheck // A migration cleanup error cannot replace the migration result.
func closeMigrator(migration *migrate.Migrate) {
	_, _ = migration.Close()
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

func legacySchemaVersion(ctx context.Context, database *sql.DB) (version int, exists bool, err error) {
	exists, err = hasTable(ctx, database, "schema_migrations")
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
		return fmt.Errorf("opening state schema reference: %w", err)
	}
	defer closeDatabase(reference)
	source, err := iofs.New(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("opening state schema reference migrations: %w", err)
	}
	driver, err := sqlite.WithInstance(reference, &sqlite.Config{MigrationsTable: migrationsTable})
	if err != nil {
		return fmt.Errorf("opening state schema reference registry: %w", err)
	}
	migration, err := migrate.NewWithInstance("iofs", source, migrationsTable, driver)
	if err != nil {
		return fmt.Errorf("starting state schema reference migrations: %w", err)
	}
	defer closeMigrator(migration)
	upErr := migration.Up()
	if upErr != nil && !errors.Is(upErr, migrate.ErrNoChange) {
		return fmt.Errorf("building state schema reference: %w", upErr)
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
	if err := rows.Err(); err != nil {
		return fmt.Errorf("checking state foreign keys: %w", err)
	}
	return nil
}

func schemaFingerprint(ctx context.Context, database *sql.DB) ([]string, error) {
	schemaObjects, err := listSchemaObjects(ctx, database)
	if err != nil {
		return nil, err
	}

	var objects []string
	for _, object := range schemaObjects {
		switch object.kind {
		case "table":
			shape, err := tableFingerprint(ctx, database, object.name)
			if err != nil {
				return nil, err
			}
			objects = append(objects, "table "+object.name+" "+shape)
		case "index":
			shape, err := indexFingerprint(ctx, database, object.name)
			if err != nil {
				return nil, err
			}
			objects = append(objects, "index "+object.name+" "+shape)
		default:
			return nil, fmt.Errorf("unexpected state schema object %s %q", object.kind, object.name)
		}
	}
	sort.Strings(objects)
	return objects, nil
}

func listSchemaObjects(ctx context.Context, database *sql.DB) ([]schemaObject, error) {
	rows, err := database.QueryContext(ctx, `SELECT type, name FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' AND tbl_name NOT IN (?, ?) ORDER BY type, name`, "schema_migrations", migrationsTable)
	if err != nil {
		return nil, fmt.Errorf("listing state schema objects: %w", err)
	}
	defer closeRows(rows)
	var schemaObjects []schemaObject
	for rows.Next() {
		var kind, name string
		scanErr := rows.Scan(&kind, &name)
		if scanErr != nil {
			return nil, fmt.Errorf("reading state schema object: %w", scanErr)
		}
		schemaObjects = append(schemaObjects, schemaObject{kind: kind, name: name})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("listing state schema objects: %w", rowsErr)
	}
	return schemaObjects, nil
}

func tableFingerprint(ctx context.Context, database *sql.DB, table string) (string, error) {
	rows, err := database.QueryContext(ctx, `SELECT cid, name, type, "notnull", dflt_value, pk FROM pragma_table_info(?)`, table)
	if err != nil {
		return "", fmt.Errorf("reading state table %q: %w", table, err)
	}
	defer closeRows(rows)
	var columns []string
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, kind string
		var defaultValue sql.NullString
		scanErr := rows.Scan(&cid, &name, &kind, &notNull, &defaultValue, &primaryKey)
		if scanErr != nil {
			return "", fmt.Errorf("reading state table %q: %w", table, scanErr)
		}
		columns = append(columns, fmt.Sprintf("%d:%s:%s:%d:%s:%d", cid, name, kind, notNull, defaultValue.String, primaryKey))
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return "", fmt.Errorf("reading state table %q: %w", table, rowsErr)
	}
	foreignKeys, err := pragmaRows(ctx, database, `SELECT id, seq, "table", "from", "to", on_update, on_delete, match FROM pragma_foreign_key_list(?)`, table)
	if err != nil {
		return "", err
	}
	indexes, err := pragmaRows(ctx, database, `SELECT index_list."unique", index_list.origin, index_list.partial, group_concat(index_info.name, ',') FROM pragma_index_list(?) AS index_list JOIN pragma_index_info(index_list.name) AS index_info GROUP BY index_list.name, index_list."unique", index_list.origin, index_list.partial ORDER BY index_list.name`, table)
	if err != nil {
		return "", err
	}
	var definition string
	if err := database.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&definition); err != nil {
		return "", fmt.Errorf("reading state table %q definition: %w", table, err)
	}
	checkParts := checkConstraints(definition)
	return strings.Join(columns, ",") + "|" + strings.Join(foreignKeys, ",") + "|" + strings.Join(indexes, ",") + "|" + strings.Join(checkParts, ","), nil
}

func pragmaRows(ctx context.Context, database *sql.DB, query, table string) ([]string, error) {
	rows, err := database.QueryContext(ctx, query, table)
	if err != nil {
		return nil, fmt.Errorf("reading state schema pragma: %w", err)
	}
	defer closeRows(rows)
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("reading state schema pragma columns: %w", err)
	}
	var values []string
	for rows.Next() {
		items := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for index := range items {
			pointers[index] = &items[index]
		}
		scanErr := rows.Scan(pointers...)
		if scanErr != nil {
			return nil, fmt.Errorf("reading state schema pragma row: %w", scanErr)
		}
		parts := make([]string, len(items))
		for index, item := range items {
			parts[index] = fmt.Sprint(item)
		}
		values = append(values, strings.Join(parts, ":"))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading state schema pragma: %w", err)
	}
	return values, nil
}

func indexFingerprint(ctx context.Context, database *sql.DB, index string) (string, error) {
	rows, err := database.QueryContext(ctx, `SELECT seqno, cid, name FROM pragma_index_info(?) ORDER BY seqno`, index)
	if err != nil {
		return "", fmt.Errorf("reading state index %q: %w", index, err)
	}
	defer closeRows(rows)
	var columns []string
	for rows.Next() {
		var sequence, column int
		var name string
		scanErr := rows.Scan(&sequence, &column, &name)
		if scanErr != nil {
			return "", fmt.Errorf("reading state index %q: %w", index, scanErr)
		}
		columns = append(columns, fmt.Sprintf("%d:%d:%s", sequence, column, name))
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return "", fmt.Errorf("reading state index %q: %w", index, rowsErr)
	}
	return strings.Join(columns, ","), nil
}

func checkConstraints(definition string) []string {
	definition = strings.ToLower(definition)
	checks := []string{}
	for offset := 0; offset < len(definition); {
		check := strings.Index(definition[offset:], "check")
		if check < 0 {
			break
		}
		opening := offset + check + len("check")
		for opening < len(definition) && (definition[opening] == ' ' || definition[opening] == '\n' || definition[opening] == '\t') {
			opening++
		}
		if opening == len(definition) || definition[opening] != '(' {
			offset = opening
			continue
		}
		depth := 0
		var quote byte
		for closing := opening; closing < len(definition); closing++ {
			character := definition[closing]
			if quote != 0 {
				if character == quote {
					quote = 0
				}
				continue
			}
			if character == '\'' || character == '"' {
				quote = character
				continue
			}
			switch character {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					checks = append(checks, strings.Join(strings.Fields(definition[opening+1:closing]), " "))
					offset = closing + 1
					goto next
				}
			}
		}
		break
	next:
	}
	return checks
}
