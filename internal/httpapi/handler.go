// Package httpapi owns identity-gated JSON, OAuth, and browser UI HTTP handling.
package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/mail"
	"net/url"
	"slices"
	"strings"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
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

// maximumSettingsBytes bounds the one body that is larger by design. The
// basemap list carries two URLs per entry, so the cap every other route is
// right to have would refuse a legitimate edit.
const maximumSettingsBytes = 16 << 10

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

	// RideModelValidation is the loaded coefficient profile's measured
	// unseen-route error, when a profile is configured and its file carries
	// one. It describes the profile as a whole, so every stage response
	// carries the same value rather than one derived per stage. Nil when no
	// profile is configured or the loaded file predates #217's validation
	// fields.
	RideModelValidationFunc func() *RideModelValidation

	// Settings are the runtime settings this handler both serves and edits.
	// Required.
	//
	// Every read of one goes through here per request rather than being held,
	// because an operator edits them while the service runs: a basemap added to
	// the list has to reach the page's configuration and the
	// Content-Security-Policy header at once, not at the next restart.
	Settings SettingsState

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
}

// RideModelValidation is the frozen coefficient profile's measured
// unseen-route error, from the same route-disjoint benchmark that froze it.
type RideModelValidation struct {
	BiasPercent    float64
	MAEPercent     float64
	P90Percent     float64
	EvaluatedRides int
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
	rideModelValidation func() *RideModelValidation
	settings            SettingsState
	buildRevision       string
	buildImageDigest    string
	browserOrigin       string
	allowedEmail        string
	signOutURL          string
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
	if options.AccessVerifier == nil {
		return nil, errors.New("an access verifier is required")
	}
	accessEmail, err := accessEmailOf(options.AccessEmail)
	if err != nil {
		return nil, err
	}
	signOutURL, err := signOutPathOf(options.AccessSignOutURL)
	if err != nil {
		return nil, err
	}
	browserOrigin, err := browserOriginOf(options.BrowserOriginURL)
	if err != nil {
		return nil, err
	}
	// The settings themselves are not re-checked here. They reach this handler
	// already validated, by the same rules an edit written through it is held
	// to, and a second copy of those rules here is the drift the runtime
	// settings package exists to prevent.
	if options.Settings == nil {
		return nil, errors.New("the runtime settings are required")
	}

	handler := &Handler{
		mux:                 http.NewServeMux(),
		oauth:               oauthService,
		state:               state,
		syncRuns:            syncRuns,
		assets:              assets,
		weather:             weather,
		settings:            options.Settings,
		buildRevision:       publishableRevision(options.BuildRevision),
		buildImageDigest:    publishableDigest(options.BuildImageDigest),
		browserOrigin:       browserOrigin,
		surfaceIndex:        options.SurfaceIndexFunc,
		rideModelValidation: options.RideModelValidationFunc,
		now:                 time.Now,

		accessVerifier: options.AccessVerifier,
		allowedEmail:   accessEmail,
		signOutURL:     signOutURL,
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
	h.mux.HandleFunc("GET /v1/providers/{provider}/sourceRoutes/{sourceRouteId}/routes/{stageOrder}", h.GetRoute)
	h.mux.HandleFunc(
		"GET /v1/providers/{provider}/sourceRoutes/{sourceRouteId}/routes/{stageOrder}/geometry",
		h.GetRouteGeometry)
	h.mux.HandleFunc(
		"POST /v1/providers/{provider}/sourceRoutes/{sourceRouteId}/routes/{stageOrder}/reprocess",
		h.ReprocessRoute)
	h.mux.HandleFunc("GET /v1/providers/{provider}/routes/{routeId}/stages/{stage}", h.RedirectStageRoute)
	h.mux.HandleFunc(
		"GET /v1/providers/{provider}/routes/{routeId}/stages/{stage}/geometry", h.RedirectStageGeometry)
	h.mux.HandleFunc(
		"POST /v1/providers/{provider}/routes/{routeId}/stages/{stage}/reprocess", h.RedirectStageReprocess)
	h.mux.HandleFunc("GET /v1/routes/{routeId}/stages/{stage}", h.RedirectLegacyRoute)
	h.mux.HandleFunc("GET /v1/routes/{routeId}/stages/{stage}/geometry", h.RedirectLegacyGeometry)
	h.mux.HandleFunc("POST /v1/routes/{routeId}/stages/{stage}/reprocess", h.RedirectLegacyReprocess)
	h.mux.HandleFunc("GET /v1/settings", h.GetSettings)
	h.mux.HandleFunc("PUT /v1/settings/wahoo", h.SetWahooApplication)
	h.mux.HandleFunc("PUT /v1/settings/targets", h.SetTargets)
	h.mux.HandleFunc("PUT /v1/settings/sources/{provider}", h.SetSource)
	h.mux.HandleFunc("PUT /v1/settings/notifications", h.SetNotifications)
	h.mux.HandleFunc("PUT /v1/settings/basemaps", h.SetBasemaps)
	h.mux.HandleFunc("PUT /v1/settings/surface", h.SetSurface)
	h.mux.HandleFunc("PUT /v1/settings/ridemodel", h.SetRideModel)
	h.mux.HandleFunc("PUT /v1/settings/sync", h.SetSync)
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
	h.mux.HandleFunc("GET /catalogue", h.GetCataloguePage)
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
			request.Body = http.MaxBytesReader(writer, request.Body, requestLimit(request.URL.Path))
		}
		next.ServeHTTP(writer, request)
	})
}

// requestLimit is how large a body one path may carry.
func requestLimit(path string) int64 {
	if path == basemapsPath {
		return maximumSettingsBytes
	}

	return maximumRequestBytes
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
	// A list that cannot be reduced to origins is one that was never allowed to
	// be stored, so the error is a bug rather than a state to serve around. The
	// header then names no tile origin at all, which blanks the map rather than
	// opening the policy.
	tileOrigins, err := tileOriginsOf(h.settings.Values().Basemaps)
	if err != nil {
		tileOrigins = nil
	}

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
		"img-src 'self' data: blob: " + strings.Join(tileOrigins, " "),
		"connect-src 'self' " + strings.Join(tileOrigins, " "),
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

// accessEmailOf returns the one address an assertion may name.
//
// It is checked for shape and not only for presence, because the page is now
// told what it is: the contract publishes it as an email, and a deployment that
// wrote something else there would have this service serving a response its own
// schema does not describe. `mail.ParseAddress` is the reading Go already has,
// and the address it returns has to be the whole of what was configured —
// otherwise `Rider <rider@example.test>` would pass, and the gate compares the
// asserted address against this one literally.
func accessEmailOf(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", errors.New("an access email is required")
	}
	parsed, err := mail.ParseAddress(trimmed)
	if err != nil || parsed.Address != trimmed {
		return "", errors.New("the access email must be a bare address")
	}

	return trimmed, nil
}

// signOutPathOf returns the path a page may offer as the way out, or empty for
// a deployment that named none.
//
// The value reaches a browser as the href of a link a reader clicks, so what it
// may be is worth stating rather than assuming. It is a path on this service's
// own origin: an absolute one is a different site, `//host` is a different site
// spelled to look like a path, and `javascript:` is script that runs on click.
// None of those is a way out of this session, and refusing here means a
// deployment that misnames one fails to start rather than serving a link that
// leaves — or executes — on press.
//
// Nothing configurable reaches this today; `cmd/domestique` passes a constant.
// The check is here because that is a property of the caller rather than of
// this option, and the option is what the page trusts.
func signOutPathOf(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	if trimmed != value || strings.ContainsAny(trimmed, " \t\r\n") {
		return "", errors.New("the sign-out URL must not contain whitespace")
	}
	if !strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "//") {
		return "", errors.New("the sign-out URL must be a path on this service's own origin")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" {
		return "", errors.New("the sign-out URL must be a path on this service's own origin")
	}

	return trimmed, nil
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

// targetIDs are the destination slots configured right now. They are read per
// request rather than held, because the list is a setting an operator edits.
func (h *Handler) targetIDs() []string {
	return h.settings.Values().Wahoo.Targets
}

// sourceBaseURL is one provider's own web application, or empty when that
// provider is not configured.
func sourceBaseURL(sources []runtimeconfig.Source, provider route.Provider) string {
	for _, source := range sources {
		if source.Provider == provider {
			return source.BaseURL
		}
	}

	return ""
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
func tileOriginsOf(basemaps []runtimeconfig.Basemap) ([]string, error) {
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
