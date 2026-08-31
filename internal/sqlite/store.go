// Package sqlite persists Domestique's encrypted, local reconciliation state.
package sqlite

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite driver registration.
)

const driverName = "sqlite"

// forwardCompatibleMigrations is how far ahead of this binary a state file may
// be and still open: one, so a deploy that failed its health gate can roll back
// onto the previous binary. A release that must stay rollable appends one.
const forwardCompatibleMigrations = 1

// schemaAheadMessage is the stable prefix of that refusal. It is a contract with
// deploy/domestique-deploy.sh, which matches on it.
const schemaAheadMessage = "state schema version is newer than this service"

var (
	// ErrTargetNotFound reports an operation for an unknown configured target slot.
	ErrTargetNotFound = errors.New("target slot not found")
	// ErrRefreshTokenUnavailable reports a target that needs an authorization flow.
	ErrRefreshTokenUnavailable = errors.New("refresh token is unavailable")
	// ErrWahooUserAlreadyAuthorized reports an account already bound to another slot.
	ErrWahooUserAlreadyAuthorized = errors.New("wahoo user is already authorized for another target slot")
	// ErrStateUnreadable reports encrypted state that cannot be authenticated.
	ErrStateUnreadable = errors.New("encrypted state is unreadable")
	// ErrOAuthTransactionNotFound reports an unknown OAuth callback state.
	ErrOAuthTransactionNotFound = errors.New("oauth transaction was not found")
	// ErrOAuthTransactionExpired reports a callback state that exceeded its deadline.
	ErrOAuthTransactionExpired = errors.New("oauth transaction has expired")
	// ErrOAuthTransactionUsed reports a callback state that was already consumed.
	ErrOAuthTransactionUsed = errors.New("oauth transaction was already used")
	// ErrOAuthTransactionIdentityMismatch reports a callback from another identity.
	ErrOAuthTransactionIdentityMismatch = errors.New("oauth transaction identity did not match")
)

// Store is an SQLite-backed state store whose OAuth tokens use AES-GCM at rest.
type Store struct {
	database *sql.DB
	aead     cipher.AEAD
}

// Open creates or opens a state database, applies pending migrations, and
// configures encrypted token storage. The database path must be absolute.
func Open(ctx context.Context, databasePath string, encryptionKey [32]byte) (*Store, error) {
	if !filepath.IsAbs(databasePath) {
		return nil, errors.New("database path must be absolute")
	}
	if err := os.MkdirAll(filepath.Dir(databasePath), 0o700); err != nil {
		return nil, fmt.Errorf("creating state directory: %w", err)
	}

	block, err := aes.NewCipher(encryptionKey[:])
	if err != nil {
		return nil, fmt.Errorf("creating state cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("creating state cipher: %w", err)
	}

	database, err := sql.Open(driverName, databasePath)
	if err != nil {
		return nil, fmt.Errorf("opening state database: %w", err)
	}
	database.SetMaxOpenConns(1)
	database.SetMaxIdleConns(1)

	store := &Store{database: database, aead: aead}
	if err := store.configure(ctx); err != nil {
		closeDatabase(database)
		return nil, err
	}
	if err := store.migrate(ctx); err != nil {
		closeDatabase(database)
		return nil, err
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		closeDatabase(database)
		return nil, fmt.Errorf("protecting state database: %w", err)
	}

	return store, nil
}

// Close releases the database connection.
func (s *Store) Close() error {
	if err := s.database.Close(); err != nil {
		return fmt.Errorf("closing state database: %w", err)
	}

	return nil
}

func (s *Store) configure(ctx context.Context) error {
	if err := s.database.PingContext(ctx); err != nil {
		return fmt.Errorf("opening state database: %w", err)
	}
	for _, statement := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := s.database.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("configuring state database: %w", err)
		}
	}

	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting state migration: %w", err)
	}
	defer rollback(transaction)

	if _, err := transaction.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at_unix INTEGER NOT NULL
		)
	`); err != nil {
		return fmt.Errorf("creating migration registry: %w", err)
	}

	var currentVersion int
	if err := transaction.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion); err != nil {
		return fmt.Errorf("reading state schema version: %w", err)
	}
	migrations := schemaMigrations()
	if currentVersion > len(migrations)+forwardCompatibleMigrations {
		return fmt.Errorf("%s: the state file is at version %d and this service knows %d",
			schemaAheadMessage, currentVersion, len(migrations))
	}

	for version := currentVersion + 1; version <= len(migrations); version++ {
		for _, statement := range migrations[version-1] {
			if _, err := transaction.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("applying state migration %d: %w", version, err)
			}
		}
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO schema_migrations (version, applied_at_unix) VALUES (?, ?)
		`, version, time.Now().Unix()); err != nil {
			return fmt.Errorf("recording state migration %d: %w", version, err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing state migration: %w", err)
	}

	return nil
}

//nolint:errcheck // A cleanup error cannot replace the already-returned state error.
func closeDatabase(database *sql.DB) {
	_ = database.Close()
}

//nolint:errcheck // A query cleanup error is superseded by the query result.
func closeRows(rows *sql.Rows) {
	_ = rows.Close()
}

//nolint:errcheck // Rollback after a committed transaction is expected to return sql.ErrTxDone.
func rollback(transaction *sql.Tx) {
	_ = transaction.Rollback()
}

// schemaMigrations is the ordered, append-only history of this database. A
// migration is identified by its position, so one is appended and never inserted.
//
// A migration must leave the previous release's binary working — additive with
// defaults. It must not:
//
//   - drop or rename a table, column, or index an earlier binary reads or writes;
//   - add a NOT NULL column without a default to a table an earlier binary
//     inserts into;
//   - tighten a CHECK or add a UNIQUE index an earlier binary's writes could
//     violate; or
//   - change what the values of an existing column mean.
//
// TestNewMigrationsStayReadableByThePreviousRelease enforces the structural half.
func schemaMigrations() [][]string {
	return [][]string{
		{
			`CREATE TABLE targets (
			slot TEXT PRIMARY KEY,
			wahoo_user_id TEXT UNIQUE,
			refresh_token BLOB,
			authorization_state TEXT NOT NULL CHECK (authorization_state IN ('not_authorized', 'authorized', 'needs_reauthorization')),
			updated_at_unix INTEGER NOT NULL
		)`,
			`CREATE TABLE oauth_transactions (
			id TEXT PRIMARY KEY,
			target_slot TEXT NOT NULL REFERENCES targets(slot),
			state_digest BLOB NOT NULL,
			code_verifier BLOB NOT NULL,
			expires_at_unix INTEGER NOT NULL,
			used_at_unix INTEGER
		)`,
			`CREATE TABLE source_stages (
			route_id INTEGER NOT NULL,
			stage_order INTEGER NOT NULL,
			source_revision TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			PRIMARY KEY (route_id, stage_order)
		)`,
			`CREATE TABLE target_stages (
			target_slot TEXT NOT NULL REFERENCES targets(slot),
			route_id INTEGER NOT NULL,
			stage_order INTEGER NOT NULL,
			wahoo_route_id INTEGER NOT NULL,
			content_hash TEXT NOT NULL,
			PRIMARY KEY (target_slot, route_id, stage_order)
		)`,
			`CREATE TABLE trusted_inventory (
			target_slot TEXT PRIMARY KEY REFERENCES targets(slot),
			captured_at_unix INTEGER NOT NULL
		)`,
			`CREATE TABLE trusted_inventory_stages (
			target_slot TEXT NOT NULL REFERENCES trusted_inventory(target_slot),
			route_id INTEGER NOT NULL,
			stage_order INTEGER NOT NULL,
			wahoo_route_id INTEGER NOT NULL,
			PRIMARY KEY (target_slot, route_id, stage_order)
		)`,
			`CREATE TABLE sync_runs (
			id INTEGER PRIMARY KEY,
			started_at_unix INTEGER NOT NULL,
			finished_at_unix INTEGER,
			outcome TEXT NOT NULL,
			detail TEXT
		)`,
			`CREATE TABLE notification_state (
			kind TEXT PRIMARY KEY,
			last_sent_at_unix INTEGER NOT NULL
		)`,
		},
		{
			`ALTER TABLE oauth_transactions ADD COLUMN caller_login TEXT NOT NULL DEFAULT ''`,
			`CREATE UNIQUE INDEX oauth_transactions_state_digest_index ON oauth_transactions(state_digest)`,
		},
		{
			`ALTER TABLE target_stages ADD COLUMN source_revision TEXT NOT NULL DEFAULT ''`,
		},
		{
			`ALTER TABLE sync_runs ADD COLUMN source_stages INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE sync_runs ADD COLUMN created INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE sync_runs ADD COLUMN updated INTEGER NOT NULL DEFAULT 0`,
			`ALTER TABLE sync_runs ADD COLUMN deleted INTEGER NOT NULL DEFAULT 0`,
		},
		{
			// The route map view's rendering cache. Separate from source_stages, which
			// backs the deletion guard: this may be dropped at any time.
			`CREATE TABLE stage_geometry (
				route_id        INTEGER NOT NULL,
				stage_order     INTEGER NOT NULL,
				content_hash    TEXT    NOT NULL,
				route_name      TEXT    NOT NULL,
				stage_name      TEXT    NOT NULL,
				point_count     INTEGER NOT NULL,
				distance_metres REAL    NOT NULL,
				min_longitude   REAL    NOT NULL,
				min_latitude    REAL    NOT NULL,
				max_longitude   REAL    NOT NULL,
				max_latitude    REAL    NOT NULL,
				coordinates     BLOB    NOT NULL,
				updated_at_unix INTEGER NOT NULL,
				PRIMARY KEY (route_id, stage_order)
			)`,
		},
		{
			// Climbing and steepest gradient, derived from the stored profile.
			// Presentation only; existing rows refill on the next content-hash change.
			`ALTER TABLE stage_geometry ADD COLUMN ascent_metres REAL NOT NULL DEFAULT 0`,
			`ALTER TABLE stage_geometry ADD COLUMN max_gradient_percent REAL NOT NULL DEFAULT 0`,
			// Geometry is rewritten only when a content hash changes, so clearing the
			// hash is what makes the next run refill the new columns once.
			`UPDATE stage_geometry SET content_hash = ''`,
		},
		{
			// The surface classification of each stage, filled by a later pass, so a
			// stage may sit here missing. content_hash records the geometry the ranges
			// were measured against, and every read matches on it.
			`CREATE TABLE stage_surface (
				route_id        INTEGER NOT NULL,
				stage_order     INTEGER NOT NULL,
				content_hash    TEXT    NOT NULL,
				ranges          BLOB    NOT NULL,
				matched_metres  REAL    NOT NULL,
				updated_at_unix INTEGER NOT NULL,
				PRIMARY KEY (route_id, stage_order)
			)`,
		},
		{
			// Which half of a synchronization the timer may start. One row, defaulted
			// to both halves on.
			`CREATE TABLE sync_schedule (
				id              INTEGER PRIMARY KEY CHECK (id = 1),
				source_enabled  INTEGER NOT NULL CHECK (source_enabled IN (0, 1)),
				targets_enabled INTEGER NOT NULL CHECK (targets_enabled IN (0, 1)),
				updated_at_unix INTEGER NOT NULL
			)`,
			`INSERT INTO sync_schedule (id, source_enabled, targets_enabled, updated_at_unix)
				VALUES (1, 1, 1, 0)`,
			// Runs are recorded per phase. Rows written earlier keep an empty phase
			// rather than claiming to be one of them.
			`ALTER TABLE sync_runs ADD COLUMN phase TEXT NOT NULL DEFAULT ''`,
		},
		{
			// One stage an operator has asked to be redone. Its own table because storing
			// an inventory replaces every source_stages row. Cleared by the pass that
			// consumes it.
			`CREATE TABLE stage_reprocess (
				route_id          INTEGER NOT NULL,
				stage_order       INTEGER NOT NULL,
				requested_at_unix INTEGER NOT NULL,
				PRIMARY KEY (route_id, stage_order)
			)`,
		},
		{
			// The last recorded reconciliation of each target, one row per slot. sync_runs
			// answers whether a run succeeded, not which slot is behind.
			`CREATE TABLE target_runs (
				target_slot      TEXT PRIMARY KEY REFERENCES targets(slot),
				finished_at_unix INTEGER NOT NULL,
				outcome          TEXT    NOT NULL,
				detail           TEXT    NOT NULL
			)`,
		},
		{
			// A name for one recorded run, carrying no meaning: random bytes, so a
			// notification and the record an operator opens are legibly the same run.
			`ALTER TABLE sync_runs ADD COLUMN reference TEXT NOT NULL DEFAULT ''`,
			// Existing rows are named here, so history predating this migration is as
			// addressable as history after it.
			`UPDATE sync_runs SET reference = lower(hex(randomblob(6))) WHERE reference = ''`,
			// Not unique: an earlier binary rolled back onto this schema still inserts
			// rows carrying the column's default.
			`CREATE INDEX sync_runs_reference_index ON sync_runs(reference)`,
		},
		{
			// Which build of the local map a cached classification was measured against.
			// A new index is a new set of answers, so the generation is part of what makes
			// a cached classification current. Existing rows default to the empty
			// generation and reclassify once.
			`ALTER TABLE stage_surface ADD COLUMN index_generation TEXT NOT NULL DEFAULT ''`,
			// When the index was last built, and what it produced. One row, read at
			// startup, which turns the rebuild interval into time between builds.
			`CREATE TABLE surface_index (
				id            INTEGER PRIMARY KEY CHECK (id = 1),
				built_at_unix INTEGER NOT NULL,
				generation    TEXT    NOT NULL
			)`,
			`INSERT INTO surface_index (id, built_at_unix, generation) VALUES (1, 0, '')`,
		},
		{
			// A stage identity now names the provider that issued its route ID. Every
			// table keying a stage by (route_id, stage_order) gains a provider column
			// joined into the key; SQLite cannot widen a PRIMARY KEY in place, so each
			// is rebuilt.
			//
			// This is the accepted exception to the rollback rule above, recorded in
			// docs/specs/service.md. A previous release can still read all four rebuilt
			// tables but cannot write target_stages, stage_geometry, stage_surface, or
			// stage_reprocess: it issues ON CONFLICT (route_id, stage_order), which no
			// longer names an existing unique constraint.
			`CREATE TABLE source_stages_v13 (
				provider TEXT NOT NULL DEFAULT 'veloplanner',
				route_id INTEGER NOT NULL,
				stage_order INTEGER NOT NULL,
				source_revision TEXT NOT NULL,
				content_hash TEXT NOT NULL,
				PRIMARY KEY (provider, route_id, stage_order)
			)`,
			`INSERT INTO source_stages_v13 (provider, route_id, stage_order, source_revision, content_hash)
				SELECT 'veloplanner', route_id, stage_order, source_revision, content_hash FROM source_stages`,
			`DROP TABLE source_stages`,
			`ALTER TABLE source_stages_v13 RENAME TO source_stages`,
			`CREATE TABLE target_stages_v13 (
				target_slot TEXT NOT NULL REFERENCES targets(slot),
				provider TEXT NOT NULL DEFAULT 'veloplanner',
				route_id INTEGER NOT NULL,
				stage_order INTEGER NOT NULL,
				wahoo_route_id INTEGER NOT NULL,
				content_hash TEXT NOT NULL,
				source_revision TEXT NOT NULL DEFAULT '',
				PRIMARY KEY (target_slot, provider, route_id, stage_order)
			)`,
			`INSERT INTO target_stages_v13 (
				target_slot, provider, route_id, stage_order, wahoo_route_id, content_hash, source_revision
			)
				SELECT target_slot, 'veloplanner', route_id, stage_order, wahoo_route_id, content_hash, source_revision
				FROM target_stages`,
			`DROP TABLE target_stages`,
			`ALTER TABLE target_stages_v13 RENAME TO target_stages`,
			`CREATE TABLE trusted_inventory_stages_v13 (
				target_slot TEXT NOT NULL REFERENCES trusted_inventory(target_slot),
				provider TEXT NOT NULL DEFAULT 'veloplanner',
				route_id INTEGER NOT NULL,
				stage_order INTEGER NOT NULL,
				wahoo_route_id INTEGER NOT NULL,
				PRIMARY KEY (target_slot, provider, route_id, stage_order)
			)`,
			`INSERT INTO trusted_inventory_stages_v13 (target_slot, provider, route_id, stage_order, wahoo_route_id)
				SELECT target_slot, 'veloplanner', route_id, stage_order, wahoo_route_id FROM trusted_inventory_stages`,
			`DROP TABLE trusted_inventory_stages`,
			`ALTER TABLE trusted_inventory_stages_v13 RENAME TO trusted_inventory_stages`,
			`CREATE TABLE stage_geometry_v13 (
				provider        TEXT    NOT NULL DEFAULT 'veloplanner',
				route_id        INTEGER NOT NULL,
				stage_order     INTEGER NOT NULL,
				content_hash    TEXT    NOT NULL,
				route_name      TEXT    NOT NULL,
				stage_name      TEXT    NOT NULL,
				point_count     INTEGER NOT NULL,
				distance_metres REAL    NOT NULL,
				ascent_metres   REAL    NOT NULL DEFAULT 0,
				max_gradient_percent REAL NOT NULL DEFAULT 0,
				min_longitude   REAL    NOT NULL,
				min_latitude    REAL    NOT NULL,
				max_longitude   REAL    NOT NULL,
				max_latitude    REAL    NOT NULL,
				coordinates     BLOB    NOT NULL,
				updated_at_unix INTEGER NOT NULL,
				PRIMARY KEY (provider, route_id, stage_order)
			)`,
			`INSERT INTO stage_geometry_v13 (
				provider, route_id, stage_order, content_hash, route_name, stage_name,
				point_count, distance_metres, ascent_metres, max_gradient_percent,
				min_longitude, min_latitude, max_longitude, max_latitude,
				coordinates, updated_at_unix
			)
				SELECT 'veloplanner', route_id, stage_order, content_hash, route_name, stage_name,
					point_count, distance_metres, ascent_metres, max_gradient_percent,
					min_longitude, min_latitude, max_longitude, max_latitude,
					coordinates, updated_at_unix
				FROM stage_geometry`,
			`DROP TABLE stage_geometry`,
			`ALTER TABLE stage_geometry_v13 RENAME TO stage_geometry`,
			`CREATE TABLE stage_surface_v13 (
				provider         TEXT    NOT NULL DEFAULT 'veloplanner',
				route_id         INTEGER NOT NULL,
				stage_order      INTEGER NOT NULL,
				content_hash     TEXT    NOT NULL,
				ranges           BLOB    NOT NULL,
				matched_metres   REAL    NOT NULL,
				updated_at_unix  INTEGER NOT NULL,
				index_generation TEXT    NOT NULL DEFAULT '',
				PRIMARY KEY (provider, route_id, stage_order)
			)`,
			`INSERT INTO stage_surface_v13 (
				provider, route_id, stage_order, content_hash, ranges, matched_metres, updated_at_unix, index_generation
			)
				SELECT 'veloplanner', route_id, stage_order, content_hash, ranges, matched_metres, updated_at_unix, index_generation
				FROM stage_surface`,
			`DROP TABLE stage_surface`,
			`ALTER TABLE stage_surface_v13 RENAME TO stage_surface`,
			`CREATE TABLE stage_reprocess_v13 (
				provider          TEXT    NOT NULL DEFAULT 'veloplanner',
				route_id          INTEGER NOT NULL,
				stage_order       INTEGER NOT NULL,
				requested_at_unix INTEGER NOT NULL,
				PRIMARY KEY (provider, route_id, stage_order)
			)`,
			`INSERT INTO stage_reprocess_v13 (provider, route_id, stage_order, requested_at_unix)
				SELECT 'veloplanner', route_id, stage_order, requested_at_unix FROM stage_reprocess`,
			`DROP TABLE stage_reprocess`,
			`ALTER TABLE stage_reprocess_v13 RENAME TO stage_reprocess`,
		},
		{
			// Which run a success digest last covered. Bounded by run id rather than by
			// the timestamp beside it: run times are stored to the second, so a run
			// finishing in the same second would be counted twice or never.
			`ALTER TABLE notification_state ADD COLUMN last_run_id INTEGER NOT NULL DEFAULT 0`,
		},
		{
			// Predicted moving time, in the manner of stage_surface: its own table, filled
			// by a later pass reading that classification. coefficient_fingerprint
			// invalidates a re-fit against the same geometry, and content_hash records the
			// geometry every read matches on.
			`CREATE TABLE stage_duration (
				provider                TEXT    NOT NULL,
				route_id                INTEGER NOT NULL,
				stage_order             INTEGER NOT NULL,
				content_hash            TEXT    NOT NULL,
				surface_generation      TEXT    NOT NULL,
				coefficient_fingerprint TEXT    NOT NULL,
				moving_seconds          REAL,
				cumulative_seconds      BLOB,
				updated_at_unix         INTEGER NOT NULL,
				PRIMARY KEY (provider, route_id, stage_order)
			)`,
		},
		{
			// Surface ranges are stored JSON the geometry endpoint passes through
			// verbatim, so the camelCase rename migrates the cached bytes once.
			`UPDATE stage_surface
				SET ranges = REPLACE(REPLACE(ranges, '"start_index"', '"startIndex"'), '"end_index"', '"endIndex"')`,
		},
		{
			// The settings an operator changes while the service runs: one row, read at
			// startup and on every edit. Seeded with the defaults the configuration file
			// documented. Durations are whole seconds rather than Go strings.
			`CREATE TABLE runtime_settings (
				id                               INTEGER PRIMARY KEY CHECK (id = 1),
				allow_empty_source_deletion      INTEGER NOT NULL CHECK (allow_empty_source_deletion IN (0, 1)),
				stale_after_seconds              INTEGER NOT NULL CHECK (stale_after_seconds > 0),
				notifications_enabled            INTEGER NOT NULL CHECK (notifications_enabled IN (0, 1)),
				success_policy                   TEXT    NOT NULL CHECK (success_policy IN ('every', 'quiet', 'digest')),
				digest_interval_seconds          INTEGER NOT NULL CHECK (digest_interval_seconds > 0),
				pushover_base_url                TEXT    NOT NULL,
				surface_rebuild_interval_seconds INTEGER NOT NULL CHECK (surface_rebuild_interval_seconds > 0),
				updated_at_unix                  INTEGER NOT NULL
			)`,
			`INSERT INTO runtime_settings (
				id, allow_empty_source_deletion, stale_after_seconds,
				notifications_enabled, success_policy, digest_interval_seconds,
				pushover_base_url, surface_rebuild_interval_seconds, updated_at_unix
			) VALUES (1, 0, 86400, 1, 'every', 86400, 'https://api.pushover.net', 604800, 0)`,
			// The cartographies the map may be switched between, ordered. Its own table so
			// a list an operator edits is held a row at a time. Seeded with the keyless
			// default.
			`CREATE TABLE runtime_basemap (
				position         INTEGER PRIMARY KEY,
				name             TEXT    NOT NULL,
				style_url        TEXT    NOT NULL,
				style_url_dark   TEXT    NOT NULL,
				dark_cartography INTEGER NOT NULL CHECK (dark_cartography IN (0, 1))
			)`,
			`INSERT INTO runtime_basemap (position, name, style_url, style_url_dark, dark_cartography)
				VALUES (0, 'Streets', 'https://tiles.openfreemap.org/styles/bright',
					'https://tiles.openfreemap.org/styles/dark', 0)`,
			// The OpenStreetMap extracts to index, ordered as entered. Seeded empty, which
			// is surface classification switched off.
			`CREATE TABLE runtime_surface_region (
				position INTEGER PRIMARY KEY,
				region   TEXT    NOT NULL
			)`,
		},
		{
			// The rest of what the configuration file held: the Wahoo application, the
			// source libraries, the ride model, and the initial delay. Seeded unconfigured,
			// because none of it can be guessed.
			`ALTER TABLE runtime_settings ADD COLUMN wahoo_api_base_url TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE runtime_settings ADD COLUMN wahoo_oauth_base_url TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE runtime_settings ADD COLUMN wahoo_client_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE runtime_settings ADD COLUMN ridemodel_coefficients_file TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE runtime_settings ADD COLUMN sync_initial_delay_seconds INTEGER NOT NULL DEFAULT 60`,
			// The destination slots, ordered as offered. The slot name is the identity
			// stored authorizations, target stages and runs already carry.
			`CREATE TABLE runtime_target (
				position  INTEGER PRIMARY KEY,
				target_id TEXT    NOT NULL
			)`,
			// The libraries a run reads, one row per provider. Ordered because a source
			// phase reads them in turn and reports in that order.
			`CREATE TABLE runtime_source (
				position INTEGER PRIMARY KEY,
				provider TEXT    NOT NULL,
				base_url TEXT    NOT NULL
			)`,
			// The credentials those reach their upstreams with, encrypted under the state
			// key. The secret's name is the associated data, so a moved ciphertext fails
			// to open rather than authenticating as another.
			`CREATE TABLE runtime_secret (
				name            TEXT PRIMARY KEY,
				value           BLOB NOT NULL,
				updated_at_unix INTEGER NOT NULL
			)`,
		},
		{
			// What every background activity came to, refusals included. An attempt
			// that found its work already current is absent, as is one shutdown ended.
			// The detail is a stable category, never provider text.
			`CREATE TABLE task_runs (
				id               INTEGER PRIMARY KEY,
				task             TEXT    NOT NULL,
				argument         TEXT    NOT NULL DEFAULT '',
				started_at_unix  INTEGER NOT NULL,
				finished_at_unix INTEGER NOT NULL,
				outcome          TEXT    NOT NULL,
				detail           TEXT    NOT NULL DEFAULT ''
			)`,
			// Both retention and readback ask for one task's attempts newest first, so
			// that is the order indexed. Picking the latest attempt per argument scans
			// one task's retained rows instead, which its own bound keeps small.
			`CREATE INDEX task_runs_task_index ON task_runs(task, id DESC)`,
		},
		{
			// Which stages an enrichment pass could not finish, and why. One row per
			// stage per pass, replaced when the pass tries again and removed when it
			// succeeds, so what is here is what is wrong now rather than a log.
			`CREATE TABLE stage_enrichment_failure (
				provider       TEXT    NOT NULL,
				route_id       INTEGER NOT NULL,
				stage_order    INTEGER NOT NULL,
				pass           TEXT    NOT NULL,
				reason         TEXT    NOT NULL,
				failed_at_unix INTEGER NOT NULL,
				PRIMARY KEY (provider, route_id, stage_order, pass)
			)`,
		},
		{
			// The name a message about a run can carry. It is random and means
			// nothing on its own, which is what makes it safe to send.
			`ALTER TABLE task_runs ADD COLUMN reference TEXT NOT NULL DEFAULT ''`,
			`UPDATE task_runs SET reference = lower(hex(randomblob(6))) WHERE reference = ''`,
		},
		{
			// Which alerts an operator wants delivered, one row per alert of one
			// task over one scope. A row exists only once somebody has decided:
			// what is absent is what nobody has said anything about, which is not
			// the same as switched off.
			`CREATE TABLE alert_toggle (
				task       TEXT    NOT NULL,
				scope      TEXT    NOT NULL DEFAULT '',
				alert      TEXT    NOT NULL,
				enabled    INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				updated_at_unix INTEGER NOT NULL,
				PRIMARY KEY (task, scope, alert)
			)`,
		},
		{
			// The zone this service reads local time in. Seeded with the one every
			// route it holds is in, which is what the forecast was already asked in.
			`ALTER TABLE runtime_settings ADD COLUMN timezone TEXT NOT NULL DEFAULT 'Europe/Berlin'`,
		},
		{
			// Which tasks the schedule may start. A row exists only once somebody
			// has switched one off or back on; what is absent is a task nobody has
			// said anything about, which runs.
			`CREATE TABLE task_schedule (
				task            TEXT    NOT NULL PRIMARY KEY,
				enabled         INTEGER NOT NULL CHECK (enabled IN (0, 1)),
				updated_at_unix INTEGER NOT NULL
			)`,
			// The two switches it replaces, carried over by name so an operator who
			// had turned a half off does not find it running again after a deploy.
			`INSERT INTO task_schedule (task, enabled, updated_at_unix)
				SELECT 'sync:source', source_enabled, 0 FROM sync_schedule WHERE id = 1`,
			`INSERT INTO task_schedule (task, enabled, updated_at_unix)
				SELECT 'sync:target', targets_enabled, 0 FROM sync_schedule WHERE id = 1`,
		},
	}
}
