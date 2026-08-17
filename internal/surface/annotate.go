package surface

import (
	"context"
	"encoding/json"
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
// pass's limit, and leaves the rest for a later run.
//
// A stage already classified against its current content hash is skipped without
// contacting the endpoint, so a settled library costs nothing. A stage that
// produced nothing is still recorded: knowing that the question has been asked
// and answered with silence is what stops it being asked again every run.
//
// The first failure ends the pass and is returned. Enrichment is not what a sync
// is for, and the alternative — working through the rest of the inventory
// against an endpoint that has just failed — spends a volunteer's capacity to
// arrive at the same answer more slowly.
func (a *Annotator) Annotate(ctx context.Context, stages []route.Stage) error {
	annotated := 0
	for index := range stages {
		if annotated >= a.limit {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("surface: annotating stages: %w", err)
		}

		stage := &stages[index]
		key := stage.Key()
		cachedHash, found, err := a.cache.StageSurfaceHash(ctx, key.RouteID(), key.StageOrder())
		if err != nil {
			return fmt.Errorf("surface: reading cached classification: %w", err)
		}
		if found && cachedHash == stage.ContentHash() {
			continue
		}

		if err := a.annotateStage(ctx, stage); err != nil {
			return err
		}
		annotated++
	}

	return nil
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
