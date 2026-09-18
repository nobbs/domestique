package sync

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func planStage(t *testing.T, planID int64, revision string) route.Route {
	t.Helper()

	return testProviderStage(t, route.ProviderLocal, planID, 1, revision, "hash-"+revision)
}

// rewindTokens undoes the rotation a run performed, because the fake target
// files each account's routes under the access token that wrote them.
func rewindTokens(state *fakeState) {
	for targetID := range state.refreshTokens {
		state.refreshTokens[targetID] = targetID
	}
}

func newPlanService(t *testing.T, state *fakeState, target *fakeTarget, sources ...Source) *Service {
	t.Helper()
	service, err := New(
		syncOptions(false, sources, "a", "b"),
		state, identityProcessor{}, &fakeEncoder{}, target, nil, nil,
	)
	require.NoError(t, err, "New()")
	service.now = func() time.Time { return time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC) }

	return service
}

// A push writes the one plan it names and nothing else, even where the library
// and another plan are just as stale on the same target.
func TestRunPlansWritesOnlyTheNamedPlan(t *testing.T) {
	library := &fakeSource{stages: []route.Route{testStage(t, 7, 1, "lib", "lib-hash")}}
	pushed, other := planStage(t, 1, "r1"), planStage(t, 2, "r1")
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{pushed, other}}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newPlanService(t, state, target, library, local)

	result := service.RunPlans(t.Context(), 1)

	assert.Equal(t, OutcomeSucceeded, result.Outcome, "RunPlans() outcome")
	assert.Equal(t, 2, result.Created, "one creation per target")
	for _, targetID := range []string{"a", "b"} {
		assert.Contains(t, target.routes[accessFor(targetID)], pushed.Key().ExternalID(), "pushed plan on %s", targetID)
		assert.NotContains(t, target.routes[accessFor(targetID)], other.Key().ExternalID(), "other plan on %s", targetID)
		assert.Len(t, target.routes[accessFor(targetID)], 1, "routes on %s", targetID)
	}
	assert.Zero(t, library.calls, "the library is not read")
}

// The push stores the plan in the trusted inventory, so a full reconciliation
// with the source read switched off keeps the copy rather than deleting it.
func TestRunPlansSurvivesAReconciliationWithoutASourceRead(t *testing.T) {
	pushed := planStage(t, 1, "r1")
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{pushed}}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newPlanService(t, state, target, local)
	require.Equal(t, OutcomeSucceeded, service.RunPlans(t.Context(), 1).Outcome, "RunPlans()")
	rewindTokens(state)

	result := service.RunTargets(t.Context())

	assert.Equal(t, OutcomeSucceeded, result.Outcome, "RunTargets() outcome")
	assert.Zero(t, result.Deleted, "deleted")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
}

// A plan every target already holds costs no Wahoo request at all.
func TestRunPlansLeavesACurrentTargetUncontacted(t *testing.T) {
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{planStage(t, 1, "r1")}}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newPlanService(t, state, target, local)
	require.Equal(t, OutcomeSucceeded, service.RunPlans(t.Context(), 0).Outcome, "first RunPlans()")
	rewindTokens(state)
	refreshes, listings := len(target.refreshTokens), target.listCalls

	result := service.RunPlans(t.Context(), 0)

	assert.Equal(t, OutcomeSkipped, result.Outcome, "second RunPlans() outcome")
	assert.Len(t, target.refreshTokens, refreshes, "token refreshes")
	assert.Equal(t, listings, target.listCalls, "route listings")
}

// A plan saved again is rewritten in place, not created a second time.
func TestRunPlansUpdatesARevisedPlan(t *testing.T) {
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{planStage(t, 1, "r1")}}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newPlanService(t, state, target, local)
	require.Equal(t, OutcomeSucceeded, service.RunPlans(t.Context(), 1).Outcome, "first RunPlans()")
	rewindTokens(state)
	local.stages = []route.Route{planStage(t, 1, "r2")}

	result := service.RunPlans(t.Context(), 1)

	assert.Equal(t, 2, result.Updated, "updated")
	assert.Zero(t, result.Created, "created")
}

// A copy the target already owns under the plan's external ID is adopted, not
// duplicated: ownership is asked before anything is created.
func TestRunPlansAdoptsAnOwnedCopy(t *testing.T) {
	pushed := planStage(t, 1, "r1")
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{pushed}}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	target.seedRoute("a", &pushed, 55)
	service := newPlanService(t, state, target, local)

	result := service.RunPlans(t.Context(), 1)

	assert.Equal(t, 1, result.Created, "created only where no copy existed")
	assert.Equal(t, int64(55), state.mappings["a"][pushed.Key()].wahooRouteID, "adopted mapping")
}

// Unpublishing removes that plan's owned copy and leaves every other route,
// the library's included, where it is.
func TestRunPlansRemovesOnlyTheWithdrawnPlan(t *testing.T) {
	libraryStage := testStage(t, 7, 1, "lib", "lib-hash")
	library := &fakeSource{stages: []route.Route{libraryStage}}
	withdrawn, kept := planStage(t, 1, "r1"), planStage(t, 2, "r1")
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{withdrawn, kept}}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newPlanService(t, state, target, library, local)
	require.Equal(t, OutcomeSucceeded, runBoth(t.Context(), service).Outcome, "full run")
	rewindTokens(state)
	local.stages = []route.Route{kept}

	result := service.RunPlans(t.Context(), 1)

	assert.Equal(t, OutcomeSucceeded, result.Outcome, "RunPlans() outcome")
	assert.Equal(t, 2, result.Deleted, "one removal per target")
	for _, targetID := range []string{"a", "b"} {
		routes := target.routes[accessFor(targetID)]
		assert.NotContains(t, routes, withdrawn.Key().ExternalID(), "withdrawn plan on %s", targetID)
		assert.Contains(t, routes, kept.Key().ExternalID(), "kept plan on %s", targetID)
		assert.Contains(t, routes, libraryStage.Key().ExternalID(), "library route on %s", targetID)
		assert.NotContains(t, state.mappings[targetID], withdrawn.Key(), "withdrawn mapping on %s", targetID)
	}
}

// More removals than one run may make is blocked before any is made.
func TestRunPlansHoldsTheDeletionLimit(t *testing.T) {
	stages := make([]route.Route, 0, maxDeletionsPerTarget+1)
	for planID := int64(1); planID <= maxDeletionsPerTarget+1; planID++ {
		stages = append(stages, planStage(t, planID, "r1"))
	}
	local := &fakeSource{provider: route.ProviderLocal, stages: stages}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newPlanService(t, state, target, local)
	require.Equal(t, OutcomeSucceeded, service.RunPlans(t.Context(), 0).Outcome, "first RunPlans()")
	rewindTokens(state)
	local.stages = nil

	result := service.RunPlans(t.Context(), 0)

	assert.Equal(t, OutcomeBlocked, result.Outcome, "RunPlans() outcome")
	assert.Equal(t, FailureDeletionLimit, result.Failure, "RunPlans() failure")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
}

// A rider who must reconnect is not asked anything; the others still get the
// plan, and the planner is told which rider is waiting on what.
func TestRunPlansPassesOverAnUnauthorizedTarget(t *testing.T) {
	pushed := planStage(t, 1, "r1")
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{pushed}}
	state := newFakeState("a", "b")
	state.authorizations["b"] = "needs_reauthorization"
	target := newFakeTarget()
	service := newPlanService(t, state, target, local)

	result := service.RunPlans(t.Context(), 1)

	assert.Equal(t, OutcomeSucceeded, result.Outcome, "RunPlans() outcome")
	assert.NotContains(t, target.refreshTokens, "b", "unauthorized target asked")
	deliveries, err := service.PlanDelivery(t.Context(), 1, pushed.Revision())
	require.NoError(t, err, "PlanDelivery()")
	assert.Equal(t, []Delivery{
		{TargetID: "a", State: DeliveryCurrent, DeliveredAt: service.now().UTC()},
		{TargetID: "b", State: DeliveryFailed, Failure: FailureAuthorization},
	}, deliveries, "deliveries")
}

// A failed write is reported against the revision it tried, and a later save
// of the plan reads as owed again rather than as the old failure.
func TestPlanDeliveryReportsAFailedPushUntilTheNextRevision(t *testing.T) {
	first := planStage(t, 1, "r1")
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{first}}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newPlanService(t, state, target, local)
	require.Equal(t, OutcomeSucceeded, service.RunPlans(t.Context(), 1).Outcome, "first RunPlans()")
	rewindTokens(state)
	second := planStage(t, 1, "r2")
	local.stages = []route.Route{second}
	target.failUpdateAccess = accessFor("a")

	result := service.RunPlans(t.Context(), 1)

	assert.Equal(t, OutcomeFailed, result.Outcome, "RunPlans() outcome")
	deliveries, err := service.PlanDelivery(t.Context(), 1, second.Revision())
	require.NoError(t, err, "PlanDelivery()")
	assert.Equal(t, DeliveryFailed, deliveries[0].State, "failed target state")
	assert.Equal(t, FailureDestination, deliveries[0].Failure, "failed target failure")
	assert.Equal(t, DeliveryCurrent, deliveries[1].State, "other target state")

	later, err := service.PlanDelivery(t.Context(), 1, "r3")
	require.NoError(t, err, "PlanDelivery() for a newer revision")
	assert.Equal(t, DeliveryPending, later[0].State, "state once revised again")
}

// A draft nobody holds is absent; one still held after unpublishing is owed a
// removal.
func TestPlanDeliveryReportsAbsenceAndOwedRemovals(t *testing.T) {
	pushed := planStage(t, 1, "r1")
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{pushed}}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newPlanService(t, state, target, local)

	draft, err := service.PlanDelivery(t.Context(), 1, "")
	require.NoError(t, err, "PlanDelivery() before any push")
	assert.Equal(t, DeliveryAbsent, draft[0].State, "draft state")

	require.Equal(t, OutcomeSucceeded, service.RunPlans(t.Context(), 1).Outcome, "RunPlans()")
	rewindTokens(state)
	withdrawn, err := service.PlanDelivery(t.Context(), 1, "")
	require.NoError(t, err, "PlanDelivery() after unpublishing")
	assert.Equal(t, DeliveryPending, withdrawn[0].State, "state while the removal is owed")
}

// Without a planner configured there is nothing to push.
func TestRunPlansIsNotReadyWithoutAPlanner(t *testing.T) {
	service := newPlanService(t, newFakeState("a"), newFakeTarget())

	assert.Equal(t, OutcomeNotReady, service.RunPlans(t.Context(), 1).Outcome, "RunPlans() outcome")
}

// Whatever stops a push partway, the plan's copies are left as they were and
// the push says why, never a claim that it went through.
func TestRunPlansReportsWhatStoppedIt(t *testing.T) {
	errDown := errors.New("down")
	tests := map[string]struct {
		arrange func(state *fakeState, target *fakeTarget, encoder *fakeEncoder, local *fakeSource)
		failure FailureCategory
	}{
		"planner unreadable":   {func(_ *fakeState, _ *fakeTarget, _ *fakeEncoder, local *fakeSource) { local.err = errDown }, FailureSource},
		"inventory unreadable": {func(state *fakeState, _ *fakeTarget, _ *fakeEncoder, _ *fakeSource) { state.trustedErr = errDown }, FailureState},
		"authorization unread": {func(state *fakeState, _ *fakeTarget, _ *fakeEncoder, _ *fakeSource) { state.authorizationErr = errDown }, FailureState},
		"mappings unreadable":  {func(state *fakeState, _ *fakeTarget, _ *fakeEncoder, _ *fakeSource) { state.stagesErr = errDown }, FailureState},
		"listing refused":      {func(_ *fakeState, target *fakeTarget, _ *fakeEncoder, _ *fakeSource) { target.listErr = errDestination }, FailureDestination},
		"course unencodable":   {func(_ *fakeState, _ *fakeTarget, encoder *fakeEncoder, _ *fakeSource) { encoder.err = errDown }, FailureCourse},
		"create refused": {func(_ *fakeState, target *fakeTarget, _ *fakeEncoder, _ *fakeSource) {
			target.createErr = errDestination
		}, FailureDestination},
		"mapping unwritable":       {func(state *fakeState, _ *fakeTarget, _ *fakeEncoder, _ *fakeSource) { state.upsertErr = errDown }, FailureState},
		"refresh token unreadable": {func(state *fakeState, _ *fakeTarget, _ *fakeEncoder, _ *fakeSource) { state.refreshTokenErr = errDown }, FailureState},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{planStage(t, 1, "r1")}}
			state := newFakeState("a", "b")
			target := newFakeTarget()
			encoder := &fakeEncoder{}
			service, err := New(
				syncOptions(false, []Source{local}, "a", "b"), state, identityProcessor{}, encoder, target, nil, nil,
			)
			require.NoError(t, err, "New()")
			test.arrange(state, target, encoder, local)

			result := service.RunPlans(t.Context(), 1)

			assert.Equal(t, OutcomeFailed, result.Outcome, "RunPlans() outcome")
			assert.Equal(t, test.failure, result.Failure, "RunPlans() failure")
			assert.Zero(t, result.Created, "created")
		})
	}
}

// A refused removal, or one whose record cannot be forgotten, is reported and
// leaves the plan owed rather than forgotten.
func TestRunPlansReportsAFailedRemoval(t *testing.T) {
	tests := map[string]struct {
		arrange func(state *fakeState, target *fakeTarget)
		failure FailureCategory
	}{
		"delete refused": {func(state *fakeState, target *fakeTarget) {
			target.failDeleteAccess = accessFor(state.refreshTokens["a"])
		}, FailureDestination},
		"mapping unforgettable": {func(state *fakeState, _ *fakeTarget) { state.deleteStageErr = errors.New("down") }, FailureState},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{planStage(t, 1, "r1")}}
			state := newFakeState("a", "b")
			target := newFakeTarget()
			service := newPlanService(t, state, target, local)
			require.Equal(t, OutcomeSucceeded, service.RunPlans(t.Context(), 1).Outcome, "first RunPlans()")
			rewindTokens(state)
			local.stages = nil
			test.arrange(state, target)

			result := service.RunPlans(t.Context(), 1)

			assert.Equal(t, test.failure, result.Failure, "RunPlans() failure")
			deliveries, err := service.PlanDelivery(t.Context(), 1, "")
			require.NoError(t, err, "PlanDelivery()")
			assert.Equal(t, DeliveryFailed, deliveries[0].State, "state of the failed removal")
		})
	}
}

func TestRunPlansIsFailedWhenThePlannerCannotBeBuilt(t *testing.T) {
	service, err := New(&Options{
		AllowEmptySourceDeletion: emptySourceDeletion(false),
		Sources:                  func() ([]Source, error) { return nil, nil },
		SourceFor:                func(route.Provider) (Source, bool, error) { return nil, false, errors.New("down") },
		TargetIDs:                func() []string { return []string{"a"} },
	}, newFakeState("a"), identityProcessor{}, &fakeEncoder{}, newFakeTarget(), nil, nil)
	require.NoError(t, err, "New()")

	assert.Equal(t, FailureState, service.RunPlans(t.Context(), 1).Failure, "RunPlans() failure")
}

func TestPlanDeliveryFailsWhenStateIsUnreadable(t *testing.T) {
	for name, arrange := range map[string]func(*fakeState){
		"authorization": func(state *fakeState) { state.authorizationErr = errors.New("down") },
		"mappings":      func(state *fakeState) { state.stagesErr = errors.New("down") },
	} {
		t.Run(name, func(t *testing.T) {
			state := newFakeState("a")
			arrange(state)
			service := newPlanService(t, state, newFakeTarget())

			_, err := service.PlanDelivery(t.Context(), 1, "r1")

			assert.Error(t, err, "PlanDelivery()")
		})
	}
}

// A push that fails before it reaches any rider reports every rider still
// owed the plan as failed, not as sending, until a push gets through.
func TestPlanDeliveryReportsAPushThatReachedNoOne(t *testing.T) {
	pushed := planStage(t, 1, "r1")
	local := &fakeSource{provider: route.ProviderLocal, err: errors.New("down")}
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newPlanService(t, state, target, local)

	require.Equal(t, OutcomeFailed, service.RunPlans(t.Context(), 1).Outcome, "failed RunPlans()")
	failed, err := service.PlanDelivery(t.Context(), 1, pushed.Revision())
	require.NoError(t, err, "PlanDelivery() after the failure")
	assert.Equal(t, Delivery{TargetID: "a", State: DeliveryFailed, Failure: FailureSource}, failed[0], "delivery")

	local.err, local.stages = nil, []route.Route{pushed}
	require.Equal(t, OutcomeSucceeded, service.RunPlans(t.Context(), 0).Outcome, "sweep")
	recovered, err := service.PlanDelivery(t.Context(), 1, pushed.Revision())
	require.NoError(t, err, "PlanDelivery() after the sweep")
	assert.Equal(t, DeliveryCurrent, recovered[0].State, "state once a sweep got through")
}

// Enrichment follows a change to the stored plans, reaching a rider or not.
func TestRunPlansReportsWhetherTheStoredPlansChanged(t *testing.T) {
	local := &fakeSource{provider: route.ProviderLocal, stages: []route.Route{planStage(t, 1, "r1")}}
	state := newFakeState("a")
	state.authorizations["a"] = "needs_reauthorization"
	service := newPlanService(t, state, newFakeTarget(), local)

	first := service.RunPlans(t.Context(), 1)
	second := service.RunPlans(t.Context(), 1)

	assert.Equal(t, OutcomeSkipped, first.Outcome, "no rider reachable")
	assert.True(t, first.SourceStored, "first push stored a new plan")
	assert.False(t, second.SourceStored, "second push changed nothing")
}
