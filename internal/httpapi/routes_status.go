package httpapi

import (
	"net/http"
	"slices"
	"time"
)

const authorizedState = "authorized"

// status reports readiness, target authorisation, and the last terminal run.
func (h *Handler) status(writer http.ResponseWriter, request *http.Request, _ string) {
	authorizations := make(map[string]string, len(h.targetIDs))
	if err := h.state.ForEachTarget(request.Context(), func(id, authorization string) error {
		if slices.Contains(h.targetIDs, id) {
			authorizations[id] = authorization
		}

		return nil
	}); err != nil {
		h.unavailable(writer)

		return
	}

	targets := make([]targetView, 0, len(h.targetIDs))
	ready := true
	for _, targetID := range h.targetIDs {
		authorization, found := authorizations[targetID]
		if !found {
			h.unavailable(writer)

			return
		}
		targets = append(targets, targetView{ID: targetID, Authorization: authorization})
		ready = ready && authorization == authorizedState
	}

	view := statusView{Ready: ready, Targets: targets, Sync: syncView{State: "not_ready"}}
	if ready {
		view.Sync.State = "idle"
	}
	completedAt, outcome, _, sourceStages, created, updated, deleted, found, err := h.state.LastSyncRun(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}
	if found {
		view.Sync.State, view.Sync.LastResult = outcome, outcome
		view.Sync.LastCompletedAt = completedAt.Format(time.RFC3339)
		view.Sync.SourceStages, view.Sync.Created, view.Sync.Updated, view.Sync.Deleted =
			sourceStages, created, updated, deleted
	}
	h.writeJSON(writer, http.StatusOK, view)
}

// sync queues one immediate run through the same reporting path as the schedule.
func (h *Handler) sync(writer http.ResponseWriter, _ *http.Request, _ string) {
	if !h.syncTrigger.Trigger() {
		h.error(writer, http.StatusConflict, "sync_in_progress", "a synchronization is already running")

		return
	}
	h.writeJSON(writer, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *Handler) unavailable(writer http.ResponseWriter) {
	h.error(writer, http.StatusInternalServerError, "unavailable", "service state is unavailable")
}
