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
// be and still open. It exists for one case: a deploy that migrated the state
// and then failed its health gate, whose rollback puts the previous binary back
// in front of a database it has never seen. Refusing there leaves the host down
// with no way back, which is worse than running one release behind the schema —
// the migration compatibility rule on schemaMigrations is what makes running
// behind safe. Beyond that the refusal stands: a binary many releases old
// against a much newer schema is a mistake to report, not to absorb. It also
// means a release that appends more than one migration cannot be rolled back
// through, so a release that must stay rollable appends one.
const forwardCompatibleMigrations = 1

// schemaAheadMessage is the stable prefix of the refusal above. The deploy
// script matches on it to tell a rollback that cannot read its state from a
// rollback that is unhealthy for any other reason, so it is a contract with
// deploy/domestique-deploy.sh rather than free-form error text.
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

// schemaMigrations is the ordered, append-only history of this database.
//
// A migration is identified by its position and by nothing else: the applied
// version is a count, so element N is "migration N" forever. A new migration is
// therefore appended and never inserted — inserting one renumbers every element
// after it, and a deployment that has already applied the old numbering then
// re-runs somebody else's migration under the new one. That fails at startup on
// exactly the databases that carry the operator's data, and passes every test
// that only ever migrates an empty file.
//
// A migration must also leave the previous release's binary working, because a
// deploy that migrates the state and then fails its health gate is rolled back
// onto that binary — see forwardCompatibleMigrations. Additive with defaults is
// the allowed shape. A migration must not:
//
//   - drop or rename a table, column, or index an earlier binary reads or
//     writes;
//   - add a NOT NULL column without a default to a table an earlier binary
//     inserts into;
//   - tighten a CHECK constraint or add a UNIQUE index an earlier binary's
//     writes could violate; or
//   - change what the values of an existing column mean.
//
// TestNewMigrationsStayReadableByThePreviousRelease enforces the structural half
// of this; the last item is on the author.
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
			// The route map view's rendering cache. It is deliberately separate from
			// source_stages: that table backs the deletion-safety guard and is
			// replaced wholesale every sync, whereas this one is written only when a
			// stage's content hash changes and may be dropped at any time without
			// affecting sync safety.
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
			// Existing rows default to zero and are refilled by the next sync
			// that changes their content hash; the cache is presentation only,
			// so a stale zero degrades a card rather than anything else.
			`ALTER TABLE stage_geometry ADD COLUMN ascent_metres REAL NOT NULL DEFAULT 0`,
			`ALTER TABLE stage_geometry ADD COLUMN max_gradient_percent REAL NOT NULL DEFAULT 0`,
			// Geometry is only rewritten when a stage's content hash changes, so
			// rows cached before this migration would keep zeroed statistics
			// indefinitely. Clearing the hash makes the next run refill them once.
			// The rows stay readable meanwhile, showing a route without its
			// climbing figures rather than disappearing.
			`UPDATE stage_geometry SET content_hash = ''`,
		},
		{
			// The surface classification of each stage, derived from OpenStreetMap.
			// It is a third table rather than more columns on stage_geometry
			// because it is filled by a later pass that talks to a remote service:
			// the geometry cache is written inside the inventory transaction and
			// must not wait on the network, and a stage whose surface could not be
			// fetched has to be able to sit here missing while everything else
			// about it is current.
			//
			// content_hash records the geometry the ranges were measured against.
			// The ranges are positions in that geometry's coordinate array, so they
			// mean nothing beside a different revision of the stage, and every read
			// matches on the hash rather than trusting the row to be current.
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
			// Which half of a synchronization the timer is allowed to start.
			// Reading the source and writing to the targets are separately
			// switchable, so an operator can stop touching devices while still
			// refreshing the library, or the reverse, without editing
			// configuration on the host and restarting the service.
			//
			// One row, defaulted to the behaviour every existing deployment
			// already has: both halves on.
			`CREATE TABLE sync_schedule (
				id              INTEGER PRIMARY KEY CHECK (id = 1),
				source_enabled  INTEGER NOT NULL CHECK (source_enabled IN (0, 1)),
				targets_enabled INTEGER NOT NULL CHECK (targets_enabled IN (0, 1)),
				updated_at_unix INTEGER NOT NULL
			)`,
			`INSERT INTO sync_schedule (id, source_enabled, targets_enabled, updated_at_unix)
				VALUES (1, 1, 1, 0)`,
			// Runs are now recorded per phase. Rows written before this migration
			// covered both halves at once; they keep an empty phase rather than
			// claiming to be one of them.
			`ALTER TABLE sync_runs ADD COLUMN phase TEXT NOT NULL DEFAULT ''`,
		},
		{
			// One stage an operator has asked to be redone from scratch.
			//
			// It is its own table rather than a column on source_stages, because
			// storing an inventory replaces every source_stages row: a mark kept
			// there would be deleted by the very run that is meant to honour it.
			//
			// A mark outlives nothing else. It is cleared by the pass that
			// consumes it, so a request that arrives while a run is in flight is
			// honoured by the next one rather than lost.
			`CREATE TABLE stage_reprocess (
				route_id          INTEGER NOT NULL,
				stage_order       INTEGER NOT NULL,
				requested_at_unix INTEGER NOT NULL,
				PRIMARY KEY (route_id, stage_order)
			)`,
		},
		{
			// The last recorded reconciliation of each target, one row per slot.
			//
			// The aggregate run in sync_runs answers "did that run succeed", which
			// is a different question from "is this account current": a run that
			// wrote one slot and failed the other is recorded once as failed, and
			// nothing in it says which slot is behind. Only the last attempt per
			// slot is kept, because that is the whole of what convergence needs
			// and a per-target history nobody reads is state to migrate forever.
			`CREATE TABLE target_runs (
				target_slot      TEXT PRIMARY KEY REFERENCES targets(slot),
				finished_at_unix INTEGER NOT NULL,
				outcome          TEXT    NOT NULL,
				detail           TEXT    NOT NULL
			)`,
		},
		{
			// A name for one recorded run, so a notification about it and the
			// record an operator opens afterwards are legibly the same run.
			//
			// It carries no meaning: the row identifier is a position in this
			// file and the counts are a position in the operator's week, and
			// neither is something to hand to a reader as a name. Random bytes
			// name the run and say nothing else about it.
			`ALTER TABLE sync_runs ADD COLUMN reference TEXT NOT NULL DEFAULT ''`,
			// Existing rows are named here rather than left empty, so history
			// that predates this migration is as addressable as history after
			// it.
			`UPDATE sync_runs SET reference = lower(hex(randomblob(6))) WHERE reference = ''`,
			// Deliberately not unique: an earlier binary rolled back onto this
			// schema still inserts rows with the column's default, and a second
			// such row must not fail its own run.
			`CREATE INDEX sync_runs_reference_index ON sync_runs(reference)`,
		},
		{
			// Which build of the local map a cached classification was measured
			// against.
			//
			// Surfaces now come from an index this service builds from published
			// OpenStreetMap extracts, and a new index is a new set of answers: a
			// road resurfaced since the last build classifies differently from
			// the same geometry read a week ago. The generation is therefore part
			// of what makes a cached classification current, alongside the
			// content hash that already tracked the stage's own shape.
			//
			// Existing rows default to the empty generation, which matches no
			// index and so reclassifies once, on the first pass after the first
			// build. That is the correct outcome: those rows came from Overpass
			// and nothing recorded which day's map they describe.
			`ALTER TABLE stage_surface ADD COLUMN index_generation TEXT NOT NULL DEFAULT ''`,
			// When the index was last built, and what it produced.
			//
			// The scheduler counts its interval from process start, which on a
			// service deployed several times a day would mean a weekly rebuild
			// either runs on every deploy or never runs at all. One row, read at
			// startup, is what turns the interval into time between builds.
			`CREATE TABLE surface_index (
				id            INTEGER PRIMARY KEY CHECK (id = 1),
				built_at_unix INTEGER NOT NULL,
				generation    TEXT    NOT NULL
			)`,
			`INSERT INTO surface_index (id, built_at_unix, generation) VALUES (1, 0, '')`,
		},
		{
			// A stage identity now names which upstream source issued its route
			// ID, because a second provider will one day issue the same numeric
			// route ID as VeloPlanner. Every table that keys a stage by
			// (route_id, stage_order) gets a provider column, defaulted to the
			// only provider that has ever existed, and that column joins the key
			// so two providers' stages can occupy the same table without
			// colliding.
			//
			// SQLite cannot widen a PRIMARY KEY in place, so each table is
			// rebuilt: a new table is created with the wider key, the existing
			// rows are copied into it as 'veloplanner', and the old table is
			// dropped in favour of the rebuilt one.
			//
			// This is where the append-only rollback-compatibility rule the rest
			// of this file follows cannot be kept in full, and it is kept here
			// rather than silently weakened. The service specification records
			// it as an accepted exception licensing this one migration; a later
			// migration carries the additive obligation
			// unchanged. TestNewMigrationsStayReadableByThePreviousRelease
			// only compares column and index shape, so it passes: every carried
			// column keeps its type, nullability, and default, and the new
			// provider column is NOT NULL with a default. What it cannot see is
			// that source_stages and trusted_inventory_stages were never written
			// through an ON CONFLICT clause and so stay genuinely readable by the
			// previous release, while target_stages, stage_geometry,
			// stage_surface, and stage_reprocess were: a previous release's
			// binary still issues `ON CONFLICT (route_id, stage_order)`, and
			// SQLite requires that column list to name an existing unique
			// constraint exactly. Once the primary key on those four tables
			// widens to include provider, that old statement fails to prepare.
			// A deployment rolled back onto this schema would need the previous
			// release's binary to write to those four tables, which it could not
			// do. Reading them still works. This is accepted because the
			// alternative — leaving provider out of those tables' keys — would
			// leave four of the six tables unable to hold two providers' stages
			// at once, defeating the reason this migration exists.
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
			// Which run a success digest last covered.
			//
			// The timestamp beside it says when the digest was sent and is what
			// the interval is measured from, but it cannot bound the window: run
			// times are stored to the second, so a run finishing in the same
			// second as a digest would fall on the wrong side of any comparison
			// with it and be counted twice or, once the window moved on, never.
			// The run id is monotonic and exact, and the rows it names are the
			// same rows the digest totals.
			`ALTER TABLE notification_state ADD COLUMN last_run_id INTEGER NOT NULL DEFAULT 0`,
		},
		{
			// Predicted moving time from internal/ridemodel, in the manner of
			// stage_surface: a fourth table rather than more columns on
			// stage_geometry, because it is filled by a later pass reading the
			// surface classification stage_surface itself holds, and because a
			// re-fit against the same geometry must still invalidate it. That is
			// what coefficient_fingerprint is for; surface_generation plays the
			// same role stage_surface's own index_generation plays there.
			//
			// content_hash records the geometry the prediction was measured
			// against; every read matches on it rather than trusting the row to
			// be current, for the same reason stage_surface does.
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
			// Surface ranges are stored JSON that the geometry endpoint passes
			// through verbatim. The HTTP contract changed its field names to
			// camelCase, so migrate the cached bytes once rather than decoding and
			// re-encoding them on every response.
			`UPDATE stage_surface
				SET ranges = REPLACE(REPLACE(ranges, '"start_index"', '"startIndex"'), '"end_index"', '"endIndex"')`,
		},
		{
			// The settings an operator changes while the service runs, in the
			// manner sync_schedule established: one row, read at startup and on
			// every edit, so a deletion gate flipped for one deliberate run or a
			// notification quieted for a week costs no restart.
			//
			// The values seeded here are the defaults the configuration file
			// documented for the same keys, so a deployment that upgrades and
			// changes nothing else runs on exactly what it ran on before.
			//
			// Durations are whole seconds rather than the Go strings the file
			// carried: the column is then a number the database can check, and
			// the one place a duration has to be parsed from text is the file
			// loader that no longer reads these keys.
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
			// The cartographies the map may be switched between, ordered by the
			// position they are offered in. Its own table rather than a column of
			// packed JSON, because a list an operator edits is a list the database
			// can hold a row at a time.
			//
			// Seeded with the keyless default the file shipped: a deployment that
			// never opens the settings page still gets a map, on a provider that
			// needs no credential.
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
			// The OpenStreetMap extracts to index, ordered the way they were
			// entered. Seeded empty, which is surface classification switched off
			// and is what a deployment that named no region already has: the
			// regions somebody rides are a property of where they live and cannot
			// be guessed.
			`CREATE TABLE runtime_surface_region (
				position INTEGER PRIMARY KEY,
				region   TEXT    NOT NULL
			)`,
		},
		{
			// The rest of what the configuration file used to hold: the Wahoo
			// application, the source libraries, the ride model, and the delay
			// before the first run. Columns are added to the one settings row for
			// the single values and given tables for the lists, the way migration
			// 17 arranged the settings it moved.
			//
			// Everything is seeded unconfigured rather than with a default,
			// because none of it can be guessed: an OAuth application, a target
			// slot name and a library account are all specific to one operator.
			// A service holding these seeds starts, serves the settings page, and
			// runs nothing until they are filled in.
			`ALTER TABLE runtime_settings ADD COLUMN wahoo_api_base_url TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE runtime_settings ADD COLUMN wahoo_oauth_base_url TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE runtime_settings ADD COLUMN wahoo_client_id TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE runtime_settings ADD COLUMN ridemodel_coefficients_file TEXT NOT NULL DEFAULT ''`,
			`ALTER TABLE runtime_settings ADD COLUMN sync_initial_delay_seconds INTEGER NOT NULL DEFAULT 60`,
			// The destination slots, ordered the way they are offered. The slot
			// name is the identity every stored authorization, target stage and
			// recorded run already carries, so this table names the slots that are
			// configured and the targets table keeps owning their state.
			`CREATE TABLE runtime_target (
				position  INTEGER PRIMARY KEY,
				target_id TEXT    NOT NULL
			)`,
			// The libraries a run reads, one row per provider. Ordered because a
			// source phase reads them in turn and reports each result in the order
			// they were configured.
			`CREATE TABLE runtime_source (
				position INTEGER PRIMARY KEY,
				provider TEXT    NOT NULL,
				base_url TEXT    NOT NULL
			)`,
			// The credentials those two reach their upstreams with, encrypted
			// under the state key the same way a Wahoo refresh token is. The
			// secret's own name is the associated data, so a ciphertext moved from
			// one row to another fails to open rather than authenticating as the
			// wrong credential.
			`CREATE TABLE runtime_secret (
				name            TEXT PRIMARY KEY,
				value           BLOB NOT NULL,
				updated_at_unix INTEGER NOT NULL
			)`,
		},
	}
}
