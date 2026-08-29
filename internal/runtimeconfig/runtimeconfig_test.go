package runtimeconfig

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

// stubStore stands in for the SQLite store: it remembers the last values
// written and can be made to fail either half.
type stubStore struct {
	readErr  error
	writeErr error
	secrets  map[SecretName]Secret
	values   Values
	writing  sync.Mutex
	writes   int
}

func (s *stubStore) RuntimeSettings(context.Context) (Values, error) {
	if s.readErr != nil {
		return Values{}, s.readErr
	}

	return s.values, nil
}

//nolint:gocritic // value param: this double conforms to the Store contract.
func (s *stubStore) SetRuntimeSettings(_ context.Context, values Values) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.writes++
	s.values = values

	return nil
}

func (s *stubStore) RuntimeSecrets(context.Context) (map[SecretName]Secret, error) {
	if s.readErr != nil {
		return nil, s.readErr
	}

	return maps.Clone(s.secrets), nil
}

func (s *stubStore) SetRuntimeSecrets(_ context.Context, secrets map[SecretName]Secret) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	// The real store is a database and serialises its own writes; this one is a
	// map, and the test that saves several sections at once writes to it from
	// every goroutine at the same moment.
	s.writing.Lock()
	defer s.writing.Unlock()
	s.writes++
	if s.secrets == nil {
		s.secrets = make(map[SecretName]Secret, len(secrets))
	}
	maps.Copy(s.secrets, secrets)

	return nil
}

// replaceWith is the whole-object edit: every test here replaces all of the
// settings, where the service replaces one section at a time.
//
//nolint:gocritic // value param: mirrors Update's own copy-in.
func replaceWith(values Values) func(Values) Values {
	return func(Values) Values { return values }
}

func validValues() Values {
	return Values{
		Sync: Sync{StaleAfter: 24 * time.Hour, InitialDelay: time.Minute},
		Notifications: Notifications{
			Enabled:         true,
			Policy:          SuccessPolicyDigest,
			DigestInterval:  24 * time.Hour,
			PushoverBaseURL: "https://api.pushover.net",
		},
		Basemaps: []Basemap{{Name: "Streets", StyleURL: "https://tiles.example.test/styles/bright"}},
		Surface:  Surface{Regions: []string{"europe/germany"}, RebuildInterval: 7 * 24 * time.Hour},
	}
}

func TestLoadPublishesTheStoredSettings(t *testing.T) {
	store := &stubStore{values: validValues()}

	current, err := Load(t.Context(), store)
	require.NoError(t, err)

	values := current.Values()
	assert.Equal(t, 24*time.Hour, values.Sync.StaleAfter, "Sync.StaleAfter")
	assert.Equal(t, SuccessPolicyDigest, values.Notifications.Policy, "Notifications.Policy")
	assert.Equal(t, []string{"europe/germany"}, values.Surface.Regions, "Surface.Regions")
	assert.Zero(t, store.writes, "reading settings must not write them back")
}

// A database edited into something the write path would have refused is a
// startup failure naming the setting, not a service running on it.
func TestLoadRefusesStoredSettingsThatFailTheirOwnRules(t *testing.T) {
	values := validValues()
	values.Notifications.Policy = "loudly"

	_, err := Load(t.Context(), &stubStore{values: values})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "notifications.success_policy")
}

func TestLoadReportsAStoreThatCannotBeRead(t *testing.T) {
	_, err := Load(t.Context(), &stubStore{readErr: errors.New("disk gone")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reading runtime settings")
}

func TestLoadNormalisesWhatItPublishes(t *testing.T) {
	values := validValues()
	values.Basemaps[0].Name = "  Streets  "
	values.Surface.Regions = []string{"europe/germany", " ", "europe/germany"}

	current, err := Load(t.Context(), &stubStore{values: values})
	require.NoError(t, err)

	assert.Equal(t, "Streets", current.Values().Basemaps[0].Name, "a trimmed name is what readers see")
	assert.Equal(t, []string{"europe/germany"}, current.Values().Surface.Regions, "the repeat is dropped")
}

func TestUpdatePersistsThenPublishes(t *testing.T) {
	store := &stubStore{values: validValues()}
	current, err := Load(t.Context(), store)
	require.NoError(t, err)

	next := validValues()
	next.Sync.AllowEmptySourceDeletion = true
	next.Notifications.Enabled = false
	require.NoError(t, current.Update(t.Context(), replaceWith(next)))

	assert.Equal(t, 1, store.writes, "the edit is written once")
	assert.True(t, store.values.Sync.AllowEmptySourceDeletion, "the store holds the new value")
	assert.True(t, current.Values().Sync.AllowEmptySourceDeletion, "readers see the new value")
	assert.False(t, current.Values().Notifications.Enabled, "readers see the whole edit")
}

// Validation runs before the write, so a refused edit changes neither half.
func TestUpdateRefusesInvalidValuesWithoutWriting(t *testing.T) {
	store := &stubStore{values: validValues()}
	current, err := Load(t.Context(), store)
	require.NoError(t, err)

	invalid := validValues()
	invalid.Basemaps = nil
	require.Error(t, current.Update(t.Context(), replaceWith(invalid)))

	assert.Zero(t, store.writes, "a refused edit is not written")
	assert.Len(t, current.Values().Basemaps, 1, "the live settings are untouched")
}

// A write that fails must not be published, or the snapshot would describe a
// service the database does not.
func TestSetKeepsTheLiveValuesWhenTheStoreFails(t *testing.T) {
	store := &stubStore{values: validValues(), writeErr: errors.New("disk full")}
	current, err := Load(t.Context(), store)
	require.NoError(t, err)

	next := validValues()
	next.Sync.StaleAfter = time.Hour
	err = current.Update(t.Context(), replaceWith(next))
	require.ErrorIs(t, err, ErrStore)
	assert.Equal(t, 24*time.Hour, current.Values().Sync.StaleAfter, "the live settings are untouched")
}

// A reader that holds a snapshot must not be able to change what the next
// reader sees, which a shared backing array would allow.
func TestValuesHandsOutIndependentLists(t *testing.T) {
	current, err := Load(t.Context(), &stubStore{values: validValues()})
	require.NoError(t, err)

	held := current.Values()
	held.Basemaps[0].Name = "Rewritten"
	held.Surface.Regions[0] = "elsewhere"

	assert.Equal(t, "Streets", current.Values().Basemaps[0].Name, "Basemaps")
	assert.Equal(t, []string{"europe/germany"}, current.Values().Surface.Regions, "Surface.Regions")
}

func TestValidateSync(t *testing.T) {
	valid := Sync{StaleAfter: time.Second, InitialDelay: time.Second}
	require.NoError(t, ValidateSync(valid))
	require.Error(t, ValidateSync(Sync{}), "a bound of zero would report every inventory stale")
	require.Error(t, ValidateSync(Sync{StaleAfter: time.Millisecond, InitialDelay: time.Second}))
	require.Error(t, ValidateSync(Sync{StaleAfter: time.Second}),
		"a first run with no delay would start before the listeners are up")
}

func TestValidateNotifications(t *testing.T) {
	valid := validValues().Notifications

	tests := []struct {
		mutate  func(*Notifications)
		name    string
		wantErr string
	}{
		{name: "a policy that is not one of the three", mutate: func(n *Notifications) { n.Policy = "sometimes" }, wantErr: "success_policy"},
		{name: "a digest with no period", mutate: func(n *Notifications) { n.DigestInterval = 0 }, wantErr: "at least 1s"},
		{
			name:    "a period below the second these settings are stored in",
			mutate:  func(n *Notifications) { n.DigestInterval = 500 * time.Millisecond },
			wantErr: "at least 1s",
		},
		{
			name:    "a period reaching past the recorded history",
			mutate:  func(n *Notifications) { n.DigestInterval = 8 * 24 * time.Hour },
			wantErr: "must not exceed",
		},
		{
			name:    "an origin with a path",
			mutate:  func(n *Notifications) { n.PushoverBaseURL = "https://api.pushover.net/1" },
			wantErr: "without a path",
		},
		{
			name:    "a plaintext origin",
			mutate:  func(n *Notifications) { n.PushoverBaseURL = "http://api.pushover.net" },
			wantErr: "absolute HTTPS URL",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			notifications := valid
			test.mutate(&notifications)
			_, err := ValidateNotifications(notifications)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}

	// The period is checked whichever policy is selected, so switching to a
	// digest later cannot be the moment the setting turns out to be invalid.
	quiet := valid
	quiet.Policy = SuccessPolicyQuiet
	quiet.DigestInterval = 0
	_, err := ValidateNotifications(quiet)
	require.Error(t, err)

	trailing := valid
	trailing.PushoverBaseURL = " " + valid.PushoverBaseURL + " "
	normalised, err := ValidateNotifications(trailing)
	require.NoError(t, err)
	assert.Equal(t, valid.PushoverBaseURL, normalised.PushoverBaseURL, "the origin is stored trimmed")
}

// Validate reports the first rule that fails whichever field holds it, because
// every one of them names the setting the operator has to fix.
func TestValidateReportsEachFailingSetting(t *testing.T) {
	tests := []struct {
		mutate  func(*Values)
		name    string
		wantErr string
	}{
		{name: "sync", mutate: func(v *Values) { v.Sync.StaleAfter = 0 }, wantErr: "sync.stale_after"},
		{name: "notifications", mutate: func(v *Values) { v.Notifications.DigestInterval = 0 }, wantErr: "digest_interval"},
		{name: "basemaps", mutate: func(v *Values) { v.Basemaps = nil }, wantErr: "webui.basemaps"},
		{name: "surface", mutate: func(v *Values) { v.Surface.Regions = []string{"Europe"} }, wantErr: "surface.regions"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			values := validValues()
			test.mutate(&values)
			_, err := values.Validate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestValidateHTTPSOrigin(t *testing.T) {
	require.NoError(t, ValidateHTTPSOrigin("origin", "https://api.example.test"))
	require.NoError(t, ValidateHTTPSOrigin("origin", "https://api.example.test/"))
	require.Error(t, ValidateHTTPSOrigin("origin", "https://api.example.test/v1"))
	require.Error(t, ValidateHTTPSOrigin("origin", "https://api.example.test?key=abc"))
	// A rejection fixture for URL userinfo, not a real credential.
	require.Error(t, ValidateHTTPSOrigin("origin", "https://user:pass@api.example.test"))
	require.Error(t, ValidateHTTPSOrigin("origin", "api.example.test"))
}

func TestSameOriginRejectsWhatItCannotParse(t *testing.T) {
	assert.True(t, SameOrigin("https://TILES.example.test/a", "https://tiles.example.test/b"),
		"a host differs in case only, which the browser treats as one origin")
	assert.False(t, SameOrigin("/relative", "https://tiles.example.test/b"))
	assert.False(t, SameOrigin("https://tiles.example.test/a", "/relative"))
}

func TestValidateWahoo(t *testing.T) {
	// Every part empty is the state a deployment starts in, and is accepted
	// here so it can be reported as missing rather than refused as invalid.
	empty, err := ValidateWahoo(Wahoo{})
	require.NoError(t, err)
	assert.Empty(t, empty.Targets, "an unconfigured application has no slots")

	valid := Wahoo{
		APIBaseURL:   " https://api.wahooligan.com ",
		OAuthBaseURL: "https://api.wahooligan.com",
		ClientID:     " client-id ",
		Targets:      []string{" rider-a ", "rider-b"},
	}
	normalised, err := ValidateWahoo(valid)
	require.NoError(t, err)
	assert.Equal(t, "https://api.wahooligan.com", normalised.APIBaseURL, "APIBaseURL")
	assert.Equal(t, "client-id", normalised.ClientID, "ClientID")
	assert.Equal(t, []string{"rider-a", "rider-b"}, normalised.Targets, "Targets")

	tests := []struct {
		mutate  func(*Wahoo)
		name    string
		wantErr string
	}{
		{name: "a plaintext API address", mutate: func(w *Wahoo) { w.APIBaseURL = "http://api.wahooligan.com" }, wantErr: "wahoo.api_base_url"},
		{name: "an OAuth address that is not a URL", mutate: func(w *Wahoo) { w.OAuthBaseURL = "wahooligan.com" }, wantErr: "wahoo.oauth_base_url"},
		// The client parses both as origins, so a path is refused here, where the
		// message can name the setting, rather than when the client is built.
		{
			name:    "an API address carrying a path",
			mutate:  func(w *Wahoo) { w.APIBaseURL = "https://api.wahooligan.com/v1" },
			wantErr: "wahoo.api_base_url must be an origin",
		},
		{name: "a slot with no name", mutate: func(w *Wahoo) { w.Targets = []string{" "} }, wantErr: "is required"},
		{name: "the same slot twice", mutate: func(w *Wahoo) { w.Targets = []string{"rider-a", "rider-a"} }, wantErr: "duplicated"},
		{
			name:    "more slots than a run reconciles",
			mutate:  func(w *Wahoo) { w.Targets = []string{"a", "b", "c"} },
			wantErr: "more than 2",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wahoo := valid
			test.mutate(&wahoo)
			_, err := ValidateWahoo(wahoo)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestValidateSources(t *testing.T) {
	none, err := ValidateSources(nil)
	require.NoError(t, err)
	assert.Empty(t, none, "a service with nothing to read yet is not a mistake")

	sources, err := ValidateSources([]Source{
		{Provider: route.ProviderVeloPlanner, BaseURL: " https://veloplanner.com "},
		{Provider: route.ProviderKomoot, BaseURL: "https://api.komoot.de"},
	})
	require.NoError(t, err)
	assert.Equal(t, "https://veloplanner.com", sources[0].BaseURL, "the address is stored trimmed")

	tests := []struct {
		name    string
		wantErr string
		sources []Source
	}{
		{
			name:    "a provider nothing can read",
			sources: []Source{{Provider: "strava", BaseURL: "https://www.strava.com"}},
			wantErr: "is not a known source",
		},
		{
			name: "the same provider twice",
			sources: []Source{
				{Provider: route.ProviderKomoot, BaseURL: "https://api.komoot.de"},
				{Provider: route.ProviderKomoot, BaseURL: "https://api.komoot.de"},
			},
			wantErr: "duplicated",
		},
		{
			name:    "an address carrying a path",
			sources: []Source{{Provider: route.ProviderKomoot, BaseURL: "https://api.komoot.de/v007"}},
			wantErr: "without a path",
		},
		{
			name:    "a plaintext address",
			sources: []Source{{Provider: route.ProviderKomoot, BaseURL: "http://api.komoot.de"}},
			wantErr: "absolute HTTPS URL",
		},
		{
			name:    "no address at all",
			sources: []Source{{Provider: route.ProviderKomoot}},
			wantErr: "sources[0].base_url",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateSources(test.sources)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestValidateRideModel(t *testing.T) {
	_, err := ValidateRideModel(RideModel{})
	require.NoError(t, err, "no file is prediction switched off")

	// The path arrives from a form field, so it is trimmed like every other one.
	normalised, err := ValidateRideModel(RideModel{CoefficientsFile: " /etc/domestique/ridemodel.toml "})
	require.NoError(t, err)
	assert.Equal(t, "/etc/domestique/ridemodel.toml", normalised.CoefficientsFile)

	_, err = ValidateRideModel(RideModel{CoefficientsFile: "ridemodel.toml"})
	require.Error(t, err,
		"a relative path would resolve against whatever directory the process happens to run in")
}

// A settings page has to be able to store a credential it was never told the
// current value of, and has to leave every other one alone while it does.
func TestSetSecretsReplacesOnlyWhatItWasGiven(t *testing.T) {
	store := &stubStore{
		values:  validValues(),
		secrets: map[SecretName]Secret{SecretKomootEmail: NewSecret([]byte("rider@example.test"))},
	}
	current, err := Load(t.Context(), store)
	require.NoError(t, err)

	require.NoError(t, current.SetSecrets(t.Context(), map[SecretName]Secret{
		SecretKomootPassword: NewSecret([]byte("opensesame")),
	}))

	assert.Equal(t, []byte("opensesame"), current.Secret(SecretKomootPassword).Bytes(), "the new credential is live")
	assert.True(t, current.SecretIsSet(SecretKomootEmail), "the one that was not submitted is untouched")
	assert.False(t, current.SecretIsSet(SecretWahooClientSecret), "one that was never stored stays unset")
}

// The first credential a deployment stores arrives when there are none, which
// is the state a new database is in.
func TestSetSecretsStoresTheFirstCredentialAServiceIsGiven(t *testing.T) {
	current, err := Load(t.Context(), &stubStore{values: validValues()})
	require.NoError(t, err)

	require.NoError(t, current.SetSecrets(t.Context(), map[SecretName]Secret{
		SecretWahooClientSecret: NewSecret([]byte("opensesame")),
	}))

	assert.True(t, current.SecretIsSet(SecretWahooClientSecret))
}

// Every save of the settings page sends only the credentials that were typed,
// so a save that typed none must not move the generation everything holding
// something built from these settings watches.
func TestSetSecretsWritesNothingWhenNoCredentialWasSubmitted(t *testing.T) {
	store := &stubStore{values: validValues()}
	current, err := Load(t.Context(), store)
	require.NoError(t, err)
	generation := current.Generation()

	require.NoError(t, current.SetSecrets(t.Context(), nil))

	assert.Zero(t, store.writes, "writes")
	assert.Equal(t, generation, current.Generation(), "generation")
}

// Each section of the settings page saves its own credentials, so several
// credentials can be on their way at the same moment. Every one of them has to
// survive: a save that reads the stored credentials, adds its own and writes
// them back must not put back a copy the save beside it is missing from.
func TestSetSecretsKeepsEveryCredentialSavedAtOnce(t *testing.T) {
	names := SecretNames()
	// Losing one is a matter of how two saves interleave, so the same race is run
	// several times over rather than once.
	for round := range 8 {
		current, err := Load(t.Context(), &stubStore{values: validValues()})
		require.NoError(t, err)

		// The saves are held until every goroutine is ready so that they read the
		// stored credentials at the same moment, which is when one can be lost.
		start := make(chan struct{})
		var saving sync.WaitGroup
		for _, name := range names {
			saving.Go(func() {
				<-start
				assert.NoError(t, current.SetSecrets(t.Context(),
					map[SecretName]Secret{name: NewSecret([]byte("opensesame"))}))
			})
		}
		close(start)
		saving.Wait()

		for _, name := range names {
			require.True(t, current.SecretIsSet(name), "%s, round %d", name, round)
		}
	}
}

func TestSetSecretsRefusesACredentialNothingReads(t *testing.T) {
	store := &stubStore{values: validValues()}
	current, err := Load(t.Context(), store)
	require.NoError(t, err)

	err = current.SetSecrets(t.Context(), map[SecretName]Secret{"strava.email": NewSecret([]byte("rider"))})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "strava.email")
	assert.Zero(t, store.writes, "a refused credential is not written")
}

// The type exists to keep a credential out of anything observable, so every verb
// that would print one is checked. %s renders an unexported []byte as text, and
// so does %+v, which is the verb slog reaches for. %#v bypasses String entirely
// and reads the field.
func TestSecretDoesNotRenderItsValue(t *testing.T) {
	secret := NewSecret([]byte("opensesame"))

	encoded, err := json.Marshal(struct {
		Value Secret `json:"value"`
	}{Value: secret})
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "opensesame", "JSON")

	var logged bytes.Buffer
	slog.New(slog.NewTextHandler(&logged, nil)).Info("stored", "secret", secret)
	assert.NotContains(t, logged.String(), "opensesame", "slog")

	for _, verb := range []string{"%v", "%+v", "%#v", "%s", "%q"} {
		assert.NotContains(t, fmt.Sprintf(verb, secret), "opensesame", verb)
	}
}

// A credential handed out has to be a copy, or the caller that decrypts it once
// can rewrite what every later reader gets.
func TestSecretHandsOutACopy(t *testing.T) {
	secret := NewSecret([]byte("opensesame"))
	secret.Bytes()[0] = 'X'

	assert.Equal(t, []byte("opensesame"), secret.Bytes())
}

// Missing is what a service that has never been configured says about itself,
// and it is the whole of what a settings page needs to say it.
func TestMissingNamesEverySettingARunNeeds(t *testing.T) {
	current, err := Load(t.Context(), &stubStore{values: validValues()})
	require.NoError(t, err)

	// In the order the settings page offers them, which is the order the list is
	// read back to the operator in.
	assert.Equal(t, []string{
		"wahoo.api_base_url",
		"wahoo.oauth_base_url",
		"wahoo.client_id",
		"wahoo.client_secret",
		"wahoo.targets",
		"sources",
		"notifications.pushover.application_token",
		"notifications.pushover.user_key",
	}, current.Missing())
}

func TestMissingIsEmptyOnceEverythingIsConfigured(t *testing.T) {
	values := validValues()
	values.Wahoo = Wahoo{
		APIBaseURL:   "https://api.wahooligan.com",
		OAuthBaseURL: "https://api.wahooligan.com",
		ClientID:     "client-id",
		Targets:      []string{"rider-a"},
	}
	values.Sources = []Source{{Provider: route.ProviderKomoot, BaseURL: "https://api.komoot.de"}}

	store := &stubStore{values: values, secrets: map[SecretName]Secret{}}
	for _, name := range SecretNames() {
		store.secrets[name] = NewSecret([]byte("configured"))
	}
	current, err := Load(t.Context(), store)
	require.NoError(t, err)

	assert.Empty(t, current.Missing())
}

// A source whose account is not entered is named, because a run refuses rather
// than reading part of a library and calling it the whole inventory.
func TestMissingNamesAConfiguredSourceWithNoAccount(t *testing.T) {
	values := validValues()
	values.Sources = []Source{{Provider: route.ProviderVeloPlanner, BaseURL: "https://veloplanner.com"}}

	current, err := Load(t.Context(), &stubStore{values: values})
	require.NoError(t, err)

	assert.Contains(t, current.Missing(), "veloplanner.email")
	assert.Contains(t, current.Missing(), "veloplanner.password")
}

// Notifications that are switched off need no credentials, so a deployment that
// does not want them is not told it is incomplete.
func TestMissingSkipsPushoverWhenNotificationsAreOff(t *testing.T) {
	values := validValues()
	values.Notifications.Enabled = false

	current, err := Load(t.Context(), &stubStore{values: values})
	require.NoError(t, err)

	assert.NotContains(t, current.Missing(), "notifications.pushover.user_key")
}
