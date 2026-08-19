// Package readiness serves the loopback readiness probe: the one answer to
// "can this process do its job with what the host gave it", as opposed to the
// liveness probe's "can this process answer HTTP at all".
//
// It is a separate package, on its own listener, with a single dependency on
// local state, because that is what keeps the two probes from drifting into
// each other. Nothing here can reach VeloPlanner, Wahoo, Pushover, Cloudflare,
// Tailscale, or the tile provider: this package is given no client that could,
// so a readiness check cannot become a paid, rate-limited, or failing call to
// somebody else's service. The identity-gated surface is served by internal
// httpapi on another socket, and neither handler knows the other's routes.
package readiness

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
)

// stateTimeout bounds the one local read a probe performs. A probe that hangs
// is a probe that never reports anything, which is worse for an operator than
// one that says "not ready" while the state file is stuck.
const stateTimeout = 3 * time.Second

// State is the local state access readiness verifies. It is deliberately the
// whole of this package's dependencies: readiness proves that the process can
// read what it was configured with, and nothing more.
type State interface {
	ForEachTarget(ctx context.Context, visit func(id, authorization string) error) error
}

// Handler answers the readiness probe and nothing else.
type Handler struct {
	mux       *http.ServeMux
	state     State
	targetIDs []string
}

// New creates the readiness handler for the configured target slots.
func New(targetIDs []string, state State) (*Handler, error) {
	if state == nil {
		return nil, errors.New("readiness requires state")
	}
	if len(targetIDs) < 1 {
		return nil, errors.New("readiness requires at least one target ID")
	}
	for _, targetID := range targetIDs {
		if strings.TrimSpace(targetID) == "" {
			return nil, errors.New("readiness target IDs must not be empty")
		}
	}

	handler := &Handler{
		mux:       http.NewServeMux(),
		state:     state,
		targetIDs: append([]string(nil), targetIDs...),
	}
	handler.mux.HandleFunc("GET /readyz", handler.ready)
	// Anything else, including the liveness path, is not served here. The two
	// probes answer on two sockets on purpose, and a probe that quietly answered
	// for the other one would hide which listener an operator actually reached.
	handler.mux.HandleFunc("/", func(writer http.ResponseWriter, _ *http.Request) {
		writeJSON(writer, http.StatusNotFound, map[string]string{"status": "not_found"})
	})

	return handler, nil
}

// ServeHTTP applies the probe response headers and dispatches.
func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	header := writer.Header()
	header.Set("Cache-Control", "no-store")
	header.Set("X-Content-Type-Options", "nosniff")
	h.mux.ServeHTTP(writer, request)
}

// ready reports whether the configured targets are backed by readable local
// state. It deliberately says nothing about whether a target is authorised: a
// slot waiting for its one-time OAuth onboarding is a process working exactly as
// deployed, and a probe that called that "not ready" would keep a correctly
// running container marked unhealthy until a human visited a browser.
func (h *Handler) ready(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), stateTimeout)
	defer cancel()

	known := make(map[string]struct{}, len(h.targetIDs))
	if err := h.state.ForEachTarget(ctx, func(id, _ string) error {
		if slices.Contains(h.targetIDs, id) {
			known[id] = struct{}{}
		}

		return nil
	}); err != nil {
		// The category only. The reason a state read failed can name a path or a
		// key, and this response is written to whatever asked.
		unready(writer, "state_unreadable")

		return
	}
	for _, targetID := range h.targetIDs {
		if _, found := known[targetID]; !found {
			unready(writer, "state_incomplete")

			return
		}
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ready"})
}

func unready(writer http.ResponseWriter, reason string) {
	writeJSON(writer, http.StatusServiceUnavailable, map[string]string{
		"status": "unready",
		"reason": reason,
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		return
	}
}
