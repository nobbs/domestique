package sqlite

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/runtimeconfig"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The seeded settings are the defaults the configuration file documented for
// the same keys, so an upgraded deployment that changes nothing runs on exactly
// what it ran on before.
func TestStoreSeedsTheDocumentedRuntimeSettings(t *testing.T) {
	store := openTestStore(t, testKey(1))

	values, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings()")

	assert.False(t, values.Sync.AllowEmptySourceDeletion, "final-library deletion is denied until asked for")
	assert.Equal(t, 24*time.Hour, values.Sync.StaleAfter, "Sync.StaleAfter")
	assert.True(t, values.Notifications.Enabled, "notifications are on")
	assert.Equal(t, "https://api.pushover.net", values.Notifications.PushoverBaseURL, "Notifications.PushoverBaseURL")
	require.Len(t, values.Basemaps, 1, "Basemaps")
	assert.Equal(t, "Streets", values.Basemaps[0].Name, "Basemaps[0].Name")
	assert.Equal(t, "https://tiles.openfreemap.org/styles/bright", values.Basemaps[0].StyleURL, "Basemaps[0].StyleURL")
	assert.Empty(t, values.Surface.Regions, "surface classification is off until regions are named")
	assert.Equal(t, 7*24*time.Hour, values.Surface.RebuildInterval, "Surface.RebuildInterval")
	assert.Equal(t, time.Minute, values.Sync.InitialDelay, "Sync.InitialDelay")
	assert.Empty(t, values.RideModel.CoefficientsFile, "prediction is off until a profile is named")
}

// Everything that names an upstream is seeded unconfigured instead, because
// none of it can be guessed: a service holding these seeds starts, serves its
// settings page, and runs nothing.
func TestStoreSeedsNoUpstreamAtAll(t *testing.T) {
	store := openTestStore(t, testKey(1))

	values, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings()")

	assert.Empty(t, values.Wahoo.APIBaseURL, "Wahoo.APIBaseURL")
	assert.Empty(t, values.Wahoo.OAuthBaseURL, "Wahoo.OAuthBaseURL")
	assert.Empty(t, values.Wahoo.ClientID, "Wahoo.ClientID")
	assert.Empty(t, values.Wahoo.Targets, "Wahoo.Targets")
	assert.Empty(t, values.Sources, "Sources")

	secrets, err := store.RuntimeSecrets(t.Context())
	require.NoError(t, err, "RuntimeSecrets()")
	assert.Empty(t, secrets, "no credential is seeded")
}

func TestStoreKeepsTheRuntimeSettingsItWasGiven(t *testing.T) {
	store := openTestStore(t, testKey(1))

	next := runtimeconfig.Values{
		Sync: runtimeconfig.Sync{
			AllowEmptySourceDeletion: true,
			StaleAfter:               90 * time.Minute,
			InitialDelay:             5 * time.Minute,
		},
		Wahoo: runtimeconfig.Wahoo{
			APIBaseURL:   "https://api.wahooligan.com",
			OAuthBaseURL: "https://api.wahooligan.com",
			ClientID:     "client-id",
			Targets:      []string{"rider-a", "rider-b"},
		},
		Sources: []runtimeconfig.Source{
			{Provider: route.ProviderKomoot, BaseURL: "https://api.komoot.de"},
			{Provider: route.ProviderVeloPlanner, BaseURL: "https://veloplanner.com"},
		},
		RideModel: runtimeconfig.RideModel{CoefficientsFile: "/etc/domestique/ridemodel.toml"},
		Notifications: runtimeconfig.Notifications{
			Enabled:         false,
			PushoverBaseURL: "https://pushover.example.test",
		},
		Basemaps: []runtimeconfig.Basemap{
			{Name: "Streets", StyleURL: "https://tiles.example.test/bright"},
			{Name: "Satellite", StyleURL: "https://imagery.example.test/style.json", DarkCartography: true},
		},
		Surface: runtimeconfig.Surface{
			Regions:         []string{"europe/germany/rheinland-pfalz", "europe/france"},
			RebuildInterval: 48 * time.Hour,
		},
	}
	require.NoError(t, store.SetRuntimeSettings(t.Context(), next), "SetRuntimeSettings()")

	stored, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings() after the write")
	assert.Equal(t, next, stored, "what was written is what is read back, in the order it was arranged in")
}

// A shorter list is the whole list. Rewriting it must remove the entries that
// are gone rather than leave them behind at their old positions.
func TestStoreReplacesTheRuntimeListsWhole(t *testing.T) {
	store := openTestStore(t, testKey(1))
	values, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings()")

	values.Basemaps = append(values.Basemaps, runtimeconfig.Basemap{
		Name:     "Satellite",
		StyleURL: "https://imagery.example.test/style.json",
	})
	values.Surface.Regions = []string{"europe/germany", "europe/france"}
	values.Wahoo.Targets = []string{"rider-a", "rider-b"}
	values.Sources = []runtimeconfig.Source{
		{Provider: route.ProviderKomoot, BaseURL: "https://api.komoot.de"},
		{Provider: route.ProviderVeloPlanner, BaseURL: "https://veloplanner.com"},
	}
	require.NoError(t, store.SetRuntimeSettings(t.Context(), values), "SetRuntimeSettings()")

	values.Basemaps = values.Basemaps[:1]
	values.Surface.Regions = nil
	values.Wahoo.Targets = values.Wahoo.Targets[:1]
	values.Sources = nil
	require.NoError(t, store.SetRuntimeSettings(t.Context(), values), "SetRuntimeSettings() with shorter lists")

	stored, err := store.RuntimeSettings(t.Context())
	require.NoError(t, err, "RuntimeSettings() after the second write")
	assert.Len(t, stored.Basemaps, 1, "the removed basemap is gone")
	assert.Empty(t, stored.Surface.Regions, "the removed regions are gone")
	assert.Equal(t, []string{"rider-a"}, stored.Wahoo.Targets, "the removed slot is gone")
	assert.Empty(t, stored.Sources, "the removed sources are gone")
}

func TestStoreEncryptsTheCredentialsItStores(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRuntimeSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretKomootPassword: runtimeconfig.NewSecret([]byte("opensesame")),
	}), "SetRuntimeSecrets()")

	var ciphertext []byte
	require.NoError(t, store.database.QueryRowContext(t.Context(),
		"SELECT value FROM runtime_secret WHERE name = ?", "komoot.password").Scan(&ciphertext), "query the stored credential")
	assert.NotContains(t, string(ciphertext), "opensesame", "the database stores the credential in plaintext")

	secrets, err := store.RuntimeSecrets(t.Context())
	require.NoError(t, err, "RuntimeSecrets()")
	assert.Equal(t, []byte("opensesame"), secrets[runtimeconfig.SecretKomootPassword].Bytes(), "the credential reads back")
}

// A write carries only the credentials that were typed into the form, so it has
// to leave every other one exactly as it was.
func TestStoreReplacesOnlyTheCredentialsItWasGiven(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRuntimeSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretKomootEmail:    runtimeconfig.NewSecret([]byte("rider@example.test")),
		runtimeconfig.SecretKomootPassword: runtimeconfig.NewSecret([]byte("opensesame")),
	}), "SetRuntimeSecrets()")

	require.NoError(t, store.SetRuntimeSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretKomootPassword: runtimeconfig.NewSecret([]byte("rotated")),
	}), "SetRuntimeSecrets() with one credential")

	secrets, err := store.RuntimeSecrets(t.Context())
	require.NoError(t, err, "RuntimeSecrets()")
	assert.Equal(t, []byte("rotated"), secrets[runtimeconfig.SecretKomootPassword].Bytes(), "the replaced credential")
	assert.Equal(t, []byte("rider@example.test"), secrets[runtimeconfig.SecretKomootEmail].Bytes(), "the one left alone")
}

func TestStoreRemovesACredentialWrittenWithNoValue(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRuntimeSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretKomootPassword: runtimeconfig.NewSecret([]byte("opensesame")),
	}), "SetRuntimeSecrets()")

	require.NoError(t, store.SetRuntimeSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretKomootPassword: {},
	}), "SetRuntimeSecrets() with no value")

	secrets, err := store.RuntimeSecrets(t.Context())
	require.NoError(t, err, "RuntimeSecrets()")
	assert.NotContains(t, secrets, runtimeconfig.SecretKomootPassword)
}

// The name is the associated data, so a ciphertext moved from one row to
// another fails to open rather than authenticating as the wrong credential.
func TestStoreBindsACredentialToItsName(t *testing.T) {
	store := openTestStore(t, testKey(1))
	require.NoError(t, store.SetRuntimeSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretKomootPassword: runtimeconfig.NewSecret([]byte("opensesame")),
	}), "SetRuntimeSecrets()")

	_, err := store.database.ExecContext(t.Context(), `
		INSERT INTO runtime_secret (name, value, updated_at_unix)
		SELECT ?, value, updated_at_unix FROM runtime_secret WHERE name = ?
	`, "veloplanner.password", "komoot.password")
	require.NoError(t, err, "copy the ciphertext to another name")

	_, err = store.RuntimeSecrets(t.Context())
	require.ErrorIs(t, err, ErrStateUnreadable, "RuntimeSecrets()")
}

// A database written under another key holds credentials nothing can open, and
// starting on it anyway would reach every upstream as nobody.
func TestStoreRejectsCredentialsWrittenUnderAnotherKey(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state.db")
	store, openErr := Open(t.Context(), databasePath, testKey(1))
	require.NoError(t, openErr, "Open()")
	require.NoError(t, store.SetRuntimeSecrets(t.Context(), map[runtimeconfig.SecretName]runtimeconfig.Secret{
		runtimeconfig.SecretKomootPassword: runtimeconfig.NewSecret([]byte("opensesame")),
	}), "SetRuntimeSecrets()")
	require.NoError(t, store.Close(), "Close()")

	reopened, err := Open(t.Context(), databasePath, testKey(2))
	require.NoError(t, err, "Open() with different key")
	t.Cleanup(func() {
		assert.NoError(t, reopened.Close(), "Close()")
	})

	_, err = reopened.RuntimeSecrets(t.Context())
	require.ErrorIs(t, err, ErrStateUnreadable, "RuntimeSecrets()")
}
