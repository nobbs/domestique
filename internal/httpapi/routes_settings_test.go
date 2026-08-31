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
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
)

// The settings are edited one section at a time, and each of these paths
// replaces the whole of the section it names.
const (
	settingsPath              = "/v1/settings"
	settingsWahooPath         = "/v1/settings/wahoo"
	settingsTargetsPath       = "/v1/settings/targets"
	settingsKomootPath        = "/v1/settings/sources/komoot"
	settingsVeloPlannerPath   = "/v1/settings/sources/veloplanner"
	settingsNotificationsPath = "/v1/settings/notifications"
	settingsSurfacePath       = "/v1/settings/surface"
	settingsRideModelPath     = "/v1/settings/ridemodel"
	settingsSyncPath          = "/v1/settings/sync"
	settingsAlertsPath        = "/v1/settings/alerts"
	settingsTimezonePath      = "/v1/settings/timezone"
)

// Each of these differs from settingsWith in every field it carries, so a test
// that sends one and reads the settings back cannot pass on what was already
// there.
const (
	wahooSubmission = `{
		"apiBaseUrl": "https://api.wahoo.example.test",
		"oauthBaseUrl": "https://oauth.wahoo.example.test",
		"clientId": "client-id",
		"clientSecret": "client-secret"
	}`
	targetsSubmission       = `{"targets": ["rider-b"]}`
	notificationsSubmission = `{
		"enabled": false,
		"pushoverBaseUrl": "https://pushover.example.test",
		"applicationToken": "application-token",
		"userKey": "user-key"
	}`
	basemapsSubmission = `{"basemaps": [{
		"name": "Satellite",
		"styleUrl": "https://imagery.example.test/styles/hybrid",
		"darkCartography": true
	}]}`
	surfaceSubmission   = `{"regions": ["europe/germany"], "rebuildIntervalSeconds": 604800}`
	rideModelSubmission = `{"coefficientsFile": "/var/lib/domestique/coefficients.json"}`
	syncSubmission      = `{"allowEmptySourceDeletion": true, "staleAfterSeconds": 90000, "initialDelaySeconds": 300}`
	alertsSubmission    = `{"alerts": [{"task": "sync", "alert": "source", "enabled": false}]}`
	komootSubmission    = `{
		"read": true,
		"baseUrl": "https://komoot.example.test",
		"email": "rider@example.test",
		"password": "opensesame"
	}`
)

// settingsHandler builds a handler over settings a test can read back.
func settingsHandler(t *testing.T) (*Handler, *staticSettings) {
	t.Helper()
	settings := settingsWith(testBasemaps())
	handler, err := New(
		&Options{
			Alerts:           &fakeAlerts{},
			Tasks:            &fakeTasks{},
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

// saveSection sends one section's edit and returns the settings it answers with.
func saveSection(t *testing.T, handler *Handler, path, body string) openapi.Settings {
	t.Helper()

	return settingsOf(t, handler, authenticatedRequestWithBody(http.MethodPut, path, body))
}

func TestGetSettingsServesWhatIsInForce(t *testing.T) {
	handler, _ := settingsHandler(t)

	view := settingsOf(t, handler, authenticatedRequest(http.MethodGet, settingsPath))
	assert.False(t, view.Sync.AllowEmptySourceDeletion, "the deletion gate")
	assert.Equal(t, int((26 * time.Hour).Seconds()), view.Sync.StaleAfterSeconds, "stale after")
	require.Len(t, view.Basemaps, 1, "basemaps")
	assert.Equal(t, "Streets", view.Basemaps[0].Name, "basemap name")
	assert.Empty(t, view.Surface.Regions, "regions")
}

// Each section is written whole by the endpoint that names it.
func TestEachSectionIsStoredByItsOwnEndpoint(t *testing.T) {
	tests := []struct {
		stored func(t *testing.T, values runtimeconfig.Values)
		name   string
		path   string
		body   string
	}{
		{
			name: "the wahoo application",
			path: settingsWahooPath,
			body: wahooSubmission,
			stored: func(t *testing.T, values runtimeconfig.Values) {
				t.Helper()
				assert.Equal(t, "client-id", values.Wahoo.ClientID, "the application")
				assert.Equal(t, "https://api.wahoo.example.test", values.Wahoo.APIBaseURL, "the api origin")
				assert.Equal(t, []string{"rider-a"}, values.Wahoo.Targets, "the slots it writes to")
			},
		},
		{
			name: "the destination slots",
			path: settingsTargetsPath,
			body: targetsSubmission,
			stored: func(t *testing.T, values runtimeconfig.Values) {
				t.Helper()
				assert.Equal(t, []string{"rider-b"}, values.Wahoo.Targets, "the slots")
			},
		},
		{
			name: "notifications",
			path: settingsNotificationsPath,
			body: notificationsSubmission,
			stored: func(t *testing.T, values runtimeconfig.Values) {
				t.Helper()
				assert.False(t, values.Notifications.Enabled, "the switch")
				assert.Equal(t, "https://pushover.example.test", values.Notifications.PushoverBaseURL, "the origin")
			},
		},
		{
			name: "basemaps",
			path: basemapsPath,
			body: basemapsSubmission,
			stored: func(t *testing.T, values runtimeconfig.Values) {
				t.Helper()
				assert.Equal(t, []runtimeconfig.Basemap{{
					Name:            "Satellite",
					StyleURL:        "https://imagery.example.test/styles/hybrid",
					DarkCartography: true,
				}}, values.Basemaps, "the list, with an omitted dark style read as unset")
			},
		},
		{
			name: "the surface index",
			path: settingsSurfacePath,
			body: surfaceSubmission,
			stored: func(t *testing.T, values runtimeconfig.Values) {
				t.Helper()
				assert.Equal(t, []string{"europe/germany"}, values.Surface.Regions, "the regions")
			},
		},
		{
			name: "the ride model",
			path: settingsRideModelPath,
			body: rideModelSubmission,
			stored: func(t *testing.T, values runtimeconfig.Values) {
				t.Helper()
				assert.Equal(t, "/var/lib/domestique/coefficients.json", values.RideModel.CoefficientsFile, "the file")
			},
		},
		{
			name: "the run settings",
			path: settingsSyncPath,
			body: syncSubmission,
			stored: func(t *testing.T, values runtimeconfig.Values) {
				t.Helper()
				assert.True(t, values.Sync.AllowEmptySourceDeletion, "the deletion gate")
				assert.Equal(t, 25*time.Hour, values.Sync.StaleAfter, "the staleness bound")
				assert.Equal(t, 5*time.Minute, values.Sync.InitialDelay, "the first-run delay")
			},
		},
		{
			name: "one library",
			path: settingsKomootPath,
			body: komootSubmission,
			stored: func(t *testing.T, values runtimeconfig.Values) {
				t.Helper()
				assert.Equal(t, []runtimeconfig.Source{
					{Provider: route.ProviderKomoot, BaseURL: "https://komoot.example.test"},
				}, values.Sources, "the libraries read")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			handler, settings := settingsHandler(t)

			saveSection(t, handler, test.path, test.body)
			test.stored(t, settings.Values())
		})
	}
}

// A section replaces itself and nothing else, which is what lets one page hold
// a save button per section.
func TestASectionLeavesEveryOtherSectionAsItWas(t *testing.T) {
	handler, settings := settingsHandler(t)
	want := settingsWith(testBasemaps()).Values()
	want.Surface = runtimeconfig.Surface{Regions: []string{"europe/germany"}, RebuildInterval: 7 * 24 * time.Hour}

	saveSection(t, handler, settingsSurfacePath, surfaceSubmission)
	assert.Equal(t, want, settings.Values())
}

// The answer is every setting in force rather than the section just written, so
// the page's copy is the service's copy without a second read.
func TestASavedSectionIsAnsweredWithEverySetting(t *testing.T) {
	handler, _ := settingsHandler(t)

	view := saveSection(t, handler, settingsTargetsPath, targetsSubmission)
	assert.Equal(t, []string{"rider-b"}, view.Wahoo.Targets, "the section that was written")
	assert.Equal(t, int((26 * time.Hour).Seconds()), view.Sync.StaleAfterSeconds, "a section that was not")
}

// Turning a library on puts it in the order a run reads the libraries in,
// whatever order they were turned on in.
func TestLibrariesAreReadInTheirOwnOrderHoweverTheyAreTurnedOn(t *testing.T) {
	handler, settings := settingsHandler(t)

	saveSection(t, handler, settingsKomootPath, komootSubmission)
	saveSection(t, handler, settingsVeloPlannerPath,
		`{"read": true, "baseUrl": "https://veloplanner.example.test"}`)

	assert.Equal(t, []route.Provider{route.ProviderVeloPlanner, route.ProviderKomoot},
		[]route.Provider{settings.Values().Sources[0].Provider, settings.Values().Sources[1].Provider})
}

// A library turned off is not read, and the account it was read with stays
// stored, so turning it back on does not ask for the credentials again.
func TestALibraryTurnedOffKeepsItsAccount(t *testing.T) {
	handler, settings := settingsHandler(t)
	saveSection(t, handler, settingsKomootPath, komootSubmission)

	saveSection(t, handler, settingsKomootPath, `{"read": false, "baseUrl": "https://komoot.example.test"}`)
	assert.Empty(t, settings.Values().Sources, "the libraries read")
	assert.True(t, settings.SecretIsSet(runtimeconfig.SecretKomootPassword), "the stored account")
}

// A credential is written but never read back: the response says it is set, and
// says nothing else about it.
func TestASectionStoresItsCredentialsWithoutEverServingThem(t *testing.T) {
	handler, settings := settingsHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsKomootPath, komootSubmission))
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	assert.NotContains(t, response.Body.String(), "opensesame", "the response carried a credential")

	assert.Equal(t, []byte("opensesame"),
		settings.secrets[runtimeconfig.SecretKomootPassword].Bytes(), "the stored credential")

	view := settingsOf(t, handler, authenticatedRequest(http.MethodGet, settingsPath))
	assert.True(t, view.SecretsSet[string(runtimeconfig.SecretKomootPassword)], "the credential that was sent")
	assert.False(t, view.SecretsSet[string(runtimeconfig.SecretWahooClientSecret)], "one that never was")
}

// The page is never told what a credential is, so it has nothing to send back:
// one left out of an edit keeps its stored value, and one sent empty is the
// credential being removed.
func TestOnlyTheCredentialsAnEditCarriesAreWritten(t *testing.T) {
	handler, settings := settingsHandler(t)
	saveSection(t, handler, settingsKomootPath, komootSubmission)

	saveSection(t, handler, settingsKomootPath, `{"read": true, "baseUrl": "https://komoot.example.test"}`)
	assert.True(t, settings.SecretIsSet(runtimeconfig.SecretKomootPassword), "a credential left out")

	saveSection(t, handler, settingsKomootPath,
		`{"read": true, "baseUrl": "https://komoot.example.test", "password": ""}`)
	assert.False(t, settings.SecretIsSet(runtimeconfig.SecretKomootPassword), "a credential sent empty")
	assert.True(t, settings.SecretIsSet(runtimeconfig.SecretKomootEmail), "the one beside it")
}

// The page is told what is still needed, so an operator reads it there rather
// than finding out from a run that did nothing.
func TestGetSettingsNamesWhatIsStillNeeded(t *testing.T) {
	handler, settings := settingsHandler(t)
	settings.missing = []string{"wahoo.client_id"}

	view := settingsOf(t, handler, authenticatedRequest(http.MethodGet, settingsPath))
	assert.Equal(t, []string{"wahoo.client_id"}, view.Missing, "the settings still needed")
}

// A body that is not one whole section is refused rather than merged, because a
// merge would let a page that had gone stale write back a field its reader
// never looked at.
func TestASectionRejectsAnythingButOneWholeSection(t *testing.T) {
	bodies := map[string]string{
		"a missing field":  `{"regions": ["europe/germany"]}`,
		"an unknown field": `{"regions": ["europe/germany"], "rebuildIntervalSeconds": 604800, "other": 1}`,
		"another section":  syncSubmission,
		"not json":         "not json",
		"two objects":      surfaceSubmission + surfaceSubmission,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			handler, settings := settingsHandler(t)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsSurfacePath, body))
			assert.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
			assert.Empty(t, settings.Values().Surface.Regions, "a refused body reached the settings")
		})
	}
}

// A credential belongs to the section that owns it, so one offered to a section
// that does not read it is refused where it is entered rather than stored
// somewhere nothing would look for it.
func TestASectionRefusesACredentialItDoesNotOwn(t *testing.T) {
	handler, settings := settingsHandler(t)
	body := `{"regions": ["europe/germany"], "rebuildIntervalSeconds": 604800, "password": "opensesame"}`

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsSurfacePath, body))
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Empty(t, settings.secrets, "a refused body reached the credentials")
}

// The libraries this path accepts are named in the contract, so a request for
// anything else is refused before a handler is reached and cannot store a
// library nothing reads or an account nothing owns.
func TestASourceThatIsNotALibraryIsRefused(t *testing.T) {
	handler, settings := settingsHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response,
		authenticatedRequestWithBody(http.MethodPut, "/v1/settings/sources/strava", komootSubmission))
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Empty(t, settings.Values().Sources, "the libraries read")
	assert.Empty(t, settings.secrets, "a refused body reached the credentials")
}

// The section is written before its credentials, so a section the rules refuse
// leaves the account it carried unwritten: the page shows the failure and there
// is nothing stored behind it.
func TestARefusedSectionStoresNoCredential(t *testing.T) {
	handler, settings := settingsHandler(t)
	body := strings.Replace(komootSubmission, `"https://komoot.example.test"`,
		`"https://komoot.example.test/library"`, 1)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsKomootPath, body))
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "base_url", "the message names the setting")
	assert.Empty(t, settings.secrets, "a refused section reached the credentials")
}

// A value the document allows but the rules refuse is refused here, naming the
// setting — the same rules, and the same message, the stored values are read
// back through at startup.
func TestASectionRefusesASettingThatFailsItsRules(t *testing.T) {
	handler, settings := settingsHandler(t)
	body := strings.Replace(notificationsSubmission,
		`"pushoverBaseUrl": "https://pushover.example.test"`,
		`"pushoverBaseUrl": "http://pushover.example.test"`, 1)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsNotificationsPath, body))
	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "notifications.pushover.base_url", "the message names the setting")
	assert.Equal(t, "https://api.pushover.net", settings.Values().Notifications.PushoverBaseURL,
		"a refused edit reached the settings")
}

func TestASectionReportsAStoreThatCannotBeWritten(t *testing.T) {
	handler, settings := settingsHandler(t)
	settings.storeFail = errors.New("disk gone")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsSyncPath, syncSubmission))
	assert.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
}

// The basemaps are the one body larger than the cap every other route carries.
// A list of a dozen basemaps is an ordinary edit and is well past 1 KiB.
func TestSetBasemapsAcceptsABodyPastTheDefaultCap(t *testing.T) {
	handler, _ := settingsHandler(t)
	entries := make([]string, 0, 12)
	for index := range 12 {
		entries = append(entries, `{"name": "Provider `+strconv.Itoa(index)+
			`", "styleUrl": "https://tiles.example.test/styles/very-long-provider-style-name-`+
			strconv.Itoa(index)+`"}`)
	}
	body := `{"basemaps": [` + strings.Join(entries, ",") + `]}`
	require.Greater(t, len(body), maximumRequestBytes, "the body under test is past the default cap")

	view := saveSection(t, handler, basemapsPath, body)
	assert.Len(t, view.Basemaps, 12, "basemaps")
}

// Every other route keeps the cap it is right to have.
func TestOnlyTheBasemapsPathCarriesTheLargerBody(t *testing.T) {
	assert.Equal(t, int64(maximumSettingsBytes), requestLimit(basemapsPath), "the basemaps path")
	assert.Equal(t, int64(maximumRequestBytes), requestLimit(settingsSurfacePath), "every other path")
}

// The basemap list the page reads and the origins the policy names both come
// off the live settings, so an edit reaches them without a restart.
func TestEditedBasemapsReachTheConfigAndThePolicy(t *testing.T) {
	handler, _ := settingsHandler(t)
	saveSection(t, handler, basemapsPath, basemapsSubmission)

	config := httptest.NewRecorder()
	handler.ServeHTTP(config, authenticatedRequest(http.MethodGet, "/v1/webui/config"))
	require.Equal(t, http.StatusOK, config.Code, config.Body.String())
	assert.Contains(t, config.Body.String(), "imagery.example.test", "the page's basemap list")
	assert.Contains(t, config.Header().Get("Content-Security-Policy"), "https://imagery.example.test",
		"the policy's tile origins")
}

// alertsHandler builds a handler over an alert matrix a test can read back.
func alertsHandler(t *testing.T, catalogue ...AlertSetting) (*Handler, *fakeAlerts) {
	t.Helper()
	alerts := &fakeAlerts{catalogue: catalogue}
	handler, err := New(
		&Options{
			Settings:         settingsWith(testBasemaps()),
			Alerts:           alerts,
			Tasks:            &fakeTasks{},
			AccessVerifier:   &recordingVerifier{email: testAccessEmail},
			AccessEmail:      testAccessEmail,
			BrowserOriginURL: testBrowserOriginURL,
		},
		&fakeOAuth{}, &fakeState{}, &fakeSync{accepted: true}, &fakeAssets{}, &fakeWeather{},
	)
	require.NoError(t, err, "New()")

	return handler, alerts
}

// The page draws the matrix from what the settings carry, so every alert the
// service can raise has to be in there whether or not anyone has ruled on it.
func TestGetSettingsCarriesEveryAlertTheServiceCanRaise(t *testing.T) {
	handler, _ := alertsHandler(t,
		AlertSetting{Task: "sync", Alert: "failed", Enabled: true},
		AlertSetting{Task: "surface:index", Alert: "failed", Decided: true},
	)

	view := settingsOf(t, handler, authenticatedRequest(http.MethodGet, settingsPath))
	assert.Equal(t, []openapi.AlertSetting{
		{Task: "sync", Alert: "failed", Enabled: true, Decided: false},
		{Task: "surface:index", Alert: "failed", Enabled: false, Decided: true},
	}, view.Alerts, "the alert matrix")
}

func TestGetSettingsSendsAnEmptyAlertMatrixAsAList(t *testing.T) {
	handler, _ := alertsHandler(t)

	view := settingsOf(t, handler, authenticatedRequest(http.MethodGet, settingsPath))
	assert.NotNil(t, view.Alerts, "an empty matrix was sent as null")
	assert.Empty(t, view.Alerts, "the alert matrix")
}

func TestSetAlertsRecordsWhatWasDecided(t *testing.T) {
	handler, alerts := alertsHandler(t,
		AlertSetting{Task: "sync", Alert: "failed", Enabled: true},
		AlertSetting{Task: "surface:index", Alert: "failed", Enabled: true},
	)

	view := saveSection(t, handler, settingsAlertsPath,
		`{"alerts": [{"task": "sync", "alert": "failed", "enabled": false}]}`)

	assert.Equal(t, []AlertDecision{{Task: "sync", Alert: "failed", Enabled: false}}, alerts.decided, "the decisions")
	// A decision is answered with the matrix it produced, so a page that reads
	// the response is looking at what is now in force rather than what it sent.
	assert.Equal(t, []openapi.AlertSetting{
		{Task: "sync", Alert: "failed", Enabled: false, Decided: true},
		{Task: "surface:index", Alert: "failed", Enabled: true, Decided: false},
	}, view.Alerts, "the answered matrix")
}

// A decision about an alert nothing raises would store a row nobody reads and
// draw a switch on the page that does nothing.
func TestSetAlertsRefusesAnAlertNothingRaises(t *testing.T) {
	handler, alerts := alertsHandler(t, AlertSetting{Task: "sync", Alert: "failed"})

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsAlertsPath,
		`{"alerts": [{"task": "sync", "alert": "invented", "enabled": true}]}`))

	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	// The reason is named, and the request's own strings are not sent back.
	assert.Contains(t, response.Body.String(), "an alert no task raises", "the message names the reason")
	assert.NotContains(t, response.Body.String(), "invented", "the message echoed the request")
	assert.Empty(t, alerts.decided, "a refused edit reached the matrix")
}

func TestSetAlertsReportsAStoreThatCannotBeWritten(t *testing.T) {
	handler, alerts := alertsHandler(t, AlertSetting{Task: "sync", Alert: "failed"})
	alerts.err = errors.New("disk gone")

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(http.MethodPut, settingsAlertsPath,
		`{"alerts": [{"task": "sync", "alert": "failed", "enabled": false}]}`))

	assert.Equal(t, http.StatusServiceUnavailable, response.Code, response.Body.String())
}

func TestSetTimezoneStoresTheZoneAndAnswersWithIt(t *testing.T) {
	handler, settings := settingsHandler(t)

	view := saveSection(t, handler, settingsTimezonePath, `{"timezone": "Europe/Lisbon"}`)

	assert.Equal(t, "Europe/Lisbon", view.Timezone, "the answered zone")
	assert.Equal(t, "Europe/Lisbon", settings.Values().Timezone, "the stored zone")
}

// A zone this service cannot load has no local time, so a schedule reading it
// would have no time to run at. It is refused where it is entered.
func TestSetTimezoneRefusesAZoneItCannotLoad(t *testing.T) {
	handler, settings := settingsHandler(t)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, authenticatedRequestWithBody(
		http.MethodPut, settingsTimezonePath, `{"timezone": "Middle/Earth"}`))

	require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
	assert.Contains(t, response.Body.String(), "timezone", "the message names the setting")
	assert.Equal(t, "Europe/Berlin", settings.Values().Timezone, "a refused edit reached the settings")
}
