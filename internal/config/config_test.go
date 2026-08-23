package config

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadUsesFileSecretsAndDefaults(t *testing.T) {
	configPath, key := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)

	assert.Equal(t, key, settings.State.EncryptionKey(), "State.EncryptionKey()")
	assert.Equal(t, EmptySourceDeletionDeny, settings.Sync.EmptySourceDeletion, "Sync.EmptySourceDeletion")
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
	require.ErrorContains(t, err, "already exists")
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

func TestLoadDefaultsToAKeylessTileStyle(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	// One entry: a deployment that configured no basemap gets a map, and the
	// page offers no choice because there is nothing to choose between.
	require.Len(t, settings.WebUI.Basemaps, 1, "WebUI.Basemaps")
	basemap := settings.WebUI.Basemaps[0]
	assert.Equal(t, defaultBasemapName, basemap.Name, "Basemap.Name")
	assert.Equal(t, defaultBasemapStyleURL, basemap.StyleURL, "Basemap.StyleURL")
	// The dark default is the same provider's other style, so a default
	// deployment follows the colour scheme without reaching a second origin.
	assert.Equal(t, defaultBasemapStyleURLDark, basemap.StyleURLDark, "Basemap.StyleURLDark")
	assert.Truef(t, sameOrigin(basemap.StyleURL, basemap.StyleURLDark),
		"default styles are on different origins: %q and %q", basemap.StyleURL, basemap.StyleURLDark)
	assert.False(t, basemap.DarkCartography, "the default cartography follows the colour scheme")
}

func TestLoadReadsAListOfBasemaps(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	appendToFile(t, configPath, `
[[webui.basemaps]]
name = "Streets"
style_url = "https://tiles.example.test/styles/bright"
style_url_dark = "https://tiles.example.test/styles/dark"

[[webui.basemaps]]
name = "Satellite"
style_url = "https://imagery.example.test/maps/hybrid/style.json?key=abc"
dark_cartography = true
`)
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	// The configured list replaces the default outright rather than extending
	// it, so what an operator wrote down is the whole of what the page offers.
	require.Len(t, settings.WebUI.Basemaps, 2, "WebUI.Basemaps")
	assert.Equal(t, "Streets", settings.WebUI.Basemaps[0].Name, "the first entry keeps its place")
	assert.Equal(t, "Satellite", settings.WebUI.Basemaps[1].Name, "the second entry keeps its place")
	assert.True(t, settings.WebUI.Basemaps[1].DarkCartography, "imagery is dark ground in either scheme")
}

// The old two-key shape is refused rather than quietly ignored: a deployment
// that upgrades without editing its file would otherwise keep the default map
// and never learn that its configured style went unread.
func TestLoadRefusesTheReplacedTileStyleKeys(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	appendToFile(t, configPath, "\n[webui]\ntile_style_url = \"https://tiles.example.test/styles/bright\"\n")
	t.Setenv(configFileEnv, configPath)

	_, err := Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tile_style_url", "the error names the key that is no longer read")
}

func TestValidateBasemaps(t *testing.T) {
	const light = "https://tiles.example.test/styles/liberty"

	tests := []struct {
		name    string
		wantErr string
		raw     []rawBasemap
	}{
		{
			name:    "an empty list leaves the map nothing to paint",
			raw:     nil,
			wantErr: "at least one entry",
		},
		{
			name:    "a nameless entry cannot be picked",
			raw:     []rawBasemap{{Name: "  ", StyleURL: light}},
			wantErr: "name is required",
		},
		{
			name: "a repeated name is two entries with one identity",
			raw: []rawBasemap{
				{Name: "Streets", StyleURL: light},
				{Name: "Streets", StyleURL: "https://other.example.test/style.json"},
			},
			wantErr: "duplicated",
		},
		{
			name:    "a style that is not an absolute HTTPS URL",
			raw:     []rawBasemap{{Name: "Streets", StyleURL: "http://tiles.example.test/style.json"}},
			wantErr: "webui.basemaps[0].style_url",
		},
		{
			name: "a dark twin on a second origin widens the policy",
			raw: []rawBasemap{
				{Name: "Streets", StyleURL: light, StyleURLDark: "https://dark.example.test/styles/dark"},
			},
			wantErr: "same origin",
		},
		{
			name: "dark cartography and a dark twin contradict each other",
			raw: []rawBasemap{
				{
					Name:            "Streets",
					StyleURL:        light,
					StyleURLDark:    "https://tiles.example.test/styles/dark",
					DarkCartography: true,
				},
			},
			wantErr: "must not set both",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := validateBasemaps(test.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestValidateBasemapsAcceptsAMixedList(t *testing.T) {
	basemaps, err := validateBasemaps([]rawBasemap{
		{
			Name:         "  Streets  ",
			StyleURL:     "  https://tiles.example.test/styles/bright  ",
			StyleURLDark: "  https://TILES.example.test/styles/dark  ",
		},
		{
			Name:            "Satellite",
			StyleURL:        "https://imagery.example.test/maps/hybrid/style.json?key=abc",
			DarkCartography: true,
		},
	})

	require.NoError(t, err)
	// Trimmed on the way in, because a hand-edited file carries whitespace and
	// what the page receives has to be the value that was checked.
	assert.Equal(t, "Streets", basemaps[0].Name)
	assert.Equal(t, "https://tiles.example.test/styles/bright", basemaps[0].StyleURL)
	assert.Equal(t, "https://TILES.example.test/styles/dark", basemaps[0].StyleURLDark)
	assert.True(t, basemaps[1].DarkCartography)
}

func TestValidateStyleURL(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "keyless default", value: defaultBasemapStyleURL},
		{name: "keyed provider query is permitted", value: "https://tiles.example.test/style.json?key=abc"},
		{name: "plaintext is rejected", value: "http://tiles.example.test/style.json", wantErr: true},
		//nolint:gosec // A rejection fixture for URL userinfo, not a real credential.
		{name: "credentials are rejected", value: "https://user:pass@tiles.example.test/s.json", wantErr: true},
		{name: "relative is rejected", value: "/styles/liberty", wantErr: true},
		{name: "empty is rejected", value: "", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateStyleURL("webui.basemaps[0].style_url", test.value)
			if test.wantErr {
				require.Errorf(t, err, "validateStyleURL(%q)", test.value)

				return
			}
			require.NoErrorf(t, err, "validateStyleURL(%q)", test.value)
		})
	}
}

func TestLoadDefaultsToNoRegionsAndAWeeklyRebuild(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Empty(t, settings.Surface.Regions, "surface classification is off until regions are named")
	assert.Equal(t, defaultRebuildInterval, settings.Surface.RebuildInterval, "Surface.RebuildInterval")
}

func TestLoadDefaultsToPushoversOwnOrigin(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, defaultPushoverURL, settings.Notifications.Pushover.BaseURL, "Notifications.Pushover.BaseURL")
}

// An operator who says nothing about notification volume keeps the per-run
// message this service has always sent. A quiet default would silently change
// how much a running deployment reports.
func TestLoadDefaultsToNotifyingEverySuccess(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, SuccessPolicyEvery, settings.Notifications.Success.Policy, "Notifications.Success.Policy")
	assert.Equal(t, defaultDigestInterval, settings.Notifications.Success.DigestInterval, "Notifications.Success.DigestInterval")
}

func TestLoadReadsAConfiguredSuccessPolicy(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	appendToFile(t, configPath, "\n[notifications]\nsuccess_policy = \"digest\"\ndigest_interval = \"12h\"\n")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, SuccessPolicyDigest, settings.Notifications.Success.Policy, "Notifications.Success.Policy")
	assert.Equal(t, 12*time.Hour, settings.Notifications.Success.DigestInterval, "Notifications.Success.DigestInterval")
}

// A development environment overrides the origin to keep a placeholder token off
// the real service, so an override has to survive loading intact.
func TestLoadKeepsAnOverriddenPushoverOrigin(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	replaceInFile(t, configPath, "[notifications.pushover]", "[notifications.pushover]\nbase_url = \"https://pushover.example.test\"")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "https://pushover.example.test", settings.Notifications.Pushover.BaseURL, "Notifications.Pushover.BaseURL")
}

// Regions are what an operator actually configures, so they have to survive
// loading in the order and shape they were written.
func TestLoadKeepsConfiguredRegions(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	appendToFile(t, configPath,
		"\n[surface]\nregions = [\"europe/germany/rheinland-pfalz\", \" europe/germany/hessen \"]\n"+
			"rebuild_interval = \"72h\"\n")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"europe/germany/rheinland-pfalz", "europe/germany/hessen"},
		settings.Surface.Regions,
		"surrounding whitespace is not part of a slug",
	)
	assert.Equal(t, 72*time.Hour, settings.Surface.RebuildInterval, "Surface.RebuildInterval")
}

// A region named twice is a typo that would otherwise be paid for twice, in
// download, decode, and index size, for an index that answers identically.
func TestLoadNamesEachRegionOnce(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	appendToFile(t, configPath,
		"\n[surface]\nregions = [\"europe/germany/rheinland-pfalz\", \"europe/germany/hessen\", "+
			"\" europe/germany/rheinland-pfalz \"]\nrebuild_interval = \"72h\"\n")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Equal(t,
		[]string{"europe/germany/rheinland-pfalz", "europe/germany/hessen"},
		settings.Surface.Regions,
		"a repeat is dropped and the first position kept",
	)
}

// An empty list is how an operator declines the whole feature, so it must load
// rather than fail as a missing setting.
func TestLoadTreatsNoRegionsAsDisabled(t *testing.T) {
	configPath, _ := writeValidConfiguration(t, t.TempDir())
	appendToFile(t, configPath, "\n[surface]\nregions = []\n")
	t.Setenv(configFileEnv, configPath)

	settings, err := Load()
	require.NoError(t, err)
	assert.Empty(t, settings.Surface.Regions, "no regions declines classification")
}

func TestValidateSurface(t *testing.T) {
	tests := []struct {
		name    string
		surface rawSurface
		wantErr bool
	}{
		{name: "no regions at all", surface: rawSurface{}},
		{
			name:    "a region path",
			surface: rawSurface{Regions: []string{"europe/germany/rheinland-pfalz"}, RebuildInterval: time.Hour},
		},
		{
			name:    "a top-level region",
			surface: rawSurface{Regions: []string{"antarctica"}, RebuildInterval: time.Hour},
		},
		{
			name:    "a traversal is rejected",
			surface: rawSurface{Regions: []string{"europe/../../etc/passwd"}, RebuildInterval: time.Hour},
			wantErr: true,
		},
		{
			name:    "an absolute URL is rejected",
			surface: rawSurface{Regions: []string{"https://example.test/x.osm.pbf"}, RebuildInterval: time.Hour},
			wantErr: true,
		},
		{
			name:    "uppercase is rejected",
			surface: rawSurface{Regions: []string{"europe/Germany"}, RebuildInterval: time.Hour},
			wantErr: true,
		},
		{
			name:    "a region without a cadence is rejected",
			surface: rawSurface{Regions: []string{"antarctica"}},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateSurface(test.surface)
			if test.wantErr {
				require.Errorf(t, err, "validateSurface(%v)", test.surface)

				return
			}
			require.NoErrorf(t, err, "validateSurface(%v)", test.surface)
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
			name: "a region that is not a region path",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[surface]\nregions = [\"../../etc/passwd\"]\n")
			},
			want: "surface.regions",
		},
		{
			name: "plaintext notification origin",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "[notifications.pushover]", "[notifications.pushover]\nbase_url = \"http://pushover.example.test\"")
			},
			want: "notifications.pushover.base_url",
		},
		{
			name: "notification origin carrying a path",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "[notifications.pushover]", "[notifications.pushover]\nbase_url = \"https://pushover.example.test/1/messages.json\"")
			},
			want: "must be an origin",
		},
		{
			name: "dark tile style on another origin",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[[webui.basemaps]]\nname = \"Streets\"\n"+
					"style_url = \"https://tiles.example.test/styles/bright\"\n"+
					"style_url_dark = \"https://dark.example.test/styles/dark\"\n")
			},
			want: "same origin",
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
			name: "non canonical schedule",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "interval = \"1h\"", "interval = \"2h\"")
			},
			want: "must equal 1h",
		},
		{
			name: "non-positive stale-after bound",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "max_deletions_per_target = 5", "max_deletions_per_target = 5\nstale_after = \"0s\"")
			},
			want: "sync.stale_after must be at least 1s",
		},
		{
			// Sub-second truncates to a zero max_age_seconds in the status
			// response, which would flag every service as permanently stale.
			name: "sub-second stale-after bound",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				replaceInFile(t, path, "max_deletions_per_target = 5", "max_deletions_per_target = 5\nstale_after = \"500ms\"")
			},
			want: "sync.stale_after must be at least 1s",
		},
		{
			name: "unknown success policy",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[notifications]\nsuccess_policy = \"silent\"\n")
			},
			want: "notifications.success_policy must be every, quiet, or digest",
		},
		{
			name: "digest policy without a period",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[notifications]\nsuccess_policy = \"digest\"\ndigest_interval = \"0s\"\n")
			},
			want: "notifications.digest_interval must be positive",
		},
		{
			// A period the recorded run history does not reach back over would
			// report a total missing every run already pruned from under it.
			name: "digest period beyond the recorded history",
			mutate: func(t *testing.T, path string) {
				t.Helper()
				appendToFile(t, path, "\n[notifications]\nsuccess_policy = \"digest\"\ndigest_interval = \"169h\"\n")
			},
			want: "notifications.digest_interval must not exceed 168h0m0s",
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
	replaceInFile(t, configPath, `interval = "1h"`, `interval = "2h"`)
	t.Setenv(configFileEnv, configPath)
	t.Setenv(envPrefix+"WAHOO__CLIENT_SECRET", "direct-client-secret")

	_, err := Load()
	require.ErrorContains(t, err, "must equal 1h")

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
interval = "1h"
max_deletions_per_target = 5

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
