package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

type openAPIContract struct {
	Paths    map[string]map[string]openAPIOperation `yaml:"paths"`
	Fallback struct {
		Status int `yaml:"status"`
	} `yaml:"x-domestique-unmatched-path"` //nolint:tagliatelle // OpenAPI extension names are fixed.
}

type openAPIOperation struct {
	Responses map[string]yaml.Node `yaml:"responses"`
}

func loadOpenAPIContract(t *testing.T) openAPIContract {
	t.Helper()
	contents, err := os.ReadFile("../../api/openapi.yaml")
	require.NoError(t, err, "reading the authoritative contract")

	var document openAPIContract
	require.NoError(t, yaml.Unmarshal(contents, &document), "parsing the authoritative contract")
	assert.Contains(t, string(contents), "openapi: 3.1.0", "OpenAPI version")
	require.NotEmpty(t, document.Paths, "documented paths")

	return document
}

// TestOpenAPIContractResponses keeps deterministic handler behavior coupled to
// the response codes and wire media types declared in api/openapi.yaml. The
// more focused handler tests exercise each safety branch; this table proves
// their shared registered surface still matches the source of truth.
func TestOpenAPIContractResponses(t *testing.T) {
	document := loadOpenAPIContract(t)
	state := &fakeState{
		summaries: []route.Summary{{
			Provider: route.ProviderVeloPlanner, RouteID: 12, StageOrder: 1,
			RouteName: "Contract route", PointCount: 2,
		}},
		coordinates: json.RawMessage(`[[8,49],[8.1,49.1]]`),
	}
	handler := newHandler(t, &fakeOAuth{}, state)

	tests := []struct {
		name, path, method, contentType, cache, location string
		wantStatus                                       int
		authenticated                                    bool
	}{
		{"health", "/healthz", http.MethodGet, "application/json", cacheAPI, "", http.StatusOK, false},
		{"status", "/v1/status", http.MethodGet, "application/json", cacheAPI, "", http.StatusOK, true},
		{"unauthorized", "/v1/status", http.MethodGet, "application/json", cacheAPI, "", http.StatusUnauthorized, false},
		{"geometry", "/v1/providers/{provider}/routes/{routeId}/stages/{stage}/geometry", http.MethodGet, "application/geo+json", cacheAPI, "", http.StatusOK, true},
		{"trigger", "/v1/sync", http.MethodPost, "application/json", cacheAPI, "", http.StatusAccepted, true},
		{"legacy redirect", "/v1/routes/{routeId}/stages/{stage}", http.MethodGet, "", cacheAPI, "/v1/providers/veloplanner/routes/12/stages/1", http.StatusPermanentRedirect, true},
		{"browser entry", "/", http.MethodGet, "text/html", cacheDocument, "", http.StatusOK, true},
		{"asset", "/assets/{asset}", http.MethodGet, "text/javascript", cacheImmutable, "", http.StatusOK, true},
		{"unmatched", "", http.MethodGet, "application/json", cacheAPI, "", document.Fallback.Status, true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.path != "" {
				operation, ok := document.Paths[test.path][strings.ToLower(test.method)]
				require.True(t, ok, "operation %s %s is documented", test.method, test.path)
				_, ok = operation.Responses[http.StatusText(test.wantStatus)]
				if !ok {
					_, ok = operation.Responses[statusCode(test.wantStatus)]
				}
				require.True(t, ok, "response %d is documented", test.wantStatus)
			} else {
				require.Equal(t, http.StatusNotFound, document.Fallback.Status, "unmatched-path fallback status")
			}

			target := test.path
			if target == "" {
				target = "/missing"
			}
			target = strings.NewReplacer("{provider}", "veloplanner", "{routeId}", "12", "{stage}", "1", "{asset}", "app.js").Replace(target)
			request := httptest.NewRequestWithContext(context.Background(), test.method, target, http.NoBody)
			if test.authenticated {
				request.Header.Set(assertionHeader, testAssertion)
				withBrowserOrigin(request)
			}

			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			assert.Equal(t, test.wantStatus, response.Code)
			assert.Equal(t, test.cache, response.Header().Get("Cache-Control"))
			if test.contentType != "" {
				assert.Contains(t, response.Header().Get("Content-Type"), test.contentType)
			}
			if test.location != "" {
				assert.Equal(t, test.location, response.Header().Get("Location"))
			}
			if response.Code >= http.StatusBadRequest {
				assert.Contains(t, response.Body.String(), `"error"`)
			}
		})
	}
}

func statusCode(status int) string { return strconv.Itoa(status) }
