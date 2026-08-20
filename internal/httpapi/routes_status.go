package httpapi

import (
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"time"
)

// The authorisation words this service serves. Three of them are the states a
// target slot durably holds; "pending" is the fourth, and is derived rather
// than stored, because it describes the flow rather than the slot — the moment
// between a protected start request and the callback that ends it. Deriving it
// is what keeps the slot from needing transitions on expiry, denial, and
// exchange failure, none of which anything tells this service about.
const (
	notAuthorizedState        = "not_authorized"
	pendingState              = "pending"
	authorizedState           = "authorized"
	needsReauthorizationState = "needs_reauthorization"
)

// reportedAuthorization is what the status view says about one slot, given what
// the store holds and whether an authorization is in flight for it.
//
// The substitution is deliberately narrow. A slot that is already authorised
// stays authorised while a fresh flow runs: it holds a working refresh token
// until that flow replaces it, and reporting otherwise would say the service
// had stopped being able to write to an account it can still write to.
func reportedAuthorization(stored string, inFlight bool) string {
	if inFlight && (stored == notAuthorizedState || stored == needsReauthorizationState) {
		return pendingState
	}

	return stored
}

// The words for work that has not finished. None of them may be replaced by the
// outcome of an earlier run: a run in flight has produced no result, and one
// that has not started has produced nothing at all.
const (
	queuedState  = "queued"
	runningState = "running"
	delayedState = "delayed"
)

// liveSyncState names the state of a run that has not finished, and reports
// false when none is under way.
//
// A run outranks a delay, because a manual trigger during the initial delay is
// work happening rather than work waiting for its turn.
func liveSyncState(activity SyncActivityState) (string, bool) {
	switch {
	case activity.Running && activity.Phase != "":
		return runningState, true
	case activity.Running:
		return queuedState, true
	case !activity.StartsAt.IsZero():
		return delayedState, true
	}

	return "", false
}

// status reports readiness, per-target convergence, and the last terminal run.
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

	inFlight := make(map[string]bool, len(h.targetIDs))
	if err := h.state.ForEachPendingAuthorization(request.Context(), func(targetID string) error {
		inFlight[targetID] = true

		return nil
	}); err != nil {
		h.unavailable(writer)

		return
	}

	stageCounts, err := h.targetStageCounts(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}
	runs, err := h.targetRuns(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}

	targets := make([]targetView, 0, len(h.targetIDs))
	// The aggregate of the per-target counts, which is the only progress a run
	// in flight reports: how much of the library is already on the configured
	// accounts, and how much of it is still owed to them.
	allStages := targetStagesView{}
	ready, converged := true, true
	for _, targetID := range h.targetIDs {
		stored, found := authorizations[targetID]
		if !found {
			h.unavailable(writer)

			return
		}
		authorization := reportedAuthorization(stored, inFlight[targetID])
		stages := stageCounts[targetID]
		var lastRun *targetRunView
		if run, recorded := runs[targetID]; recorded {
			lastRun = &run
		}
		convergence := convergenceState(authorization, stages, lastRun)
		targets = append(targets, targetView{
			ID:            targetID,
			Authorization: authorization,
			Convergence:   convergence,
			Stages:        stages,
			LastRun:       lastRun,
		})
		allStages.Current += stages.Current
		allStages.Pending += stages.Pending
		ready = ready && authorization == authorizedState
		// Overall convergence is the conjunction, so one lagging slot is enough
		// to say the library is not everywhere it belongs.
		converged = converged && convergence == convergenceCurrent
	}

	activity := h.syncRuns.Activity()
	view := statusView{
		Ready:     ready,
		Converged: converged,
		Targets:   targets,
		Sync:      syncView{State: "not_ready"},
	}
	// Only when the revision is known: the digest alone would say which image is
	// running without saying what is in it, and a group with nothing to identify
	// is worse than no group.
	if h.buildRevision != "" {
		view.Build = &buildView{Revision: h.buildRevision, ImageDigest: h.buildImageDigest}
	}
	if ready {
		view.Sync.State = "idle"
	}
	scheduleSource, scheduleTargets, err := h.state.SyncSchedule(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}
	view.Sync.Schedule = syncScheduleView{Source: scheduleSource, Targets: scheduleTargets}
	classified, total, coverageErr := h.state.SurfaceCoverage(request.Context())
	if coverageErr != nil {
		h.unavailable(writer)

		return
	}
	view.Sync.Surface = surfaceView{Classified: classified, Total: total}
	if phaseErr := h.state.ForEachPhaseRun(request.Context(), func(
		phase string, completedAt time.Time, outcome, detail string,
		sourceStages, created, updated, deleted int,
	) error {
		run := phaseRunView{
			LastCompletedAt: completedAt.Format(time.RFC3339),
			LastResult:      outcome,
			LastFailure:     detail,
			SourceStages:    sourceStages,
			Created:         created,
			Updated:         updated,
			Deleted:         deleted,
		}
		switch phase {
		case string(SyncPhaseSource):
			view.Sync.Phases.Source = &run
		case string(SyncPhaseTargets):
			view.Sync.Phases.Targets = &run
		}

		return nil
	}); phaseErr != nil {
		h.unavailable(writer)

		return
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
	// A run that has not finished outranks the last one that did. Reporting
	// "succeeded" while work is under way would be describing the previous run
	// with nothing to say so, and an operator who has just pressed a button
	// would read it as their answer.
	if state, live := liveSyncState(activity); live {
		view.Sync.State = state
		view.Sync.Active = &activeView{
			Phase:   string(activity.Phase),
			Targets: len(h.targetIDs),
			Stages:  allStages,
		}
		// Only a delay has a due instant to report. A run triggered while the
		// first one is still being held back is under way now, and saying when
		// something else was going to start would read as its own progress.
		if state == delayedState {
			view.Sync.Active.StartsAt = activity.StartsAt.UTC().Format(time.RFC3339)
		}
	}
	h.writeJSON(writer, http.StatusOK, view)
}

// setSyncSchedule switches either half of the scheduled synchronization on or
// off. It changes nothing about a run already in flight, and never starts one.
func (h *Handler) setSyncSchedule(writer http.ResponseWriter, request *http.Request, _ string) {
	var body struct {
		Source  *bool `json:"source"`
		Targets *bool `json:"targets"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(writer, request.Body, maximumRequestBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Source == nil || body.Targets == nil {
		h.error(writer, http.StatusBadRequest, "invalid_request", "both schedule switches are required")

		return
	}
	// One object, and nothing after it. A body carrying a second value is a
	// caller who thinks they sent something this service never read, and
	// silently acting on the first half of that is how a switch ends up in a
	// state nobody asked for.
	if decoder.More() {
		h.error(writer, http.StatusBadRequest, "invalid_request", "the request body must be one object")

		return
	}
	if err := h.state.SetSyncSchedule(request.Context(), *body.Source, *body.Targets); err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, syncScheduleView{Source: *body.Source, Targets: *body.Targets})
}

// The recorded history is served a page at a time. The default is what the page
// shows without asking for more, and the ceiling keeps one request from reading
// the whole retained window into a response.
const (
	defaultSyncRunPage = 20
	maximumSyncRunPage = 100
)

// syncHistory serves one page of the recorded run history, newest first,
// followed by the cursor for the page after it.
//
// Every field is read from the same local records the status response is
// derived from, so a page names no route, carries no geometry, quotes nothing a
// provider said, and costs no provider call.
func (h *Handler) syncHistory(writer http.ResponseWriter, request *http.Request, _ string) {
	query := request.URL.Query()
	limit := defaultSyncRunPage
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > maximumSyncRunPage {
			h.error(writer, http.StatusBadRequest, "invalid_request",
				"limit must be between 1 and "+strconv.Itoa(maximumSyncRunPage))

			return
		}
		limit = parsed
	}
	// An empty history is an empty list rather than a null one: the page is the
	// answer either way.
	view := syncRunsView{Runs: []syncRunView{}}
	next, usable, err := h.state.ForEachSyncRun(request.Context(), query.Get("after"), limit, func(
		reference, phase string, completedAt time.Time, outcome, detail string,
		sourceStages, created, updated, deleted int,
	) error {
		view.Runs = append(view.Runs, syncRunView{
			Reference:    reference,
			Phase:        phase,
			CompletedAt:  completedAt.Format(time.RFC3339),
			Result:       outcome,
			Failure:      detail,
			SourceStages: sourceStages,
			Created:      created,
			Updated:      updated,
			Deleted:      deleted,
		})

		return nil
	})
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !usable {
		h.error(writer, http.StatusBadRequest, "invalid_request",
			"the history cursor is not one this service issued")

		return
	}
	view.Next = next
	h.writeJSON(writer, http.StatusOK, view)
}

// sync queues one immediate run through the same reporting path as the schedule.
func (h *Handler) sync(writer http.ResponseWriter, _ *http.Request, _ string) {
	h.trigger(writer, SyncPhaseAll)
}

// syncSource queues one immediate read of the source library. It runs whether or
// not the schedule is allowed to start that phase: the switch governs unattended
// runs, and an operator asking for one now has already decided.
func (h *Handler) syncSource(writer http.ResponseWriter, _ *http.Request, _ string) {
	h.trigger(writer, SyncPhaseSource)
}

// syncTargets queues one immediate reconciliation of stored state onto the
// targets, on the same terms as syncSource.
func (h *Handler) syncTargets(writer http.ResponseWriter, _ *http.Request, _ string) {
	h.trigger(writer, SyncPhaseTargets)
}

func (h *Handler) trigger(writer http.ResponseWriter, phase SyncPhase) {
	if !h.syncRuns.Trigger(phase) {
		h.error(writer, http.StatusConflict, "sync_in_progress", "a synchronization is already running")

		return
	}
	h.writeJSON(writer, http.StatusAccepted, map[string]string{"status": "accepted"})
}

func (h *Handler) unavailable(writer http.ResponseWriter) {
	h.error(writer, http.StatusInternalServerError, "unavailable", "service state is unavailable")
}
