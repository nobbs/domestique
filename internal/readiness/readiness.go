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
	"time"

	"github.com/nobbs/domestique/internal/readiness/contract"
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
	targetIDs func() []string
}

// New creates the readiness handler over the target slots configured right now.
//
// The slots are read per probe rather than held, because they are a setting an
// operator edits. A deployment that has none yet is ready: it is running
// exactly as deployed, waiting to be configured through the browser, and a
// probe that called that unhealthy would roll the deploy back before anyone
// could configure it.
func New(targetIDs func() []string, state State) (*Handler, error) {
	if state == nil {
		return nil, errors.New("readiness requires state")
	}
	if targetIDs == nil {
		return nil, errors.New("readiness requires its target IDs to be readable")
	}

	handler := &Handler{
		mux:       http.NewServeMux(),
		state:     state,
		targetIDs: targetIDs,
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

	targetIDs := h.targetIDs()
	known := make(map[string]struct{}, len(targetIDs))
	if err := h.state.ForEachTarget(ctx, func(id, _ string) error {
		if slices.Contains(targetIDs, id) {
			known[id] = struct{}{}
		}

		return nil
	}); err != nil {
		// The category only. The reason a state read failed can name a path or a
		// key, and this response is written to whatever asked.
		unready(writer, contract.Unready_ReasonStateUnreadable)

		return
	}
	for _, targetID := range targetIDs {
		if _, found := known[targetID]; !found {
			unready(writer, contract.Unready_ReasonStateIncomplete)

			return
		}
	}
	writeJSON(writer, http.StatusOK, contract.Readiness{Status: "ready"})
}

// unready reports why the probe failed, in the contract's own words: the reason
// is the generated enum rather than a string, so a reason this package can
// report is a reason api/openapi.yaml declares.
func unready(writer http.ResponseWriter, reason contract.Unready_Reason) {
	writeJSON(writer, http.StatusServiceUnavailable, contract.Unready{
		Status: "unready",
		Reason: reason,
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	if err := json.NewEncoder(writer).Encode(value); err != nil {
		return
	}
}
