package basemap

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

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

// refusing answers every request with a transport failure, as a provider whose
// host does not resolve does.
type refusing struct{}

func (refusing) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("no route to host")
}

func TestResolverReadsOnEveryTickUntilItIsStopped(t *testing.T) {
	served := &documents{bodies: map[string]string{
		"https://styles.example.test/bright": `{"glyphs": "https://fonts.example.test/{fontstack}/{range}.pbf"}`,
	}}
	resolver, err := NewResolver(Options{
		Styles:    fixedStyles(styleURL),
		Transport: served,
		Timeout:   time.Second,
		Interval:  10 * time.Millisecond,
	})
	require.NoError(t, err, "NewResolver()")

	ctx, cancel := context.WithCancel(t.Context())
	stopped := make(chan struct{})
	go func() { defer close(stopped); resolver.Run(ctx) }()

	// The first read happens before the first tick, so a deployment does not
	// serve a policy missing a style's hosts for a whole interval after start.
	require.Eventually(t, func() bool { return len(resolver.Origins()) == 1 }, time.Second, 5*time.Millisecond,
		"the styles were not read at start")
	require.Eventually(t, func() bool {
		served.mu.Lock()
		defer served.mu.Unlock()

		return len(served.requests) > 1
	}, time.Second, 5*time.Millisecond, "the styles were not read again on the interval")

	cancel()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Run() did not return when its context was cancelled")
	}
}

// redirecting sends every request on to plain HTTP, as a provider with a
// misconfigured edge does.
type redirecting struct{}

func (redirecting) RoundTrip(request *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{"http://styles.example.test/bright"}},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    request,
	}, nil
}

func TestResolverNamesNothingForAStyleItCannotRead(t *testing.T) {
	tests := []struct {
		name      string
		transport http.RoundTripper
		bodies    map[string]string
		style     string
	}{
		{name: "not a URL", style: "://tiles", bodies: map[string]string{}},
		{name: "not HTTPS", style: "http://styles.example.test/bright", bodies: map[string]string{}},
		{name: "unreachable", style: styleURL, transport: refusing{}},
		// A policy naming an origin the page could not reach from an HTTPS
		// document would admit nothing and hide the provider's misconfiguration.
		{name: "redirected off HTTPS", style: styleURL, transport: redirecting{}},
		{name: "not found", style: styleURL, bodies: map[string]string{}},
		{
			name:   "not a document",
			style:  styleURL,
			bodies: map[string]string{"https://styles.example.test/bright": "<html>rate limited</html>"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			transport := test.transport
			if transport == nil {
				transport = &documents{bodies: test.bodies}
			}
			resolver, err := NewResolver(Options{Styles: fixedStyles(test.style), Transport: transport})
			require.NoError(t, err, "NewResolver()")

			resolver.Refresh(t.Context())
			assert.Empty(t, resolver.Origins(), "a style that cannot be read must name no origin")
		})
	}
}

// A style is a third party's document: whatever shape its fields arrive in, the
// hosts that are readable are still read and the rest is passed over.
func TestResolverPassesOverThePartsOfAStyleItCannotRead(t *testing.T) {
	resolver, _ := newResolver(t, map[string]string{
		"https://styles.example.test/bright": `{
			"glyphs": "https://fonts.example.test/{fontstack}/{range}.pbf",
			"sprite": 7,
			"sources": {
				"broken": "not an object",
				"catalogued": {"url": "https://catalogue.example.test/streets.json"}
			}
		}`,
		"https://catalogue.example.test/streets.json": "not a document either",
	}, fixedStyles(styleURL))

	resolver.Refresh(t.Context())

	// The TileJSON's own host survives its unreadable body: the page has to
	// reach the document to find that out, exactly as this did.
	assert.Equal(t, []string{"https://catalogue.example.test", "https://fonts.example.test"},
		resolver.Origins(), "the origins a partly unreadable style names")
}

// A style naming more sources than the cap must not turn one refresh into a
// crawl of a provider's whole catalogue.
func TestResolverBoundsTheTileJSONLookupsOneStyleCauses(t *testing.T) {
	sources := make([]string, 0, maximumSourceLookups+4)
	bodies := make(map[string]string, maximumSourceLookups+5)
	for index := range maximumSourceLookups + 4 {
		name := fmt.Sprintf("source-%02d", index)
		catalogue := fmt.Sprintf("https://catalogue-%02d.example.test/streets.json", index)
		sources = append(sources, fmt.Sprintf("%q: {\"url\": %q}", name, catalogue))
		bodies[catalogue] = fmt.Sprintf(`{"tiles": ["https://tiles-%02d.example.test/{z}/{x}/{y}.pbf"]}`, index)
	}
	bodies["https://styles.example.test/bright"] = `{"sources": {` + strings.Join(sources, ",") + `}}`
	resolver, served := newResolver(t, bodies, fixedStyles(styleURL))

	resolver.Refresh(t.Context())

	assert.Len(t, served.requests, maximumSourceLookups+1, "the style and its capped TileJSON lookups")
	// Every source's own host is named, because that one is in the style itself;
	// only the tile hosts behind the cap go unseen.
	origins := resolver.Origins()
	assert.Len(t, origins, 2*maximumSourceLookups+4, "the origins a capped read names")
	assert.Contains(t, origins, "https://catalogue-11.example.test", "a source past the cap")
	assert.NotContains(t, origins, "https://tiles-11.example.test", "a tile host behind the cap")
}
