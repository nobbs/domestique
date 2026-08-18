package sync

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"testing"

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
	if got, want := result.Outcome, OutcomeFailed; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureSource; got != want {
		t.Errorf("runBoth() failure = %q, want %q", got, want)
	}
	if got := len(target.deletedRouteIDs); got != 0 {
		t.Errorf("deleted routes = %d, want 0", got)
	}
	if got := len(target.refreshTokens); got != 0 {
		t.Errorf("refresh calls = %d, want 0", got)
	}
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
	if got, want := result.Outcome, OutcomeFailed; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureDestination; got != want {
		t.Errorf("runBoth() failure = %q, want %q", got, want)
	}
	if got, want := result.Updated, 1; got != want {
		t.Errorf("updated routes = %d, want %d", got, want)
	}
	if got, want := result.Deleted, 1; got != want {
		t.Errorf("deleted routes = %d, want %d", got, want)
	}
	if got, want := target.deletedAccess, []string{accessFor("b")}; !equalStrings(got, want) {
		t.Errorf("delete callers = %v, want %v", got, want)
	}
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
	if got, want := result.Outcome, OutcomeBlocked; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureDeletionLimit; got != want {
		t.Errorf("runBoth() failure = %q, want %q", got, want)
	}
	if got := len(target.deletedRouteIDs); got != 0 {
		t.Errorf("deleted routes = %d, want 0", got)
	}
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
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if result.Created != 0 || result.Updated != 0 || result.Deleted != 0 {
		t.Errorf("runBoth() mutation counts = %+v, want all zero", result)
	}
	key := keyFor(&desired)
	for _, targetID := range []string{"a", "b"} {
		if _, found := state.mappings[targetID][key]; !found {
			t.Errorf("target %q did not adopt the existing route", targetID)
		}
	}
	if got := len(target.deletedRouteIDs); got != 0 {
		t.Errorf("deleted routes = %d, want 0", got)
	}
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
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Created, 2; got != want {
		t.Errorf("created routes = %d, want %d", got, want)
	}
	if got := len(target.deletedRouteIDs); got != 0 {
		t.Errorf("deleted routes = %d, want 0", got)
	}
	for _, targetID := range []string{"a", "b"} {
		if _, found := target.routes[accessFor(targetID)]["manual-route"]; !found {
			t.Errorf("target %q removed a manual route", targetID)
		}
	}
}

func TestServiceMarksOnlyRejectedTargetForReauthorization(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a", "b")
	target := newFakeTarget()
	target.rejectRefreshToken["a"] = true
	service := newService(t, state, &fakeSource{stages: []route.Stage{desired}}, &fakeEncoder{}, target, false)

	result := runBoth(t.Context(), service)
	if got, want := result.Outcome, OutcomeFailed; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureAuthorization; got != want {
		t.Errorf("runBoth() failure = %q, want %q", got, want)
	}
	if got, want := state.authorizations["a"], "needs_reauthorization"; got != want {
		t.Errorf("target a authorization = %q, want %q", got, want)
	}
	if got, want := state.authorizations["b"], authorizedState; got != want {
		t.Errorf("target b authorization = %q, want %q", got, want)
	}
	if got, want := result.Created, 1; got != want {
		t.Errorf("created routes = %d, want %d", got, want)
	}
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
	if got, want := result.Outcome, OutcomeBlocked; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureEmptySource; got != want {
		t.Errorf("runBoth() failure = %q, want %q", got, want)
	}
	if got := len(target.deletedRouteIDs); got != 0 {
		t.Errorf("deleted routes = %d, want 0", got)
	}
	if got := state.storeInventoryCalls; got != 0 {
		t.Errorf("stored inventories = %d, want 0", got)
	}
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
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Deleted, 10; got != want {
		t.Errorf("deleted routes = %d, want %d", got, want)
	}
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
	if got, want := source.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("RunSource() outcome = %q, want %q", got, want)
	}
	if got, want := len(state.trusted), 1; got != want {
		t.Errorf("stored stages = %d, want %d", got, want)
	}
	if got, want := service.RunTargets(t.Context()).Outcome, OutcomeNotReady; got != want {
		t.Errorf("RunTargets() outcome = %q, want %q", got, want)
	}
	if got := len(target.routes); got != 0 {
		t.Errorf("target routes = %d, want none", got)
	}
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
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("RunTargets() outcome = %q, want %q", got, want)
	}
	if got, want := result.Created, 2; got != want {
		t.Errorf("RunTargets() created = %d, want %d", got, want)
	}
	if source.calls != 0 {
		t.Errorf("source inventory calls = %d, want 0", source.calls)
	}
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
	if got, want := result.Outcome, OutcomeFailed; got != want {
		t.Errorf("RunTargets() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureState; got != want {
		t.Errorf("RunTargets() failure = %q, want %q", got, want)
	}
	if got := len(target.deletedRouteIDs); got != 0 {
		t.Errorf("deleted routes = %d, want 0", got)
	}
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
		&Options{TargetIDs: []string{"a"}, MaxDeletionsPerTarget: 5},
		state, &fakeSource{}, identityProcessor{}, &fakeEncoder{}, target, nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result := service.RunTargets(t.Context())
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Fatalf("RunTargets() outcome = %q, want %q", got, want)
	}
	if got, want := result.Updated, 1; got != want {
		t.Errorf("RunTargets() updated = %d, want %d", got, want)
	}
	if got, want := target.updatedRouteIDs, []int64{101}; len(got) != len(want) || got[0] != want[0] {
		t.Errorf("updated routes = %v, want %v — the owned route is rewritten in place", got, want)
	}
	if got := len(target.deletedRouteIDs); got != 0 {
		t.Errorf("deleted routes = %d, want 0", got)
	}
}

func TestServiceSkipsOverlappingRun(t *testing.T) {
	service := newService(t, newFakeState("a", "b"), &fakeSource{}, &fakeEncoder{}, newFakeTarget(), false)
	service.running.Store(true)

	if got, want := runBoth(t.Context(), service).Outcome, OutcomeSkipped; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
}

func TestServiceSupportsOneTarget(t *testing.T) {
	desired := testStage(t, 1, 1, "current", "current-hash")
	state := newFakeState("a")
	target := newFakeTarget()
	service, err := New(&Options{TargetIDs: []string{"a"}, MaxDeletionsPerTarget: 5}, state, &fakeSource{stages: []route.Stage{desired}}, identityProcessor{}, &fakeEncoder{}, target, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result := runBoth(t.Context(), service)
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Created, 1; got != want {
		t.Errorf("runBoth() created = %d, want %d", got, want)
	}
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
	service, err := New(&Options{TargetIDs: []string{"a"}, MaxDeletionsPerTarget: 5}, state, &fakeSource{stages: []route.Stage{desired}}, identityProcessor{}, &fakeEncoder{}, target, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	result := runBoth(t.Context(), service)
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Updated, 1; got != want {
		t.Errorf("runBoth() updated = %d, want %d", got, want)
	}
	if got, want := state.mappings["a"][keyFor(&desired)].contentHash, encodedContentHash(&desired); got != want {
		t.Errorf("stored content hash = %q, want %q", got, want)
	}
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
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Fatalf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := annotator.calls, 1; got != want {
		t.Fatalf("annotate calls = %d, want %d", got, want)
	}
	if got, want := len(annotator.stages), 1; got != want {
		t.Fatalf("annotated stages = %d, want %d", got, want)
	}
	if got, want := annotator.createdOnEntry, 1; got != want {
		t.Errorf("routes on the target when annotation began = %d, want %d", got, want)
	}
	if got, want := elevationOf(t, &annotator.stages[0]), exportedElevation; got != want {
		t.Errorf("annotated elevation = %v, want the exported profile %v", got, want)
	}
	if got, want := elevationOf(t, &state.trusted[0]), exportedElevation; got != want {
		t.Errorf("stored elevation = %v, want the exported profile %v", got, want)
	}
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
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("runBoth() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureNone; got != want {
		t.Errorf("runBoth() failure = %q, want %q", got, want)
	}
	if got, want := result.Created, 1; got != want {
		t.Errorf("created routes = %d, want %d", got, want)
	}
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
	if got, want := result.Outcome, OutcomeBlocked; got != want {
		t.Fatalf("runBoth() outcome = %q, want %q", got, want)
	}
	if got := annotator.calls; got != 0 {
		t.Errorf("annotate calls = %d, want 0", got)
	}
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
		&Options{TargetIDs: []string{"a"}, MaxDeletionsPerTarget: 5},
		state,
		source,
		exportProcessor{},
		&fakeEncoder{},
		target,
		annotator,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

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
	if len(points) == 0 || points[0].Elevation == nil {
		t.Fatalf("stage carries no elevation")
	}

	return *points[0].Elevation
}

func newService(t *testing.T, state *fakeState, source *fakeSource, encoder *fakeEncoder, target *fakeTarget, allowEmpty bool) *Service {
	t.Helper()
	service, err := New(&Options{
		TargetIDs:                []string{"a", "b"},
		MaxDeletionsPerTarget:    5,
		AllowEmptySourceDeletion: allowEmpty,
	}, state, source, identityProcessor{}, encoder, target, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return service
}

type fakeSource struct {
	err    error
	stages []route.Stage
	calls  int
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
	authorizations      map[string]string
	refreshTokens       map[string]string
	mappings            map[string]map[stageKey]targetStage
	trusted             []route.Stage
	storeInventoryCalls int
}

func newFakeState(targetIDs ...string) *fakeState {
	state := &fakeState{
		authorizations: make(map[string]string, len(targetIDs)),
		refreshTokens:  make(map[string]string, len(targetIDs)),
		mappings:       make(map[string]map[stageKey]targetStage, len(targetIDs)),
	}
	for _, targetID := range targetIDs {
		state.authorizations[targetID] = authorizedState
		state.refreshTokens[targetID] = targetID
		state.mappings[targetID] = make(map[stageKey]targetStage)
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
	return s.authorizations[targetID], nil
}

func (s *fakeState) RefreshToken(_ context.Context, targetID string) (string, error) {
	return s.refreshTokens[targetID], nil
}

func (s *fakeState) ReplaceRefreshToken(_ context.Context, targetID, refreshToken string) error {
	s.refreshTokens[targetID] = refreshToken

	return nil
}

func (s *fakeState) MarkNeedsReauthorization(_ context.Context, targetID string) error {
	s.authorizations[targetID] = "needs_reauthorization"

	return nil
}

func (s *fakeState) TrustedInventoryCount(_ context.Context) (int, error) {
	return len(s.trusted), nil
}

func (s *fakeState) StoreTrustedInventory(_ context.Context, stages []route.Stage) error {
	s.storeInventoryCalls++
	s.trusted = append([]route.Stage(nil), stages...)

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
	visit func(routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error,
) error {
	keys := make([]stageKey, 0, len(s.mappings[targetID]))
	for key := range s.mappings[targetID] {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(left, right int) bool {
		if keys[left].routeID != keys[right].routeID {
			return keys[left].routeID < keys[right].routeID
		}

		return keys[left].stageOrder < keys[right].stageOrder
	})
	for _, key := range keys {
		mapping := s.mappings[targetID][key]
		if err := visit(key.routeID, key.stageOrder, mapping.sourceRevision, mapping.contentHash, mapping.wahooRouteID); err != nil {
			return err
		}
	}

	return nil
}

func (s *fakeState) UpsertTargetStage(
	_ context.Context,
	targetID string,
	routeID int64,
	stageOrder int,
	sourceRevision, contentHash string,
	wahooRouteID int64,
) error {
	s.mappings[targetID][stageKey{routeID: routeID, stageOrder: stageOrder}] = targetStage{
		sourceRevision: sourceRevision,
		contentHash:    contentHash,
		wahooRouteID:   wahooRouteID,
	}

	return nil
}

func (s *fakeState) DeleteTargetStage(_ context.Context, targetID string, routeID int64, stageOrder int) error {
	delete(s.mappings[targetID], stageKey{routeID: routeID, stageOrder: stageOrder})

	return nil
}

type fakeTarget struct {
	routes             map[string]map[string]int64
	rejectRefreshToken map[string]bool
	failUpdateAccess   string
	deletedAccess      []string
	deletedRouteIDs    []int64
	updatedRouteIDs    []int64
	refreshTokens      []string
	nextRouteID        int64
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

func (t *fakeTarget) RouteByExternalID(_ context.Context, accessToken, externalID string) (routeID int64, found bool, err error) {
	routeID, found = t.routes[accessToken][externalID]

	return routeID, found, nil
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

func keyFor(stage *route.Stage) stageKey {
	key := stage.Key()

	return stageKey{routeID: key.RouteID(), stageOrder: key.StageOrder()}
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
	stage, err := route.NewStage(
		routeID,
		stageOrder,
		revision,
		"Route",
		"",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.401, Latitude: 49.001}},
		contentHash,
	)
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	return stage
}

func equalStrings(left, right []string) bool {
	return slices.Equal(left, right)
}
