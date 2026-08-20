// Package sqlite persists Domestique's encrypted, local reconciliation state.
package sqlite

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/route"

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

// retainedSyncRuns bounds the recorded run history. Both halves run on the
// configured interval, so an hourly deployment writes about fifty rows a day
// and this window holds a little over a week of them — long enough to answer
// "when did this start going wrong" and short enough that the state file this
// service keeps forever stops growing.
const retainedSyncRuns = 500

// syncRunReferenceBytes is how much randomness names a run. Twelve hex
// characters are readable aloud and leave the retained window nowhere near a
// collision.
const syncRunReferenceBytes = 6

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

// AuthorizationState identifies a target slot's durable OAuth state.
type AuthorizationState string

const (
	// AuthorizationNotAuthorized means the target has not completed OAuth.
	AuthorizationNotAuthorized AuthorizationState = "not_authorized"
	// AuthorizationAuthorized means the target has a usable refresh token.
	AuthorizationAuthorized AuthorizationState = "authorized"
	// AuthorizationNeedsReauthorization means token refresh failed permanently.
	AuthorizationNeedsReauthorization AuthorizationState = "needs_reauthorization"
)

// Target is durable, non-secret state for one configured Wahoo target slot.
type Target struct {
	ID                 string
	WahooUserID        string
	AuthorizationState AuthorizationState
}

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

// EnsureTargets creates durable records for configured target slots. It never
// removes a target, preserving state until an explicit migration is designed.
func (s *Store) EnsureTargets(ctx context.Context, targetIDs []string) error {
	if err := validateTargetIDs(targetIDs); err != nil {
		return err
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting target initialization: %w", err)
	}
	defer rollback(transaction)

	for _, targetID := range targetIDs {
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO targets (slot, authorization_state, updated_at_unix)
			VALUES (?, ?, ?)
			ON CONFLICT(slot) DO NOTHING
		`, targetID, AuthorizationNotAuthorized, time.Now().Unix()); err != nil {
			return fmt.Errorf("creating target slot: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing target initialization: %w", err)
	}

	return nil
}

// Targets returns all target slots without exposing their refresh tokens.
func (s *Store) Targets(ctx context.Context) ([]Target, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT slot, COALESCE(wahoo_user_id, ''), authorization_state
		FROM targets
		ORDER BY slot
	`)
	if err != nil {
		return nil, fmt.Errorf("listing target slots: %w", err)
	}
	defer closeRows(rows)

	var targets []Target
	for rows.Next() {
		var target Target
		if err := rows.Scan(&target.ID, &target.WahooUserID, &target.AuthorizationState); err != nil {
			return nil, fmt.Errorf("reading target slot: %w", err)
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating target slots: %w", err)
	}

	return targets, nil
}

// ForEachTarget visits each configured target without exposing its Wahoo
// identity or refresh token.
func (s *Store) ForEachTarget(ctx context.Context, visit func(id, authorization string) error) error {
	if visit == nil {
		return errors.New("target visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT slot, authorization_state FROM targets ORDER BY slot
	`)
	if err != nil {
		return fmt.Errorf("listing targets: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var id, authorization string
		if err := rows.Scan(&id, &authorization); err != nil {
			return fmt.Errorf("reading target: %w", err)
		}
		if err := visit(id, authorization); err != nil {
			return fmt.Errorf("visiting target: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating targets: %w", err)
	}

	return nil
}

// ForEachSourceStage visits trusted source-stage metadata in stable order.
func (s *Store) ForEachSourceStage(ctx context.Context, visit func(routeID int64, stageOrder int, sourceRevision, contentHash string) error) error {
	if visit == nil {
		return errors.New("source stage visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT route_id, stage_order, source_revision, content_hash
		FROM source_stages ORDER BY route_id, stage_order
	`)
	if err != nil {
		return fmt.Errorf("listing source stages: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var routeID int64
		var stageOrder int
		var sourceRevision, contentHash string
		if err := rows.Scan(&routeID, &stageOrder, &sourceRevision, &contentHash); err != nil {
			return fmt.Errorf("reading source stage: %w", err)
		}
		if err := visit(routeID, stageOrder, sourceRevision, contentHash); err != nil {
			return fmt.Errorf("visiting source stage: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating source stages: %w", err)
	}

	return nil
}

// ForEachStageSummary visits trusted source stages with their display metadata
// in stable order. A stage whose geometry has not yet been cached is still
// visited, with zeroed geometry facts, so the inventory listing never hides a
// synced stage.
func (s *Store) ForEachStageSummary(ctx context.Context, visit func(summary route.Summary) error) error {
	if visit == nil {
		return errors.New("stage summary visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT
			source_stages.route_id,
			source_stages.stage_order,
			source_stages.source_revision,
			source_stages.content_hash,
			COALESCE(stage_geometry.route_name, ''),
			COALESCE(stage_geometry.stage_name, ''),
			COALESCE(stage_geometry.point_count, 0),
			COALESCE(stage_geometry.distance_metres, 0),
			COALESCE(stage_geometry.ascent_metres, 0),
			COALESCE(stage_geometry.max_gradient_percent, 0),
			COALESCE(stage_geometry.min_longitude, 0),
			COALESCE(stage_geometry.min_latitude, 0),
			COALESCE(stage_geometry.max_longitude, 0),
			COALESCE(stage_geometry.max_latitude, 0)
		FROM source_stages
		LEFT JOIN stage_geometry
			ON stage_geometry.route_id = source_stages.route_id
			AND stage_geometry.stage_order = source_stages.stage_order
		ORDER BY source_stages.route_id, source_stages.stage_order
	`)
	if err != nil {
		return fmt.Errorf("listing stage summaries: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var summary route.Summary
		if err := rows.Scan(
			&summary.RouteID, &summary.StageOrder, &summary.SourceRevision, &summary.ContentHash,
			&summary.RouteName, &summary.StageName, &summary.PointCount, &summary.DistanceMetres,
			&summary.AscentMetres, &summary.MaxGradientPercent,
			&summary.Bounds.MinLongitude, &summary.Bounds.MinLatitude,
			&summary.Bounds.MaxLongitude, &summary.Bounds.MaxLatitude,
		); err != nil {
			return fmt.Errorf("reading stage summary: %w", err)
		}
		if err := visit(summary); err != nil {
			return fmt.Errorf("visiting stage summary: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating stage summaries: %w", err)
	}

	return nil
}

// StageGeometry returns one stage's cached geometry with its display metadata.
// The coordinates are a JSON array of [longitude, latitude, elevation?]
// positions, ready to serve as a GeoJSON coordinate list without re-encoding.
func (s *Store) StageGeometry(
	ctx context.Context,
	routeID int64,
	stageOrder int,
) (route.Summary, json.RawMessage, bool, error) {
	var summary route.Summary
	var coordinates []byte
	err := s.database.QueryRowContext(ctx, `
		SELECT
			stage_geometry.route_id,
			stage_geometry.stage_order,
			COALESCE(source_stages.source_revision, ''),
			stage_geometry.content_hash,
			stage_geometry.route_name,
			stage_geometry.stage_name,
			stage_geometry.point_count,
			stage_geometry.distance_metres,
			stage_geometry.ascent_metres,
			stage_geometry.max_gradient_percent,
			stage_geometry.min_longitude,
			stage_geometry.min_latitude,
			stage_geometry.max_longitude,
			stage_geometry.max_latitude,
			stage_geometry.coordinates
		FROM stage_geometry
		LEFT JOIN source_stages
			ON source_stages.route_id = stage_geometry.route_id
			AND source_stages.stage_order = stage_geometry.stage_order
		WHERE stage_geometry.route_id = ? AND stage_geometry.stage_order = ?
	`, routeID, stageOrder).Scan(
		&summary.RouteID, &summary.StageOrder, &summary.SourceRevision, &summary.ContentHash,
		&summary.RouteName, &summary.StageName, &summary.PointCount, &summary.DistanceMetres,
		&summary.AscentMetres, &summary.MaxGradientPercent,
		&summary.Bounds.MinLongitude, &summary.Bounds.MinLatitude,
		&summary.Bounds.MaxLongitude, &summary.Bounds.MaxLatitude,
		&coordinates,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return route.Summary{}, nil, false, nil
	}
	if err != nil {
		return route.Summary{}, nil, false, fmt.Errorf("reading stage geometry: %w", err)
	}

	return summary, json.RawMessage(coordinates), true, nil
}

// StageSurface returns one stage's cached surface classification, but only where
// it was measured against the geometry named by contentHash.
//
// The ranges are positions in that geometry's coordinate array, so serving them
// beside a different revision of the stage would put bands of gravel over
// whatever now happens to sit at those indices. Matching on the hash makes a
// stale row absent rather than wrong: the caller sees a stage whose surface is
// not known yet, which is the truth until the next enrichment pass runs.
//
// The ranges are returned as stored, ready to serve without re-encoding.
func (s *Store) StageSurface(
	ctx context.Context,
	routeID int64,
	stageOrder int,
	contentHash string,
) (ranges json.RawMessage, matchedMetres float64, found bool, err error) {
	var stored []byte
	err = s.database.QueryRowContext(ctx, `
		SELECT ranges, matched_metres
		FROM stage_surface
		WHERE route_id = ? AND stage_order = ? AND content_hash = ?
	`, routeID, stageOrder, contentHash).Scan(&stored, &matchedMetres)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("reading stage surface: %w", err)
	}

	return json.RawMessage(stored), matchedMetres, true, nil
}

// StageSurfaceHash returns the content hash the stored classification was
// measured against, so a caller can tell what still needs fetching without
// reading the ranges themselves.
func (s *Store) StageSurfaceHash(
	ctx context.Context,
	routeID int64,
	stageOrder int,
) (contentHash string, found bool, err error) {
	err = s.database.QueryRowContext(ctx, `
		SELECT content_hash FROM stage_surface WHERE route_id = ? AND stage_order = ?
	`, routeID, stageOrder).Scan(&contentHash)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("reading stage surface hash: %w", err)
	}

	return contentHash, true, nil
}

// StoreStageSurface caches one stage's classification. The ranges are stored as
// given, which is exactly the JSON the geometry endpoint serves.
func (s *Store) StoreStageSurface(
	ctx context.Context,
	routeID int64,
	stageOrder int,
	contentHash string,
	ranges []byte,
	matchedMetres float64,
) error {
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO stage_surface (
			route_id, stage_order, content_hash, ranges, matched_metres, updated_at_unix
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (route_id, stage_order) DO UPDATE SET
			content_hash = excluded.content_hash,
			ranges = excluded.ranges,
			matched_metres = excluded.matched_metres,
			updated_at_unix = excluded.updated_at_unix
	`, routeID, stageOrder, contentHash, ranges, matchedMetres, time.Now().UTC().Unix()); err != nil {
		return fmt.Errorf("storing stage surface: %w", err)
	}

	return nil
}

// pruneStageSurface drops classifications that no longer describe anything, in
// the caller's transaction.
//
// A row goes when its stage has left the inventory, and equally when the stage
// has been re-planned: the cached ranges address the coordinates of the geometry
// they were measured against, and once that geometry is replaced they are not
// stale data to be corrected but positions in an array that no longer exists.
func pruneStageSurface(ctx context.Context, transaction *sql.Tx) error {
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM stage_surface
		WHERE NOT EXISTS (
			SELECT 1 FROM stage_geometry
			WHERE stage_geometry.route_id = stage_surface.route_id
			  AND stage_geometry.stage_order = stage_surface.stage_order
			  AND stage_geometry.content_hash = stage_surface.content_hash
		)
	`); err != nil {
		return fmt.Errorf("pruning stage surface: %w", err)
	}

	return nil
}

// LastSyncRun returns the most recently recorded terminal run, if any.
//
//nolint:gocritic // The primitive callback boundary keeps httpapi independent of SQLite record types.
func (s *Store) LastSyncRun(ctx context.Context) (completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int, found bool, err error) {
	var completedUnix int64
	err = s.database.QueryRowContext(ctx, `
		SELECT finished_at_unix, outcome, COALESCE(detail, ''), source_stages, created, updated, deleted
		FROM sync_runs ORDER BY id DESC LIMIT 1
	`).Scan(&completedUnix, &outcome, &detail, &sourceStages, &created, &updated, &deleted)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, "", "", 0, 0, 0, 0, false, nil
	}
	if err != nil {
		return time.Time{}, "", "", 0, 0, 0, 0, false, fmt.Errorf("reading last sync run: %w", err)
	}

	return time.Unix(completedUnix, 0).UTC(), outcome, detail, sourceStages, created, updated, deleted, true, nil
}

// TrustedInventory rebuilds the stored source inventory as the stages a target
// reconciliation works from.
//
// This is the handover between the two halves of a synchronization: reading the
// source writes it, writing to the targets reads it, and neither has to be
// running for the other to work. The geometry it returns is the export profile
// that was stored, so a course encoded from it is the one the source pass
// derived rather than a second, subtly different derivation of the same stage.
//
// A stage whose geometry is missing or was cached against a different content
// hash fails the whole read. Returning the rest would describe the library as
// smaller than it is, and a smaller library is exactly what reconciliation
// treats as an instruction to delete.
func (s *Store) TrustedInventory(ctx context.Context) ([]route.Stage, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT
			source_stages.route_id,
			source_stages.stage_order,
			source_stages.source_revision,
			source_stages.content_hash,
			stage_geometry.content_hash,
			stage_geometry.route_name,
			stage_geometry.stage_name,
			stage_geometry.coordinates
		FROM source_stages
		LEFT JOIN stage_geometry
			ON stage_geometry.route_id = source_stages.route_id
			AND stage_geometry.stage_order = source_stages.stage_order
		ORDER BY source_stages.route_id, source_stages.stage_order
	`)
	if err != nil {
		return nil, fmt.Errorf("reading the trusted inventory: %w", err)
	}
	defer closeRows(rows)

	stages := make([]route.Stage, 0)
	for rows.Next() {
		var routeID int64
		var stageOrder int
		var revision, contentHash string
		var geometryHash, routeName, stageName sql.NullString
		var coordinates []byte
		if err := rows.Scan(
			&routeID, &stageOrder, &revision, &contentHash,
			&geometryHash, &routeName, &stageName, &coordinates,
		); err != nil {
			return nil, fmt.Errorf("reading a trusted inventory stage: %w", err)
		}
		if !geometryHash.Valid || geometryHash.String != contentHash {
			return nil, fmt.Errorf(
				"trusted inventory stage %d/%d has no geometry for its content hash", routeID, stageOrder,
			)
		}
		points, err := decodeCoordinates(coordinates)
		if err != nil {
			return nil, err
		}
		stage, err := route.NewStage(
			routeID, stageOrder, revision, routeName.String, stageName.String, points, contentHash,
		)
		if err != nil {
			return nil, fmt.Errorf("rebuilding trusted inventory stage %d/%d: %w", routeID, stageOrder, err)
		}
		stages = append(stages, stage)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading the trusted inventory: %w", err)
	}

	return stages, nil
}

// reprocessSentinel is what a target mapping records instead of the revision and
// content hash it last pushed, once a reprocess has been requested for it.
//
// It is deliberately not the empty string. A mapping is only usable to the
// reconciler while every field is present — an empty revision is read as a
// broken row and fails the whole target phase — so forgetting what was pushed
// has to be written down as a value, not as an absence. Nothing produces this
// value by accident: a real revision comes from the source and a real content
// hash is hexadecimal.
const reprocessSentinel = "reprocess-requested"

// RequestStageReprocess asks for one stage to be redone from scratch.
//
// It changes no route data. It removes the three answers the service would
// otherwise reuse for that stage, so the next passes have to work them out
// again: the geometry cache is marked for rewriting even though the source
// content has not changed, the target mappings forget which revision they last
// pushed so every target is written again, and the surface classification is
// dropped so it is asked for afresh.
//
// The Wahoo route identity is deliberately kept, and so is the shape of the
// mapping: what changes is that it no longer claims to have pushed the revision
// it holds, so the next reconciliation takes the update path. A reprocess
// re-writes the route the service already owns; it never deletes one and never
// creates a second.
//
// Reports whether the stage is in the stored inventory. A stage that is not
// cannot be redone, and saying so is better than leaving a mark that nothing
// will ever consume.
func (s *Store) RequestStageReprocess(ctx context.Context, routeID int64, stageOrder int) (bool, error) {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("starting reprocess request: %w", err)
	}
	defer rollback(transaction)

	var exists int
	err = transaction.QueryRowContext(ctx, `
		SELECT 1 FROM source_stages WHERE route_id = ? AND stage_order = ?
	`, routeID, stageOrder).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the stage to reprocess: %w", err)
	}

	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO stage_reprocess (route_id, stage_order, requested_at_unix)
		VALUES (?, ?, ?)
		ON CONFLICT (route_id, stage_order) DO UPDATE SET requested_at_unix = excluded.requested_at_unix
	`, routeID, stageOrder, time.Now().UTC().Unix()); err != nil {
		return false, fmt.Errorf("recording the reprocess request: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		UPDATE target_stages SET source_revision = ?, content_hash = ?
		WHERE route_id = ? AND stage_order = ?
	`, reprocessSentinel, reprocessSentinel, routeID, stageOrder); err != nil {
		return false, fmt.Errorf("forgetting the pushed revision: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM stage_surface WHERE route_id = ? AND stage_order = ?
	`, routeID, stageOrder); err != nil {
		return false, fmt.Errorf("dropping the stage surface: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("committing the reprocess request: %w", err)
	}

	return true, nil
}

// ForEachPhaseRun visits the most recent recorded run of each phase.
//
// The phases run and fail independently, so the single most recent run answers
// only half the question an operator is asking. Runs recorded before phases
// existed carry no phase and are left out rather than attributed to one.
func (s *Store) ForEachPhaseRun(
	ctx context.Context,
	visit func(phase string, completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error,
) error {
	if visit == nil {
		return errors.New("phase run visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT phase, finished_at_unix, outcome, COALESCE(detail, ''),
			source_stages, created, updated, deleted
		FROM sync_runs
		WHERE phase <> '' AND id IN (SELECT MAX(id) FROM sync_runs WHERE phase <> '' GROUP BY phase)
		ORDER BY phase
	`)
	if err != nil {
		return fmt.Errorf("reading the last run of each phase: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var phase, outcome, detail string
		var completedUnix int64
		var sourceStages, created, updated, deleted int
		if err := rows.Scan(
			&phase, &completedUnix, &outcome, &detail, &sourceStages, &created, &updated, &deleted,
		); err != nil {
			return fmt.Errorf("reading a phase run: %w", err)
		}
		if err := visit(
			phase, time.Unix(completedUnix, 0).UTC(), outcome, detail, sourceStages, created, updated, deleted,
		); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading the runs of each phase: %w", err)
	}

	return nil
}

// ForEachSyncRun visits one page of the recorded history, newest first, and
// returns the cursor for the page after it — empty when the history ends here.
// A cursor this store did not issue is reported as unusable rather than as a
// failure: it is the caller's input, not a broken store.
//
// The page starts after the run the caller's cursor names, or at the newest run
// when there is none. A cursor is a position rather than a name: the run it was
// taken from may have been pruned by the time the next page is asked for, and a
// position still resolves where a name would have to fail.
func (s *Store) ForEachSyncRun(
	ctx context.Context,
	after string,
	limit int,
	visit func(reference, phase string, completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error,
) (next string, usable bool, err error) {
	if visit == nil {
		return "", false, errors.New("sync run visitor is required")
	}
	if limit <= 0 {
		return "", false, errors.New("a positive page size is required")
	}
	position := int64(math.MaxInt64)
	if after != "" {
		cursor, parseErr := strconv.ParseInt(after, 10, 64)
		if parseErr != nil {
			return "", false, nil
		}
		issued, readErr := s.lastSyncRunID(ctx)
		if readErr != nil {
			return "", false, readErr
		}
		// Positions are handed out from one upwards and the highest only grows,
		// because pruning never drops the newest run. A cursor outside that
		// range is one this store never issued; inside it, a pruned row is
		// still a position the runs before it can be read from.
		if cursor <= 0 || cursor > issued {
			return "", false, nil
		}
		position = cursor
	}
	// One row past the page, so "is there more" is read rather than guessed
	// from a page that happened to come back full.
	rows, err := s.database.QueryContext(ctx, `
		SELECT id, reference, phase, finished_at_unix, outcome, COALESCE(detail, ''),
			source_stages, created, updated, deleted
		FROM sync_runs
		WHERE id < ?
		ORDER BY id DESC
		LIMIT ?
	`, position, limit+1)
	if err != nil {
		return "", false, fmt.Errorf("reading sync runs: %w", err)
	}
	defer closeRows(rows)

	visited := 0
	for rows.Next() {
		var id, completedUnix int64
		var reference, phase, outcome, detail string
		var sourceStages, created, updated, deleted int
		if err := rows.Scan(
			&id, &reference, &phase, &completedUnix, &outcome, &detail,
			&sourceStages, &created, &updated, &deleted,
		); err != nil {
			return "", false, fmt.Errorf("reading a sync run: %w", err)
		}
		if visited == limit {
			return next, true, nil
		}
		visited++
		next = strconv.FormatInt(id, 10)
		if err := visit(
			reference, phase, time.Unix(completedUnix, 0).UTC(), outcome, detail,
			sourceStages, created, updated, deleted,
		); err != nil {
			return "", false, err
		}
	}
	if err := rows.Err(); err != nil {
		return "", false, fmt.Errorf("reading sync runs: %w", err)
	}

	// The page was not filled, so nothing follows it.
	return "", true, nil
}

// lastSyncRunID reports the highest position the store has issued to a recorded
// run, or zero when it has recorded none.
func (s *Store) lastSyncRunID(ctx context.Context) (int64, error) {
	var highest int64
	if err := s.database.QueryRowContext(
		ctx, `SELECT COALESCE(MAX(id), 0) FROM sync_runs`,
	).Scan(&highest); err != nil {
		return 0, fmt.Errorf("reading sync runs: %w", err)
	}

	return highest, nil
}

// SurfaceCoverage reports how many stored stages carry a classification of the
// geometry they currently hold, and how many stages there are.
//
// It is the answer to the question an operator actually asks when a route has no
// surface on the map: is this one stage waiting its turn, or has nothing been
// classified in a week. Counting the classification against the current content
// hash is what makes it honest — a stage whose shape changed has a stored
// classification that describes a line it no longer has, and is not classified
// in any sense the map can use.
func (s *Store) SurfaceCoverage(ctx context.Context) (classified, total int, err error) {
	if err := s.database.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM source_stages),
			(SELECT COUNT(*)
				FROM stage_surface
				JOIN source_stages
					ON source_stages.route_id = stage_surface.route_id
					AND source_stages.stage_order = stage_surface.stage_order
				WHERE stage_surface.content_hash = source_stages.content_hash)
	`).Scan(&total, &classified); err != nil {
		return 0, 0, fmt.Errorf("reading surface coverage: %w", err)
	}

	return classified, total, nil
}

// SyncSchedule reports which phases the timer is allowed to start.
func (s *Store) SyncSchedule(ctx context.Context) (source, targets bool, err error) {
	if err := s.database.QueryRowContext(ctx, `
		SELECT source_enabled, targets_enabled FROM sync_schedule WHERE id = 1
	`).Scan(&source, &targets); err != nil {
		return false, false, fmt.Errorf("reading the sync schedule: %w", err)
	}

	return source, targets, nil
}

// SetSyncSchedule records which phases the timer may start from now on.
//
// It never starts or stops a run in flight: a phase already running finishes,
// and the switch decides what the next tick does.
func (s *Store) SetSyncSchedule(ctx context.Context, source, targets bool) error {
	if _, err := s.database.ExecContext(ctx, `
		UPDATE sync_schedule
		SET source_enabled = ?, targets_enabled = ?, updated_at_unix = ?
		WHERE id = 1
	`, source, targets, time.Now().Unix()); err != nil {
		return fmt.Errorf("storing the sync schedule: %w", err)
	}

	return nil
}

// Target returns one target slot without exposing its refresh token.
func (s *Store) Target(ctx context.Context, targetID string) (Target, error) {
	var target Target
	err := s.database.QueryRowContext(ctx, `
		SELECT slot, COALESCE(wahoo_user_id, ''), authorization_state
		FROM targets
		WHERE slot = ?
	`, targetID).Scan(&target.ID, &target.WahooUserID, &target.AuthorizationState)
	if errors.Is(err, sql.ErrNoRows) {
		return Target{}, ErrTargetNotFound
	}
	if err != nil {
		return Target{}, fmt.Errorf("reading target slot: %w", err)
	}

	return target, nil
}

// TargetAuthorization returns the durable authorization state for one target
// without exposing its Wahoo identity or refresh token.
func (s *Store) TargetAuthorization(ctx context.Context, targetID string) (string, error) {
	target, err := s.Target(ctx, targetID)
	if err != nil {
		return "", err
	}

	return string(target.AuthorizationState), nil
}

// AuthorizeTarget atomically binds a Wahoo user and encrypted refresh token to
// a configured target. One Wahoo user cannot authorize more than one slot.
func (s *Store) AuthorizeTarget(ctx context.Context, targetID, wahooUserID, refreshToken string) error {
	if strings.TrimSpace(targetID) == "" || strings.TrimSpace(wahooUserID) == "" || refreshToken == "" {
		return errors.New("target ID, Wahoo user ID, and refresh token are required")
	}

	encryptedToken, err := s.encrypt(targetID, []byte(refreshToken))
	if err != nil {
		return fmt.Errorf("encrypting refresh token: %w", err)
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting target authorization: %w", err)
	}
	defer rollback(transaction)

	var existingTargetID string
	err = transaction.QueryRowContext(ctx, `
		SELECT slot
		FROM targets
		WHERE wahoo_user_id = ? AND slot != ?
	`, wahooUserID, targetID).Scan(&existingTargetID)
	if err == nil {
		return ErrWahooUserAlreadyAuthorized
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("checking existing target authorization: %w", err)
	}

	result, err := transaction.ExecContext(ctx, `
		UPDATE targets
		SET wahoo_user_id = ?, refresh_token = ?, authorization_state = ?, updated_at_unix = ?
		WHERE slot = ?
	`, wahooUserID, encryptedToken, AuthorizationAuthorized, time.Now().Unix(), targetID)
	if err != nil {
		return fmt.Errorf("storing target authorization: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking target authorization: %w", err)
	}
	if updated == 0 {
		return ErrTargetNotFound
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing target authorization: %w", err)
	}

	return nil
}

// RefreshToken returns the decrypted refresh token for a configured target.
func (s *Store) RefreshToken(ctx context.Context, targetID string) (string, error) {
	var encryptedToken []byte
	err := s.database.QueryRowContext(ctx, `
		SELECT refresh_token
		FROM targets
		WHERE slot = ?
	`, targetID).Scan(&encryptedToken)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrTargetNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading refresh token: %w", err)
	}
	if len(encryptedToken) == 0 {
		return "", ErrRefreshTokenUnavailable
	}

	decryptedToken, err := s.decrypt(targetID, encryptedToken)
	if err != nil {
		return "", err
	}

	return string(decryptedToken), nil
}

// ReplaceRefreshToken atomically stores the refresh token returned by a
// successful Wahoo refresh. The replacement happens before another API request
// can use the prior token.
func (s *Store) ReplaceRefreshToken(ctx context.Context, targetID, refreshToken string) error {
	if strings.TrimSpace(targetID) == "" || refreshToken == "" {
		return errors.New("target ID and refresh token are required")
	}

	encryptedToken, err := s.encrypt(targetID, []byte(refreshToken))
	if err != nil {
		return fmt.Errorf("encrypting refresh token: %w", err)
	}

	result, err := s.database.ExecContext(ctx, `
		UPDATE targets
		SET refresh_token = ?, authorization_state = ?, updated_at_unix = ?
		WHERE slot = ?
	`, encryptedToken, AuthorizationAuthorized, time.Now().Unix(), targetID)
	if err != nil {
		return fmt.Errorf("replacing refresh token: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking refreshed target: %w", err)
	}
	if updated == 0 {
		return ErrTargetNotFound
	}

	return nil
}

// TrustedInventoryCount returns the number of stages in the last fully
// validated source inventory. Zero means there is no prior trusted stage.
func (s *Store) TrustedInventoryCount(ctx context.Context) (int, error) {
	var count int
	if err := s.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM source_stages").Scan(&count); err != nil {
		return 0, fmt.Errorf("counting trusted source inventory: %w", err)
	}

	return count, nil
}

// StoreTrustedInventory atomically replaces the last fully validated source
// inventory. It stores metadata only, never geometry or FIT bytes.
func (s *Store) StoreTrustedInventory(ctx context.Context, stages []route.Stage) error {
	seen := make(map[route.Key]struct{}, len(stages))
	for _, stage := range stages {
		key := stage.Key()
		if _, exists := seen[key]; exists {
			return errors.New("trusted source inventory contains a duplicate stage")
		}
		seen[key] = struct{}{}
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting trusted inventory update: %w", err)
	}
	defer rollback(transaction)

	if _, err := transaction.ExecContext(ctx, "DELETE FROM source_stages"); err != nil {
		return fmt.Errorf("clearing trusted source inventory: %w", err)
	}
	for _, stage := range stages {
		key := stage.Key()
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO source_stages (route_id, stage_order, source_revision, content_hash)
			VALUES (?, ?, ?, ?)
		`, key.RouteID(), key.StageOrder(), stage.Revision(), stage.ContentHash()); err != nil {
			return fmt.Errorf("storing trusted source stage: %w", err)
		}
	}
	if err := storeStageGeometry(ctx, transaction, stages); err != nil {
		return err
	}
	if err := pruneStageSurface(ctx, transaction); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing trusted inventory update: %w", err)
	}

	return nil
}

// storeStageGeometry refreshes the map-view rendering cache inside the caller's
// transaction. A stage whose content hash is unchanged is left untouched, so an
// unchanged library does not rewrite the whole cache on every scheduled run.
// Rows whose stage has left the inventory are pruned.
func storeStageGeometry(ctx context.Context, transaction *sql.Tx, stages []route.Stage) error {
	updatedAt := time.Now().UTC().Unix()
	requested, err := requestedReprocessing(ctx, transaction)
	if err != nil {
		return err
	}
	for index := range stages {
		stage := &stages[index]
		key := stage.Key()
		_, reprocess := requested[stageIdentity{routeID: key.RouteID(), stageOrder: key.StageOrder()}]

		var storedHash string
		err := transaction.QueryRowContext(ctx, `
			SELECT content_hash FROM stage_geometry WHERE route_id = ? AND stage_order = ?
		`, key.RouteID(), key.StageOrder()).Scan(&storedHash)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return fmt.Errorf("reading cached stage geometry: %w", err)
		case storedHash == stage.ContentHash() && !reprocess:
			continue
		}

		geometry := stage.Geometry()
		coordinates, err := encodeCoordinates(geometry)
		if err != nil {
			return err
		}
		bounds := stage.Bounds()
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO stage_geometry (
				route_id, stage_order, content_hash, route_name, stage_name,
				point_count, distance_metres, ascent_metres, max_gradient_percent,
				min_longitude, min_latitude, max_longitude, max_latitude,
				coordinates, updated_at_unix
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (route_id, stage_order) DO UPDATE SET
				content_hash = excluded.content_hash,
				route_name = excluded.route_name,
				stage_name = excluded.stage_name,
				point_count = excluded.point_count,
				distance_metres = excluded.distance_metres,
				ascent_metres = excluded.ascent_metres,
				max_gradient_percent = excluded.max_gradient_percent,
				min_longitude = excluded.min_longitude,
				min_latitude = excluded.min_latitude,
				max_longitude = excluded.max_longitude,
				max_latitude = excluded.max_latitude,
				coordinates = excluded.coordinates,
				updated_at_unix = excluded.updated_at_unix
		`,
			key.RouteID(), key.StageOrder(), stage.ContentHash(), stage.RouteName(), stage.StageName(),
			len(geometry), stage.DistanceMetres(), stage.ElevationGainMetres(), stage.MaxGradientPercent(),
			bounds.MinLongitude, bounds.MinLatitude, bounds.MaxLongitude, bounds.MaxLatitude,
			coordinates, updatedAt,
		); err != nil {
			return fmt.Errorf("storing stage geometry: %w", err)
		}
	}

	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM stage_geometry
		WHERE NOT EXISTS (
			SELECT 1 FROM source_stages
			WHERE source_stages.route_id = stage_geometry.route_id
			  AND source_stages.stage_order = stage_geometry.stage_order
		)
	`); err != nil {
		return fmt.Errorf("pruning stage geometry: %w", err)
	}
	// The marks are consumed here, in the transaction that acted on them, so a
	// request is honoured exactly once and never outlives the pass that met it.
	if _, err := transaction.ExecContext(ctx, "DELETE FROM stage_reprocess"); err != nil {
		return fmt.Errorf("clearing reprocess requests: %w", err)
	}

	return nil
}

// stageIdentity is one stage, for lookups that carry nothing else about it.
type stageIdentity struct {
	routeID    int64
	stageOrder int
}

// requestedReprocessing reads the stages an operator has asked to have redone.
func requestedReprocessing(ctx context.Context, transaction *sql.Tx) (map[stageIdentity]struct{}, error) {
	rows, err := transaction.QueryContext(ctx, "SELECT route_id, stage_order FROM stage_reprocess")
	if err != nil {
		return nil, fmt.Errorf("reading reprocess requests: %w", err)
	}
	defer closeRows(rows)

	requested := make(map[stageIdentity]struct{})
	for rows.Next() {
		var identity stageIdentity
		if err := rows.Scan(&identity.routeID, &identity.stageOrder); err != nil {
			return nil, fmt.Errorf("reading a reprocess request: %w", err)
		}
		requested[identity] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading reprocess requests: %w", err)
	}

	return requested, nil
}

// decodeCoordinates reads back what encodeCoordinates wrote. A position carries
// an elevation or it does not, and the difference is preserved: a stage missing
// elevation must stay missing rather than gain a confident zero.
func decodeCoordinates(encoded []byte) ([]route.Point, error) {
	var positions [][]float64
	if err := json.Unmarshal(encoded, &positions); err != nil {
		return nil, fmt.Errorf("decoding stage coordinates: %w", err)
	}
	points := make([]route.Point, 0, len(positions))
	for _, position := range positions {
		if len(position) < 2 {
			return nil, errors.New("decoding stage coordinates: a position needs a longitude and a latitude")
		}
		point := route.Point{Longitude: position[0], Latitude: position[1]}
		if len(position) > 2 {
			elevation := position[2]
			point.Elevation = &elevation
		}
		points = append(points, point)
	}

	return points, nil
}

// encodeCoordinates renders geometry as a JSON position array. The stored bytes
// are exactly a GeoJSON LineString's coordinate list, so serving them needs no
// decode and re-encode. Elevation is emitted only where the source supplied it.
func encodeCoordinates(points []route.Point) ([]byte, error) {
	positions := make([][]float64, 0, len(points))
	for _, point := range points {
		if point.Elevation == nil {
			positions = append(positions, []float64{point.Longitude, point.Latitude})

			continue
		}
		positions = append(positions, []float64{point.Longitude, point.Latitude, *point.Elevation})
	}
	encoded, err := json.Marshal(positions)
	if err != nil {
		return nil, fmt.Errorf("encoding stage coordinates: %w", err)
	}

	return encoded, nil
}

// ForEachTargetStage visits one target's tracked Wahoo routes in stable source
// order. The visitor receives metadata only and must not retain secrets.
func (s *Store) ForEachTargetStage(
	ctx context.Context,
	targetID string,
	visit func(routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error,
) error {
	if strings.TrimSpace(targetID) == "" || visit == nil {
		return errors.New("target ID and target stage visitor are required")
	}

	rows, err := s.database.QueryContext(ctx, `
		SELECT route_id, stage_order, source_revision, content_hash, wahoo_route_id
		FROM target_stages
		WHERE target_slot = ?
		ORDER BY route_id, stage_order
	`, targetID)
	if err != nil {
		return fmt.Errorf("listing target stages: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var (
			routeID        int64
			stageOrder     int
			sourceRevision string
			contentHash    string
			wahooRouteID   int64
		)
		if err := rows.Scan(&routeID, &stageOrder, &sourceRevision, &contentHash, &wahooRouteID); err != nil {
			return fmt.Errorf("reading target stage: %w", err)
		}
		if err := visit(routeID, stageOrder, sourceRevision, contentHash, wahooRouteID); err != nil {
			return fmt.Errorf("visiting target stage: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating target stages: %w", err)
	}

	return nil
}

// UpsertTargetStage records the successfully applied Wahoo route for one
// source stage. It must be called only after the corresponding remote operation
// has succeeded.
func (s *Store) UpsertTargetStage(
	ctx context.Context,
	targetID string,
	routeID int64,
	stageOrder int,
	sourceRevision, contentHash string,
	wahooRouteID int64,
) error {
	if strings.TrimSpace(targetID) == "" || routeID <= 0 || stageOrder <= 0 ||
		sourceRevision == "" || contentHash == "" || wahooRouteID <= 0 {
		return errors.New("complete target stage metadata is required")
	}

	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO target_stages (
			target_slot, route_id, stage_order, wahoo_route_id, content_hash, source_revision
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(target_slot, route_id, stage_order) DO UPDATE SET
			wahoo_route_id = excluded.wahoo_route_id,
			content_hash = excluded.content_hash,
			source_revision = excluded.source_revision
	`, targetID, routeID, stageOrder, wahooRouteID, contentHash, sourceRevision); err != nil {
		return fmt.Errorf("storing target stage: %w", err)
	}

	return nil
}

// DeleteTargetStage removes the durable mapping after the owned remote Wahoo
// route was deleted successfully.
func (s *Store) DeleteTargetStage(ctx context.Context, targetID string, routeID int64, stageOrder int) error {
	if strings.TrimSpace(targetID) == "" || routeID <= 0 || stageOrder <= 0 {
		return errors.New("target ID and source stage key are required")
	}

	result, err := s.database.ExecContext(ctx, `
		DELETE FROM target_stages
		WHERE target_slot = ? AND route_id = ? AND stage_order = ?
	`, targetID, routeID, stageOrder)
	if err != nil {
		return fmt.Errorf("deleting target stage: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking target stage deletion: %w", err)
	}
	if deleted == 0 {
		return errors.New("target stage was not found")
	}

	return nil
}

// RecordSyncRun stores one terminal synchronization result and returns the
// reference naming it. Its detail is a stable failure category, never provider
// text or a route name.
//
// Recording a run also prunes the history back to its bound, so the file this
// service keeps forever holds a fixed number of runs rather than one per hour
// for as long as it is deployed.
func (s *Store) RecordSyncRun(
	ctx context.Context,
	phase string,
	startedAt, finishedAt time.Time,
	outcome, detail string,
	sourceStages, created, updated, deleted int,
) (string, error) {
	if phase == "" || startedAt.IsZero() || finishedAt.IsZero() || finishedAt.Before(startedAt) || outcome == "" ||
		sourceStages < 0 || created < 0 || updated < 0 || deleted < 0 {
		return "", errors.New("complete non-negative sync run metadata is required")
	}
	reference, err := newSyncRunReference()
	if err != nil {
		return "", err
	}
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("recording sync run: %w", err)
	}
	defer rollback(transaction)

	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO sync_runs (
			reference, phase, started_at_unix, finished_at_unix, outcome, detail,
			source_stages, created, updated, deleted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, reference, phase, startedAt.Unix(), finishedAt.Unix(), outcome, detail,
		sourceStages, created, updated, deleted); err != nil {
		return "", fmt.Errorf("recording sync run: %w", err)
	}
	if err := pruneSyncRuns(ctx, transaction); err != nil {
		return "", err
	}
	if err := transaction.Commit(); err != nil {
		return "", fmt.Errorf("committing sync run: %w", err)
	}

	return reference, nil
}

// newSyncRunReference mints the name one run is known by. Six random bytes are
// short enough to read back off a phone and far enough apart that the retained
// history will not hold two the same.
func newSyncRunReference() (string, error) {
	reference := make([]byte, syncRunReferenceBytes)
	if _, err := io.ReadFull(rand.Reader, reference); err != nil {
		return "", fmt.Errorf("naming sync run: %w", err)
	}

	return hex.EncodeToString(reference), nil
}

// pruneSyncRuns drops everything past the retained window, in the caller's
// transaction.
//
// The most recent run of each phase is kept whatever its age. It is not history
// there: the status response reads it as what that half last came to, and a
// half switched off for a week must not lose its last answer because the other
// half filled the window.
func pruneSyncRuns(ctx context.Context, transaction *sql.Tx) error {
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM sync_runs
		WHERE id NOT IN (SELECT id FROM sync_runs ORDER BY id DESC LIMIT ?)
		  AND id NOT IN (SELECT MAX(id) FROM sync_runs GROUP BY phase)
	`, retainedSyncRuns); err != nil {
		return fmt.Errorf("pruning sync runs: %w", err)
	}

	return nil
}

// RecordTargetRun stores the terminal result of one target's reconciliation,
// replacing whatever that slot recorded before. Its detail is a stable failure
// category, never provider text, a route name, or a Wahoo identifier.
func (s *Store) RecordTargetRun(
	ctx context.Context,
	targetID string,
	finishedAt time.Time,
	outcome, detail string,
) error {
	if strings.TrimSpace(targetID) == "" || finishedAt.IsZero() || outcome == "" {
		return errors.New("target ID, finish time, and outcome are required")
	}
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO target_runs (target_slot, finished_at_unix, outcome, detail)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(target_slot) DO UPDATE SET
			finished_at_unix = excluded.finished_at_unix,
			outcome = excluded.outcome,
			detail = excluded.detail
	`, targetID, finishedAt.Unix(), outcome, detail); err != nil {
		return fmt.Errorf("recording target run: %w", err)
	}

	return nil
}

// ForEachTargetRun visits the last recorded reconciliation of each target in
// stable slot order. A slot that has never been reconciled is not visited, which
// is how a reader tells "never attempted" from "attempted and failed".
func (s *Store) ForEachTargetRun(
	ctx context.Context,
	visit func(targetID string, finishedAt time.Time, outcome, detail string) error,
) error {
	if visit == nil {
		return errors.New("target run visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT target_slot, finished_at_unix, outcome, detail
		FROM target_runs ORDER BY target_slot
	`)
	if err != nil {
		return fmt.Errorf("listing target runs: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var targetID, outcome, detail string
		var finishedUnix int64
		if err := rows.Scan(&targetID, &finishedUnix, &outcome, &detail); err != nil {
			return fmt.Errorf("reading a target run: %w", err)
		}
		if err := visit(targetID, time.Unix(finishedUnix, 0).UTC(), outcome, detail); err != nil {
			return fmt.Errorf("visiting a target run: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating target runs: %w", err)
	}

	return nil
}

// LastFailureNotification returns the previous delivery time for one safe
// failure category. The caller decides whether the configured suppression
// interval has elapsed.
func (s *Store) LastFailureNotification(ctx context.Context, category string) (time.Time, bool, error) {
	if category == "" {
		return time.Time{}, false, errors.New("failure category is required")
	}
	var sentAt int64
	err := s.database.QueryRowContext(ctx, `
		SELECT last_sent_at_unix FROM notification_state WHERE kind = ?
	`, "failure:"+category).Scan(&sentAt)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, fmt.Errorf("reading failure notification state: %w", err)
	}

	return time.Unix(sentAt, 0).UTC(), true, nil
}

// RecordFailureNotification records a delivered notification after Pushover
// accepted it, so failed delivery attempts are retried on the next run.
func (s *Store) RecordFailureNotification(ctx context.Context, category string, sentAt time.Time) error {
	if category == "" || sentAt.IsZero() {
		return errors.New("failure category and notification time are required")
	}
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO notification_state (kind, last_sent_at_unix) VALUES (?, ?)
		ON CONFLICT(kind) DO UPDATE SET last_sent_at_unix = excluded.last_sent_at_unix
	`, "failure:"+category, sentAt.Unix()); err != nil {
		return fmt.Errorf("recording failure notification: %w", err)
	}

	return nil
}

// MarkNeedsReauthorization clears a target's refresh token after a permanent
// OAuth failure and leaves it ready for a fresh interactive authorization.
func (s *Store) MarkNeedsReauthorization(ctx context.Context, targetID string) error {
	result, err := s.database.ExecContext(ctx, `
		UPDATE targets
		SET refresh_token = NULL, authorization_state = ?, updated_at_unix = ?
		WHERE slot = ?
	`, AuthorizationNeedsReauthorization, time.Now().Unix(), targetID)
	if err != nil {
		return fmt.Errorf("marking target for reauthorization: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking reauthorization update: %w", err)
	}
	if updated == 0 {
		return ErrTargetNotFound
	}

	return nil
}

// BeginAuthorization saves a hashed, expiring OAuth state bound to one target
// slot and one caller identity. The raw state value is never persisted.
func (s *Store) BeginAuthorization(
	ctx context.Context,
	targetID, callerLogin string,
	stateDigest []byte,
	expiresAt time.Time,
) error {
	if strings.TrimSpace(targetID) == "" || strings.TrimSpace(callerLogin) == "" ||
		len(stateDigest) != 32 || !expiresAt.After(time.Now()) {
		return errors.New("target ID, caller identity, state digest, and future expiry are required")
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("starting oauth transaction: %w", err)
	}
	defer rollback(transaction)

	var targetExists bool
	if err := transaction.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM targets WHERE slot = ?)`, targetID).Scan(&targetExists); err != nil {
		return fmt.Errorf("checking oauth target: %w", err)
	}
	if !targetExists {
		return ErrTargetNotFound
	}

	now := time.Now().Unix()
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM oauth_transactions
		WHERE expires_at_unix <= ? OR (target_slot = ? AND caller_login = ? AND used_at_unix IS NULL)
	`, now, targetID, callerLogin); err != nil {
		return fmt.Errorf("clearing prior oauth transactions: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO oauth_transactions (
			id, target_slot, state_digest, code_verifier, expires_at_unix, caller_login
		) VALUES (?, ?, ?, ?, ?, ?)
	`, hex.EncodeToString(stateDigest), targetID, stateDigest, []byte{}, expiresAt.Unix(), callerLogin); err != nil {
		return fmt.Errorf("storing oauth transaction: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing oauth transaction: %w", err)
	}

	return nil
}

// ConsumeAuthorization verifies and marks a pending OAuth state used. It
// returns the bound target slot, never the raw state or caller identity.
func (s *Store) ConsumeAuthorization(ctx context.Context, callerLogin string, stateDigest []byte) (string, error) {
	if strings.TrimSpace(callerLogin) == "" || len(stateDigest) != 32 {
		return "", errors.New("caller identity and state digest are required")
	}

	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return "", fmt.Errorf("starting oauth callback transaction: %w", err)
	}
	defer rollback(transaction)

	var (
		targetID string
		caller   string
		expires  int64
		usedAt   sql.NullInt64
	)
	err = transaction.QueryRowContext(ctx, `
		SELECT target_slot, caller_login, expires_at_unix, used_at_unix
		FROM oauth_transactions
		WHERE state_digest = ?
	`, stateDigest).Scan(&targetID, &caller, &expires, &usedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrOAuthTransactionNotFound
	}
	if err != nil {
		return "", fmt.Errorf("reading oauth transaction: %w", err)
	}
	if caller != callerLogin {
		return "", ErrOAuthTransactionIdentityMismatch
	}
	if usedAt.Valid {
		return "", ErrOAuthTransactionUsed
	}
	if expires <= time.Now().Unix() {
		return "", ErrOAuthTransactionExpired
	}

	result, err := transaction.ExecContext(ctx, `
		UPDATE oauth_transactions
		SET used_at_unix = ?
		WHERE state_digest = ? AND used_at_unix IS NULL
	`, time.Now().Unix(), stateDigest)
	if err != nil {
		return "", fmt.Errorf("consuming oauth transaction: %w", err)
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return "", fmt.Errorf("checking oauth transaction consumption: %w", err)
	}
	if updated == 0 {
		return "", ErrOAuthTransactionUsed
	}
	if err := transaction.Commit(); err != nil {
		return "", fmt.Errorf("committing oauth callback transaction: %w", err)
	}

	return targetID, nil
}

// ForEachPendingAuthorization visits every target slot that has an
// authorization in flight: a transaction this service started, that has neither
// expired nor been consumed.
//
// It exists because "pending" is a state of the flow rather than of the slot.
// The targets table holds the three states a slot durably has, and the moment
// between a start request and its callback is not one of them — it is the
// presence of exactly this row, which BeginAuthorization writes and
// ConsumeAuthorization retires. Reading it here rather than storing a fourth
// state keeps that moment from needing transitions of its own on expiry,
// denial, and exchange failure, none of which the service is told about.
//
// It reports the slot alone. The state digest, the caller identity, and the
// expiry are the flow's own secrets and never leave this method.
func (s *Store) ForEachPendingAuthorization(ctx context.Context, visit func(targetID string) error) error {
	if visit == nil {
		return errors.New("pending authorization visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT DISTINCT target_slot
		FROM oauth_transactions
		WHERE used_at_unix IS NULL AND expires_at_unix > ?
		ORDER BY target_slot
	`, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("listing pending authorizations: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var targetID string
		if err := rows.Scan(&targetID); err != nil {
			return fmt.Errorf("reading pending authorization: %w", err)
		}
		if err := visit(targetID); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating pending authorizations: %w", err)
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

func (s *Store) encrypt(targetID string, value []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("creating encryption nonce: %w", err)
	}

	return s.aead.Seal(nonce, nonce, value, []byte(targetID)), nil
}

func (s *Store) decrypt(targetID string, value []byte) ([]byte, error) {
	nonceSize := s.aead.NonceSize()
	if len(value) <= nonceSize {
		return nil, ErrStateUnreadable
	}
	decrypted, err := s.aead.Open(nil, value[:nonceSize], value[nonceSize:], []byte(targetID))
	if err != nil {
		return nil, ErrStateUnreadable
	}

	return decrypted, nil
}

func validateTargetIDs(targetIDs []string) error {
	if len(targetIDs) == 0 {
		return errors.New("at least one target ID is required")
	}

	seen := make(map[string]struct{}, len(targetIDs))
	for _, targetID := range targetIDs {
		if strings.TrimSpace(targetID) == "" {
			return errors.New("target ID is required")
		}
		if _, found := seen[targetID]; found {
			return fmt.Errorf("target ID %q is duplicated", targetID)
		}
		seen[targetID] = struct{}{}
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
	}
}
