package sqlite

import (
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// storeRideAt stores one ride for rider-a starting at starts, derived when asked.
func storeRideAt(t *testing.T, store *Store, id int64, starts time.Time, derived bool) {
	t.Helper()

	require.NoError(t, store.StoreActivity(t.Context(), "rider-a",
		activity.Listing{ID: id, TypeID: 15, LocationID: 1, Starts: starts},
		activity.Summary{DistanceMetres: 1000, MovingSeconds: 3600, ElapsedSeconds: 3900, Raw: []byte(`{}`)},
		starts,
	), "StoreActivity()")
	if derived {
		require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", id, derivedMetrics(testInputs(), testCoefficients())),
			"StoreActivityMetrics()")
	}
}

func testAnalysis(text string) activity.Analysis {
	return activity.Analysis{AnalysedAt: activityNow(), Text: text, Model: "model", PromptRevision: 1}
}

func pendingIDs(pending []activity.PendingAnalysis) []int64 {
	ids := make([]int64, 0, len(pending))
	for _, ride := range pending {
		ids = append(ids, ride.ID)
	}

	return ids
}

func TestRecordAnalysisEnabledKeepsTheFirstInstant(t *testing.T) {
	t.Parallel()
	store := openTestStore(t, testKey(1))

	first, err := store.RecordAnalysisEnabled(t.Context(), activityNow())
	require.NoError(t, err, "RecordAnalysisEnabled()")
	later, err := store.RecordAnalysisEnabled(t.Context(), activityNow().Add(24*time.Hour))
	require.NoError(t, err, "RecordAnalysisEnabled() on a later start")

	assert.Equal(t, activityNow(), first)
	assert.Equal(t, activityNow(), later, "a later start never moves the instant")
}

// No backfill: a ride started before the instant is never owed one, even when
// it was stored after it.
func TestActivitiesAwaitingAnalysisListsDerivedUnanalysedRidesSinceTheInstant(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	since := activityNow()
	storeRideAt(t, store, 1, since.Add(-time.Minute), true)
	storeRideAt(t, store, 2, since.Add(2*time.Hour), true)
	storeRideAt(t, store, 3, since, true)
	storeRideAt(t, store, 4, since.Add(time.Hour), false)
	storeRideAt(t, store, 5, since.Add(3*time.Hour), true)
	require.NoError(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 5, testAnalysis("said")), "StoreActivityAnalysis()")

	pending, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", since, 10)
	require.NoError(t, err, "ActivitiesAwaitingAnalysis()")
	assert.Equal(t, []int64{3, 2}, pendingIDs(pending), "oldest first; not before the instant, underived or analysed")
	assert.Equal(t, since, pending[0].StartedAt)

	bounded, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", since, 1)
	require.NoError(t, err, "ActivitiesAwaitingAnalysis() bounded")
	assert.Equal(t, []int64{3}, pendingIDs(bounded))

	other, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-b", since, 10)
	require.NoError(t, err, "ActivitiesAwaitingAnalysis() for another target")
	assert.Empty(t, other)
}

func TestAnalysesBeforeReadsEarlierRidesNewestFirst(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	start := activityNow()
	for id := int64(1); id <= 4; id++ {
		storeRideAt(t, store, id, start.Add(time.Duration(id)*time.Hour), true)
		analysis := testAnalysis("ride " + string(rune('0'+id)))
		analysis.PromptRevision = int(id)
		require.NoError(t, store.StoreActivityAnalysis(t.Context(), "rider-a", id, analysis), "StoreActivityAnalysis()")
	}

	earlier, err := store.AnalysesBefore(t.Context(), "rider-a", start.Add(4*time.Hour), 2)
	require.NoError(t, err, "AnalysesBefore()")
	assert.Equal(t, []activity.Analysis{
		{AnalysedAt: activityNow(), Text: "ride 3", Model: "model", PromptRevision: 3},
		{AnalysedAt: activityNow(), Text: "ride 2", Model: "model", PromptRevision: 2},
	}, earlier, "the ride itself and anything after it are not its context")
}

// A derivation that takes a ride's figures away takes what was said about
// them, and the ride is owed an analysis again once it has figures.
func TestADerivationThatYieldsNothingRemovesTheAnalysis(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	storeRideAt(t, store, 1, activityNow(), true)
	require.NoError(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 1, testAnalysis("said")), "StoreActivityAnalysis()")

	require.NoError(t, store.StoreRideDerivation(t.Context(), "rider-a", 1, nil, nil, activity.RideMetrics{}),
		"StoreRideDerivation() with nothing derived")
	earlier, err := store.AnalysesBefore(t.Context(), "rider-a", activityNow().Add(time.Hour), 5)
	require.NoError(t, err, "AnalysesBefore()")
	assert.Empty(t, earlier)

	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"StoreActivityMetrics() again")
	pending, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", activityNow(), 5)
	require.NoError(t, err, "ActivitiesAwaitingAnalysis()")
	assert.Equal(t, []int64{1}, pendingIDs(pending))
}

func TestClearActivityMetricsTakesTheAnalysesWithIt(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	storeRideAt(t, store, 1, activityNow(), true)
	require.NoError(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 1, testAnalysis("said")), "StoreActivityAnalysis()")

	_, err := store.ClearActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ClearActivityMetrics()")

	earlier, err := store.AnalysesBefore(t.Context(), "rider-a", activityNow().Add(time.Hour), 5)
	require.NoError(t, err, "AnalysesBefore()")
	assert.Empty(t, earlier)
}

func TestStoreActivityAnalysisRefusesTextOutsideTheBound(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	storeRideAt(t, store, 1, activityNow(), true)

	require.Error(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 1, testAnalysis("")), "empty text")
	require.Error(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 1, testAnalysis(strings.Repeat("é", 2001))),
		"text over two thousand characters")
	assert.NoError(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 1, testAnalysis(strings.Repeat("é", 2000))),
		"the bound counts characters, not bytes")
}
