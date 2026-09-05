package main

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// relayWeatherGrid maps any non-2xx upstream status to 502; the demo stub
// must answer the same way a real provider failure would look to a client.
func TestSyntheticWeatherGridAnswersUnavailableAsBadGateway(t *testing.T) {
	t.Parallel()

	grid := syntheticWeatherGrid{}

	latest, err := grid.Latest(t.Context())
	require.NoError(t, err)
	defer func() { assert.NoError(t, latest.Body.Close()) }()
	assert.Equal(t, http.StatusBadGateway, latest.StatusCode)

	object, err := grid.Object(t.Context(), time.Now(), time.Now(), http.MethodGet, "")
	require.NoError(t, err)
	defer func() { assert.NoError(t, object.Body.Close()) }()
	assert.Equal(t, http.StatusBadGateway, object.StatusCode)
}
