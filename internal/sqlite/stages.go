package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

// ForEachSourceStage visits trusted source-stage metadata in stable order.
func (s *Store) ForEachSourceStage(ctx context.Context, visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string) error) error {
	if visit == nil {
		return errors.New("source stage visitor is required")
	}
	rows, err := s.database.QueryContext(ctx, `
		SELECT provider, route_id, stage_order, source_revision, content_hash
		FROM source_stages ORDER BY provider, route_id, stage_order
	`)
	if err != nil {
		return fmt.Errorf("listing source stages: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var provider route.Provider
		var routeID int64
		var stageOrder int
		var sourceRevision, contentHash string
		if err := rows.Scan(&provider, &routeID, &stageOrder, &sourceRevision, &contentHash); err != nil {
			return fmt.Errorf("reading source stage: %w", err)
		}
		if err := visit(provider, routeID, stageOrder, sourceRevision, contentHash); err != nil {
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
			source_stages.provider,
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
			stage_duration.moving_seconds,
			COALESCE(stage_geometry.min_longitude, 0),
			COALESCE(stage_geometry.min_latitude, 0),
			COALESCE(stage_geometry.max_longitude, 0),
			COALESCE(stage_geometry.max_latitude, 0)
		FROM source_stages
		LEFT JOIN stage_geometry
			ON stage_geometry.provider = source_stages.provider
			AND stage_geometry.route_id = source_stages.route_id
			AND stage_geometry.stage_order = source_stages.stage_order
		LEFT JOIN stage_duration
			ON stage_duration.provider = source_stages.provider
			AND stage_duration.route_id = source_stages.route_id
			AND stage_duration.stage_order = source_stages.stage_order
			AND stage_duration.content_hash = source_stages.content_hash
		ORDER BY source_stages.provider, source_stages.route_id, source_stages.stage_order
	`)
	if err != nil {
		return fmt.Errorf("listing stage summaries: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var summary route.Summary
		var movingSeconds sql.NullFloat64
		if err := rows.Scan(
			&summary.Provider, &summary.SourceRouteID, &summary.StageOrder, &summary.SourceRevision, &summary.ContentHash,
			&summary.SourceRouteName, &summary.RouteName, &summary.PointCount, &summary.DistanceMetres,
			&summary.AscentMetres, &summary.MaxGradientPercent, &movingSeconds,
			&summary.Bounds.MinLongitude, &summary.Bounds.MinLatitude,
			&summary.Bounds.MaxLongitude, &summary.Bounds.MaxLatitude,
		); err != nil {
			return fmt.Errorf("reading stage summary: %w", err)
		}
		if movingSeconds.Valid {
			summary.MovingSeconds = &movingSeconds.Float64
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
// Coordinates are a JSON array of [longitude, latitude, elevation?] positions and
// cumulativeSeconds a JSON array of predicted moving time, both ready to serve
// as-is. cumulativeSeconds is nil unless measured against this exact geometry.
func (s *Store) StageGeometry(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
) (summary route.Summary, coordinates, cumulativeSeconds json.RawMessage, found bool, err error) {
	var coordinatesBytes []byte
	var cumulativeSecondsBytes []byte
	var movingSeconds sql.NullFloat64
	err = s.database.QueryRowContext(ctx, `
		SELECT
			stage_geometry.provider,
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
			stage_duration.moving_seconds,
			stage_geometry.min_longitude,
			stage_geometry.min_latitude,
			stage_geometry.max_longitude,
			stage_geometry.max_latitude,
			stage_geometry.coordinates,
			stage_duration.cumulative_seconds
		FROM stage_geometry
		LEFT JOIN source_stages
			ON source_stages.provider = stage_geometry.provider
			AND source_stages.route_id = stage_geometry.route_id
			AND source_stages.stage_order = stage_geometry.stage_order
		LEFT JOIN stage_duration
			ON stage_duration.provider = stage_geometry.provider
			AND stage_duration.route_id = stage_geometry.route_id
			AND stage_duration.stage_order = stage_geometry.stage_order
			AND stage_duration.content_hash = stage_geometry.content_hash
		WHERE stage_geometry.provider = ? AND stage_geometry.route_id = ? AND stage_geometry.stage_order = ?
	`, provider, routeID, stageOrder).Scan(
		&summary.Provider, &summary.SourceRouteID, &summary.StageOrder, &summary.SourceRevision, &summary.ContentHash,
		&summary.SourceRouteName, &summary.RouteName, &summary.PointCount, &summary.DistanceMetres,
		&summary.AscentMetres, &summary.MaxGradientPercent, &movingSeconds,
		&summary.Bounds.MinLongitude, &summary.Bounds.MinLatitude,
		&summary.Bounds.MaxLongitude, &summary.Bounds.MaxLatitude,
		&coordinatesBytes,
		&cumulativeSecondsBytes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return route.Summary{}, nil, nil, false, nil
	}
	if err != nil {
		return route.Summary{}, nil, nil, false, fmt.Errorf("reading stage geometry: %w", err)
	}
	if movingSeconds.Valid {
		summary.MovingSeconds = &movingSeconds.Float64
	}

	return summary, json.RawMessage(coordinatesBytes), json.RawMessage(cumulativeSecondsBytes), true, nil
}

// reprocessSentinel is what a target mapping records instead of the revision and
// content hash it last pushed, once a reprocess is requested. Not the empty
// string: a mapping is usable only while every field is present, so forgetting
// has to be written as a value. Nothing produces it by accident.
const reprocessSentinel = "reprocess-requested"

// RequestStageReprocess asks for one stage to be redone from scratch, changing no
// route data: it drops the geometry cache, the revision each target last pushed,
// and the surface classification. The Wahoo route identity is kept, so the next
// reconciliation takes the update path and never creates a second route. Reports
// whether the stage is in the stored inventory.
func (s *Store) RequestStageReprocess(ctx context.Context, provider route.Provider, routeID int64, stageOrder int) (bool, error) {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return false, fmt.Errorf("starting reprocess request: %w", err)
	}
	defer rollback(transaction)

	var exists int
	err = transaction.QueryRowContext(ctx, `
		SELECT 1 FROM source_stages WHERE provider = ? AND route_id = ? AND stage_order = ?
	`, provider, routeID, stageOrder).Scan(&exists)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("reading the stage to reprocess: %w", err)
	}

	if _, err := transaction.ExecContext(ctx, `
		INSERT INTO stage_reprocess (provider, route_id, stage_order, requested_at_unix)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (provider, route_id, stage_order) DO UPDATE SET requested_at_unix = excluded.requested_at_unix
	`, provider, routeID, stageOrder, time.Now().UTC().Unix()); err != nil {
		return false, fmt.Errorf("recording the reprocess request: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		UPDATE target_stages SET source_revision = ?, content_hash = ?
		WHERE provider = ? AND route_id = ? AND stage_order = ?
	`, reprocessSentinel, reprocessSentinel, provider, routeID, stageOrder); err != nil {
		return false, fmt.Errorf("forgetting the pushed revision: %w", err)
	}
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM stage_surface WHERE provider = ? AND route_id = ? AND stage_order = ?
	`, provider, routeID, stageOrder); err != nil {
		return false, fmt.Errorf("dropping the stage surface: %w", err)
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
func storeStageGeometry(ctx context.Context, transaction *sql.Tx, provider route.Provider, stages []route.Route) error {
	updatedAt := time.Now().UTC().Unix()
	requested, err := requestedReprocessing(ctx, transaction)
	if err != nil {
		return err
	}
	for index := range stages {
		stage := &stages[index]
		key := stage.Key()
		_, reprocess := requested[key]

		var storedHash string
		err := transaction.QueryRowContext(ctx, `
			SELECT content_hash FROM stage_geometry WHERE provider = ? AND route_id = ? AND stage_order = ?
		`, key.Provider(), key.SourceRouteID(), key.StageOrder()).Scan(&storedHash)
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
				provider, route_id, stage_order, content_hash, route_name, stage_name,
				point_count, distance_metres, ascent_metres, max_gradient_percent,
				min_longitude, min_latitude, max_longitude, max_latitude,
				coordinates, updated_at_unix
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT (provider, route_id, stage_order) DO UPDATE SET
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
			key.Provider(), key.SourceRouteID(), key.StageOrder(), stage.ContentHash(), stage.SourceRouteName(), stage.RouteName(),
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
			WHERE source_stages.provider = stage_geometry.provider
			  AND source_stages.route_id = stage_geometry.route_id
			  AND source_stages.stage_order = stage_geometry.stage_order
		)
	`); err != nil {
		return fmt.Errorf("pruning stage geometry: %w", err)
	}
	// The marks are consumed here, in the transaction that acted on them, so a
	// request is honoured exactly once and never outlives the pass that met it.
	// Scoped to this provider: a request for another source's stage belongs to a
	// pass that has not stored anything for it yet, and must still be waiting.
	if _, err := transaction.ExecContext(ctx, "DELETE FROM stage_reprocess WHERE provider = ?", provider); err != nil {
		return fmt.Errorf("clearing reprocess requests: %w", err)
	}

	return nil
}

// requestedReprocessing reads the stages an operator has asked to have redone.
func requestedReprocessing(ctx context.Context, transaction *sql.Tx) (map[route.Key]struct{}, error) {
	rows, err := transaction.QueryContext(ctx, "SELECT provider, route_id, stage_order FROM stage_reprocess")
	if err != nil {
		return nil, fmt.Errorf("reading reprocess requests: %w", err)
	}
	defer closeRows(rows)

	requested := make(map[route.Key]struct{})
	for rows.Next() {
		var provider route.Provider
		var routeID int64
		var stageOrder int
		if err := rows.Scan(&provider, &routeID, &stageOrder); err != nil {
			return nil, fmt.Errorf("reading a reprocess request: %w", err)
		}
		requested[route.NewKey(provider, routeID, stageOrder)] = struct{}{}
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
