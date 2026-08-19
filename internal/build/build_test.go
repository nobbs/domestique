package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestCurrentReportsNothingForAnUninjectedBuild(t *testing.T) {
	// Every local build lands here. Absent rather than "unknown" or a short
	// abbreviation, so nothing downstream can render a link out of it.
	info := Current("")
	assert.Empty(t, info.Revision, "a build with no injected revision must report none")
	assert.Empty(t, info.ImageDigest, "no image reference was given, so there is no digest to report")
}

func TestValidRevisionAcceptsOnlyAFullObjectName(t *testing.T) {
	assert.Equal(t, testRevision, validRevision("  "+testRevision+"\n"), "validRevision(padded)")

	// A short SHA is refused on purpose: an abbreviation can become ambiguous
	// as the repository grows, and a link built from one would rot silently.
	for _, value := range []string{
		"",
		"0123456",
		testRevision[:39],
		testRevision + "0",
		strings.ToUpper(testRevision),
		"0123456789abcdef0123456789abcdef0123456g",
		"refs/heads/main",
		"$GITHUB_SHA",
	} {
		assert.Empty(t, validRevision(value), "validRevision(%q)", value)
	}
}

func TestValidDigestKeepsTheDigestAndDropsWhereItCameFrom(t *testing.T) {
	digest := "sha256:" + strings.Repeat("ab", 32)

	// The repository and registry in front of the digest say where a host pulls
	// from. That is deployment topology, and it must not travel with the answer.
	for _, reference := range []string{
		digest,
		"  " + digest + "\n",
		"ghcr.io/nobbs/domestique@" + digest,
		"registry.internal.example/mirror/domestique@" + digest,
	} {
		assert.Equal(t, digest, validDigest(reference), "validDigest(%q)", reference)
	}

	// A tag names whatever was last pushed to it, so it cannot answer "which
	// image is this" and is dropped rather than reported.
	for _, reference := range []string{
		"",
		"ghcr.io/nobbs/domestique:latest",
		"ghcr.io/nobbs/domestique:sha-0123456",
		"sha256:" + strings.Repeat("ab", 31),
		"sha256:" + strings.Repeat("ab", 32) + "cd",
		"sha512:" + strings.Repeat("ab", 32),
		"SHA256:" + strings.Repeat("ab", 32),
		"sha256:" + strings.Repeat("AB", 32),
		"ghcr.io/nobbs/domestique@notadigest",
	} {
		assert.Empty(t, validDigest(reference), "validDigest(%q)", reference)
	}
}

// TestInjectedRevisionReachesInfo compiles the documented -X symbol into a real
// binary. The Dockerfile and the Makefile both write that symbol as a string:
// nothing else in the build would notice if this package renamed the variable,
// and the failure would be a service that quietly reports no revision.
func TestInjectedRevisionReachesInfo(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go tool is not on PATH")
	}

	digest := "sha256:" + strings.Repeat("cd", 32)
	binary := filepath.Join(t.TempDir(), "revision")
	//nolint:gosec // The tool path comes from LookPath and every argument is a constant in this test.
	build := exec.CommandContext(t.Context(), goTool, "build",
		"-ldflags", "-X github.com/nobbs/domestique/internal/build.revision="+testRevision,
		"-o", binary, "./testdata/revision")
	output, buildErr := build.CombinedOutput()
	require.NoErrorf(t, buildErr, "building the probe:\n%s", output)

	//nolint:gosec // The binary is the one this test just built into its own temporary directory.
	probe := exec.CommandContext(t.Context(), binary)
	probe.Env = append(os.Environ(), "IMAGE_REFERENCE=ghcr.io/nobbs/domestique@"+digest)
	output, probeErr := probe.Output()
	require.NoError(t, probeErr, "running the probe")

	assert.Equal(t, testRevision+" "+digest, strings.TrimSpace(string(output)), "the probe reported the wrong build")
}
