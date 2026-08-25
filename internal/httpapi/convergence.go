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
// act on. They are a summary of the counts and the last run beside them, so a
// reader that wants the detail already has it.
const (
	// convergenceUnauthorized means the slot has never completed, or has lost,
	// its one-time browser onboarding. Nothing can be written until it does.
	convergenceUnauthorized = "unauthorized"
	// convergenceFailed means this slot's own last reconciliation did not
	// succeed. The reason is the safe category in its last run, which for a
	// blocked run is a safety gate holding rather than a fault.
	convergenceFailed = "failed"
	// convergenceLagging means the slot is onboarded and its last run was fine,
	// but stored stages are still owed to it.
	convergenceLagging = "lagging"
	// convergenceCurrent means the Wahoo account holds every stored stage at the
	// revision the library holds now. It is not a claim that any head unit has
	// downloaded them.
	convergenceCurrent = "current"
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

// targetStageCounts derives, for each configured target, how much of the stored
// library that target already holds and how much it still owes.
//
// It is a comparison of two local tables and nothing else: the revision each
// stage is stored at, against the revision each target was last successfully
// written at. Wahoo is never asked, because a status request must answer while a
// provider is down, and because the honest question here is what this service
// has applied — what a device has since fetched is not observable from here.
//
// Only the source revision is compared, not the content hash: the hash a target
// records is the encoded course's, derived by the layer that writes it, and this
// layer has no business knowing how a course is encoded. A stage whose content
// changed changes revision with it.
func (h *Handler) targetStageCounts(ctx context.Context) (map[string]openapi.TargetStages, error) {
	revisions := make(map[sourceStageKey]string)
	if err := h.state.ForEachSourceStage(
		ctx,
		func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, _ string) error {
			revisions[sourceStageKey{provider: provider, routeID: routeID, stageOrder: stageOrder}] = sourceRevision

			return nil
		},
	); err != nil {
		return nil, fmt.Errorf("read stored stages: %w", err)
	}

	counts := make(map[string]openapi.TargetStages, len(h.targetIDs))
	for _, targetID := range h.targetIDs {
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
			return nil, fmt.Errorf("read applied stages: %w", err)
		}
		counts[targetID] = openapi.TargetStages{
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
	runs := make(map[string]openapi.TargetRun, len(h.targetIDs))
	if err := h.state.ForEachTargetRun(
		ctx,
		func(targetID string, finishedAt time.Time, outcome, detail string) error {
			// A slot left over from an earlier configuration is not part of this
			// deployment and is not reported.
			if slices.Contains(h.targetIDs, targetID) {
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
func convergenceState(authorization string, stages openapi.TargetStages, run *openapi.TargetRun) string {
	if authorization != authorizedState {
		return convergenceUnauthorized
	}
	// A failed or blocked attempt is reported even when nothing is outstanding:
	// the account may hold the library and still have stopped being writable,
	// and that is the case an operator most needs to see before the next change.
	if run != nil && run.Result != succeededOutcome {
		return convergenceFailed
	}
	if stages.Pending > 0 {
		return convergenceLagging
	}

	return convergenceCurrent
}
