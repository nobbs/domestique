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

	storeRecordedRide(t, store, activity.Listing{ID: id, TypeID: 15, LocationID: 1, Starts: starts}, derived)
}

func storeRecordedRide(t *testing.T, store *Store, listing activity.Listing, derived bool) {
	t.Helper()

	id, starts := listing.ID, listing.Starts
	require.NoError(t, store.StoreActivity(t.Context(), "rider-a",
		listing,
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

	pending, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", since, since, nil, 10)
	require.NoError(t, err, "ActivitiesAwaitingAnalysis()")
	assert.Equal(t, []int64{3, 2}, pendingIDs(pending), "oldest first; not before the instant, underived or analysed")
	assert.Equal(t, since, pending[0].StartedAt)

	bounded, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", since, since, nil, 1)
	require.NoError(t, err, "ActivitiesAwaitingAnalysis() bounded")
	assert.Equal(t, []int64{3}, pendingIDs(bounded))

	other, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-b", since, since, nil, 10)
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
	pending, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", activityNow(), activityNow(), nil, 5)
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

// A limit is the run's bound; zero or less would silently mean nothing or everything.
func TestAnalysisReadsRefuseANonPositiveLimit(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)

	_, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", activityNow(), activityNow(), nil, 0)
	require.Error(t, err, "ActivitiesAwaitingAnalysis() with no limit")
	_, err = store.AnalysesBefore(t.Context(), "rider-a", activityNow(), -1)
	assert.Error(t, err, "AnalysesBefore() with a negative limit")
}

// A head unit's indoor ride waits, from its end, for the Zwift copy that may
// replace it; the Zwift copy itself, an outdoor ride and a ride past the hold do not.
func TestActivitiesAwaitingAnalysisHoldsARecentHeadUnitIndoorRide(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	since, heldSince := activityNow(), activityNow().Add(7*time.Hour)
	const indoor = 12
	ends := func(at time.Time) time.Time { return at.Add(-3900 * time.Second) }
	storeRecordedRide(t, store, activity.Listing{ID: 1, TypeID: indoor, Starts: ends(heldSince)}, true)
	storeRecordedRide(t, store, activity.Listing{ID: 2, TypeID: indoor, Starts: ends(heldSince.Add(-time.Second))}, true)
	storeRecordedRide(t, store, activity.Listing{ID: 3, TypeID: indoor, Starts: heldSince.Add(time.Hour), Provider: activity.ProviderZwift}, true)
	storeRecordedRide(t, store, activity.Listing{ID: 4, TypeID: 15, Starts: heldSince.Add(time.Hour)}, true)

	held, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", since, heldSince, []int{indoor}, 10)
	require.NoError(t, err, "ActivitiesAwaitingAnalysis()")
	assert.Equal(t, []int64{2, 3, 4}, pendingIDs(held))

	unheld, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", since, heldSince, nil, 10)
	require.NoError(t, err, "ActivitiesAwaitingAnalysis() for a rider without Zwift")
	assert.Equal(t, []int64{2, 1, 3, 4}, pendingIDs(unheld), "no held types holds nothing")
}

// A records re-read drops the metrics row but not the analysis: removal is a
// derivation's to make, and a records-version bump must not re-analyse every ride.
func TestARecordsReReadKeepsTheAnalysis(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	storeRideAt(t, store, 1, activityNow(), true)
	require.NoError(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 1, testAnalysis("said")), "StoreActivityAnalysis()")

	require.NoError(t, store.StoreActivityRecords(t.Context(), "rider-a", 1, activity.FIT{
		Records: []activity.Record{{Time: activityNow()}},
	}, activity.RecordsVersion), "StoreActivityRecords()")
	derived, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	require.NotContains(t, derived, int64(1), "the re-read drops the metrics row")

	earlier, err := store.AnalysesBefore(t.Context(), "rider-a", activityNow().Add(time.Hour), 5)
	require.NoError(t, err, "AnalysesBefore()")
	assert.Len(t, earlier, 1)
	require.NoError(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, derivedMetrics(testInputs(), testCoefficients())),
		"StoreActivityMetrics() after the re-read")
	pending, err := store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", activityNow(), activityNow(), nil, 5)
	require.NoError(t, err, "ActivitiesAwaitingAnalysis()")
	assert.Empty(t, pending, "re-derived, the ride is not owed again")
}

func TestAnalysisReadsAndWritesReportAnUnreadableStore(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	require.NoError(t, store.Close(), "Close()")

	_, err := store.RecordAnalysisEnabled(t.Context(), activityNow())
	require.ErrorContains(t, err, "recording when the analysis was enabled")
	_, err = store.ActivitiesAwaitingAnalysis(t.Context(), "rider-a", activityNow(), activityNow(), nil, 5)
	require.ErrorContains(t, err, "listing activities awaiting analysis")
	_, err = store.AnalysesBefore(t.Context(), "rider-a", activityNow(), 5)
	require.ErrorContains(t, err, "reading earlier activity analyses")
	assert.ErrorContains(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 1, testAnalysis("said")),
		"recording an activity analysis")
}

// A failed analysis delete rolls the metrics delete back with it.
func TestAnAnalysisDeleteThatFailsKeepsTheMetricsRow(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	storeRideAt(t, store, 1, activityNow(), true)
	_, err := store.database.ExecContext(t.Context(), `DROP TABLE activity_analyses`)
	require.NoError(t, err)

	require.ErrorContains(t, store.StoreActivityMetrics(t.Context(), "rider-a", 1, activity.RideMetrics{}),
		"clearing the activity analysis")
	_, err = store.ClearActivityMetrics(t.Context(), "rider-a")
	require.ErrorContains(t, err, "clearing the activity analyses")

	derived, err := store.ActivityMetrics(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityMetrics()")
	assert.Contains(t, derived, int64(1))
}

// zwift:poll deletes the head unit's copy of an indoor ride; its analysis must go
// with it rather than refuse the delete.
func TestDeletingATrainerCopyTakesItsAnalysis(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	const indoor = 12
	storeRecordedRide(t, store, activity.Listing{ID: 1, TypeID: indoor, Starts: activityNow()}, true)
	require.NoError(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 1, testAnalysis("said")), "StoreActivityAnalysis()")

	removed, err := store.DeleteTrainerCopy(t.Context(), "rider-a", activityNow(), time.Minute, []int{indoor})
	require.NoError(t, err, "DeleteTrainerCopy()")
	require.Equal(t, 1, removed)

	var analyses int
	require.NoError(t, store.database.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM activity_analyses`).Scan(&analyses))
	assert.Zero(t, analyses)
}

func TestHoldsHeadUnitRideFindsOnlyARecentWahooRideOfTheTypes(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	const indoor = 12
	since := activityNow()
	storeRecordedRide(t, store, activity.Listing{ID: 1, TypeID: indoor, Starts: since.Add(-3901 * time.Second)}, false)
	storeRecordedRide(t, store, activity.Listing{ID: 2, TypeID: indoor, Starts: since, Provider: activity.ProviderZwift}, false)
	storeRecordedRide(t, store, activity.Listing{ID: 3, TypeID: 15, Starts: since}, false)

	held, err := store.HoldsHeadUnitRide(t.Context(), "rider-a", []int{indoor}, since)
	require.NoError(t, err, "HoldsHeadUnitRide()")
	assert.False(t, held, "ended before the instant, a Zwift ride, or outdoors")

	storeRecordedRide(t, store, activity.Listing{ID: 4, TypeID: indoor, Starts: since.Add(-3900 * time.Second)}, false)
	held, err = store.HoldsHeadUnitRide(t.Context(), "rider-a", []int{indoor}, since)
	require.NoError(t, err, "HoldsHeadUnitRide()")
	assert.True(t, held)

	require.NoError(t, store.Close(), "Close()")
	_, err = store.HoldsHeadUnitRide(t.Context(), "rider-a", []int{indoor}, since)
	assert.ErrorContains(t, err, "checking for a head unit ride")
}

func TestActivityAnalysesReadsEachRidesAnalysisForOneTarget(t *testing.T) {
	t.Parallel()
	store := metricsStore(t)
	storeRideAt(t, store, 1, activityNow(), true)
	storeRideAt(t, store, 2, activityNow().Add(time.Hour), true)
	require.NoError(t, store.StoreActivityAnalysis(t.Context(), "rider-a", 1, testAnalysis("said")), "StoreActivityAnalysis()")

	analyses, err := store.ActivityAnalyses(t.Context(), "rider-a")
	require.NoError(t, err, "ActivityAnalyses()")
	assert.Equal(t, map[int64]activity.Analysis{1: testAnalysis("said")}, analyses)

	other, err := store.ActivityAnalyses(t.Context(), "rider-b")
	require.NoError(t, err, "ActivityAnalyses() for another target")
	assert.Empty(t, other)

	require.NoError(t, store.Close(), "Close()")
	_, err = store.ActivityAnalyses(t.Context(), "rider-a")
	assert.ErrorContains(t, err, "reading the activity analyses")
}
