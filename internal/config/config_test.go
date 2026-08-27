package config

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUsesFileSecretsAndDefaults(t *testing.T) {
	configPath, key := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)

	assert.Equal(t, key, settings.State.EncryptionKey(), "State.EncryptionKey()")
	assert.Equal(t, []Target{{ID: "rider-a"}, {ID: "rider-b"}}, settings.Wahoo.Targets())
	assert.Equal(t, "rider@example.test", string(settings.VeloPlanner.Email().Bytes()), "VeloPlanner.Email()")
}

func TestLoadConfiguresBothSourcesAsDistinctClients(t *testing.T) {
	directory := t.TempDir()
	configPath, _ := writeValidConfiguration(t, directory)
	appendValidKomootSection(t, directory, configPath)
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)

	require.NotNil(t, settings.VeloPlanner, "VeloPlanner")
	require.NotNil(t, settings.Komoot, "Komoot")
	assert.Equal(t, "https://veloplanner.example.test", settings.VeloPlanner.BaseURL, "VeloPlanner.BaseURL")
	assert.Equal(t, "https://komoot.example.test", settings.Komoot.BaseURL, "Komoot.BaseURL")
	assert.Equal(t, "rider@example.test", string(settings.VeloPlanner.Email().Bytes()), "VeloPlanner.Email()")
	assert.Equal(t, "komoot-rider@example.test", string(settings.Komoot.Email().Bytes()), "Komoot.Email()")
	assert.Equal(t, "komoot-password", string(settings.Komoot.Password().Bytes()), "Komoot.Password()")
	assert.NotEqual(t, settings.VeloPlanner.Email().Bytes(), settings.Komoot.Email().Bytes(),
		"the two sources must not share a credential")
}

func TestLoadConfiguresKomootAlone(t *testing.T) {
	directory := t.TempDir()
	configPath, _ := writeValidConfiguration(t, directory)
	removeConfigurationLine(t, configPath, "[veloplanner]")
	removeConfigurationLine(t, configPath, "base_url = \"https://veloplanner.example.test\"")
	removeConfigurationLine(t, configPath, "email_file = ")
	removeConfigurationLine(t, configPath, "password_file = ")
	appendValidKomootSection(t, directory, configPath)
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)

	assert.Nil(t, settings.VeloPlanner, "VeloPlanner")
	require.NotNil(t, settings.Komoot, "Komoot")
	assert.Equal(t, "https://komoot.example.test", settings.Komoot.BaseURL, "Komoot.BaseURL")
}

func TestLoadTreatsAnEnvironmentOnlySourceAsConfigured(t *testing.T) {
	// Presence is asked of the fully merged configuration, not the TOML file
	// alone, so a source named only through environment variables — no
	// matching section in the file at all — must still count as configured.
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	removeConfigurationLine(t, configPath, "[veloplanner]")
	removeConfigurationLine(t, configPath, "base_url = \"https://veloplanner.example.test\"")
	removeConfigurationLine(t, configPath, "email_file = ")
	removeConfigurationLine(t, configPath, "password_file = ")
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"KOMOOT__BASE_URL", "https://komoot.example.test")
	t.Setenv(envPrefix+"KOMOOT__EMAIL", "komoot-rider@example.test")
	t.Setenv(envPrefix+"KOMOOT__PASSWORD", "komoot-password")

	settings, err := Load()
	require.NoError(t, err)

	assert.Nil(t, settings.VeloPlanner, "VeloPlanner")
	require.NotNil(t, settings.Komoot, "Komoot")
	assert.Equal(t, "https://komoot.example.test", settings.Komoot.BaseURL, "Komoot.BaseURL")
}

func TestLoadRefusesZeroSources(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	removeConfigurationLine(t, configPath, "[veloplanner]")
	removeConfigurationLine(t, configPath, "base_url = \"https://veloplanner.example.test\"")
	removeConfigurationLine(t, configPath, "email_file = ")
	removeConfigurationLine(t, configPath, "password_file = ")
	t.Setenv(configFileEnv, configPath)

	_, err := Load()
	require.ErrorContains(t, err, "at least one source")
}

func TestLoadRefusesADuplicateSourceSection(t *testing.T) {
	// TOML itself rejects a redefined table as a parse error, before any of
	// this package's own validation runs — this is a regression test for that
	// guarantee, not a Go-level duplicate check, since there is no such check
	// to exercise.
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	appendToFile(t, configPath, "\n[veloplanner]\nbase_url = \"https://veloplanner-2.example.test\"\n")
	t.Setenv(configFileEnv, configPath)

	_, err := Load()
	// The exact wording is the TOML library's own and could change with a
	// dependency bump; assert this package's stable wrapper and the name of
	// the duplicated table, not the library's precise phrasing.
	require.ErrorContains(t, err, "parsing configuration")
	require.ErrorContains(t, err, "veloplanner")
}

// appendValidKomootSection writes Komoot's two secret files into directory and
// appends a matching, valid [komoot] section to the configuration at path.
func appendValidKomootSection(t *testing.T, directory, path string) {
	t.Helper()

	emailPath := writeSecretFile(t, directory, "komoot-email", "komoot-rider@example.test")
	passwordPath := writeSecretFile(t, directory, "komoot-password", "komoot-password")
	appendToFile(t, path, fmt.Sprintf(`
[komoot]
base_url = "https://komoot.example.test"
email_file = %q
password_file = %q
`, emailPath, passwordPath))
}

func TestLoadCarriesThePinnedImageReferenceWithoutTreatingItAsASetting(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)
	reference := "ghcr.io/nobbs/domestique@sha256:" + strings.Repeat("ab", 32)
	t.Setenv(imageReferenceEnv, reference)

	// Every other DOMESTIQUE_ variable is a setting and an unknown one is fatal,
	// so this has to be consumed rather than left for Koanf to trip over. The
	// deployed compose file passes it, which means getting this wrong would stop
	// the service from starting at all.
	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, reference, settings.ImageReference, "ImageReference")
	// Taken out of the environment, so nothing later in startup can read a
	// deployment fact out of it by accident.
	_, found := os.LookupEnv(imageReferenceEnv)
	assert.Falsef(t, found, "%s is still set after Load()", imageReferenceEnv)
}

func TestLoadReportsNoImageReferenceWhenTheHostNamedNone(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Empty(t, settings.ImageReference, "a host that pinned no image must report none")
}

func TestLoadDefaultsToNoRideModelCoefficients(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Empty(t, settings.RideModel.CoefficientsFile, "ride model prediction is off until a coefficients file is named")
}

func TestLoadReadsTheRideModelCoefficientsFile(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	appendToFile(t, configPath, "\n[ridemodel]\ncoefficients_file = \"/etc/domestique/ridemodel.toml\"\n")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "/etc/domestique/ridemodel.toml", settings.RideModel.CoefficientsFile, "RideModel.CoefficientsFile")
}

func TestValidateRideModel(t *testing.T) {
	tests := []struct {
		name      string
		rideModel rawRideModel
		wantErr   bool
	}{
		{name: "no coefficients file at all"},
		{name: "an absolute path", rideModel: rawRideModel{CoefficientsFile: "/etc/domestique/ridemodel.toml"}},
		{name: "a relative path is rejected", rideModel: rawRideModel{CoefficientsFile: "ridemodel.toml"}, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateRideModel(test.rideModel)
			if test.wantErr {
				require.Errorf(t, err, "validateRideModel(%v)", test.rideModel)

				return
			}
			require.NoErrorf(t, err, "validateRideModel(%v)", test.rideModel)
		})
	}
}

func TestLoadDirectSecretWinsAndIsCleared(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	removeConfigurationLine(t, configPath, "email_file =")
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"VELOPLANNER__EMAIL", "environment@example.test")

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "environment@example.test", string(settings.VeloPlanner.Email().Bytes()), "VeloPlanner.Email()")
	_, found := os.LookupEnv(envPrefix + "VELOPLANNER__EMAIL")
	assert.False(t, found, "the direct secret environment value remains after Load()")
}

func TestLoadFileEnvironmentOverridesTOML(t *testing.T) {
	directory := t.TempDir()
	configPath, _ := writeValidConfiguration(t, directory)
	overridePath := writeSecretFile(t, directory, "overridden-user-key", "override-user-key")
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"NOTIFICATIONS__PUSHOVER__USER_KEY_FILE", overridePath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "override-user-key", string(settings.Notifications.Pushover.UserKey().Bytes()), "Pushover.UserKey()")
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, path string)
		want   string
	}{
		{
			name: "literal TOML secret",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "email_file = ", "email = \"not-allowed\"\n# email_file = ")
			},
			want: "literal secret",
		},
		{
			// The settings that moved into the database left the file schema
			// entirely. An upgraded deployment whose config.toml still names one
			// has to be told, because the value it holds is no longer read and a
			// silent start would run on whatever the database says instead.
			name: "a setting that moved to the database",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "initial_delay = \"1m\"", "initial_delay = \"1m\"\nempty_source_deletion = \"allow\"")
			},
			want: "empty_source_deletion",
		},
		{
			name: "unknown setting",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[unknown]\nsetting = true\n")
			},
			want: "decoding configuration",
		},
		{
			name: "wrong target count",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "\n[[wahoo.targets]]\nid = \"rider-a\"\n", "\n")
				replaceInFile(t, path, "\n[[wahoo.targets]]\nid = \"rider-b\"\n", "\n")
			},
			want: "between one and two",
		},
		{
			name: "a relative ride model coefficients file",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[ridemodel]\ncoefficients_file = \"ridemodel.toml\"\n")
			},
			want: "ridemodel.coefficients_file",
		},
		{
			name: "readiness address sharing the served port",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "listen_address = \":8080\"", "listen_address = \":8080\"\nreadiness_address = \":8080\"")
			},
			want: "must not be http.listen_address",
		},
		{
			name: "readiness address naming a host",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "listen_address = \":8080\"", "listen_address = \":8080\"\nreadiness_address = \"0.0.0.0:8081\"")
			},
			want: "http.readiness_address",
		},
		{
			name: "readiness address without a port",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "listen_address = \":8080\"", "listen_address = \":8080\"\nreadiness_address = \":0\"")
			},
			want: "valid port",
		},
		{
			name: "komoot missing email",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				passwordPath := writeSecretFile(t, filepath.Dir(path), "komoot-password", "komoot-password")
				appendToFile(t, path, fmt.Sprintf("\n[komoot]\nbase_url = \"https://komoot.example.test\"\npassword_file = %q\n", passwordPath))
			},
			want: "Komoot email is not configured",
		},
		{
			name: "komoot missing password",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				emailPath := writeSecretFile(t, filepath.Dir(path), "komoot-email", "komoot-rider@example.test")
				appendToFile(t, path, fmt.Sprintf("\n[komoot]\nbase_url = \"https://komoot.example.test\"\nemail_file = %q\n", emailPath))
			},
			want: "Komoot password is not configured",
		},
		{
			name: "komoot base_url not https",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				emailPath := writeSecretFile(t, filepath.Dir(path), "komoot-email", "komoot-rider@example.test")
				passwordPath := writeSecretFile(t, filepath.Dir(path), "komoot-password", "komoot-password")
				appendToFile(t, path, fmt.Sprintf("\n[komoot]\nbase_url = \"http://komoot.example.test\"\nemail_file = %q\npassword_file = %q\n", emailPath, passwordPath))
			},
			want: "komoot.base_url",
		},
		{
			name: "veloplanner base_url not https",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, `base_url = "https://veloplanner.example.test"`, `base_url = "http://veloplanner.example.test"`)
			},
			want: "veloplanner.base_url",
		},
		{
			// The adapter itself requires an origin with no path; a value that
			// merely parses as an absolute HTTPS URL must still be refused here,
			// or the failure would surface later at client construction instead.
			name: "veloplanner base_url carrying a path",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, `base_url = "https://veloplanner.example.test"`, `base_url = "https://veloplanner.example.test/user_routes"`)
			},
			want: "must be an origin",
		},
		{
			name: "komoot base_url carrying a path",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				emailPath := writeSecretFile(t, filepath.Dir(path), "komoot-email", "komoot-rider@example.test")
				passwordPath := writeSecretFile(t, filepath.Dir(path), "komoot-password", "komoot-password")
				appendToFile(t, path, fmt.Sprintf("\n[komoot]\nbase_url = \"https://komoot.example.test/v007\"\nemail_file = %q\npassword_file = %q\n", emailPath, passwordPath))
			},
			want: "must be an origin",
		},
		{
			name: "veloplanner missing password",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				removeConfigurationLine(t, path, "password_file = ")
			},
			want: "VeloPlanner password is not configured",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath, _ := writeValidConfiguration(t, t.TempDir())
			test.mutate(t, configPath)
			t.Setenv(configFileEnv, configPath)

			_, err := Load()
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestLoadAllowsOneWahooTarget(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, "\n[[wahoo.targets]]\nid = \"rider-b\"\n", "\n")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []Target{{ID: "rider-a"}}, settings.Wahoo.Targets())
}

func TestLoadDoesNotExposeSecretFilePath(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	secretPath := filepath.Join(t.TempDir(), "sensitive-secret-path")
	replaceInFile(t, configPath, `email_file = "`, `email_file = "`+secretPath)
	t.Setenv(configFileEnv, configPath)

	_, err := Load()
	require.Error(t, err, "an unreadable secret file must stop the service")
	assert.NotContains(t, err.Error(), secretPath, "Load() exposed the secret file path")
}

func TestLoadRejectsAmbiguousSecretEnvironment(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"WAHOO__CLIENT_SECRET", "direct-secret")
	t.Setenv(envPrefix+"WAHOO__CLIENT_SECRET_FILE", "/run/secrets/wahoo-client-secret")

	_, err := Load()
	require.ErrorContains(t, err, "both direct and file environment")
	assert.NotContains(t, err.Error(), "direct-secret", "Load() exposed the direct secret")
}

func TestLoadClearsDirectSecretsWhenValidationFails(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, `initial_delay = "1m"`, `initial_delay = "0s"`)
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"WAHOO__CLIENT_SECRET", "direct-client-secret")

	_, err := Load()
	require.ErrorContains(t, err, "sync.initial_delay must be positive")

	_, found := os.LookupEnv(envPrefix + "WAHOO__CLIENT_SECRET")
	assert.False(t, found, "the direct secret environment value remains after a failed Load()")
}

func writeValidConfiguration(t *testing.T, directory string) (configPath string, key [32]byte) {
	t.Helper()

	for index := range key {
		key[index] = byte(index + 1)
	}
	keyPath := writeSecretFile(t, directory, "state-key", base64.RawURLEncoding.EncodeToString(key[:]))
	emailPath := writeSecretFile(t, directory, "veloplanner-email", "rider@example.test")
	passwordPath := writeSecretFile(t, directory, "veloplanner-password", "password")
	clientSecretPath := writeSecretFile(t, directory, "wahoo-client-secret", "client-secret")
	applicationTokenPath := writeSecretFile(t, directory, "pushover-application-token", "application-token")
	userKeyPath := writeSecretFile(t, directory, "pushover-user-key", "user-key")

	contents := fmt.Sprintf(`
[http]
listen_address = ":8080"

[access.cloudflare]
team_domain = "example.cloudflareaccess.com"
application_aud = "aud-tag"
allowed_email = "rider@example.test"

[state]
database_path = %q
encryption_key_file = %q

[veloplanner]
base_url = "https://veloplanner.example.test"
email_file = %q
password_file = %q

[wahoo]
api_base_url = "https://api.sandbox.wahooligan.example.test"
oauth_base_url = "https://api.sandbox.wahooligan.example.test"
client_id = "client-id"
client_secret_file = %q
redirect_url = "https://domestique.example.ts.net/oauth/wahoo/callback"

[[wahoo.targets]]
id = "rider-a"

[[wahoo.targets]]
id = "rider-b"

[sync]
initial_delay = "1m"

[notifications.pushover]
application_token_file = %q
user_key_file = %q
`, filepath.Join(directory, "state.db"), keyPath, emailPath, passwordPath, clientSecretPath, applicationTokenPath, userKeyPath)
	configPath = filepath.Join(directory, "config.toml")
	require.NoErrorf(t, os.WriteFile(configPath, []byte(contents), 0o600), "WriteFile(%q)", configPath)

	return configPath, key
}

func writeSecretFile(t *testing.T, directory, name, value string) string {
	t.Helper()

	path := filepath.Join(directory, name)
	require.NoErrorf(t, os.WriteFile(path, []byte(value+"\n"), 0o600), "WriteFile(%q)", path)

	return path
}

func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()

	//nolint:gosec // The test passes only a path in its own temporary directory.
	contents, err := os.ReadFile(path)
	require.NoErrorf(t, err, "ReadFile(%q)", path)
	require.Containsf(t, string(contents), old, "the configuration has nothing to replace")

	updated := bytes.Replace(contents, []byte(old), []byte(replacement), 1)
	//nolint:gosec // The test passes only a path in its own temporary directory.
	require.NoErrorf(t, os.WriteFile(path, updated, 0o600), "WriteFile(%q)", path)
}

func appendToFile(t *testing.T, path, text string) {
	t.Helper()

	//nolint:gosec // The test passes only a path in its own temporary directory.
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	require.NoErrorf(t, err, "OpenFile(%q)", path)
	defer func() {
		assert.NoErrorf(t, file.Close(), "Close(%q)", path)
	}()

	_, err = file.WriteString(text)
	require.NoErrorf(t, err, "WriteString(%q)", path)
}

func removeConfigurationLine(t *testing.T, path, prefix string) {
	t.Helper()

	//nolint:gosec // The test passes only a path in its own temporary directory.
	contents, err := os.ReadFile(path)
	require.NoErrorf(t, err, "ReadFile(%q)", path)

	lines := strings.Split(string(contents), "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			copy(lines[index:], lines[index+1:])
			updated := lines[:len(lines)-1]
			//nolint:gosec // The test passes only a path in its own temporary directory.
			require.NoErrorf(t, os.WriteFile(path, []byte(strings.Join(updated, "\n")), 0o600),
				"WriteFile(%q)", path)

			return
		}
	}

	require.Failf(t, "nothing to remove", "the configuration has no line beginning with %q", prefix)
}

// Cloudflare Access is the only gate this service has. A configuration that
// cannot verify an assertion cannot authenticate anyone, so it is refused at
// startup rather than left to answer every request with a 401.
func TestLoadRequiresCloudflareAccess(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, `[access.cloudflare]
team_domain = "example.cloudflareaccess.com"
application_aud = "aud-tag"
allowed_email = "rider@example.test"
`, "")
	t.Setenv(configFileEnv, configPath)

	_, err := Load()
	require.ErrorContains(t, err, "access.cloudflare is required")
}

func TestLoadReadsCloudflareAccess(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "example.cloudflareaccess.com", settings.Access.Cloudflare.TeamDomain, "TeamDomain")
	assert.Equal(t, "aud-tag", settings.Access.Cloudflare.ApplicationAUD, "ApplicationAUD")
	assert.Equal(t, "rider@example.test", settings.Access.Cloudflare.AllowedEmail, "AllowedEmail")
}

// Each value carries its own weight: without the audience tag an assertion
// minted for any other application of the same team would verify, and without
// the allowed address any member of the team would be let in.
func TestLoadRejectsPartialCloudflareAccess(t *testing.T) {
	cases := map[string]string{
		"missing team domain": "team_domain = \"example.cloudflareaccess.com\"\n",
		"missing audience":    "application_aud = \"aud-tag\"\n",
		"missing email":       "allowed_email = \"rider@example.test\"\n",
	}

	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			configPath, _ := writeValidConfiguration(t, t.TempDir())
			replaceInFile(t, configPath, line, "")
			t.Setenv(configFileEnv, configPath)

			_, err := Load()
			require.ErrorContains(t, err, "access.cloudflare is required")
		})
	}
}

// An existing deployment's configuration file says nothing about readiness, so
// the probe has to arrive with a listener of its own rather than an empty one.
func TestLoadDefaultsTheReadinessListenerToItsOwnPort(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)

	assert.Equal(t, ":8081", settings.HTTP.ReadinessAddress, "HTTP.ReadinessAddress")
	assert.NotEqual(t, settings.HTTP.ListenAddress, settings.HTTP.ReadinessAddress,
		"the readiness listener must not be the served listener")
}

func TestLoadReadsAConfiguredReadinessListener(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, "listen_address = \":8080\"", "listen_address = \":8080\"\nreadiness_address = \":9101\"")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)

	assert.Equal(t, ":9101", settings.HTTP.ReadinessAddress, "HTTP.ReadinessAddress")
}
