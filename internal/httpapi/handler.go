// Package httpapi owns Tailnet-gated JSON and OAuth HTTP handling.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const identityHeader = "Tailscale-User-Login"

// OAuth performs the protected Wahoo onboarding flow.
type OAuth interface {
	Start(ctx context.Context, tailnetUserLogin, targetID string) (string, error)
	Complete(ctx context.Context, tailnetUserLogin, state, code string) error
}

// State provides only non-secret metadata for the JSON read model.
type State interface {
	ForEachTarget(ctx context.Context, visit func(id, authorization string) error) error
	ForEachSourceStage(ctx context.Context, visit func(routeID int64, stageOrder int, sourceRevision, contentHash string) error) error
	LastSyncRun(ctx context.Context) (completedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int, found bool, err error)
}

// Handler enforces Tailnet identity and exposes the small v1 HTTP surface.
type Handler struct {
	oauth        OAuth
	state        State
	allowedLogin string
}

// New creates a handler. Health checks are intentionally unauthenticated;
// deployment must keep the listener private to the local Tailscale proxy.
func New(allowedLogin string, oauthService OAuth, state State) (*Handler, error) {
	if strings.TrimSpace(allowedLogin) == "" || oauthService == nil || state == nil {
		return nil, errors.New("tailnet login, oauth service, and state are required")
	}

	return &Handler{allowedLogin: allowedLogin, oauth: oauthService, state: state}, nil
}

// ServeHTTP handles the fixed v1 API without echoing sensitive query values.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", "no-store")
	if request.URL.Path == "/healthz" {
		h.health(writer, request)
		return
	}
	login, ok := h.authorize(writer, request)
	if !ok {
		return
	}
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/v1/status":
		h.status(writer, request)
	case request.Method == http.MethodGet && request.URL.Path == "/v1/routes":
		h.routes(writer, request, 0, 0, false)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/v1/routes/"):
		routeID, stageOrder, parsed := stagePath(request.URL.Path)
		if !parsed {
			h.error(writer, http.StatusNotFound, "not_found", "resource was not found")
			return
		}
		h.routes(writer, request, routeID, stageOrder, true)
	case request.Method == http.MethodGet && strings.HasPrefix(request.URL.Path, "/oauth/wahoo/start/"):
		h.start(writer, request, login)
	case request.Method == http.MethodGet && request.URL.Path == "/oauth/wahoo/callback":
		h.callback(writer, request, login)
	default:
		h.error(writer, http.StatusNotFound, "not_found", "resource was not found")
	}
}

func (h *Handler) health(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		h.error(writer, http.StatusMethodNotAllowed, "method_not_allowed", "method is not allowed")
		return
	}
	h.writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) authorize(writer http.ResponseWriter, request *http.Request) (string, bool) {
	login := request.Header.Get(identityHeader)
	if login == "" {
		h.error(writer, http.StatusUnauthorized, "unauthorized", "tailnet identity is required")
		return "", false
	}
	if login != h.allowedLogin {
		h.error(writer, http.StatusForbidden, "forbidden", "tailnet identity is not permitted")
		return "", false
	}

	return login, true
}

func (h *Handler) start(writer http.ResponseWriter, request *http.Request, login string) {
	targetID := strings.TrimPrefix(request.URL.Path, "/oauth/wahoo/start/")
	if targetID == "" || strings.Contains(targetID, "/") {
		h.error(writer, http.StatusNotFound, "not_found", "resource was not found")
		return
	}
	location, err := h.oauth.Start(request.Context(), login, targetID)
	if err != nil {
		h.error(writer, http.StatusBadRequest, "authorization_failed", "wahoo authorization could not be started")
		return
	}
	parsedLocation, parseErr := url.Parse(location)
	if parseErr != nil || parsedLocation.Scheme != "https" || parsedLocation.Host == "" {
		h.error(writer, http.StatusInternalServerError, "unavailable", "service state is unavailable")
		return
	}
	//nolint:gosec // The OAuth service returned a validated HTTPS Wahoo authorization URL.
	http.Redirect(writer, request, location, http.StatusFound)
}

func (h *Handler) callback(writer http.ResponseWriter, request *http.Request, login string) {
	if err := h.oauth.Complete(request.Context(), login, request.URL.Query().Get("state"), request.URL.Query().Get("code")); err != nil {
		h.error(writer, http.StatusBadRequest, "authorization_failed", "wahoo authorization could not be completed")
		return
	}
	http.Redirect(writer, request, "/v1/status", http.StatusSeeOther)
}

func (h *Handler) status(writer http.ResponseWriter, request *http.Request) {
	targets := make([]targetView, 0, 2)
	ready := true
	if err := h.state.ForEachTarget(request.Context(), func(id, authorization string) error {
		targets = append(targets, targetView{ID: id, Authorization: authorization})
		ready = ready && authorization == "authorized"
		return nil
	}); err != nil {
		h.error(writer, http.StatusInternalServerError, "unavailable", "service state is unavailable")
		return
	}
	view := statusView{Ready: ready, Targets: targets, Sync: syncView{State: "not_ready"}}
	if ready {
		view.Sync.State = "idle"
	}
	completedAt, outcome, _, sourceStages, created, updated, deleted, found, err := h.state.LastSyncRun(request.Context())
	if err != nil {
		h.error(writer, http.StatusInternalServerError, "unavailable", "service state is unavailable")
		return
	}
	if found {
		view.Sync.State, view.Sync.LastResult = outcome, outcome
		view.Sync.LastCompletedAt = completedAt.Format(time.RFC3339)
		view.Sync.SourceStages, view.Sync.Created, view.Sync.Updated, view.Sync.Deleted = sourceStages, created, updated, deleted
	}
	h.writeJSON(writer, http.StatusOK, view)
}

func (h *Handler) routes(writer http.ResponseWriter, request *http.Request, routeID int64, stageOrder int, single bool) {
	stages := make([]stageView, 0)
	err := h.state.ForEachSourceStage(request.Context(), func(id int64, order int, revision, contentHash string) error {
		if single && (id != routeID || order != stageOrder) {
			return nil
		}
		stages = append(stages, stageView{RouteID: id, StageOrder: order, SourceRevision: revision, ContentHash: contentHash})
		return nil
	})
	if err != nil {
		h.error(writer, http.StatusInternalServerError, "unavailable", "service state is unavailable")
		return
	}
	if single {
		if len(stages) == 0 {
			h.error(writer, http.StatusNotFound, "not_found", "route stage was not found")
			return
		}
		h.writeJSON(writer, http.StatusOK, stages[0])
		return
	}
	h.writeJSON(writer, http.StatusOK, map[string][]stageView{"stages": stages})
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

func stagePath(path string) (routeID int64, stageOrder int, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/v1/routes/"), "/")
	if len(parts) != 3 || parts[1] != "stages" {
		return 0, 0, false
	}
	routeID, routeErr := strconv.ParseInt(parts[0], 10, 64)
	stageOrder, stageErr := strconv.Atoi(parts[2])
	return routeID, stageOrder, routeErr == nil && stageErr == nil && routeID > 0 && stageOrder > 0
}

type targetView struct {
	ID            string `json:"id"`
	Authorization string `json:"authorisation"`
}
type stageView struct {
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	SourceRevision string `json:"source_revision"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	ContentHash string `json:"content_hash"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	RouteID    int64 `json:"route_id"`
	StageOrder int   `json:"stage"`
}
type syncView struct {
	State string `json:"state"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastCompletedAt string `json:"last_completed_at,omitempty"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastResult string `json:"last_result,omitempty"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	SourceStages int `json:"source_stages"`
	Created      int `json:"created"`
	Updated      int `json:"updated"`
	Deleted      int `json:"deleted"`
}
type statusView struct {
	Targets []targetView `json:"targets"`
	Sync    syncView     `json:"sync"`
	Ready   bool         `json:"ready"`
}
