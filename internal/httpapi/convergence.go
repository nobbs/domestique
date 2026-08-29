package httpapi

import (
	"context"
	"fmt"
	"slices"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
)

// The one word each target gets, in decreasing order of what an operator has to
// act on. They are aliases for the contract's own enum members, so dropping one
// from api/openapi.yaml stops this file compiling.
const (
	// convergenceUnauthorized means the slot has never completed, or has lost,
	// its one-time browser onboarding. Nothing can be written until it does.
	convergenceUnauthorized = openapi.TargetStatus_ConvergenceUnauthorized
	// convergenceFailed means this slot's own last reconciliation did not
	// succeed. The reason is the safe category in its last run, which for a
	// blocked run is a safety gate holding rather than a fault.
	convergenceFailed = openapi.TargetStatus_ConvergenceFailed
	// convergenceLagging means the slot is onboarded and its last run was fine,
	// but stored routes are still owed to it.
	convergenceLagging = openapi.TargetStatus_ConvergenceLagging
	// convergenceCurrent means the Wahoo account holds every stored stage at the
	// revision the library holds now. It is not a claim that any head unit has
	// downloaded them.
	convergenceCurrent = openapi.TargetStatus_ConvergenceCurrent
)

// succeededOutcome is the one run result that is not something to look into. It
// is restated here rather than imported so this package keeps knowing nothing
// about how synchronization is implemented.
const succeededOutcome = "succeeded"

// sourceStageKey identifies one stored stage. It is the join key between the
// library and what a target was last written at.
type sourceStageKey struct {
	provider   route.Provider
	routeID    int64
	stageOrder int
}

// targetRouteCounts derives, for each configured target, how much of the stored
// library it holds and how much it owes. It compares two local tables and nothing
// else, so a status request answers while a provider is down. Only the source
// revision is compared, not the content hash: the hash a target records is the
// encoded course's, and this layer has no business knowing how one is encoded.
func (h *Handler) targetRouteCounts(ctx context.Context) (map[string]openapi.TargetRoutes, error) {
	revisions := make(map[sourceStageKey]string)
	if err := h.state.ForEachSourceStage(
		ctx,
		func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, _ string) error {
			revisions[sourceStageKey{provider: provider, routeID: routeID, stageOrder: stageOrder}] = sourceRevision

			return nil
		},
	); err != nil {
		return nil, fmt.Errorf("read stored routes: %w", err)
	}

	targetIDs := h.targetIDs()
	counts := make(map[string]openapi.TargetRoutes, len(targetIDs))
	for _, targetID := range targetIDs {
		var current, orphaned int
		if err := h.state.ForEachTargetStage(
			ctx,
			targetID,
			func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, _ string, _ int64) error {
				stored, tracked := revisions[sourceStageKey{provider: provider, routeID: routeID, stageOrder: stageOrder}]
				if !tracked {
					// The library no longer has this stage, so the target is
					// carrying a route the next reconciliation will remove. That
					// removal is outstanding work like any other.
					orphaned++

					return nil
				}
				if stored == sourceRevision {
					current++
				}

				return nil
			},
		); err != nil {
			return nil, fmt.Errorf("read applied routes: %w", err)
		}
		counts[targetID] = openapi.TargetRoutes{
			Current: current,
			Pending: len(revisions) - current + orphaned,
		}
	}

	return counts, nil
}

// targetRuns reads each configured target's own last reconciliation. A slot with
// no row has never been reconciled, and is reported as absent rather than as a
// run that succeeded with nothing to do.
func (h *Handler) targetRuns(ctx context.Context) (map[string]openapi.TargetRun, error) {
	targetIDs := h.targetIDs()
	runs := make(map[string]openapi.TargetRun, len(targetIDs))
	if err := h.state.ForEachTargetRun(
		ctx,
		func(targetID string, finishedAt time.Time, outcome, detail string) error {
			// A slot left over from an earlier configuration is not part of this
			// deployment and is not reported.
			if slices.Contains(targetIDs, targetID) {
				runs[targetID] = openapi.TargetRun{
					CompletedAt: wireTime(finishedAt),
					Result:      outcome,
					Failure:     optionalString(detail),
				}
			}

			return nil
		},
	); err != nil {
		return nil, fmt.Errorf("read target runs: %w", err)
	}

	return runs, nil
}

// convergenceState reduces one target to the word that describes it.
func convergenceState(
	authorization string, routes openapi.TargetRoutes, run *openapi.TargetRun,
) openapi.TargetStatus_Convergence {
	if authorization != authorizedState {
		return convergenceUnauthorized
	}
	// A failed or blocked attempt is reported even when nothing is outstanding:
	// the account may hold the library and still have stopped being writable,
	// and that is the case an operator most needs to see before the next change.
	if run != nil && run.Result != succeededOutcome {
		return convergenceFailed
	}
	if routes.Pending > 0 {
		return convergenceLagging
	}

	return convergenceCurrent
}
