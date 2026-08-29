package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/nobbs/domestique/internal/route"
)

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
func (s *Store) TrustedInventory(ctx context.Context) ([]route.Route, error) {
	rows, err := s.database.QueryContext(ctx, `
		SELECT
			source_stages.provider,
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
			ON stage_geometry.provider = source_stages.provider
			AND stage_geometry.route_id = source_stages.route_id
			AND stage_geometry.stage_order = source_stages.stage_order
		ORDER BY source_stages.provider, source_stages.route_id, source_stages.stage_order
	`)
	if err != nil {
		return nil, fmt.Errorf("reading the trusted inventory: %w", err)
	}
	defer closeRows(rows)

	stages := make([]route.Route, 0)
	for rows.Next() {
		var provider route.Provider
		var routeID int64
		var stageOrder int
		var revision, contentHash string
		var geometryHash, routeName, stageName sql.NullString
		var coordinates []byte
		if err := rows.Scan(
			&provider, &routeID, &stageOrder, &revision, &contentHash,
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
		stage, err := route.NewRoute(
			provider, routeID, stageOrder, revision, routeName.String, stageName.String, points, contentHash,
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

// TrustedInventoryCount returns the number of stages in the last fully
// validated inventory for one source. Zero means there is no prior trusted
// stage for that provider.
func (s *Store) TrustedInventoryCount(ctx context.Context, provider route.Provider) (int, error) {
	var count int
	if err := s.database.QueryRowContext(
		ctx, "SELECT COUNT(*) FROM source_stages WHERE provider = ?", provider,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting trusted source inventory: %w", err)
	}

	return count, nil
}

// StoreTrustedInventory atomically replaces one source's share of the trusted
// inventory, leaving every other provider's stored stages untouched. It stores
// metadata only, never geometry or FIT bytes.
//
// Scoping the replacement to one provider is what lets a source that failed to
// read this run keep the stages it was last known to have: only a source that
// was read successfully reaches this call at all.
func (s *Store) StoreTrustedInventory(ctx context.Context, provider route.Provider, stages []route.Route) error {
	seen := make(map[route.Key]struct{}, len(stages))
	for _, stage := range stages {
		key := stage.Key()
		if key.Provider() != provider {
			return errors.New("trusted source inventory stage does not match its provider")
		}
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

	if _, err := transaction.ExecContext(ctx, "DELETE FROM source_stages WHERE provider = ?", provider); err != nil {
		return fmt.Errorf("clearing trusted source inventory: %w", err)
	}
	for _, stage := range stages {
		key := stage.Key()
		if _, err := transaction.ExecContext(ctx, `
			INSERT INTO source_stages (provider, route_id, stage_order, source_revision, content_hash)
			VALUES (?, ?, ?, ?, ?)
		`, key.Provider(), key.SourceRouteID(), key.StageOrder(), stage.Revision(), stage.ContentHash()); err != nil {
			return fmt.Errorf("storing trusted source stage: %w", err)
		}
	}
	if err := storeStageGeometry(ctx, transaction, provider, stages); err != nil {
		return err
	}
	if err := pruneStageSurface(ctx, transaction); err != nil {
		return err
	}
	if err := pruneStageDuration(ctx, transaction); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing trusted inventory update: %w", err)
	}

	return nil
}

// ForEachTargetStage visits one target's tracked Wahoo routes in stable source
// order. The visitor receives metadata only and must not retain secrets.
func (s *Store) ForEachTargetStage(
	ctx context.Context,
	targetID string,
	visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error,
) error {
	if strings.TrimSpace(targetID) == "" || visit == nil {
		return errors.New("target ID and target stage visitor are required")
	}

	rows, err := s.database.QueryContext(ctx, `
		SELECT provider, route_id, stage_order, source_revision, content_hash, wahoo_route_id
		FROM target_stages
		WHERE target_slot = ?
		ORDER BY provider, route_id, stage_order
	`, targetID)
	if err != nil {
		return fmt.Errorf("listing target stages: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var (
			provider       route.Provider
			routeID        int64
			stageOrder     int
			sourceRevision string
			contentHash    string
			wahooRouteID   int64
		)
		if err := rows.Scan(&provider, &routeID, &stageOrder, &sourceRevision, &contentHash, &wahooRouteID); err != nil {
			return fmt.Errorf("reading target stage: %w", err)
		}
		if err := visit(provider, routeID, stageOrder, sourceRevision, contentHash, wahooRouteID); err != nil {
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
	provider route.Provider,
	routeID int64,
	stageOrder int,
	sourceRevision, contentHash string,
	wahooRouteID int64,
) error {
	if strings.TrimSpace(targetID) == "" || provider == "" || routeID <= 0 || stageOrder <= 0 ||
		sourceRevision == "" || contentHash == "" || wahooRouteID <= 0 {
		return errors.New("complete target stage metadata is required")
	}

	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO target_stages (
			target_slot, provider, route_id, stage_order, wahoo_route_id, content_hash, source_revision
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(target_slot, provider, route_id, stage_order) DO UPDATE SET
			wahoo_route_id = excluded.wahoo_route_id,
			content_hash = excluded.content_hash,
			source_revision = excluded.source_revision
	`, targetID, provider, routeID, stageOrder, wahooRouteID, contentHash, sourceRevision); err != nil {
		return fmt.Errorf("storing target stage: %w", err)
	}

	return nil
}

// DeleteTargetStage removes the durable mapping after the owned remote Wahoo
// route was deleted successfully.
func (s *Store) DeleteTargetStage(ctx context.Context, targetID string, provider route.Provider, routeID int64, stageOrder int) error {
	if strings.TrimSpace(targetID) == "" || provider == "" || routeID <= 0 || stageOrder <= 0 {
		return errors.New("target ID and source stage key are required")
	}

	result, err := s.database.ExecContext(ctx, `
		DELETE FROM target_stages
		WHERE target_slot = ? AND provider = ? AND route_id = ? AND stage_order = ?
	`, targetID, provider, routeID, stageOrder)
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
