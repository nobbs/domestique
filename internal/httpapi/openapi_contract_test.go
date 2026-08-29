package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/nobbs/domestique/internal/route"
	validator "github.com/pb33f/libopenapi-validator"
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
	Responses  map[string]yaml.Node `yaml:"responses"`
	Tags       []string             `yaml:"tags"`
	Parameters []struct {
		Ref string `yaml:"$ref"`
	} `yaml:"parameters"`
	Security []map[string][]string `yaml:"security"`
}

// Every contract operation must have an explicit native route. This replaces
// the generated ServerInterface compile-time check without repeating the
// contract as a second route table.
func TestEveryContractOperationIsRegistered(t *testing.T) {
	document := loadOpenAPIContract(t)
	handler := newTestHandler(t)
	pathValues := strings.NewReplacer(
		"{provider}", "veloplanner", "{routeId}", "12", "{stage}", "1",
		"{target}", "rider-a", "{asset}", "app.js",
	)

	for path, operations := range document.Paths {
		for method, operation := range operations {
			if slices.Contains(operation.Tags, readinessTag) {
				continue
			}
			t.Run(strings.ToUpper(method)+" "+path, func(t *testing.T) {
				request := httptest.NewRequestWithContext(
					t.Context(), strings.ToUpper(method), pathValues.Replace(path), http.NoBody,
				)
				_, pattern := handler.mux.Handler(request)
				assert.NotEqual(t, "/", pattern, "the contract operation is not registered")
			})
		}
	}
}

// declaresOrigin reports whether this operation requires the browser origin,
// which is how the contract names an operation as state-changing.
func (operation *openAPIOperation) declaresOrigin() bool {
	for _, requirement := range operation.Security {
		if _, required := requirement[browserOriginScheme]; required {
			return true
		}
	}

	return false
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
			Provider: route.ProviderVeloPlanner, SourceRouteID: 12, StageOrder: 1,
			SourceRouteName: "Contract route", PointCount: 2,
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
		{"geometry", "/v1/providers/{provider}/sourceRoutes/{sourceRouteId}/routes/{stageOrder}/geometry", http.MethodGet, "application/geo+json", cacheAPI, "", http.StatusOK, true},
		{"trigger", "/v1/sync", http.MethodPost, "application/json", cacheAPI, "", http.StatusAccepted, true},
		{"legacy redirect", "/v1/routes/{routeId}/stages/{stage}", http.MethodGet, "", cacheAPI, "/v1/providers/veloplanner/sourceRoutes/12/routes/1", http.StatusPermanentRedirect, true},
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
			target = strings.NewReplacer(
				"{provider}", "veloplanner", "{sourceRouteId}", "12", "{stageOrder}", "1",
				"{routeId}", "12", "{stage}", "1", "{asset}", "app.js",
			).Replace(target)
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

// Every operation the contract marks as state-changing is refused without the
// browser's origin, and no other operation is refused for wanting one. The
// document is now the only place that distinction is written down — the
// validator reads the security requirement rather than matching paths — so this
// walks the document and proves the served behaviour matches it operation by
// operation.
//
// Each is also reached with a reserved character percent-encoded inside a path
// parameter. A target identifier is operator-configured free text, so "a%2Fb"
// addresses a configured slot rather than being malformed, and a guard that
// read the decoded path would see a different operation than the one the
// request actually reaches.
func TestEveryStateChangingOperationRequiresTheBrowserOrigin(t *testing.T) {
	document := loadOpenAPIContract(t)
	handler := newTestHandler(t)
	plain := strings.NewReplacer(
		"{provider}", "veloplanner", "{sourceRouteId}", "12", "{stageOrder}", "1",
		"{routeId}", "12", "{stage}", "1",
		"{target}", "rider-a", "{asset}", "app.js",
	)
	escaped := strings.NewReplacer(
		"{provider}", "velo%2Fplanner", "{sourceRouteId}", "12", "{stageOrder}", "1",
		"{routeId}", "12", "{stage}", "1",
		"{target}", "a%2Fb", "{asset}", "app%2Ejs",
	)

	for path, operations := range document.Paths {
		for method, operation := range operations {
			for name, replacer := range map[string]*strings.Replacer{"plain": plain, "escaped": escaped} {
				t.Run(strings.ToUpper(method)+" "+path+" ("+name+")", func(t *testing.T) {
					request := httptest.NewRequestWithContext(
						t.Context(), strings.ToUpper(method), replacer.Replace(path), http.NoBody,
					)
					request.Header.Set(assertionHeader, testAssertion)

					response := httptest.NewRecorder()
					handler.ServeHTTP(response, request)

					if operation.declaresOrigin() {
						assert.Equal(t, http.StatusForbidden, response.Code,
							"a documented state-changing operation served without an origin")

						return
					}
					assert.NotEqual(t, http.StatusForbidden, response.Code,
						"an operation the contract does not gate on provenance was refused for it")
				})
			}
		}
	}
}

// The served listener answers within its own contract, checked the other way
// round: the request validator holds callers to the document, and this holds
// the handlers to it.
//
// It is a test rather than a middleware on purpose. Validating a response costs
// decoding every body this service serves, geometry included, on a path whose
// whole point is that geometry is never decoded. The class of defect it catches
// is one nothing else does: a stored value that is outside the enum the
// document declares reaches the wire silently, and is found in review months
// later, if at all.
func TestServedResponsesSatisfyTheContract(t *testing.T) {
	spec, err := servedSpec()
	require.NoError(t, err, "the source contract")
	contractValidator := validator.NewValidatorFromV3Model(spec)
	t.Cleanup(contractValidator.Release)

	// The history fixture is reused so the run page carries real rows: a page
	// of nothing would satisfy any schema.
	state := historyStateFixture()
	state.summaries = []route.Summary{{
		Provider: route.ProviderVeloPlanner, SourceRouteID: 12, StageOrder: 1,
		SourceRouteName: "Contract route", PointCount: 2,
	}}
	state.coordinates = json.RawMessage(`[[8,49],[8.1,49.1]]`)
	handler := newHandler(t, &fakeOAuth{}, state)

	for _, target := range []string{
		"/healthz",
		"/v1/status",
		"/v1/routes",
		"/v1/sync/runs",
		"/v1/webui/config",
		"/v1/settings",
		"/v1/providers/veloplanner/sourceRoutes/12/routes/1",
		"/v1/providers/veloplanner/sourceRoutes/12/routes/1/geometry",
	} {
		t.Run(target, func(t *testing.T) {
			request := authenticatedRequest(http.MethodGet, target)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			require.Equal(t, http.StatusOK, response.Code, response.Body.String())

			result := response.Result()
			defer func() { require.NoError(t, result.Body.Close(), "closing the recorded body") }()

			valid, validationErrors := contractValidator.ValidateHttpResponse(request, result)
			require.True(t, valid, "the response does not satisfy the contract: %v", validationErrors)
		})
	}
}
