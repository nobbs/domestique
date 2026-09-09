package activity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeZwiftSource serves pages of listings and one FIT file, and classifies the
// errors a test set exactly as the real client does.
type fakeZwiftSource struct {
	signInErr    error
	listErr      error
	downloadErr  error
	signedInWith [2]string
	fit          []byte
	pages        [][]Listing
	// unrecordable is how many entries a page held that narrowing dropped.
	unrecordable map[int]int
	downloaded   []int64
}

func (s *fakeZwiftSource) SignIn(_ context.Context, email, password []byte) (ZwiftReader, error) {
	if s.signInErr != nil {
		return nil, s.signInErr
	}
	s.signedInWith = [2]string{string(email), string(password)}

	return s, nil
}

func (s *fakeZwiftSource) ListActivities(_ context.Context, start, limit int) ([]Listing, int, error) {
	if s.listErr != nil {
		return nil, 0, s.listErr
	}
	page := start / limit
	if page >= len(s.pages) {
		return nil, 0, nil
	}

	return slices.Clone(s.pages[page]), len(s.pages[page]) + s.unrecordable[page], nil
}

func (s *fakeZwiftSource) DownloadActivityFIT(_ context.Context, summary Summary) ([]byte, error) {
	s.downloaded = append(s.downloaded, zwiftSummaryID(summary))
	if s.downloadErr != nil {
		return nil, s.downloadErr
	}

	return s.fit, nil
}

func (s *fakeZwiftSource) IsUnauthorized(err error) bool { return errors.Is(err, errUnauthorized) }

func (s *fakeZwiftSource) IsUnreadable(err error) bool { return errors.Is(err, errUnreadable) }

// zwiftSummaryID reads back the id zwiftListing wrote into a summary document.
func zwiftSummaryID(summary Summary) int64 {
	var id int64
	for _, digit := range summary.Raw {
		id = id*10 + int64(digit-'0')
	}

	return id
}

type fakeZwiftStore struct {
	ownerErr       error
	credentialsErr error
	recordsErr     error
	pendingErr     error
	storeErr       error
	deleteErr      error
	records        map[int64]FIT
	password       string
	owner          string
	email          string
	unreadable     []int64
	pending        []PendingActivity
	deleted        []time.Time
	stored         []storedActivity
	known          []int64
	deleteCount    int
}

func newFakeZwiftStore() *fakeZwiftStore {
	return &fakeZwiftStore{
		owner: "rider-a", email: "rider@example.test", password: "hunter2",
		records: map[int64]FIT{},
	}
}

func (s *fakeZwiftStore) TargetOwner(_ context.Context, _ string) (string, error) {
	return s.owner, s.ownerErr
}

func (s *fakeZwiftStore) RiderZwiftCredentials(_ context.Context, _ string) (email, password []byte, err error) {
	if s.credentialsErr != nil {
		return nil, nil, s.credentialsErr
	}

	return []byte(s.email), []byte(s.password), nil
}

func (s *fakeZwiftStore) KnownActivityIDs(_ context.Context, _, _ string) ([]int64, error) {
	return slices.Clone(s.known), nil
}

func (s *fakeZwiftStore) StoreActivity(
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

func (s *fakeZwiftStore) DeleteTrainerCopy(
	_ context.Context, _ string, at time.Time, _ time.Duration, _ []int,
) (int, error) {
	if s.deleteErr != nil {
		return 0, s.deleteErr
	}
	s.deleted = append(s.deleted, at)

	return s.deleteCount, nil
}

func (s *fakeZwiftStore) ActivitiesAwaitingRecords(
	_ context.Context, _, provider string, _, _ int,
) ([]PendingActivity, error) {
	if s.pendingErr != nil {
		return nil, s.pendingErr
	}
	if provider != ProviderZwift {
		return nil, nil
	}

	return slices.Clone(s.pending), nil
}

//nolint:gocritic // value param: conforms to the store contract.
func (s *fakeZwiftStore) StoreActivityRecords(_ context.Context, _ string, id int64, fit FIT, _ int) error {
	if s.recordsErr != nil {
		return s.recordsErr
	}
	s.records[id] = fit
	s.settled(id)

	return nil
}

func (s *fakeZwiftStore) MarkActivityUnreadable(_ context.Context, _ string, id int64) error {
	s.unreadable = append(s.unreadable, id)
	s.settled(id)

	return nil
}

// settled drops a ride from what is still awaiting its records, as storing or
// marking it does in the real store.
func (s *fakeZwiftStore) settled(id int64) {
	s.pending = slices.DeleteFunc(s.pending, func(ride PendingActivity) bool { return ride.ID == id })
}

// zwiftListing is one listed ride as the composition root's mapping builds it:
// an indoor virtual type, carrying its own summary.
func zwiftListing(id int64, starts time.Time) Listing {
	summary := Summary{Raw: fmt.Appendf(nil, "%d", id), DistanceMetres: 30_000, MovingSeconds: 3_600}

	return Listing{ID: id, TypeID: 68, Starts: starts, Summary: &summary, Provider: ProviderZwift}
}

func newTestZwiftPoller(t *testing.T, source ZwiftSource, store ZwiftStore) *ZwiftPoller {
	t.Helper()
	poller, err := NewZwiftPoller(source, store, []int{68}, pollNow)
	require.NoError(t, err, "NewZwiftPoller()")

	return poller
}

func TestNewZwiftPollerNeedsItsCollaborators(t *testing.T) {
	_, err := NewZwiftPoller(nil, newFakeZwiftStore(), []int{68}, pollNow)
	require.ErrorContains(t, err, "are required")
}

// Most riders have entered no Zwift credentials; there is nothing to poll for
// them, and nothing to report as broken either.
func TestZwiftPollIsNotReadyWithoutCredentials(t *testing.T) {
	store := newFakeZwiftStore()
	store.password = ""
	source := &fakeZwiftSource{}

	result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, NotReady, result.Outcome)
	assert.Empty(t, source.signedInWith[0], "the account was read without credentials")
}

func TestZwiftPollIsNotReadyForAnUnownedSlot(t *testing.T) {
	store := newFakeZwiftStore()
	store.owner = ""

	assert.Equal(t, NotReady, newTestZwiftPoller(t, &fakeZwiftSource{}, store).Poll(t.Context(), "rider-a").Outcome)
}

// The first poll of a connected account stores each ride and fills its samples.
func TestZwiftPollStoresAndFillsNewRides(t *testing.T) {
	store := newFakeZwiftStore()
	source := &fakeZwiftSource{
		fit:   testFIT(t),
		pages: [][]Listing{{zwiftListing(1, at(0)), zwiftListing(2, at(90))}},
	}

	result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Polled, result.Outcome)
	assert.Equal(t, 2, result.Stored, "stored")
	assert.Equal(t, 2, result.RecordsStored, "records stored")
	assert.Equal(t, [2]string{"rider@example.test", "hunter2"}, source.signedInWith, "credentials")
	assert.Len(t, store.records, 2, "the samples of both rides")
	assert.Equal(t, ProviderZwift, store.stored[0].listing.Provider, "the stored provider")
}

// A second poll over the same account has nothing to add and nothing to fill.
func TestZwiftPollReportsAnAccountWithNothingNew(t *testing.T) {
	store := newFakeZwiftStore()
	store.known = []int64{1}
	source := &fakeZwiftSource{fit: testFIT(t), pages: [][]Listing{{zwiftListing(1, at(0))}}}

	result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Unchanged, result.Outcome)
	assert.Empty(t, store.stored, "a known ride was stored again")
	assert.Empty(t, source.downloaded, "a known ride's file was downloaded again")
}

// A page holding only rides already stored is where a poll stops: everything
// below it is older still.
func TestZwiftPollStopsAtTheFirstPageHoldingNothingNew(t *testing.T) {
	store := newFakeZwiftStore()
	store.known = []int64{1}
	source := &fakeZwiftSource{
		fit:   testFIT(t),
		pages: [][]Listing{{zwiftListing(1, at(0))}, {zwiftListing(2, at(-90))}},
	}

	result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Unchanged, result.Outcome)
	assert.Empty(t, store.stored, "the poll read past a page holding nothing new")
}

// A page holding nothing but runs narrows to no ride at all, and says nothing
// about the rides behind it.
func TestZwiftPollReadsPastAPageOfRuns(t *testing.T) {
	store := newFakeZwiftStore()
	source := &fakeZwiftSource{
		fit:          testFIT(t),
		pages:        [][]Listing{{}, {zwiftListing(2, at(-90))}},
		unrecordable: map[int]int{0: 3},
	}

	result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Polled, result.Outcome)
	require.Len(t, store.stored, 1, "the ride behind the page of runs")
	assert.Equal(t, int64(2), store.stored[0].listing.ID)
}

// A refused grant is the rider's password to re-enter; nothing is marked, since
// there is no authorization for them to be sent back to.
func TestZwiftPollReportsARefusedGrant(t *testing.T) {
	store := newFakeZwiftStore()
	source := &fakeZwiftSource{signInErr: errUnauthorized}

	result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureAuthorization, result.Failure)
	assert.Empty(t, store.stored, "a refused grant stored a ride")
}

func TestZwiftPollReportsAListingThatFailedUpstream(t *testing.T) {
	source := &fakeZwiftSource{listErr: errUpstream}

	result := newTestZwiftPoller(t, source, newFakeZwiftStore()).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureUpstream, result.Failure)
}

// A file that will not decode is the ride's own fault: it is marked, and the
// rides after it are still filled.
func TestZwiftPollMarksAnUndecodableFileAndCarriesOn(t *testing.T) {
	store := newFakeZwiftStore()
	source := &fakeZwiftSource{
		fit:   []byte("not a fit file"),
		pages: [][]Listing{{zwiftListing(1, at(0)), zwiftListing(2, at(90))}},
	}

	result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Polled, result.Outcome)
	assert.Equal(t, 2, result.RecordsUnreadable, "both files were unreadable")
	assert.Equal(t, []int64{1, 2}, store.unreadable, "the rides marked unreadable")
	assert.Equal(t, []int64{1, 2}, source.downloaded, "the second file was still asked for")
}

// A download refused for the connection's sake stops the poll: the next one
// retries it, and nothing is marked.
func TestZwiftPollStopsOnADownloadThatIsNotTheFilesOwnFault(t *testing.T) {
	store := newFakeZwiftStore()
	source := &fakeZwiftSource{
		fit:         testFIT(t),
		downloadErr: errUpstream,
		pages:       [][]Listing{{zwiftListing(1, at(0))}},
	}

	result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureUpstream, result.Failure)
	assert.Empty(t, store.unreadable, "an upstream failure marked a ride unreadable")
}

// The head unit records the same trainer ride, and the two are one ride: the
// Zwift file is kept for the power it carries, and the copy goes.
func TestZwiftPollRemovesTheTrainerCopyOfARideItStores(t *testing.T) {
	store := newFakeZwiftStore()
	store.deleteCount = 1
	source := &fakeZwiftSource{fit: testFIT(t), pages: [][]Listing{{zwiftListing(1, at(0))}}}

	result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

	require.Equal(t, Polled, result.Outcome)
	assert.Equal(t, []time.Time{at(0)}, store.deleted, "the start the copy was looked for at")
	require.Len(t, store.stored, 1, "the Zwift ride was stored")
}

func TestZwiftPollReportsUnreadableState(t *testing.T) {
	store := newFakeZwiftStore()
	store.ownerErr = errUpstream

	result := newTestZwiftPoller(t, &fakeZwiftSource{}, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureState, result.Failure)
}

// Every failure to read or write stored state stops the poll under one
// category, so an operator sees a database problem rather than an upstream one.
func TestZwiftPollReportsEveryStateFailure(t *testing.T) {
	for name, brokenStore := range map[string]func(*fakeZwiftStore){
		"credentials": func(s *fakeZwiftStore) { s.credentialsErr = errUpstream },
		"delete":      func(s *fakeZwiftStore) { s.deleteErr = errUpstream },
		"store":       func(s *fakeZwiftStore) { s.storeErr = errUpstream },
		"pending":     func(s *fakeZwiftStore) { s.pendingErr = errUpstream },
		"records":     func(s *fakeZwiftStore) { s.recordsErr = errUpstream },
	} {
		t.Run(name, func(t *testing.T) {
			store := newFakeZwiftStore()
			brokenStore(store)
			source := &fakeZwiftSource{fit: testFIT(t), pages: [][]Listing{{zwiftListing(1, at(0))}}}

			result := newTestZwiftPoller(t, source, store).Poll(t.Context(), "rider-a")

			assert.Equal(t, Failed, result.Outcome, "outcome")
			assert.Equal(t, FailureState, result.Failure, "failure")
		})
	}
}

// The records budget bounds one run: the fill under way finishes, and the next
// poll picks up what is left rather than holding the resource indefinitely.
func TestZwiftPollStopsFillingWhenTheBudgetIsSpent(t *testing.T) {
	store := newFakeZwiftStore()
	source := &fakeZwiftSource{
		fit:   testFIT(t),
		pages: [][]Listing{{zwiftListing(1, at(0)), zwiftListing(2, at(90))}},
	}
	// The clock is past the budget from the first fill onwards, so the phase
	// starts none at all and the rides stay awaiting their samples.
	spent := 0
	poller, err := NewZwiftPoller(source, store, []int{68}, func() time.Time {
		spent++

		return pollNow().Add(time.Duration(spent) * RecordsBudgetPerPoll)
	})
	require.NoError(t, err, "NewZwiftPoller()")

	result := poller.Poll(t.Context(), "rider-a")

	assert.Equal(t, Polled, result.Outcome)
	assert.Equal(t, 2, result.Stored, "both rides were stored")
	assert.Zero(t, result.RecordsStored, "no fill was started past the budget")
	assert.Len(t, store.pending, 2, "both rides still await their samples")
}
