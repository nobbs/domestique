// Package basemap discovers the origins a MapLibre style reaches beyond its
// own. A style document names its glyphs, its sprite, and its tile endpoints by
// URL, and a provider is free to serve any of them from a different host than
// the style itself. Those hosts are in the document rather than in the
// configuration, so a Content-Security-Policy built from the configured style
// URLs alone omits them and the browser blocks them: a map that draws its
// ground with no labels on it, or no streets under them.
package basemap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	defaultTimeout = 15 * time.Second
	// defaultInterval is how often a configured style is read again. A provider
	// that moves its glyph or tile host is followed within one, without an
	// operator touching the settings.
	defaultInterval = 6 * time.Hour

	// The two documents fetched here, bounded. A style with every layer spelled
	// out runs to a few hundred kilobytes; a TileJSON is a small fraction of that.
	maximumStyleBytes    = 4 << 20
	maximumTileJSONBytes = 1 << 20

	// maximumSourceLookups bounds the TileJSON fetches one style may cause, so a
	// style naming many sources cannot turn one refresh into a crawl.
	maximumSourceLookups = 8
)

// Options configures a Resolver.
type Options struct {
	// Styles reports the style URLs currently configured, light and dark alike.
	// Read on every refresh rather than once, so an operator's edit reaches the
	// next refresh rather than the next restart. Required.
	Styles func() []string

	Transport http.RoundTripper

	// Timeout bounds one document fetch, not a whole refresh.
	Timeout time.Duration

	// Interval is how often every configured style is read again.
	Interval time.Duration
}

// Resolver holds what each configured style was last seen to name. It is safe
// for concurrent use: one goroutine refreshes it while every response being
// composed reads it.
type Resolver struct {
	styles  func() []string
	client  *http.Client
	byStyle map[string][]string

	mu       sync.RWMutex
	interval time.Duration
}

// NewResolver creates a resolver that has not yet read anything. Until its
// first refresh it reports no origins, which is the policy this service sent
// before styles were read at all.
func NewResolver(options Options) (*Resolver, error) {
	if options.Styles == nil {
		return nil, errors.New("the configured style URLs are required")
	}
	timeout := options.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	interval := options.Interval
	if interval <= 0 {
		interval = defaultInterval
	}

	return &Resolver{
		styles:   options.Styles,
		interval: interval,
		byStyle:  make(map[string][]string),
		client: &http.Client{
			Timeout:   timeout,
			Transport: options.Transport,
			// A provider redirecting its style to plain HTTP would otherwise put an
			// origin in the policy that the page may not reach from an HTTPS
			// document anyway.
			CheckRedirect: func(request *http.Request, _ []*http.Request) error {
				if request.URL.Scheme != "https" {
					return fmt.Errorf("refusing a redirect to %s", request.URL.Scheme)
				}

				return nil
			},
		},
	}, nil
}

// Origins reports every origin the configured styles were last seen to name,
// sorted and deduplicated. It answers from what was last resolved and never
// blocks on the network, because it is called while a response header is being
// composed.
//
// It reads the configured list rather than the whole cache, so a basemap
// removed on the settings page stops being named by the next response rather
// than at the next refresh.
func (r *Resolver) Origins() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	origins := make([]string, 0, len(r.byStyle))
	for _, style := range r.styles() {
		origins = append(origins, r.byStyle[style]...)
	}
	slices.Sort(origins)

	return slices.Compact(origins)
}

// Refresh reads every configured style once and replaces what is known about
// it. A style that cannot be read keeps the origins it last resolved to:
// dropping them would blank a working map for as long as the provider is
// unreachable, which is the failure this whole component exists to avoid.
func (r *Resolver) Refresh(ctx context.Context) {
	styles := r.styles()
	resolved := make(map[string][]string, len(styles))
	for _, style := range styles {
		if _, done := resolved[style]; done {
			continue
		}
		origins, err := r.resolveStyle(ctx, style)
		if err != nil {
			if previous, known := r.originsOf(style); known {
				resolved[style] = previous
			}

			continue
		}
		resolved[style] = origins
	}

	// Replaced whole rather than merged, so a style no longer configured is
	// forgotten rather than accumulating across a deployment's lifetime.
	r.mu.Lock()
	r.byStyle = resolved
	r.mu.Unlock()
}

// Run resolves the configured styles now and again on the refresh interval,
// until ctx is done.
func (r *Resolver) Run(ctx context.Context) {
	r.Refresh(ctx)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.Refresh(ctx)
		}
	}
}

func (r *Resolver) originsOf(style string) ([]string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	origins, known := r.byStyle[style]

	return origins, known
}

// styleDocument is the part of a MapLibre style that names a URL. Every other
// field is ignored, so a style gaining one does not need this type to change.
type styleDocument struct {
	Sources map[string]json.RawMessage `json:"sources"`
	Glyphs  string                     `json:"glyphs"`
	// Sprite is a bare URL in older styles and a list of named ones in newer,
	// so it is decoded by hand.
	Sprite json.RawMessage `json:"sprite"`
}

// sourceDocument is one entry of a style's `sources`. A source names its tiles
// directly, points at a TileJSON that names them, or — for GeoJSON — carries
// either a URL or the geometry itself.
type sourceDocument struct {
	Data  json.RawMessage `json:"data"`
	URL   string          `json:"url"`
	Tiles []string        `json:"tiles"`
}

type tileJSONDocument struct {
	Tiles []string `json:"tiles"`
}

// resolveStyle reads one style document and returns the origins it names.
func (r *Resolver) resolveStyle(ctx context.Context, styleURL string) ([]string, error) {
	base, err := url.Parse(strings.TrimSpace(styleURL))
	if err != nil || base.Scheme != "https" || base.Host == "" {
		return nil, fmt.Errorf("%s is not an absolute HTTPS URL", redacted(styleURL))
	}

	var document styleDocument
	if fetchErr := r.fetchJSON(ctx, base, maximumStyleBytes, &document); fetchErr != nil {
		return nil, fetchErr
	}

	origins := make([]string, 0, 4)
	add := func(reference string) {
		if target, ok := absolute(base, reference); ok {
			origins = append(origins, target.Scheme+"://"+strings.ToLower(target.Host))
		}
	}
	add(document.Glyphs)
	for _, sprite := range spriteURLs(document.Sprite) {
		add(sprite)
	}

	// Sorted rather than ranged over the map, so which sources are looked up
	// when a style names more than the cap is the same on every refresh.
	lookups := 0
	for _, name := range slices.Sorted(maps.Keys(document.Sources)) {
		var source sourceDocument
		if json.Unmarshal(document.Sources[name], &source) != nil {
			continue
		}
		for _, tile := range source.Tiles {
			add(tile)
		}
		add(stringOf(source.Data))
		if source.URL == "" {
			continue
		}
		add(source.URL)

		// A TileJSON is an indirection: it names the tile endpoints, and a
		// provider is free to serve them from a host its own URL does not name.
		tileJSON, ok := absolute(base, source.URL)
		if !ok || lookups >= maximumSourceLookups {
			continue
		}
		lookups++
		var described tileJSONDocument
		if r.fetchJSON(ctx, tileJSON, maximumTileJSONBytes, &described) != nil {
			continue
		}
		for _, tile := range described.Tiles {
			add(tile)
		}
	}
	slices.Sort(origins)

	return slices.Compact(origins), nil
}

func (r *Resolver) fetchJSON(ctx context.Context, target *url.URL, limit int64, into any) (err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), http.NoBody)
	if err != nil {
		return fmt.Errorf("requesting %s: %w", redacted(target.String()), err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := r.client.Do(request)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", redacted(target.String()), err)
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetching %s: status %d", redacted(target.String()), response.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, limit)).Decode(into); err != nil {
		return fmt.Errorf("reading %s: %w", redacted(target.String()), err)
	}

	return nil
}

// spriteURLs reads a style's `sprite`, which is one URL in a style written
// against the older spec and a list of named ones in a newer.
func spriteURLs(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}
	if single := stringOf(raw); single != "" {
		return []string{single}
	}
	var named []struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(raw, &named) != nil {
		return nil
	}
	sprites := make([]string, 0, len(named))
	for _, sprite := range named {
		sprites = append(sprites, sprite.URL)
	}

	return sprites
}

// stringOf reads a field that may be either a URL or an inline document, as a
// GeoJSON source's `data` is. An inline one names no host and is skipped.
func stringOf(raw json.RawMessage) string {
	var value string
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return ""
	}

	return value
}

// absolute resolves one of a style's references against the style's own URL and
// keeps it only when it lands on HTTPS. A `mapbox://` reference and a plain
// HTTP host are both dropped rather than named in a policy: the page is served
// over HTTPS and could not reach either.
//
// A reference is usually a template — `{fontstack}`, `{z}/{x}/{y}` — but a
// placeholder only ever stands in part of a path or query, never in the host,
// so the origin is readable without expanding it.
func absolute(base *url.URL, reference string) (*url.URL, bool) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		return nil, false
	}
	target, err := base.Parse(reference)
	if err != nil || target.Scheme != "https" || target.Host == "" {
		return nil, false
	}

	return target, true
}

// redacted names a URL without its query, because a keyed provider carries its
// API key there and a failure to read a style is worth reporting without it.
func redacted(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return "the style URL"
	}
	parsed.RawQuery, parsed.Fragment, parsed.User = "", "", nil

	return parsed.String()
}
