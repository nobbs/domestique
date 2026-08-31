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

// assertionHeader carries the signed Cloudflare Access token, the only identity
// this service accepts. Tailscale-User-Login is deliberately never read.
const assertionHeader = "Cf-Access-Jwt-Assertion"

// maximumRequestBytes bounds the only request bodies this service reads. They
// carry two booleans, so anything larger is a mistake or an attempt.
const maximumRequestBytes = 1 << 10

// maximumSettingsBytes bounds the one body that is larger by design: the basemap
// list carries two URLs per entry.
const maximumSettingsBytes = 16 << 10

const (
	cacheAPI       = "no-store"
	cacheDocument  = "no-cache"
	cacheImmutable = "public, max-age=31536000, immutable"
)

// The shapes a build stamp may have before this package will serve it: only a
// full commit object name and a `sha256:` digest leave the service.
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

	// SurfaceIndexFunc reports the map build classifications are read from, false
	// when off or still building. It asks the live index, not the state file.
	SurfaceIndexFunc func() (generation string, builtAt time.Time, ok bool)

	// RideModelValidation is the loaded profile's measured unseen-route error. One
	// value per profile, so every stage carries the same. Nil when absent.
	RideModelValidationFunc func() *RideModelValidation

	// Settings are the runtime settings this handler serves and edits. Required.
	// Read per request, never held: an edit reaches the page and the CSP at once.
	Settings SettingsState

	// Alerts are what this service can announce and what an operator has decided
	// about each. Required.
	Alerts Alerts

	// Tasks are the background activities this handler lists and starts.
	// Required.
	Tasks Tasks

	// BuildRevision and BuildImageDigest name the source commit and the image
	// running it. Both optional, and published only when well-formed.
	BuildRevision string

	// BuildImageDigest is the digest alone. Whatever registry and repository the
	// host pulls it from is deployment topology and stays on the host.
	BuildImageDigest string

	// AccessEmail is the one address an Access assertion may name, and the
	// principal every authenticated request resolves to.
	AccessEmail string

	// AccessSignOutURL ends the session and is served by whatever fronts this
	// service. Empty where nothing would answer it.
	AccessSignOutURL string

	// BrowserOriginURL is an absolute HTTPS URL on the hostname a browser reaches
	// this service at. Only scheme and host are read; it is the one allowed origin.
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
	alerts              Alerts
	tasks               Tasks
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
	if options.Alerts == nil {
		return nil, errors.New("the alert matrix is required")
	}
	if options.Tasks == nil {
		return nil, errors.New("the task list is required")
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
	// The settings are not re-checked here: they arrive already validated by the
	// same rules an edit through this handler is held to.
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
		alerts:              options.Alerts,
		tasks:               options.Tasks,
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
	h.mux.HandleFunc("PUT /v1/settings/alerts", h.SetAlerts)
	h.mux.HandleFunc("PUT /v1/settings/timezone", h.SetTimezone)
	h.mux.HandleFunc("GET /v1/tasks", h.ListTasks)
	h.mux.HandleFunc("POST /v1/tasks/{name}/run", h.RunTask)
	h.mux.HandleFunc("POST /v1/tasks/{name}/run/{argument}", h.RunTask)
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
	// Browser routes are explicit application navigation, not OpenAPI operations.
	// Separate because ServeMux has no pattern for the unmatched-path fallback.
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
// declared body whole, without a limit of its own.
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

// gated rejects any caller that is not the single configured identity, verifying
// the signed Access assertion on every request. No Tailscale-User-Login branch.
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

		// The asserted address is proven and then dropped: OAuth state is bound to the
		// configured spelling, which differs from it only in case.
		next.ServeHTTP(writer, request)
	})
}

// contentSecurityPolicy confines the page to this service's origin plus each
// configured basemap's. Three allowances are MapLibre's, confirmed by render:
//   - worker-src 'self' and blob: it loads a bundled worker and spawns blob ones;
//   - style-src 'unsafe-inline': it styles its own controls inline;
//   - img-src and connect-src tile origins: sprites, glyphs, and tiles.
func (h *Handler) contentSecurityPolicy() string {
	// A list that cannot be reduced to origins was never allowed to be stored, so
	// this is a bug. The header then names no tile origin, blanking the map.
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

// browserOriginOf reduces a URL to the origin a browser names in an Origin
// header: lowercase scheme and host, default HTTPS port dropped.
func browserOriginOf(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("browser origin URL must be an absolute HTTPS URL")
	}

	return "https://" + strings.TrimSuffix(strings.ToLower(parsed.Host), ":443"), nil
}

// accessEmailOf returns the one address an assertion may name. Checked for shape
// because the contract publishes it as an email and the gate compares literally.
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

// signOutPathOf returns the path a page may offer as the way out, or empty. It
// must be a path on this origin: absolute, //host and javascript: are refused.
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
// empty. Dropped rather than refused, so a bad stamp cannot stop the service.
func publishableRevision(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != revisionLength || !isLowerHex(trimmed) {
		return ""
	}

	return trimmed
}

// publishableDigest returns the image digest this build may claim, or empty. A
// reference still carrying a registry and repository is refused, not trimmed.
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

// tileOriginsOf reduces the basemaps to the distinct origins the page may reach,
// sorted. A dark style is held to its own entry's origin here too.
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

	// Lowercased because a host is not case-sensitive, matching the configuration
	// layer's provenance comparison.
	return parsed.Scheme + "://" + strings.ToLower(parsed.Host), nil
}
