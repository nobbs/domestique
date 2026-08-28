package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/route"
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
	err = s.database.QueryRowContext(ctx, `
		SELECT content_hash, surface_generation, coefficient_fingerprint
		FROM stage_duration WHERE provider = ? AND route_id = ? AND stage_order = ?
	`, provider, routeID, stageOrder).Scan(&contentHash, &surfaceGeneration, &coefficientFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", false, nil
	}
	if err != nil {
		return "", "", "", false, fmt.Errorf("reading stage duration fingerprint: %w", err)
	}

	return contentHash, surfaceGeneration, coefficientFingerprint, true, nil
}

// StoreStageDuration caches one stage's predicted moving time, or its absence,
// against the geometry, surface generation, and coefficient fingerprint it was
// computed from. A nil movingSeconds records that this exact combination was
// asked and answered with no prediction — a stage with no usable elevation —
// which is what stops it being asked again every run.
func (s *Store) StoreStageDuration(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
	contentHash, surfaceGeneration, coefficientFingerprint string,
	movingSeconds *float64,
	cumulativeSeconds []byte,
) error {
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO stage_duration (
			provider, route_id, stage_order, content_hash, surface_generation, coefficient_fingerprint,
			moving_seconds, cumulative_seconds, updated_at_unix
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, route_id, stage_order) DO UPDATE SET
			content_hash = excluded.content_hash,
			surface_generation = excluded.surface_generation,
			coefficient_fingerprint = excluded.coefficient_fingerprint,
			moving_seconds = excluded.moving_seconds,
			cumulative_seconds = excluded.cumulative_seconds,
			updated_at_unix = excluded.updated_at_unix
	`,
		provider, routeID, stageOrder, contentHash, surfaceGeneration, coefficientFingerprint,
		movingSeconds, cumulativeSeconds, time.Now().UTC().Unix(),
	); err != nil {
		return fmt.Errorf("storing stage duration: %w", err)
	}

	return nil
}

// PruneStageDurationsWithDifferentFingerprint deletes every cached prediction
// not measured against currentFingerprint. It exists for the gap
// pruneStageDuration does not cover: that function only reacts to a stage's
// geometry changing, so a coefficient file edited or removed between restarts
// would otherwise leave the previous file's predictions sitting in the read
// path, served as current until the next enrichment pass happens to overwrite
// or is never one for a stage whose geometry has not changed.
//
// Called once at startup. currentFingerprint is empty when no coefficient
// file is configured, which clears every row — matching the contract that an
// unconfigured deployment predicts nothing, rather than serving whatever a
// previous configuration left behind.
func (s *Store) PruneStageDurationsWithDifferentFingerprint(ctx context.Context, currentFingerprint string) error {
	if _, err := s.database.ExecContext(ctx, `
		DELETE FROM stage_duration WHERE coefficient_fingerprint != ?
	`, currentFingerprint); err != nil {
		return fmt.Errorf("pruning stale ride model predictions: %w", err)
	}

	return nil
}

// pruneStageDuration drops predictions that no longer describe anything, on the
// same terms as pruneStageSurface: a row measured against a geometry that has
// since been re-planned addresses a stage that, as far as this row knows, no
// longer exists.
func pruneStageDuration(ctx context.Context, transaction *sql.Tx) error {
	if _, err := transaction.ExecContext(ctx, `
		DELETE FROM stage_duration
		WHERE NOT EXISTS (
			SELECT 1 FROM stage_geometry
			WHERE stage_geometry.provider = stage_duration.provider
			  AND stage_geometry.route_id = stage_duration.route_id
			  AND stage_geometry.stage_order = stage_duration.stage_order
			  AND stage_geometry.content_hash = stage_duration.content_hash
		)
	`); err != nil {
		return fmt.Errorf("pruning stage duration: %w", err)
	}

	return nil
}
