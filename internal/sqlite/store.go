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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/route"

	_ "modernc.org/sqlite" // Pure Go SQLite driver registration.
)

const driverName = "sqlite"

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
	// ErrOAuthTransactionIdentityMismatch reports a callback from another Tailnet user.
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
	for index := range stages {
		stage := &stages[index]
		key := stage.Key()

		var storedHash string
		err := transaction.QueryRowContext(ctx, `
			SELECT content_hash FROM stage_geometry WHERE route_id = ? AND stage_order = ?
		`, key.RouteID(), key.StageOrder()).Scan(&storedHash)
		switch {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return fmt.Errorf("reading cached stage geometry: %w", err)
		case storedHash == stage.ContentHash():
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

	return nil
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

// RecordSyncRun stores one terminal synchronization result. Its detail is a
// stable failure category, never provider text or a route name.
func (s *Store) RecordSyncRun(
	ctx context.Context,
	startedAt, finishedAt time.Time,
	outcome, detail string,
	sourceStages, created, updated, deleted int,
) error {
	if startedAt.IsZero() || finishedAt.IsZero() || finishedAt.Before(startedAt) || outcome == "" ||
		sourceStages < 0 || created < 0 || updated < 0 || deleted < 0 {
		return errors.New("complete non-negative sync run metadata is required")
	}
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO sync_runs (
			started_at_unix, finished_at_unix, outcome, detail, source_stages, created, updated, deleted
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, startedAt.Unix(), finishedAt.Unix(), outcome, detail, sourceStages, created, updated, deleted); err != nil {
		return fmt.Errorf("recording sync run: %w", err)
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
// slot and one Tailnet identity. The raw state value is never persisted.
func (s *Store) BeginAuthorization(
	ctx context.Context,
	targetID, tailnetUserLogin string,
	stateDigest []byte,
	expiresAt time.Time,
) error {
	if strings.TrimSpace(targetID) == "" || strings.TrimSpace(tailnetUserLogin) == "" ||
		len(stateDigest) != 32 || !expiresAt.After(time.Now()) {
		return errors.New("target ID, Tailnet identity, state digest, and future expiry are required")
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
	`, now, targetID, tailnetUserLogin); err != nil {
		return fmt.Errorf("clearing prior oauth transactions: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO oauth_transactions (
			id, target_slot, state_digest, code_verifier, expires_at_unix, caller_login
		) VALUES (?, ?, ?, ?, ?, ?)
	`, hex.EncodeToString(stateDigest), targetID, stateDigest, []byte{}, expiresAt.Unix(), tailnetUserLogin); err != nil {
		return fmt.Errorf("storing oauth transaction: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing oauth transaction: %w", err)
	}

	return nil
}

// ConsumeAuthorization verifies and marks a pending OAuth state used. It
// returns the bound target slot, never the raw state or caller identity.
func (s *Store) ConsumeAuthorization(ctx context.Context, tailnetUserLogin string, stateDigest []byte) (string, error) {
	if strings.TrimSpace(tailnetUserLogin) == "" || len(stateDigest) != 32 {
		return "", errors.New("tailnet identity and state digest are required")
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
	if caller != tailnetUserLogin {
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
	if currentVersion > len(migrations) {
		return errors.New("state schema version is newer than this service")
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
	}
}
