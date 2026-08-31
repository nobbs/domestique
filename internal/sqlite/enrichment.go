package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

// The enrichment passes a stage can be missing. They are stored rather than
// derived, so a pass that is not configured at all leaves no rows behind.
const (
	PassSurface  = "surface"
	PassDuration = "duration"
)

// RecordStageSurfaceFailure remembers that a stage could not be classified.
func (s *Store) RecordStageSurfaceFailure(
	ctx context.Context, provider route.Provider, routeID int64, stageOrder int, reason string,
) error {
	return s.recordStageEnrichmentFailure(ctx, provider, routeID, stageOrder, PassSurface, reason)
}

// RecordStageDurationFailure remembers that a stage could not be timed.
func (s *Store) RecordStageDurationFailure(
	ctx context.Context, provider route.Provider, routeID int64, stageOrder int, reason string,
) error {
	return s.recordStageEnrichmentFailure(ctx, provider, routeID, stageOrder, PassDuration, reason)
}

// recordStageEnrichmentFailure remembers what one pass could not finish for one
// stage. reason is a stable category, never an upstream message or a local path.
func (s *Store) recordStageEnrichmentFailure(
	ctx context.Context, provider route.Provider, routeID int64, stageOrder int, pass, reason string,
) error {
	if provider == "" || reason == "" {
		return errors.New("a provider and a reason are required")
	}

	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO stage_enrichment_failure (provider, route_id, stage_order, pass, reason, failed_at_unix)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, route_id, stage_order, pass) DO UPDATE SET
			reason = excluded.reason,
			failed_at_unix = excluded.failed_at_unix
	`, provider, routeID, stageOrder, pass, reason, time.Now().UTC().Unix()); err != nil {
		return fmt.Errorf("recording a stage enrichment failure: %w", err)
	}

	return nil
}

// ForEachStageEnrichmentFailure visits what each pass currently cannot finish,
// in stable stage order. It is what turns a count of incomplete stages into the
// stages themselves.
func (s *Store) ForEachStageEnrichmentFailure(
	ctx context.Context,
	visit func(key route.Key, pass, reason string, failedAt time.Time) error,
) error {
	if visit == nil {
		return errors.New("a stage enrichment failure visitor is required")
	}

	rows, err := s.database.QueryContext(ctx, `
		SELECT provider, route_id, stage_order, pass, reason, failed_at_unix
		FROM stage_enrichment_failure
		ORDER BY provider, route_id, stage_order, pass
	`)
	if err != nil {
		return fmt.Errorf("reading stage enrichment failures: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var provider, pass, reason string
		var routeID int64
		var stageOrder int
		var failedAt int64
		if err := rows.Scan(&provider, &routeID, &stageOrder, &pass, &reason, &failedAt); err != nil {
			return fmt.Errorf("reading a stage enrichment failure: %w", err)
		}
		key := route.NewKey(route.Provider(provider), routeID, stageOrder)
		if err := visit(key, pass, reason, time.Unix(failedAt, 0).UTC()); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("reading stage enrichment failures: %w", err)
	}

	return nil
}

// CountStageEnrichmentFailures reports how many stages currently have some
// pass recorded against them, for a status surface that wants the number
// rather than the rows themselves.
func (s *Store) CountStageEnrichmentFailures(ctx context.Context) (count int, err error) {
	if err := s.database.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM stage_enrichment_failure
	`).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting stage enrichment failures: %w", err)
	}

	return count, nil
}

// pruneStageEnrichmentFailure drops rows whose stage has left the inventory, in
// the caller's transaction.
func pruneStageEnrichmentFailure(ctx context.Context, transaction *sql.Tx) error {
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM stage_enrichment_failure
		WHERE NOT EXISTS (
			SELECT 1 FROM stage_geometry
			WHERE stage_geometry.provider = stage_enrichment_failure.provider
			  AND stage_geometry.route_id = stage_enrichment_failure.route_id
			  AND stage_geometry.stage_order = stage_enrichment_failure.stage_order
		)
	`); err != nil {
		return fmt.Errorf("pruning stage enrichment failures: %w", err)
	}

	return nil
}

// ClearStageDurationFailures drops what the timing pass could not finish, for a
// pass that is no longer configured to run at all. A stage cannot be failing a
// pass nothing is asking for.
func (s *Store) ClearStageDurationFailures(ctx context.Context) error {
	if _, err := s.database.ExecContext(ctx, `
		DELETE FROM stage_enrichment_failure WHERE pass = ?
	`, PassDuration); err != nil {
		return fmt.Errorf("clearing stage duration failures: %w", err)
	}

	return nil
}

// clearStageEnrichmentFailure drops what a pass last could not finish, in the
// caller's transaction.
func clearStageEnrichmentFailure(
	ctx context.Context, transaction *sql.Tx, provider route.Provider, routeID int64, stageOrder int, pass string,
) error {
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM stage_enrichment_failure
		WHERE provider = ? AND route_id = ? AND stage_order = ? AND pass = ?
	`, provider, routeID, stageOrder, pass); err != nil {
		return fmt.Errorf("clearing a stage enrichment failure: %w", err)
	}

	return nil
}
