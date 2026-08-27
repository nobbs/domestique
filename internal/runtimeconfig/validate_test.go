package runtimeconfig

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateBasemaps(t *testing.T) {
	const light = "https://tiles.example.test/styles/liberty"

	tests := []struct {
		name    string
		wantErr string
		raw     []Basemap
	}{
		{
			name:    "an empty list leaves the map nothing to paint",
			raw:     nil,
			wantErr: "at least one entry",
		},
		{
			name:    "a nameless entry cannot be picked",
			raw:     []Basemap{{Name: "  ", StyleURL: light}},
			wantErr: "name is required",
		},
		{
			name: "a repeated name is two entries with one identity",
			raw: []Basemap{
				{Name: "Streets", StyleURL: light},
				{Name: "Streets", StyleURL: "https://other.example.test/style.json"},
			},
			wantErr: "duplicated",
		},
		{
			name:    "a style that is not an absolute HTTPS URL",
			raw:     []Basemap{{Name: "Streets", StyleURL: "http://tiles.example.test/style.json"}},
			wantErr: "webui.basemaps[0].style_url",
		},
		{
			name: "a dark twin on a second origin widens the policy",
			raw: []Basemap{
				{Name: "Streets", StyleURL: light, StyleURLDark: "https://dark.example.test/styles/dark"},
			},
			wantErr: "same origin",
		},
		{
			name: "dark cartography and a dark twin contradict each other",
			raw: []Basemap{
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
			_, err := ValidateBasemaps(test.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}

func TestValidateBasemapsAcceptsAMixedList(t *testing.T) {
	basemaps, err := ValidateBasemaps([]Basemap{
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
		{name: "keyless default", value: "https://tiles.openfreemap.org/styles/bright"},
		{name: "keyed provider query is permitted", value: "https://tiles.example.test/style.json?key=abc"},
		{name: "plaintext is rejected", value: "http://tiles.example.test/style.json", wantErr: true},
		//nolint:gosec // A rejection fixture for URL userinfo, not a real credential.
		{name: "credentials are rejected", value: "https://user:pass@tiles.example.test/s.json", wantErr: true},
		{name: "relative is rejected", value: "/styles/liberty", wantErr: true},
		{name: "empty is rejected", value: "", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateStyleURL("webui.basemaps[0].style_url", test.value)
			if test.wantErr {
				require.Errorf(t, err, "ValidateStyleURL(%q)", test.value)

				return
			}
			require.NoErrorf(t, err, "ValidateStyleURL(%q)", test.value)
		})
	}
}

func TestValidateSurface(t *testing.T) {
	tests := []struct {
		name    string
		surface Surface
		wantErr bool
	}{
		{name: "no regions at all", surface: Surface{RebuildInterval: 7 * 24 * time.Hour}},
		{name: "no cadence at all is rejected", surface: Surface{}, wantErr: true},
		{
			name:    "a region path",
			surface: Surface{Regions: []string{"europe/germany/rheinland-pfalz"}, RebuildInterval: time.Hour},
		},
		{
			name:    "a top-level region",
			surface: Surface{Regions: []string{"antarctica"}, RebuildInterval: time.Hour},
		},
		{
			name:    "a traversal is rejected",
			surface: Surface{Regions: []string{"europe/../../etc/passwd"}, RebuildInterval: time.Hour},
			wantErr: true,
		},
		{
			name:    "an absolute URL is rejected",
			surface: Surface{Regions: []string{"https://example.test/x.osm.pbf"}, RebuildInterval: time.Hour},
			wantErr: true,
		},
		{
			name:    "uppercase is rejected",
			surface: Surface{Regions: []string{"europe/Germany"}, RebuildInterval: time.Hour},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ValidateSurface(test.surface)
			if test.wantErr {
				require.Errorf(t, err, "ValidateSurface(%v)", test.surface)

				return
			}
			require.NoErrorf(t, err, "ValidateSurface(%v)", test.surface)
		})
	}
}
