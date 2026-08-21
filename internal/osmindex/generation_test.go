package osmindex

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A slug is operator configuration that becomes a URL, so the shape is checked
// rather than trusted.
func TestValidateRegion(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		wantErr bool
	}{
		{name: "a region path", slug: "europe/germany/rheinland-pfalz"},
		{name: "a top-level region", slug: "antarctica"},
		{name: "digits in a segment", slug: "us/california-2"},
		{name: "empty", slug: "", wantErr: true},
		{name: "a leading slash", slug: "/europe/germany", wantErr: true},
		{name: "a trailing slash", slug: "europe/germany/", wantErr: true},
		{name: "a traversal", slug: "europe/../../etc/passwd", wantErr: true},
		{name: "an absolute URL", slug: "https://example.test/x.osm.pbf", wantErr: true},
		{name: "a query", slug: "europe/germany?x=1", wantErr: true},
		{name: "uppercase", slug: "europe/Germany", wantErr: true},
		{name: "an underscore", slug: "europe/rheinland_pfalz", wantErr: true},
		{name: "a doubled hyphen", slug: "europe/rheinland--pfalz", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateRegion(test.slug)
			if test.wantErr {
				require.Errorf(t, err, "ValidateRegion(%q)", test.slug)

				return
			}
			require.NoErrorf(t, err, "ValidateRegion(%q)", test.slug)
		})
	}
}

// The generation is what decides whether a scheduled rebuild has anything to do,
// so it has to be a function of the bytes that would be read and of nothing
// else — not of the order the operator happened to write the regions in.
func TestGenerationIgnoresRegionOrderAndFollowsTheChecksums(t *testing.T) {
	first := generationOf(map[string]string{"europe/germany": "aaaa", "europe/france": "bbbb"})
	same := generationOf(map[string]string{"europe/france": "bbbb", "europe/germany": "aaaa"})
	assert.Equal(t, first, same, "the same regions in a different order")

	republished := generationOf(map[string]string{"europe/germany": "cccc", "europe/france": "bbbb"})
	assert.NotEqual(t, first, republished, "one region's extract was republished")

	added := generationOf(map[string]string{
		"europe/germany": "aaaa", "europe/france": "bbbb", "europe/spain": "dddd",
	})
	assert.NotEqual(t, first, added, "a region was added")

	assert.Len(t, first, 12, "a generation is what goes in a filename and onto a status page")
}

func TestRegionsRoundTripThroughIndexMetadata(t *testing.T) {
	regions := []string{"europe/germany/rheinland-pfalz", "europe/germany/hessen"}
	assert.Equal(t, regions, splitRegions(joinRegions(regions)), "regions through the metadata row")
	assert.Nil(t, splitRegions(""), "an index built from no regions")
	assert.Nil(t, splitRegions("   "), "a metadata row holding only whitespace")
}
