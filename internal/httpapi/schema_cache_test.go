package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pb33f/libopenapi-validator/cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testSchemaCache is the one compiled contract every test handler shares:
// compiling it once per New() dominated the package under the race detector.
//
//nolint:gochecknoglobals // a per-binary cache is the point; New() reads it only
var testSchemaCache = cache.NewDefaultCache()

// A handler built without a shared cache compiles its own and still holds
// requests to the contract, which is how the composition root builds one.
func TestNewWithoutASharedSchemaCacheStillValidates(t *testing.T) {
	handler, err := New(
		&Options{
			Alerts:           &fakeAlerts{},
			Tasks:            &fakeTasks{},
			Settings:         settingsWith(testBasemaps()),
			Sessions:         newFakeSessions(),
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{}, &fakeWeatherGrid{},
	)
	require.NoError(t, err, "New()")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequest(http.MethodGet, "/v1/providers/veloplanner/sourceRoutes/abc/routes/1"))
	assert.Equal(t, http.StatusBadRequest, response.Code, "a malformed identifier is refused by the contract")
}
