package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	activities "github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/session"
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
	// RateLimit reports Wahoo's most recently advertised request quota, when it
	// next refills, and when it was read. ok is false until a request has carried
	// a quota header.
	RateLimit() (remaining int, resetAt, observedAt time.Time, ok bool)
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
	// NextRunAt is when the next scheduled run is due, zero for a task nothing
	// schedules and while the schedule's own attempt runs.
	NextRunAt time.Time
	Name      string
	// Interval is the gap between runs for a task on a fixed schedule, zero for
	// one with no schedule or a calendar one.
	Interval time.Duration
	// Running is how many attempts are in flight.
	Running int
	// Scheduled reports whether the task has a schedule at all.
	Scheduled bool
	// Enabled reports whether the schedule may start it. A task nobody has ruled
	// on runs.
	Enabled bool
}

// WebhookTokens verifies the shared token an inbound notification carries. The
// comparison lives behind this port because this package is never told a
// credential's value, only that one is stored.
type WebhookTokens interface {
	// VerifyWahooWebhookToken compares presented against the configured token in
	// constant time. It reports false, not an error, when none is configured.
	VerifyWahooWebhookToken(ctx context.Context, presented string) (bool, error)
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
	RateLimitFunc         func() (remaining int, resetAt, observedAt time.Time, ok bool)
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
func (f SyncFuncs) RateLimit() (remaining int, resetAt, observedAt time.Time, ok bool) {
	if f.RateLimitFunc == nil {
		return 0, time.Time{}, time.Time{}, false
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
	TaskRunState
	ActivityState
}

// ActivityState is what a poll recorded about each target's rides. The
// provider's own summary document stays in the store.
type ActivityState interface {
	// ActivitiesBetween lists one target's activities that started within the
	// half-open window [from, to), newest first, at most limit of them.
	ActivitiesBetween(ctx context.Context, targetID string, from, to time.Time, limit int) ([]activities.Stored, error)
	// ActivityTrack lists the positioned samples of one target's activity, in
	// the order they were recorded.
	ActivityTrack(ctx context.Context, targetID string, id int64) ([]activities.TrackPoint, error)
}

// TargetState is what is known locally about each self-service Wahoo target.
// Every read is local: a status request never asks Wahoo what it holds.
type TargetState interface {
	// EnsureTargetOwner creates a subject's own target if this is the first
	// time they have been seen, and leaves an existing one alone. The one
	// self-service creation point: a target's identity is its owning
	// subject's own value, so this is safe on every "Connect" attempt.
	EnsureTargetOwner(ctx context.Context, subject string) error
	// ForEachTarget visits every target, unfiltered by owner — scoping to what
	// one caller may see is this package's job, not the store's.
	ForEachTarget(ctx context.Context, visit func(id, authorizationState, ownerSubject string) error) error
	// ForEachPendingAuthorization visits the slots with an authorization in
	// flight. Stored state holds what a slot durably is, which this is not.
	ForEachPendingAuthorization(ctx context.Context, visit func(targetID string) error) error
	// TargetByWahooUser is the slot one Wahoo user authorized, and whether there
	// is one. A user this deployment does not know is not a failure: only the
	// webhook receiver asks, and it answers such a delivery as if it had acted.
	TargetByWahooUser(ctx context.Context, wahooUserID string) (targetID string, found bool, err error)
	ForEachTargetStage(ctx context.Context, targetID string, visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error) error
	ForEachTargetRun(ctx context.Context, visit func(targetID string, finishedAt time.Time, outcome, detail string) error) error
	// LatestSessionNicknames returns each subject's most recently signed-in
	// nickname, keyed by subject — a display label for an owner already known,
	// never a way to find one.
	LatestSessionNicknames(ctx context.Context) (map[string]string, error)
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
	CountStageEnrichmentFailures(ctx context.Context) (count int, err error)
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

// TaskRunState is what the background task layer recorded about its own
// attempts. Read a page at a time, because the retained history spans every
// registered task.
type TaskRunState interface {
	// ForEachTaskRunPage visits one page of recorded attempts, newest first, and
	// returns the cursor for the page after it. An empty task is every task; a
	// cursor this store did not issue is unusable rather than a failure.
	ForEachTaskRunPage(ctx context.Context, task, after string, limit int, visit func(task, argument, trigger string, startedAt, finishedAt time.Time, outcome, detail, reference string) error) (next string, usable bool, err error)
}

// StyleOrigins reports the origins the configured basemap styles name for
// their glyphs, sprites, and tiles. A style document is free to point at hosts
// its own URL does not name, and a policy that omits them leaves the reader a
// map with no labels or no streets. Satisfied by *basemap.Resolver.
type StyleOrigins interface {
	// Origins answers from what was last read, never from the network: it is
	// called while a response header is being composed.
	Origins() []string

	// Refresh reads the configured styles again, so a basemap saved on the
	// settings page is admitted by the responses that follow the save rather
	// than at the next scheduled read.
	Refresh(ctx context.Context)
}

// SettingsState is the settings an operator edits while the service runs, held
// live and replaced a section at a time. Satisfied by *runtimeconfig.Current.
type SettingsState interface {
	Values() runtimeconfig.Values

	// Update replaces the part of the settings one edit names. The change is
	// handed the settings as they are at the moment of the write, so two
	// sections saved at once do not each undo the other.
	Update(ctx context.Context, change func(runtimeconfig.Values) runtimeconfig.Values) error
	UpdateWithSecrets(ctx context.Context, change func(runtimeconfig.Values) runtimeconfig.Values, secrets map[runtimeconfig.SecretName]runtimeconfig.Secret) error

	// SecretIsSet is all this handler is ever told about a credential. The
	// value itself is never read here, so it cannot reach a response by
	// accident.
	SecretIsSet(name runtimeconfig.SecretName) bool

	// Missing names the settings a run still needs, so the page can say what is
	// left rather than leaving an operator to find out from a failed run.
	Missing() []string
}

// Sessions is who is signed in, as this surface needs it; *session.Service
// satisfies it.
type Sessions interface {
	// Verify admits a caller.
	Verify(ctx context.Context, token string) (session.Identity, error)
	Begin(ctx context.Context) (session.Login, error)
	Complete(ctx context.Context, state, cookieState, code string) (session.Completion, error)
	Revoke(ctx context.Context, token string) error
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
	CloudCoverPercent               []float64
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

// WeatherGrid relays Open-Meteo's spatial data files to whoever asks, so the
// browser's own reader never reaches a third party directly. It is satisfied
// by internal/openmeteogrid's concrete client, which returns *http.Response —
// a stdlib type, so this package needs no adapter-specific type to declare it
// and no Func wrapper to adapt one, the way Weather needs WeatherFunc. The
// caller closes the returned response's body.
type WeatherGrid interface {
	// Latest returns the model's own capture manifest.
	Latest(ctx context.Context) (*http.Response, error)
	// Object returns one .om file's bytes, or answers a HEAD, for the run
	// named by referenceTime and the hour named by validTime. rangeHeader is
	// forwarded verbatim when non-empty. method must be GET or HEAD.
	Object(ctx context.Context, referenceTime, validTime time.Time, method, rangeHeader string) (*http.Response, error)
}
