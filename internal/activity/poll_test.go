package activity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"testing"
	"time"

	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	errUpstream   = errors.New("upstream failed")
	errUnreadable = errors.New("this activity is unreadable")
)

var errUnauthorized = errors.New("unauthorized")

func TestNewPollerNeedsItsCollaborators(t *testing.T) {
	_, err := NewPoller(nil, newFakeStore(), func() time.Time { return time.Unix(0, 0) })
	require.ErrorContains(t, err, "are required")
}

// A target nobody has connected yet is not a failure; there is nothing to read.
func TestPollReportsATargetThatIsNotAuthorized(t *testing.T) {
	store := newFakeStore()
	store.authorization = "not_authorized"
	source := newFakeSource(t)

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, NotReady, result.Outcome)
	assert.Empty(t, source.listed, "the account was read for an unauthorized target")
}

func TestPollReportsUnreadableState(t *testing.T) {
	store := newFakeStore()
	store.authorizationErr = errors.New("state failed")

	result := newTestPoller(t, newFakeSource(t), store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureState, result.Failure)
}

// A refused refresh is the one failure that changes stored state: the slot is
// marked for interactive OAuth rather than retried against a dead token.
func TestPollMarksReauthorizationWhenTheRefreshIsRefused(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.refreshErr = errUnauthorized

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureAuthorization, result.Failure)
	assert.True(t, store.markedForReauthorization, "the slot was not marked for reauthorization")
}

func TestPollReportsARefreshThatFailedUpstream(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.refreshErr = errUpstream

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, FailureUpstream, result.Failure)
	assert.False(t, store.markedForReauthorization, "an upstream failure marked the slot for reauthorization")
}

// The replacement refresh token is stored before the access token is used, so an
// interrupted poll never leaves the next one holding a spent token.
func TestPollReplacesTheRefreshToken(t *testing.T) {
	store := newFakeStore()

	require.Equal(t, Unchanged, newTestPoller(t, newFakeSource(t), store).Poll(t.Context(), "rider-a").Outcome)
	assert.Equal(t, "refresh-token-next", store.refreshToken)
}

func TestPollReportsAListingThatFailed(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}}
	source.listErr = errUpstream

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureUpstream, result.Failure)
	assert.Empty(t, store.stored, "a failed listing stored an activity")
}

// Nothing new is not an empty account: it is one whose every ride is stored.
func TestPollReportsAnAccountWithNothingNew(t *testing.T) {
	store := newFakeStore()
	store.known = []int64{1, 2}
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1}, {ID: 2}}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Unchanged, result.Outcome)
	assert.Zero(t, result.Stored)
	assert.Empty(t, source.summarized, "a stored activity was read again")
}

func TestPollStoresOnlyWhatIsNotStoredYet(t *testing.T) {
	store := newFakeStore()
	store.known = []int64{1}
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2), TypeID: 15, LocationID: 1}}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	require.Equal(t, Polled, result.Outcome)
	assert.Equal(t, 1, result.Stored)
	require.Len(t, store.stored, 1)
	assert.Equal(t, int64(2), store.stored[0].listing.ID)
	assert.Equal(t, 15, store.stored[0].listing.TypeID)
	assert.InDelta(t, 2, store.stored[0].summary.DistanceMetres, 1e-9)
	assert.Equal(t, pollNow(), store.stored[0].now)
}

// One poll spends at most MaxNewPerPoll of a shared daily request budget, and
// spends it on the oldest rides so a long history fills in chronologically.
func TestPollReadsTheOldestActivitiesUpToItsCap(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	for index := range MaxNewPerPoll + 5 {
		// Listed newest first, as an account lists them.
		source.listings = append(source.listings, Listing{ID: int64(index + 1), Starts: at(MaxNewPerPoll + 5 - index)})
	}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	require.Equal(t, Polled, result.Outcome)
	assert.Equal(t, MaxNewPerPoll, result.Stored)
	require.Len(t, source.summarized, MaxNewPerPoll)
	assert.Equal(t, int64(MaxNewPerPoll+5), source.summarized[0], "the oldest activity was not read first")
	assert.Equal(t, int64(6), source.summarized[MaxNewPerPoll-1], "the read did not stop at the oldest MaxNewPerPoll")
}

// A summary that fails stops the poll, but every activity read before it stays
// stored: the next poll continues rather than starting over.
func TestPollKeepsWhatItStoredBeforeASummaryFailed(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}, {ID: 3, Starts: at(3)}}
	source.summaryErrs = map[int64]error{2: errUpstream}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureUpstream, result.Failure)
	assert.Equal(t, 1, result.Stored)
	require.Len(t, store.stored, 1)
	assert.Equal(t, int64(1), store.stored[0].listing.ID)
	assert.False(t, store.markedForReauthorization, "a failed summary read marked the slot for reauthorization")
	assert.Equal(t, "refresh-token-next", store.refreshToken, "a failed summary read discarded the replacement refresh token")
}

// One activity whose own summary is refused is set aside, and every activity
// after it is still read: the account keeps filling in past it.
func TestPollSkipsAnActivityOnlyItsOwnSummaryRejects(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}, {ID: 3, Starts: at(3)}}
	source.summaryErrs = map[int64]error{2: fmt.Errorf("HTTP 404: %w", errUnreadable)}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Result{Outcome: Polled, Stored: 2, Skipped: 1, RecordsStored: 2}, result)
	assert.Equal(t, []int64{1, 3}, source.summarized)
	require.Len(t, store.skipped, 1)
	assert.Equal(t, recordedSkip{id: 2, observed: "HTTP 404: this activity is unreadable", now: pollNow()}, store.skipped[0])
	assert.False(t, store.markedForReauthorization, "a skipped activity marked the slot for reauthorization")
}

// A failure that belongs to the connection, the quota or the grant — or one the
// poll does not recognise — still stops the run and condemns no activity.
func TestPollStopsWithoutSkippingOnAFailureThatIsNotTheActivitysOwn(t *testing.T) {
	cases := map[string]struct {
		err     error
		failure Failure
	}{
		"upstream":     {err: errUpstream, failure: FailureUpstream},
		"unrecognised": {err: errors.New("something new"), failure: FailureUpstream},
		"unauthorized": {err: errUnauthorized, failure: FailureAuthorization},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			source := newFakeSource(t)
			source.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}}
			source.summaryErrs = map[int64]error{1: tc.err}

			result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

			assert.Equal(t, Result{Outcome: Failed, Failure: tc.failure}, result)
			assert.Empty(t, store.skipped, "a run-level failure was recorded as a skip")
			assert.Empty(t, source.summarized, "the poll went on past a run-level failure")
		})
	}
}

// A skipped activity is offered again only once its wait has passed, so a
// handful of unreadable rides cannot spend the request window every poll.
func TestPollRetriesASkipOnlyAfterItsBackoff(t *testing.T) {
	cases := map[string]struct {
		skip Skip
		want []int64
	}{
		"waiting":   {skip: Skip{ID: 2, Attempts: 1, LastAttempt: pollNow().Add(-23 * time.Hour)}, want: []int64{1}},
		"due":       {skip: Skip{ID: 2, Attempts: 1, LastAttempt: pollNow().Add(-24 * time.Hour)}, want: []int64{1, 2}},
		"backedOff": {skip: Skip{ID: 2, Attempts: 3, LastAttempt: pollNow().Add(-3 * 24 * time.Hour)}, want: []int64{1}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			store := newFakeStore()
			store.skips = []Skip{tc.skip}
			source := newFakeSource(t)
			source.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}}

			result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

			assert.Equal(t, Polled, result.Outcome)
			assert.Equal(t, tc.want, source.summarized)
		})
	}
}

// Nothing new and every skip still waiting is a quiet poll, not a failure.
func TestPollWithOnlyWaitingSkipsIsUnchanged(t *testing.T) {
	store := newFakeStore()
	store.skips = []Skip{{ID: 1, Attempts: 1, LastAttempt: pollNow()}}
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}}

	assert.Equal(t, Unchanged, newTestPoller(t, source, store).Poll(t.Context(), "rider-a").Outcome)
}

func TestRetryDueDoublesTheWaitUpToFourWeeks(t *testing.T) {
	waits := map[int]time.Duration{
		0: 24 * time.Hour, 1: 24 * time.Hour, 2: 48 * time.Hour, 3: 96 * time.Hour,
		5: 16 * 24 * time.Hour, 6: 28 * 24 * time.Hour, 40: 28 * 24 * time.Hour,
	}
	for attempts, wait := range waits {
		skip := Skip{Attempts: attempts, LastAttempt: pollNow()}
		assert.Falsef(t, retryDue(skip, pollNow().Add(wait-time.Second)), "attempt %d due before %s", attempts, wait)
		assert.Truef(t, retryDue(skip, pollNow().Add(wait)), "attempt %d not due at %s", attempts, wait)
	}
}

// Every read or write of local state that fails stops the poll as a state
// failure rather than as anything an operator would chase upstream.
func TestPollReportsStateItCouldNotReadOrWrite(t *testing.T) {
	failing := errors.New("state failed")
	cases := map[string]func(*fakeStore, *fakeSource){
		"known ids":         func(store *fakeStore, _ *fakeSource) { store.knownErr = failing },
		"refresh token":     func(store *fakeStore, _ *fakeSource) { store.refreshTokenErr = failing },
		"token replacement": func(store *fakeStore, _ *fakeSource) { store.replaceErr = failing },
		"reauthorization": func(store *fakeStore, source *fakeSource) {
			store.markErr = failing
			source.refreshErr = errUnauthorized
		},
		"skips": func(store *fakeStore, source *fakeSource) {
			store.skipsErr = failing
			source.listings = []Listing{{ID: 1}}
		},
		"skip": func(store *fakeStore, source *fakeSource) {
			store.skipErr = failing
			source.listings = []Listing{{ID: 1}}
			source.summaryErrs = map[int64]error{1: errUnreadable}
		},
		"activities awaiting records": func(store *fakeStore, _ *fakeSource) { store.pendingErr = failing },
		"records write": func(store *fakeStore, _ *fakeSource) {
			store.pending = []PendingActivity{pendingActivity(1)}
			store.recordsErr = failing
		},
		"unreadable mark": func(store *fakeStore, source *fakeSource) {
			store.pending = []PendingActivity{pendingActivity(1)}
			store.unreadableErr = failing
			source.fitFor["ride-1"] = []byte("not a FIT file")
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			store, source := newFakeStore(), newFakeSource(t)
			arrange(store, source)

			result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

			assert.Equal(t, Failed, result.Outcome, "outcome")
			assert.Equal(t, FailureState, result.Failure, "failure")
		})
	}
}

// Two rides that started in the same second still have one order, so a capped
// poll reads the same ones every time rather than a different half each run.
func TestPollOrdersActivitiesThatStartedTogetherByTheirID(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 9, Starts: at(1)}, {ID: 4, Starts: at(1)}}

	require.Equal(t, Polled, newTestPoller(t, source, store).Poll(t.Context(), "rider-a").Outcome)
	assert.Equal(t, []int64{4, 9}, source.summarized)
}

func TestPollReportsAStoreThatWillNotTakeAnActivity(t *testing.T) {
	store := newFakeStore()
	store.storeErr = errors.New("state failed")
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1}}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureState, result.Failure)
	assert.Zero(t, result.Stored)
}

// The FIT file of every activity whose samples are still absent is downloaded
// and decoded, and only those: a stored ride is never fetched again.
func TestPollFillsRecordsForActivitiesAwaitingThem(t *testing.T) {
	store := newFakeStore()
	store.pending = []PendingActivity{pendingActivity(1), pendingActivity(2)}
	source := newFakeSource(t)

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	require.Equal(t, Polled, result.Outcome)
	assert.Zero(t, result.Stored)
	assert.Equal(t, 2, result.RecordsStored)
	assert.Equal(t, []string{"ride-1", "ride-2"}, source.downloaded)
	require.Len(t, store.records[1].Records, 1)
	assert.InDelta(t, 200.0, store.records[1].Records[0].PowerWatts, 0)
	assert.Empty(t, store.pending)
}

// Records are as bounded as summaries are: a long history fills in over runs.
func TestPollFillsRecordsUpToItsCap(t *testing.T) {
	store := newFakeStore()
	for id := range int64(MaxRecordsPerPoll + 3) {
		store.pending = append(store.pending, pendingActivity(id+1))
	}
	source := newFakeSource(t)

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	require.Equal(t, Polled, result.Outcome)
	assert.Equal(t, MaxRecordsPerPoll, result.RecordsStored)
	assert.Equal(t, MaxRecordsPerPoll, store.recordLimit)
	assert.Len(t, store.pending, 3)
}

// A file that does not decode is that ride's own fault: it is recorded as
// unreadable so no later poll spends a download on it, and the run carries on.
func TestPollMarksAnUndecodableFITAndCarriesOn(t *testing.T) {
	store := newFakeStore()
	store.pending = []PendingActivity{pendingActivity(1), pendingActivity(2)}
	source := newFakeSource(t)
	source.fitFor["ride-1"] = []byte("not a FIT file")

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	require.Equal(t, Polled, result.Outcome)
	assert.Equal(t, 1, result.RecordsStored)
	assert.Equal(t, 1, result.RecordsUnreadable)
	assert.Equal(t, []int64{1}, store.unreadable)
	assert.Contains(t, store.records, int64(2), "the readable ride must still be stored")
}

// A summary that names no file is that ride's fault too, marked the same way
// as a file that does not decode rather than read as a provider outage.
func TestPollMarksASummaryWithoutAFileUnreadable(t *testing.T) {
	store := newFakeStore()
	store.pending = []PendingActivity{pendingActivity(1), pendingActivity(2)}
	source := newFakeSource(t)
	source.downloadErr = fmt.Errorf("wrapped: %w", ErrNoActivityFile)

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Polled, result.Outcome, "marking a file unreadable is a change")
	assert.Equal(t, FailureNone, result.Failure)
	assert.Equal(t, 2, result.RecordsUnreadable)
	assert.Equal(t, []int64{1, 2}, store.unreadable)
	assert.Empty(t, store.records)
}

// A download that failed for anything but this file's own sake — a rate limit
// or an outage — stops the phase and marks nothing, for the next poll to retry.
func TestPollStopsTheRecordsPhaseWhenADownloadFails(t *testing.T) {
	store := newFakeStore()
	store.pending = []PendingActivity{pendingActivity(1), pendingActivity(2)}
	source := newFakeSource(t)
	source.downloadErr = errUpstream

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureUpstream, result.Failure)
	assert.Zero(t, result.RecordsStored)
	assert.Empty(t, store.unreadable, "an outage condemned a ride as unreadable")
	assert.Empty(t, store.records)
}

// One run both stores what the account newly lists and fills that same ride's
// samples, so a fresh activity is complete after a single poll.
func TestPollReportsBothWhatItStoredAndWhatItRecorded(t *testing.T) {
	store := newFakeStore()
	store.pending = []PendingActivity{pendingActivity(7)}
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	require.Equal(t, Polled, result.Outcome)
	assert.Equal(t, 1, result.Stored)
	assert.Equal(t, 2, result.RecordsStored)
	assert.Contains(t, store.records, int64(1))
	assert.Contains(t, store.records, int64(7))
}

// pollNow is the fixed clock every poll in this file reads; no test waits on a
// wall clock.
// The point of the backlog: the account's whole list is read once, and the polls
// that drain it afterwards do not read it again.
func TestPollDoesNotReadTheWholeListAgainWhileDrainingTheBacklog(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	for id := 1; id <= MaxNewPerPoll+5; id++ {
		source.listings = append(source.listings, Listing{ID: int64(id), Starts: at(id)})
	}
	poller := newTestPoller(t, source, store)

	first := poller.Poll(t.Context(), "rider-a")
	require.Equal(t, Polled, first.Outcome)
	require.Equal(t, MaxNewPerPoll, first.Stored)
	require.Equal(t, 1, source.listed, "the first poll did not read the account")

	second := poller.Poll(t.Context(), "rider-a")

	assert.Equal(t, Polled, second.Outcome)
	assert.Equal(t, 5, second.Stored, "the backlog did not carry the rest of the account")
	assert.Equal(t, int64(MaxNewPerPoll+1), source.summarized[MaxNewPerPoll],
		"the backlog was not drained oldest first")
	assert.Equal(t, 1, source.listed, "the second poll read the whole list again")
	assert.Equal(t, 2, source.headed, "each poll costs exactly one listing request beyond its summaries")
}

// A steady-state poll pays for the head request and nothing else.
func TestPollReadsOnlyTheListingHeadWhenNothingChanged(t *testing.T) {
	store := newFakeStore()
	store.known = []int64{1, 2}
	store.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}}
	store.readAt = pollNow()
	source := newFakeSource(t)
	source.listings = store.listings

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Unchanged, result.Outcome)
	assert.Zero(t, source.listed, "an unchanged account was listed in full")
	assert.Equal(t, 1, source.headed)
}

// The kept listings are the account's, not what is left to read, so a ride the
// rider deleted after it was stored settles after one reading rather than
// leaving the store permanently holding more than the account lists.
func TestPollSettlesAfterAStoredActivityIsDeletedFromTheAccount(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}}
	poller := newTestPoller(t, source, store)
	require.Equal(t, 2, poller.Poll(t.Context(), "rider-a").Stored)
	require.Equal(t, 1, source.listed)

	source.listings = []Listing{{ID: 2, Starts: at(2)}}

	require.Equal(t, Unchanged, poller.Poll(t.Context(), "rider-a").Outcome)
	require.Equal(t, 2, source.listed, "the deletion was not taken up")

	assert.Equal(t, Unchanged, poller.Poll(t.Context(), "rider-a").Outcome)
	assert.Equal(t, 2, source.listed, "the account is read in full on every poll after a deletion")
}

// An account holding more activities than the store has accounted for is read
// in full, wherever in the list the extra ones sit.
func TestPollReadsTheWholeListWhenTheAccountHoldsMore(t *testing.T) {
	store := newFakeStore()
	store.known = []int64{1, 2}
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}}
	source.hasTotal, source.total = true, 3

	require.Equal(t, Unchanged, newTestPoller(t, source, store).Poll(t.Context(), "rider-a").Outcome)
	assert.Equal(t, 1, source.listed, "an account with more activities was not read in full")
}

// The count cannot see an addition the rider balanced by a deletion, and the
// first page only shows one the provider puts there. Neither is relied on to
// notice it forever: the reading is taken again once it has aged out.
func TestPollTakesTheReadingAgainOnceItHasAgedOut(t *testing.T) {
	store := newFakeStore()
	store.known = []int64{1, 2, 3}
	store.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}, {ID: 3, Starts: at(3)}}
	store.readAt = pollNow()
	source := newFakeSource(t)
	// The rider deleted 1 and uploaded 4 with an older start, which the account
	// lists behind its first page.
	source.listings = []Listing{{ID: 2, Starts: at(2)}, {ID: 3, Starts: at(3)}, {ID: 4, Starts: at(-500)}}
	source.head = []Listing{{ID: 2, Starts: at(2)}, {ID: 3, Starts: at(3)}}

	now := pollNow()
	poller, err := NewPoller(source, store, func() time.Time { return now })
	require.NoError(t, err, "NewPoller()")

	require.Equal(t, Unchanged, poller.Poll(t.Context(), "rider-a").Outcome)
	require.Zero(t, source.listed, "a reading still in date was taken again")

	now = pollNow().Add(MaxReadingAge)
	result := poller.Poll(t.Context(), "rider-a")

	assert.Equal(t, Polled, result.Outcome)
	assert.Equal(t, 1, source.listed, "the aged-out reading was not taken again")
	require.Len(t, store.stored, 1)
	assert.Equal(t, int64(4), store.stored[0].listing.ID)
}

// A ride uploaded with an old start time sits wherever the account puts it, so
// what triggers the re-read is the account holding one more activity, not where
// that activity appears.
func TestPollPicksUpABackdatedActivity(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 2, Starts: at(20)}}
	poller := newTestPoller(t, source, store)
	require.Equal(t, 1, poller.Poll(t.Context(), "rider-a").Stored)

	source.listings = append(source.listings, Listing{ID: 3, Starts: at(-500)})

	result := poller.Poll(t.Context(), "rider-a")

	require.Equal(t, Polled, result.Outcome)
	require.Len(t, store.stored, 2)
	assert.Equal(t, int64(3), store.stored[1].listing.ID)
}

// An addition the rider balanced by deleting another ride leaves the account's
// count unchanged, so the count alone would read as nothing new.
func TestPollPicksUpAnAdditionBalancedByADeletion(t *testing.T) {
	store := newFakeStore()
	store.known = []int64{1, 2}
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 2, Starts: at(2)}, {ID: 3, Starts: at(3)}}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	require.Equal(t, Polled, result.Outcome)
	require.Len(t, store.stored, 1)
	assert.Equal(t, int64(3), store.stored[0].listing.ID)
}

// The constraint the skip backoff puts on the backlog: a skipped activity stays
// pending, so its retry is still offered once its wait has passed even though
// no poll in between read the account's list again.
func TestPollReoffersASkippedActivityFromTheBacklog(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 7, Starts: at(1)}}
	source.summaryErrs[7] = fmt.Errorf("HTTP 404: %w", errUnreadable)
	poller := newTestPoller(t, source, store)
	require.Equal(t, 1, poller.Poll(t.Context(), "rider-a").Skipped)

	store.skips = []Skip{{ID: 7, Attempts: 1, LastAttempt: pollNow().Add(-23 * time.Hour)}}
	require.Equal(t, Unchanged, poller.Poll(t.Context(), "rider-a").Outcome, "a deferred skip was read early")

	store.skips = []Skip{{ID: 7, Attempts: 1, LastAttempt: pollNow().Add(-24 * time.Hour)}}
	delete(source.summaryErrs, 7)

	result := poller.Poll(t.Context(), "rider-a")

	assert.Equal(t, Polled, result.Outcome)
	require.Len(t, store.stored, 1)
	assert.Equal(t, int64(7), store.stored[0].listing.ID)
	assert.Equal(t, 1, source.listed, "the retry re-read the whole account")
}

// A poll that stops part way keeps what it stored, and the next one carries on
// from the backlog rather than from the account.
func TestPollResumesTheBacklogAfterAFailedRead(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}, {ID: 3, Starts: at(3)}}
	source.summaryErrs[2] = errUpstream
	poller := newTestPoller(t, source, store)

	first := poller.Poll(t.Context(), "rider-a")
	require.Equal(t, Failed, first.Outcome)
	require.Equal(t, 1, first.Stored)

	delete(source.summaryErrs, 2)
	second := poller.Poll(t.Context(), "rider-a")

	assert.Equal(t, 2, second.Stored)
	assert.Equal(t, 1, source.listed, "the resumed poll read the whole account again")
}

func TestPollReportsAnUnreadableBacklog(t *testing.T) {
	store := newFakeStore()
	store.listingsErr = errors.New("state failed")

	result := newTestPoller(t, newFakeSource(t), store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureState, result.Failure)
}

func TestPollReportsABacklogItCouldNotReplace(t *testing.T) {
	store := newFakeStore()
	store.replaceListingsErr = errors.New("state failed")
	source := newFakeSource(t)
	source.listings = []Listing{{ID: 1, Starts: at(1)}}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureState, result.Failure)
	assert.Empty(t, store.stored, "an unrecorded backlog was read anyway")
}

func TestPollReportsAListingHeadThatFailed(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource(t)
	source.headErr = errUpstream

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureUpstream, result.Failure)
	assert.Zero(t, source.listed, "a failed head request listed the account in full")
}

func pollNow() time.Time {
	return time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
}

func at(minute int) time.Time {
	return pollNow().Add(time.Duration(minute) * time.Minute)
}

func newTestPoller(t *testing.T, source Source, store Store) *Poller {
	t.Helper()
	poller, err := NewPoller(source, store, pollNow)
	require.NoError(t, err, "NewPoller()")

	return poller
}

type fakeSource struct {
	summaryErrs map[int64]error
	fitFor      map[string][]byte
	refreshErr  error
	listErr     error
	headErr     error
	downloadErr error
	fit         []byte
	listings    []Listing
	head        []Listing
	summarized  []int64
	downloaded  []string
	listed      int
	headed      int
	total       int
	hasTotal    bool
}

func newFakeSource(t *testing.T) *fakeSource {
	t.Helper()

	return &fakeSource{summaryErrs: map[int64]error{}, fitFor: map[string][]byte{}, fit: testFIT(t)}
}

// testFIT is one valid single-record activity file, encoded here rather than
// checked in so no recorded ride of anyone's becomes a fixture.
func testFIT(t *testing.T) []byte {
	t.Helper()
	file := &filedef.Activity{}
	file.FileId.SetType(typedef.FileActivity)
	file.Records = append(file.Records, mesgdef.NewRecord(nil).SetTimestamp(pollNow()).SetPower(200))

	return encode(t, file)
}

func (s *fakeSource) RefreshAccessToken(
	_ context.Context, _ string,
) (accessToken, replacementRefreshToken string, err error) {
	if s.refreshErr != nil {
		return "", "", s.refreshErr
	}

	return "access-token", "refresh-token-next", nil
}

// ActivityListingHead answers with the account's whole listing as its first
// page, which is what a hundred-workout page size means for these fixtures, and
// with the account's own count of activities.
func (s *fakeSource) ActivityListingHead(_ context.Context, _ string) (listings []Listing, total int, err error) {
	s.headed++
	if s.headErr != nil {
		return nil, 0, s.headErr
	}
	page := s.listings
	if s.head != nil {
		page = s.head
	}
	if s.hasTotal {
		return page, s.total, nil
	}

	return page, len(s.listings), nil
}

func (s *fakeSource) ListActivities(_ context.Context, _ string) ([]Listing, error) {
	s.listed++
	if s.listErr != nil {
		return nil, s.listErr
	}

	return s.listings, nil
}

func (s *fakeSource) ActivitySummary(_ context.Context, _ string, id int64) (Summary, error) {
	if err := s.summaryErrs[id]; err != nil {
		return Summary{}, err
	}
	s.summarized = append(s.summarized, id)

	return Summary{DistanceMetres: float64(id), Raw: []byte(`{}`)}, nil
}

func (s *fakeSource) DownloadActivityFIT(_ context.Context, summary Summary) ([]byte, error) {
	if s.downloadErr != nil {
		return nil, s.downloadErr
	}
	s.downloaded = append(s.downloaded, string(summary.Raw))
	if override, ok := s.fitFor[string(summary.Raw)]; ok {
		return override, nil
	}

	return s.fit, nil
}

func (s *fakeSource) IsUnauthorized(err error) bool {
	return errors.Is(err, errUnauthorized)
}

func (s *fakeSource) IsUnreadable(err error) bool {
	return errors.Is(err, errUnreadable)
}

type recordedSkip struct {
	now      time.Time
	observed string
	id       int64
}

type storedActivity struct {
	now     time.Time
	listing Listing
	summary Summary
}

type fakeStore struct {
	authorizationErr         error
	storeErr                 error
	knownErr                 error
	refreshTokenErr          error
	replaceErr               error
	markErr                  error
	skipsErr                 error
	skipErr                  error
	listingsErr              error
	replaceListingsErr       error
	pendingErr               error
	recordsErr               error
	unreadableErr            error
	records                  map[int64]FIT
	readAt                   time.Time
	authorization            string
	refreshToken             string
	known                    []int64
	listings                 []Listing
	skips                    []Skip
	stored                   []storedActivity
	skipped                  []recordedSkip
	pending                  []PendingActivity
	unreadable               []int64
	recordLimit              int
	markedForReauthorization bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		authorization: authorizedState, refreshToken: "refresh-token", records: map[int64]FIT{},
	}
}

// pendingActivity is one stored activity still awaiting its records; its raw
// summary doubles as the key the fake source serves a FIT file under.
func pendingActivity(id int64) PendingActivity {
	return PendingActivity{ID: id, Summary: Summary{Raw: fmt.Appendf(nil, "ride-%d", id)}}
}

func (s *fakeStore) TargetAuthorization(_ context.Context, _ string) (string, error) {
	return s.authorization, s.authorizationErr
}

func (s *fakeStore) RefreshToken(_ context.Context, _ string) (string, error) {
	return s.refreshToken, s.refreshTokenErr
}

func (s *fakeStore) ReplaceRefreshToken(_ context.Context, _, refreshToken string) error {
	if s.replaceErr != nil {
		return s.replaceErr
	}
	s.refreshToken = refreshToken

	return nil
}

func (s *fakeStore) MarkNeedsReauthorization(_ context.Context, _ string) error {
	s.markedForReauthorization = true

	return s.markErr
}

func (s *fakeStore) KnownActivityIDs(_ context.Context, _ string) ([]int64, error) {
	return s.known, s.knownErr
}

func (s *fakeStore) ActivityListings(_ context.Context, _ string) ([]Listing, time.Time, error) {
	return slices.Clone(s.listings), s.readAt, s.listingsErr
}

func (s *fakeStore) ReplaceActivityListings(
	_ context.Context, _ string, listings []Listing, now time.Time,
) error {
	if s.replaceListingsErr != nil {
		return s.replaceListingsErr
	}
	s.listings, s.readAt = slices.Clone(listings), now

	return nil
}

func (s *fakeStore) ActivitySkips(_ context.Context, _ string) ([]Skip, error) {
	return s.skips, s.skipsErr
}

func (s *fakeStore) RecordActivitySkip(_ context.Context, _ string, id int64, observed string, now time.Time) error {
	if s.skipErr != nil {
		return s.skipErr
	}
	s.skipped = append(s.skipped, recordedSkip{id: id, observed: observed, now: now})

	return nil
}

func (s *fakeStore) StoreActivity(
	_ context.Context, _ string, listing Listing, summary Summary, now time.Time,
) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.stored = append(s.stored, storedActivity{listing: listing, summary: summary, now: now})
	s.known = append(s.known, listing.ID)
	s.pending = append(s.pending, PendingActivity{ID: listing.ID, Summary: summary})

	return nil
}

func TestPollLogsWhyATargetWasMarkedForReauthorization(t *testing.T) {
	logged := captureLogs(t)
	store := newFakeStore()
	store.refreshToken = "secret-refresh-token"
	source := newFakeSource(t)
	source.refreshErr = fmt.Errorf("wahoo: token request rejected with HTTP 400: %w", errUnauthorized)

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	require.Equal(t, FailureAuthorization, result.Failure)
	line := logged.String()
	assert.Contains(t, line, "target=rider-a")
	assert.Contains(t, line, "HTTP 400")
	assert.NotContains(t, line, "secret-refresh-token")
}

func TestPollDoesNotLogAnAuthorizationLossForAnUpstreamFailure(t *testing.T) {
	logged := captureLogs(t)
	source := newFakeSource(t)
	source.listErr = errUpstream

	newTestPoller(t, source, newFakeStore()).Poll(t.Context(), "rider-a")

	assert.NotContains(t, logged.String(), "rejected the target authorization")
}

// captureLogs redirects the default logger for one test. Package-level state, so
// these tests must not run in parallel.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	buffer := &bytes.Buffer{}
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	return buffer
}

func (s *fakeStore) ActivitiesAwaitingRecords(_ context.Context, _ string, limit int) ([]PendingActivity, error) {
	s.recordLimit = limit
	if s.pendingErr != nil {
		return nil, s.pendingErr
	}

	// Cloned: the real store hands back rows, not a view a later write shifts.
	return slices.Clone(s.pending[:min(len(s.pending), limit)]), nil
}

func (s *fakeStore) StoreActivityRecords(_ context.Context, _ string, id int64, fit FIT) error {
	if s.recordsErr != nil {
		return s.recordsErr
	}
	s.records[id] = fit
	s.settled(id)

	return nil
}

func (s *fakeStore) MarkActivityUnreadable(_ context.Context, _ string, id int64) error {
	if s.unreadableErr != nil {
		return s.unreadableErr
	}
	s.unreadable = append(s.unreadable, id)
	s.settled(id)

	return nil
}

func (s *fakeStore) settled(id int64) {
	s.pending = slices.DeleteFunc(s.pending, func(activity PendingActivity) bool { return activity.ID == id })
}
