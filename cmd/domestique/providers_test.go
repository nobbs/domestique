package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/auth0"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/nobbs/domestique/internal/session"
	"github.com/nobbs/domestique/internal/wahoo"
)

// signInProvider is a thin forwarding adapter to *auth0.Client; these exercise
// that both methods forward rather than reimplement, against a client an
// unroutable domain guarantees never completes a real exchange.
func TestSignInProviderForwardsToTheAuth0Client(t *testing.T) {
	t.Parallel()

	client, err := auth0.New(&auth0.Options{
		Domain:       "127.0.0.1:1",
		ClientID:     "cid",
		ClientSecret: []byte("secret"),
		RedirectURL:  "https://app.example/callback",
	})
	require.NoError(t, err, "auth0.New()")
	provider := signInProvider{client: client}

	url, err := provider.AuthorizationURL(t.Context(), "state-1", "nonce-1", "verifier-1")
	require.NoError(t, err, "AuthorizationURL()")
	assert.Contains(t, url, "state=state-1", "AuthorizationURL() did not forward the state")

	_, err = provider.Exchange(t.Context(), "code", "verifier-1", "nonce-1")
	assert.Error(t, err, "Exchange() against an unroutable domain")
}

// The success path needs no client at all: exchangedIdentityFrom is exactly
// the mapping Exchange applies to whatever the client hands back, split out
// so a wrong or transposed field is caught here rather than only reachable
// through a live or fake token exchange.
func TestExchangedIdentityFromCopiesEveryField(t *testing.T) {
	t.Parallel()

	got := exchangedIdentityFrom(auth0.Identity{
		Subject: "github|123456",
		Email:   "rider@example.test",
		Name:    "Rider Example",
		Access:  true,
		Admin:   true,
	})

	assert.Equal(t, session.ExchangedIdentity{
		Subject: "github|123456",
		Email:   "rider@example.test",
		Name:    "Rider Example",
		Access:  true,
		Admin:   true,
	}, got)
}

// Wahoo's word for a ride stops at this adapter: what the activity package
// reads is the service's own vocabulary, with its own field names.
func TestActivityListingsNarrowWahooWorkouts(t *testing.T) {
	t.Parallel()

	starts := time.Date(2026, 4, 1, 6, 30, 0, 0, time.UTC)
	listings := activityListings([]wahoo.Workout{
		{ID: 42, WorkoutTypeID: 15, WorkoutTypeLocationID: 1, Starts: starts},
	})

	assert.Equal(t, []activity.Listing{{ID: 42, TypeID: 15, LocationID: 1, Starts: starts}}, listings)
}

// A summary the listing carried travels with its listing, so a poll can store
// it without asking for it again.
func TestActivityListingsCarryTheListedSummary(t *testing.T) {
	t.Parallel()

	listings := activityListings([]wahoo.Workout{
		{ID: 1, Summary: &wahoo.WorkoutSummary{Raw: []byte(`{"id":1}`), DistanceMetres: 10, ActiveSeconds: 20, TotalSeconds: 30, AscentMetres: 40}},
		{ID: 2},
	})

	require.Len(t, listings, 2)
	assert.Equal(t, &activity.Summary{Raw: []byte(`{"id":1}`), DistanceMetres: 10, MovingSeconds: 20, ElapsedSeconds: 30, AscentMetres: 40}, listings[0].Summary)
	assert.Nil(t, listings[1].Summary)
}

// A reading counts what the account holds, cycling or not: it is compared with
// the account's own total, which counts every sport. What this service records
// is a separate question, asked after that comparison.
func TestActivityListingsCountEverySport(t *testing.T) {
	t.Parallel()

	starts := time.Date(2026, 4, 1, 6, 30, 0, 0, time.UTC)
	listings := activityListings([]wahoo.Workout{
		{ID: 1, WorkoutTypeID: 1, Starts: starts},
		{ID: 2, WorkoutTypeID: wahoo.WorkoutTypeBikingRoad, WorkoutTypeLocationID: 1, Starts: starts},
	})

	assert.Len(t, listings, 2, "a run is still one of the account's workouts")
}

// This service records cycling; an indoor ride is cycling, a run is not.
func TestIsRecordableKeepsCyclingAlone(t *testing.T) {
	t.Parallel()

	provider := newWahooProvider(testSettings(t, testStore(t, t.TempDir())), nil, "https://domestique.example.test")

	assert.True(t, provider.IsRecordable(activity.Listing{TypeID: wahoo.WorkoutTypeBikingRoad}))
	assert.True(t, provider.IsRecordable(activity.Listing{TypeID: wahoo.WorkoutTypeBikingIndoorTrainer}),
		"an indoor ride is still cycling")
	assert.False(t, provider.IsRecordable(activity.Listing{TypeID: 1}))
	assert.False(t, provider.IsRecordable(activity.Listing{TypeID: 3}))
}

// A stored summary is what a download is addressed by, and a service with no
// Wahoo application configured downloads nothing at all.
func TestDownloadActivityFITRefusesWhatItCannotAddress(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	current := testSettings(t, store)
	provider := newWahooProvider(current, store, "https://domestique.example.test")

	_, err := provider.DownloadActivityFIT(t.Context(), activity.Summary{Raw: []byte(`{}`)})
	require.Error(t, err, "an unconfigured service downloaded a file")

	configureWahoo(t, current)
	_, err = provider.DownloadActivityFIT(t.Context(), activity.Summary{Raw: []byte(`{}`)})
	require.ErrorContains(t, err, "does not name a file")

	// Anywhere but Wahoo's own CDN is refused by the client, not followed.
	_, err = provider.DownloadActivityFIT(t.Context(),
		activity.Summary{Raw: []byte(`{"file":{"url":"https://elsewhere.example.test/a.fit"}}`)})
	require.ErrorContains(t, err, "downloading a Wahoo workout file")
}

// Where the FIT file's URL sits inside a summary is Wahoo's shape, so it is
// read here rather than anywhere the activity package can see.
func TestSummaryFileURLReadsWahoosOwnShape(t *testing.T) {
	t.Parallel()

	fileURL, err := summaryFileURL([]byte(`{"id":42,"file":{"url":"https://cdn.wahooligan.com/a.fit"}}`))
	require.NoError(t, err)
	assert.Equal(t, "https://cdn.wahooligan.com/a.fit", fileURL)

	_, err = summaryFileURL([]byte(`{"id":42}`))
	require.ErrorContains(t, err, "does not name a file")

	_, err = summaryFileURL([]byte("not json"))
	require.ErrorContains(t, err, "could not be read")
}

func TestActivitySummaryOfCopiesEveryTotal(t *testing.T) {
	t.Parallel()

	summary := activitySummaryOf(wahoo.WorkoutSummary{
		Raw:            []byte(`{"id":42}`),
		DistanceMetres: 1234.5,
		ActiveSeconds:  3600,
		TotalSeconds:   3900,
		AscentMetres:   120.25,
	})

	assert.Equal(t, activity.Summary{
		Raw:            []byte(`{"id":42}`),
		DistanceMetres: 1234.5,
		MovingSeconds:  3600,
		ElapsedSeconds: 3900,
		AscentMetres:   120.25,
	}, summary)
}

// wahooQuotaStore is the one place the Wahoo client and the state store meet,
// so what it must not lose is every field of a reading, in both directions.
func TestWahooQuotaStoreRoundTripsAReading(t *testing.T) {
	t.Parallel()

	quotas := wahooQuotaStore{store: testStore(t, t.TempDir())}
	observedAt := time.Date(2026, time.September, 6, 7, 0, 0, 0, time.UTC)
	quota := wahoo.Quota{
		ObservedAt: observedAt,
		ExpiresAt:  observedAt.Add(5 * time.Minute),
		ResetAt:    observedAt.Add(2 * time.Minute),
		NotBefore:  observedAt.Add(2 * time.Minute),
		Remaining:  17,
	}

	_, found, err := quotas.LoadQuota(t.Context())
	require.NoError(t, err, "LoadQuota()")
	require.False(t, found, "a quota was reported before one was stored")

	require.NoError(t, quotas.SaveQuota(t.Context(), &quota), "SaveQuota()")
	loaded, found, err := quotas.LoadQuota(t.Context())
	require.NoError(t, err, "LoadQuota()")
	require.True(t, found, "the stored quota is not readable")
	assert.Equal(t, quota, loaded)
}

func TestWahooQuotaStoreReportsAnUnusableStore(t *testing.T) {
	t.Parallel()

	store := testStore(t, t.TempDir())
	quotas := wahooQuotaStore{store: store}
	require.NoError(t, store.Close(), "Close()")

	_, _, err := quotas.LoadQuota(t.Context())
	require.Error(t, err, "LoadQuota() on a closed store")
	require.Error(t, quotas.SaveQuota(t.Context(), &wahoo.Quota{
		ObservedAt: time.Now().UTC(), ExpiresAt: time.Now().UTC().Add(time.Minute),
	}), "SaveQuota() on a closed store")
}

// The webhook verifier is the whole of what the HTTP surface may learn about
// the token: it never sees the value, and an unconfigured token admits nobody
// rather than admitting everybody with an empty one.
func TestWebhookTokensVerifyOnlyTheConfiguredToken(t *testing.T) {
	t.Parallel()

	settings := testSettings(t, testStore(t, t.TempDir()))
	verifier := webhookTokens{settings: settings}

	for name, presented := range map[string]string{"a guess": "a-token", "nothing": ""} {
		t.Run("unconfigured, "+name, func(t *testing.T) {
			verified, err := verifier.VerifyWahooWebhookToken(t.Context(), presented)
			require.NoError(t, err, "VerifyWahooWebhookToken()")
			assert.False(t, verified, "an unconfigured token admitted a caller")
		})
	}

	require.NoError(t, settings.SetSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretWahooWebhookToken: runtimeconfig.NewSecret([]byte("the-configured-token")),
	}), "SetSecrets()")

	for name, expected := range map[string]struct {
		presented string
		verified  bool
	}{
		"the configured token": {presented: "the-configured-token", verified: true},
		"a prefix of it":       {presented: "the-configured", verified: false},
		"a longer value":       {presented: "the-configured-token-and-more", verified: false},
		"nothing at all":       {presented: "", verified: false},
	} {
		t.Run(name, func(t *testing.T) {
			verified, err := verifier.VerifyWahooWebhookToken(t.Context(), expected.presented)
			require.NoError(t, err, "VerifyWahooWebhookToken()")
			assert.Equal(t, expected.verified, verified)
		})
	}
}
