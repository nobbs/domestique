package activity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var errUpstream = errors.New("upstream failed")

var errUnauthorized = errors.New("unauthorized")

func TestNewPollerNeedsItsCollaborators(t *testing.T) {
	_, err := NewPoller(nil, newFakeStore(), func() time.Time { return time.Unix(0, 0) })
	require.ErrorContains(t, err, "are required")
}

// A target nobody has connected yet is not a failure; there is nothing to read.
func TestPollReportsATargetThatIsNotAuthorized(t *testing.T) {
	store := newFakeStore()
	store.authorization = "not_authorized"
	source := newFakeSource()

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, NotReady, result.Outcome)
	assert.Empty(t, source.listed, "the account was read for an unauthorized target")
}

func TestPollReportsUnreadableState(t *testing.T) {
	store := newFakeStore()
	store.authorizationErr = errors.New("state failed")

	result := newTestPoller(t, newFakeSource(), store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureState, result.Failure)
}

// A refused refresh is the one failure that changes stored state: the slot is
// marked for interactive OAuth rather than retried against a dead token.
func TestPollMarksReauthorizationWhenTheRefreshIsRefused(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource()
	source.refreshErr = errUnauthorized

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureAuthorization, result.Failure)
	assert.True(t, store.markedForReauthorization, "the slot was not marked for reauthorization")
}

func TestPollReportsARefreshThatFailedUpstream(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource()
	source.refreshErr = errUpstream

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, FailureUpstream, result.Failure)
	assert.False(t, store.markedForReauthorization, "an upstream failure marked the slot for reauthorization")
}

// The replacement refresh token is stored before the access token is used, so an
// interrupted poll never leaves the next one holding a spent token.
func TestPollReplacesTheRefreshToken(t *testing.T) {
	store := newFakeStore()

	require.Equal(t, Unchanged, newTestPoller(t, newFakeSource(), store).Poll(t.Context(), "rider-a").Outcome)
	assert.Equal(t, "refresh-token-next", store.refreshToken)
}

func TestPollReportsAListingThatFailed(t *testing.T) {
	store := newFakeStore()
	source := newFakeSource()
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
	source := newFakeSource()
	source.listings = []Listing{{ID: 1}, {ID: 2}}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Unchanged, result.Outcome)
	assert.Zero(t, result.Stored)
	assert.Empty(t, source.summarized, "a stored activity was read again")
}

func TestPollStoresOnlyWhatIsNotStoredYet(t *testing.T) {
	store := newFakeStore()
	store.known = []int64{1}
	source := newFakeSource()
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
	source := newFakeSource()
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
	source := newFakeSource()
	source.listings = []Listing{{ID: 1, Starts: at(1)}, {ID: 2, Starts: at(2)}, {ID: 3, Starts: at(3)}}
	source.summaryErrs = map[int64]error{2: errUpstream}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureUpstream, result.Failure)
	assert.Equal(t, 1, result.Stored)
	require.Len(t, store.stored, 1)
	assert.Equal(t, int64(1), store.stored[0].listing.ID)
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
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			store, source := newFakeStore(), newFakeSource()
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
	source := newFakeSource()
	source.listings = []Listing{{ID: 9, Starts: at(1)}, {ID: 4, Starts: at(1)}}

	require.Equal(t, Polled, newTestPoller(t, source, store).Poll(t.Context(), "rider-a").Outcome)
	assert.Equal(t, []int64{4, 9}, source.summarized)
}

func TestPollReportsAStoreThatWillNotTakeAnActivity(t *testing.T) {
	store := newFakeStore()
	store.storeErr = errors.New("state failed")
	source := newFakeSource()
	source.listings = []Listing{{ID: 1}}

	result := newTestPoller(t, source, store).Poll(t.Context(), "rider-a")

	assert.Equal(t, Failed, result.Outcome)
	assert.Equal(t, FailureState, result.Failure)
	assert.Zero(t, result.Stored)
}

// pollNow is the fixed clock every poll in this file reads; no test waits on a
// wall clock.
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
	refreshErr  error
	listErr     error
	listings    []Listing
	summarized  []int64
	listed      int
}

func newFakeSource() *fakeSource {
	return &fakeSource{summaryErrs: map[int64]error{}}
}

func (s *fakeSource) RefreshAccessToken(
	_ context.Context, _ string,
) (accessToken, replacementRefreshToken string, err error) {
	if s.refreshErr != nil {
		return "", "", s.refreshErr
	}

	return "access-token", "refresh-token-next", nil
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

func (s *fakeSource) IsUnauthorized(err error) bool {
	return errors.Is(err, errUnauthorized)
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
	authorization            string
	refreshToken             string
	known                    []int64
	stored                   []storedActivity
	markedForReauthorization bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{authorization: authorizedState, refreshToken: "refresh-token"}
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

func (s *fakeStore) StoreActivity(
	_ context.Context, _ string, listing Listing, summary Summary, now time.Time,
) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.stored = append(s.stored, storedActivity{listing: listing, summary: summary, now: now})

	return nil
}

func TestPollLogsWhyATargetWasMarkedForReauthorization(t *testing.T) {
	logged := captureLogs(t)
	store := newFakeStore()
	store.refreshToken = "secret-refresh-token"
	source := newFakeSource()
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
	source := newFakeSource()
	source.listErr = errUpstream

	newTestPoller(t, source, newFakeStore()).Poll(t.Context(), "rider-a")

	assert.NotContains(t, logged.String(), "reauthorization")
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
