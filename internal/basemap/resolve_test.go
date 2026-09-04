package basemap

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const styleURL = "https://styles.example.test/bright?key=secret"

// documents serves canned JSON for a URL, keyed without its query so a keyed
// style URL is written once. A URL it does not hold answers 404, which is how
// an unreachable provider is spelled here.
type documents struct {
	bodies   map[string]string
	requests []string
	mu       sync.Mutex
}

func (d *documents) RoundTrip(request *http.Request) (*http.Response, error) {
	address := request.URL.Scheme + "://" + request.URL.Host + request.URL.Path
	d.mu.Lock()
	body, found := d.bodies[address]
	d.requests = append(d.requests, address)
	d.mu.Unlock()
	status := http.StatusOK
	if !found {
		status, body = http.StatusNotFound, "{}"
	}

	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Request:    request,
	}, nil
}

func newResolver(t *testing.T, bodies map[string]string, styles func() []string) (*Resolver, *documents) {
	t.Helper()
	served := &documents{bodies: bodies}
	resolver, err := NewResolver(Options{Styles: styles, Transport: served})
	require.NoError(t, err, "NewResolver()")

	return resolver, served
}

func fixedStyles(urls ...string) func() []string {
	return func() []string { return urls }
}

func TestResolverNamesTheHostsAStyleReachesBeyondItsOwn(t *testing.T) {
	resolver, _ := newResolver(t, map[string]string{
		"https://styles.example.test/bright": `{
			"glyphs": "https://fonts.example.test/{fontstack}/{range}.pbf",
			"sprite": "https://sprites.example.test/bright",
			"sources": {
				"terrain": {"tiles": ["https://terrain.example.test/{z}/{x}/{y}.png"]},
				"streets": {"url": "https://catalogue.example.test/streets.json"}
			}
		}`,
		"https://catalogue.example.test/streets.json": `{
			"tiles": ["https://tiles.example.test/streets/{z}/{x}/{y}.pbf"]
		}`,
	}, fixedStyles(styleURL))

	resolver.Refresh(context.Background())

	// The TileJSON's own host and the tile host it names are both here: a
	// provider that splits them needs both admitted, and following the
	// indirection is the only way the second is ever seen.
	assert.Equal(t, []string{
		"https://catalogue.example.test",
		"https://fonts.example.test",
		"https://sprites.example.test",
		"https://terrain.example.test",
		"https://tiles.example.test",
	}, resolver.Origins(), "the origins a style names")
}

func TestResolverKeepsOnlyReferencesABrowserCouldFollow(t *testing.T) {
	resolver, _ := newResolver(t, map[string]string{
		"https://styles.example.test/bright": `{
			"glyphs": "/fonts/{fontstack}/{range}.pbf",
			"sprite": [
				{"id": "default", "url": "https://sprites.example.test/bright"},
				{"id": "shields", "url": "mapbox://sprites/shields"}
			],
			"sources": {
				"insecure": {"tiles": ["http://plain.example.test/{z}/{x}/{y}.pbf"]},
				"inline": {"data": {"type": "FeatureCollection", "features": []}},
				"remote": {"data": "https://shapes.example.test/routes.geojson"}
			}
		}`,
	}, fixedStyles(styleURL))

	resolver.Refresh(context.Background())

	// The relative glyph URL resolves onto the style's own host; the mapbox://
	// sprite, the plain-HTTP tiles, and the inline GeoJSON name no origin the
	// page could reach and are dropped rather than written into a policy.
	assert.Equal(t, []string{
		"https://shapes.example.test",
		"https://sprites.example.test",
		"https://styles.example.test",
	}, resolver.Origins(), "the origins a style names")
}

func TestResolverForgetsAStyleThatIsNoLongerConfigured(t *testing.T) {
	configured := []string{styleURL}
	resolver, _ := newResolver(t, map[string]string{
		"https://styles.example.test/bright": `{"glyphs": "https://fonts.example.test/{fontstack}/{range}.pbf"}`,
	}, func() []string { return configured })

	resolver.Refresh(context.Background())
	require.NotEmpty(t, resolver.Origins(), "the style's origins after a read")

	// Removing the basemap stops its origins being named by the next response,
	// without waiting for the read that will prune the cache.
	configured = nil
	assert.Empty(t, resolver.Origins(), "a removed basemap's origins")
}

func TestResolverHoldsWhatItKnowsWhenAProviderCannotBeRead(t *testing.T) {
	bodies := map[string]string{
		"https://styles.example.test/bright": `{"glyphs": "https://fonts.example.test/{fontstack}/{range}.pbf"}`,
	}
	resolver, served := newResolver(t, bodies, fixedStyles(styleURL))
	resolver.Refresh(context.Background())
	require.Equal(t, []string{"https://fonts.example.test"}, resolver.Origins(), "the origins a style names")

	served.mu.Lock()
	delete(served.bodies, "https://styles.example.test/bright")
	served.mu.Unlock()
	resolver.Refresh(context.Background())

	// Dropping them would blank a map that is working for as long as the
	// provider is down, which is the failure this component exists to prevent.
	assert.Equal(t, []string{"https://fonts.example.test"}, resolver.Origins(),
		"a provider that cannot be read must not cost the policy what it already knew")
}

func TestResolverReadsAStyleSharedByTwoBasemapsOnce(t *testing.T) {
	resolver, served := newResolver(t, map[string]string{
		"https://styles.example.test/bright": `{"glyphs": "https://fonts.example.test/{fontstack}/{range}.pbf"}`,
	}, fixedStyles(styleURL, styleURL))

	resolver.Refresh(context.Background())

	assert.Equal(t, []string{"https://styles.example.test/bright"}, served.requests, "the documents read")
}

func TestNewResolverRefusesAnUnknownStyleList(t *testing.T) {
	_, err := NewResolver(Options{})
	assert.Error(t, err, "NewResolver() without the configured styles")
}
