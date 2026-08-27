package runtimeconfig

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubStore stands in for the SQLite store: it remembers the last values
// written and can be made to fail either half.
type stubStore struct {
	values   Values
	readErr  error
	writeErr error
	writes   int
}

func (s *stubStore) RuntimeSettings(context.Context) (Values, error) {
	if s.readErr != nil {
		return Values{}, s.readErr
	}

	return s.values, nil
}

func (s *stubStore) SetRuntimeSettings(_ context.Context, values Values) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	s.writes++
	s.values = values

	return nil
}

func validValues() Values {
	return Values{
		Sync: Sync{StaleAfter: 24 * time.Hour},
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

func TestSetPersistsThenPublishes(t *testing.T) {
	store := &stubStore{values: validValues()}
	current, err := Load(t.Context(), store)
	require.NoError(t, err)

	next := validValues()
	next.Sync.AllowEmptySourceDeletion = true
	next.Notifications.Enabled = false
	require.NoError(t, current.Set(t.Context(), next))

	assert.Equal(t, 1, store.writes, "the edit is written once")
	assert.True(t, store.values.Sync.AllowEmptySourceDeletion, "the store holds the new value")
	assert.True(t, current.Values().Sync.AllowEmptySourceDeletion, "readers see the new value")
	assert.False(t, current.Values().Notifications.Enabled, "readers see the whole edit")
}

// Validation runs before the write, so a refused edit changes neither half.
func TestSetRefusesInvalidValuesWithoutWriting(t *testing.T) {
	store := &stubStore{values: validValues()}
	current, err := Load(t.Context(), store)
	require.NoError(t, err)

	invalid := validValues()
	invalid.Basemaps = nil
	require.Error(t, current.Set(t.Context(), invalid))

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
	err = current.Set(t.Context(), next)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "storing runtime settings")
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
	require.NoError(t, ValidateSync(Sync{StaleAfter: time.Second}))
	require.Error(t, ValidateSync(Sync{}), "a bound of zero would report every inventory stale")
	require.Error(t, ValidateSync(Sync{StaleAfter: time.Millisecond}))
}

func TestValidateNotifications(t *testing.T) {
	valid := validValues().Notifications

	tests := []struct {
		mutate  func(*Notifications)
		name    string
		wantErr string
	}{
		{name: "a policy that is not one of the three", mutate: func(n *Notifications) { n.Policy = "sometimes" }, wantErr: "success_policy"},
		{name: "a digest with no period", mutate: func(n *Notifications) { n.DigestInterval = 0 }, wantErr: "must be positive"},
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
			err := ValidateNotifications(notifications)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}

	// The period is checked whichever policy is selected, so switching to a
	// digest later cannot be the moment the setting turns out to be invalid.
	quiet := valid
	quiet.Policy = SuccessPolicyQuiet
	quiet.DigestInterval = 0
	require.Error(t, ValidateNotifications(quiet))

	require.NoError(t, ValidateNotifications(valid))
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
	//nolint:gosec // A rejection fixture for URL userinfo, not a real credential.
	require.Error(t, ValidateHTTPSOrigin("origin", "https://user:pass@api.example.test"))
	require.Error(t, ValidateHTTPSOrigin("origin", "api.example.test"))
}

func TestSameOriginRejectsWhatItCannotParse(t *testing.T) {
	assert.True(t, SameOrigin("https://TILES.example.test/a", "https://tiles.example.test/b"),
		"a host differs in case only, which the browser treats as one origin")
	assert.False(t, SameOrigin("/relative", "https://tiles.example.test/b"))
	assert.False(t, SameOrigin("https://tiles.example.test/a", "/relative"))
}
