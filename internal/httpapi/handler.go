// Package httpapi owns identity-gated JSON, OAuth, and browser UI HTTP handling.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

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

// SyncTrigger starts one manual synchronization and reports whether it was
// accepted. An accepted run continues independently of the HTTP request.
type SyncTrigger interface {
	Trigger(phase SyncPhase) bool
}

// SyncTriggerFunc adapts a function to SyncTrigger for manual wiring.
type SyncTriggerFunc func(phase SyncPhase) bool

// Trigger starts the adapted manual synchronization.
func (f SyncTriggerFunc) Trigger(phase SyncPhase) bool {
	return f(phase)
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
	ForEachTarget(ctx context.Context, visit func(id, authorization string) error) error
	ForEachStageSummary(ctx context.Context, visit func(summary route.Summary) error) error
	StageGeometry(ctx context.Context, routeID int64, stageOrder int) (route.Summary, json.RawMessage, bool, error)
	StageSurface(ctx context.Context, routeID int64, stageOrder int, contentHash string) (json.RawMessage, float64, bool, error)
	LastSyncRun(ctx context.Context) (completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int, found bool, err error)
	ForEachPhaseRun(ctx context.Context, visit func(phase string, completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error) error
	SurfaceCoverage(ctx context.Context) (classified, total int, err error)
	SyncSchedule(ctx context.Context) (source, targets bool, err error)
	SetSyncSchedule(ctx context.Context, source, targets bool) error
	RequestStageReprocess(ctx context.Context, routeID int64, stageOrder int) (found bool, err error)
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

// Options carries the non-secret settings the HTTP surface needs.
type Options struct {
	// AccessVerifier checks the Cloudflare Access assertion on every request.
	// It is required: without it the service has no gate at all.
	AccessVerifier AccessVerifier

	TileStyleURL string

	// TileStyleURLDark is the style the page loads instead under a dark system
	// colour scheme. It is optional, and must be on TileStyleURL's origin.
	TileStyleURLDark string

	// SourceBaseURL is the provider's own web application, as the operator
	// configured it. The page builds an outbound link to a stage's source route
	// from it, so an operator can open the route this stage was made from
	// without hunting for it by name.
	//
	// Optional: without it the page shows no such link rather than a broken one.
	SourceBaseURL string

	// AccessEmail is the one address an Access assertion may name, and the
	// principal every authenticated request resolves to.
	AccessEmail string

	// BrowserOriginURL is an absolute HTTPS URL on the hostname a browser
	// reaches this service at. Only its scheme and host are read: together they
	// are the one origin a state-changing request may come from. The Wahoo
	// redirect URL is that hostname by construction — it is where a browser
	// returns from Wahoo — which is why it is what the composition root passes.
	BrowserOriginURL string

	TargetIDs []string
}

// Handler enforces Cloudflare Access identity and exposes the v1 HTTP surface.
type Handler struct {
	mux              *http.ServeMux
	oauth            OAuth
	syncTrigger      SyncTrigger
	state            State
	assets           Assets
	accessVerifier   AccessVerifier
	tileStyleURL     string
	tileStyleURLDark string
	sourceBaseURL    string
	tileOrigin       string
	browserOrigin    string
	allowedEmail     string
	targetIDs        []string
}

// New creates a handler. Health checks are intentionally unauthenticated;
// deployment must keep the listener private to the local Tailscale proxy.
func New(
	options *Options,
	oauthService OAuth,
	state State,
	syncTrigger SyncTrigger,
	assets Assets,
) (*Handler, error) {
	if options == nil || oauthService == nil || state == nil || syncTrigger == nil || assets == nil {
		return nil, errors.New("http options, oauth service, state, sync trigger, and assets are required")
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
	tileOrigin, err := originOf(options.TileStyleURL)
	if err != nil {
		return nil, err
	}
	// The dark style is admitted by the same Content-Security-Policy source as
	// the light one, so a style on another origin would be served to the page
	// and then blocked by the header it was served with. Refuse it here instead.
	if options.TileStyleURLDark != "" {
		darkOrigin, darkErr := originOf(options.TileStyleURLDark)
		if darkErr != nil || darkOrigin != tileOrigin {
			return nil, errors.New("dark tile style URL must be on the tile style URL's origin")
		}
	}

	// Validated here rather than trusted, because it leaves the service as a
	// link a browser will follow. A configured value that cannot be one is a
	// mistake worth refusing at startup; the absent case is the supported one.
	// Trimmed before validating and then stored trimmed, so the value the page
	// receives is the one that was checked: surrounding whitespace survives a
	// hand-edited config file, and a browser will not parse it back into a URL.
	sourceBaseURL := strings.TrimSpace(options.SourceBaseURL)
	if sourceBaseURL != "" {
		if _, sourceErr := originOf(sourceBaseURL); sourceErr != nil {
			return nil, errors.New("source base URL must be an absolute HTTPS URL")
		}
	}

	handler := &Handler{
		mux:              http.NewServeMux(),
		oauth:            oauthService,
		state:            state,
		syncTrigger:      syncTrigger,
		assets:           assets,
		tileStyleURL:     options.TileStyleURL,
		tileStyleURLDark: options.TileStyleURLDark,
		sourceBaseURL:    sourceBaseURL,
		tileOrigin:       tileOrigin,
		browserOrigin:    browserOrigin,
		targetIDs:        append([]string(nil), options.TargetIDs...),

		accessVerifier: options.AccessVerifier,
		allowedEmail:   strings.TrimSpace(options.AccessEmail),
	}
	handler.routes()

	return handler, nil
}

// routes registers the fixed v1 surface. Every pattern except the liveness
// probe is wrapped by the Access identity gate, and every pattern that triggers
// a run or writes state is additionally wrapped by the provenance check.
func (h *Handler) routes() {
	h.mux.HandleFunc("GET /healthz", h.health)

	h.mux.Handle("GET /v1/status", h.gated(h.status))
	h.mux.Handle("POST /v1/sync", h.gated(h.sameOrigin(h.sync)))
	h.mux.Handle("POST /v1/sync/source", h.gated(h.sameOrigin(h.syncSource)))
	h.mux.Handle("POST /v1/sync/targets", h.gated(h.sameOrigin(h.syncTargets)))
	h.mux.Handle("PUT /v1/sync/schedule", h.gated(h.sameOrigin(h.setSyncSchedule)))
	h.mux.Handle("GET /v1/routes", h.gated(h.stages))
	h.mux.Handle("GET /v1/routes/{routeID}/stages/{stage}", h.gated(h.stage))
	h.mux.Handle("GET /v1/routes/{routeID}/stages/{stage}/geometry", h.gated(h.stageGeometry))
	h.mux.Handle(
		"POST /v1/routes/{routeID}/stages/{stage}/reprocess",
		h.gated(h.sameOrigin(h.reprocessStage)),
	)
	h.mux.Handle("GET /v1/webui/config", h.gated(h.webUIConfig))

	h.mux.Handle("GET /oauth/wahoo/start/{target}", h.gated(h.start))
	h.mux.Handle("GET /oauth/wahoo/callback", h.gated(h.callback))

	h.mux.Handle("GET /assets/", h.gated(h.staticAsset))
	h.mux.Handle("GET /favicon.svg", h.gated(h.staticAsset))
	h.mux.Handle("GET /{$}", h.gated(h.index))
	h.mux.Handle("GET /routes/{routeID}/{stage}", h.gated(h.index))

	// Unknown paths still answer as JSON, and still require the identity, so an
	// unauthenticated caller cannot probe which paths exist.
	h.mux.Handle("/", h.gated(func(writer http.ResponseWriter, _ *http.Request, _ string) {
		h.error(writer, http.StatusNotFound, "not_found", "resource was not found")
	}))
}

// ServeHTTP applies the shared response headers and dispatches.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	header := writer.Header()
	header.Set("Content-Security-Policy", h.contentSecurityPolicy())
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", cacheAPI)
	h.mux.ServeHTTP(writer, request)
}

// gatedFunc is a handler that has already proven the caller's identity. The
// principal is the single configured address every request resolves to.
type gatedFunc func(writer http.ResponseWriter, request *http.Request, principal string)

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
func (h *Handler) gated(next gatedFunc) http.Handler {
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

		// The configured address, not the asserted one, is what goes downstream.
		// They differ only in case, and OAuth state is bound to this string, so
		// resolving to one spelling keeps a flow started in one request
		// consumable by the next.
		next(writer, request, h.allowedEmail)
	})
}

// sameOrigin refuses a state-changing request that did not come from this
// service's own browser UI. Every route that starts a run or writes state must
// be wrapped in it.
//
// The identity gate proves who is calling; it does not prove that they meant to
// call. An Access session lives in an ordinary browser, so a page on any other
// site could otherwise post to these routes and have that session start a
// synchronization or reprocess a stage on the operator's behalf.
//
// A browser attaches Origin to every request whose method is not GET or HEAD,
// including a same-origin one, so the UI's own requests always carry it. A
// missing header is therefore not "same-origin, header omitted" — it is a
// caller that is not this UI, and it is refused rather than trusted. So is
// "null", which is what a sandboxed or redirected context sends.
//
// The OAuth callback is deliberately not wrapped: it is a cross-site GET the
// browser is redirected into, and what protects it is its one-time,
// identity-bound, expiring state rather than its provenance.
func (h *Handler) sameOrigin(next gatedFunc) gatedFunc {
	return func(writer http.ResponseWriter, request *http.Request, principal string) {
		if request.Header.Get("Origin") != h.browserOrigin {
			h.error(writer, http.StatusForbidden, "forbidden", "request origin is not permitted")

			return
		}

		next(writer, request, principal)
	}
}

// contentSecurityPolicy confines the page to this service's own origin plus the
// single configured tile origin. Both basemap styles share that origin, so a
// page that follows the system colour scheme still reaches exactly one.
//
// Three allowances are deliberate, and each was confirmed against a real
// MapLibre render rather than assumed:
//   - worker-src needs 'self' because MapLibre loads its worker from a bundled
//     same-origin module, and blob: because it also spawns blob workers;
//   - style-src needs 'unsafe-inline' because MapLibre styles its own controls
//     inline;
//   - img-src and connect-src need the tile origin for sprites, glyphs, and
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
		"img-src 'self' data: blob: " + h.tileOrigin,
		"connect-src 'self' " + h.tileOrigin,
	}, "; ")
}

func (h *Handler) health(writer http.ResponseWriter, _ *http.Request) {
	h.writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) error(writer http.ResponseWriter, status int, code, message string) {
	h.writeJSON(writer, status, map[string]map[string]string{"error": {"code": code, "message": message}})
}

func (h *Handler) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
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

// originOf reduces a URL to its scheme and host for use in a CSP source list.
func originOf(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("tile style URL must be an absolute HTTPS URL")
	}

	return parsed.Scheme + "://" + parsed.Host, nil
}
