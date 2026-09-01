package sqlite

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// TrustedInventory rebuilds the stored source inventory as the stages a target
// reconciliation works from: the handover between the two halves. The geometry
// returned is the stored export profile, so a course encoded from it is the one
// the source pass derived. A stage whose geometry is missing or cached against a
// different hash fails the whole read — a smaller library reads as a deletion.
func (s *Store) TrustedInventory(ctx context.Context) ([]route.Route, error) {
	rows, err := s.queries.ListTrustedInventory(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the trusted inventory: %w", err)
	}

	stages := make([]route.Route, 0)
	//nolint:gocritic // sqlc rows are short-lived; the clearer value range avoids needless indexing.
	for _, row := range rows {
		if !row.GeometryHash.Valid || row.GeometryHash.String != row.ContentHash {
			return nil, fmt.Errorf(
				"trusted inventory stage %d/%d has no geometry for its content hash", row.RouteID, row.StageOrder,
			)
		}
		points, err := decodeCoordinates(row.Coordinates)
		if err != nil {
			return nil, err
		}
		stage, err := route.NewRoute(
			route.Provider(row.Provider), row.RouteID, int(row.StageOrder), row.SourceRevision, row.RouteName.String, row.StageName.String, points, row.ContentHash,
		)
		if err != nil {
			return nil, fmt.Errorf("rebuilding trusted inventory stage %d/%d: %w", row.RouteID, row.StageOrder, err)
		}
		stages = append(stages, stage)
	}

	return stages, nil
}

// TrustedInventoryCount returns the number of stages in the last fully
// validated inventory for one source. Zero means there is no prior trusted
// stage for that provider.
func (s *Store) TrustedInventoryCount(ctx context.Context, provider route.Provider) (int, error) {
	count, err := s.queries.CountTrustedInventory(ctx, string(provider))
	if err != nil {
		return 0, fmt.Errorf("counting trusted source inventory: %w", err)
	}
	return int(count), nil
}

// StoreTrustedInventory atomically replaces one source's share of the trusted
// inventory, leaving every other provider's stages untouched. Metadata only,
// never geometry or FIT bytes. Scoping to one provider is what lets a source that
// failed this run keep the stages it was last known to have.
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

	queries := s.queries.WithTx(transaction)
	if err := queries.DeleteSourceStagesByProvider(ctx, string(provider)); err != nil {
		return fmt.Errorf("clearing trusted source inventory: %w", err)
	}
	for _, stage := range stages {
		key := stage.Key()
		if err := queries.InsertSourceStage(ctx, sqlcgen.InsertSourceStageParams{
			Provider: string(key.Provider()), RouteID: key.SourceRouteID(), StageOrder: int64(key.StageOrder()),
			SourceRevision: stage.Revision(), ContentHash: stage.ContentHash(),
		}); err != nil {
			return fmt.Errorf("storing trusted source stage: %w", err)
		}
	}
	if err := storeStageGeometry(ctx, queries, provider, stages); err != nil {
		return err
	}
	if err := pruneStageSurface(ctx, queries); err != nil {
		return err
	}
	if err := pruneStageDuration(ctx, queries); err != nil {
		return err
	}
	if err := pruneStageEnrichmentFailure(ctx, queries); err != nil {
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

	rows, err := s.queries.ListTargetStages(ctx, targetID)
	if err != nil {
		return fmt.Errorf("listing target stages: %w", err)
	}
	for _, row := range rows {
		if err := visit(
			route.Provider(row.Provider), row.RouteID, int(row.StageOrder),
			row.SourceRevision, row.ContentHash, row.WahooRouteID,
		); err != nil {
			return fmt.Errorf("visiting target stage: %w", err)
		}
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

	if err := s.queries.UpsertTargetStage(ctx, sqlcgen.UpsertTargetStageParams{
		TargetSlot: targetID, Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
		WahooRouteID: wahooRouteID, ContentHash: contentHash, SourceRevision: sourceRevision,
	}); err != nil {
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

	result, err := s.queries.DeleteTargetStage(ctx, sqlcgen.DeleteTargetStageParams{
		TargetSlot: targetID, Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
	})
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
