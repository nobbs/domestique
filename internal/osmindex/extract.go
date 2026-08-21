package osmindex

import (
	"context"
	"crypto/md5" //nolint:gosec // Geofabrik publishes MD5; this checks transfer integrity, not authenticity.
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// DefaultBaseURL is where the regional extracts come from. It is a setting
// rather than a constant only so a test can point it somewhere local.
const DefaultBaseURL = "https://download.geofabrik.de"

// extractSuffix is what Geofabrik appends to a region slug for the current
// extract, and checksumSuffix for the checksum published beside it.
const (
	extractSuffix  = "-latest.osm.pbf"
	checksumSuffix = ".md5"
)

// checksumTimeout bounds fetching one small checksum file. Extracts themselves
// are bounded by the caller's context instead: they are hundreds of megabytes
// onto a small host and there is no useful fixed deadline for that.
const checksumTimeout = 30 * time.Second

// fetchChecksums reads the published checksum for every region.
//
// This runs before anything is downloaded, and its result decides both the
// generation and whether there is any work to do. A rebuild that finds every
// checksum unchanged costs one small request per region and stops there.
func fetchChecksums(ctx context.Context, client *http.Client, baseURL string, regions []string) (map[string]string, error) {
	checksums := make(map[string]string, len(regions))
	for _, region := range regions {
		checksum, err := fetchChecksum(ctx, client, baseURL, region)
		if err != nil {
			return nil, err
		}
		checksums[region] = checksum
	}

	return checksums, nil
}

func fetchChecksum(ctx context.Context, client *http.Client, baseURL, region string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, checksumTimeout)
	defer cancel()

	url := extractURL(baseURL, region) + checksumSuffix
	body, err := get(ctx, client, url)
	if err != nil {
		return "", err
	}
	defer closeBody(body)

	raw, err := io.ReadAll(io.LimitReader(body, 512))
	if err != nil {
		return "", fmt.Errorf("osmindex: reading checksum for %s: %w", region, err)
	}

	// The file is "<hex>  <filename>"; only the digest is of interest.
	fields := strings.Fields(string(raw))
	if len(fields) == 0 || len(fields[0]) != md5.Size*2 {
		return "", fmt.Errorf("osmindex: checksum for %s is not an MD5 digest", region)
	}
	if _, err := hex.DecodeString(fields[0]); err != nil {
		return "", fmt.Errorf("osmindex: checksum for %s is not hexadecimal", region)
	}

	return strings.ToLower(fields[0]), nil
}

// downloadExtract streams one region's extract to disk and verifies it against
// the checksum already fetched.
//
// The body is written straight through to the file while the digest is computed
// from the same stream, so a 300 MB extract never exists in memory. A digest
// that does not match leaves nothing behind: an extract truncated by a dropped
// connection would otherwise decode as a valid but partial map and produce an
// index quietly missing half its roads.
func downloadExtract(
	ctx context.Context,
	client *http.Client,
	baseURL, region, expectedChecksum, directory string,
) (string, error) {
	path := filepath.Join(directory, strings.ReplaceAll(region, "/", "_")+extractSuffix)

	body, err := get(ctx, client, extractURL(baseURL, region))
	if err != nil {
		return "", err
	}
	defer closeBody(body)

	file, err := os.Create(path) //nolint:gosec // The path is composed here from a validated region slug.
	if err != nil {
		return "", fmt.Errorf("osmindex: creating extract file for %s: %w", region, err)
	}

	digest := md5.New() //nolint:gosec // Integrity against the publisher's own digest.
	_, copyErr := io.Copy(io.MultiWriter(file, digest), body)
	closeErr := file.Close()
	switch {
	case copyErr != nil:
		removeFile(path)

		return "", fmt.Errorf("osmindex: downloading extract for %s: %w", region, copyErr)
	case closeErr != nil:
		removeFile(path)

		return "", fmt.Errorf("osmindex: writing extract for %s: %w", region, closeErr)
	}

	if actual := hex.EncodeToString(digest.Sum(nil)); actual != expectedChecksum {
		removeFile(path)

		return "", fmt.Errorf(
			"osmindex: extract for %s failed its checksum: published %s, received %s",
			region, expectedChecksum, actual,
		)
	}

	return path, nil
}

// extractURL composes the address of one region's extract. The slug has been
// validated to be nothing but lowercase path segments, so it can only ever
// select a path below the base.
func extractURL(baseURL, region string) string {
	return strings.TrimSuffix(baseURL, "/") + "/" + region + extractSuffix
}

//nolint:errcheck // A response body that will not close cannot change the result.
func closeBody(body io.ReadCloser) { _ = body.Close() }

func get(ctx context.Context, client *http.Client, url string) (io.ReadCloser, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("osmindex: building request for %s: %w", url, err)
	}

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("osmindex: fetching %s: %w", url, err)
	}
	if response.StatusCode != http.StatusOK {
		closeBody(response.Body)

		return nil, fmt.Errorf("osmindex: fetching %s: %s", url, response.Status)
	}

	return response.Body, nil
}
