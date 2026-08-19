package build

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testRevision = "0123456789abcdef0123456789abcdef01234567"

func TestCurrentReportsNothingForAnUninjectedBuild(t *testing.T) {
	// Every local build lands here. Absent rather than "unknown" or a short
	// abbreviation, so nothing downstream can render a link out of it.
	info := Current("")
	if info.Revision != "" {
		t.Errorf("revision = %q, want empty for a build with no injected revision", info.Revision)
	}
	if info.ImageDigest != "" {
		t.Errorf("image digest = %q, want empty when no image reference was given", info.ImageDigest)
	}
}

func TestValidRevisionAcceptsOnlyAFullObjectName(t *testing.T) {
	if got := validRevision("  " + testRevision + "\n"); got != testRevision {
		t.Errorf("validRevision(padded) = %q, want %q", got, testRevision)
	}

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
		if got := validRevision(value); got != "" {
			t.Errorf("validRevision(%q) = %q, want empty", value, got)
		}
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
		if got := validDigest(reference); got != digest {
			t.Errorf("validDigest(%q) = %q, want %q", reference, got, digest)
		}
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
		if got := validDigest(reference); got != "" {
			t.Errorf("validDigest(%q) = %q, want empty", reference, got)
		}
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
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		t.Fatalf("building the probe: %v\n%s", buildErr, output)
	}

	//nolint:gosec // The binary is the one this test just built into its own temporary directory.
	probe := exec.CommandContext(t.Context(), binary)
	probe.Env = append(os.Environ(), "IMAGE_REFERENCE=ghcr.io/nobbs/domestique@"+digest)
	output, probeErr := probe.Output()
	if probeErr != nil {
		t.Fatalf("running the probe: %v", probeErr)
	}

	if got, want := strings.TrimSpace(string(output)), testRevision+" "+digest; got != want {
		t.Errorf("probe reported %q, want %q", got, want)
	}
}
