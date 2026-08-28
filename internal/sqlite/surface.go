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

// StageSurface returns one stage's cached surface classification, but only where
// it was measured against the geometry named by contentHash.
//
// The ranges are positions in that geometry's coordinate array, so serving them
// beside a different revision of the stage would put bands of gravel over
// whatever now happens to sit at those indices. Matching on the hash makes a
// stale row absent rather than wrong: the caller sees a stage whose surface is
// not known yet, which is the truth until the next enrichment pass runs.
//
// The index generation is deliberately not part of this filter, though it is
// half of what StageSurfaceHash checks. The two mismatches are not alike. A row
// measured against an earlier index is stale rather than wrong — its ranges
// still index the geometry the stage actually has — and the table holds one row
// per stage, so withholding it here would serve no surface at all rather than a
// newer one. Every rebuild would blank the whole library until enrichment had
// walked it again, to correct the rare road that was genuinely resurfaced.
// Re-measurement is what corrects those, and StageSurfaceHash is what schedules
// it.
//
// The ranges are returned as stored, ready to serve without re-encoding.
func (s *Store) StageSurface(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
	contentHash string,
) (ranges json.RawMessage, matchedMetres float64, found bool, err error) {
	var stored []byte
	err = s.database.QueryRowContext(ctx, `
		SELECT ranges, matched_metres
		FROM stage_surface
		WHERE provider = ? AND route_id = ? AND stage_order = ? AND content_hash = ?
	`, provider, routeID, stageOrder, contentHash).Scan(&stored, &matchedMetres)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("reading stage surface: %w", err)
	}

	return json.RawMessage(stored), matchedMetres, true, nil
}

// StageSurfaceHash returns what the stored classification was measured against —
// the stage's geometry and the build of the map — so a caller can tell what still
// needs classifying without reading the ranges themselves.
//
// Both halves have to match for a cached row to still be an answer. The content
// hash covers the stage changing under a fixed map; the generation covers the
// map changing under a fixed stage, which is the ordinary case here: a weekly
// index is a weekly opportunity for a road to have been resurfaced.
func (s *Store) StageSurfaceHash(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
) (contentHash, indexGeneration string, found bool, err error) {
	err = s.database.QueryRowContext(ctx, `
		SELECT content_hash, index_generation
		FROM stage_surface WHERE provider = ? AND route_id = ? AND stage_order = ?
	`, provider, routeID, stageOrder).Scan(&contentHash, &indexGeneration)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("reading stage surface hash: %w", err)
	}

	return contentHash, indexGeneration, true, nil
}

// StoreStageSurface caches one stage's classification against the geometry and
// the index build it was measured from. The ranges are stored as given, which is
// exactly the JSON the geometry endpoint serves.
func (s *Store) StoreStageSurface(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
	contentHash, indexGeneration string,
	ranges []byte,
	matchedMetres float64,
) error {
	if _, err := s.database.ExecContext(ctx, `
		INSERT INTO stage_surface (
			provider, route_id, stage_order, content_hash, index_generation, ranges, matched_metres, updated_at_unix
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (provider, route_id, stage_order) DO UPDATE SET
			content_hash = excluded.content_hash,
			index_generation = excluded.index_generation,
			ranges = excluded.ranges,
			matched_metres = excluded.matched_metres,
			updated_at_unix = excluded.updated_at_unix
	`, provider, routeID, stageOrder, contentHash, indexGeneration, ranges, matchedMetres, time.Now().UTC().Unix()); err != nil {
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
			WHERE stage_geometry.provider = stage_surface.provider
			  AND stage_geometry.route_id = stage_surface.route_id
			  AND stage_geometry.stage_order = stage_surface.stage_order
			  AND stage_geometry.content_hash = stage_surface.content_hash
		)
	`); err != nil {
		return fmt.Errorf("pruning stage surface: %w", err)
	}

	return nil
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
//
// It counts on the same terms StageSurface serves on, which is what keeps this
// number and the map agreeing. The index generation is no more a condition here
// than it is there: a stage measured against an earlier index is still shown a
// surface, so counting it as unclassified would report nothing covered after
// every rebuild while every route on the map still had its surfaces.
func (s *Store) SurfaceCoverage(ctx context.Context) (classified, total int, err error) {
	if err := s.database.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM source_stages),
			(SELECT COUNT(*)
				FROM stage_surface
				JOIN source_stages
					ON source_stages.provider = stage_surface.provider
					AND source_stages.route_id = stage_surface.route_id
					AND source_stages.stage_order = stage_surface.stage_order
				WHERE stage_surface.content_hash = source_stages.content_hash)
	`).Scan(&total, &classified); err != nil {
		return 0, 0, fmt.Errorf("reading surface coverage: %w", err)
	}

	return classified, total, nil
}

// SurfaceIndexBuild reports when the surface index was last built and which
// generation that build produced. A service that has never built one reports the
// zero time and an empty generation.
func (s *Store) SurfaceIndexBuild(ctx context.Context) (builtAt time.Time, generation string, err error) {
	var builtAtUnix int64
	if err := s.database.QueryRowContext(ctx, `
		SELECT built_at_unix, generation FROM surface_index WHERE id = 1
	`).Scan(&builtAtUnix, &generation); err != nil {
		return time.Time{}, "", fmt.Errorf("reading the surface index build: %w", err)
	}
	if builtAtUnix == 0 {
		return time.Time{}, generation, nil
	}

	return time.Unix(builtAtUnix, 0).UTC(), generation, nil
}

// RecordSurfaceIndexBuild writes down that a build finished.
//
// It is written for a build that found nothing to do as well as for one that
// produced a new index, because what the next start needs to know is when the
// upstream was last looked at, not when the file last changed.
func (s *Store) RecordSurfaceIndexBuild(ctx context.Context, builtAt time.Time, generation string) error {
	if _, err := s.database.ExecContext(ctx, `
		UPDATE surface_index SET built_at_unix = ?, generation = ? WHERE id = 1
	`, builtAt.UTC().Unix(), generation); err != nil {
		return fmt.Errorf("storing the surface index build: %w", err)
	}

	return nil
}
