package sqlite

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func TestStageEnrichmentFailuresReadBackByStageAndPass(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.RecordStageSurfaceFailure(t.Context(), "veloplanner", 7, 1, "ways"), "surface failure")
	require.NoError(t, store.RecordStageDurationFailure(t.Context(), "veloplanner", 7, 1, "encode"), "duration failure")

	assert.Equal(t, []recordedFailure{
		{key: route.NewKey("veloplanner", 7, 1), pass: PassDuration, reason: "encode"},
		{key: route.NewKey("veloplanner", 7, 1), pass: PassSurface, reason: "ways"},
	}, readFailures(t, store), "recorded failures")
}

// What is here is what is wrong now, so a pass that tries again replaces what
// it last said rather than adding to it.
func TestARepeatedStageFailureReplacesTheLastOne(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.RecordStageSurfaceFailure(t.Context(), "veloplanner", 7, 1, "ways"), "first failure")
	require.NoError(t, store.RecordStageSurfaceFailure(t.Context(), "veloplanner", 7, 1, "cache"), "second failure")

	failures := readFailures(t, store)
	require.Len(t, failures, 1, "a repeated failure was recorded twice")
	assert.Equal(t, "cache", failures[0].reason, "the recorded reason")
}

// Stored and still listed as failing cannot both be true.
func TestStoringAnEnrichmentClearsWhatItLastFailedFor(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.RecordStageSurfaceFailure(t.Context(), "veloplanner", 7, 1, "ways"), "surface failure")
	require.NoError(t, store.RecordStageDurationFailure(t.Context(), "veloplanner", 7, 1, "encode"), "duration failure")

	require.NoError(t, store.StoreStageSurface(
		t.Context(), "veloplanner", 7, 1, "hash", "generation", []byte("[]"), 1,
	), "StoreStageSurface()")
	assert.Equal(t, []recordedFailure{
		{key: route.NewKey("veloplanner", 7, 1), pass: PassDuration, reason: "encode"},
	}, readFailures(t, store), "storing a classification left it listed as failing")

	require.NoError(t, store.StoreStageDuration(
		t.Context(), "veloplanner", 7, 1, "hash", "generation", "fingerprint", nil, nil,
	), "StoreStageDuration()")
	assert.Empty(t, readFailures(t, store), "storing a prediction left it listed as failing")
}

func TestRecordStageEnrichmentFailureRefusesIncompleteMetadata(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))

	require.Error(t, store.RecordStageSurfaceFailure(t.Context(), "", 7, 1, "ways"), "no provider")
	require.Error(t, store.RecordStageSurfaceFailure(t.Context(), "veloplanner", 7, 1, ""), "no reason")
}

func TestForEachStageEnrichmentFailureRefusesNoVisitor(t *testing.T) {
	t.Parallel()

	require.Error(t, openTestStore(t, testKey(1)).ForEachStageEnrichmentFailure(t.Context(), nil), "no visitor")
}

func TestStageEnrichmentFailureReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, store.RecordStageSurfaceFailure(t.Context(), "veloplanner", 7, 1, "ways"), "record")
	require.Error(t, store.ForEachStageEnrichmentFailure(t.Context(),
		func(route.Key, string, string, time.Time) error { return nil }), "read")
}

type recordedFailure struct {
	pass   string
	reason string
	key    route.Key
}

func readFailures(t *testing.T, store *Store) []recordedFailure {
	t.Helper()

	var failures []recordedFailure
	require.NoError(t, store.ForEachStageEnrichmentFailure(t.Context(),
		func(key route.Key, pass, reason string, failedAt time.Time) error {
			assert.False(t, failedAt.IsZero(), "a failure was recorded without a time")
			failures = append(failures, recordedFailure{key: key, pass: pass, reason: reason})

			return nil
		}), "ForEachStageEnrichmentFailure()")

	return failures
}

// A visitor that gives up stops the read rather than being called again for
// every remaining row.
func TestForEachStageEnrichmentFailureStopsWhenTheVisitorDoes(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	for stage := range 3 {
		require.NoError(t, store.RecordStageSurfaceFailure(
			t.Context(), "veloplanner", 7, stage, "ways",
		), "RecordStageSurfaceFailure()")
	}

	visits := 0
	giveUp := errors.New("enough")
	err := store.ForEachStageEnrichmentFailure(t.Context(),
		func(route.Key, string, string, time.Time) error {
			visits++

			return giveUp
		})

	require.ErrorIs(t, err, giveUp, "ForEachStageEnrichmentFailure()")
	assert.Equal(t, 1, visits, "the visitor was called again after giving up")
}

func TestStoreStageDurationReportsAnUnreadableDatabase(t *testing.T) {
	t.Parallel()

	store := openTestStore(t, testKey(1))
	require.NoError(t, store.Close(), "Close()")

	require.Error(t, store.StoreStageDuration(
		t.Context(), "veloplanner", 7, 1, "hash", "generation", "fingerprint", nil, nil,
	), "StoreStageDuration() on a closed database")
}
