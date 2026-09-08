package openmeteo

import (
	"errors"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// maxForecastCacheEntries bounds the map so a pathological caller cannot grow
// it without limit; the entries themselves are small (a handful of series).
const maxForecastCacheEntries = 256

// forecastCacheEntry is one Forecast answer, cached against the exact
// upstream request that produced it.
type forecastCacheEntry struct {
	expires time.Time
	series  []Series
}

// forecastCache shares one Forecast answer across every caller asking the
// same upstream request within its TTL, and collapses concurrent misses for
// the same key into one upstream call. An error is never cached.
type forecastCache struct {
	group singleflight.Group
	now   func() time.Time
	items map[string]forecastCacheEntry
	ttl   time.Duration
	mu    sync.Mutex
}

func newForecastCache(now func() time.Time, ttl time.Duration) *forecastCache {
	return &forecastCache{now: now, ttl: ttl, items: make(map[string]forecastCacheEntry)}
}

// get answers key from the cache, or calls miss on a cache miss. Concurrent
// misses for the same key share one call to miss.
func (c *forecastCache) get(key string, miss func() ([]Series, error)) ([]Series, error) {
	if series, ok := c.lookup(key); ok {
		return series, nil
	}

	result, err, _ := c.group.Do(key, func() (any, error) {
		// Another caller may have populated the cache while this one waited
		// to enter the singleflight call.
		if series, ok := c.lookup(key); ok {
			return series, nil
		}
		series, missErr := miss()
		if missErr != nil {
			return nil, missErr
		}
		c.store(key, series)

		return series, nil
	})
	if err != nil {
		return nil, err //nolint:wrapcheck // the miss closure's own error is already an openmeteo error.
	}
	series, ok := result.([]Series)
	if !ok {
		return nil, errors.New("openmeteo: forecast cache returned an unexpected value")
	}

	return series, nil
}

func (c *forecastCache) lookup(key string) ([]Series, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.items[key]
	if !ok || !c.now().Before(entry.expires) {
		return nil, false
	}

	return entry.series, true
}

func (c *forecastCache) store(key string, series []Series) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for k, entry := range c.items {
		if !now.Before(entry.expires) {
			delete(c.items, k)
		}
	}
	if len(c.items) >= maxForecastCacheEntries {
		for k := range c.items {
			delete(c.items, k)

			break
		}
	}
	c.items[key] = forecastCacheEntry{series: series, expires: now.Add(c.ttl)}
}
