package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/runtimeconfig"
)

// settingsBody is one complete submission, which is the only shape this
// endpoint accepts. It differs from settingsWith in every field, so a test that
// sends it and reads the settings back cannot pass on what was already there.
const settingsBody = `{
	"sync": {"allowEmptySourceDeletion": true, "staleAfterSeconds": 90000},
	"notifications": {
		"enabled": false,
		"successPolicy": "digest",
		"digestIntervalSeconds": 3600,
		"pushoverBaseUrl": "https://pushover.example.test"
	},
	"basemaps": [{
		"name": "Satellite",
		"styleUrl": "https://imagery.example.test/styles/hybrid",
		"darkCartography": true
	}],
	"surface": {"regions": ["europe/germany"], "rebuildIntervalSeconds": 604800}
}`

// settingsHandler builds a handler over settings a test can read back.
func settingsHandler(t *testing.T) (*Handler, *staticSettings) {
	t.Helper()
	settings := settingsWith(testBasemaps())
	handler, err := New(
		&Options{
			TargetIDs:        []string{"rider-a"},
			Settings:         settings,
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{},
	)
	require.NoError(t, err, "New()")

	return handler, settings
}

func settingsOf(t *testing.T, handler *Handler, request *http.Request) openapi.Settings {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	var view openapi.Settings
	require.NoError(t, json.NewDecoder(response.Body).Decode(&view), "decoding the settings")

	return view
}

func TestGetSettingsServesWhatIsInForce(t *testing.T) {
	handler, _ := settingsHandler(t)

	view := settingsOf(t, handler, authenticatedRequest(http.MethodGet, settingsPath))
	assert.False(t, view.Sync.AllowEmptySourceDeletion, "the deletion gate")
	assert.Equal(t, int((26 * time.Hour).Seconds()), view.Sync.StaleAfterSeconds, "stale after")
	assert.Equal(t, openapi.NotificationSettings_SuccessPolicyEvery, view.Notifications.SuccessPolicy, "policy")
	require.Len(t, view.Basemaps, 1, "basemaps")
	assert.Equal(t, "Streets", view.Basemaps[0].Name, "basemap name")
	assert.Empty(t, view.Surface.Regions, "regions")
}

// The whole object is written and the whole object comes back, so the page's
// copy is the service's copy without a second read.
func TestSetSettingsStoresAndEchoesTheWholeObject(t *testing.T) {
	handler, settings := settingsHandler(t)

	view := settingsOf(t, handler, authenticatedRequestWithBody(http.MethodPut, settingsPath, settingsBody))
	assert.True(t, view.Sync.AllowEmptySourceDeletion, "the deletion gate")
	assert.Equal(t, openapi.NotificationSettings_SuccessPolicyDigest, view.Notifications.SuccessPolicy, "policy")
	require.Len(t, view.Basemaps, 1, "basemaps")
	assert.Equal(t, "Satellite", view.Basemaps[0].Name, "basemap name")
	assert.Equal(t, []string{"europe/germany"}, view.Surface.Regions, "regions")

	stored := settings.Values()
	assert.True(t, stored.Sync.AllowEmptySourceDeletion, "the stored deletion gate")
	assert.False(t, stored.Notifications.Enabled, "the stored notification switch")
	assert.Equal(t, 25*time.Hour, stored.Sync.StaleAfter, "the stored staleness bound")
	assert.Equal(t, "https://pushover.example.test", stored.Notifications.PushoverBaseURL, "the stored origin")
}

// A body that is not one whole object is refused rather than merged, because a
// merge would let a page that had gone stale write back a section its reader
// never looked at.
func TestSetSettingsRejectsAnythingButOneWholeObject(t *testing.T) {
	partial := strings.Replace(settingsBody, `"surface": {"regions": ["europe/germany"], "rebuildIntervalSeconds": 604800}`, `"other": 1`, 1)
	bodies := map[string]string{
		"a missing section": strings.Replace(settingsBody, `"sync"`, `"unread"`, 1),
		"an unknown field":  partial,
		"not json":          "not json",
		"two objects":       settingsBody + settingsBody,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			handler, settings := settingsHandler(t)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsPath, body))
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			assert.Equal(t, testBasemaps(), settings.Values().Basemaps, "a refused body reached the settings")
		})
	}
}

// A value the document allows but the rules refuse is refused here, naming the
// setting — the same rules, and the same message, the stored values are read
// back through at startup.
func TestSetSettingsRefusesASettingThatFailsItsRules(t *testing.T) {
	handler, settings := settingsHandler(t)
	body := strings.Replace(settingsBody, `"digestIntervalSeconds": 3600`, `"digestIntervalSeconds": 2592000`, 1)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsPath, body))
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "notifications.digest_interval", "the message names the setting")
	assert.Equal(t, testBasemaps(), settings.Values().Basemaps, "a refused edit reached the settings")
}

func TestSetSettingsReportsAStoreThatCannotBeWritten(t *testing.T) {
	handler, settings := settingsHandler(t)
	settings.storeFail = errors.New("disk gone")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsPath, settingsBody))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
}

// The settings are the one body larger than the cap every other route carries.
// A list of a dozen basemaps is an ordinary edit and is well past 1 KiB.
func TestSetSettingsAcceptsABodyPastTheDefaultCap(t *testing.T) {
	handler, _ := settingsHandler(t)
	entries := make([]string, 0, 12)
	for index := range 12 {
		entries = append(entries, `{"name": "Provider `+strconv.Itoa(index)+
			`", "styleUrl": "https://tiles.example.test/styles/very-long-provider-style-name-`+
			strconv.Itoa(index)+`"}`)
	}
	body := strings.Replace(settingsBody,
		`[{
		"name": "Satellite",
		"styleUrl": "https://imagery.example.test/styles/hybrid",
		"darkCartography": true
	}]`, "["+strings.Join(entries, ",")+"]", 1)
	require.Greater(t, len(body), maximumRequestBytes, "the body under test is past the default cap")

	view := settingsOf(t, handler, authenticatedRequestWithBody(http.MethodPut, settingsPath, body))
	assert.Len(t, view.Basemaps, 12, "basemaps")
}

// Every other route keeps the cap it is right to have.
func TestOnlyTheSettingsPathCarriesTheLargerBody(t *testing.T) {
	assert.Equal(t, int64(maximumSettingsBytes), requestLimit(settingsPath), "the settings path")
	assert.Equal(t, int64(maximumRequestBytes), requestLimit("/v1/sync/schedule"), "every other path")
}

// The basemap list the page reads and the origins the policy names both come
// off the live settings, so an edit reaches them without a restart.
func TestEditedBasemapsReachTheConfigAndThePolicy(t *testing.T) {
	handler, _ := settingsHandler(t)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsPath, settingsBody))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())

	config := httptest.NewRecorder()
	handler.ServeHTTP(config, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	require.Equal(t, http.StatusOK, config.Code, config.Body.String())
	assert.Contains(t, config.Body.String(), "imagery.example.test", "the page's basemap list")
	assert.Contains(t, config.Header().Get("Content-Security-Policy"), "https://imagery.example.test",
		"the policy's tile origins")
}

func TestSettingsValuesReadsAnOmittedBasemapFieldAsUnset(t *testing.T) {
	values := settingsValues(
		&openapi.SyncSettings{StaleAfterSeconds: 3600},
		&openapi.NotificationSettings{DigestIntervalSeconds: 60},
		[]openapi.BrowserBasemap{{Name: "Streets", StyleURL: testTileStyleURL}},
		&openapi.SurfaceSettings{},
	)
	assert.Equal(t, []runtimeconfig.Basemap{{Name: "Streets", StyleURL: testTileStyleURL}}, values.Basemaps,
		"an omitted dark style and dark-cartography flag")
}
