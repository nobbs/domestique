// Package httpapi owns Tailnet-gated JSON, OAuth, and browser UI HTTP handling.
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

const identityHeader = "Tailscale-User-Login"

const (
	cacheAPI       = "no-store"
	cacheDocument  = "no-cache"
	cacheImmutable = "public, max-age=31536000, immutable"
)

// OAuth performs the protected Wahoo onboarding flow.
type OAuth interface {
	Start(ctx context.Context, tailnetUserLogin, targetID string) (string, error)
	Complete(ctx context.Context, tailnetUserLogin, state, code string) error
}

// SyncTrigger starts one manual synchronization and reports whether it was
// accepted. An accepted run continues independently of the HTTP request.
type SyncTrigger interface {
	Trigger() bool
}

// SyncTriggerFunc adapts a function to SyncTrigger for manual wiring.
type SyncTriggerFunc func() bool

// Trigger starts the adapted manual synchronization.
func (f SyncTriggerFunc) Trigger() bool {
	return f()
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
	LastSyncRun(ctx context.Context) (completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int, found bool, err error)
}

// Options carries the non-secret settings the HTTP surface needs.
type Options struct {
	TailnetUserLogin string
	TileStyleURL     string
	TargetIDs        []string
}

// Handler enforces Tailnet identity and exposes the v1 HTTP surface.
type Handler struct {
	mux          *http.ServeMux
	oauth        OAuth
	syncTrigger  SyncTrigger
	state        State
	assets       Assets
	allowedLogin string
	tileStyleURL string
	tileOrigin   string
	targetIDs    []string
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
	if strings.TrimSpace(options.TailnetUserLogin) == "" {
		return nil, errors.New("tailnet login is required")
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
	tileOrigin, err := originOf(options.TileStyleURL)
	if err != nil {
		return nil, err
	}

	handler := &Handler{
		mux:          http.NewServeMux(),
		oauth:        oauthService,
		state:        state,
		syncTrigger:  syncTrigger,
		assets:       assets,
		allowedLogin: options.TailnetUserLogin,
		tileStyleURL: options.TileStyleURL,
		tileOrigin:   tileOrigin,
		targetIDs:    append([]string(nil), options.TargetIDs...),
	}
	handler.routes()

	return handler, nil
}

// routes registers the fixed v1 surface. Every pattern except the liveness
// probe is wrapped by the Tailnet identity gate.
func (h *Handler) routes() {
	h.mux.HandleFunc("GET /healthz", h.health)

	h.mux.Handle("GET /v1/status", h.gated(h.status))
	h.mux.Handle("POST /v1/sync", h.gated(h.sync))
	h.mux.Handle("GET /v1/routes", h.gated(h.stages))
	h.mux.Handle("GET /v1/routes/{routeID}/stages/{stage}", h.gated(h.stage))
	h.mux.Handle("GET /v1/routes/{routeID}/stages/{stage}/geometry", h.gated(h.stageGeometry))
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

// gatedFunc is a handler that has already proven the caller's identity.
type gatedFunc func(writer http.ResponseWriter, request *http.Request, login string)

// gated rejects any caller that is not the single configured Tailnet identity.
func (h *Handler) gated(next gatedFunc) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		login := request.Header.Get(identityHeader)
		if login == "" {
			h.error(writer, http.StatusUnauthorized, "unauthorized", "tailnet identity is required")

			return
		}
		if login != h.allowedLogin {
			h.error(writer, http.StatusForbidden, "forbidden", "tailnet identity is not permitted")

			return
		}
		next(writer, request, login)
	})
}

// contentSecurityPolicy confines the page to this service's own origin plus the
// single configured tile origin.
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

// originOf reduces a URL to its scheme and host for use in a CSP source list.
func originOf(value string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return "", errors.New("tile style URL must be an absolute HTTPS URL")
	}

	return parsed.Scheme + "://" + parsed.Host, nil
}
