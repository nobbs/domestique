package ridemodel

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/nobbs/domestique/internal/route"
)

// Cache holds predicted durations between runs, on the same terms as
// surface.Cache: keyed by stage identity, invalidated by fingerprint rather
// than trusted to still be current, and dealing in encoded bytes so the store
// stays free of this package's types.
type Cache interface {
	StageDurationFingerprint(
		ctx context.Context, provider route.Provider, routeID int64, stageOrder int,
	) (contentHash, surfaceGeneration, coefficientFingerprint string, found bool, err error)
	// RecordStageDurationFailure remembers that a stage could not be timed, and
	// why. Storing a prediction for it clears that.
	RecordStageDurationFailure(
		ctx context.Context, provider route.Provider, routeID int64, stageOrder int, reason string,
	) error
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
// its current geometry and the loaded coefficients' own fingerprint is skipped
// without reading anything else.
type Predictor struct {
	cache        Cache
	coefficients Coefficients
}

// NewPredictor creates a predictor over a duration cache and the coefficients
// loaded once at startup.
//
//nolint:gocritic // value param: called once at composition; not a hot path.
func NewPredictor(cache Cache, coefficients Coefficients) *Predictor {
	return &Predictor{cache: cache, coefficients: coefficients}
}

// Predict predicts every stage whose cached prediction is not current against
// its geometry and the loaded coefficients. It reports how many it predicted
// and how many it could not, on the same terms surface.Annotator does: a stage
// that failed is not a stage silently skipped.
func (p *Predictor) Predict(ctx context.Context, stages []route.Route) (predicted, failed int, err error) {
	for index := range stages {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return predicted, failed, fmt.Errorf("ridemodel: predicting stages: %w", ctxErr)
		}

		stage := &stages[index]
		key := stage.Key()

		cachedHash, cachedSurfaceGeneration, cachedFingerprint, found, hashErr := p.cache.StageDurationFingerprint(
			ctx, key.Provider(), key.SourceRouteID(), key.StageOrder(),
		)
		if hashErr != nil {
			return predicted, failed, fmt.Errorf("ridemodel: reading cached duration: %w", hashErr)
		}
		// Prediction no longer reads the ground, so a row is current only with an
		// empty generation; one cached with a generation is recomputed once.
		if found && cachedHash == stage.ContentHash() && cachedSurfaceGeneration == "" &&
			cachedFingerprint == p.coefficients.Fingerprint {
			continue
		}

		if reason := p.predictStage(ctx, stage); reason != "" {
			// A pass a shutdown ended has learned nothing about this stage, so
			// it records nothing about it and stops here rather than at the top
			// of the next turn.
			if ctxErr := ctx.Err(); ctxErr != nil {
				return predicted, failed, fmt.Errorf("ridemodel: predicting stages: %w", ctxErr)
			}
			p.recordFailure(ctx, key, reason)
			failed++

			continue
		}
		predicted++
	}

	return predicted, failed, nil
}

// What a stage could not be timed for. They are stable words a status surface
// may show, and name a part of this service rather than an upstream.
const (
	// ReasonEncode means the predicted series could not be written down.
	ReasonEncode = "encode"
	// ReasonCache means it could not be stored.
	ReasonCache = "cache"
)

// predictStage predicts one stage and caches the result, reporting what stopped
// it or an empty reason when nothing did.
func (p *Predictor) predictStage(ctx context.Context, stage *route.Route) string {
	result, ok := Predict(stage.Geometry(), p.coefficients)

	var movingSeconds *float64
	var cumulative []byte
	if ok {
		movingSeconds = &result.MovingSeconds
		encoded, encodeErr := json.Marshal(result.CumulativeSeconds)
		if encodeErr != nil {
			return ReasonEncode
		}
		cumulative = encoded
	}

	key := stage.Key()
	if err := p.cache.StoreStageDuration(
		ctx, key.Provider(), key.SourceRouteID(), key.StageOrder(),
		stage.ContentHash(), "", p.coefficients.Fingerprint,
		movingSeconds, cumulative,
	); err != nil {
		return ReasonCache
	}

	return ""
}

// recordFailure names the stage a pass could not finish. A failure that cannot
// itself be written down leaves the count as the only account of it.
func (p *Predictor) recordFailure(ctx context.Context, key route.Key, reason string) {
	// A shutdown reaching the write is the shutdown, not a lost record: this
	// pass had already decided to record nothing about a cancelled stage.
	if err := p.cache.RecordStageDurationFailure(
		ctx, key.Provider(), key.SourceRouteID(), key.StageOrder(), reason,
	); err != nil && ctx.Err() == nil {
		slog.Warn("stage prediction failure not recorded", "reason", reason, "error", err)
	}
}
