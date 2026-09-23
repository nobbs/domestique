//go:build wahoodevice_acceptance

package wahoodevice_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/wahoodevice"
)

// TestWahooDeviceAcceptance signs in to a real Wahoo account the way an ELEMNT
// does and lists its routes. It says whether the undocumented sign-in and
// listing still answer; it writes nothing and prints no route or token.
func TestWahooDeviceAcceptance(t *testing.T) {
	email := os.Getenv("DOMESTIQUE_WAHOO_DEVICE_EMAIL")
	password := os.Getenv("DOMESTIQUE_WAHOO_DEVICE_PASSWORD")
	if email == "" || password == "" {
		t.Skip("DOMESTIQUE_WAHOO_DEVICE_EMAIL and DOMESTIQUE_WAHOO_DEVICE_PASSWORD are required for the Wahoo device acceptance check")
	}

	client, err := wahoodevice.New(&wahoodevice.Options{})
	require.NoError(t, err, "constructing the client")

	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	token, err := client.SignIn(ctx, []byte(email), []byte(password))
	require.NoError(t, err, "the device sign-in refused this request or these credentials")

	_, err = client.Routes(ctx, token)
	require.NoError(t, err, "listing routes with the device session")
}
