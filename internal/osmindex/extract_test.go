package osmindex

import (
	"crypto/md5" //nolint:gosec // Matching what the publisher publishes.
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A slug is validated to be nothing but lowercase path segments, so composing
// the URL can only ever select a path below the base.
func TestExtractURLStaysBelowTheBase(t *testing.T) {
	assert.Equal(t,
		"https://download.geofabrik.de/europe/germany/rheinland-pfalz-latest.osm.pbf",
		extractURL("https://download.geofabrik.de", "europe/germany/rheinland-pfalz"),
	)
	assert.Equal(t,
		"https://download.geofabrik.de/antarctica-latest.osm.pbf",
		extractURL("https://download.geofabrik.de/", "antarctica"),
		"a trailing slash on the base does not double",
	)
}

func TestFetchChecksumReadsThePublishedDigest(t *testing.T) {
	digest := digestOf([]byte("an extract"))
	server := extractServer(t, map[string]string{
		"/europe/germany-latest.osm.pbf.md5": digest + "  germany-latest.osm.pbf\n",
	})

	checksum, err := fetchChecksum(t.Context(), server.Client(), server.URL, "europe/germany")
	require.NoError(t, err, "fetchChecksum()")
	assert.Equal(t, digest, checksum, "the published digest")
}

func TestFetchChecksumRejectsSomethingThatIsNotADigest(t *testing.T) {
	tests := map[string]string{
		"an error page":      "<html>404 not found</html>\n",
		"an empty file":      "",
		"a truncated digest": "abc123  germany-latest.osm.pbf\n",
		"a non-hex digest":   strings.Repeat("z", 32) + "  germany-latest.osm.pbf\n",
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			server := extractServer(t, map[string]string{"/europe/germany-latest.osm.pbf.md5": body})

			_, err := fetchChecksum(t.Context(), server.Client(), server.URL, "europe/germany")
			require.Error(t, err, "fetchChecksum() accepted %q", body)
		})
	}
}

// An extract truncated by a dropped connection decodes as a valid but partial
// map, which would produce an index quietly missing half its roads. The digest
// is what catches it, and nothing may be left behind for a later run to find.
func TestDownloadExtractRefusesAndRemovesAMismatchedFile(t *testing.T) {
	directory := t.TempDir()
	server := extractServer(t, map[string]string{
		"/europe/germany-latest.osm.pbf": "half an extract",
	})

	path, err := downloadExtract(
		t.Context(), server.Client(), server.URL, "europe/germany", digestOf([]byte("a whole extract")), directory,
	)
	require.Error(t, err, "downloadExtract() accepted a body that failed its checksum")
	assert.Contains(t, err.Error(), "failed its checksum")
	assert.Empty(t, path, "downloadExtract() named a file it refused")

	entries, readErr := os.ReadDir(directory)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "the refused extract was left on disk")
}

// A dropped connection is the failure this download is most likely to meet, and
// it has to leave nothing behind: a partial file that survives is one a later run
// could mistake for a whole one. The checksum would catch a short body too, but
// only after writing it; this is the earlier branch.
func TestDownloadExtractRemovesWhatADroppedConnectionLeft(t *testing.T) {
	directory := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Length", "4096")
		_, err := writer.Write([]byte("the first few bytes"))
		assert.NoError(t, err, "writing the truncated body")
		// Flushed so the response reaches the client and the copy is genuinely
		// under way; abandoned after, which the client sees as a connection that
		// went away rather than as a body that ended.
		flusher, ok := writer.(http.Flusher)
		if !assert.True(t, ok, "the test server's writer flushes") {
			return
		}
		flusher.Flush()
		panic(http.ErrAbortHandler)
	}))
	t.Cleanup(server.Close)

	path, err := downloadExtract(
		t.Context(), server.Client(), server.URL, "europe/germany", digestOf([]byte("a whole extract")), directory,
	)
	require.Error(t, err, "downloadExtract() accepted a transfer that never finished")
	assert.Contains(t, err.Error(), "downloading extract", "the transfer itself is what failed")
	assert.Empty(t, path, "downloadExtract() named a file it refused")

	entries, readErr := os.ReadDir(directory)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "the partial extract was left on disk")
}

func TestDownloadExtractKeepsAVerifiedFile(t *testing.T) {
	directory := t.TempDir()
	body := "a whole extract"
	server := extractServer(t, map[string]string{"/europe/germany-latest.osm.pbf": body})

	path, err := downloadExtract(
		t.Context(), server.Client(), server.URL, "europe/germany", digestOf([]byte(body)), directory,
	)
	require.NoError(t, err, "downloadExtract()")
	assert.Equal(t, filepath.Join(directory, "europe_germany-latest.osm.pbf"), path,
		"a slug becomes one filename rather than a directory tree")

	written, err := os.ReadFile(path) //nolint:gosec // The path is the one downloadExtract just reported.
	require.NoError(t, err)
	assert.Equal(t, body, string(written), "the extract on disk")
}

// A checksum is fetched before anything is downloaded, so an upstream that is
// down costs one small request and stops there.
func TestBuildFailsBeforeDownloadingWhenTheChecksumIsUnavailable(t *testing.T) {
	directory := t.TempDir()
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requested = append(requested, request.URL.Path)
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	_, err := Build(t.Context(), Options{
		Regions:   []string{"europe/germany"},
		Directory: directory,
		BaseURL:   server.URL,
		Client:    server.Client(),
	}, "")
	require.Error(t, err, "Build() against an unavailable upstream")
	assert.Equal(t, []string{"/europe/germany-latest.osm.pbf.md5"}, requested,
		"the extract was requested despite its checksum being unavailable")

	entries, readErr := os.ReadDir(directory)
	require.NoError(t, readErr)
	assert.Empty(t, entries, "a failed build left something behind")
}

// A rebuild that finds every extract republished unchanged costs one small
// request per region and writes nothing, which is what makes a weekly schedule
// affordable on a domestic connection.
func TestBuildReportsUnchangedWithoutDownloadingAnything(t *testing.T) {
	directory := t.TempDir()
	digest := digestOf([]byte("an extract"))
	var requested []string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requested = append(requested, request.URL.Path)
		if !strings.HasSuffix(request.URL.Path, checksumSuffix) {
			writer.WriteHeader(http.StatusInternalServerError)

			return
		}
		_, writeErr := fmt.Fprintf(writer, "%s  germany-latest.osm.pbf\n", digest)
		assert.NoError(t, writeErr, "writing the checksum")
	}))
	t.Cleanup(server.Close)

	current := generationOf(map[string]string{"europe/germany": digest})
	result, err := Build(t.Context(), Options{
		Regions:   []string{"europe/germany"},
		Directory: directory,
		BaseURL:   server.URL,
		Client:    server.Client(),
	}, current)
	require.NoError(t, err, "Build()")
	assert.True(t, result.Unchanged, "Build() rebuilt an index nothing upstream had changed")
	assert.Equal(t, current, result.Generation, "Build().Generation")
	assert.Empty(t, result.Path, "Build() named a file it did not write")
	assert.Equal(t, []string{"/europe/germany-latest.osm.pbf.md5"}, requested, "requests made")
}

// A mistyped region is a startup error the operator reads, not a request.
func TestBuildRefusesARegionThatIsNotARegionPath(t *testing.T) {
	var requested int
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requested++ }))
	t.Cleanup(server.Close)

	_, err := Build(t.Context(), Options{
		Regions:   []string{"../../etc/passwd"},
		Directory: t.TempDir(),
		BaseURL:   server.URL,
		Client:    server.Client(),
	}, "")
	require.Error(t, err, "Build() accepted a region that is not a region path")
	assert.Zero(t, requested, "a rejected region still reached the network")
}

func extractServer(t *testing.T, bodies map[string]string) *httptest.Server {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, found := bodies[request.URL.Path]
		if !found {
			writer.WriteHeader(http.StatusNotFound)

			return
		}
		_, err := writer.Write([]byte(body))
		assert.NoError(t, err, "writing the body")
	}))
	t.Cleanup(server.Close)

	return server
}

func digestOf(body []byte) string {
	sum := md5.Sum(body) //nolint:gosec // Matching what the publisher publishes.

	return hex.EncodeToString(sum[:])
}
