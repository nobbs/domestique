package readiness

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func TestReadinessMatchesOpenAPI(t *testing.T) {
	contents, err := os.ReadFile("../../api/openapi.yaml")
	require.NoError(t, err, "reading the authoritative contract")

	var document struct {
		Paths map[string]map[string]struct {
			Responses map[string]yaml.Node `yaml:"responses"`
		} `yaml:"paths"`
	}
	require.NoError(t, yaml.Unmarshal(contents, &document), "parsing the authoritative contract")
	responses := document.Paths["/readyz"][strings.ToLower(http.MethodGet)].Responses
	require.Contains(t, responses, "200")
	require.Contains(t, responses, "503")

	handler := newHandler(t, &fakeState{authorizations: map[string]string{"rider-a": "authorized"}}, "rider-a")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", http.NoBody))

	assert.Equal(t, http.StatusOK, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Contains(t, response.Header().Get("Content-Type"), "application/json")
	assert.JSONEq(t, `{"status":"ready"}`, response.Body.String())
}
