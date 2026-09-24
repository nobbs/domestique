package sync

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	gosync "sync"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

// DeliveryState is where one target stands with one plan.
type DeliveryState string

const (
	// DeliveryCurrent means the target holds the plan's current revision.
	DeliveryCurrent DeliveryState = "current"
	// DeliveryPending means a write or removal is owed and not yet refused.
	DeliveryPending DeliveryState = "pending"
	// DeliveryFailed means the last push of this revision, or the removal,
	// failed; Failure says why.
	DeliveryFailed DeliveryState = "failed"
	// DeliveryAbsent means the target holds no copy and is owed none.
	DeliveryAbsent DeliveryState = "absent"
)

// Delivery is one target's standing with one plan.
type Delivery struct {
	// DeliveredAt is when this process last wrote the copy the target holds;
	// zero when it was written before the last restart or by a full run.
	DeliveredAt time.Time
	TargetID    string
	State       DeliveryState
	Failure     FailureCategory
}

// planAttempt is the last push of one plan to one target. An empty revision
// was a removal.
type planAttempt struct {
	at       time.Time
	revision string
	failure  FailureCategory
}

type attemptKey struct {
	targetID string
	planID   int64
}

// planAttempts remembers each target's last push of each plan. Memory only:
// after a restart a failed push reads as pending until the sweep asks again.
type planAttempts struct {
	byKey map[attemptKey]planAttempt
	// runs holds each push's own outcome by the plan it named, zero for every
	// plan, so a push that failed before reaching any target still reads failed.
	runs  map[int64]planAttempt
	mutex gosync.Mutex
}

func (a *planAttempts) recordRun(planID int64, attempt planAttempt) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.runs == nil {
		a.runs = make(map[int64]planAttempt)
	}
	a.runs[planID] = attempt
}

// lastRun is the newer of the last push naming this plan and the last sweep.
func (a *planAttempts) lastRun(planID int64) (planAttempt, bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	named, namedFound := a.runs[planID]
	sweep, sweepFound := a.runs[0]
	if !sweepFound || (namedFound && named.at.After(sweep.at)) {
		return named, namedFound
	}

	return sweep, true
}

func (a *planAttempts) record(targetID string, planID int64, attempt planAttempt) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	if a.byKey == nil {
		a.byKey = make(map[attemptKey]planAttempt)
	}
	a.byKey[attemptKey{targetID: targetID, planID: planID}] = attempt
}

func (a *planAttempts) last(targetID string, planID int64) (planAttempt, bool) {
	a.mutex.Lock()
	defer a.mutex.Unlock()
	attempt, found := a.byKey[attemptKey{targetID: targetID, planID: planID}]

	return attempt, found
}

// RunPlans pushes plans alone: it reads the local source into the stored
// inventory, then writes or removes only the plans each target is stale on,
// contacting no target that is current. A planID of zero is every plan. The
// rest of the library is left to the full runs. SourceStored reports whether
// the stored plans changed.
func (s *Service) RunPlans(ctx context.Context, planID int64) Result {
	result, unreached := s.runPlans(ctx, planID)
	s.attempts.recordRun(planID, planAttempt{at: s.now().UTC(), failure: unreached})

	return result
}

// runPlans also reports the failure that stopped it before any plan's own
// attempt was recorded, which is the only failure no target's row carries.
func (s *Service) runPlans(ctx context.Context, planID int64) (Result, FailureCategory) {
	source, configured, err := s.sourceFor(route.ProviderLocal)
	if err != nil {
		return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}, FailureState
	}
	if !configured {
		return Result{Phase: PhaseTargets, Outcome: OutcomeNotReady}, FailureNone
	}
	outcome, failure, _, changed := s.runOneSource(ctx, source, route.ProviderLocal)
	if failure != FailureNone {
		return Result{Phase: PhaseTargets, Outcome: outcome, Failure: failure}, failure
	}
	stored, err := s.state.TrustedInventory(ctx)
	if err != nil {
		return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}, FailureState
	}
	desired, _, err := normalizeInventory(stored)
	if err != nil {
		return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}, FailureState
	}

	result := Result{Phase: PhaseTargets, SourceStored: changed}
	unreached := FailureNone
	for _, targetID := range s.targetIDs() {
		push := s.pushPlans(ctx, targetID, desired, planID)
		if !push.contacted {
			continue
		}
		s.label(ctx, targetID)
		applied, failure := push.applied, push.failure
		if !push.attempted && unreached == FailureNone {
			unreached = failure
		}
		result.Created += applied.created
		result.Updated += applied.updated
		result.Deleted += applied.deleted
		result.Targets = append(result.Targets, TargetResult{ID: targetID, Outcome: targetOutcome(failure), Failure: failure})
		if failure == FailureNone {
			continue
		}
		if failure == FailureDeletionLimit && (result.Outcome == "" || result.Outcome == OutcomeBlocked) {
			result.Outcome = OutcomeBlocked
		} else {
			result.Outcome = OutcomeFailed
		}
		if result.Failure == FailureNone {
			result.Failure = failure
		}
	}
	switch {
	case len(result.Targets) == 0:
		result.Outcome = OutcomeSkipped
	case result.Outcome == "":
		result.Outcome = OutcomeSucceeded
	}

	return result, unreached
}

// planPush is one target's share of a push. Contacted is whether the target
// was stale at all; attempted, whether its plans' own attempts were recorded.
type planPush struct {
	failure   FailureCategory
	applied   counts
	contacted bool
	attempted bool
}

// pushPlans brings one target's plans in line with the stored inventory.
func (s *Service) pushPlans(
	ctx context.Context, targetID string, desired map[route.Key]route.Route, planID int64,
) planPush {
	authorization, err := s.state.TargetAuthorization(ctx, targetID)
	if err != nil {
		return planPush{failure: FailureState, contacted: true}
	}
	// Reconnecting is the rider's to do; asking Wahoo again changes nothing.
	if authorization != authorizedState {
		return planPush{}
	}
	mappings, err := s.targetStages(ctx, targetID)
	if err != nil {
		return planPush{failure: FailureState, contacted: true}
	}
	writes, removals := stalePlans(mappings, desired, planID)
	if len(writes) == 0 && len(removals) == 0 {
		return planPush{}
	}

	push := planPush{contacted: true, attempted: true}
	push.failure = s.writePlans(ctx, targetID, writes, removals, mappings, &push.applied)

	return push
}

// writePlans applies the stale plans and records each one's attempt. Once a
// step fails the rest are recorded with its failure, not attempted.
func (s *Service) writePlans(
	ctx context.Context, targetID string, writes []route.Route, removals []route.Key,
	mappings map[route.Key]targetStage, result *counts,
) FailureCategory {
	failure := FailureNone
	if len(removals) > maxDeletionsPerTarget {
		failure = FailureDeletionLimit
	}
	var accessToken string
	var owned map[string]int64
	if failure == FailureNone {
		accessToken, failure = s.accessToken(ctx, targetID)
	}
	if failure == FailureNone {
		var listErr error
		if owned, listErr = s.target.ListOwnedRoutes(ctx, accessToken); listErr != nil {
			failure = s.handleTargetError(ctx, targetID, listErr)
		}
	}
	for index := range writes {
		stage := &writes[index]
		if failure == FailureNone {
			failure = s.applyStage(ctx, targetID, accessToken, stage, mappings, owned, result)
		}
		s.attempts.record(targetID, stage.Key().SourceRouteID(), planAttempt{
			at: s.now().UTC(), revision: stage.Revision(), failure: failure,
		})
	}
	for _, key := range removals {
		if failure == FailureNone {
			failure = s.removeStage(ctx, targetID, accessToken, key, owned, result)
		}
		s.attempts.record(targetID, key.SourceRouteID(), planAttempt{at: s.now().UTC(), failure: failure})
	}

	return failure
}

// stalePlans lists the plans one target is owed a write of, and the recorded
// plans the inventory no longer holds, in key order.
func stalePlans(
	mappings map[route.Key]targetStage, desired map[route.Key]route.Route, planID int64,
) ([]route.Route, []route.Key) {
	wanted := func(key route.Key) bool {
		return key.Provider() == route.ProviderLocal && (planID == 0 || key.SourceRouteID() == planID)
	}
	var writes []route.Route
	for key, stage := range desired {
		if !wanted(key) {
			continue
		}
		recorded, tracked := mappings[key]
		if tracked && recorded.sourceRevision == stage.Revision() && recorded.contentHash == encodedContentHash(&stage) {
			continue
		}
		writes = append(writes, stage)
	}
	slices.SortFunc(writes, func(left, right route.Route) int {
		return compareKeys(left.Key(), right.Key())
	})
	var removals []route.Key
	for key := range mappings {
		if _, kept := desired[key]; wanted(key) && !kept {
			removals = append(removals, key)
		}
	}
	slices.SortFunc(removals, compareKeys)

	return writes, removals
}

// compareKeys orders plans by id; a plan is always a single stage.
func compareKeys(left, right route.Key) int {
	return cmp.Compare(left.SourceRouteID(), right.SourceRouteID())
}

// PlanDelivery reports each target's standing with one plan, against the
// revision the target should hold: empty when it should hold none, as for a
// draft or a deleted plan. It asks no target anything.
func (s *Service) PlanDelivery(ctx context.Context, planID int64, revision string) ([]Delivery, error) {
	key := route.NewKey(route.ProviderLocal, planID, 1)
	targetIDs := s.targetIDs()
	deliveries := make([]Delivery, 0, len(targetIDs))
	for _, targetID := range targetIDs {
		authorization, err := s.state.TargetAuthorization(ctx, targetID)
		if err != nil {
			return nil, fmt.Errorf("reading target authorization: %w", err)
		}
		mappings, err := s.targetStages(ctx, targetID)
		if err != nil {
			return nil, err
		}
		recorded, tracked := mappings[key]
		attempt, attempted := s.attempts.last(targetID, planID)
		result := delivery(targetID, authorization, revision, recorded, tracked, attempt, attempted)
		// A push that failed before reaching this target leaves it owed, not
		// sending: nothing will send it until the next push.
		if run, ran := s.attempts.lastRun(planID); result.State == DeliveryPending && ran && run.failure != FailureNone {
			result.State, result.Failure = DeliveryFailed, run.failure
		}
		deliveries = append(deliveries, result)
	}

	return deliveries, nil
}

func delivery(
	targetID, authorization, revision string, recorded targetStage, tracked bool,
	attempt planAttempt, attempted bool,
) Delivery {
	result := Delivery{TargetID: targetID, State: DeliveryPending}
	switch {
	case revision == "" && !tracked:
		result.State = DeliveryAbsent
	case revision != "" && tracked && recorded.sourceRevision == revision:
		result.State = DeliveryCurrent
		if attempted && attempt.revision == revision && attempt.failure == FailureNone {
			result.DeliveredAt = attempt.at
		}
	case authorization != authorizedState:
		result.State, result.Failure = DeliveryFailed, FailureAuthorization
	case attempted && attempt.revision == revision && attempt.failure != FailureNone:
		result.State, result.Failure = DeliveryFailed, attempt.failure
	}

	return result
}
