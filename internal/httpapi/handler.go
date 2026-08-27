// Package httpapi owns identity-gated JSON, OAuth, and browser UI HTTP handling.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
)

// assertionHeader carries the signed Cloudflare Access token. It is the only
// identity this service accepts.
//
// Tailscale Serve still fronts the listener, because cloudflared reaches it by
// Service name, but it authenticates as a tagged device and so carries no
// Tailnet identity. Deliberately absent is any handling of Tailscale-User-Login:
// Serve remains reachable by Tailnet members directly, and honouring that header
// would leave a second front door with a second identity source behind it.
const assertionHeader = "Cf-Access-Jwt-Assertion"

// maximumRequestBytes bounds the only request bodies this service reads. They
// carry two booleans, so anything larger is a mistake or an attempt.
const maximumRequestBytes = 1 << 10

const (
	cacheAPI       = "no-store"
	cacheDocument  = "no-cache"
	cacheImmutable = "public, max-age=31536000, immutable"
)

// The shapes a build stamp may have before this package will serve it. They are
// restated here rather than imported because this is the layer that publishes
// them: whatever produced the value, only a full commit object name and a
// `sha256:` digest leave the service.
const (
	revisionLength = 40
	digestPrefix   = "sha256:"
	digestLength   = 64
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
	ForEachTarget(ctx context.Context, visit func(id, authorization string) error) error
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

// Options carries the non-secret settings the HTTP surface needs.
type Options struct {
	// AccessVerifier checks the Cloudflare Access assertion on every request.
	// It is required: without it the service has no gate at all.
	AccessVerifier AccessVerifier

	// SurfaceIndexFunc reports the map build classifications are currently being
	// read from. It answers false when surface classification is switched off,
	// and while a first index is still being built.
	//
	// It asks the live index rather than the state file on purpose: what the
	// status page should say is what is loaded, and a recorded build whose file
	// did not survive a restart is exactly the case where the two differ.
	SurfaceIndexFunc func() (generation string, builtAt time.Time, ok bool)

	// SourceBaseURLs are each configured source's own web application, as the
	// operator configured it, keyed by the provider it belongs to. Where the
	// page also knows that provider's route path, it builds an outbound link
	// to a stage's source route from the matching entry, so an operator can
	// open the route a stage was made from without hunting for it by name —
	// today that is VeloPlanner alone; a provider whose path the page does not
	// yet know offers no link even though its base URL is here.
	//
	// A provider with nothing configured for it is simply absent from the
	// map, rather than present with an empty value.
	SourceBaseURLs map[route.Provider]string

	// RideModelValidation is the loaded coefficient profile's measured
	// unseen-route error, when a profile is configured and its file carries
	// one. It describes the profile as a whole, so every stage response
	// carries the same value rather than one derived per stage. Nil when no
	// profile is configured or the loaded file predates #217's validation
	// fields.
	RideModelValidation *RideModelValidation

	// BuildRevision is the public source commit this binary was built from, and
	// BuildImageDigest the immutable digest of the image running it. Both are
	// optional: a local build knows neither, and the status response then says
	// so rather than naming something it cannot stand behind.
	//
	// Each is published only when it is what it claims to be — a full commit
	// object name, and a `sha256:` digest — because this is the boundary that
	// serves them, and a malformed value here would become a link to nowhere in
	// a browser.
	BuildRevision string

	// BuildImageDigest is the digest alone. Whatever registry and repository the
	// host pulls it from is deployment topology and stays on the host.
	BuildImageDigest string

	// AccessEmail is the one address an Access assertion may name, and the
	// principal every authenticated request resolves to.
	AccessEmail string

	// AccessSignOutURL ends the session, and is served by whatever stands in
	// front of this service rather than by this service. Only a deployment
	// knows whether anything does, so it is named at the composition root and
	// left empty everywhere nothing would answer it — the page then offers no
	// way out rather than a link to a 404.
	AccessSignOutURL string

	// BrowserOriginURL is an absolute HTTPS URL on the hostname a browser
	// reaches this service at. Only its scheme and host are read: together they
	// are the one origin a state-changing request may come from. The Wahoo
	// redirect URL is that hostname by construction — it is where a browser
	// returns from Wahoo — which is why it is what the composition root passes.
	BrowserOriginURL string

	// Basemaps are the cartographies the page may switch the map between, in
	// the order they are offered. At least one is required. Each entry's dark
	// style, where it has one, must be on that entry's own origin.
	Basemaps []Basemap

	TargetIDs []string

	// SourceStaleAfter bounds how long the trusted source inventory may go
	// without a successful refresh before the status response reports it as
	// stale. Optional: zero omits trusted-inventory freshness from the response
	// entirely, which is what a caller supplying no bound gets.
	SourceStaleAfter time.Duration
}

// RideModelValidation is the frozen coefficient profile's measured
// unseen-route error, from the same route-disjoint benchmark that froze it.
type RideModelValidation struct {
	BiasPercent    float64
	MAEPercent     float64
	P90Percent     float64
	EvaluatedRides int
}

// Basemap is one cartography the page may load, as the operator configured it.
type Basemap struct {
	// Name labels the entry in the page's picker and is the identity a browser
	// remembers its choice by. Required, and unique across the list.
	Name string

	// StyleURL is the MapLibre style document. Absolute HTTPS; its origin joins
	// the Content-Security-Policy.
	StyleURL string

	// StyleURLDark is loaded instead under a dark system colour scheme.
	// Optional, and on StyleURL's origin when set.
	StyleURLDark string

	// DarkCartography marks ground that is dark in either colour scheme, such
	// as satellite imagery. The page paints its route ink to match.
	//
	// It contradicts StyleURLDark: a provider publishing a dark twin has light
	// cartography to switch away from. Configuring both is refused.
	DarkCartography bool
}

// Handler enforces Cloudflare Access identity and exposes the v1 HTTP surface.
type Handler struct {
	oauth    OAuth
	syncRuns Sync
	state    State
	assets   Assets
	weather  Weather
	// validate holds every request to the document before it reaches a
	// handler: parameter bounds, request bodies, and provenance.
	validate            func(http.Handler) http.Handler
	accessVerifier      AccessVerifier
	surfaceIndex        func() (string, time.Time, bool)
	now                 func() time.Time
	mux                 *http.ServeMux
	sourceBaseURLs      map[route.Provider]string
	rideModelValidation *RideModelValidation
	buildRevision       string
	buildImageDigest    string
	browserOrigin       string
	allowedEmail        string
	signOutURL          string
	tileOrigins         []string
	targetIDs           []string
	basemaps            []Basemap
	sourceStaleAfter    time.Duration
}

// New creates a handler. Health checks are intentionally unauthenticated;
// deployment must keep the listener private to the local Tailscale proxy.
func New(
	options *Options,
	oauthService OAuth,
	state State,
	syncRuns Sync,
	assets Assets,
	weather Weather,
) (*Handler, error) {
	if options == nil || oauthService == nil || state == nil || syncRuns == nil || assets == nil || weather == nil {
		return nil, errors.New("http options, oauth service, state, sync process, assets, and weather are required")
	}
	if len(options.TargetIDs) < 1 || len(options.TargetIDs) > 2 {
		return nil, errors.New("between one and two target IDs are required")
	}
	for index, targetID := range options.TargetIDs {
		if strings.TrimSpace(targetID) == "" {
			return nil, errors.New("target IDs must not be empty")
		}
		if slices.Contains(options.TargetIDs[:index], targetID) {
			return nil, errors.New("target IDs must be unique")
		}
	}
	if options.AccessVerifier == nil {
		return nil, errors.New("an access verifier is required")
	}
	if strings.TrimSpace(options.AccessEmail) == "" {
		return nil, errors.New("an access email is required")
	}
	browserOrigin, err := browserOriginOf(options.BrowserOriginURL)
	if err != nil {
		return nil, err
	}
	// Checked here as well as in the configuration, for the reason the target
	// IDs above are: this struct's own documentation promises it, and the name
	// is the identity a browser remembers a reader's choice by. Two entries
	// sharing one would make that memory ambiguous.
	for index, basemap := range options.Basemaps {
		if strings.TrimSpace(basemap.Name) == "" {
			return nil, errors.New("basemap names must not be empty")
		}
		if slices.ContainsFunc(options.Basemaps[:index], func(earlier Basemap) bool {
			return earlier.Name == basemap.Name
		}) {
			return nil, errors.New("basemap names must be unique")
		}
		if basemap.DarkCartography && basemap.StyleURLDark != "" {
			return nil, errors.New("a basemap must not set both dark cartography and a dark style")
		}
	}
	tileOrigins, err := tileOriginsOf(options.Basemaps)
	if err != nil {
		return nil, err
	}

	// Validated here rather than trusted, because it leaves the service as a
	// link a browser will follow. A configured value that cannot be one is a
	// mistake worth refusing at startup; the absent case is the supported one.
	// Trimmed before validating and then stored trimmed, so the value the page
	// receives is the one that was checked: surrounding whitespace survives a
	// hand-edited config file, and a browser will not parse it back into a URL.
	sourceBaseURLs := make(map[route.Provider]string, len(options.SourceBaseURLs))
	for provider, value := range options.SourceBaseURLs {
		if provider != route.ProviderVeloPlanner && provider != route.ProviderKomoot {
			return nil, fmt.Errorf("source base URL provider %q is not in the HTTP contract", provider)
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if err := validateSourceBaseURL(trimmed); err != nil {
			return nil, fmt.Errorf("source base URL for %s: %w", provider, err)
		}
		sourceBaseURLs[provider] = trimmed
	}

	// Copied rather than stored by reference, on the same terms as Basemaps
	// and TargetIDs below: the handler serves concurrently, and a caller
	// that mutated its own Options value after New returned — even
	// inadvertently, in a test — must not be able to race the handler that
	// reads it on every request.
	var rideModelValidation *RideModelValidation
	if options.RideModelValidation != nil {
		copied := *options.RideModelValidation
		rideModelValidation = &copied
	}

	handler := &Handler{
		mux:                 http.NewServeMux(),
		oauth:               oauthService,
		state:               state,
		syncRuns:            syncRuns,
		assets:              assets,
		weather:             weather,
		basemaps:            append([]Basemap(nil), options.Basemaps...),
		sourceBaseURLs:      sourceBaseURLs,
		buildRevision:       publishableRevision(options.BuildRevision),
		buildImageDigest:    publishableDigest(options.BuildImageDigest),
		tileOrigins:         tileOrigins,
		browserOrigin:       browserOrigin,
		targetIDs:           append([]string(nil), options.TargetIDs...),
		surfaceIndex:        options.SurfaceIndexFunc,
		sourceStaleAfter:    options.SourceStaleAfter,
		rideModelValidation: rideModelValidation,
		now:                 time.Now,

		accessVerifier: options.AccessVerifier,
		allowedEmail:   strings.TrimSpace(options.AccessEmail),
		signOutURL:     strings.TrimSpace(options.AccessSignOutURL),
	}
	if err := handler.useContractValidation(); err != nil {
		return nil, err
	}
	handler.routes()

	return handler, nil
}

// routes registers the API and browser surfaces. The identity gate sits outside
// both, while the document validator applies only to API and OAuth requests.
func (h *Handler) routes() {
	h.mux.HandleFunc("GET /healthz", h.GetHealth)
	h.mux.HandleFunc("GET /v1/status", h.GetStatus)
	h.mux.HandleFunc("POST /v1/sync", h.TriggerSync)
	h.mux.HandleFunc("POST /v1/sync/source", h.TriggerSourceSync)
	h.mux.HandleFunc("POST /v1/sync/targets", h.TriggerTargetsSync)
	h.mux.HandleFunc("POST /v1/sync/targets/{target}", h.TriggerTargetSync)
	h.mux.HandleFunc("POST /v1/targets/{target}/clear", h.ClearTarget)
	h.mux.HandleFunc("POST /v1/sync/surface", h.TriggerSurfaceSync)
	h.mux.HandleFunc("PUT /v1/sync/schedule", h.SetSyncSchedule)
	h.mux.HandleFunc("GET /v1/sync/runs", h.GetSyncRuns)
	h.mux.HandleFunc("GET /v1/routes", h.GetRoutes)
	h.mux.HandleFunc("GET /v1/providers/{provider}/routes/{routeId}/stages/{stage}", h.GetRoute)
	h.mux.HandleFunc("GET /v1/providers/{provider}/routes/{routeId}/stages/{stage}/geometry", h.GetRouteGeometry)
	h.mux.HandleFunc("POST /v1/providers/{provider}/routes/{routeId}/stages/{stage}/reprocess", h.ReprocessRoute)
	h.mux.HandleFunc("GET /v1/routes/{routeId}/stages/{stage}", h.RedirectLegacyRoute)
	h.mux.HandleFunc("GET /v1/routes/{routeId}/stages/{stage}/geometry", h.RedirectLegacyGeometry)
	h.mux.HandleFunc("POST /v1/routes/{routeId}/stages/{stage}/reprocess", h.RedirectLegacyReprocess)
	h.mux.HandleFunc("GET /v1/webui/config", h.GetWebUIConfig)
	h.mux.HandleFunc("GET /v1/weather", h.GetWeather)
	h.mux.HandleFunc("GET /oauth/wahoo/start/{target}", h.StartOAuth)
	h.mux.HandleFunc("GET /oauth/wahoo/callback", h.CompleteOAuth)
	h.mux.HandleFunc("GET /assets/{asset}", h.GetAsset)
	h.mux.HandleFunc("GET /favicon.svg", h.GetFavicon)
	h.mux.HandleFunc("GET /icon-256.png", h.GetIcon256)
	h.mux.HandleFunc("GET /icon-512.png", h.GetIcon512)
	h.mux.HandleFunc("GET /manifest.webmanifest", h.GetManifest)
	h.mux.HandleFunc("GET /{$}", h.GetIndex)
	h.mux.HandleFunc("GET /routes/{provider}/{routeId}/{stage}", h.GetRoutePage)
	h.mux.HandleFunc("GET /routes/{routeId}/{stage}", h.RedirectLegacyRoutePage)
	h.mux.HandleFunc("GET /sync", h.GetSyncPage)
	h.mux.HandleFunc("GET /settings", h.GetSettingsPage)
	// Browser routes are deliberately explicit: they are application navigation
	// and assets, not OpenAPI operations. This remains separate because
	// ServeMux has no pattern for the unmatched-path fallback.
	h.mux.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		h.notFound(writer)
	})
}

// ServeHTTP applies the shared response headers and dispatches.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	header := writer.Header()
	header.Set("Content-Security-Policy", h.contentSecurityPolicy())
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", cacheAPI)
	// HEAD as well as GET: Go's "GET /healthz" pattern answers both, and a
	// liveness probe that sends HEAD must not be told it needs an identity.
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) &&
		request.URL.Path == "/healthz" {
		h.mux.ServeHTTP(writer, request)

		return
	}
	if strings.HasPrefix(request.URL.Path, "/v1/") || strings.HasPrefix(request.URL.Path, "/oauth/") {
		h.gated(h.bounded(h.validate(h.mux))).ServeHTTP(writer, request)

		return
	}
	h.gated(h.mux).ServeHTTP(writer, request)
}

// bounded caps the body before the validator reads it. The validator reads a
// declared request body whole in order to check it against its schema, and does
// so without a limit of its own, so the cap has to be in place before it runs
// rather than in the one handler that decodes a body.
func (h *Handler) bounded(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Body != nil && request.Body != http.NoBody {
			request.Body = http.MaxBytesReader(writer, request.Body, maximumRequestBytes)
		}
		next.ServeHTTP(writer, request)
	})
}

// gated rejects any caller that is not the single configured identity.
//
// Requests arrive through Cloudflare Access and cloudflared, which runs on a
// tagged node, so Serve injects no identity header and the signed Access
// assertion is the only identity a request carries. It is verified here on
// every request rather than assumed to have been checked upstream.
//
// Tailscale Serve still fronts this listener and Tailnet members can still
// reach it, so there is deliberately no Tailscale-User-Login branch: it would
// be a second front door, and a forgeable one, since a tunnel forwards client
// headers verbatim.
func (h *Handler) gated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		assertion := request.Header.Get(assertionHeader)
		if assertion == "" {
			h.error(writer, http.StatusUnauthorized, "unauthorized", "an authenticated identity is required")

			return
		}

		email, err := h.accessVerifier.Verify(request.Context(), assertion)
		if err != nil {
			// The reason stays here. Telling an unauthenticated caller why its
			// assertion failed describes the check it has to defeat.
			h.error(writer, http.StatusUnauthorized, "unauthorized", "an authenticated identity is required")

			return
		}
		if !strings.EqualFold(email, h.allowedEmail) {
			h.error(writer, http.StatusForbidden, "forbidden", "identity is not permitted")

			return
		}

		// The asserted address is proven and then dropped: it differs from the
		// configured one only in case, and the routes that need an identity
		// read h.allowedEmail. OAuth state is bound to that one spelling, so a
		// flow started in one request stays consumable by the next.
		next.ServeHTTP(writer, request)
	})
}

// contentSecurityPolicy confines the page to this service's own origin plus the
// origin of every configured basemap. An entry's light and dark styles share an
// origin, so the list is as long as the number of distinct providers offered and
// no longer.
//
// Naming more than one is what makes a switchable map possible, and it is worth
// being exact about what it costs. The policy says which origins the page may
// reach; it does not make the page reach them. Only the basemap on screen is
// ever requested, so what a single provider learns — the area of a viewed route
// — is unchanged. What grew is the set of providers that could be asked, and
// that set is exactly the one the operator wrote down.
//
// Three allowances are deliberate, and each was confirmed against a real
// MapLibre render rather than assumed:
//   - worker-src needs 'self' because MapLibre loads its worker from a bundled
//     same-origin module, and blob: because it also spawns blob workers;
//   - style-src needs 'unsafe-inline' because MapLibre styles its own controls
//     inline;
//   - img-src and connect-src need the tile origins for sprites, glyphs, and
//     tiles.
func (h *Handler) contentSecurityPolicy() string {
	return strings.Join([]string{
		"default-src 'self'",
		"base-uri 'none'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		"form-action 'none'",
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"font-src 'self'",
		"worker-src 'self' blob:",
		"child-src 'self' blob:",
		"img-src 'self' data: blob: " + strings.Join(h.tileOrigins, " "),
		"connect-src 'self' " + strings.Join(h.tileOrigins, " "),
	}, "; ")
}

// GetHealth answers the liveness probe: this process is answering HTTP. It
// reads nothing, so it stays outside the identity gate.
func (h *Handler) GetHealth(writer http.ResponseWriter, _ *http.Request) {
	h.writeJSON(writer, http.StatusOK, openapi.Health{Status: "ok"})
}

func (h *Handler) error(writer http.ResponseWriter, status int, code, message string) {
	body := openapi.Error{}
	body.Error.Code, body.Error.Message = code, message
	h.writeJSON(writer, status, body)
}

func (h *Handler) writeJSON(writer http.ResponseWriter, status int, value any) {
	if writer.Header().Get("Content-Type") == "" {
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		return
	}
}

// browserOriginOf reduces the URL a browser reaches this service at to the
// origin a browser would name in an Origin header: a lowercase scheme and host,
// without the port when it is HTTPS's default, and nothing after the host.
func browserOriginOf(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("browser origin URL must be an absolute HTTPS URL")
	}

	return "https://" + strings.TrimSuffix(strings.ToLower(parsed.Host), ":443"), nil
}

// publishableRevision returns the commit object name this build may claim, or
// empty. Dropped rather than refused: a binary that reports no revision still
// works, whereas one that refuses to start over a build stamp is a service an
// operator loses for a reason that has nothing to do with cycling.
func publishableRevision(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != revisionLength || !isLowerHex(trimmed) {
		return ""
	}

	return trimmed
}

// publishableDigest returns the image digest this build may claim, or empty. A
// reference with a registry and repository still in front of it is refused here
// rather than trimmed: the composition root is where a deployment reference is
// read, and this layer serving one would mean the topology had already reached
// a response body once.
func publishableDigest(value string) string {
	trimmed := strings.TrimSpace(value)
	hex, found := strings.CutPrefix(trimmed, digestPrefix)
	if !found || len(hex) != digestLength || !isLowerHex(hex) {
		return ""
	}

	return trimmed
}

// isLowerHex reports whether value is a non-empty run of lowercase hex digits,
// which is the shape both a commit object name and a digest have to have.
func isLowerHex(value string) bool {
	if value == "" {
		return false
	}
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
		case character >= 'a' && character <= 'f':
		default:
			return false
		}
	}

	return true
}

// validateSourceBaseURL checks a provider base URL before it is handed to the
// browser. It is stricter than originOf, and deliberately so: this value is
// echoed in a response rather than only compared against another, so anything
// riding on it is observable. Credentials would be a secret in a JSON body, and
// a query or fragment would be something the operator's configuration sends to
// the provider on every visit. A path prefix is allowed — a provider may be
// hosted under one — and nothing else is.
func validateSourceBaseURL(value string) error {
	invalid := errors.New("source base URL must be an absolute HTTPS URL without credentials, query, or fragment")

	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.Opaque != "" {
		return invalid
	}

	return nil
}

// tileOriginsOf reduces the configured basemaps to the distinct origins the page
// is allowed to reach, sorted so the header a deployment sends does not depend
// on the order the entries happen to be written in.
//
// A dark style is held to its own entry's origin here as well as in the
// configuration, because a style admitted by neither the light entry's source
// nor its own would be served to the page and then blocked by the header it was
// served with. Refusing it at construction makes that a startup error rather
// than a map that goes blank after dark.
func tileOriginsOf(basemaps []Basemap) ([]string, error) {
	if len(basemaps) == 0 {
		return nil, errors.New("at least one basemap is required")
	}

	origins := make([]string, 0, len(basemaps))
	for _, basemap := range basemaps {
		origin, err := originOf(basemap.StyleURL)
		if err != nil {
			return nil, err
		}
		if basemap.StyleURLDark != "" {
			darkOrigin, darkErr := originOf(basemap.StyleURLDark)
			if darkErr != nil || darkOrigin != origin {
				return nil, errors.New("dark tile style URL must be on its basemap's style URL origin")
			}
		}
		origins = append(origins, origin)
	}
	slices.Sort(origins)

	return slices.Compact(origins), nil
}

// originOf reduces a URL to its scheme and host for use in a CSP source list.
func originOf(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("tile style URL must be an absolute HTTPS URL")
	}

	// Lowercased because a host is not case-sensitive, matching the provenance
	// comparison in the configuration layer: a dark style differing from its
	// light counterpart only by host case is the same origin a browser would
	// use, and must not fail here after passing validation there.
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}
