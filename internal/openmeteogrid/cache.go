package openmeteogrid

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// maxManifestBodyBytes bounds what Latest buffers; the manifest is a few
// kilobytes of JSON, so anything near this is a malformed upstream.
const maxManifestBodyBytes = 1 << 20

// manifestCacheEntry is one buffered Latest answer: the upstream status, only
// the headers relayWeatherGrid forwards, and the body bytes.
type manifestCacheEntry struct {
	header    http.Header
	body      []byte
	status    int
	cacheable bool
}

// manifestCache shares one Latest answer across every caller within its TTL,
// and collapses concurrent misses into one upstream call. There is only ever
// one manifest, so this holds a single entry rather than a keyed map.
type manifestCache struct {
	expires time.Time
	group   singleflight.Group
	entry   *manifestCacheEntry
	mu      sync.Mutex
}

func newManifestCache() *manifestCache {
	return &manifestCache{}
}

func (c *manifestCache) lookup(now time.Time) (*manifestCacheEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.entry == nil || !now.Before(c.expires) {
		return nil, false
	}

	return c.entry, true
}

func (c *manifestCache) store(entry *manifestCacheEntry, expires time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entry = entry
	c.expires = expires
}

// forwardedManifestHeaders keeps the headers the relay forwards; Content-Length
// is restated from the buffered body rather than trusted from upstream.
func forwardedManifestHeaders(source http.Header) http.Header {
	out := make(http.Header, 3)
	for _, name := range [...]string{"Content-Type", "ETag", "Last-Modified"} {
		if value := source.Get(name); value != "" {
			out.Set(name, value)
		}
	}

	return out
}

// respondFromManifest synthesises a fresh *http.Response per caller, so the
// read-then-close contract holds for a hit; a matching If-None-Match is a 304.
func respondFromManifest(entry *manifestCacheEntry, conditional http.Header) *http.Response {
	if etag := entry.header.Get("ETag"); etag != "" && conditional.Get("If-None-Match") == etag {
		header := make(http.Header, 2)
		header.Set("ETag", etag)
		if lastModified := entry.header.Get("Last-Modified"); lastModified != "" {
			header.Set("Last-Modified", lastModified)
		}

		return &http.Response{StatusCode: http.StatusNotModified, Header: header, Body: http.NoBody}
	}

	header := entry.header.Clone()
	header.Set("Content-Length", strconv.Itoa(len(entry.body)))

	return &http.Response{
		StatusCode:    entry.status,
		Header:        header,
		Body:          io.NopCloser(bytes.NewReader(entry.body)),
		ContentLength: int64(len(entry.body)),
	}
}
