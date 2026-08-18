package surface

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/nobbs/domestique/internal/route"
)

// maximumStagesPerRun bounds how many stages one pass will fetch.
//
// Classifying a stage costs the endpoint tens of seconds of work, so a library
// of fifty routes seen for the first time would otherwise hold a sync run open
// for the better part of an hour and spend all of it on a volunteer-run server.
// Filling the cache a few stages at a time spreads that over several runs, and
// costs nothing once it is full: a stage is only fetched again when its geometry
// changes.
const maximumStagesPerRun = 10

// Source supplies the candidate ways near a piece of geometry.
type Source interface {
	Ways(ctx context.Context, points []route.Point) ([]Way, error)
}

// Cache holds classifications between runs. It is described here rather than
// taken as a concrete store so this package stays free of any storage concern,
// and it deals in encoded bytes so the store stays free of this package's types.
type Cache interface {
	// StageSurfaceHash returns the content hash a stored classification was
	// measured against.
	StageSurfaceHash(ctx context.Context, routeID int64, stageOrder int) (string, bool, error)
	// StoreStageSurface caches one stage's classification.
	StoreStageSurface(
		ctx context.Context,
		routeID int64,
		stageOrder int,
		contentHash string,
		ranges []byte,
		matchedMetres float64,
	) error
}

// Annotator classifies the ground under an inventory of stages and caches what
// it finds.
type Annotator struct {
	source Source
	cache  Cache
	limit  int
}

// NewAnnotator creates an annotator over a way source and a cache.
func NewAnnotator(source Source, cache Cache) *Annotator {
	return &Annotator{source: source, cache: cache, limit: maximumStagesPerRun}
}

// Annotate classifies the stages whose surface is not already known, up to this
// pass's limit, and leaves the rest for a later run. It reports how many stages
// it classified and how many it could not.
//
// A stage already classified against its current content hash is skipped without
// contacting the endpoint, so a settled library costs nothing. A stage that
// produced nothing is still recorded: knowing that the question has been asked
// and answered with silence is what stops it being asked again every run.
//
// A stage that fails does not end the pass. The endpoint refuses a share of
// queries under load, and a long stage — classified only once every one of its
// chunk queries lands — fails far more often than a short one. Stopping at the
// first failure meant one such stage starved every stage behind it, in that run
// and in every run after, because the inventory is always walked in the same
// order. Each stage now gets its own attempt whatever happened to the last.
//
// Rate limiting is the exception, and ends the pass. It is the endpoint saying
// it has no capacity, which is an answer about the server rather than about a
// stage: continuing would spend a volunteer's capacity to be refused again.
func (a *Annotator) Annotate(ctx context.Context, stages []route.Stage) (classified, failed int, err error) {
	attempted := 0
	for index := range stages {
		if attempted >= a.limit {
			return classified, failed, nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return classified, failed, fmt.Errorf("surface: annotating stages: %w", ctxErr)
		}

		stage := &stages[index]
		key := stage.Key()
		cachedHash, found, hashErr := a.cache.StageSurfaceHash(ctx, key.RouteID(), key.StageOrder())
		if hashErr != nil {
			return classified, failed, fmt.Errorf("surface: reading cached classification: %w", hashErr)
		}
		if found && cachedHash == stage.ContentHash() {
			continue
		}

		attempted++
		if stageErr := a.annotateStage(ctx, stage); stageErr != nil {
			failed++
			if errors.Is(stageErr, ErrRateLimited) {
				return classified, failed, stageErr
			}

			continue
		}
		classified++
	}

	return classified, failed, nil
}

// annotateStage classifies one stage and caches the result.
func (a *Annotator) annotateStage(ctx context.Context, stage *route.Stage) error {
	geometry := stage.Geometry()
	ways, err := a.source.Ways(ctx, geometry)
	if err != nil {
		return fmt.Errorf("surface: reading ways along the stage: %w", err)
	}

	kinds := Match(geometry, ways)
	ranges, err := encodeRanges(Compress(kinds))
	if err != nil {
		return err
	}

	key := stage.Key()
	if err := a.cache.StoreStageSurface(
		ctx,
		key.RouteID(),
		key.StageOrder(),
		stage.ContentHash(),
		ranges,
		MatchedMetres(geometry, kinds),
	); err != nil {
		return fmt.Errorf("surface: caching classification: %w", err)
	}

	return nil
}

// storedRange is the wire form of one range. The stored bytes are exactly what
// the geometry endpoint serves, so caching them costs no decode and re-encode,
// and the class travels as its stable name rather than as an integer nothing
// outside this package could interpret.
type storedRange struct {
	Kind string `json:"kind"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	StartIndex int `json:"start_index"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	EndIndex int `json:"end_index"`
}

// encodeRanges renders ranges as the JSON array the endpoint serves.
func encodeRanges(ranges []Range) ([]byte, error) {
	stored := make([]storedRange, 0, len(ranges))
	for _, band := range ranges {
		stored = append(stored, storedRange{
			Kind:       band.Kind.String(),
			StartIndex: band.StartIndex,
			EndIndex:   band.EndIndex,
		})
	}

	encoded, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("surface: encoding surface ranges: %w", err)
	}

	return encoded, nil
}
