package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
)

// OAuth performs the protected Wahoo onboarding flow.
type OAuth interface {
	Start(ctx context.Context, callerLogin, targetID string) (string, error)
	Complete(ctx context.Context, callerLogin, state, code string) error
}

// SyncPhase names the half of a synchronization a manual trigger asks for, or
// both halves together. It is declared here rather than imported so this package
// keeps knowing nothing about how synchronization is implemented.
type SyncPhase string

const (
	// SyncPhaseAll reads the source and then writes to the targets.
	SyncPhaseAll SyncPhase = "all"
	// SyncPhaseSource reads the source library into stored state.
	SyncPhaseSource SyncPhase = "source"
	// SyncPhaseTargets reconciles stored state onto the targets.
	SyncPhaseTargets SyncPhase = "targets"
)

// Sync is the synchronization process behind this surface: it starts a manual
// run, and it says what has not finished yet.
//
// Both answers come from the process rather than from stored state. A run that
// has not finished has recorded nothing, and one that has not started has
// nothing to record, so a status built from state alone can only describe the
// last run that ended.
type Sync interface {
	// Trigger starts one manual synchronization and reports whether it was
	// accepted. An accepted run continues independently of the HTTP request.
	Trigger(phase SyncPhase) bool
	// TriggerTarget starts a manual reconciliation of exactly one configured
	// target, on the same terms as Trigger scoped to that slot alone.
	TriggerTarget(targetID string) bool
	// TriggerClear starts a manual clear of exactly one configured target,
	// deleting every route this service owns there. It is destructive, and
	// reachable only because an operator asked for it directly.
	TriggerClear(targetID string) bool
	// Activity reports the run that has not finished, if there is one.
	Activity() SyncActivityState
	// TriggerAnnotate starts one manual surface-classification pass and reports
	// whether it was accepted. Unlike Trigger, it never reads the source or
	// writes a target — it shares Trigger's guard, so it is refused while a
	// synchronization or another such pass is already under way.
	TriggerAnnotate() bool
	// SurfaceIncomplete reports how many stages the most recently completed
	// classification pass could not classify.
	SurfaceIncomplete() int
	// RateLimit reports Wahoo's most recently advertised request quota and when
	// it next refills. ok is false until a request has carried a quota header.
	RateLimit() (remaining int, resetAt time.Time, ok bool)
}

// SyncActivityState is what the process knows about a run that has not
// finished. Its zero value says nothing is under way.
type SyncActivityState struct {
	// StartsAt is when a run being deliberately held back is due to start —
	// an initial delay rather than the ordinary wait for the next tick. Zero
	// when nothing is being held.
	StartsAt time.Time
	// Phase names the half in flight. Empty while a run has been accepted but
	// no half of it has started.
	Phase SyncPhase
	// Running is true from the moment a run is accepted until its last half has
	// finished.
	Running bool
}

// SyncFuncs adapts a pair of functions to Sync for manual wiring. An unset
// ActivityFunc reports no work under way, which is the honest answer from a
// process whose runs begin and end inside the request that asked for one.
type SyncFuncs struct {
	TriggerFunc           func(phase SyncPhase) bool
	TriggerTargetFunc     func(targetID string) bool
	TriggerClearFunc      func(targetID string) bool
	ActivityFunc          func() SyncActivityState
	TriggerAnnotateFunc   func() bool
	SurfaceIncompleteFunc func() int
	RateLimitFunc         func() (remaining int, resetAt time.Time, ok bool)
}

// Trigger starts the adapted manual synchronization.
func (f SyncFuncs) Trigger(phase SyncPhase) bool {
	return f.TriggerFunc(phase)
}

// TriggerTarget starts the adapted manual single-target reconciliation.
func (f SyncFuncs) TriggerTarget(targetID string) bool {
	return f.TriggerTargetFunc(targetID)
}

// TriggerClear starts the adapted manual single-target clear. An unset
// TriggerClearFunc refuses, so a wiring that never offered the operation
// answers as though it were already busy rather than panicking on a route
// nothing serves.
func (f SyncFuncs) TriggerClear(targetID string) bool {
	if f.TriggerClearFunc == nil {
		return false
	}

	return f.TriggerClearFunc(targetID)
}

// Activity reports the adapted process state.
func (f SyncFuncs) Activity() SyncActivityState {
	if f.ActivityFunc == nil {
		return SyncActivityState{}
	}

	return f.ActivityFunc()
}

// TriggerAnnotate starts the adapted manual classification pass. False when
// unset, the honest answer from a process with no classification pass to run.
func (f SyncFuncs) TriggerAnnotate() bool {
	if f.TriggerAnnotateFunc == nil {
		return false
	}

	return f.TriggerAnnotateFunc()
}

// SurfaceIncomplete reports the adapted process's incomplete count. Zero when
// unset, which is the honest answer from a process that tracks none.
func (f SyncFuncs) SurfaceIncomplete() int {
	if f.SurfaceIncompleteFunc == nil {
		return 0
	}

	return f.SurfaceIncompleteFunc()
}

// RateLimit reports the adapted quota. Unknown when unset, the honest answer
// from a wiring with no Wahoo client behind it.
func (f SyncFuncs) RateLimit() (remaining int, resetAt time.Time, ok bool) {
	if f.RateLimitFunc == nil {
		return 0, time.Time{}, false
	}

	return f.RateLimitFunc()
}

// Assets serves the embedded browser UI. It is declared here so this package
// stays independent of how the UI is built or embedded.
type Assets interface {
	// Index writes the application entry document.
	Index(writer http.ResponseWriter, request *http.Request)
	// Static serves one hashed, immutable build artefact.
	Static(writer http.ResponseWriter, request *http.Request)
}

// State provides only non-secret metadata and stored geometry for the read
// model. It never exposes tokens or upstream response bodies.
//
// It is composed of the concerns behind the served surface rather than written
// as one long list, so a reader can see which of them a route actually touches.
type State interface {
	TargetState
	StageState
	RunState
	ScheduleState
}

// TargetState is what is known locally about each configured Wahoo account:
// whether it is onboarded, what it was last written, and how its own last
// reconciliation ended. Every one of these is a local read — a status request
// never asks Wahoo what it holds.
type TargetState interface {
	ForEachTarget(ctx context.Context, visit func(id, authorizationState string) error) error
	// ForEachPendingAuthorization visits the slots with an authorization in
	// flight. The stored state cannot say so — it holds what a slot durably is,
	// and being midway through the browser flow is not that — so the status view
	// reads the two together.
	ForEachPendingAuthorization(ctx context.Context, visit func(targetID string) error) error
	ForEachTargetStage(ctx context.Context, targetID string, visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error) error
	ForEachTargetRun(ctx context.Context, visit func(targetID string, finishedAt time.Time, outcome, detail string) error) error
}

// StageState is the stored library: what each stage is, the revision it is held
// at, and the geometry and classification derived from it. The revision here,
// against the one in TargetState, is all convergence is derived from.
type StageState interface {
	ForEachSourceStage(ctx context.Context, visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string) error) error
	ForEachStageSummary(ctx context.Context, visit func(summary route.Summary) error) error
	StageGeometry(ctx context.Context, provider route.Provider, routeID int64, stageOrder int) (route.Summary, json.RawMessage, json.RawMessage, bool, error)
	StageSurface(ctx context.Context, provider route.Provider, routeID int64, stageOrder int, contentHash string) (json.RawMessage, float64, bool, error)
	SurfaceCoverage(ctx context.Context) (classified, total int, err error)
	RequestStageReprocess(ctx context.Context, provider route.Provider, routeID int64, stageOrder int) (found bool, err error)
}

// RunState is what the last synchronization runs recorded, in aggregate and per
// half.
type RunState interface {
	LastSyncRun(ctx context.Context) (completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int, found bool, err error)
	ForEachPhaseRun(ctx context.Context, visit func(phase string, completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error) error
	ForEachSyncRun(ctx context.Context, after string, limit int, visit func(reference, phase string, completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error) (next string, usable bool, err error)
	// LastSuccessfulPhaseCompletion returns when a phase last recorded a
	// success, which is what the trusted inventory's reported age is measured
	// against.
	LastSuccessfulPhaseCompletion(ctx context.Context, phase string) (completedAt time.Time, found bool, err error)
}

// SettingsState is the settings an operator edits while the service runs, held
// live and replaced a section at a time. It is satisfied by
// *runtimeconfig.Current, which validates before it persists, so what is read
// back here has passed the same rules startup applies.
type SettingsState interface {
	Values() runtimeconfig.Values

	// Update replaces the part of the settings one edit names. The change is
	// handed the settings as they are at the moment of the write, so two
	// sections saved at once do not each undo the other.
	Update(ctx context.Context, change func(runtimeconfig.Values) runtimeconfig.Values) error

	// SecretIsSet is all this handler is ever told about a credential. The
	// value itself is never read here, so it cannot reach a response by
	// accident.
	SecretIsSet(name runtimeconfig.SecretName) bool
	SetSecrets(ctx context.Context, secrets map[runtimeconfig.SecretName]runtimeconfig.Secret) error

	// Missing names the settings a run still needs, so the page can say what is
	// left rather than leaving an operator to find out from a failed run.
	Missing() []string
}

// ScheduleState is the pair of switches governing unattended runs.
type ScheduleState interface {
	SyncSchedule(ctx context.Context) (source, targets bool, err error)
	SetSyncSchedule(ctx context.Context, source, targets bool) error
}

// AccessVerifier proves the identity behind a Cloudflare Access assertion. It
// is satisfied by internal/cfaccess and is nil when no public path is deployed.
type AccessVerifier interface {
	// Verify returns the email address a valid assertion names.
	Verify(ctx context.Context, assertion string) (string, error)
}

// AccessVerifierFunc adapts a function to AccessVerifier.
type AccessVerifierFunc func(ctx context.Context, assertion string) (string, error)

// Verify calls f.
func (f AccessVerifierFunc) Verify(ctx context.Context, assertion string) (string, error) {
	return f(ctx, assertion)
}

// WeatherSeries is one coordinate's hourly forecast, column-oriented: index i
// across every slice describes the same hour.
type WeatherSeries struct {
	Time                            []time.Time
	TemperatureCelsius              []float64
	ApparentTemperatureCelsius      []float64
	PrecipitationMillimetres        []float64
	PrecipitationProbabilityPercent []float64
	WindSpeedKMH                    []float64
	WindDirectionDegrees            []float64
	WeatherCode                     []int
}

// Weather asks a forecast provider for an hourly series at each of a set of
// coordinates, over one shared time window. It is satisfied by
// internal/openmeteo through WeatherFunc, kept to primitive parameter and
// return types so this package never imports that adapter.
type Weather interface {
	// Forecast returns one hourly series per coordinate, in the order the
	// coordinates are given.
	Forecast(ctx context.Context, latitudes, longitudes []float64, from, to time.Time) ([]WeatherSeries, error)
}

// WeatherFunc adapts a function to Weather.
type WeatherFunc func(ctx context.Context, latitudes, longitudes []float64, from, to time.Time) ([]WeatherSeries, error)

// Forecast calls f.
func (f WeatherFunc) Forecast(ctx context.Context, latitudes, longitudes []float64, from, to time.Time) ([]WeatherSeries, error) {
	return f(ctx, latitudes, longitudes, from, to)
}
