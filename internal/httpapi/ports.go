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
// both. Declared here so this package knows nothing about the implementation.
type SyncPhase string

const (
	// SyncPhaseAll reads the source and then writes to the targets.
	SyncPhaseAll SyncPhase = "all"
	// SyncPhaseSource reads the source library into stored state.
	SyncPhaseSource SyncPhase = "source"
	// SyncPhaseTargets reconciles stored state onto the targets.
	SyncPhaseTargets SyncPhase = "targets"
)

// Sync is what only the synchronization process can answer about itself. What
// starts a run is the task layer's; this is what has not finished, which a run
// that has not finished has recorded nowhere else.
type Sync interface {
	// Activity reports the run that has not finished, if there is one.
	Activity() SyncActivityState
	// SurfaceIncomplete reports how many stages the most recently completed
	// classification pass could not classify.
	SurfaceIncomplete() int
	// RateLimit reports Wahoo's most recently advertised request quota and when
	// it next refills. ok is false until a request has carried a quota header.
	RateLimit() (remaining int, resetAt time.Time, ok bool)
}

// Tasks is the background activities as this surface needs them: what this
// build registers, and a way to start one.
type Tasks interface {
	// Registered lists every task, in a stable order, with what is known about
	// each right now.
	Registered() []RegisteredTask
	// Run starts one attempt and reports whether it was accepted. It takes no
	// context: an accepted attempt outlives the request that asked for it, and a
	// request context is cancelled the moment its handler returns.
	Run(name, argument string) bool
	// Schedule records whether the schedule may start one task. It governs
	// unattended runs only: a task switched off still runs when asked for.
	Schedule(ctx context.Context, name string, enabled bool) error
}

// RegisteredTask is one background activity and what is true of it now.
type RegisteredTask struct {
	// NextRunAt is when the first scheduled run is due, zero once it has started
	// and for a task nothing schedules.
	NextRunAt time.Time
	Name      string
	// Running is how many attempts are in flight.
	Running int
	// Scheduled reports whether the task has a schedule at all.
	Scheduled bool
	// Enabled reports whether the schedule may start it. A task nobody has ruled
	// on runs.
	Enabled bool
}

// Alerts is the alert matrix as this surface needs it: what this service can
// raise, what an operator has decided about each, and a way to decide.
type Alerts interface {
	// Catalogue lists every alert this service can raise, in a stable order,
	// with what has been decided about each.
	Catalogue() []AlertSetting
	// Decide records what an operator decided. An alert left out keeps whatever
	// it had, because deciding is what creates a record.
	Decide(ctx context.Context, decisions []AlertDecision) error
}

// AlertSetting is one alert and what is currently true of it.
type AlertSetting struct {
	Task    string
	Alert   string
	Enabled bool
	Decided bool
}

// AlertDecision is one alert an operator has ruled on.
type AlertDecision struct {
	Task    string
	Alert   string
	Enabled bool
}

// SyncActivityState is what the process knows about a run that has not
// finished. Its zero value says nothing is under way.
type SyncActivityState struct {
	// StartsAt is when a run being held back by the initial delay is due. Zero
	// when nothing is being held.
	StartsAt time.Time
	// Phase names the half in flight. Empty while a run has been accepted but
	// no half of it has started.
	Phase SyncPhase
	// Running is true from the moment a run is accepted until its last half has
	// finished.
	Running bool
}

// SyncFuncs adapts functions to Sync for manual wiring. An unset ActivityFunc
// reports no work under way.
type SyncFuncs struct {
	ActivityFunc          func() SyncActivityState
	SurfaceIncompleteFunc func() int
	RateLimitFunc         func() (remaining int, resetAt time.Time, ok bool)
}

// Activity reports the adapted process state.
func (f SyncFuncs) Activity() SyncActivityState {
	if f.ActivityFunc == nil {
		return SyncActivityState{}
	}

	return f.ActivityFunc()
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
type State interface {
	TargetState
	StageState
	RunState
	ScheduleState
}

// TargetState is what is known locally about each configured Wahoo account.
// Every read is local: a status request never asks Wahoo what it holds.
type TargetState interface {
	ForEachTarget(ctx context.Context, visit func(id, authorizationState string) error) error
	// ForEachPendingAuthorization visits the slots with an authorization in
	// flight. Stored state holds what a slot durably is, which this is not.
	ForEachPendingAuthorization(ctx context.Context, visit func(targetID string) error) error
	ForEachTargetStage(ctx context.Context, targetID string, visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error) error
	ForEachTargetRun(ctx context.Context, visit func(targetID string, finishedAt time.Time, outcome, detail string) error) error
}

// StageState is the stored library: what each stage is, its revision, and what
// is derived from it. That revision against TargetState's is all convergence is.
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
	// LastSuccessfulPhaseCompletion returns when a phase last recorded a success,
	// which the trusted inventory's reported age is measured against.
	LastSuccessfulPhaseCompletion(ctx context.Context, phase string) (completedAt time.Time, found bool, err error)
}

// SettingsState is the settings an operator edits while the service runs, held
// live and replaced a section at a time. Satisfied by *runtimeconfig.Current.
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
