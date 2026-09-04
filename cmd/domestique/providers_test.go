package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/auth0"
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
