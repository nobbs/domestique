//go:build zwift_acceptance

package zwift_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/zwift"
)

// TestZwiftAcceptance signs in to a real Zwift account and reads one activity
// end to end. It exists to say, on the operator's own account and before
// deployment, whether the public client id and token endpoint this package
// relies on still work — the facts a unit test cannot verify. It asserts
// facts only: no ride name, coordinate or id is ever printed.
func TestZwiftAcceptance(t *testing.T) {
	email := os.Getenv("DOMESTIQUE_ZWIFT_EMAIL")
	password := os.Getenv("DOMESTIQUE_ZWIFT_PASSWORD")
	if email == "" || password == "" {
		t.Skip("DOMESTIQUE_ZWIFT_EMAIL and DOMESTIQUE_ZWIFT_PASSWORD are required for the Zwift acceptance check")
	}

	client, err := zwift.New(&zwift.Options{Timeout: 30 * time.Second})
	require.NoError(t, err, "constructing the client")

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	session, err := client.Session(ctx, []byte(email), []byte(password))
	require.NoError(t, err, "the token endpoint rejected this client id or these credentials")
	assert.NotEmpty(t, session.AccessToken, "token response carried no access token")
	assert.NotEmpty(t, session.RefreshToken, "token response carried no refresh token")

	playerID, err := client.PlayerID(ctx, session)
	require.NoError(t, err, "reading the signed-in rider's own player id")
	require.Positive(t, playerID)

	listing, err := client.Activities(ctx, session, playerID, 0, 1)
	require.NoError(t, err, "listing the newest activity")
	require.NotEmpty(t, listing, "this account has no recorded activity to verify against")

	one, err := client.Activity(ctx, session, listing[0].ID)
	require.NoError(t, err, "reading the newest activity")
	require.NotEmpty(t, one.FullDataURL, "the activity carried no downloadable file")

	raw, err := client.DownloadFIT(ctx, session, one.FullDataURL)
	require.NoError(t, err, "downloading the FIT file needed the bearer token")
	require.NotEmpty(t, raw)

	decoded, err := activity.DecodeFIT(raw)
	require.NoError(t, err, "decoding the downloaded FIT file")
	require.NotEmpty(t, decoded.Records, "the decoded file carried no records")

	hasPower := false
	for _, record := range decoded.Records {
		if record.HasPower {
			hasPower = true

			break
		}
	}
	assert.True(t, hasPower, "an indoor Zwift ride is expected to carry a power channel")

	t.Logf("zwift acceptance: signed in, read %d record(s), power channel present: %v", len(decoded.Records), hasPower)
}
