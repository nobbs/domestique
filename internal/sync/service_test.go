package sync

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

var (
	errDestination  = errors.New("destination failed")
	errUnauthorized = errors.New("destination authorization failed")
)

func TestServiceDoesNotMutateTargetsWhenSourceFails(t *testing.T) {
	state := newFakeState("a", "b")
	stale := testStage(t, 2, 1, "old", "old-hash")
	seedMapping(state, "a", &stale, 101)
	seedMapping(state, "b", &stale, 201)
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{err: errors.New("source unavailable")}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeFailed, result.Outcome, "runBoth() outcome")
	assert.Equal(t, FailureSource, result.Failure, "runBoth() failure")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
	assert.Empty(t, target.refreshTokens, "refresh calls")
}

func TestServiceSkipsFailedTargetDeletionButContinuesOtherTarget(t *testing.T) {
	previous := testStage(t, 1, 1, "old", "old-hash")
	desired := testStage(t, 1, 1, "new", "new-hash")
	stale := testStage(t, 2, 1, "old", "old-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	for _, targetID := range []string{"a", "b"} {
		seedMapping(state, targetID, &previous, remoteID(targetID, 1))
		seedMapping(state, targetID, &stale, remoteID(targetID, 2))
		target.seedRoute(targetID, &previous, remoteID(targetID, 1))
		target.seedRoute(targetID, &stale, remoteID(targetID, 2))
	}
	target.failUpdateAccess = accessFor("a")
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeFailed, result.Outcome, "runBoth() outcome")
	assert.Equal(t, FailureDestination, result.Failure, "runBoth() failure")
	assert.Equal(t, 1, result.Updated, "updated routes")
	assert.Equal(t, 1, result.Deleted, "deleted routes")
	assert.Equal(t, []string{accessFor("b")}, target.deletedAccess, "delete callers")
}

func TestServiceBlocksSixthDeletionWithoutDeleting(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	for _, targetID := range []string{"a", "b"} {
		for routeID := int64(2); routeID <= 7; routeID++ {
			stale := testStage(t, routeID, 1, "old", "old-hash")
			seedMapping(state, targetID, &stale, remoteID(targetID, routeID))
			target.seedRoute(targetID, &stale, remoteID(targetID, routeID))
		}
	}
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeBlocked, result.Outcome, "runBoth() outcome")
	assert.Equal(t, FailureDeletionLimit, result.Failure, "runBoth() failure")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
}

func TestServiceAdoptsOwnedRoutesAfterStateLoss(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	for _, targetID := range []string{"a", "b"} {
		target.seedRoute(targetID, &desired, remoteID(targetID, 1))
	}
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "runBoth() outcome")
	assert.Zero(t, result.Created, "created routes")
	assert.Zero(t, result.Updated, "updated routes")
	assert.Zero(t, result.Deleted, "deleted routes")
	key := keyFor(&desired)
	for _, targetID := range []string{"a", "b"} {
		assert.Containsf(t, state.mappings[targetID], key, "target %q did not adopt the existing route", targetID)
	}
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
}

func TestServiceRecreatesMissingOwnedRoutesWithoutDeletingManualRoutes(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	for _, targetID := range []string{"a", "b"} {
		seedMapping(state, targetID, &desired, remoteID(targetID, 1))
		target.ensureAccess(accessFor(targetID))["manual-route"] = remoteID(targetID, 99)
	}
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "runBoth() outcome")
	assert.Equal(t, 2, result.Created, "created routes")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
	for _, targetID := range []string{"a", "b"} {
		assert.Containsf(t, target.routes[accessFor(targetID)], "manual-route", "target %q removed a manual route", targetID)
	}
}

func TestServiceMarksOnlyRejectedTargetForReauthorization(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	target.rejectRefreshToken["a"] = true
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeFailed, result.Outcome, "runBoth() outcome")
	assert.Equal(t, FailureAuthorization, result.Failure, "runBoth() failure")
	assert.Equal(t, "needs_reauthorization", state.authorizations["a"], "target a authorization")
	assert.Equal(t, authorizedState, state.authorizations["b"], "target b authorization")
	assert.Equal(t, 1, result.Created, "created routes")
}

func TestServiceBlocksUnexpectedEmptySourceWithoutDeleting(t *testing.T) {
	previous := testStage(t, 1, 1, "old", "old-hash")
	state := newFakeState("a", "b")
	state.trusted = []route.Stage{previous}
	target := newFakeTarget()
	for _, targetID := range []string{"a", "b"} {
		seedMapping(state, targetID, &previous, remoteID(targetID, 1))
		target.seedRoute(targetID, &previous, remoteID(targetID, 1))
	}
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeBlocked, result.Outcome, "runBoth() outcome")
	assert.Equal(t, FailureEmptySource, result.Failure, "runBoth() failure")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
	assert.Zero(t, state.storeInventoryCalls, "stored inventories")
}

func TestServiceDeletesUpToFiveOwnedRoutesPerTarget(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	for _, targetID := range []string{"a", "b"} {
		seedMapping(state, targetID, &desired, remoteID(targetID, 1))
		target.seedRoute(targetID, &desired, remoteID(targetID, 1))
		for routeID := int64(2); routeID <= 6; routeID++ {
			stale := testStage(t, routeID, 1, "old", "old-hash")
			seedMapping(state, targetID, &stale, remoteID(targetID, routeID))
			target.seedRoute(targetID, &stale, remoteID(targetID, routeID))
		}
	}
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "runBoth() outcome")
	assert.Equal(t, 10, result.Deleted, "deleted routes")
}

// The two halves are independent: a library refresh must keep working while a
// target waits to be reauthorised, because the refresh touches no target.
func TestServiceReadsTheSourceWhileATargetNeedsReauthorization(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	state.authorizations["b"] = "needs_reauthorization"
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	source := service.RunSource(t.Context())
	assert.Equal(t, OutcomeSucceeded, source.Outcome, "RunSource() outcome")
	assert.Len(t, state.trusted, 1, "stored stages")
	assert.Equal(t, OutcomeNotReady, service.RunTargets(t.Context()).Outcome, "RunTargets() outcome")
	assert.Empty(t, target.routes, "target routes")
}

// Writing to the targets works from the inventory the last read stored, so a
// target that was unreachable catches up without asking the source again.
func TestServiceReconcilesStoredInventoryWithoutReadingTheSource(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	source := &fakeSource{err: errors.New("source unavailable")}
	service := newService(t, state, source, &fakeEncoder{}, newFakeTarget(), false)
	state.trusted = []route.Stage{desired}

	result := service.RunTargets(t.Context())
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "RunTargets() outcome")
	assert.Equal(t, 2, result.Created, "RunTargets() created")
	assert.Zero(t, source.calls, "source inventory calls")
}

// The whole point of listing: a library where nothing moved asks each target
// once and writes nothing at all. Every stage used to cost its own lookup,
// which is what exhausted a request quota shared by every target.
func TestServiceReconcilingAnUnchangedLibraryCostsOneListingPerTarget(t *testing.T) {
	first := testStage(t, 1, 1, "current", "current-hash")
	second := testStage(t, 2, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	state.trusted = []route.Stage{first, second}
	for _, targetID := range []string{"a", "b"} {
		for index, stage := range []*route.Stage{&first, &second} {
			routeID := int64(100 + index)
			target.seedRoute(targetID, stage, routeID)
			seedMapping(state, targetID, stage, routeID)
		}
	}
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTargets(t.Context())
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "RunTargets() outcome")
	assert.Equal(t, 2, target.listCalls, "one listing per target, however many stages it holds")
	assert.Zero(t, result.Created, "created")
	assert.Zero(t, result.Updated, "updated")
	assert.Zero(t, result.Deleted, "deleted")
	assert.Empty(t, target.updatedRouteIDs, "updated route ids")
	assert.Empty(t, target.deletedRouteIDs, "deleted route ids")
}

// A target whose routes cannot be read is a target whose ownership cannot be
// established, so nothing may be written to or removed from it.
func TestServiceWritesNothingWhenTheTargetListingFails(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	stale := testStage(t, 2, 1, "old", "old-hash")
	state := newFakeState("a", "b")
	seedMapping(state, "a", &stale, 101)
	state.trusted = []route.Stage{desired}
	target := newFakeTarget()
	target.listErr = errDestination
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTargets(t.Context())
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunTargets() outcome")
	assert.Equal(t, FailureDestination, result.Failure, "RunTargets() failure")
	assert.Zero(t, result.Created, "created")
	assert.Empty(t, target.deletedRouteIDs, "deleted route ids")
}

func TestServiceClearTargetRemovesEveryOwnedRouteAndItsMappings(t *testing.T) {
	first := testStage(t, 1, 1, "current", "current-hash")
	second := testStage(t, 2, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	state.trusted = []route.Stage{first, second}
	for _, stage := range []*route.Stage{&first, &second} {
		for index, targetID := range []string{"a", "b"} {
			routeID := int64(100 + index)
			target.seedRoute(targetID, stage, routeID)
			seedMapping(state, targetID, stage, routeID)
		}
	}
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.ClearTarget(t.Context(), "a")
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "ClearTarget() outcome")
	assert.Equal(t, 2, result.Deleted, "ClearTarget() deleted")
	assert.Empty(t, state.mappings["a"], "the cleared slot still remembers stages")
	// A clear reads the account through its own path, which tolerates the
	// duplicate external IDs reconciliation's listing refuses.
	assert.Equal(t, 1, target.clearCalls, "clears")
	assert.Zero(t, target.listCalls, "a clear reconciled")

	// The other slot is untouched: clearing one target says nothing about any
	// other, and the library itself is not what was cleared.
	assert.Len(t, state.mappings["b"], 2, "the other target's mappings")
	assert.Len(t, state.trusted, 2, "the stored library")
}

func TestServiceClearTargetLeavesRoutesItDoesNotOwn(t *testing.T) {
	// The listing only ever answers with routes carrying an external ID this
	// service issued, so a hand-made route is not merely skipped here — it is
	// never visible to this path at all. Nothing to delete means nothing
	// deleted, and the mappings still go.
	state := newFakeState("a")
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.ClearTarget(t.Context(), "a")
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "ClearTarget() outcome")
	assert.Zero(t, result.Deleted, "ClearTarget() deleted")
	assert.Empty(t, target.deletedRouteIDs, "deleted route ids")
}

func TestServiceClearTargetIgnoresAnUnconfiguredSlot(t *testing.T) {
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.ClearTarget(t.Context(), "not-a-slot")
	assert.Equal(t, OutcomeSkipped, result.Outcome, "ClearTarget() outcome")
	assert.Empty(t, target.deletedRouteIDs, "deleted route ids")
}

func TestServiceClearTargetRefusesUnusableTargetState(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")

	tests := []struct {
		name    string
		arrange func(state *fakeState, target *fakeTarget)
		outcome Outcome
		failure FailureCategory
	}{
		{
			// Deleting from an account nobody has connected is not a recovery,
			// it is a request the service cannot even authenticate.
			name:    "target awaiting authorization",
			arrange: func(state *fakeState, _ *fakeTarget) { state.authorizations["a"] = "needs_reauthorization" },
			outcome: OutcomeNotReady,
		},
		{
			name:    "authorization unreadable",
			arrange: func(state *fakeState, _ *fakeTarget) { state.authorizationErr = errors.New("state unavailable") },
			outcome: OutcomeFailed,
			failure: FailureState,
		},
		{
			name:    "refresh token rejected",
			arrange: func(_ *fakeState, target *fakeTarget) { target.rejectRefreshToken["a"] = true },
			outcome: OutcomeFailed,
			failure: FailureAuthorization,
		},
		{
			// Ownership cannot be established, so nothing may be removed.
			name:    "listing fails",
			arrange: func(_ *fakeState, target *fakeTarget) { target.listErr = errDestination },
			outcome: OutcomeFailed,
			failure: FailureDestination,
		},
		{
			name:    "stored refresh token unreadable",
			arrange: func(state *fakeState, _ *fakeTarget) { state.refreshTokenErr = errors.New("state unavailable") },
			outcome: OutcomeFailed,
			failure: FailureState,
		},
		{
			// The rotated token could not be recorded, so the next request
			// would authenticate with one Wahoo has already replaced.
			name:    "rotated refresh token unstorable",
			arrange: func(state *fakeState, _ *fakeTarget) { state.replaceTokenErr = errors.New("state unavailable") },
			outcome: OutcomeFailed,
			failure: FailureState,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := newFakeState("a")
			target := newFakeTarget()
			target.seedRoute("a", &desired, 101)
			seedMapping(state, "a", &desired, 101)
			test.arrange(state, target)
			service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

			result := service.ClearTarget(t.Context(), "a")
			assert.Equal(t, test.outcome, result.Outcome, "ClearTarget() outcome")
			assert.Equal(t, test.failure, result.Failure, "ClearTarget() failure")
			assert.Empty(t, target.deletedRouteIDs, "deleted route ids")
			assert.Len(t, state.mappings["a"], 1, "mappings were forgotten without deleting anything")
		})
	}
}

func TestServiceClearTargetKeepsMappingsWhenADeletionFails(t *testing.T) {
	// Remote first, local second: a clear that could not finish removing
	// routes must not forget the ones it still owns, or they would be stranded
	// with nothing recording that they exist.
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	target.seedRoute("a", &desired, 101)
	seedMapping(state, "a", &desired, 101)
	target.failDeleteAccess = accessFor("a")
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.ClearTarget(t.Context(), "a")
	assert.Equal(t, OutcomeFailed, result.Outcome, "ClearTarget() outcome")
	assert.Equal(t, FailureDestination, result.Failure, "ClearTarget() failure")
	assert.Len(t, state.mappings["a"], 1, "mappings were forgotten despite a failed deletion")
}

// A library that cannot be read back whole is indistinguishable from one whose
// missing stages are meant to be deleted, so it stops the phase instead.
func TestServiceDoesNotReconcileAnUnreadableStoredInventory(t *testing.T) {
	stale := testStage(t, 2, 1, "old", "old-hash")
	state := newFakeState("a", "b")
	seedMapping(state, "a", &stale, 101)
	state.trustedErr = errors.New("state unavailable")
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTargets(t.Context())
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunTargets() outcome")
	assert.Equal(t, FailureState, result.Failure, "RunTargets() failure")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
}

// What a reprocess leaves behind: a mapping that still names the Wahoo route it
// owns, but no longer claims to have pushed the revision it holds. It has to
// read as an update of that route — a mapping the reconciler cannot read at all
// would fail the whole target phase over one stage.
func TestServiceRewritesAStageWhosePushedRevisionWasForgotten(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	target.seedRoute("a", &desired, 101)
	state.trusted = []route.Stage{desired}
	state.mappings["a"][keyFor(&desired)] = targetStage{
		sourceRevision: "reprocess-requested",
		contentHash:    "reprocess-requested",
		wahooRouteID:   101,
	}
	service, err := New(
		&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)},
		state, []Source{&fakeSource{}}, identityProcessor{}, &fakeEncoder{}, target, nil, nil,
	)
	require.NoError(t, err, "New()")

	result := service.RunTargets(t.Context())
	require.Equal(t, OutcomeSucceeded, result.Outcome, "RunTargets() outcome")
	assert.Equal(t, 1, result.Updated, "RunTargets() updated")
	// The owned route is rewritten in place rather than replaced.
	assert.Equal(t, []int64{101}, target.updatedRouteIDs, "updated routes")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
}

func TestServiceSkipsOverlappingRun(t *testing.T) {
	service := newService(t, newFakeState("a", "b"), &fakeSource{}, &fakeEncoder{}, newFakeTarget(), false)
	service.running.Store(true)

	assert.Equal(t, OutcomeSkipped, runBoth(t.Context(), service).Outcome, "runBoth() outcome")
}

// RunTarget must mutate exactly the slot it was asked for, and nothing about
// the other configured target: it reads the same stored inventory, but the
// account never contacted is proof no other slot's routes were even read.
func TestServiceRunTargetReconcilesOnlyTheNamedTarget(t *testing.T) {
	previous := testStage(t, 1, 1, "old", "old-hash")
	desired := testStage(t, 1, 1, "new", "new-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	for _, targetID := range []string{"a", "b"} {
		seedMapping(state, targetID, &previous, remoteID(targetID, 1))
		target.seedRoute(targetID, &previous, remoteID(targetID, 1))
	}
	state.trusted = []route.Stage{desired}
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTarget(t.Context(), "a")
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "RunTarget() outcome")
	assert.Equal(t, 1, result.Updated, "RunTarget() updated")
	assert.Equal(t, []TargetResult{{ID: "a", Outcome: OutcomeSucceeded}}, result.Targets)
	assert.Equal(t, []int64{remoteID("a", 1)}, target.updatedRouteIDs, "updated routes")
	assert.Equal(t, []string{"a"}, target.refreshTokens, "a target other than the one asked for was contacted")
}

// A target-specific request keeps the same deletion-limit gate a full target
// phase applies to that slot.
func TestServiceRunTargetKeepsTheDeletionLimit(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	for routeID := int64(2); routeID <= 7; routeID++ {
		stale := testStage(t, routeID, 1, "old", "old-hash")
		seedMapping(state, "a", &stale, remoteID("a", routeID))
		target.seedRoute("a", &stale, remoteID("a", routeID))
	}
	state.trusted = []route.Stage{desired}
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTarget(t.Context(), "a")
	assert.Equal(t, OutcomeBlocked, result.Outcome, "RunTarget() outcome")
	assert.Equal(t, FailureDeletionLimit, result.Failure, "RunTarget() failure")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
}

// A target this service was never configured with performs no work, the same
// defensive answer a concurrent run gives: the caller is expected to have
// already refused the slot.
func TestServiceRunTargetSkipsAnUnconfiguredTarget(t *testing.T) {
	state := newFakeState("a", "b")
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTarget(t.Context(), "unknown")
	assert.Equal(t, OutcomeSkipped, result.Outcome, "RunTarget() outcome")
	assert.Empty(t, target.refreshTokens, "an unconfigured target was contacted")
}

// A slot awaiting onboarding reconciles nothing rather than failing: onboarding
// is the operator's next move, not a fault the run caused.
func TestServiceRunTargetReportsNotReadyWhenTheSlotNeedsOnboarding(t *testing.T) {
	state := newFakeState("a", "b")
	state.authorizations["a"] = "needs_reauthorization"
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTarget(t.Context(), "a")
	assert.Equal(t, OutcomeNotReady, result.Outcome, "RunTarget() outcome")
	assert.Empty(t, target.refreshTokens, "a not-ready target was contacted")
}

// An authorization read that fails is a state failure, the same as it is for
// a full target phase.
func TestServiceRunTargetFailsWhenAuthorizationCannotBeRead(t *testing.T) {
	state := newFakeState("a", "b")
	state.authorizationErr = errors.New("state unavailable")
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTarget(t.Context(), "a")
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunTarget() outcome")
	assert.Equal(t, FailureState, result.Failure, "RunTarget() failure")
	assert.Empty(t, target.refreshTokens, "a target was contacted despite an unreadable authorization")
}

// A stored inventory that cannot be read back whole must not be reconciled,
// the same as it must not for a full target phase.
func TestServiceRunTargetFailsWhenTheStoredInventoryCannotBeRead(t *testing.T) {
	state := newFakeState("a", "b")
	state.trustedErr = errors.New("state unavailable")
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTarget(t.Context(), "a")
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunTarget() outcome")
	assert.Equal(t, FailureState, result.Failure, "RunTarget() failure")
	assert.Empty(t, target.deletedRouteIDs, "deleted routes")
}

// A stored inventory that somehow holds a duplicate stage cannot be
// reconciled either: the reconciler has no way to say which copy is desired.
func TestServiceRunTargetFailsOnADuplicateStoredStage(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	state.trusted = []route.Stage{desired, desired}
	target := newFakeTarget()
	service := newService(t, state, &fakeSource{}, &fakeEncoder{}, target, false)

	result := service.RunTarget(t.Context(), "a")
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunTarget() outcome")
	assert.Equal(t, FailureState, result.Failure, "RunTarget() failure")
}

// RunTarget shares the same mutual exclusion as a full synchronization: it
// must not race a run in flight over the same stored state.
func TestServiceRunTargetSkipsOverlappingRun(t *testing.T) {
	service := newService(t, newFakeState("a", "b"), &fakeSource{}, &fakeEncoder{}, newFakeTarget(), false)
	service.running.Store(true)

	assert.Equal(t, OutcomeSkipped, service.RunTarget(t.Context(), "a").Outcome, "RunTarget() outcome")
}

func TestServiceSupportsOneTarget(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	service, err := New(&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)}, state, []Source{&fakeSource{stages: []route.Stage{desired}}}, identityProcessor{}, &fakeEncoder{}, target, nil, nil)
	require.NoError(t, err, "New()")

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "runBoth() outcome")
	assert.Equal(t, 1, result.Created, "runBoth() created")
}

func TestServiceUpdatesLegacyEncoderOutput(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	target.seedRoute("a", &desired, 101)
	state.mappings["a"][keyFor(&desired)] = targetStage{
		sourceRevision: desired.Revision(),
		contentHash:    desired.ContentHash(),
		wahooRouteID:   101,
	}
	service, err := New(&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)}, state, []Source{&fakeSource{stages: []route.Stage{desired}}}, identityProcessor{}, &fakeEncoder{}, target, nil, nil)
	require.NoError(t, err, "New()")

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "runBoth() outcome")
	assert.Equal(t, 1, result.Updated, "runBoth() updated")
	assert.Equal(t, encodedContentHash(&desired), state.mappings["a"][keyFor(&desired)].contentHash, "stored content hash")
}

// The annotator classifies stored geometry, so it must see the same inventory
// that was stored, and only after the routes are on the targets.
func TestServiceAnnotatesTheStoredInventoryAfterReconciling(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	annotator := &fakeAnnotator{}
	annotator.observe = func() { annotator.createdOnEntry = len(target.routes[accessFor("a")]) }
	service := newAnnotatedService(t, state, &fakeSource{stages: []route.Stage{desired}}, target, annotator)

	result := runBoth(t.Context(), service)
	require.Equal(t, OutcomeSucceeded, result.Outcome, "runBoth() outcome")
	require.Equal(t, 1, annotator.calls, "annotate calls")
	require.Len(t, annotator.stages, 1, "annotated stages")
	assert.Equal(t, 1, annotator.createdOnEntry, "routes on the target when annotation began")
	assert.InDelta(t, exportedElevation, elevationOf(t, &annotator.stages[0]), 0.001, "annotated elevation")
	assert.InDelta(t, exportedElevation, elevationOf(t, &state.trusted[0]), 0.001, "stored elevation")
}

// Enrichment is not what a synchronization is for: a tagging endpoint that is
// slow, rate limited, or simply gone must not turn a completed sync into a
// failure the operator is notified about.
func TestServiceSucceedsWhenAnnotationFails(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	annotator := &fakeAnnotator{err: errors.New("endpoint unavailable")}
	service := newAnnotatedService(t, state, &fakeSource{stages: []route.Stage{desired}}, target, annotator)

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "runBoth() outcome")
	assert.Equal(t, FailureNone, result.Failure, "runBoth() failure")
	assert.Equal(t, 1, result.Created, "created routes")
}

func TestServiceSkipsAnnotationWhenNothingWasStored(t *testing.T) {
	previous := testStage(t, 1, 1, "old", "old-hash")
	state := newFakeState("a")
	state.trusted = []route.Stage{previous}
	target := newFakeTarget()
	seedMapping(state, "a", &previous, remoteID("a", 1))
	target.seedRoute("a", &previous, remoteID("a", 1))
	annotator := &fakeAnnotator{}
	service := newAnnotatedService(t, state, &fakeSource{}, target, annotator)

	result := runBoth(t.Context(), service)
	require.Equal(t, OutcomeBlocked, result.Outcome, "runBoth() outcome")
	assert.Zero(t, annotator.calls, "annotate calls")
}

// AnnotateStored reports the annotator's own counts back, which is what lets a
// caller distinguish a stage that keeps failing from one nobody has asked
// about yet.
func TestServiceAnnotateStoredReportsTheAnnotatorsCounts(t *testing.T) {
	stage := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	state.trusted = []route.Stage{stage}
	service := newAnnotatedService(t, state, &fakeSource{}, newFakeTarget(), &fakeAnnotator{})

	classified, failed := service.AnnotateStored(t.Context())
	assert.Equal(t, 1, classified, "classified")
	assert.Zero(t, failed, "failed")
}

// An inventory that cannot be read back leaves nothing to classify, and that
// must not be reported as a stage this pass failed on.
func TestServiceAnnotateStoredReportsNothingWhenTheInventoryCannotBeRead(t *testing.T) {
	state := newFakeState("a")
	state.trustedErr = errors.New("state unavailable")
	service := newAnnotatedService(t, state, &fakeSource{}, newFakeTarget(), &fakeAnnotator{})

	classified, failed := service.AnnotateStored(t.Context())
	assert.Zero(t, classified, "classified")
	assert.Zero(t, failed, "failed")
}

func newAnnotatedService(
	t *testing.T,
	state *fakeState,
	source *fakeSource,
	target *fakeTarget,
	annotator Annotator,
) *Service {
	t.Helper()
	service, err := New(
		&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)},
		state,
		[]Source{source},
		exportProcessor{},
		&fakeEncoder{},
		target,
		annotator,
		nil,
	)
	require.NoError(t, err, "New()")

	return service
}

type fakeAnnotator struct {
	err            error
	observe        func()
	stages         []route.Stage
	calls          int
	createdOnEntry int
}

func (a *fakeAnnotator) Annotate(
	_ context.Context, stages []route.Stage,
) (classified, failed int, err error) {
	if a.observe != nil {
		a.observe()
	}
	a.calls++
	a.stages = append([]route.Stage(nil), stages...)
	if a.err != nil {
		return 0, len(stages), a.err
	}

	return len(stages), 0, nil
}

// The predictor sees the same inventory the annotator does, in the same pass,
// so a stage's prediction can read the classification the annotator just
// wrote rather than waiting a full cycle behind it.
func TestServicePredictsTheStoredInventoryAfterReconciling(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	predictor := &fakePredictor{}
	service, err := New(
		&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)},
		state, []Source{&fakeSource{stages: []route.Stage{desired}}}, exportProcessor{}, &fakeEncoder{}, target,
		nil, predictor,
	)
	require.NoError(t, err, "New()")

	result := runBoth(t.Context(), service)
	require.Equal(t, OutcomeSucceeded, result.Outcome, "runBoth() outcome")
	require.Equal(t, 1, predictor.calls, "predict calls")
	require.Len(t, predictor.stages, 1, "predicted stages")
}

// Ride model prediction is not what a synchronization is for, on the same
// terms surface classification is not: a coefficient file that cannot be
// read must not turn a completed sync into a failure the operator is
// notified about.
func TestServiceSucceedsWhenPredictionFails(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	predictor := &fakePredictor{err: errors.New("coefficient file unavailable")}
	service, err := New(
		&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)},
		state, []Source{&fakeSource{stages: []route.Stage{desired}}}, exportProcessor{}, &fakeEncoder{}, target,
		nil, predictor,
	)
	require.NoError(t, err, "New()")

	result := runBoth(t.Context(), service)
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "runBoth() outcome")
	assert.Equal(t, FailureNone, result.Failure, "runBoth() failure")
	assert.Equal(t, 1, result.Created, "created routes")
}

func TestServiceAnnotatesAndPredictsInTheSamePass(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	state.trusted = []route.Stage{desired}
	target := newFakeTarget()
	annotator := &fakeAnnotator{}
	predictor := &fakePredictor{}
	service, err := New(
		&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)},
		state, []Source{&fakeSource{stages: []route.Stage{desired}}}, exportProcessor{}, &fakeEncoder{}, target,
		annotator, predictor,
	)
	require.NoError(t, err, "New()")

	classified, failed := service.AnnotateStored(t.Context())
	assert.Equal(t, 1, annotator.calls, "annotate calls")
	assert.Equal(t, 1, predictor.calls, "predict calls")
	assert.Equal(t, 1, classified, "classified")
	assert.Zero(t, failed, "failed")
}

type fakePredictor struct {
	err    error
	stages []route.Stage
	calls  int
}

func (p *fakePredictor) Predict(
	_ context.Context, stages []route.Stage,
) (predicted, failed int, err error) {
	p.calls++
	p.stages = append([]route.Stage(nil), stages...)
	if p.err != nil {
		return 0, len(stages), p.err
	}

	return len(stages), 0, nil
}

// exportedElevation is the elevation exportProcessor writes, standing in for the
// device profile the real processor derives.
const exportedElevation = 111.0

// exportProcessor derives a stage that differs observably from its source, so a
// test can tell the exported inventory from the raw one.
type exportProcessor struct{}

func (exportProcessor) Process(stage *route.Stage) (route.Stage, error) {
	points := stage.Geometry()
	for index := range points {
		elevation := exportedElevation
		points[index].Elevation = &elevation
	}
	key := stage.Key()
	exported, err := route.NewStage(
		key.Provider(),
		key.RouteID(),
		key.StageOrder(),
		stage.Revision(),
		stage.RouteName(),
		stage.StageName(),
		points,
		stage.ContentHash(),
	)
	if err != nil {
		return route.Stage{}, fmt.Errorf("deriving the exported stage: %w", err)
	}

	return exported, nil
}

func elevationOf(t *testing.T, stage *route.Stage) float64 {
	t.Helper()
	points := stage.Geometry()
	require.NotEmpty(t, points, "the stage carries no geometry")
	require.NotNil(t, points[0].Elevation, "the stage carries no elevation")

	return *points[0].Elevation
}

// emptySourceDeletion is the gate the service reads, fixed for the length of a
// test. Live it is a settings read, so it is a function here too.
func emptySourceDeletion(allowed bool) func() bool {
	return func() bool { return allowed }
}

func newService(t *testing.T, state *fakeState, source *fakeSource, encoder *fakeEncoder, target *fakeTarget, allowEmpty bool) *Service {
	t.Helper()
	service, err := New(&Options{
		TargetIDs:                []string{"a", "b"},
		AllowEmptySourceDeletion: emptySourceDeletion(allowEmpty),
	}, state, []Source{source}, identityProcessor{}, encoder, target, nil, nil)
	require.NoError(t, err, "New()")

	return service
}

type fakeSource struct {
	provider route.Provider
	err      error
	stages   []route.Stage
	calls    int
}

func (s *fakeSource) Provider() route.Provider {
	if s.provider == "" {
		return route.ProviderVeloPlanner
	}

	return s.provider
}

func (s *fakeSource) Inventory(_ context.Context) ([]route.Stage, error) {
	s.calls++
	if s.err != nil {
		return nil, s.err
	}

	return append([]route.Stage(nil), s.stages...), nil
}

type fakeEncoder struct {
	err error
}

type identityProcessor struct{}

func (identityProcessor) Process(stage *route.Stage) (route.Stage, error) {
	return *stage, nil
}

//nolint:gocritic // This test double conforms to the production encoder contract.
func (e *fakeEncoder) Encode(_ context.Context, _ route.Stage) ([]byte, error) {
	if e.err != nil {
		return nil, e.err
	}

	return []byte("fit"), nil
}

type fakeState struct {
	trustedErr          error
	trustedCountErr     error
	storeErr            error
	authorizationErr    error
	refreshTokenErr     error
	replaceTokenErr     error
	authorizations      map[string]string
	refreshTokens       map[string]string
	mappings            map[string]map[route.Key]targetStage
	trusted             []route.Stage
	storeInventoryCalls int
}

func newFakeState(targetIDs ...string) *fakeState {
	state := &fakeState{
		authorizations: make(map[string]string, len(targetIDs)),
		refreshTokens:  make(map[string]string, len(targetIDs)),
		mappings:       make(map[string]map[route.Key]targetStage, len(targetIDs)),
	}
	for _, targetID := range targetIDs {
		state.authorizations[targetID] = authorizedState
		state.refreshTokens[targetID] = targetID
		state.mappings[targetID] = make(map[route.Key]targetStage)
	}

	return state
}

// runBoth performs a whole synchronization the way a scheduled tick does: read
// the source, then write to the targets. The source count travels into the
// merged result because it describes the library both phases worked from.
func runBoth(ctx context.Context, service *Service) Result {
	source := service.RunSource(ctx)
	if source.Outcome != OutcomeSucceeded {
		return source
	}
	targets := service.RunTargets(ctx)
	targets.SourceStages = source.SourceStages
	service.AnnotateStored(ctx)

	return targets
}

func (s *fakeState) TargetAuthorization(_ context.Context, targetID string) (string, error) {
	if s.authorizationErr != nil {
		return "", s.authorizationErr
	}

	return s.authorizations[targetID], nil
}

func (s *fakeState) RefreshToken(_ context.Context, targetID string) (string, error) {
	if s.refreshTokenErr != nil {
		return "", s.refreshTokenErr
	}

	return s.refreshTokens[targetID], nil
}

func (s *fakeState) ReplaceRefreshToken(_ context.Context, targetID, refreshToken string) error {
	if s.replaceTokenErr != nil {
		return s.replaceTokenErr
	}
	s.refreshTokens[targetID] = refreshToken

	return nil
}

func (s *fakeState) MarkNeedsReauthorization(_ context.Context, targetID string) error {
	s.authorizations[targetID] = "needs_reauthorization"

	return nil
}

func (s *fakeState) TrustedInventoryCount(_ context.Context, provider route.Provider) (int, error) {
	if s.trustedCountErr != nil {
		return 0, s.trustedCountErr
	}
	count := 0
	for _, stage := range s.trusted {
		if stage.Key().Provider() == provider {
			count++
		}
	}

	return count, nil
}

func (s *fakeState) StoreTrustedInventory(_ context.Context, provider route.Provider, stages []route.Stage) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.storeInventoryCalls++
	kept := make([]route.Stage, 0, len(s.trusted))
	for _, stage := range s.trusted {
		if stage.Key().Provider() != provider {
			kept = append(kept, stage)
		}
	}
	kept = append(kept, stages...)
	s.trusted = kept

	return nil
}

func (s *fakeState) TrustedInventory(_ context.Context) ([]route.Stage, error) {
	if s.trustedErr != nil {
		return nil, s.trustedErr
	}

	return append([]route.Stage(nil), s.trusted...), nil
}

func (s *fakeState) ForEachTargetStage(
	_ context.Context,
	targetID string,
	visit func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error,
) error {
	keys := make([]route.Key, 0, len(s.mappings[targetID]))
	for key := range s.mappings[targetID] {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].Provider() != keys[right].Provider() {
			return keys[left].Provider() < keys[right].Provider()
		}
		if keys[left].RouteID() != keys[right].RouteID() {
			return keys[left].RouteID() < keys[right].RouteID()
		}

		return keys[left].StageOrder() < keys[right].StageOrder()
	})
	for _, key := range keys {
		mapping := s.mappings[targetID][key]
		if err := visit(key.Provider(), key.RouteID(), key.StageOrder(), mapping.sourceRevision, mapping.contentHash, mapping.wahooRouteID); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeState) UpsertTargetStage(
	_ context.Context,
	targetID string,
	provider route.Provider,
	routeID int64,
	stageOrder int,
	sourceRevision, contentHash string,
	wahooRouteID int64,
) error {
	s.mappings[targetID][route.NewKey(provider, routeID, stageOrder)] = targetStage{
		sourceRevision: sourceRevision,
		contentHash:    contentHash,
		wahooRouteID:   wahooRouteID,
	}

	return nil
}

func (s *fakeState) DeleteTargetStage(_ context.Context, targetID string, provider route.Provider, routeID int64, stageOrder int) error {
	delete(s.mappings[targetID], route.NewKey(provider, routeID, stageOrder))

	return nil
}

type fakeTarget struct {
	routes             map[string]map[string]int64
	rejectRefreshToken map[string]bool
	listErr            error
	failUpdateAccess   string
	failDeleteAccess   string
	deletedAccess      []string
	deletedRouteIDs    []int64
	updatedRouteIDs    []int64
	refreshTokens      []string
	nextRouteID        int64
	// listCalls counts reconciliation's route listings, so a test can assert
	// that an unchanged library asks the target exactly once per run rather
	// than once per stage. clearCalls counts the clear's own bulk delete
	// separately: one counter for both would let a clear hide a regression in
	// how often reconciliation lists.
	listCalls  int
	clearCalls int
}

func newFakeTarget() *fakeTarget {
	return &fakeTarget{
		routes:             make(map[string]map[string]int64),
		rejectRefreshToken: make(map[string]bool),
		nextRouteID:        1_000,
	}
}

func (t *fakeTarget) RefreshAccessToken(_ context.Context, refreshToken string) (accessToken, replacementRefreshToken string, err error) {
	t.refreshTokens = append(t.refreshTokens, refreshToken)
	if t.rejectRefreshToken[refreshToken] {
		return "", "", errUnauthorized
	}

	return accessFor(refreshToken), refreshToken + "-replacement", nil
}

func (t *fakeTarget) ListOwnedRoutes(_ context.Context, accessToken string) (map[string]int64, error) {
	t.listCalls++
	if t.listErr != nil {
		return nil, t.listErr
	}
	owned := make(map[string]int64, len(t.routes[accessToken]))
	maps.Copy(owned, t.routes[accessToken])

	return owned, nil
}

func (t *fakeTarget) DeleteOwnedRoutes(ctx context.Context, accessToken string) (int, error) {
	t.clearCalls++
	if t.listErr != nil {
		return 0, t.listErr
	}
	owned := make([]int64, 0, len(t.routes[accessToken]))
	for _, routeID := range t.routes[accessToken] {
		owned = append(owned, routeID)
	}
	slices.Sort(owned)

	deleted := 0
	for _, routeID := range owned {
		if err := t.DeleteRoute(ctx, routeID, accessToken); err != nil {
			return deleted, err
		}
		deleted++
	}

	return deleted, nil
}

func (t *fakeTarget) CreateRoute(_ context.Context, accessToken string, stage *route.Stage, _ []byte) (routeID int64, err error) {
	t.nextRouteID++
	t.ensureAccess(accessToken)[stage.Key().ExternalID()] = t.nextRouteID

	return t.nextRouteID, nil
}

func (t *fakeTarget) UpdateRoute(_ context.Context, routeID int64, accessToken string, _ *route.Stage, _ []byte) (updatedRouteID int64, err error) {
	if accessToken == t.failUpdateAccess {
		return 0, errDestination
	}
	t.updatedRouteIDs = append(t.updatedRouteIDs, routeID)

	return routeID, nil
}

func (t *fakeTarget) DeleteRoute(_ context.Context, routeID int64, accessToken string) error {
	if t.failDeleteAccess == accessToken {
		return errDestination
	}
	for externalID, candidateID := range t.routes[accessToken] {
		if candidateID == routeID {
			delete(t.routes[accessToken], externalID)
			break
		}
	}
	t.deletedAccess = append(t.deletedAccess, accessToken)
	t.deletedRouteIDs = append(t.deletedRouteIDs, routeID)

	return nil
}

func (t *fakeTarget) IsUnauthorized(err error) bool {
	return errors.Is(err, errUnauthorized)
}

func (t *fakeTarget) seedRoute(targetID string, stage *route.Stage, routeID int64) {
	t.ensureAccess(accessFor(targetID))[stage.Key().ExternalID()] = routeID
}

func (t *fakeTarget) ensureAccess(accessToken string) map[string]int64 {
	routes := t.routes[accessToken]
	if routes == nil {
		routes = make(map[string]int64)
		t.routes[accessToken] = routes
	}

	return routes
}

func seedMapping(state *fakeState, targetID string, stage *route.Stage, wahooRouteID int64) {
	state.mappings[targetID][keyFor(stage)] = targetStage{
		sourceRevision: stage.Revision(),
		contentHash:    encodedContentHash(stage),
		wahooRouteID:   wahooRouteID,
	}
}

func keyFor(stage *route.Stage) route.Key {
	return stage.Key()
}

func accessFor(refreshToken string) string {
	return "access:" + refreshToken
}

func remoteID(targetID string, routeID int64) int64 {
	if targetID == "a" {
		return 100 + routeID
	}

	return 200 + routeID
}

func testStage(t *testing.T, routeID int64, stageOrder int, revision, contentHash string) route.Stage {
	t.Helper()

	return testProviderStage(t, route.ProviderVeloPlanner, routeID, stageOrder, revision, contentHash)
}

// testProviderStage2 is a second provider distinct from route.ProviderVeloPlanner,
// used to exercise multi-source behavior without a second real provider existing.
const testProviderStage2 route.Provider = "second-provider"

func testProviderStage(t *testing.T, provider route.Provider, routeID int64, stageOrder int, revision, contentHash string) route.Stage {
	t.Helper()
	stage, err := route.NewStage(
		provider,
		routeID,
		stageOrder,
		revision,
		"Route",
		"",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.401, Latitude: 49.001}},
		contentHash,
	)
	require.NoError(t, err, "NewStage()")

	return stage
}

func newMultiSourceService(t *testing.T, state *fakeState, sources []Source, target *fakeTarget, allowEmpty bool) *Service {
	t.Helper()
	service, err := New(&Options{
		TargetIDs:                []string{"a"},
		AllowEmptySourceDeletion: emptySourceDeletion(allowEmpty),
	}, state, sources, identityProcessor{}, &fakeEncoder{}, target, nil, nil)
	require.NoError(t, err, "New()")

	return service
}

// One source failing must not stop the other from being read, and must not
// widen into a target deletion for the stages the failing source last had.
func TestServiceIsolatesOneSourceFailureFromTheOthers(t *testing.T) {
	stale := testStage(t, 1, 1, "old", "old-hash")
	fresh := testProviderStage(t, testProviderStage2, 1, 1, "new", "new-hash")
	state := newFakeState("a")
	state.trusted = []route.Stage{stale}
	seedMapping(state, "a", &stale, 101)
	target := newFakeTarget()
	target.seedRoute("a", &stale, 101)
	failing := &fakeSource{provider: route.ProviderVeloPlanner, err: errors.New("source unavailable")}
	healthy := &fakeSource{provider: testProviderStage2, stages: []route.Stage{fresh}}
	service := newMultiSourceService(t, state, []Source{failing, healthy}, target, false)

	result := service.RunSource(t.Context())
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunSource() outcome")
	assert.Equal(t, FailureSource, result.Failure, "RunSource() failure")
	assert.Equal(t, []SourceResult{
		{Provider: route.ProviderVeloPlanner, Outcome: OutcomeFailed, Failure: FailureSource},
		{Provider: testProviderStage2, Outcome: OutcomeSucceeded, StageCount: 1},
	}, result.Sources, "RunSource() sources")
	assert.ElementsMatch(t, []route.Stage{stale, fresh}, state.trusted, "the failing source's stages must be kept as last known")

	// The target phase reads the merged inventory back regardless of the source
	// outcome, exactly as it does when a single source fails outright.
	targets := service.RunTargets(t.Context())
	assert.Equal(t, OutcomeSucceeded, targets.Outcome, "RunTargets() outcome")
	assert.Empty(t, target.deletedRouteIDs, "the failing source's stage must not be deleted from the target")
	assert.Equal(t, 1, targets.Created, "the healthy source's new stage must be created")
}

// Two sources at the same route ID and stage order are still two stages, not
// one, because their identity carries the provider that issued it.
func TestServiceStoresTheUnionOfMultipleSuccessfulSources(t *testing.T) {
	first := testStage(t, 1, 1, "current", "current-hash")
	second := testProviderStage(t, testProviderStage2, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	sourceOne := &fakeSource{provider: route.ProviderVeloPlanner, stages: []route.Stage{first}}
	sourceTwo := &fakeSource{provider: testProviderStage2, stages: []route.Stage{second}}
	service := newMultiSourceService(t, state, []Source{sourceOne, sourceTwo}, target, false)

	result := service.RunSource(t.Context())
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "RunSource() outcome")
	assert.Equal(t, 2, result.SourceStages, "RunSource() aggregate stage count")
	assert.ElementsMatch(t, []route.Stage{first, second}, state.trusted, "stored inventory must be the union of both sources")
}

// The empty-source deletion gate blocks the source that emptied out, but a
// sibling source that still has stages proceeds normally.
func TestServiceBlocksOnlyTheSourceThatBecameEmpty(t *testing.T) {
	staleVelo := testStage(t, 1, 1, "old", "old-hash")
	freshSecond := testProviderStage(t, testProviderStage2, 2, 1, "current", "current-hash")
	state := newFakeState("a")
	state.trusted = []route.Stage{staleVelo}
	target := newFakeTarget()
	emptied := &fakeSource{provider: route.ProviderVeloPlanner}
	healthy := &fakeSource{provider: testProviderStage2, stages: []route.Stage{freshSecond}}
	service := newMultiSourceService(t, state, []Source{emptied, healthy}, target, false)

	result := service.RunSource(t.Context())
	assert.Equal(t, OutcomeBlocked, result.Outcome, "RunSource() outcome")
	assert.Equal(t, FailureEmptySource, result.Failure, "RunSource() failure")
	assert.Equal(t, []SourceResult{
		{Provider: route.ProviderVeloPlanner, Outcome: OutcomeBlocked, Failure: FailureEmptySource},
		{Provider: testProviderStage2, Outcome: OutcomeSucceeded, StageCount: 1},
	}, result.Sources, "RunSource() sources")
	assert.ElementsMatch(t, []route.Stage{staleVelo, freshSecond}, state.trusted, "the blocked source's stages must be kept as last known")
}

// A real failure always outranks an empty-source block in the aggregate
// failure category: the guard describes no fault, so recording it once a
// later source has genuinely failed would report and alert on the wrong
// category, even though the run's outcome is correctly "failed" either way.
func TestServiceReportsARealFailureOverAnEarlierEmptySourceBlock(t *testing.T) {
	staleVelo := testStage(t, 1, 1, "old", "old-hash")
	state := newFakeState("a")
	state.trusted = []route.Stage{staleVelo}
	emptied := &fakeSource{provider: route.ProviderVeloPlanner}
	failing := &fakeSource{provider: testProviderStage2, err: errors.New("source unavailable")}
	service := newMultiSourceService(t, state, []Source{emptied, failing}, newFakeTarget(), false)

	result := service.RunSource(t.Context())
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunSource() outcome")
	assert.Equal(t, FailureSource, result.Failure, "RunSource() failure")
}

// The empty-source acknowledgement releases exactly the source it is set for.
func TestServiceEmptySourceAcknowledgementReleasesTheBlockedSource(t *testing.T) {
	staleVelo := testStage(t, 1, 1, "old", "old-hash")
	state := newFakeState("a")
	state.trusted = []route.Stage{staleVelo}
	target := newFakeTarget()
	emptied := &fakeSource{provider: route.ProviderVeloPlanner}
	service := newMultiSourceService(t, state, []Source{emptied}, target, true)

	result := service.RunSource(t.Context())
	assert.Equal(t, OutcomeSucceeded, result.Outcome, "RunSource() outcome")
	assert.Empty(t, state.trusted, "the acknowledged source's inventory must be allowed to empty out")
}

func TestServiceFailsASourceWhenItsPriorCountCannotBeRead(t *testing.T) {
	state := newFakeState("a")
	state.trustedCountErr = errors.New("state unavailable")
	service := newMultiSourceService(t, state, []Source{&fakeSource{}}, newFakeTarget(), false)

	result := service.RunSource(t.Context())
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunSource() outcome")
	assert.Equal(t, FailureState, result.Failure, "RunSource() failure")
	assert.Equal(t, []SourceResult{
		{Provider: route.ProviderVeloPlanner, Outcome: OutcomeFailed, Failure: FailureState},
	}, result.Sources, "RunSource() sources")
}

// A source reporting a stage under a provider other than its own would let one
// source's read corrupt another's stored share, so it is refused outright.
func TestServiceFailsASourceThatReportsAnotherProvidersStage(t *testing.T) {
	mismatched := testProviderStage(t, testProviderStage2, 1, 1, "current", "current-hash")
	source := &fakeSource{provider: route.ProviderVeloPlanner, stages: []route.Stage{mismatched}}
	service := newMultiSourceService(t, newFakeState("a"), []Source{source}, newFakeTarget(), false)

	result := service.RunSource(t.Context())
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunSource() outcome")
	assert.Equal(t, FailureSource, result.Failure, "RunSource() failure")
}

func TestServiceFailsASourceReportingADuplicateStage(t *testing.T) {
	one := testStage(t, 1, 1, "current", "current-hash")
	duplicate := testStage(t, 1, 1, "other", "other-hash")
	source := &fakeSource{stages: []route.Stage{one, duplicate}}
	service := newMultiSourceService(t, newFakeState("a"), []Source{source}, newFakeTarget(), false)

	result := service.RunSource(t.Context())
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunSource() outcome")
	assert.Equal(t, FailureSource, result.Failure, "RunSource() failure")
}

func TestServiceFailsASourceWhenItsShareCannotBeStored(t *testing.T) {
	state := newFakeState("a")
	state.storeErr = errors.New("state unavailable")
	source := &fakeSource{stages: []route.Stage{testStage(t, 1, 1, "current", "current-hash")}}
	service := newMultiSourceService(t, state, []Source{source}, newFakeTarget(), false)

	result := service.RunSource(t.Context())
	assert.Equal(t, OutcomeFailed, result.Outcome, "RunSource() outcome")
	assert.Equal(t, FailureState, result.Failure, "RunSource() failure")
}

func TestNewRejectsAnEmptySourceSet(t *testing.T) {
	_, err := New(
		&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)},
		newFakeState("a"), nil, identityProcessor{}, &fakeEncoder{}, newFakeTarget(), nil, nil,
	)
	assert.Error(t, err, "New() with no configured sources")
}

func TestNewRejectsANilSource(t *testing.T) {
	_, err := New(
		&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)},
		newFakeState("a"), []Source{nil}, identityProcessor{}, &fakeEncoder{}, newFakeTarget(), nil, nil,
	)
	assert.Error(t, err, "New() with a nil source")
}

func TestNewRejectsASourceWithNoProvider(t *testing.T) {
	_, err := New(
		&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)},
		newFakeState("a"), []Source{unnamedSource{}}, identityProcessor{}, &fakeEncoder{}, newFakeTarget(), nil, nil,
	)
	assert.Error(t, err, "New() with a source that names no provider")
}

func TestNewRejectsDuplicateSourceProviders(t *testing.T) {
	_, err := New(
		&Options{TargetIDs: []string{"a"}, AllowEmptySourceDeletion: emptySourceDeletion(false)},
		newFakeState("a"), []Source{&fakeSource{}, &fakeSource{}}, identityProcessor{}, &fakeEncoder{}, newFakeTarget(), nil, nil,
	)
	assert.Error(t, err, "New() with two sources naming the same provider")
}

// unnamedSource is a minimal Source whose Provider is deliberately empty,
// which fakeSource cannot express since it defaults an unset provider.
type unnamedSource struct{}

func (unnamedSource) Provider() route.Provider { return "" }

func (unnamedSource) Inventory(_ context.Context) ([]route.Stage, error) { return nil, nil }

// A run is recorded once, so its outcome is the worst of what happened. The
// operator's question is about one Wahoo account, and a run that wrote one slot
// and could not write the other has to answer it for each.
func TestServiceReportsEachTargetsOwnOutcome(t *testing.T) {
	previous := testStage(t, 1, 1, "old", "old-hash")
	desired := testStage(t, 1, 1, "new", "new-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	for _, targetID := range []string{"a", "b"} {
		seedMapping(state, targetID, &previous, remoteID(targetID, 1))
		target.seedRoute(targetID, &previous, remoteID(targetID, 1))
	}
	target.failUpdateAccess = accessFor("a")
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, []TargetResult{
		{ID: "a", Outcome: OutcomeFailed, Failure: FailureDestination},
		{ID: "b", Outcome: OutcomeSucceeded},
	}, result.Targets)
}

// A deletion limit is a guard doing its job, not a broken account, and the slot
// it held reports the same word the run does.
func TestServiceReportsABlockedTargetAsBlocked(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	for _, targetID := range []string{"a", "b"} {
		for routeID := int64(2); routeID <= 7; routeID++ {
			stale := testStage(t, routeID, 1, "old", "old-hash")
			seedMapping(state, targetID, &stale, remoteID(targetID, routeID))
			target.seedRoute(targetID, &stale, remoteID(targetID, routeID))
		}
	}
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	assert.Equal(t, []TargetResult{
		{ID: "a", Outcome: OutcomeBlocked, Failure: FailureDeletionLimit},
		{ID: "b", Outcome: OutcomeBlocked, Failure: FailureDeletionLimit},
	}, result.Targets)
}

// The source phase touches no target, so it claims nothing about one.
func TestServiceReportsNoTargetOutcomesForASourceRun(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, newFakeTarget(), false)

	assert.Empty(t, service.RunSource(t.Context()).Targets)
}
