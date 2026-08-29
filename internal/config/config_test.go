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
	assert.Equal(t, testBrowserOriginURL, settings.HTTP.BrowserOriginURL, "HTTP.BrowserOriginURL")
}

// The origin is the gate every state-changing request is checked against, and
// the one Wahoo sends an authorization back to. A trailing slash is the same
// origin spelled differently, and would otherwise reach both as a URL nothing
// matches.
func TestLoadDropsATrailingSlashFromTheBrowserOrigin(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, testBrowserOriginURL+`"`, testBrowserOriginURL+`/"`)
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, testBrowserOriginURL, settings.HTTP.BrowserOriginURL, "HTTP.BrowserOriginURL")
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

func TestLoadDirectSecretWinsAndIsCleared(t *testing.T) {
	directory := t.TempDir()
	configPath, _ := writeValidConfiguration(t, directory)
	removeConfigurationLine(t, configPath, "encryption_key_file =")
	t.Setenv(configFileEnv, configPath)
	var key [32]byte
	for index := range key {
		key[index] = byte(index + 9)
	}
	t.Setenv(envPrefix+"STATE__ENCRYPTION_KEY", base64.RawURLEncoding.EncodeToString(key[:]))

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, key, settings.State.EncryptionKey(), "State.EncryptionKey()")
	_, found := os.LookupEnv(envPrefix + "STATE__ENCRYPTION_KEY")
	assert.False(t, found, "the direct secret environment value remains after Load()")
}

func TestLoadFileEnvironmentOverridesTOML(t *testing.T) {
	directory := t.TempDir()
	configPath, _ := writeValidConfiguration(t, directory)
	var key [32]byte
	for index := range key {
		key[index] = byte(index + 17)
	}
	overridePath := writeSecretFile(t, directory, "overridden-state-key", base64.RawURLEncoding.EncodeToString(key[:]))
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"STATE__ENCRYPTION_KEY_FILE", overridePath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, key, settings.State.EncryptionKey(), "State.EncryptionKey()")
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
				replaceInFile(t, path, "encryption_key_file = ", "encryption_key = \"not-allowed\"\n# encryption_key_file = ")
			},
			want: "literal secret",
		},
		{
			// Everything but the listeners, the identity gate and the state file
			// moved into the database. A config.toml still naming one has to be
			// told: the value is no longer read, and a silent start would run on
			// whatever the database says.
			name: "a setting that moved to the database",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[wahoo]\nclient_id = \"client-id\"\n")
			},
			want: "wahoo",
		},
		{
			name: "a source that moved to the database",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[veloplanner]\nbase_url = \"https://veloplanner.example.test\"\n")
			},
			want: "veloplanner",
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
			name: "no browser origin",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				removeConfigurationLine(t, path, "browser_origin_url = ")
			},
			want: "http.browser_origin_url",
		},
		{
			name: "a browser origin that is not https",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, testBrowserOriginURL, "http://domestique.example.ts.net")
			},
			want: "http.browser_origin_url",
		},
		{
			// The callback path is appended to it, and every state-changing
			// request is compared against it, so a value carrying a path of its
			// own is neither.
			name: "a browser origin carrying a path",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, testBrowserOriginURL, testBrowserOriginURL+"/domestique")
			},
			want: "must be an origin",
		},
		{
			name: "a relative state database path",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "database_path = \"", "database_path = \"state/")
			},
			want: "state.database_path",
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

func TestLoadDoesNotExposeSecretFilePath(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	secretPath := filepath.Join(t.TempDir(), "sensitive-secret-path")
	replaceInFile(t, configPath, `encryption_key_file = "`, `encryption_key_file = "`+secretPath)
	t.Setenv(configFileEnv, configPath)

	_, err := Load()
	require.Error(t, err, "an unreadable secret file must stop the service")
	assert.NotContains(t, err.Error(), secretPath, "Load() exposed the secret file path")
}

func TestLoadRejectsAmbiguousSecretEnvironment(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"STATE__ENCRYPTION_KEY", "direct-secret")
	t.Setenv(envPrefix+"STATE__ENCRYPTION_KEY_FILE", "/run/secrets/state-key")

	_, err := Load()
	require.ErrorContains(t, err, "both direct and file environment")
	assert.NotContains(t, err.Error(), "direct-secret", "Load() exposed the direct secret")
}

func TestLoadClearsDirectSecretsWhenValidationFails(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, `listen_address = ":8080"`, `listen_address = "8080"`)
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"STATE__ENCRYPTION_KEY", "direct-state-key")

	_, err := Load()
	require.ErrorContains(t, err, "http.listen_address")

	_, found := os.LookupEnv(envPrefix + "STATE__ENCRYPTION_KEY")
	assert.False(t, found, "the direct secret environment value remains after a failed Load()")
}

const testBrowserOriginURL = "https://domestique.example.ts.net"

func writeValidConfiguration(t *testing.T, directory string) (configPath string, key [32]byte) {
	t.Helper()

	for index := range key {
		key[index] = byte(index + 1)
	}
	keyPath := writeSecretFile(t, directory, "state-key", base64.RawURLEncoding.EncodeToString(key[:]))

	contents := fmt.Sprintf(`
[http]
listen_address = ":8080"
browser_origin_url = %q

[access.cloudflare]
team_domain = "example.cloudflareaccess.com"
application_aud = "aud-tag"
allowed_email = "rider@example.test"

[state]
database_path = %q
encryption_key_file = %q
`, testBrowserOriginURL, filepath.Join(directory, "state.db"), keyPath)
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
	replaceInFile(t, configPath, "\n[access.cloudflare]\n", "\n[access.cloudflare]\n# ")
	t.Setenv(configFileEnv, configPath)

	_, err := Load()
	require.ErrorContains(t, err, "access.cloudflare.team_domain")
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

func TestLoadRejectsPartialCloudflareAccess(t *testing.T) {
	for prefix, want := range map[string]string{
		"team_domain = ":     "access.cloudflare.team_domain",
		"application_aud = ": "access.cloudflare.application_aud",
		"allowed_email = ":   "access.cloudflare.allowed_email",
	} {
		t.Run(prefix, func(t *testing.T) {
			configPath, _ := writeValidConfiguration(t, t.TempDir())
			removeConfigurationLine(t, configPath, prefix)
			t.Setenv(configFileEnv, configPath)

			_, err := Load()
			require.ErrorContains(t, err, want)
		})
	}
}

func TestLoadDefaultsTheReadinessListenerToItsOwnPort(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, defaultReadinessAddress, settings.HTTP.ReadinessAddress, "HTTP.ReadinessAddress")
}

func TestLoadReadsAConfiguredReadinessListener(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, `listen_address = ":8080"`, "listen_address = \":8080\"\nreadiness_address = \":9090\"")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, ":9090", settings.HTTP.ReadinessAddress, "HTTP.ReadinessAddress")
}
