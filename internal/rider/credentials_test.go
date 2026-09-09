package rider_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialIsSetOnlyWhenGivenBytes(t *testing.T) {
	t.Parallel()
	assert.False(t, rider.Credential{}.IsSet(), "the zero value")
	assert.False(t, rider.NewCredential(nil).IsSet())
	assert.True(t, rider.NewCredential([]byte("opensesame")).IsSet())
}

func TestCredentialHandsOutACopy(t *testing.T) {
	t.Parallel()
	credential := rider.NewCredential([]byte("opensesame"))
	credential.Bytes()[0] = 'X'

	assert.Equal(t, []byte("opensesame"), credential.Bytes())
}

func TestCredentialDoesNotRenderItsValue(t *testing.T) {
	t.Parallel()
	credential := rider.NewCredential([]byte("opensesame"))

	encoded, err := json.Marshal(struct {
		Value rider.Credential `json:"value"`
	}{Value: credential})
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "opensesame", "JSON")

	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		assert.NotContains(t, fmt.Sprintf(verb, credential), "opensesame", verb)
	}
}
