package sqlite

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
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

	if err := s.queries.UpsertStageEnrichmentFailure(ctx, sqlcgen.UpsertStageEnrichmentFailureParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
		Pass: pass, Reason: reason, FailedAtUnix: time.Now().UTC().Unix(),
	}); err != nil {
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

	rows, err := s.queries.ListStageEnrichmentFailures(ctx)
	if err != nil {
		return fmt.Errorf("reading stage enrichment failures: %w", err)
	}
	for _, row := range rows {
		key := route.NewKey(route.Provider(row.Provider), row.RouteID, int(row.StageOrder))
		if err := visit(key, row.Pass, row.Reason, time.Unix(row.FailedAtUnix, 0).UTC()); err != nil {
			return err
		}
	}

	return nil
}

// CountStageEnrichmentFailures reports how many stages currently have some
// pass recorded against them, for a status surface that wants the number
// rather than the rows themselves.
func (s *Store) CountStageEnrichmentFailures(ctx context.Context) (count int, err error) {
	stored, err := s.queries.CountStageEnrichmentFailures(ctx)
	if err != nil {
		return 0, fmt.Errorf("counting stage enrichment failures: %w", err)
	}
	return int(stored), nil
}

// pruneStageEnrichmentFailure drops rows whose stage has left the inventory, in
// the caller's transaction.
func pruneStageEnrichmentFailure(ctx context.Context, queries *sqlcgen.Queries) error {
	if err := queries.PruneStageEnrichmentFailures(ctx); err != nil {
		return fmt.Errorf("pruning stage enrichment failures: %w", err)
	}

	return nil
}

// ClearStageDurationFailures drops what the timing pass could not finish, for a
// pass that is no longer configured to run at all. A stage cannot be failing a
// pass nothing is asking for.
func (s *Store) ClearStageDurationFailures(ctx context.Context) error {
	if err := s.queries.DeleteStageEnrichmentFailuresByPass(ctx, PassDuration); err != nil {
		return fmt.Errorf("clearing stage duration failures: %w", err)
	}

	return nil
}

// clearStageEnrichmentFailure drops what a pass last could not finish, in the
// caller's transaction.
func clearStageEnrichmentFailure(
	ctx context.Context, queries *sqlcgen.Queries, provider route.Provider, routeID int64, stageOrder int, pass string,
) error {
	if err := queries.DeleteStageEnrichmentFailure(ctx, sqlcgen.DeleteStageEnrichmentFailureParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder), Pass: pass,
	}); err != nil {
		return fmt.Errorf("clearing a stage enrichment failure: %w", err)
	}

	return nil
}
