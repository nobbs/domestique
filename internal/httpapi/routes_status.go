package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

// The authorisation words this service serves. "pending" is derived rather than
// stored: it describes the flow, which ends by expiry or denial unobserved.
const (
	notAuthorizedState        = "not_authorized"
	pendingState              = "pending"
	authorizedState           = "authorized"
	needsReauthorizationState = "needs_reauthorization"
)

// reportedAuthorization is what the status view says about one slot. A slot that
// is already authorised stays so while a fresh flow runs; its token still works.
func reportedAuthorization(stored string, inFlight bool) string {
	if inFlight && (stored == notAuthorizedState || stored == needsReauthorizationState) {
		return pendingState
	}

	return stored
}

// The words for work that has not finished. None may be replaced by the outcome
// of an earlier run.
const (
	queuedState  = "queued"
	runningState = "running"
	delayedState = "delayed"
)

// liveSyncState names the state of a run that has not finished, false when none
// is under way. A run outranks a delay.
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

// GetStatus reports readiness, per-target convergence, and the last terminal run.
func (h *Handler) GetStatus(writer http.ResponseWriter, request *http.Request) {
	// One snapshot for the whole response, so the list of targets and the count of
	// them cannot disagree mid-assembly.
	targetIDs := h.targetIDs()
	authorizations := make(map[string]string, len(targetIDs))
	if err := h.state.ForEachTarget(request.Context(), func(id, authorizationState string) error {
		if slices.Contains(targetIDs, id) {
			authorizations[id] = authorizationState
		}

		return nil
	}); err != nil {
		h.unavailable(writer)

		return
	}

	inFlight := make(map[string]bool, len(targetIDs))
	if err := h.state.ForEachPendingAuthorization(request.Context(), func(targetID string) error {
		inFlight[targetID] = true

		return nil
	}); err != nil {
		h.unavailable(writer)

		return
	}

	routeCounts, err := h.targetRouteCounts(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}
	runs, err := h.targetRuns(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}

	targets := make([]openapi.TargetStatus, 0, len(targetIDs))
	// The aggregate of the per-target counts, which is the only progress a run in
	// flight reports.
	allRoutes := openapi.TargetRoutes{}
	ready, converged := true, true
	for _, targetID := range targetIDs {
		stored, found := authorizations[targetID]
		if !found {
			h.unavailable(writer)

			return
		}
		authorization := reportedAuthorization(stored, inFlight[targetID])
		routes := routeCounts[targetID]
		var lastRun *openapi.TargetRun
		if run, recorded := runs[targetID]; recorded {
			lastRun = &run
		}
		convergence := convergenceState(authorization, routes, lastRun)
		targets = append(targets, openapi.TargetStatus{
			ID:            targetID,
			Authorisation: authorization,
			Convergence:   convergence,
			Routes:        routes,
			LastRun:       lastRun,
		})
		allRoutes.Current += routes.Current
		allRoutes.Pending += routes.Pending
		ready = ready && authorization == authorizedState
		// Overall convergence is the conjunction, so one lagging slot is enough
		// to say the library is not everywhere it belongs.
		converged = converged && convergence == convergenceCurrent
	}

	activity := h.syncRuns.Activity()
	view := openapi.Status{
		Ready:     ready,
		Converged: converged,
		Targets:   targets,
		Sync:      openapi.SyncStatus{State: "not_ready"},
	}
	// Only when the revision is known: the digest alone names an image without
	// saying what is in it.
	if h.buildRevision != "" {
		view.Build = &openapi.Build{Revision: h.buildRevision, ImageDigest: optionalString(h.buildImageDigest)}
	}
	if ready {
		view.Sync.State = "idle"
	}
	scheduleSource, scheduleTargets, err := h.state.SyncSchedule(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}
	view.Sync.Schedule = openapi.SyncSchedule{Source: scheduleSource, Targets: scheduleTargets}
	classified, total, coverageErr := h.state.SurfaceCoverage(request.Context())
	if coverageErr != nil {
		h.unavailable(writer)

		return
	}
	view.Sync.Surface = openapi.SurfaceCoverage{
		Classified: classified, Total: total, Incomplete: h.syncRuns.SurfaceIncomplete(),
	}
	if remaining, resetAt, known := h.syncRuns.RateLimit(); known {
		view.Sync.WahooRateLimit = &openapi.WahooRateLimit{
			Remaining: remaining,
			ResetsAt:  optionalTime(resetAt),
		}
	}
	if h.surfaceIndex != nil {
		if generation, builtAt, ok := h.surfaceIndex(); ok {
			view.Sync.Surface.Generation = optionalString(generation)
			view.Sync.Surface.BuiltAt = optionalTime(builtAt)
		}
	}
	if phaseErr := h.state.ForEachPhaseRun(request.Context(), func(
		phase string, completedAt time.Time, outcome, detail string,
		sourceStages, created, updated, deleted int,
	) error {
		run := openapi.SyncPhaseRun{
			LastCompletedAt: wireTime(completedAt),
			LastResult:      outcome,
			LastFailure:     optionalString(detail),
			SourceRoutes:    sourceStages,
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
	if h.settings.Values().Sync.StaleAfter > 0 {
		trustedInventory, inventoryErr := h.trustedInventoryFreshness(request.Context())
		if inventoryErr != nil {
			h.unavailable(writer)

			return
		}
		view.Sync.TrustedInventory = trustedInventory
	}
	completedAt, outcome, _, sourceStages, created, updated, deleted, found, err := h.state.LastSyncRun(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}
	if found {
		view.Sync.State, view.Sync.LastResult = outcome, optionalString(outcome)
		view.Sync.LastCompletedAt = optionalTime(completedAt)
		view.Sync.SourceRoutes, view.Sync.Created, view.Sync.Updated, view.Sync.Deleted =
			sourceStages, created, updated, deleted
	}
	// A run that has not finished outranks the last one that did: reporting
	// "succeeded" mid-run would describe the previous run with nothing saying so.
	if state, live := liveSyncState(activity); live {
		view.Sync.State = state
		view.Sync.Active = &openapi.SyncActive{
			Targets: len(targetIDs),
			Routes:  allRoutes,
		}
		if activity.Phase != "" {
			phase := openapi.SyncActive_Phase(activity.Phase)
			view.Sync.Active.Phase = &phase
		}
		// Only a delay has a due instant to report. A run under way now would read
		// another's start time as its own progress.
		if state == delayedState {
			view.Sync.Active.StartsAt = optionalTime(activity.StartsAt)
		}
	}
	h.writeJSON(writer, http.StatusOK, view)
}

// SetSyncSchedule switches either half of the scheduled synchronization on or
// off. It changes nothing about a run already in flight, and never starts one.
func (h *Handler) SetSyncSchedule(writer http.ResponseWriter, request *http.Request) {
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
	// One object, and nothing after it: a body carrying a second value means the
	// caller sent something this service never read.
	if decoder.More() {
		h.error(writer, http.StatusBadRequest, "invalid_request", "the request body must be one object")

		return
	}
	if err := h.state.SetSyncSchedule(request.Context(), *body.Source, *body.Targets); err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, openapi.SyncSchedule{Source: *body.Source, Targets: *body.Targets})
}

// The recorded history is served a page at a time. The ceiling keeps one request
// from reading the whole retained window.
const (
	defaultSyncRunPage = 20
	maximumSyncRunPage = 100
)

// GetSyncRuns serves one page of the recorded run history, newest first, with the
// cursor for the next. Local records only: no route names, no provider calls.
func (h *Handler) GetSyncRuns(writer http.ResponseWriter, request *http.Request) {
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
	view := openapi.SyncRunPage{Runs: []openapi.SyncRun{}}
	next, usable, err := h.state.ForEachSyncRun(request.Context(), query.Get("after"), limit, func(
		reference, phase string, completedAt time.Time, outcome, detail string,
		sourceStages, created, updated, deleted int,
	) error {
		view.Runs = append(view.Runs, openapi.SyncRun{
			Reference:    reference,
			Phase:        openapi.SyncRun_Phase(phase),
			CompletedAt:  wireTime(completedAt),
			Result:       outcome,
			Failure:      optionalString(detail),
			SourceRoutes: sourceStages,
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
	view.Next = optionalString(next)
	h.writeJSON(writer, http.StatusOK, view)
}

// TriggerSync queues one immediate run through the same reporting path as the schedule.
func (h *Handler) TriggerSync(writer http.ResponseWriter, _ *http.Request) {
	h.trigger(writer, SyncPhaseAll)
}

// TriggerSourceSync queues one immediate read of the source library. It runs
// whether or not the schedule is allowed to start that phase.
func (h *Handler) TriggerSourceSync(writer http.ResponseWriter, _ *http.Request) {
	h.trigger(writer, SyncPhaseSource)
}

// TriggerTargetsSync queues one immediate reconciliation of stored state onto the
// targets, on the same terms as syncSource.
func (h *Handler) TriggerTargetsSync(writer http.ResponseWriter, _ *http.Request) {
	h.trigger(writer, SyncPhaseTargets)
}

// TriggerTargetSync queues one immediate reconciliation onto exactly one target,
// on syncTargets' terms scoped to that slot. An unconfigured slot is not found.
func (h *Handler) TriggerTargetSync(writer http.ResponseWriter, request *http.Request) {
	targetID := request.PathValue("target")
	if targetID == "" || !slices.Contains(h.targetIDs(), targetID) {
		h.notFound(writer)

		return
	}
	if !h.syncRuns.TriggerTarget(targetID) {
		h.error(writer, http.StatusConflict, "sync_in_progress", "a synchronization is already running")

		return
	}
	h.writeJSON(writer, http.StatusAccepted, openapi.Accepted{Status: "accepted"})
}

// ClearTarget queues the deletion of every route this service owns on exactly one
// configured target. An unconfigured slot is not found. It carries no body.
func (h *Handler) ClearTarget(writer http.ResponseWriter, request *http.Request) {
	targetID := request.PathValue("target")
	if targetID == "" || !slices.Contains(h.targetIDs(), targetID) {
		h.notFound(writer)

		return
	}
	if !h.syncRuns.TriggerClear(targetID) {
		h.error(writer, http.StatusConflict, "sync_in_progress", "a synchronization is already running")

		return
	}
	h.writeJSON(writer, http.StatusAccepted, openapi.Accepted{Status: "accepted"})
}

func (h *Handler) trigger(writer http.ResponseWriter, phase SyncPhase) {
	if !h.syncRuns.Trigger(phase) {
		h.error(writer, http.StatusConflict, "sync_in_progress", "a synchronization is already running")

		return
	}
	h.writeJSON(writer, http.StatusAccepted, openapi.Accepted{Status: "accepted"})
}

// TriggerSurfaceSync queues one classification pass, independent of either half.
// It reads no source and writes no target, and shares their single-flight guard.
func (h *Handler) TriggerSurfaceSync(writer http.ResponseWriter, _ *http.Request) {
	if !h.syncRuns.TriggerAnnotate() {
		h.error(writer, http.StatusConflict, "sync_in_progress", "a synchronization or classification pass is already running")

		return
	}
	h.writeJSON(writer, http.StatusAccepted, openapi.Accepted{Status: "accepted"})
}

// trustedInventoryFreshness reports the inventory's age against the configured
// bound from local state. Absent last success is fresh, not stale.
func (h *Handler) trustedInventoryFreshness(ctx context.Context) (*openapi.TrustedInventory, error) {
	view := &openapi.TrustedInventory{Fresh: true, MaxAgeSeconds: int(h.settings.Values().Sync.StaleAfter / time.Second)}
	lastSuccess, found, err := h.state.LastSuccessfulPhaseCompletion(ctx, string(SyncPhaseSource))
	if err != nil {
		return nil, fmt.Errorf("reading the trusted inventory's last success: %w", err)
	}
	if !found {
		return view, nil
	}
	// Clamped rather than reported negative: a backwards clock must not be read
	// here as a claim about the future.
	age := max(h.now().Sub(lastSuccess), 0)
	ageSeconds := int(age / time.Second)
	view.LastSuccessAt = optionalTime(lastSuccess)
	view.AgeSeconds = ageSeconds
	// Fresh is derived from the same truncated seconds the response reports, so a
	// sub-second bound cannot make fresh disagree with the numbers beside it.
	view.Fresh = ageSeconds < view.MaxAgeSeconds

	return view, nil
}

func (h *Handler) unavailable(writer http.ResponseWriter) {
	h.error(writer, http.StatusServiceUnavailable, "unavailable", "service state is unavailable")
}
