package zwift

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultCDNBaseURL is where Zwift publishes its world map artwork. It answers
// no CORS header, which is why this service relays the bytes rather than
// letting a browser fetch them.
const DefaultCDNBaseURL = "https://cdn.zwift.com"

// mapImagePath is the CDN directory every world's artwork sits in.
const mapImagePath = "/static/images/maps/"

// maximumMapImageBytes bounds one downloaded image. The largest published
// world map is under 3 MB.
const maximumMapImageBytes = 8 << 20

// WorldMapOptions configures the world map relay.
type WorldMapOptions struct {
	Transport  http.RoundTripper
	CDNBaseURL string
	Timeout    time.Duration
}

// WorldMaps serves Zwift's published world map artwork from this service's own
// origin, fetching each world's image from the CDN once and keeping it.
type WorldMaps struct {
	httpClient *http.Client
	images     map[int64]worldImage
	baseURL    string
	mutex      sync.Mutex
}

// worldImage is one fetched image and the content type it was served as.
type worldImage struct {
	contentType string
	data        []byte
}

// NewWorldMaps creates the relay without contacting the CDN.
func NewWorldMaps(options *WorldMapOptions) (*WorldMaps, error) {
	if options == nil {
		return nil, errors.New("zwift: world map options are required")
	}
	if options.Timeout < 0 {
		return nil, errors.New("zwift: timeout must not be negative")
	}
	baseURL, err := parseOrigin(cmp.Or(options.CDNBaseURL, DefaultCDNBaseURL))
	if err != nil {
		return nil, fmt.Errorf("zwift: cdn base url: %w", err)
	}

	return &WorldMaps{
		baseURL: strings.TrimSuffix(baseURL.String(), "/"),
		httpClient: &http.Client{
			Transport: options.Transport,
			Timeout:   cmp.Or(options.Timeout, defaultTimeout),
		},
		images: map[int64]worldImage{},
	}, nil
}

// Image is one world's map artwork and the content type to serve it as, read
// from the CDN the first time it is asked for and from memory afterwards.
// found is false for an id no world has, which is not a failure. A failed
// fetch is not kept, so the next caller tries again.
func (m *WorldMaps) Image(
	ctx context.Context, worldID int64,
) (data []byte, contentType string, found bool, err error) {
	world, known := WorldByID(worldID)
	if !known {
		return nil, "", false, nil
	}
	m.mutex.Lock()
	held, cached := m.images[worldID]
	m.mutex.Unlock()
	if cached {
		return held.data, held.contentType, true, nil
	}
	fetched, err := m.fetch(ctx, world)
	if err != nil {
		return nil, "", false, err
	}
	m.mutex.Lock()
	m.images[worldID] = fetched
	m.mutex.Unlock()

	return fetched.data, fetched.contentType, true, nil
}

// fetch reads one world's image from the CDN. Two callers racing on the same
// world both fetch; the second simply overwrites an identical entry, which is
// cheaper than holding the lock across a network read.
func (m *WorldMaps) fetch(ctx context.Context, world World) (worldImage, error) {
	request, err := http.NewRequestWithContext(
		ctx, http.MethodGet, m.baseURL+mapImagePath+world.ImageFile, http.NoBody,
	)
	if err != nil {
		return worldImage{}, fmt.Errorf("zwift: world map request: %w", err)
	}
	response, err := m.httpClient.Do(request)
	if err != nil {
		return worldImage{}, fmt.Errorf("zwift: world map: %w", err)
	}
	//nolint:errcheck // A response body that will not close cannot change the result.
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return worldImage{}, fmt.Errorf("zwift: world map: unexpected status %d", response.StatusCode)
	}
	contentType := response.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		return worldImage{}, errors.New("zwift: world map was not served as an image")
	}
	// One byte over the cap is read deliberately, so a reply at exactly the
	// limit is told from one that was truncated at it.
	data, err := io.ReadAll(io.LimitReader(response.Body, maximumMapImageBytes+1))
	if err != nil {
		return worldImage{}, fmt.Errorf("zwift: world map: %w", err)
	}
	if len(data) > maximumMapImageBytes {
		return worldImage{}, errors.New("zwift: world map was larger than this relay allows")
	}

	return worldImage{data: data, contentType: contentType}, nil
}
