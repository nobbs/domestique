package zwift_test

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/zwift"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newWorldMaps points the relay at a local TLS test server standing in for the
// CDN, which is the only way anything here reaches an image.
func newWorldMaps(t *testing.T, handler http.HandlerFunc) *zwift.WorldMaps {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	maps, err := zwift.NewWorldMaps(&zwift.WorldMapOptions{
		Transport:  server.Client().Transport,
		CDNBaseURL: server.URL,
	})
	require.NoError(t, err, "NewWorldMaps()")

	return maps
}

func TestWorldMapsFetchesOnceAndAnswersFromMemory(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	var path atomic.Value
	maps := newWorldMaps(t, func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		path.Store(request.URL.Path)
		writer.Header().Set("Content-Type", "image/png")
		//nolint:errcheck // A test server write that fails fails the assertions below.
		_, _ = writer.Write([]byte("world-map-bytes"))
	})

	first, contentType, _, found, err := maps.Image(t.Context(), 1)
	require.NoError(t, err, "Image()")
	require.True(t, found)
	second, _, _, _, err := maps.Image(t.Context(), 1)
	require.NoError(t, err, "Image() again")

	assert.Equal(t, "image/png", contentType)
	assert.Equal(t, []byte("world-map-bytes"), first)
	assert.Equal(t, first, second)
	assert.Equal(t, int64(1), requests.Load(), "the CDN is read once")
	assert.Equal(t, "/static/images/maps/MiniMap_Watopia_2.png", path.Load())
}

func TestWorldMapsRefusesAnUnknownWorldWithoutARequest(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	maps := newWorldMaps(t, func(http.ResponseWriter, *http.Request) { requests.Add(1) })

	_, _, _, found, err := maps.Image(t.Context(), 12)

	require.NoError(t, err)
	assert.False(t, found, "no world has that id")
	assert.Equal(t, int64(0), requests.Load(), "nothing is fetched for a world that does not exist")
}

// A redirect could leave the CDN's origin, so it is refused rather than followed.
func TestWorldMapsDoesNotFollowARedirect(t *testing.T) {
	t.Parallel()
	var followed atomic.Int32

	maps := newWorldMaps(t, func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/elsewhere" {
			followed.Add(1)
			writer.Header().Set("Content-Type", "image/png")
			writer.WriteHeader(http.StatusOK)

			return
		}
		http.Redirect(writer, request, "/elsewhere", http.StatusFound)
	})

	_, _, _, _, err := maps.Image(t.Context(), 1)

	require.ErrorContains(t, err, "unexpected status 302")
	assert.Equal(t, int32(0), followed.Load(), "the redirect target must never be requested")
}

func TestWorldMapsRefusesAReplyThatIsNotAnImage(t *testing.T) {
	t.Parallel()

	maps := newWorldMaps(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/html")
		//nolint:errcheck // A test server write that fails fails the assertions below.
		_, _ = writer.Write([]byte("<html>"))
	})

	_, _, _, _, err := maps.Image(t.Context(), 1)

	require.ErrorContains(t, err, "not served as an image")
}

func TestWorldMapsRefusesAnOversizedReply(t *testing.T) {
	t.Parallel()

	maps := newWorldMaps(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "image/png")
		//nolint:errcheck // A test server write that fails fails the assertions below.
		_, _ = writer.Write(make([]byte, (8<<20)+1))
	})

	_, _, _, _, err := maps.Image(t.Context(), 1)

	require.ErrorContains(t, err, "larger than this relay allows")
}

func TestWorldMapsSurfacesAnUpstreamFailureAndCachesNothing(t *testing.T) {
	t.Parallel()

	var requests atomic.Int64
	maps := newWorldMaps(t, func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusBadGateway)
	})

	_, _, _, _, err := maps.Image(t.Context(), 1)
	require.ErrorContains(t, err, "unexpected status 502")

	_, _, _, _, err = maps.Image(t.Context(), 1)

	require.Error(t, err)
	assert.Equal(t, int64(2), requests.Load(), "a failed fetch is retried rather than remembered")
}

func TestNewWorldMapsRefusesUnusableOptions(t *testing.T) {
	t.Parallel()

	for name, options := range map[string]*zwift.WorldMapOptions{
		"none":             nil,
		"negative timeout": {Timeout: -time.Second},
		"not an origin":    {CDNBaseURL: "http://cdn.zwift.com/static"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			_, err := zwift.NewWorldMaps(options)

			require.Error(t, err)
		})
	}
}

func TestWorldMapsReportsACDNItCannotReach(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	maps, err := zwift.NewWorldMaps(&zwift.WorldMapOptions{
		Transport: server.Client().Transport, CDNBaseURL: server.URL,
	})
	require.NoError(t, err, "NewWorldMaps()")
	server.Close()

	_, _, _, _, err = maps.Image(t.Context(), 1)

	require.ErrorContains(t, err, "zwift: world map")
}

// A reply that promises more bytes than it delivers fails the read rather than
// being kept as a truncated image.
func TestWorldMapsRefusesATruncatedReply(t *testing.T) {
	t.Parallel()

	maps := newWorldMaps(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "image/png")
		writer.Header().Set("Content-Length", "64")
		//nolint:errcheck // A test server write that fails fails the assertion below.
		_, _ = writer.Write([]byte("short"))
		panic(http.ErrAbortHandler)
	})

	_, _, _, _, err := maps.Image(t.Context(), 1)

	require.Error(t, err)
}
