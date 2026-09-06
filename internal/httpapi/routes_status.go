package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
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
	targetIDs, err := h.targetIDs(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}
	admin := identityOf(request.Context()).Admin
	// A set rather than a repeated slices.Contains scan: self-service targets
	// are unbounded, so this membership check has to stay O(1) per row.
	wanted := make(map[string]struct{}, len(targetIDs))
	for _, id := range targetIDs {
		wanted[id] = struct{}{}
	}
	authorizations := make(map[string]string, len(targetIDs))
	owners := make(map[string]string, len(targetIDs))
	if targetErr := h.state.ForEachTarget(request.Context(), func(id, authorizationState, ownerSubject string) error {
		if _, found := wanted[id]; found {
			authorizations[id] = authorizationState
			owners[id] = ownerSubject
		}

		return nil
	}); targetErr != nil {
		h.unavailable(writer)

		return
	}

	inFlight := make(map[string]bool, len(targetIDs))
	if pendingErr := h.state.ForEachPendingAuthorization(request.Context(), func(targetID string) error {
		inFlight[targetID] = true

		return nil
	}); pendingErr != nil {
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
		status := openapi.TargetStatus{
			ID:            targetID,
			Authorisation: authorization,
			Convergence:   convergence,
			Routes:        routes,
			LastRun:       lastRun,
		}
		// Only an admin sees who owns a target: a non-admin's own is already
		// known to be theirs, and never sees another's here at all.
		if admin {
			status.Owner = optionalString(owners[targetID])
			status.Own = optionalBool(owners[targetID] == identityOf(request.Context()).Subject)
		}
		targets = append(targets, status)
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
	classified, total, coverageErr := h.state.SurfaceCoverage(request.Context())
	if coverageErr != nil {
		h.unavailable(writer)

		return
	}
	enrichmentFailures, enrichmentErr := h.state.CountStageEnrichmentFailures(request.Context())
	if enrichmentErr != nil {
		h.unavailable(writer)

		return
	}
	view.Sync.Surface = openapi.SurfaceCoverage{
		Classified: classified, Total: total, Incomplete: h.syncRuns.SurfaceIncomplete(),
		EnrichmentFailures: enrichmentFailures,
	}
	if remaining, resetAt, observedAt, known := h.syncRuns.RateLimit(); known {
		view.Sync.WahooRateLimit = &openapi.WahooRateLimit{
			Remaining:  remaining,
			ResetsAt:   optionalTime(resetAt),
			ObservedAt: observedAt,
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

// The recorded history is served a page at a time. The ceiling keeps one request
// from reading the whole retained window.
const (
	defaultRunPage = 20
	maximumRunPage = 100
)

// pageLimit reads the page size one request asks for, answering the caller
// itself when the number is one this service will not serve.
func (h *Handler) pageLimit(writer http.ResponseWriter, query url.Values) (int, bool) {
	raw := query.Get("limit")
	if raw == "" {
		return defaultRunPage, true
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit < 1 || limit > maximumRunPage {
		h.error(writer, http.StatusBadRequest, "invalid_request",
			"limit must be between 1 and "+strconv.Itoa(maximumRunPage))

		return 0, false
	}

	return limit, true
}

// GetSyncRuns serves one page of the recorded run history, newest first, with the
// cursor for the next. Local records only: no route names, no provider calls.
func (h *Handler) GetSyncRuns(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	limit, ok := h.pageLimit(writer, query)
	if !ok {
		return
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
