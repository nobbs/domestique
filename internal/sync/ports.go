package sync

import (
	"context"

	"github.com/nobbs/domestique/internal/route"
)

// Source provides a complete, validated inventory of one provider's stages.
type Source interface {
	// Provider names the upstream this source reads. Every stage Inventory
	// returns must carry this same provider.
	Provider() route.Provider
	Inventory(ctx context.Context) ([]route.Route, error)
}

// Encoder produces a FIT course for one source stage.
type Encoder interface {
	Encode(ctx context.Context, stage route.Route) ([]byte, error)
}

// Processor derives a device-export route without changing source identity or
// source-content state.
type Processor interface {
	Process(original *route.Route) (route.Route, error)
}

// Target performs serial Wahoo OAuth refresh and route operations.
type Target interface {
	RefreshAccessToken(ctx context.Context, refreshToken string) (accessToken, replacementRefreshToken string, err error)
	// ListOwnedRoutes returns every route the target holds that carries an
	// external ID this service issued, keyed by it. Reconciliation asks once
	// per run and answers every stage from the result, so an unchanged library
	// costs one request per target rather than one per stage.
	ListOwnedRoutes(ctx context.Context, accessToken string) (map[string]int64, error)
	// DeleteOwnedRoutes removes every route the target holds that this service
	// issued the external ID for, and reports how many. Only clearing uses it. It
	// owns its own pacing: a clear is finished only when the target is empty.
	DeleteOwnedRoutes(ctx context.Context, accessToken string) (int, error)
	CreateRoute(ctx context.Context, accessToken string, stage *route.Route, fitData []byte) (routeID int64, err error)
	UpdateRoute(ctx context.Context, routeID int64, accessToken string, stage *route.Route, fitData []byte) (updatedRouteID int64, err error)
	DeleteRoute(ctx context.Context, routeID int64, accessToken string) error
	IsUnauthorized(err error) bool
}

// Annotator enriches the stored inventory with the surface classification of the
// ground each stage covers. Optional and narrow: whatever it learns it records
// itself. It reports counts because a pass that classified nothing and a pass
// with nothing to classify look identical from outside.
type Annotator interface {
	Annotate(ctx context.Context, stages []route.Route) (classified, failed int, err error)
}

// Predictor computes and caches predicted moving time for the stored inventory.
// Optional, on the same terms as Annotator: a nil predictor leaves stages
// carrying no prediction and changes nothing else about a run.
type Predictor interface {
	Predict(ctx context.Context, stages []route.Route) (predicted, failed int, err error)
}

// State owns the minimum durable state operations required by synchronization.
// Callback iteration avoids sharing persistence record types with adapters.
type State interface {
	TargetAuthorization(ctx context.Context, targetID string) (string, error)
	RefreshToken(ctx context.Context, targetID string) (string, error)
	ReplaceRefreshToken(ctx context.Context, targetID, refreshToken string) error
	MarkNeedsReauthorization(ctx context.Context, targetID string) error
	TrustedInventoryCount(ctx context.Context, provider route.Provider) (int, error)
	StoreTrustedInventory(ctx context.Context, provider route.Provider, stages []route.Route) error
	TrustedInventory(ctx context.Context) ([]route.Route, error)
	ForEachTargetStage(ctx context.Context, targetID string, visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error) error
	UpsertTargetStage(ctx context.Context, targetID string, provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error
	DeleteTargetStage(ctx context.Context, targetID string, provider route.Provider, routeID int64, stageOrder int) error
}
