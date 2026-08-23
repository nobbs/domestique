package ridemodel

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// SurfaceSource supplies the surface classification cached for a stage's
// current geometry. It is satisfied by *sqlite.Store's existing surface reads —
// the same two methods internal/httpapi already calls to serve the geometry
// endpoint — so the predictor asks the same question surface classification
// itself answers, rather than opening a second path to the same cache.
type SurfaceSource interface {
	StageSurfaceHash(
		ctx context.Context, provider route.Provider, routeID int64, stageOrder int,
	) (contentHash, generation string, found bool, err error)
	StageSurface(
		ctx context.Context, provider route.Provider, routeID int64, stageOrder int, contentHash string,
	) (ranges json.RawMessage, matchedMetres float64, found bool, err error)
}

// Cache holds predicted durations between runs, on the same terms as
// surface.Cache: keyed by stage identity, invalidated by fingerprint rather
// than trusted to still be current, and dealing in encoded bytes so the store
// stays free of this package's types.
type Cache interface {
	StageDurationFingerprint(
		ctx context.Context, provider route.Provider, routeID int64, stageOrder int,
	) (contentHash, surfaceGeneration, coefficientFingerprint string, found bool, err error)
	// StoreStageDuration caches one stage's prediction, or its absence:
	// movingSeconds is nil for a stage with no usable elevation, which is
	// recorded rather than left to be asked about again every run.
	StoreStageDuration(
		ctx context.Context, provider route.Provider, routeID int64, stageOrder int,
		contentHash, surfaceGeneration, coefficientFingerprint string,
		movingSeconds *float64, cumulativeSeconds []byte,
	) error
}

// Predictor computes and caches moving time for stages whose geometry is
// known, in the manner of surface.Annotator: a stage already predicted against
// its current geometry, current surface generation, and the loaded
// coefficients' own fingerprint is skipped without reading anything else.
type Predictor struct {
	source       SurfaceSource
	cache        Cache
	coefficients Coefficients
}

// NewPredictor creates a predictor over a surface source, a duration cache, and
// the coefficients loaded once at startup.
//
//nolint:gocritic // value param: called once at composition; not a hot path.
func NewPredictor(source SurfaceSource, cache Cache, coefficients Coefficients) *Predictor {
	return &Predictor{source: source, cache: cache, coefficients: coefficients}
}

// Predict predicts every stage whose cached prediction is not current against
// its geometry, its surface classification, and the loaded coefficients. It
// reports how many it predicted and how many it could not, on the same terms
// surface.Annotator does: a stage that failed is not a stage silently skipped.
func (p *Predictor) Predict(ctx context.Context, stages []route.Stage) (predicted, failed int, err error) {
	for index := range stages {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return predicted, failed, fmt.Errorf("ridemodel: predicting stages: %w", ctxErr)
		}

		stage := &stages[index]
		key := stage.Key()
		surfaceGeneration := p.currentSurfaceGeneration(ctx, stage)

		cachedHash, cachedSurfaceGeneration, cachedFingerprint, found, hashErr := p.cache.StageDurationFingerprint(
			ctx, key.Provider(), key.RouteID(), key.StageOrder(),
		)
		if hashErr != nil {
			return predicted, failed, fmt.Errorf("ridemodel: reading cached duration: %w", hashErr)
		}
		if found && cachedHash == stage.ContentHash() &&
			cachedSurfaceGeneration == surfaceGeneration && cachedFingerprint == p.coefficients.Fingerprint {
			continue
		}

		if predictErr := p.predictStage(ctx, stage, surfaceGeneration); predictErr != nil {
			failed++

			continue
		}
		predicted++
	}

	return predicted, failed, nil
}

func (p *Predictor) predictStage(ctx context.Context, stage *route.Stage, surfaceGeneration string) error {
	geometry := stage.Geometry()
	kinds := p.surfaceKinds(ctx, stage, len(geometry))

	result, ok := Predict(geometry, kinds, p.coefficients)

	var movingSeconds *float64
	var cumulative []byte
	if ok {
		movingSeconds = &result.MovingSeconds
		encoded, encodeErr := json.Marshal(result.CumulativeSeconds)
		if encodeErr != nil {
			return fmt.Errorf("ridemodel: encoding cumulative series: %w", encodeErr)
		}
		cumulative = encoded
	}

	key := stage.Key()
	if err := p.cache.StoreStageDuration(
		ctx, key.Provider(), key.RouteID(), key.StageOrder(),
		stage.ContentHash(), surfaceGeneration, p.coefficients.Fingerprint,
		movingSeconds, cumulative,
	); err != nil {
		return fmt.Errorf("ridemodel: caching prediction: %w", err)
	}

	return nil
}

// currentSurfaceGeneration is the value a duration is fingerprinted against. It
// is empty both when nothing has classified this stage yet and when a stale
// classification exists for an earlier geometry, and equally when the source
// itself could not answer — a prediction is never blocked on surface
// classification being available, so any of those three reads as "no current
// classification", which is exactly what "timed as asphalt throughout" means.
func (p *Predictor) currentSurfaceGeneration(ctx context.Context, stage *route.Stage) string {
	key := stage.Key()
	contentHash, generation, found, err := p.source.StageSurfaceHash(ctx, key.Provider(), key.RouteID(), key.StageOrder())
	if err != nil || !found || contentHash != stage.ContentHash() {
		return ""
	}

	return generation
}

// surfaceKinds expands the cached classification into one class per point, or
// nil when none is cached — read by Predict as asphalt throughout. A read or
// decode failure degrades the same way rather than failing the prediction:
// surface classification is an enrichment this pass does not depend on.
func (p *Predictor) surfaceKinds(ctx context.Context, stage *route.Stage, pointCount int) []surface.Kind {
	key := stage.Key()
	ranges, _, found, err := p.source.StageSurface(ctx, key.Provider(), key.RouteID(), key.StageOrder(), stage.ContentHash())
	if err != nil || !found {
		return nil
	}

	decoded, err := surface.DecodeRanges(ranges)
	if err != nil {
		return nil
	}

	return surface.Expand(decoded, pointCount)
}
