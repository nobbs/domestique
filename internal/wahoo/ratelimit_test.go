package wahoo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeQuotaStore is an in-memory stand-in for the state store, counting writes
// so a test can tell an ordinary poll from one that recorded something.
type fakeQuotaStore struct {
	loadErr error
	saveErr error
	quota   Quota
	saves   int
	found   bool
}

func (s *fakeQuotaStore) LoadQuota(context.Context) (Quota, bool, error) {
	if s.loadErr != nil {
		return Quota{}, false, s.loadErr
	}

	return s.quota, s.found, nil
}

func (s *fakeQuotaStore) SaveQuota(_ context.Context, quota *Quota) error {
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.quota, s.found = *quota, true

	return nil
}

// A process that found a window spent must not go straight back at Wahoo after
// a restart: the stored notBefore is waited out as if it had never stopped.
func TestClientWaitsOutAStoredQuotaAfterARestart(t *testing.T) {
	now := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	store := &fakeQuotaStore{found: true, quota: Quota{
		ObservedAt: now.Add(-time.Minute),
		ExpiresAt:  now.Add(4 * time.Minute),
		ResetAt:    now.Add(time.Minute),
		NotBefore:  now.Add(time.Minute),
	}}
	client, waited := quotaTestClient(t, store, &now)

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, time.Minute, *waited, "the stored quota was not waited out")
}

// The round trip a restart is: one client spends a window and records it, the
// next reads that record back and waits it out instead of asking Wahoo again.
func TestClientCarriesASpentQuotaThroughARestart(t *testing.T) {
	now := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	store := &fakeQuotaStore{}
	spending, _ := quotaTestClient(t, store, &now, header("0", "60"))

	_, err := spending.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err)
	require.Equal(t, 1, store.saves, "the spent window was not recorded")

	restarted, waited := quotaTestClient(t, store, &now, header("0", "60"))
	_, err = restarted.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, time.Minute, *waited, "a restart went straight back at Wahoo")
}

// The expiry is the whole of what makes a stored reading safe: past it the row
// is treated exactly as if nothing had ever been observed.
func TestClientDiscardsAStoredQuotaThatHasExpired(t *testing.T) {
	now := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	store := &fakeQuotaStore{found: true, quota: Quota{
		ObservedAt: now.Add(-10 * time.Minute),
		ExpiresAt:  now.Add(-time.Minute),
		Remaining:  0,
		NotBefore:  now.Add(5 * time.Minute),
	}}
	client, waited := quotaTestClient(t, store, &now)

	_, _, _, ok := client.RateLimit()
	assert.False(t, ok, "an expired row was reported as an observed quota")

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Zero(t, *waited, "an expired row held a request back")
}

// A row written under a tier with an hour-long window keeps that tier's expiry:
// restore compares the clock against the row alone and knows no window itself.
func TestClientHonoursAStoredExpiryFromAnotherTier(t *testing.T) {
	observedAt := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	stored := Quota{
		ObservedAt: observedAt,
		ExpiresAt:  observedAt.Add(time.Hour),
		ResetAt:    observedAt.Add(time.Hour),
		NotBefore:  observedAt.Add(time.Hour),
	}

	live := observedAt.Add(30 * time.Minute)
	client, _ := quotaTestClient(t, &fakeQuotaStore{found: true, quota: stored}, &live)
	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.ErrorIs(t, err, ErrRateLimited, "a row still within its own expiry was ignored")

	past := observedAt.Add(2 * time.Hour)
	expired, expiredWait := quotaTestClient(t, &fakeQuotaStore{found: true, quota: stored}, &past)
	_, err = expired.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Zero(t, *expiredWait, "a row past its own expiry held a request back")
}

// The row as a whole can still be live while the instant it names has passed.
// The quota is restored; the wait is simply not one there is anything left of.
func TestClientDoesNotWaitOnARestoredNotBeforeThatHasPassed(t *testing.T) {
	now := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	store := &fakeQuotaStore{found: true, quota: Quota{
		ObservedAt: now.Add(-3 * time.Minute),
		ExpiresAt:  now.Add(2 * time.Minute),
		ResetAt:    now.Add(-time.Minute),
		NotBefore:  now.Add(-time.Minute),
		Remaining:  0,
	}}
	client, waited := quotaTestClient(t, store, &now)

	remaining, resetAt, observedAt, ok := client.RateLimit()
	require.True(t, ok, "a live row should still report its quota")
	assert.Zero(t, remaining)
	assert.True(t, resetAt.IsZero(), "a reset already in the past must not be reported")
	assert.Equal(t, now.Add(-3*time.Minute), observedAt, "the original observation time")

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Zero(t, *waited, "a notBefore already passed was waited on")
}

// The stored expiry is Wahoo's shortest published window, unless the response
// named a later instant of its own.
func TestClientStoresTheExpiryOfTheObservationItWrote(t *testing.T) {
	now := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	store := &fakeQuotaStore{}
	client, _ := quotaTestClient(t, store, &now, header("120", "60"))

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, now.Add(defaultRateLimitReset), store.quota.ExpiresAt, "the shortest published window")

	spent := &fakeQuotaStore{}
	spentClient, _ := quotaTestClient(t, spent, &now, header("0", "600"))
	_, err = spentClient.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err)
	assert.Equal(t, now.Add(10*time.Minute), spent.quota.ExpiresAt, "a reset beyond that window")
	assert.Equal(t, now.Add(10*time.Minute), spent.quota.NotBefore, "the instant the next request waits for")
}

// A poll whose responses say the same thing has nothing new to record, so it
// does not add a write per request.
func TestClientDoesNotWriteAnUnchangedQuotaAgain(t *testing.T) {
	now := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	store := &fakeQuotaStore{}
	client, _ := quotaTestClient(t, store, &now, header("120", "60"))

	for range 3 {
		_, err := client.AuthenticatedUser(t.Context(), "access-token")
		require.NoError(t, err)
	}
	assert.Equal(t, 1, store.saves, "an unchanged quota was written again")
}

// A store that cannot be read or written is a degradation to the in-memory
// behaviour, never a failed request.
func TestClientKeepsWorkingWhenTheQuotaStoreFails(t *testing.T) {
	now := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	store := &fakeQuotaStore{loadErr: errors.New("state unreadable"), saveErr: errors.New("state unwritable")}
	client, _ := quotaTestClient(t, store, &now, header("120", "60"))

	_, err := client.AuthenticatedUser(t.Context(), "access-token")
	require.NoError(t, err, "a failing quota store failed the request")
	assert.Equal(t, 1, store.saves, "the observation was not offered to the store")

	remaining, _, _, ok := client.RateLimit()
	assert.True(t, ok, "the in-memory quota was lost with the store")
	assert.Equal(t, 120, remaining)
}

// header is one response's quota headers, as Wahoo publishes them.
func header(remaining, reset string) map[string]string {
	return map[string]string{"X-RateLimit-Remaining": remaining, "X-RateLimit-Reset": reset}
}

// quotaTestClient builds a client over a server answering with the given quota
// headers, its clock pinned to now and its wait recorded rather than slept.
func quotaTestClient(
	t *testing.T, store QuotaStore, now *time.Time, headers ...map[string]string,
) (*Client, *time.Duration) {
	t.Helper()
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		for _, values := range headers {
			for name, value := range values {
				writer.Header().Set(name, value)
			}
		}
		writeJSON(t, writer, map[string]int64{"id": 42})
	}))
	t.Cleanup(server.Close)

	client, err := New(&Options{
		APIBaseURL:   server.URL,
		OAuthBaseURL: server.URL,
		ClientID:     "client-id",
		RedirectURL:  "https://pi.example.ts.net/oauth/wahoo/callback",
		ClientSecret: []byte("test-client-secret"),
		Timeout:      time.Second,
		Transport:    server.Client().Transport,
		QuotaStore:   store,
	})
	require.NoError(t, err)

	var waited time.Duration
	client.now = func() time.Time { return *now }
	client.wait = func(_ context.Context, duration time.Duration) error {
		waited = duration
		*now = now.Add(duration)

		return nil
	}

	return client, &waited
}
