package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// StageDurationFingerprint returns what a stored prediction was computed
// against: the stage's geometry, the surface classification generation it read,
// and the coefficient file it was fitted from. All three have to match for a
// cached row to still be an answer, on the same terms as StageSurfaceHash.
func (s *Store) StageDurationFingerprint(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
) (contentHash, surfaceGeneration, coefficientFingerprint string, found bool, err error) {
	row, err := s.queries.GetStageDurationFingerprint(ctx, sqlcgen.GetStageDurationFingerprintParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, fmt.Errorf("reading stage duration fingerprint: %w", err)
	}

	return row.ContentHash, row.SurfaceGeneration, row.CoefficientFingerprint, true, nil
}

// StoreStageDuration caches one stage's predicted moving time, or its absence,
// against the geometry, surface generation and coefficient fingerprint it was
// computed from. A nil movingSeconds records that this combination was asked and
// answered with no prediction, which stops it being asked again every run.
func (s *Store) StoreStageDuration(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
	contentHash, surfaceGeneration, coefficientFingerprint string,
	movingSeconds *float64,
	cumulativeSeconds []byte,
) error {
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storing stage duration: %w", err)
	}
	defer rollback(transaction)

	queries := s.queries.WithTx(transaction)
	storedMovingSeconds := sql.NullFloat64{}
	if movingSeconds != nil {
		storedMovingSeconds = sql.NullFloat64{Float64: *movingSeconds, Valid: true}
	}
	if err := queries.UpsertStageDuration(ctx, sqlcgen.UpsertStageDurationParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
		ContentHash: contentHash, SurfaceGeneration: surfaceGeneration,
		CoefficientFingerprint: coefficientFingerprint, MovingSeconds: storedMovingSeconds,
		CumulativeSeconds: cumulativeSeconds, UpdatedAtUnix: time.Now().UTC().Unix(),
	}); err != nil {
		return fmt.Errorf("storing stage duration: %w", err)
	}
	// Stored and still listed as failing cannot both be true, so the two move
	// together.
	if err := clearStageEnrichmentFailure(ctx, queries, provider, routeID, stageOrder, PassDuration); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing stage duration: %w", err)
	}

	return nil
}

// PruneStageDurationsWithDifferentFingerprint deletes every cached prediction not
// measured against currentFingerprint, covering what pruneStageDuration cannot: a
// coefficient file edited between restarts. Called once at startup; an empty
// fingerprint means no file is configured and clears every row.
func (s *Store) PruneStageDurationsWithDifferentFingerprint(ctx context.Context, currentFingerprint string) error {
	if err := s.queries.DeleteStageDurationsWithDifferentFingerprint(ctx, currentFingerprint); err != nil {
		return fmt.Errorf("pruning stale ride model predictions: %w", err)
	}

	return nil
}

// pruneStageDuration drops predictions that no longer describe anything, on the
// same terms as pruneStageSurface: a row measured against a geometry that has
// since been re-planned addresses a stage that, as far as this row knows, no
// longer exists.
func pruneStageDuration(ctx context.Context, queries *sqlcgen.Queries) error {
	if err := queries.PruneStageDuration(ctx); err != nil {
		return fmt.Errorf("pruning stage duration: %w", err)
	}

	return nil
}
