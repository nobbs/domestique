package config

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadUsesFileSecretsAndDefaults(t *testing.T) {
	configPath, key := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := settings.State.EncryptionKey(), key; got != want {
		t.Errorf("State.EncryptionKey() = %v, want %v", got, want)
	}
	if got, want := settings.Sync.EmptySourceDeletion, EmptySourceDeletionDeny; got != want {
		t.Errorf("Sync.EmptySourceDeletion = %q, want %q", got, want)
	}
	if got, want := settings.Wahoo.Targets(), []Target{{ID: "rider-a"}, {ID: "rider-b"}}; !sameTargets(got, want) {
		t.Errorf("Wahoo.Targets() = %#v, want %#v", got, want)
	}
	if got, want := string(settings.VeloPlanner.Email().Bytes()), "rider@example.test"; got != want {
		t.Errorf("VeloPlanner.Email() = %q, want %q", got, want)
	}
}

func TestLoadDefaultsToAKeylessTileStyle(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := settings.WebUI.TileStyleURL, defaultTileStyleURL; got != want {
		t.Errorf("WebUI.TileStyleURL = %q, want %q", got, want)
	}
}

func TestValidateTileStyleURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "keyless default", value: defaultTileStyleURL},
		{name: "keyed provider query is permitted", value: "https://tiles.example.test/style.json?key=abc"},
		{name: "plaintext is rejected", value: "http://tiles.example.test/style.json", wantErr: true},
		//nolint:gosec // A rejection fixture for URL userinfo, not a real credential.
		{name: "credentials are rejected", value: "https://user:pass@tiles.example.test/s.json", wantErr: true},
		{name: "relative is rejected", value: "/styles/liberty", wantErr: true},
		{name: "empty is rejected", value: "", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateTileStyleURL(test.value)
			if (err != nil) != test.wantErr {
				t.Errorf("validateTileStyleURL(%q) error = %v, wantErr %v", test.value, err, test.wantErr)
			}
		})
	}
}

func TestLoadDefaultsToThePublicOverpassEndpoint(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := settings.Surface.OverpassURL, defaultOverpassURL; got != want {
		t.Errorf("Surface.OverpassURL = %q, want %q", got, want)
	}
}

// An empty endpoint is how an operator declines to send stage shapes anywhere,
// so it must load rather than fail as a missing setting.
func TestLoadTreatsAnEmptyOverpassEndpointAsDisabled(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	appendToFile(t, configPath, "\n[surface]\noverpass_url = \"\"\n")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got := settings.Surface.OverpassURL; got != "" {
		t.Errorf("Surface.OverpassURL = %q, want an empty endpoint", got)
	}
}

func TestValidateOverpassURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "public default", value: defaultOverpassURL},
		{name: "self-hosted instance", value: "https://overpass.example.test/api/interpreter"},
		{name: "empty disables the lookup", value: ""},
		{name: "plaintext is rejected", value: "http://overpass.example.test/api/interpreter", wantErr: true},
		//nolint:gosec // A rejection fixture for URL userinfo, not a real credential.
		{name: "credentials are rejected", value: "https://user:pass@overpass.example.test/api", wantErr: true},
		{name: "relative is rejected", value: "/api/interpreter", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateOverpassURL(test.value)
			if (err != nil) != test.wantErr {
				t.Errorf("validateOverpassURL(%q) error = %v, wantErr %v", test.value, err, test.wantErr)
			}
		})
	}
}

func TestLoadDirectSecretWinsAndIsCleared(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	removeConfigurationLine(t, configPath, "email_file =")
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"VELOPLANNER__EMAIL", "environment@example.test")

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := string(settings.VeloPlanner.Email().Bytes()), "environment@example.test"; got != want {
		t.Errorf("VeloPlanner.Email() = %q, want %q", got, want)
	}
	if _, found := os.LookupEnv(envPrefix + "VELOPLANNER__EMAIL"); found {
		t.Error("direct secret environment value remains after Load()")
	}
}

func TestLoadFileEnvironmentOverridesTOML(t *testing.T) {
	directory := t.TempDir()
	configPath, _ := writeValidConfiguration(t, directory)
	overridePath := writeSecretFile(t, directory, "overridden-user-key", "override-user-key")
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"NOTIFICATIONS__PUSHOVER__USER_KEY_FILE", overridePath)

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := string(settings.Notifications.Pushover.UserKey().Bytes()), "override-user-key"; got != want {
		t.Errorf("Pushover.UserKey() = %q, want %q", got, want)
	}
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
			name: "plaintext surface endpoint",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[surface]\noverpass_url = \"http://overpass.example.test/api\"\n")
			},
			want: "surface.overpass_url",
		},
		{
			name: "non canonical schedule",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "interval = \"1h\"", "interval = \"2h\"")
			},
			want: "must equal 1h",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath, _ := writeValidConfiguration(t, t.TempDir())
			test.mutate(t, configPath)
			t.Setenv(configFileEnv, configPath)

			_, err := Load()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Load() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestLoadAllowsOneWahooTarget(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, "\n[[wahoo.targets]]\nid = \"rider-b\"\n", "\n")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got, want := settings.Wahoo.Targets(), []Target{{ID: "rider-a"}}; !sameTargets(got, want) {
		t.Errorf("Wahoo.Targets() = %#v, want %#v", got, want)
	}
}

func TestLoadDoesNotExposeSecretFilePath(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	secretPath := filepath.Join(t.TempDir(), "sensitive-secret-path")
	replaceInFile(t, configPath, `email_file = "`, `email_file = "`+secretPath)
	t.Setenv(configFileEnv, configPath)

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want unavailable secret file error")
	}
	if strings.Contains(err.Error(), secretPath) {
		t.Errorf("Load() error exposes secret file path: %v", err)
	}
}

func TestLoadRejectsAmbiguousSecretEnvironment(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"WAHOO__CLIENT_SECRET", "direct-secret")
	t.Setenv(envPrefix+"WAHOO__CLIENT_SECRET_FILE", "/run/secrets/wahoo-client-secret")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "both direct and file environment") {
		t.Fatalf("Load() error = %v, want ambiguous secret input error", err)
	}
	if strings.Contains(err.Error(), "direct-secret") {
		t.Errorf("Load() error exposes direct secret: %v", err)
	}
}

func TestLoadClearsDirectSecretsWhenValidationFails(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, `interval = "1h"`, `interval = "2h"`)
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"WAHOO__CLIENT_SECRET", "direct-client-secret")

	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "must equal 1h") {
		t.Fatalf("Load() error = %v, want invalid interval error", err)
	}
	if _, found := os.LookupEnv(envPrefix + "WAHOO__CLIENT_SECRET"); found {
		t.Error("direct secret environment value remains after failed Load()")
	}
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

[access]
tailnet_user_login = "rider@example.ts.net"

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
interval = "1h"
max_deletions_per_target = 5

[notifications.pushover]
application_token_file = %q
user_key_file = %q
`, filepath.Join(directory, "state.db"), keyPath, emailPath, passwordPath, clientSecretPath, applicationTokenPath, userKeyPath)
	configPath = filepath.Join(directory, "config.toml")
	if err := os.WriteFile(configPath, []byte(contents), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", configPath, err)
	}

	return configPath, key
}

func writeSecretFile(t *testing.T, directory, name, value string) string {
	t.Helper()

	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(value+"\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}

	return path
}

func replaceInFile(t *testing.T, path, old, replacement string) {
	t.Helper()

	//nolint:gosec // The test passes only a path in its own temporary directory.
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	if !bytes.Contains(contents, []byte(old)) {
		t.Fatalf("configuration does not contain %q", old)
	}
	updated := bytes.Replace(contents, []byte(old), []byte(replacement), 1)
	//nolint:gosec // The test passes only a path in its own temporary directory.
	if err := os.WriteFile(path, updated, 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func appendToFile(t *testing.T, path, text string) {
	t.Helper()

	//nolint:gosec // The test passes only a path in its own temporary directory.
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("OpenFile(%q): %v", path, err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("Close(%q): %v", path, err)
		}
	}()
	if _, err := file.WriteString(text); err != nil {
		t.Fatalf("WriteString(%q): %v", path, err)
	}
}

func removeConfigurationLine(t *testing.T, path, prefix string) {
	t.Helper()

	//nolint:gosec // The test passes only a path in its own temporary directory.
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	lines := strings.Split(string(contents), "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			copy(lines[index:], lines[index+1:])
			updated := lines[:len(lines)-1]
			//nolint:gosec // The test passes only a path in its own temporary directory.
			if err := os.WriteFile(path, []byte(strings.Join(updated, "\n")), 0o600); err != nil {
				t.Fatalf("WriteFile(%q): %v", path, err)
			}
			return
		}
	}

	t.Fatalf("configuration does not contain a line beginning with %q", prefix)
}

func sameTargets(left, right []Target) bool {
	return slices.Equal(left, right)
}
