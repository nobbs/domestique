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

	stages, err := demo.Stages()
	require.NoError(t, err)
	count, err := store.TrustedInventoryCount(t.Context())
	require.NoError(t, err)
	assert.Equal(t, len(stages), count)

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

	stages, err := demo.Stages()
	require.NoError(t, err)
	stage := &stages[0]
	key := stage.Key()

	summary, geometry, found, err := store.StageGeometry(t.Context(), key.RouteID(), key.StageOrder())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, len(stage.Geometry()), summary.PointCount)
	assert.Equal(t, stage.Title(), summary.Title())
	var coordinates [][]float64
	require.NoError(t, json.Unmarshal(geometry, &coordinates))
	assert.Len(t, coordinates, len(stage.Geometry()),
		"the endpoint serves the stored coordinates, so the fixture has to store them all")

	ranges, matched, found, err := store.StageSurface(
		t.Context(), key.RouteID(), key.StageOrder(), stage.ContentHash(),
	)
	require.NoError(t, err)
	require.True(t, found, "a surface stored under another hash would be pruned, not served")
	assert.NotEmpty(t, ranges)
	assert.Positive(t, matched)
}

func TestSeedLeavesEachSlotInTheStateItWasAskedFor(t *testing.T) {
	t.Parallel()

	store := seed(t, []demo.Slot{
		{ID: "rider-a", State: demo.SlotCurrent},
		{ID: "rider-b", State: demo.SlotFailed},
		{ID: "rider-c", State: demo.SlotUnauthorized},
	})

	authorizations := map[string]string{}
	require.NoError(t, store.ForEachTarget(t.Context(), func(id, authorization string) error {
		authorizations[id] = authorization

		return nil
	}))
	assert.Equal(t, "authorized", authorizations["rider-a"])
	assert.Equal(t, "authorized", authorizations["rider-b"])
	assert.Equal(t, "not_authorized", authorizations["rider-c"],
		"an un-onboarded slot is what the browser onboarding path is demonstrated from")

	stages, err := demo.Stages()
	require.NoError(t, err)

	behind, orphans, current := 0, 0, 0
	require.NoError(t, store.ForEachTargetStage(t.Context(), "rider-b",
		func(routeID int64, stageOrder int, sourceRevision, _ string, _ int64) error {
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
