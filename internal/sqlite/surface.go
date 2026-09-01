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

// StageSurface returns one stage's cached surface classification, but only where
// it was measured against the geometry named by contentHash: the ranges are
// positions in that geometry's coordinate array, so matching on the hash makes a
// stale row absent rather than wrong. The index generation is not part of this
// filter, though StageSurfaceHash checks it — withholding an older reading would
// blank the library after every rebuild. Ranges are returned as stored.
func (s *Store) StageSurface(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
	contentHash string,
) (ranges json.RawMessage, matchedMetres float64, found bool, err error) {
	row, err := s.queries.GetStageSurface(ctx, sqlcgen.GetStageSurfaceParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder), ContentHash: contentHash,
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, false, nil
	}
	if err != nil {
		return nil, 0, false, fmt.Errorf("reading stage surface: %w", err)
	}

	return json.RawMessage(row.Ranges), row.MatchedMetres, true, nil
}

// StageSurfaceHash returns what the stored classification was measured against —
// the stage's geometry and the build of the map — so a caller can tell what needs
// classifying without reading the ranges. Both halves must match: the content
// hash covers the stage changing, the generation covers the map changing.
func (s *Store) StageSurfaceHash(
	ctx context.Context,
	provider route.Provider,
	routeID int64,
	stageOrder int,
) (contentHash, indexGeneration string, found bool, err error) {
	row, err := s.queries.GetStageSurfaceHash(ctx, sqlcgen.GetStageSurfaceHashParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, fmt.Errorf("reading stage surface hash: %w", err)
	}

	return row.ContentHash, row.IndexGeneration, true, nil
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
	transaction, err := s.database.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storing stage surface: %w", err)
	}
	defer rollback(transaction)

	queries := s.queries.WithTx(transaction)
	if err := queries.UpsertStageSurface(ctx, sqlcgen.UpsertStageSurfaceParams{
		Provider: string(provider), RouteID: routeID, StageOrder: int64(stageOrder), ContentHash: contentHash,
		IndexGeneration: indexGeneration, Ranges: ranges, MatchedMetres: matchedMetres, UpdatedAtUnix: time.Now().UTC().Unix(),
	}); err != nil {
		return fmt.Errorf("storing stage surface: %w", err)
	}
	// Stored and still listed as failing cannot both be true, so the two move
	// together.
	if err := clearStageEnrichmentFailure(ctx, queries, provider, routeID, stageOrder, PassSurface); err != nil {
		return err
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing stage surface: %w", err)
	}

	return nil
}

// pruneStageSurface drops classifications that no longer describe anything, in
// the caller's transaction. A row goes when its stage leaves the inventory and
// when the stage is re-planned: the cached ranges address coordinates of a
// geometry that no longer exists.
func pruneStageSurface(ctx context.Context, queries *sqlcgen.Queries) error {
	if err := queries.PruneStageSurface(ctx); err != nil {
		return fmt.Errorf("pruning stage surface: %w", err)
	}

	return nil
}

// SurfaceCoverage reports how many stored stages carry a classification of the
// geometry they currently hold, and how many stages there are. Counting against
// the current content hash is what makes it honest: a stage whose shape changed
// is not classified in any sense the map can use. The index generation is no more
// a condition here than in StageSurface, so the count and the map agree.
func (s *Store) SurfaceCoverage(ctx context.Context) (classified, total int, err error) {
	row, err := s.queries.GetSurfaceCoverage(ctx)
	if err != nil {
		return 0, 0, fmt.Errorf("reading surface coverage: %w", err)
	}
	return int(row.Classified), int(row.Total), nil
}

// SurfaceIndexBuild reports when the surface index was last built and which
// generation that build produced. A service that has never built one reports the
// zero time and an empty generation.
func (s *Store) SurfaceIndexBuild(ctx context.Context) (builtAt time.Time, generation string, err error) {
	row, err := s.queries.GetSurfaceIndexBuild(ctx)
	if err != nil {
		return time.Time{}, "", fmt.Errorf("reading the surface index build: %w", err)
	}
	if row.BuiltAtUnix == 0 {
		return time.Time{}, row.Generation, nil
	}
	return time.Unix(row.BuiltAtUnix, 0).UTC(), row.Generation, nil
}

// RecordSurfaceIndexBuild writes down that a build finished, for one that found
// nothing to do as well as one that produced a new index: the next start needs
// to know when the upstream was last looked at.
func (s *Store) RecordSurfaceIndexBuild(ctx context.Context, builtAt time.Time, generation string) error {
	if err := s.queries.UpdateSurfaceIndexBuild(ctx, sqlcgen.UpdateSurfaceIndexBuildParams{
		BuiltAtUnix: builtAt.UTC().Unix(), Generation: generation,
	}); err != nil {
		return fmt.Errorf("storing the surface index build: %w", err)
	}

	return nil
}
