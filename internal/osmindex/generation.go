package osmindex

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// builderVersion changes whenever this package would pack the same input
// differently. It is part of the generation, so a release that reads the map
// differently invalidates the indexes and caches built by the one before it.
const builderVersion = "1"

// regionSegment is one path element of a Geofabrik slug. Slugs are operator
// configuration that becomes a URL, so the shape is checked rather than trusted:
// a slug supplies a path under a fixed base and can never introduce a host, a
// query, or a traversal.
var regionSegment = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// ValidateRegion checks one Geofabrik slug, such as
// "europe/germany/rheinland-pfalz".
func ValidateRegion(slug string) error {
	if slug == "" {
		return fmt.Errorf("region slug must not be empty")
	}
	if strings.HasPrefix(slug, "/") || strings.HasSuffix(slug, "/") {
		return fmt.Errorf("region slug %q must not begin or end with a slash", slug)
	}
	for segment := range strings.SplitSeq(slug, "/") {
		if !regionSegment.MatchString(segment) {
			return fmt.Errorf(
				"region slug %q must be lowercase path segments of letters, digits, and single hyphens",
				slug,
			)
		}
	}

	return nil
}

// generationOf fingerprints a build from what would change its output. The
// regions are sorted, so the same set in a different order is the same
// generation, and each is paired with Geofabrik's published checksum rather than
// a timestamp: a republished but unchanged file forces no rebuild.
func generationOf(checksums map[string]string) string {
	regions := make([]string, 0, len(checksums))
	for region := range checksums {
		regions = append(regions, region)
	}
	sort.Strings(regions)

	digest := sha256.New()
	_, _ = digest.Write([]byte("osmindex/" + builderVersion + "\n"))
	_, _ = digest.Write([]byte(strconv.FormatFloat(cellDegrees, 'g', -1, 64) + "\n"))
	_, _ = digest.Write([]byte(strconv.FormatFloat(coordinateScale, 'g', -1, 64) + "\n"))
	for _, region := range regions {
		_, _ = digest.Write([]byte(region + " " + checksums[region] + "\n"))
	}

	// Twelve hex characters is 48 bits. The generation is compared against a
	// handful of predecessors on one host, not against the world, and it goes in
	// a filename and onto a status page where being readable matters more.
	return hex.EncodeToString(digest.Sum(nil))[:12]
}

// joinRegions and splitRegions move the region list in and out of the index's
// own metadata. A slug cannot contain a space, having been validated, so a space
// is a safe separator.
func joinRegions(regions []string) string { return strings.Join(regions, " ") }

func splitRegions(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	return strings.Fields(value)
}

func parseFloat(value string) (float64, error) {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("osmindex: %q is not a number: %w", value, err)
	}

	return parsed, nil
}
