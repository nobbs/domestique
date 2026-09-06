package activity

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The delivery this exists for: one notified ride read from the account, stored,
// and its samples filled, for the two requests a notification is worth.
func TestRecordStoresTheNotifiedActivityAndItsRecords(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 7, Starts: at(7), TypeID: 15, LocationID: 1}}

	result := newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7)

	require.Equal(t, Polled, result.Outcome)
	assert.Equal(t, 1, result.Stored)
	assert.Equal(t, 1, result.RecordsStored)
	require.Len(t, store.stored, 1)
	assert.Equal(t, int64(7), store.stored[0].listing.ID)
	assert.Contains(t, store.records, int64(7))
	assert.Equal(t, 2, source.activityCalls+source.summaryCalls, "a notification cost more than two requests")
	assert.Zero(t, source.listed, "the whole account was listed for one notification")
}

// A workout whose own entry carried its summary costs the second request nothing.
func TestRecordAsksOnlyOnceWhenTheEntryCarriesItsSummary(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	summary := Summary{DistanceMetres: 7, Raw: []byte(`{}`)}
	source.listings = []Listing{{ID: 7, Starts: at(7), TypeID: 15, LocationID: 1, Summary: &summary}}

	require.Equal(t, Polled, newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7).Outcome)
	assert.Equal(t, 1, source.activityCalls+source.summaryCalls)
	assert.Empty(t, source.summarized, "a summary the entry carried was asked for again")
}

// The notification is a hint about a workout, not about what it is: a run the
// same account holds is read once and then left alone.
func TestRecordIgnoresAnActivityItDoesNotRecord(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.unrecordable = []int{1}
	source.listings = []Listing{{ID: 7, Starts: at(7), TypeID: 1}}

	result := newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7)

	assert.Equal(t, Unchanged, result.Outcome)
	assert.Empty(t, store.stored, "a workout this service does not record was stored")
	assert.Zero(t, source.summaryCalls, "an unrecorded workout spent a summary request")
}

// Wahoo may deliver the same notification more than once; a ride already stored
// and filled is nothing to do rather than a second reading of it.
func TestRecordChangesNothingForAnActivityAlreadyRecorded(t *testing.T) {
	store := newFakeStore()
	store.known = []int64{7}
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 7, Starts: at(7), TypeID: 15, LocationID: 1}}

	result := newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7)

	assert.Equal(t, Unchanged, result.Outcome)
	assert.Empty(t, store.stored)
	assert.Zero(t, source.summaryCalls, "a stored workout was read again")
}

// A summary only that workout refuses is skipped, so the poll's own retry
// schedule decides when it is asked for again rather than this delivery.
func TestRecordSkipsAnUnreadableSummary(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 7, Starts: at(7), TypeID: 15, LocationID: 1}}
	source.summaryErrs[7] = errUnreadable

	result := newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7)

	assert.Equal(t, Polled, result.Outcome)
	assert.Equal(t, 1, result.Skipped)
	require.Len(t, store.skipped, 1)
	assert.Equal(t, int64(7), store.skipped[0].id)
	assert.Empty(t, store.stored)
}

// A workout the account does not hold is the same: the notification named it,
// and the account is what decides.
func TestRecordSkipsAWorkoutTheAccountDoesNotHold(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)

	result := newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7)

	assert.Equal(t, 1, result.Skipped)
	require.Len(t, store.skipped, 1)
	assert.Equal(t, int64(7), store.skipped[0].id)
}

// A refused refresh marks the slot for interactive OAuth exactly as a poll's does.
func TestRecordMarksReauthorizationWhenTheRefreshIsRefused(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.refreshErr = errUnauthorized

	result := newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7)

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureAuthorization, result.Failure)
	assert.True(t, store.markedForReauthorization, "the slot was not marked for reauthorization")
	assert.Zero(t, source.activityCalls, "the workout was read on a refused grant")
}

// A download that failed for the CDN's own reasons leaves the activity stored
// and still awaiting its samples, which the next poll fills.
func TestRecordLeavesTheActivityPendingWhenTheDownloadFails(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 7, Starts: at(7), TypeID: 15, LocationID: 1}}
	source.downloadErr = errUpstream

	result := newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7)

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureUpstream, result.Failure)
	assert.Equal(t, 1, result.Stored)
	assert.Empty(t, store.records, "records were stored for a failed download")
	assert.Len(t, store.pending, 1, "the activity no longer awaits its records")
}

// A slot nobody has connected has nothing to read, which is not a failure.
func TestRecordReportsATargetThatIsNotAuthorized(t *testing.T) {
	store := newFakeStore()
	store.authorization = "not_authorized"
	source := newFakeSource(t)

	result := newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7)

	assert.Equal(t, NotReady, result.Outcome)
	assert.Zero(t, source.activityCalls)
}

// Every way a record can fail, and the one word it reports it as: a state write
// that refuses is never reported as the account refusing, or the other way round.
func TestRecordReportsWhatFailed(t *testing.T) {
	tests := map[string]struct {
		arrange func(*fakeSource, *fakeStore)
		want    Failure
	}{
		"authorization unreadable": {
			arrange: func(_ *fakeSource, store *fakeStore) { store.authorizationErr = errUpstream },
			want:    FailureState,
		},
		"stored activities unreadable": {
			arrange: func(_ *fakeSource, store *fakeStore) { store.knownErr = errUpstream },
			want:    FailureState,
		},
		"the activity cannot be stored": {
			arrange: func(_ *fakeSource, store *fakeStore) { store.storeErr = errUpstream },
			want:    FailureState,
		},
		"the skip cannot be recorded": {
			arrange: func(source *fakeSource, store *fakeStore) {
				source.summaryErrs[7] = errUnreadable
				store.skipErr = errUpstream
			},
			want: FailureState,
		},
		"a skip for a workout the account refused cannot be recorded": {
			arrange: func(source *fakeSource, store *fakeStore) {
				source.activityErr = errUnreadable
				store.skipErr = errUpstream
			},
			want: FailureState,
		},
		"the pending activities cannot be read": {
			arrange: func(_ *fakeSource, store *fakeStore) { store.pendingErr = errUpstream },
			want:    FailureState,
		},
		"the workout could not be read": {
			arrange: func(source *fakeSource, _ *fakeStore) { source.activityErr = errUpstream },
			want:    FailureUpstream,
		},
		"the connection refused the workout": {
			arrange: func(source *fakeSource, _ *fakeStore) { source.activityErr = errRejected },
			want:    FailureRejected,
		},
		"the summary could not be read": {
			arrange: func(source *fakeSource, _ *fakeStore) { source.summaryErrs[7] = errUpstream },
			want:    FailureUpstream,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			source := newFakeSource(t)
			source.listings = []Listing{{ID: 7, Starts: at(7), TypeID: 15, LocationID: 1}}
			test.arrange(source, store)

			result := newTestPoller(t, source, store).Record(t.Context(), "rider-a", 7)

			assert.Equal(t, Failed, result.Outcome, "outcome")
			assert.Equal(t, test.want, result.Failure, "failure")
		})
	}
}
