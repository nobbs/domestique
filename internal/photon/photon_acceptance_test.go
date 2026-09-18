//go:build photon_acceptance

// This file is not compiled into the normal suite. It contacts komoot's public
// Photon instance, because the fixture in client_test.go asserts the request
// this client sends and always agrees with it; only the geocoder can say
// whether it accepts that request and answers in the shape parsed here.
//
// Invoke it on its own:
//
//	go test -tags photon_acceptance ./internal/photon/ -run Acceptance -v
//
// It needs no credentials. The public instance asks only for fair use, and it
// is a valid geocoder for a deployment that accepts sending coordinates there;
// the coordinates below are a named public landmark, not anyone's route.
package photon_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/photon"
)

func TestAcceptancePublicInstanceNamesALandmark(t *testing.T) {
	client, err := photon.New(&photon.Options{BaseURL: "https://photon.komoot.io"})
	require.NoError(t, err)

	// Karlsruhe Palace, whose name and city Photon both hold.
	name, err := client.Reverse(t.Context(), 49.0134, 8.4044)

	require.NoError(t, err)
	assert.NotEmpty(t, name, "the geocoder names a landmark it holds")
	assert.Contains(t, name, "Karlsruhe")
}

func TestAcceptancePublicInstanceAnswersOpenWater(t *testing.T) {
	client, err := photon.New(&photon.Options{BaseURL: "https://photon.komoot.io"})
	require.NoError(t, err)

	// The middle of the Atlantic: an answer with no place is not a failure.
	_, err = client.Reverse(t.Context(), 30.0, -40.0)

	require.NoError(t, err)
}
