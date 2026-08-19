package config

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The development configuration is written by a shell script, so nothing else
// notices when a schema change invalidates its shape: the first sign is
// `mise run dev-api` refusing to start. This loads exactly what the script
// generates, so that failure lands here instead.
func TestDevSetupGeneratesALoadableConfiguration(t *testing.T) {
	directory := t.TempDir()

	var key [32]byte
	for index := range key {
		key[index] = byte(index + 1)
	}

	values := map[string]string{
		"DEV_DIR":            directory,
		"DEV_SECRETS":        directory,
		"DEPLOYED_SECRETS":   directory,
		"CF_TEAM_DOMAIN":     "example.cloudflareaccess.com",
		"CF_APPLICATION_AUD": "aud-tag",
		"CF_ALLOWED_EMAIL":   "rider@example.test",
	}
	writeSecretFile(t, directory, "state_encryption_key", base64.RawURLEncoding.EncodeToString(key[:]))
	writeSecretFile(t, directory, "veloplanner_email", "rider@example.test")
	writeSecretFile(t, directory, "veloplanner_password", "password")
	writeSecretFile(t, directory, "wahoo_client_secret", "client-secret")
	writeSecretFile(t, directory, "pushover_application_token", "application-token")
	writeSecretFile(t, directory, "pushover_user_key", "user-key")

	configPath := filepath.Join(directory, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(devSetupConfiguration(t, values)), 0o600))
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)

	assert.Equal(t, values["CF_TEAM_DOMAIN"], settings.Access.Cloudflare.TeamDomain, "TeamDomain")
	assert.Equal(t, values["CF_APPLICATION_AUD"], settings.Access.Cloudflare.ApplicationAUD, "ApplicationAUD")
	assert.Equal(t, values["CF_ALLOWED_EMAIL"], settings.Access.Cloudflare.AllowedEmail, "AllowedEmail")
}

// devSetupConfiguration renders the configuration dev/setup.sh writes, taking
// the text from the script itself so the two cannot drift apart. A shell
// variable the script interpolates and this test does not supply fails here,
// which is the point: a new value in that file has to be considered.
func devSetupConfiguration(t *testing.T, values map[string]string) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	script, err := os.ReadFile(filepath.Join(root, "dev", "setup.sh")) //nolint:gosec // a repository file, named by this test
	require.NoError(t, err)

	heredoc := regexp.MustCompile(`(?s)cat > "\$\{DEV_DIR\}/config\.toml" <<EOF\n(.*?)\nEOF\n`).
		FindSubmatch(script)
	require.NotNil(t, heredoc, "dev/setup.sh no longer writes its configuration as a heredoc this test can find")

	return regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*\}`).
		ReplaceAllStringFunc(string(heredoc[1]), func(reference string) string {
			value, known := values[reference[2:len(reference)-1]]
			require.Truef(t, known, "dev/setup.sh interpolates %s, which this test does not supply", reference)

			return value
		})
}
