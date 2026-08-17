package sync

import (
	"context"
	"errors"
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

	result := service.Run(t.Context())
	if got, want := result.Outcome, OutcomeFailed; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureSource; got != want {
		t.Errorf("Run() failure = %q, want %q", got, want)
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

	result := service.Run(t.Context())
	if got, want := result.Outcome, OutcomeFailed; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureDestination; got != want {
		t.Errorf("Run() failure = %q, want %q", got, want)
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

	result := service.Run(t.Context())
	if got, want := result.Outcome, OutcomeBlocked; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureDeletionLimit; got != want {
		t.Errorf("Run() failure = %q, want %q", got, want)
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

	result := service.Run(t.Context())
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
	if result.Created != 0 || result.Updated != 0 || result.Deleted != 0 {
		t.Errorf("Run() mutation counts = %+v, want all zero", result)
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

	result := service.Run(t.Context())
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
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

	result := service.Run(t.Context())
	if got, want := result.Outcome, OutcomeFailed; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureAuthorization; got != want {
		t.Errorf("Run() failure = %q, want %q", got, want)
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

	result := service.Run(t.Context())
	if got, want := result.Outcome, OutcomeBlocked; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
	if got, want := result.Failure, FailureEmptySource; got != want {
		t.Errorf("Run() failure = %q, want %q", got, want)
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

	result := service.Run(t.Context())
	if got, want := result.Outcome, OutcomeSucceeded; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
	if got, want := result.Deleted, 10; got != want {
		t.Errorf("deleted routes = %d, want %d", got, want)
	}
}

func TestServiceSkipsOverlappingRun(t *testing.T) {
	service := newService(t, newFakeState("a", "b"), &fakeSource{}, &fakeEncoder{}, newFakeTarget(), false)
	service.running.Store(true)

	if got, want := service.Run(t.Context()).Outcome, OutcomeSkipped; got != want {
		t.Errorf("Run() outcome = %q, want %q", got, want)
	}
}

func newService(t *testing.T, state *fakeState, source *fakeSource, encoder *fakeEncoder, target *fakeTarget, allowEmpty bool) *Service {
	t.Helper()
	service, err := New(&Options{
		TargetIDs:                []string{"a", "b"},
		MaxDeletionsPerTarget:    5,
		AllowEmptySourceDeletion: allowEmpty,
	}, state, source, encoder, target)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return service
}

type fakeSource struct {
	err    error
	stages []route.Stage
}

func (s *fakeSource) Inventory(_ context.Context) ([]route.Stage, error) {
	if s.err != nil {
		return nil, s.err
	}

	return append([]route.Stage(nil), s.stages...), nil
}

type fakeEncoder struct {
	err error
}

//nolint:gocritic // This test double conforms to the production encoder contract.
func (e *fakeEncoder) Encode(_ context.Context, _ route.Stage) ([]byte, error) {
	if e.err != nil {
		return nil, e.err
	}

	return []byte("fit"), nil
}

type fakeState struct {
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
		contentHash:    stage.ContentHash(),
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
