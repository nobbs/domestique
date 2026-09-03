package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// ForEachSourceStage visits trusted source-stage metadata in stable order.
func (s *Store) ForEachSourceStage(ctx context.Context, visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string) error) error {
	if visit == nil {
		return errors.New("source stage visitor is required")
	}
	rows, err := s.queries.ListSourceStages(ctx)
	if err != nil {
		return fmt.Errorf("listing source stages: %w", err)
	}
	for _, row := range rows {
		if err := visit(
			route.Provider(row.Provider), row.RouteID, int(row.StageOrder), row.SourceRevision, row.ContentHash,
		); err != nil {
			return fmt.Errorf("visiting source stage: %w", err)
		}
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
	rows, err := s.queries.ListStageSummaries(ctx)
	if err != nil {
		return fmt.Errorf("listing stage summaries: %w", err)
	}
	for index := range rows {
		row := &rows[index]
		summary := route.Summary{
			Provider: route.Provider(row.Provider), SourceRouteID: row.RouteID, StageOrder: int(row.StageOrder),
			SourceRevision: row.SourceRevision, ContentHash: row.ContentHash,
			SourceRouteName: row.SourceRouteName, RouteName: row.RouteName, PointCount: int(row.PointCount),
			DistanceMetres: row.DistanceMetres, AscentMetres: row.AscentMetres,
			DescentMetres: row.DescentMetres, MaxGradientPercent: row.MaxGradientPercent,
			Bounds: route.Bounds{
				MinLongitude: row.MinLongitude, MinLatitude: row.MinLatitude,
				MaxLongitude: row.MaxLongitude, MaxLatitude: row.MaxLatitude,
			},
		}
		if row.MovingSeconds.Valid {
			summary.MovingSeconds = &row.MovingSeconds.Float64
		}
		if err := visit(summary); err != nil {
			return fmt.Errorf("visiting stage summary: %w", err)
		}
	}
	return nil
}

// StageGeometry returns one stage's cached geometry with its display metadata.
// Coordinates are a JSON array of [longitude, latitude, elevation?] positions and
// cumulativeSeconds a JSON array of predicted moving time, both ready to serve
// as-is. cumulativeSeconds is nil unless measured against this exact geometry.
func (s *Store) StageGeometry(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
) (summary route.Summary, coordinates, cumulativeSeconds json.RawMessage, found bool, err error) {
	row, err := s.queries.GetStageGeometry(ctx, sqlcgen.GetStageGeometryParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return route.Summary{}, nil, nil, false, nil
	}
	if err != nil {
		return route.Summary{}, nil, nil, false, fmt.Errorf("reading stage geometry: %w", err)
	}
	summary = route.Summary{
		Provider: route.Provider(row.Provider), SourceRouteID: row.RouteID, StageOrder: int(row.StageOrder),
		SourceRevision: row.SourceRevision, ContentHash: row.ContentHash,
		SourceRouteName: row.SourceRouteName, RouteName: row.RouteName, PointCount: int(row.PointCount),
		DistanceMetres: row.DistanceMetres, AscentMetres: row.AscentMetres,
		DescentMetres: row.DescentMetres, MaxGradientPercent: row.MaxGradientPercent,
		Bounds: route.Bounds{
			MinLongitude: row.MinLongitude, MinLatitude: row.MinLatitude,
			MaxLongitude: row.MaxLongitude, MaxLatitude: row.MaxLatitude,
		},
	}
	if row.MovingSeconds.Valid {
		summary.MovingSeconds = &row.MovingSeconds.Float64
	}
	return summary, json.RawMessage(row.Coordinates), json.RawMessage(row.CumulativeSeconds), true, nil
}

// reprocessSentinel is what a target mapping records instead of the revision and
// content hash it last pushed, once a reprocess is requested. Not the empty
// string: a mapping is usable only while every field is present, so forgetting
// has to be written as a value. Nothing produces it by accident.
const reprocessSentinel = "reprocess-requested"

// RequestStageReprocess asks for one stage to be redone from scratch, changing no
// route data: it drops the geometry cache, the revision each target last pushed,
// the surface classification, and the predicted moving time. The Wahoo route
// identity is kept, so the next
// reconciliation takes the update path and never creates a second route. Reports
// whether the stage is in the stored inventory.
func (s *Store) RequestStageReprocess(ctx context.Context, provider route.Provider, routeID int64, stageOrder int) (bool, error) {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("starting reprocess request: %w", err)
	}
	defer rollback(transaction)

	queries := s.queries.WithTx(transaction)
	_, err = queries.GetSourceStageExists(ctx, sqlcgen.GetSourceStageExistsParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the stage to reprocess: %w", err)
	}

	if err := queries.UpsertStageReprocess(ctx, sqlcgen.UpsertStageReprocessParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
		RequestedAtUnix: time.Now().UTC().Unix(),
	}); err != nil {
		return false, fmt.Errorf("recording the reprocess request: %w", err)
	}
	if err := queries.MarkTargetStagesForReprocess(ctx, sqlcgen.MarkTargetStagesForReprocessParams{
		SourceRevision: reprocessSentinel, ContentHash: reprocessSentinel,
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
	}); err != nil {
		return false, fmt.Errorf("forgetting the pushed revision: %w", err)
	}
	if err := queries.DeleteStageSurface(ctx, sqlcgen.DeleteStageSurfaceParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
	}); err != nil {
		return false, fmt.Errorf("dropping the stage surface: %w", err)
	}
	// The prediction is keyed by the map's generation, not the classification
	// itself, so a reclassified stage would otherwise keep a time read off the old one.
	if err := queries.DeleteStageDuration(ctx, sqlcgen.DeleteStageDurationParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
	}); err != nil {
		return false, fmt.Errorf("dropping the stage duration: %w", err)
	}
	if err := transaction.Commit(); err != nil {
		return false, fmt.Errorf("committing the reprocess request: %w", err)
	}

	return true, nil
}

// storeStageGeometry refreshes the map-view rendering cache inside the caller's
// transaction. A stage whose content hash is unchanged is left untouched, so an
// unchanged library does not rewrite the whole cache on every scheduled run.
// Rows whose stage has left the inventory are pruned.
func storeStageGeometry(ctx context.Context, queries *sqlcgen.Queries, provider route.Provider, stages []route.Route) error {
	updatedAt := time.Now().UTC().Unix()
	requested, err := requestedReprocessing(ctx, queries)
	if err != nil {
		return err
	}
	for index := range stages {
		stage := &stages[index]
		key := stage.Key()
		_, reprocess := requested[key]

		storedHash, err := queries.GetStageGeometryHash(ctx, sqlcgen.GetStageGeometryHashParams{
			Provider: string(key.Provider()), RouteID: key.SourceRouteID(), StageOrder: int64(key.StageOrder()),
		})
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
		if err := queries.UpsertStageGeometry(ctx, sqlcgen.UpsertStageGeometryParams{
			Provider: string(key.Provider()), RouteID: key.SourceRouteID(), StageOrder: int64(key.StageOrder()),
			ContentHash: stage.ContentHash(), RouteName: stage.SourceRouteName(), StageName: stage.RouteName(),
			PointCount: int64(len(geometry)), DistanceMetres: stage.DistanceMetres(),
			AscentMetres: stage.ElevationGainMetres(), DescentMetres: stage.ElevationLossMetres(),
			MaxGradientPercent: stage.MaxGradientPercent(),
			MinLongitude:       bounds.MinLongitude, MinLatitude: bounds.MinLatitude,
			MaxLongitude: bounds.MaxLongitude, MaxLatitude: bounds.MaxLatitude,
			Coordinates: coordinates, UpdatedAtUnix: updatedAt,
		}); err != nil {
			return fmt.Errorf("storing stage geometry: %w", err)
		}
	}

	if err := queries.PruneStageGeometry(ctx); err != nil {
		return fmt.Errorf("pruning stage geometry: %w", err)
	}
	// The marks are consumed here, in the transaction that acted on them, so a
	// request is honoured exactly once and never outlives the pass that met it.
	// Scoped to this provider: a request for another source's stage belongs to a
	// pass that has not stored anything for it yet, and must still be waiting.
	if err := queries.DeleteStageReprocessByProvider(ctx, string(provider)); err != nil {
		return fmt.Errorf("clearing reprocess requests: %w", err)
	}

	return nil
}

// requestedReprocessing reads the stages an operator has asked to have redone.
func requestedReprocessing(ctx context.Context, queries *sqlcgen.Queries) (map[route.Key]struct{}, error) {
	rows, err := queries.ListStageReprocess(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading reprocess requests: %w", err)
	}
	requested := make(map[route.Key]struct{})
	for _, row := range rows {
		requested[route.NewKey(route.Provider(row.Provider), row.RouteID, int(row.StageOrder))] = struct{}{}
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
