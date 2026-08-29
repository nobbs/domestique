package surface

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nobbs/domestique/internal/route"
)

// Source supplies the candidate ways near a piece of geometry, and says which
// build of the map it is answering from.
type Source interface {
	Ways(ctx context.Context, points []route.Point) ([]Way, error)
	// Generation identifies the data behind the answers. A classification is
	// cached against it, so a source whose map has been rebuilt reclassifies
	// everything rather than serving last week's reading of a resurfaced road.
	// An empty generation means the source has nothing to answer from yet.
	Generation() string
}

// Cache holds classifications between runs. It is described here rather than
// taken as a concrete store so this package stays free of any storage concern,
// and it deals in encoded bytes so the store stays free of this package's types.
type Cache interface {
	// StageSurfaceHash returns what a stored classification was measured
	// against: the stage's geometry, and the build of the map.
	StageSurfaceHash(ctx context.Context, provider route.Provider, routeID int64, stageOrder int) (contentHash, generation string, found bool, err error)
	// StoreStageSurface caches one stage's classification.
	StoreStageSurface(
		ctx context.Context,
		provider route.Provider,
		routeID int64,
		stageOrder int,
		contentHash, generation string,
		ranges []byte,
		matchedMetres float64,
	) error
}

// Annotator classifies the ground under an inventory of stages and caches what
// it finds.
type Annotator struct {
	source Source
	cache  Cache
}

// NewAnnotator creates an annotator over a way source and a cache.
func NewAnnotator(source Source, cache Cache) *Annotator {
	return &Annotator{source: source, cache: cache}
}

// Annotate classifies every stage whose surface is not known against both its
// current geometry and the current map, reporting how many it classified and how
// many it could not. The whole inventory is walked in one pass. A stage already
// classified against its current hash and generation is skipped; one that
// produced nothing is still recorded, so it is not asked again every run; one
// that fails does not end the pass, which always walks in the same order.
func (a *Annotator) Annotate(ctx context.Context, stages []route.Route) (classified, failed int, err error) {
	// A source with no map behind it has nothing to say. Running anyway would
	// record every stage as unsurveyed and then reclassify the lot as soon as
	// the first index lands, which is worse than waiting.
	generation := a.source.Generation()
	if generation == "" {
		return 0, 0, nil
	}

	for index := range stages {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return classified, failed, fmt.Errorf("surface: annotating stages: %w", ctxErr)
		}

		stage := &stages[index]
		key := stage.Key()
		cachedHash, cachedGeneration, found, hashErr := a.cache.StageSurfaceHash(ctx, key.Provider(), key.SourceRouteID(), key.StageOrder())
		if hashErr != nil {
			return classified, failed, fmt.Errorf("surface: reading cached classification: %w", hashErr)
		}
		if found && cachedHash == stage.ContentHash() && cachedGeneration == generation {
			continue
		}

		if stageErr := a.annotateStage(ctx, stage, generation); stageErr != nil {
			failed++

			continue
		}
		classified++
	}

	return classified, failed, nil
}

// annotateStage classifies one stage and caches the result.
func (a *Annotator) annotateStage(ctx context.Context, stage *route.Route, generation string) error {
	geometry := stage.Geometry()
	ways, err := a.source.Ways(ctx, geometry)
	if err != nil {
		return fmt.Errorf("surface: reading ways along the stage: %w", err)
	}

	kinds := Match(geometry, ways)
	ranges, err := EncodeRanges(Compress(kinds))
	if err != nil {
		return err
	}

	key := stage.Key()
	if err := a.cache.StoreStageSurface(
		ctx,
		key.Provider(),
		key.SourceRouteID(),
		key.StageOrder(),
		stage.ContentHash(),
		generation,
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
	Kind       string `json:"kind"`
	StartIndex int    `json:"startIndex"`
	EndIndex   int    `json:"endIndex"`
}

// EncodeRanges renders ranges as the JSON array the endpoint serves. It is
// exported because the wire form of a classification has exactly one definition,
// and a fixture that stores a classification without going through the matcher
// still has to store that one.
func EncodeRanges(ranges []Range) ([]byte, error) {
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

// DecodeRanges is EncodeRanges's inverse, for a consumer of the cached wire
// form that needs the ranges back rather than the raw bytes — ridemodel's
// predictor, which selects rolling resistance per range.
func DecodeRanges(data []byte) ([]Range, error) {
	var stored []storedRange
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("surface: decoding surface ranges: %w", err)
	}

	ranges := make([]Range, 0, len(stored))
	for _, band := range stored {
		ranges = append(ranges, Range{StartIndex: band.StartIndex, EndIndex: band.EndIndex, Kind: parseKind(band.Kind)})
	}

	return ranges, nil
}

// parseKind is String's inverse. An unrecognised name — from a row written by
// a future version of this package — decodes as KindUnknown rather than
// failing the whole read.
func parseKind(name string) Kind {
	switch name {
	case "asphalt":
		return KindAsphalt
	case "paving":
		return KindPaving
	case "compacted":
		return KindCompacted
	case "gravel":
		return KindGravel
	case "ground":
		return KindGround
	}

	return KindUnknown
}
