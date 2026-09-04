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
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/session"
)

// The two cookies this service sets. The `__Host-` prefix is enforced by the
// browser only, so Secure, Path=/ and the absent Domain are set by hand too.
const (
	sessionCookie = "__Host-domestique_session"
	loginCookie   = "__Host-domestique_login"
)

// loginCookieSeconds bounds a sign-in that was started and never finished. It
// matches internal/session's own login lifetime.
const loginCookieSeconds = 600

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
	// cacheImmutableGated is cacheImmutable for an artefact behind the identity
	// gate. A shared cache must not hold one at all: what it would store depends
	// on the session that asked for it, and a stored copy served on to a caller
	// without one would hand out an answer the gate exists to withhold.
	cacheImmutableGated = "private, max-age=31536000, immutable"
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
	// Sessions is who is signed in, and the sign-in flow that creates a
	// session. Required: without it the service has no gate at all.
	Sessions Sessions

	// SurfaceIndexFunc reports the map build classifications are read from, false
	// when off or still building. It asks the live index, not the state file.
	SurfaceIndexFunc func() (generation string, builtAt time.Time, ok bool)

	// RideModelValidation is the loaded profile's measured unseen-route error. One
	// value per profile, so every stage carries the same. Nil when absent.
	RideModelValidationFunc func() *RideModelValidation

	// Settings are the runtime settings this handler serves and edits. Required.
	// Read per request, never held: an edit reaches the page and the CSP at once.
	Settings SettingsState

	// StyleOrigins reports the further origins the configured styles name for
	// their glyphs, sprites, and tiles. Optional: a nil one leaves the policy
	// naming the configured style origins alone, which is every host a provider
	// serving all four from one place needs.
	StyleOrigins StyleOrigins

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

	// BrowserOriginURL is an absolute HTTPS URL on the hostname a browser reaches
	// this service at. Only scheme and host are read; it is the one allowed origin.
	BrowserOriginURL string

	// Auth0Domain is the configured tenant's bare host, e.g. "example.eu.auth0.com".
	// It names the one cross-origin redirect the sign-in pages' CSP allows: a
	// browser enforces form-action against the whole redirect chain a form
	// submission follows, not just its immediate action, so StartLogin's 303 to
	// the authorization endpoint is silently refused without it. Optional like
	// tileOrigins below, and for the same reason: a missing value degrades the
	// header rather than failing every unrelated caller's construction.
	Auth0Domain string
}

// RideModelValidation is the frozen coefficient profile's measured
// unseen-route error, from the same route-disjoint benchmark that froze it.
type RideModelValidation struct {
	BiasPercent    float64
	MAEPercent     float64
	P90Percent     float64
	EvaluatedRides int
}

// Handler enforces browser-session identity and exposes the v1 HTTP surface.
type Handler struct {
	oauth    OAuth
	syncRuns Sync
	state    State
	assets   Assets
	weather  Weather
	// validate holds every request to the document before it reaches a
	// handler: parameter bounds, request bodies, and provenance.
	validate            func(http.Handler) http.Handler
	sessions            Sessions
	surfaceIndex        func() (string, time.Time, bool)
	now                 func() time.Time
	mux                 *http.ServeMux
	rideModelValidation func() *RideModelValidation
	settings            SettingsState
	styleOrigins        StyleOrigins
	alerts              Alerts
	tasks               Tasks
	buildRevision       string
	buildImageDigest    string
	browserOrigin       string
	authOrigin          string
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
	if options.Sessions == nil {
		return nil, errors.New("a session service is required")
	}
	if options.Alerts == nil {
		return nil, errors.New("the alert matrix is required")
	}
	if options.Tasks == nil {
		return nil, errors.New("the task list is required")
	}
	browserOrigin, err := browserOriginOf(options.BrowserOriginURL)
	if err != nil {
		return nil, err
	}
	var authOrigin string
	if domain := strings.TrimSpace(options.Auth0Domain); domain != "" {
		authOrigin = "https://" + domain
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
		styleOrigins:        options.StyleOrigins,
		alerts:              options.Alerts,
		tasks:               options.Tasks,
		buildRevision:       publishableRevision(options.BuildRevision),
		buildImageDigest:    publishableDigest(options.BuildImageDigest),
		browserOrigin:       browserOrigin,
		authOrigin:          authOrigin,
		surfaceIndex:        options.SurfaceIndexFunc,
		rideModelValidation: options.RideModelValidationFunc,
		now:                 time.Now,

		sessions: options.Sessions,
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
	h.mux.HandleFunc("GET /v1/sync/runs", h.GetSyncRuns)
	h.mux.HandleFunc("GET /v1/routes", h.GetRoutes)
	h.mux.HandleFunc("GET /v1/providers/{provider}/sourceRoutes/{sourceRouteId}/routes/{stageOrder}", h.GetRoute)
	h.mux.HandleFunc(
		"GET /v1/providers/{provider}/sourceRoutes/{sourceRouteId}/routes/{stageOrder}/geometry",
		h.GetRouteGeometry)
	h.mux.HandleFunc(
		"POST /v1/providers/{provider}/sourceRoutes/{sourceRouteId}/routes/{stageOrder}/reprocess",
		h.adminOnly(h.ReprocessRoute))
	h.mux.HandleFunc("GET /v1/providers/{provider}/routes/{routeId}/stages/{stage}", h.RedirectStageRoute)
	h.mux.HandleFunc(
		"GET /v1/providers/{provider}/routes/{routeId}/stages/{stage}/geometry", h.RedirectStageGeometry)
	h.mux.HandleFunc(
		"POST /v1/providers/{provider}/routes/{routeId}/stages/{stage}/reprocess", h.adminOnly(h.RedirectStageReprocess))
	h.mux.HandleFunc("GET /v1/routes/{routeId}/stages/{stage}", h.RedirectLegacyRoute)
	h.mux.HandleFunc("GET /v1/routes/{routeId}/stages/{stage}/geometry", h.RedirectLegacyGeometry)
	h.mux.HandleFunc("POST /v1/routes/{routeId}/stages/{stage}/reprocess", h.adminOnly(h.RedirectLegacyReprocess))
	h.mux.HandleFunc("GET /v1/settings", h.adminOnly(h.GetSettings))
	h.mux.HandleFunc("PUT /v1/settings/wahoo", h.adminOnly(h.SetWahooApplication))
	h.mux.HandleFunc("PUT /v1/settings/sources/{provider}", h.adminOnly(h.SetSource))
	h.mux.HandleFunc("PUT /v1/settings/notifications", h.adminOnly(h.SetNotifications))
	h.mux.HandleFunc("PUT /v1/settings/alerts", h.adminOnly(h.SetAlerts))
	h.mux.HandleFunc("PUT /v1/settings/timezone", h.adminOnly(h.SetTimezone))
	h.mux.HandleFunc("GET /v1/tasks", h.adminOnly(h.ListTasks))
	h.mux.HandleFunc("GET /v1/tasks/runs", h.adminOnly(h.GetTaskRuns))
	h.mux.HandleFunc("PUT /v1/tasks/{name}/schedule", h.adminOnly(h.SetTaskSchedule))
	h.mux.HandleFunc("POST /v1/tasks/{name}/run", h.RunTask)
	h.mux.HandleFunc("POST /v1/tasks/{name}/run/{argument}", h.RunTask)
	h.mux.HandleFunc("PUT /v1/settings/basemaps", h.adminOnly(h.SetBasemaps))
	h.mux.HandleFunc("PUT /v1/settings/surface", h.adminOnly(h.SetSurface))
	h.mux.HandleFunc("PUT /v1/settings/ridemodel", h.adminOnly(h.SetRideModel))
	h.mux.HandleFunc("PUT /v1/settings/sync", h.adminOnly(h.SetSync))
	h.mux.HandleFunc("GET /v1/webui/config", h.GetWebUIConfig)
	h.mux.HandleFunc("GET /v1/weather", h.GetWeather)
	h.mux.HandleFunc("GET /auth/login", h.GetLoginPage)
	h.mux.HandleFunc("POST /auth/start", h.StartLogin)
	h.mux.HandleFunc("GET /auth/callback", h.CompleteLogin)
	h.mux.HandleFunc("POST /auth/logout", h.Logout)
	// The bare path is a rider connecting their own account: the browser is
	// never told its own subject, so this is the only way a caller with no
	// target yet can start one at all.
	h.mux.HandleFunc("GET /oauth/wahoo/start", h.StartOAuth)
	h.mux.HandleFunc("GET /oauth/wahoo/start/{target}", h.StartOAuth)
	h.mux.HandleFunc("GET /oauth/wahoo/callback", h.CompleteOAuth)
	h.mux.HandleFunc("GET /assets/{asset}", h.GetAsset)
	h.mux.HandleFunc("GET /worker/{asset}", h.GetWorkerAsset)
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
	h.mux.HandleFunc("GET /settings/tasks", h.GetTasksPage)
	h.mux.HandleFunc("GET /admin", h.GetAdminPage)
	h.mux.HandleFunc("GET /admin/tasks", h.GetAdminTasksPage)
	// Browser routes are explicit application navigation, not OpenAPI operations.
	// Separate because ServeMux has no pattern for the unmatched-path fallback.
	h.mux.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		h.notFound(writer)
	})
}

// ServeHTTP applies the shared response headers and dispatches.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	header := writer.Header()
	header.Set("Content-Security-Policy", h.contentSecurityPolicy(request.URL.Path))
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("Cache-Control", cacheAPI)
	// Set on every answer, not only the ones that read the cookie: a blanket
	// Vary is what keeps a shared cache from reusing one caller's page for
	// another, and /healthz below is static enough not to mind.
	header.Set("Vary", "Cookie")
	// HEAD as well as GET: Go's "GET /healthz" pattern answers both, and a
	// liveness probe that sends HEAD must not be told it needs an identity.
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) &&
		request.URL.Path == "/healthz" {
		h.mux.ServeHTTP(writer, request)

		return
	}
	// The sign-in routes are how a caller becomes an identity, so gating them
	// would leave no way in. Their own guards are the login state and Origin.
	if strings.HasPrefix(request.URL.Path, "/auth/") {
		h.bounded(h.mux).ServeHTTP(writer, request)

		return
	}
	// The sign-in page is the application bundle, so the artefacts it names are
	// fetched before any identity exists. They carry build output and no state.
	// Reads only: nothing else about these paths is served without a session.
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) &&
		publicAsset(request.URL.Path) {
		// One answer for every caller, so the blanket Vary above is not true of
		// these: leaving it makes a cache re-fetch the whole bundle whenever the
		// cookie appears or goes, which is exactly at sign-in and sign-out.
		header.Del("Vary")
		h.mux.ServeHTTP(writer, request)

		return
	}
	if strings.HasPrefix(request.URL.Path, "/v1/") || strings.HasPrefix(request.URL.Path, "/oauth/") {
		h.gated(h.bounded(h.validate(h.mux))).ServeHTTP(writer, request)

		return
	}
	h.gated(h.mux).ServeHTTP(writer, request)
}

// publicAsset reports a path served to anyone: the bundle and stylesheet the
// sign-in page loads, and the icons a browser asks for without being told to.
//
// The map's worker is deliberately not among them, and is emitted outside
// `assets/` so it cannot become one by accident: see GetWorkerAsset.
func publicAsset(path string) bool {
	switch path {
	case "/favicon.svg", "/icon-256.png", "/icon-512.png", "/manifest.webmanifest":
		return true
	}

	return strings.HasPrefix(path, "/assets/")
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

// identityKey addresses the signed-in caller in a request context.
type identityKey struct{}

// identityOf is who the gate admitted. Its zero value never reaches a handler:
// everything behind gated() runs only after a session was verified.
func identityOf(ctx context.Context) session.Identity {
	identity, ok := ctx.Value(identityKey{}).(session.Identity)
	if !ok {
		return session.Identity{}
	}

	return identity
}

// gated admits a caller holding a valid session cookie and puts the identity
// in the request context.
func (h *Handler) gated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookie)
		if err != nil {
			h.unauthenticated(writer, request)

			return
		}
		identity, err := h.sessions.Verify(request.Context(), cookie.Value)
		if err != nil {
			// The reason stays here. Telling an unauthenticated caller why its
			// session failed describes the check it has to defeat.
			h.clearCookie(writer, sessionCookie)
			h.unauthenticated(writer, request)

			return
		}

		next.ServeHTTP(writer, request.WithContext(
			context.WithValue(request.Context(), identityKey{}, identity)))
	})
}

// adminOnly refuses a session without the administrator claim. It wraps the
// shared reads and writes: settings, task administration, and reprocess.
func (h *Handler) adminOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if !identityOf(request.Context()).Admin {
			h.forbidden(writer)

			return
		}
		next(writer, request)
	}
}

// forbidden is what a session without the rights an operation needs is told.
func (h *Handler) forbidden(writer http.ResponseWriter) {
	h.error(writer, http.StatusForbidden, "forbidden", "an administrator identity is required")
}

// unauthenticated answers a caller with no usable session: a browser asking for
// a page is sent to sign in, everything else is told in this service's own
// error shape.
func (h *Handler) unauthenticated(writer http.ResponseWriter, request *http.Request) {
	if (request.Method == http.MethodGet || request.Method == http.MethodHead) &&
		strings.Contains(request.Header.Get("Accept"), "text/html") {
		http.Redirect(writer, request, "/auth/login", http.StatusFound)

		return
	}
	h.error(writer, http.StatusUnauthorized, "unauthorized", "an authenticated identity is required")
}

// setSessionCookie issues the session cookie. Every attribute the `__Host-`
// prefix requires is set by hand: the prefix is a browser-side check, not a
// browser-side default.
func (h *Handler) setSessionCookie(writer http.ResponseWriter, token string, expiresAt time.Time) {
	http.SetCookie(writer, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  expiresAt,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// setLoginCookie carries the pending sign-in state. Lax rather than Strict: the
// callback is a top-level cross-site navigation, and Strict would withhold this
// cookie exactly there.
func (h *Handler) setLoginCookie(writer http.ResponseWriter, state string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     loginCookie,
		Value:    state,
		Path:     "/",
		MaxAge:   loginCookieSeconds,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearCookie expires one of this service's cookies, spelled with the same
// attributes it was set with so the browser replaces rather than adds.
func (h *Handler) clearCookie(writer http.ResponseWriter, name string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// contentSecurityPolicy confines the page to this service's origin plus each
// configured basemap's. Three allowances are MapLibre's, confirmed by render:
//   - worker-src 'self' and blob: it loads a bundled worker and spawns blob ones;
//   - style-src 'unsafe-inline': it styles its own controls inline;
//   - img-src and connect-src tile origins: sprites, glyphs, and tiles.
//
// A worker does not read this header from the document that started it: it is
// governed by the policy on its own response. The map's worker is therefore
// served from behind the identity gate, so that it is sent this policy rather
// than the one a public asset gets — see GetWorkerAsset.
//
// A basemap's own origin is only the host the style document is read from. The
// document is free to name another for its glyphs, its sprite, or its tiles,
// and a provider that splits them is common; those hosts are read from the
// style itself rather than configured, and are added here.
//
// Nothing served before an identity exists names a tile origin — the sign-in
// routes or a build artefact. The sign-in routes also allow one form: 'self'
// posts to /auth/start, whose 303 carries the same submission on to the
// configured Auth0 tenant. form-action governs the whole redirect chain a
// submission follows, not only its immediate action, so the tenant is named
// there or a browser refuses to follow it.
func (h *Handler) contentSecurityPolicy(path string) string {
	formAction := "form-action 'none'"
	var tileOrigins []string
	signIn := strings.HasPrefix(path, "/auth/")
	if signIn {
		formAction = "form-action 'self'"
		if h.authOrigin != "" {
			formAction += " " + h.authOrigin
		}
	}
	// The configured map is named to a caller that could hold an identity and to
	// no other: every answer served before one is a build artefact with no map
	// in it, and the header would otherwise hand the origins to anyone.
	if !signIn && !publicAsset(path) {
		// A list that cannot be reduced to origins was never allowed to be stored, so
		// this is a bug. The header then names no tile origin, blanking the map.
		origins, err := tileOriginsOf(h.settings.Values().Basemaps)
		if err == nil {
			tileOrigins = origins
		}
		if h.styleOrigins != nil {
			tileOrigins = append(tileOrigins, h.styleOrigins.Origins()...)
			slices.Sort(tileOrigins)
			tileOrigins = slices.Compact(tileOrigins)
		}
	}

	return strings.Join([]string{
		"default-src 'self'",
		"base-uri 'none'",
		"object-src 'none'",
		"frame-ancestors 'none'",
		formAction,
		"script-src 'self'",
		"style-src 'self' 'unsafe-inline'",
		"font-src 'self'",
		"worker-src 'self' blob:",
		"child-src 'self' blob:",
		strings.TrimSpace("img-src 'self' data: blob: " + strings.Join(tileOrigins, " ")),
		strings.TrimSpace("connect-src 'self' " + strings.Join(tileOrigins, " ")),
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

// targetIDs are the targets this caller may see: every one for an admin, or
// the caller's own — at most one — for anyone else. Read per request against
// the store rather than held, since a target can appear the moment its
// owner first connects.
func (h *Handler) targetIDs(ctx context.Context) ([]string, error) {
	identity := identityOf(ctx)
	ids := []string{}
	if err := h.state.ForEachTarget(ctx, func(id, _, ownerSubject string) error {
		if identity.Admin || ownerSubject == identity.Subject {
			ids = append(ids, id)
		}

		return nil
	}); err != nil {
		return nil, fmt.Errorf("listing targets: %w", err)
	}

	return ids, nil
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
