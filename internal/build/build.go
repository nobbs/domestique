// Package build reports which public source revision produced this binary and
// which image is running it. Both come from outside the program: the revision is
// injected at link time by CI, and the image reference is what the deploying host
// was pinned to. Neither can be derived here, so this package refuses to guess.
package build

import "strings"

const (
	// A Git object name as GitHub addresses a commit by: forty lowercase hex
	// digits. A short SHA is deliberately not accepted, because a link built
	// from an abbreviation is a link that can become ambiguous.
	revisionLength = 40
	digestPrefix   = "sha256:"
	digestLength   = 64
)

// revision is set at link time with -X …/internal/build.revision=<commit sha>.
// A build that omits it — every local build — reports no revision at all.
//
//nolint:gochecknoglobals // A link-time -X target must be a package variable; nothing writes it at runtime.
var revision string

// Info is what may safely be said in public about the running build. Every
// field is either empty or a value that has been checked, so a reader never has
// to decide whether to trust one.
type Info struct {
	// Revision is the full commit the binary was built from, or empty when it
	// was not built by CI.
	Revision string
	// ImageDigest is the immutable digest of the running image, or empty when
	// the service was not told one.
	ImageDigest string
}

// Current reports the running build. imageReference is what the host was pinned
// to, in any form an operator's environment holds it. Only the digest is kept:
// the repository and registry describe where the deployment pulls from.
func Current(imageReference string) Info {
	return Info{
		Revision:    validRevision(revision),
		ImageDigest: validDigest(imageReference),
	}
}

// validRevision returns value when it is a full commit object name, and empty
// otherwise. A wrong or truncated value would produce a link that goes nowhere,
// which is worse than admitting the revision is unknown.
func validRevision(value string) string {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) != revisionLength || !isLowerHex(trimmed) {
		return ""
	}

	return trimmed
}

// validDigest extracts the digest from an image reference. A mutable tag is not
// a digest and is dropped rather than reported: it names whatever was last
// pushed to it, so it cannot answer "which image is this".
func validDigest(reference string) string {
	trimmed := strings.TrimSpace(reference)
	if at := strings.LastIndex(trimmed, "@"); at >= 0 {
		trimmed = trimmed[at+1:]
	}

	hex, found := strings.CutPrefix(trimmed, digestPrefix)
	if !found || len(hex) != digestLength || !isLowerHex(hex) {
		return ""
	}

	return digestPrefix + hex
}

func isLowerHex(value string) bool {
	for _, character := range value {
		switch {
		case character >= '0' && character <= '9':
		case character >= 'a' && character <= 'f':
		default:
			return false
		}
	}

	return value != ""
}
