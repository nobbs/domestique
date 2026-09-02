package demo_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/demo"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/sqlite"
)

// seededAt is a fixed instant, so a seeded database is a value a test can
// compare rather than a moving one.
func seededAt() time.Time {
	return time.Date(2026, time.April, 12, 9, 30, 0, 0, time.UTC)
}

func seed(t *testing.T, slots []demo.Slot) *sqlite.Store {
	t.Helper()

	var key [32]byte
	for index := range key {
		key[index] = byte(index)
	}
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"), key)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })
	require.NoError(t, demo.Seed(t.Context(), store, slots, seededAt()))

	return store
}

func TestSeedFillsAStoreWithTheWholeLibrary(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{{ID: "rider-a", State: demo.SlotCurrent}})

	stages, err := demo.Routes()
	require.NoError(t, err)
	// Counted per source and summed, because the store now isolates one
	// source's trusted inventory from another's — the whole library is the two
	// counts together, not either alone.
	veloplanner, err := store.TrustedInventoryCount(t.Context(), route.ProviderVeloPlanner)
	require.NoError(t, err)
	komoot, err := store.TrustedInventoryCount(t.Context(), route.ProviderKomoot)
	require.NoError(t, err)
	assert.Equal(t, len(stages), veloplanner+komoot)

	summaries := 0
	require.NoError(t, store.ForEachStageSummary(t.Context(), func(_ route.Summary) error {
		summaries++

		return nil
	}))
	assert.Equal(t, len(stages), summaries, "every stored stage has a summary the stage list reads")

	classified, total, err := store.SurfaceCoverage(t.Context())
	require.NoError(t, err)
	assert.Equal(t, len(stages), total)
	assert.Less(t, classified, total, "one stage is deliberately left unclassified")
	assert.Positive(t, classified)
}

func TestSeedStoresGeometryTheEndpointCanServe(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{{ID: "rider-a", State: demo.SlotCurrent}})

	stages, err := demo.Routes()
	require.NoError(t, err)
	stage := &stages[0]
	key := stage.Key()

	summary, geometry, _, found, err := store.StageGeometry(t.Context(), key.Provider(), key.SourceRouteID(), key.StageOrder())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, len(stage.Geometry()), summary.PointCount)
	assert.Equal(t, stage.Title(), summary.Title())
	var coordinates [][]float64
	require.NoError(t, json.Unmarshal(geometry, &coordinates))
	assert.Len(t, coordinates, len(stage.Geometry()),
		"the endpoint serves the stored coordinates, so the fixture has to store them all")

	ranges, matched, found, err := store.StageSurface(
		t.Context(), key.Provider(), key.SourceRouteID(), key.StageOrder(), stage.ContentHash(),
	)
	require.NoError(t, err)
	require.True(t, found, "a surface stored under another hash would be pruned, not served")
	assert.NotEmpty(t, ranges)
	assert.Positive(t, matched)
}

// A stage with a complete elevation profile must carry a predicted moving
// time whose cumulative series ends exactly at that total: those are the two
// numbers the browser UI reads to draw a stage's own timeline, and a fixture
// where they disagree would look like a rounding bug rather than a demo.
func TestSeedStoresAPredictedDurationForAFullyElevatedStage(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{{ID: "rider-a", State: demo.SlotCurrent}})

	stages, err := demo.Routes()
	require.NoError(t, err)
	stage := &stages[0] // Synthetic Rhine Traverse / Valley floor: profiled end to end.
	key := stage.Key()

	summary, _, cumulativeSecondsRaw, found, err := store.StageGeometry(
		t.Context(), key.Provider(), key.SourceRouteID(), key.StageOrder(),
	)
	require.NoError(t, err)
	require.True(t, found)
	require.NotNil(t, summary.MovingSeconds, "a fully-elevated stage must carry a predicted moving time")
	assert.Positive(t, *summary.MovingSeconds)

	var cumulativeSeconds []float64
	require.NoError(t, json.Unmarshal(cumulativeSecondsRaw, &cumulativeSeconds))
	require.NotEmpty(t, cumulativeSeconds)
	assert.InDelta(t, *summary.MovingSeconds, cumulativeSeconds[len(cumulativeSeconds)-1], 1e-6,
		"the cumulative series should end exactly at the stage's own moving time")

	contentHash, surfaceGeneration, coefficientFingerprint, found, err := store.StageDurationFingerprint(
		t.Context(), key.Provider(), key.SourceRouteID(), key.StageOrder(),
	)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, stage.ContentHash(), contentHash)
	assert.NotEmpty(t, surfaceGeneration)
	assert.NotEmpty(t, coefficientFingerprint)
}

// ridemodel.Predict cannot answer for a stage with no elevation at all, or one
// whose profile stops partway, and the fixture must reproduce that asymmetry
// rather than fabricate a time the model itself would have refused to give.
func TestSeedLeavesElevationlessStagesWithNoPredictedDuration(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{{ID: "rider-a", State: demo.SlotCurrent}})

	stages, err := demo.Routes()
	require.NoError(t, err)

	for _, routeID := range []int64{4103, 4104} { // no profile, and a profile with a hole in it.
		var stage *route.Route
		for index := range stages {
			if stages[index].Key().SourceRouteID() == routeID {
				stage = &stages[index]
			}
		}
		require.NotNil(t, stage, "fixture must still contain route %d", routeID)
		key := stage.Key()

		summary, _, cumulativeSecondsRaw, found, err := store.StageGeometry(
			t.Context(), key.Provider(), key.SourceRouteID(), key.StageOrder(),
		)
		require.NoError(t, err)
		require.True(t, found)
		assert.Nil(t, summary.MovingSeconds, "route %d must not carry a predicted moving time", routeID)
		assert.Empty(t, cumulativeSecondsRaw, "route %d must not carry a cumulative series", routeID)
	}
}

func TestSeedLeavesEachSlotInTheStateItWasAskedFor(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{
		{ID: "rider-a", State: demo.SlotCurrent},
		{ID: "rider-b", State: demo.SlotFailed},
		{ID: "rider-c", State: demo.SlotUnauthorized},
	})

	authorizations, owners := map[string]string{}, map[string]string{}
	require.NoError(t, store.ForEachTarget(t.Context(), func(id, authorizationState, ownerSubject string) error {
		authorizations[id] = authorizationState
		owners[id] = ownerSubject

		return nil
	}))
	assert.Equal(t, "authorized", authorizations["rider-a"])
	assert.Equal(t, "authorized", authorizations["rider-b"])
	assert.Equal(t, "not_authorized", authorizations["rider-c"],
		"an un-onboarded slot is what the browser onboarding path is demonstrated from")
	assert.Equal(t, "rider-a", owners["rider-a"], "a seeded slot is owned by its own subject")
	assert.Equal(t, "rider-c", owners["rider-c"], "even one that has not onboarded")

	stages, err := demo.Routes()
	require.NoError(t, err)

	behind, orphans, current := 0, 0, 0
	require.NoError(t, store.ForEachTargetStage(t.Context(), "rider-b",
		func(_ route.Provider, routeID int64, stageOrder int, sourceRevision, _ string, _ int64) error {
			switch {
			case routeID == 4199:
				orphans++
			case sourceRevision == demo.Revision(routeID, stageOrder):
				current++
			default:
				behind++
			}

			return nil
		}))
	assert.Equal(t, 1, behind, "a slot that owes work reads as lagging rather than as identical rows")
	assert.Equal(t, 1, orphans, "a stage the library dropped is outstanding work too")
	assert.Equal(t, len(stages)-1, current)

	outcomes := map[string]string{}
	require.NoError(t, store.ForEachTargetRun(t.Context(),
		func(targetID string, _ time.Time, outcome, _ string) error {
			outcomes[targetID] = outcome

			return nil
		}))
	assert.Equal(t, "succeeded", outcomes["rider-a"])
	assert.Equal(t, "failed", outcomes["rider-b"])
	assert.NotContains(t, outcomes, "rider-c", "a slot that was never written to has no run of its own")
}

func TestSeedRecordsARunAtTheInstantItWasGiven(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{{ID: "rider-a", State: demo.SlotCurrent}})

	completedAt, outcome, _, sourceStages, _, _, _, found, err := store.LastSyncRun(t.Context())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "succeeded", outcome)
	assert.Positive(t, sourceStages)
	assert.WithinDuration(t, seededAt(), completedAt, 10*time.Minute,
		"the clock is the caller's, so a fixture is dated and not stamped")
}

func TestSeedRefusesAnEmptySlotList(t *testing.T) {
	t.Parallel()

	require.Error(t, demo.Seed(context.Background(), nil, nil, seededAt()))
}

// failingOwnerState is a real store wrapped to fail only at the one call
// Seed makes after every write that does not involve a target's ownership,
// so the failure it returns can only have come from there.
type failingOwnerState struct {
	*sqlite.Store
}

func (f failingOwnerState) EnsureTargetOwner(context.Context, string) error {
	return assert.AnError
}

func TestSeedReportsAFailureToRecordATargetsOwner(t *testing.T) {
	t.Parallel()

	var key [32]byte
	for index := range key {
		key[index] = byte(index)
	}
	store, err := sqlite.Open(t.Context(), filepath.Join(t.TempDir(), "state.db"), key)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, store.Close()) })

	err = demo.Seed(
		t.Context(), failingOwnerState{store}, []demo.Slot{{ID: "rider-a", State: demo.SlotCurrent}}, seededAt(),
	)
	require.ErrorIs(t, err, assert.AnError)
}
