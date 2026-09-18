package plan

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRouter answers Route calls from a fixed line, or an error when set.
type fakeRouter struct {
	err      error
	geometry []route.Point
	ways     []RoutedWay
	turns    []RoutedTurn
	calls    int
}

func (r *fakeRouter) Route(_ context.Context, _ []Waypoint, _ Profile) (Routed, error) {
	r.calls++
	if r.err != nil {
		return Routed{}, r.err
	}

	return Routed{Points: r.geometry, Ways: r.ways, Turns: r.turns}, nil
}

// fakeStore is an in-memory Store recording every call, for asserting a
// failed operation stored nothing. Each *Err field, when set, is returned
// instead of running the operation, for exercising Service's error wrapping.
type fakeStore struct {
	plans            map[int64]Plan
	insertErr        error
	getErr           error
	listErr          error
	listPublishedErr error
	replaceErr       error
	deleteErr        error
	inserts          int
	deletes          int
	raceReplace      bool // simulate a concurrent write losing to another writer.
	raceDelete       bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{plans: make(map[int64]Plan)}
}

func (s *fakeStore) InsertPlan(_ context.Context, plan *Plan) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.inserts++
	s.plans[plan.ID] = *plan

	return nil
}

func (s *fakeStore) GetPlan(_ context.Context, id int64) (Plan, bool, error) {
	if s.getErr != nil {
		return Plan{}, false, s.getErr
	}
	plan, found := s.plans[id]

	return plan, found, nil
}

func (s *fakeStore) ListPlans(_ context.Context) ([]Plan, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	plans := make([]Plan, 0, len(s.plans))
	for id := range s.plans {
		plan := s.plans[id]
		plans = append(plans, plan)
	}

	return plans, nil
}

func (s *fakeStore) ListPublishedPlans(_ context.Context) ([]Plan, error) {
	if s.listPublishedErr != nil {
		return nil, s.listPublishedErr
	}
	plans := make([]Plan, 0, len(s.plans))
	for id := range s.plans {
		plan := s.plans[id]
		if plan.Published {
			plans = append(plans, plan)
		}
	}

	return plans, nil
}

func (s *fakeStore) ReplacePlan(_ context.Context, plan *Plan, expectedVersion int64) (bool, error) {
	if s.replaceErr != nil {
		return false, s.replaceErr
	}
	if s.raceReplace {
		return false, nil
	}
	existing, found := s.plans[plan.ID]
	if !found || existing.Version != expectedVersion {
		return false, nil
	}
	s.plans[plan.ID] = *plan

	return true, nil
}

func (s *fakeStore) DeletePlan(_ context.Context, id, expectedVersion int64) (bool, error) {
	if s.deleteErr != nil {
		return false, s.deleteErr
	}
	if s.raceDelete {
		return false, nil
	}
	existing, found := s.plans[id]
	if !found || existing.Version != expectedVersion {
		return false, nil
	}
	s.deletes++
	delete(s.plans, id)

	return true, nil
}

func testGeometry() []route.Point {
	return []route.Point{
		{Longitude: 8.40, Latitude: 49.00},
		{Longitude: 8.41, Latitude: 49.01},
		{Longitude: 8.42, Latitude: 49.02},
	}
}

func testWaypoints() []Waypoint {
	return []Waypoint{{Longitude: 8.40, Latitude: 49.00}, {Longitude: 8.42, Latitude: 49.02}}
}

// testPace predicts a flat ten seconds per point, so a test can see the
// series reach each waypoint without asserting the real model's arithmetic.
type testPace struct{}

func (testPace) Predict(points []route.Point) (movingSeconds float64, series []float64, ok bool) {
	cumulative := make([]float64, len(points))
	for index := range points {
		cumulative[index] = float64(index) * 10
	}

	return cumulative[len(cumulative)-1], cumulative, len(points) > 1
}

func fixedNow() time.Time {
	return time.Date(2026, time.September, 14, 12, 0, 0, 0, time.UTC)
}

func sequentialID() func() (int64, error) {
	next := int64(1)

	return func() (int64, error) {
		id := next
		next++

		return id, nil
	}
}

func TestRouteAndCreateProduceIdenticalMeasurement(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{geometry: testGeometry()}
	store := newFakeStore()
	service := NewService(store, router, testPace{}, fixedNow, sequentialID())

	previewed, err := service.Route(t.Context(), testWaypoints(), Gravel)
	require.NoError(t, err, "Route()")

	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	assert.Equal(t, previewed.Geometry, created.Geometry, "geometry")
	assert.InDelta(t, previewed.DistanceMetres, created.DistanceMetres, 0, "distance")
	assert.InDelta(t, previewed.AscentMetres, created.AscentMetres, 0, "ascent")
}

func TestCreateStoresNothingWhenTheRouterFails(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{err: errors.New("engine unreachable")}
	store := newFakeStore()
	service := NewService(store, router, testPace{}, fixedNow, sequentialID())

	_, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.Error(t, err, "Create()")
	assert.Zero(t, store.inserts, "InsertPlan() must not be called")
}

func TestReplaceWithStaleVersionStoresNothing(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{geometry: testGeometry()}
	store := newFakeStore()
	service := NewService(store, router, testPace{}, fixedNow, sequentialID())

	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")
	callsBeforeReplace := router.calls

	_, err = service.Replace(t.Context(), created.ID, created.Version+1, "Renamed", Gravel, testWaypoints(), true, false)
	require.ErrorIs(t, err, ErrVersionMismatch, "Replace()")
	assert.Equal(t, "Sunday loop", store.plans[created.ID].Name, "the stored plan must be unchanged")
	assert.Equal(t, callsBeforeReplace, router.calls, "Replace() must not route a stale version")
}

func TestReplaceOfAnUnknownPlanReturnsNotFound(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{geometry: testGeometry()}
	service := NewService(newFakeStore(), router, testPace{}, fixedNow, sequentialID())

	_, err := service.Replace(t.Context(), 999, 1, "Renamed", Gravel, testWaypoints(), true, false)
	require.ErrorIs(t, err, ErrNotFound, "Replace()")
}

func TestDeleteWithStaleVersionStoresNothing(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{geometry: testGeometry()}
	store := newFakeStore()
	service := NewService(store, router, testPace{}, fixedNow, sequentialID())

	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	err = service.Delete(t.Context(), created.ID, created.Version+1)
	require.ErrorIs(t, err, ErrVersionMismatch, "Delete()")
	assert.Zero(t, store.deletes, "DeletePlan() must not be called")
	_, found := store.plans[created.ID]
	assert.True(t, found, "the plan must remain stored")
}

func TestDeleteOfAnUnknownPlanReturnsNotFound(t *testing.T) {
	t.Parallel()
	service := NewService(newFakeStore(), &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())

	err := service.Delete(t.Context(), 999, 1)
	require.ErrorIs(t, err, ErrNotFound, "Delete()")
}

func TestInventoryReturnsOnlyPublishedPlansOrderedByID(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{geometry: testGeometry()}
	store := newFakeStore()
	service := NewService(store, router, testPace{}, fixedNow, sequentialID())

	draft, err := service.Create(t.Context(), "Draft", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create() draft")
	published, err := service.Create(t.Context(), "Published", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create() published")
	_, err = service.Replace(t.Context(), published.ID, published.Version, "Published", Gravel, testWaypoints(), true, false)
	require.NoError(t, err, "Replace() to publish")

	routes, err := service.Inventory(t.Context())
	require.NoError(t, err, "Inventory()")
	require.Len(t, routes, 1, "Inventory() must skip the draft")

	stage := routes[0]
	assert.Equal(t, route.ProviderLocal, stage.Key().Provider(), "provider")
	assert.Equal(t, published.ID, stage.Key().SourceRouteID(), "source route ID")
	assert.Equal(t, 1, stage.Key().StageOrder(), "stage order")
	assert.Equal(t, "2026-09-14T12:00:00.000000000Z", stage.Revision(), "revision keeps a fixed-width fraction")
	assert.NotEmpty(t, stage.ContentHash(), "content hash")

	stored := store.plans[draft.ID]
	assert.False(t, stored.Published, "the draft must remain unpublished")
}

// The contract's name limit counts characters, so a multibyte name at the
// limit is accepted rather than refused for its byte length.
func TestValidateCountsNameCharactersNotBytes(t *testing.T) {
	t.Parallel()
	name, err := validate(strings.Repeat("\u00fc", 120), Gravel, []Waypoint{
		{Longitude: 8.4, Latitude: 49.0}, {Longitude: 8.5, Latitude: 49.1},
	})
	require.NoError(t, err)
	assert.Equal(t, strings.Repeat("\u00fc", 120), name)
}

func TestValidateRejectsOutOfRangeInput(t *testing.T) {
	t.Parallel()
	valid := testWaypoints()
	tooFew := []Waypoint{{Longitude: 8.4, Latitude: 49.0}}
	tooMany := make([]Waypoint, 51)
	for index := range tooMany {
		tooMany[index] = Waypoint{Longitude: 8.4, Latitude: 49.0}
	}

	tests := map[string]struct {
		name      string
		profile   Profile
		waypoints []Waypoint
	}{
		"too few waypoints":      {name: "Plan", profile: Gravel, waypoints: tooFew},
		"too many waypoints":     {name: "Plan", profile: Gravel, waypoints: tooMany},
		"unknown profile":        {name: "Plan", profile: Profile("unicycle"), waypoints: valid},
		"empty name":             {name: "   ", profile: Gravel, waypoints: valid},
		"name too long":          {name: strings.Repeat("\u00fc", 121), profile: Gravel, waypoints: valid},
		"out of range longitude": {name: "Plan", profile: Gravel, waypoints: []Waypoint{{Longitude: 200, Latitude: 49.0}, {Longitude: 8.4, Latitude: 49.0}}},
		"out of range latitude":  {name: "Plan", profile: Gravel, waypoints: []Waypoint{{Longitude: 8.4, Latitude: 200}, {Longitude: 8.4, Latitude: 49.0}}},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := validate(tt.name, tt.profile, tt.waypoints)
			assert.Error(t, err, name)
		})
	}
}

func TestRandomIDReturnsPositiveValuesBelowTwoToThe53(t *testing.T) {
	t.Parallel()
	for range 20 {
		id, err := RandomID()
		require.NoError(t, err, "RandomID()")
		assert.Positive(t, id, "RandomID()")
		assert.Less(t, id, int64(1<<53), "RandomID()")
	}
}

func TestRouteRefusesInvalidInputBeforeRouting(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{geometry: testGeometry()}
	service := NewService(newFakeStore(), router, testPace{}, fixedNow, sequentialID())
	_, err := service.Route(t.Context(), []Waypoint{{Longitude: 1, Latitude: 1}}, Trekking)
	require.Error(t, err, "one waypoint")
	_, err = service.Route(t.Context(), []Waypoint{{Longitude: math.NaN(), Latitude: 1}, {Longitude: 2, Latitude: 2}}, Trekking)
	require.Error(t, err, "NaN longitude")
	_, err = service.Route(t.Context(), []Waypoint{{Longitude: 1, Latitude: 1}, {Longitude: 2, Latitude: 2}}, Profile("walking"))
	require.Error(t, err, "unknown profile")
	assert.Zero(t, router.calls, "the router must not be asked")
}

func TestRouteWrapsAnInvalidRoutedGeometry(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{geometry: []route.Point{{Longitude: 8.4, Latitude: 49.0}}} // too few points
	service := NewService(newFakeStore(), router, testPace{}, fixedNow, sequentialID())

	_, err := service.Route(t.Context(), testWaypoints(), Gravel)
	require.Error(t, err, "Route()")
}

func TestCreateWrapsAStoreInsertError(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.insertErr = errors.New("disk full")
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())

	_, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.Error(t, err, "Create()")
}

func TestCreateWrapsAnIDGenerationError(t *testing.T) {
	t.Parallel()
	failingID := func() (int64, error) { return 0, errors.New("entropy unavailable") }
	service := NewService(newFakeStore(), &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, failingID)

	_, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.Error(t, err, "Create()")
}

func TestCreateRejectsInvalidInputBeforeRouting(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{geometry: testGeometry()}
	service := NewService(newFakeStore(), router, testPace{}, fixedNow, sequentialID())

	_, err := service.Create(t.Context(), "", Gravel, testWaypoints(), false)
	require.Error(t, err, "Create()")
	assert.Zero(t, router.calls, "Create() must not route invalid input")
}

func TestReplaceRejectsInvalidInputBeforeReadingTheStore(t *testing.T) {
	t.Parallel()
	service := NewService(newFakeStore(), &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())

	_, err := service.Replace(t.Context(), 1, 1, "", Gravel, testWaypoints(), false, false)
	require.Error(t, err, "Replace()")
}

func TestReplaceWrapsAStoreGetError(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.getErr = errors.New("read failed")
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())

	_, err := service.Replace(t.Context(), 1, 1, "Renamed", Gravel, testWaypoints(), false, false)
	require.Error(t, err, "Replace()")
}

func TestReplaceWrapsAStoreReplaceError(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())
	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	store.replaceErr = errors.New("write failed")
	_, err = service.Replace(t.Context(), created.ID, created.Version, "Renamed", Gravel, testWaypoints(), false, false)
	require.Error(t, err, "Replace()")
}

func TestDeleteWrapsAStoreGetError(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.getErr = errors.New("read failed")
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())

	err := service.Delete(t.Context(), 1, 1)
	require.Error(t, err, "Delete()")
}

func TestReplaceWrapsARouterFailureAfterTheVersionCheck(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())
	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	failingRouter := &fakeRouter{err: errors.New("engine unreachable")}
	service = NewService(store, failingRouter, testPace{}, fixedNow, sequentialID())
	_, err = service.Replace(t.Context(), created.ID, created.Version, "Renamed", Gravel, testWaypoints(), false, false)
	require.Error(t, err, "Replace()")
}

func TestReplaceReportsVersionMismatchWhenTheStoreLosesARace(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())
	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	store.raceReplace = true
	_, err = service.Replace(t.Context(), created.ID, created.Version, "Renamed", Gravel, testWaypoints(), false, false)
	require.ErrorIs(t, err, ErrVersionMismatch, "Replace()")
}

func TestDeleteSucceeds(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())
	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	require.NoError(t, service.Delete(t.Context(), created.ID, created.Version), "Delete()")
	_, found := store.plans[created.ID]
	assert.False(t, found, "the plan must be gone")
}

func TestDeleteReportsVersionMismatchWhenTheStoreLosesARace(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())
	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	store.raceDelete = true
	err = service.Delete(t.Context(), created.ID, created.Version)
	require.ErrorIs(t, err, ErrVersionMismatch, "Delete()")
}

func TestDeleteWrapsAStoreDeleteError(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())
	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	store.deleteErr = errors.New("write failed")
	err = service.Delete(t.Context(), created.ID, created.Version)
	require.Error(t, err, "Delete()")
}

func TestGetReturnsAPlanOrReportsNotFound(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())
	created, err := service.Create(t.Context(), "Sunday loop", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	got, found, err := service.Get(t.Context(), created.ID)
	require.NoError(t, err, "Get()")
	require.True(t, found, "Get() found")
	assert.Equal(t, created, got, "Get()")

	_, found, err = service.Get(t.Context(), 999)
	require.NoError(t, err, "Get()")
	assert.False(t, found, "Get() found")
}

func TestGetWrapsAStoreError(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.getErr = errors.New("read failed")
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())

	_, _, err := service.Get(t.Context(), 1)
	require.Error(t, err, "Get()")
}

func TestListReturnsEveryPlan(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())
	_, err := service.Create(t.Context(), "Draft", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")

	plans, err := service.List(t.Context())
	require.NoError(t, err, "List()")
	assert.Len(t, plans, 1, "List()")
}

func TestListWrapsAStoreError(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.listErr = errors.New("read failed")
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())

	_, err := service.List(t.Context())
	require.Error(t, err, "List()")
}

func TestInventoryWrapsAStoreError(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.listPublishedErr = errors.New("read failed")
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())

	_, err := service.Inventory(t.Context())
	require.Error(t, err, "Inventory()")
}

func TestInventoryCarriesElevationIntoTheContentHash(t *testing.T) {
	t.Parallel()
	elevated := 420.5
	router := &fakeRouter{geometry: []route.Point{
		{Longitude: 8.40, Latitude: 49.00, Elevation: &elevated},
		{Longitude: 8.41, Latitude: 49.01, Elevation: &elevated},
	}}
	store := newFakeStore()
	service := NewService(store, router, testPace{}, fixedNow, sequentialID())
	created, err := service.Create(t.Context(), "Elevated", Gravel, testWaypoints(), false)
	require.NoError(t, err, "Create()")
	_, err = service.Replace(t.Context(), created.ID, created.Version, "Elevated", Gravel, testWaypoints(), true, false)
	require.NoError(t, err, "Replace() to publish")

	routes, err := service.Inventory(t.Context())
	require.NoError(t, err, "Inventory()")
	require.Len(t, routes, 1, "Inventory()")
	assert.NotEmpty(t, routes[0].ContentHash(), "content hash")
}

func TestInventoryWrapsAnUnbuildableStoredPlan(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	store.plans[1] = Plan{
		ID: 1, Name: "Corrupt", Profile: Gravel, Published: true, Version: 1,
		Geometry: []route.Point{{Longitude: 8.4, Latitude: 49.0}}, // fewer than two points
	}
	service := NewService(store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())

	_, err := service.Inventory(t.Context())
	require.Error(t, err, "Inventory()")
}

func TestProviderReturnsLocal(t *testing.T) {
	t.Parallel()
	service := NewService(newFakeStore(), &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID())
	assert.Equal(t, route.ProviderLocal, service.Provider(), "Provider()")
}

func TestRouteReadsTheLineAtEachWaypoint(t *testing.T) {
	t.Parallel()
	service := NewService(
		newFakeStore(), &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID(),
	)

	measured, err := service.Route(t.Context(), testWaypoints(), Trekking)

	require.NoError(t, err)
	require.Len(t, measured.Progress, 2)
	assert.InDelta(t, 0, measured.Progress[0].DistanceMetres, 0, "the start has run no distance")
	assert.InDelta(t, 0, measured.Progress[0].MovingSeconds, 0, "the start has taken no time")
	assert.InDelta(t, measured.DistanceMetres, measured.Progress[1].DistanceMetres, 1,
		"the finish has run the whole line")
	// The finish matches the last of three points, which testPace times at 20 s.
	assert.InDelta(t, 20, measured.Progress[1].MovingSeconds, 0)
	assert.InDelta(t, 20, measured.MovingSeconds, 0)
}

func TestRouteLeavesAnUnpredictableLineUntimed(t *testing.T) {
	t.Parallel()
	service := NewService(
		newFakeStore(), &fakeRouter{geometry: testGeometry()}, nil, fixedNow, sequentialID(),
	)

	measured, err := service.Route(t.Context(), testWaypoints(), Trekking)

	require.NoError(t, err)
	assert.InDelta(t, 0, measured.MovingSeconds, 0)
	require.Len(t, measured.Progress, 2, "a waypoint still knows its distance")
	assert.InDelta(t, 0, measured.Progress[1].MovingSeconds, 0)
	assert.Positive(t, measured.Progress[1].DistanceMetres)
}

func TestGetPredictsAStoredPlanOnRead(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	service := NewService(
		store, &fakeRouter{geometry: testGeometry()}, testPace{}, fixedNow, sequentialID(),
	)
	created, err := service.Create(t.Context(), "Stored", Trekking, testWaypoints(), false)
	require.NoError(t, err)

	read, found, err := service.Get(t.Context(), created.ID)

	require.NoError(t, err)
	require.True(t, found)
	assert.InDelta(t, 20, read.MovingSeconds, 0, "predicted from the stored geometry, not from the row")
	require.Len(t, read.Progress, 2)
	assert.InDelta(t, 20, read.Progress[1].MovingSeconds, 0)
}

func TestRouteMarksWhereTheRiderWalks(t *testing.T) {
	t.Parallel()
	router := &fakeRouter{
		geometry: testGeometry(),
		ways: []RoutedWay{
			{EndMetres: 50, Tags: tags("highway", "residential")},
			{EndMetres: 100, Tags: tags("highway", "footway")},
		},
	}
	service := NewService(newFakeStore(), router, testPace{}, fixedNow, sequentialID())

	measured, err := service.Route(t.Context(), testWaypoints(), Trekking)

	require.NoError(t, err)
	require.Len(t, measured.Pushing, 1)
	// The engine's second half, scaled onto the normalised line's own length.
	assert.InDelta(t, measured.DistanceMetres/2, measured.Pushing[0].StartMetres, 0.01)
	assert.InDelta(t, measured.DistanceMetres, measured.Pushing[0].EndMetres, 0.01)
}

func TestCreateStoresWhereTheRiderWalks(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	router := &fakeRouter{
		geometry: testGeometry(),
		ways:     []RoutedWay{{EndMetres: 100, Tags: tags("highway", "footway")}},
	}
	service := NewService(store, router, testPace{}, fixedNow, sequentialID())

	created, err := service.Create(t.Context(), "Walked", Trekking, testWaypoints(), false)

	require.NoError(t, err)
	require.Len(t, created.Pushing, 1)
	stored, found, err := service.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, created.Pushing, stored.Pushing)
}

// A plan keeps the engine's turns with its line and remembers whether its
// course should carry them.
func TestCreateStoresTheTurnsAndWhetherTheCourseCarriesThem(t *testing.T) {
	t.Parallel()
	store := newFakeStore()
	router := &fakeRouter{
		geometry: testGeometry(),
		turns:    []RoutedTurn{{Turn: route.TurnLeft, Index: 1}},
	}
	service := NewService(store, router, testPace{}, fixedNow, sequentialID())

	created, err := service.Create(t.Context(), "Cued", Trekking, testWaypoints(), true)

	require.NoError(t, err)
	stored, found, err := service.Get(t.Context(), created.ID)
	require.NoError(t, err)
	require.True(t, found)
	assert.True(t, stored.Cues, "cues switched on")
	require.Len(t, stored.Turns, 1)
	assert.Equal(t, route.TurnLeft, stored.Turns[0].Turn, "turn")
	assert.Positive(t, stored.Turns[0].Metres, "turn placed along the line")
}

func TestCuesOfScalesTheEnginesDistanceOntoThePlansLength(t *testing.T) {
	t.Parallel()
	points := []route.Point{{Longitude: 8, Latitude: 49}, {Longitude: 8.01, Latitude: 49}, {Longitude: 8.02, Latitude: 49}}
	turns := []RoutedTurn{
		{Turn: route.TurnRoundabout, Index: 1, Exit: 2},
		{Turn: route.TurnRight, Index: 2},
		{Turn: route.TurnLeft, Index: 7},
	}

	cues := cuesOf(points, turns, 1000)

	require.Len(t, cues, 2, "a turn off the line is left out")
	assert.Equal(t, route.Cue{Turn: route.TurnRoundabout, Metres: 500, Exit: 2}, roundedCue(cues[0]), "halfway")
	assert.InDelta(t, 1000, cues[1].Metres, 1e-6, "at the end")
	assert.Nil(t, cuesOf(points, nil, 1000), "no turns")
	assert.Nil(t, cuesOf(points[:1], turns, 1000), "no line")
	assert.Nil(t, cuesOf([]route.Point{points[0], points[0]}, turns, 1000), "a line of no length")
}

func roundedCue(cue route.Cue) route.Cue {
	cue.Metres = math.Round(cue.Metres)

	return cue
}
