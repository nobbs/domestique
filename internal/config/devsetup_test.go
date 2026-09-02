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
		"DEV_DIR":          directory,
		"DEV_SECRETS":      directory,
		"DEPLOYED_SECRETS": directory,
		"DEV_SUBJECT":      "github|123456",
	}
	writeSecretFile(t, directory, "state_encryption_key", base64.RawURLEncoding.EncodeToString(key[:]))
	writeSecretFile(t, directory, "auth0_client_secret", "development-placeholder")

	configPath := filepath.Join(directory, "config.toml")
	require.NoError(t, os.WriteFile(configPath, []byte(devSetupConfiguration(t, values)), 0o600))
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)

	assert.Equal(t, []string{values["DEV_SUBJECT"]}, settings.Auth.Auth0.AllowedSubjects, "AllowedSubjects")
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
