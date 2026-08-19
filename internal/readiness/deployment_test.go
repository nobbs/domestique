package readiness

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readinessPort is the port the shipped container and the deployment files agree
// on. It is restated here rather than imported, because the point of these tests
// is that the image, the compose files, and the deploy script all still name the
// same one as internal/config defaults to.
const readinessPort = "8081"

// The image has to advertise the probe's port, or a host copying the published
// ports has nothing to publish.
func TestTheImageExposesTheReadinessPort(t *testing.T) {
	dockerfile := readRepositoryFile(t, "Dockerfile")

	expose := regexp.MustCompile(`(?m)^EXPOSE .*$`).FindString(dockerfile)
	require.NotEmpty(t, expose, "the Dockerfile must expose its ports")
	assert.Contains(t, expose, readinessPort)
	assert.Contains(t, expose, "8080", "the served port must stay exposed")
}

// Loopback only, on both files an operator starts from. A readiness probe
// published on a public address would be the one unauthenticated endpoint on the
// internet.
func TestTheComposeFilesPublishReadinessToLoopbackOnly(t *testing.T) {
	for _, name := range []string{"docs/compose.example.yml", "compose.macos.yml"} {
		t.Run(name, func(t *testing.T) {
			contents := readRepositoryFile(t, name)

			assert.Contains(t, contents, `"127.0.0.1:`+readinessPort+`:`+readinessPort+`"`)
			for line := range strings.SplitSeq(contents, "\n") {
				if !strings.Contains(line, ":"+readinessPort+":"+readinessPort) {
					continue
				}
				assert.Contains(t, line, "127.0.0.1:", "line %q publishes readiness off loopback", line)
			}
		})
	}
}

// The deploy gate has to ask whether the new image can read its state, not only
// whether it answers HTTP, and it has to ask over loopback.
func TestTheDeployScriptGatesOnReadiness(t *testing.T) {
	script := readRepositoryFile(t, "deploy/domestique-deploy.sh")

	assert.Contains(t, script, "READY_URL:-http://127.0.0.1:"+readinessPort+"/readyz")
	assert.Contains(t, script, "wait_healthy && wait_ready && loopback_only")
}

// Tailscale Serve fronts the served listener and nothing else. If a document
// ever tells an operator to serve the readiness port, the probe is on the
// authenticated public surface and this stops being a loopback-only probe.
func TestNoDocumentServesTheReadinessPortThroughTailscale(t *testing.T) {
	root := repositoryRoot(t)
	paths, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	require.NoError(t, err)
	paths = append(paths, filepath.Join(root, "README.md"))

	for _, path := range paths {
		contents, readErr := os.ReadFile(path) //nolint:gosec // a repository document, named by this test
		require.NoError(t, readErr)
		for line := range strings.SplitSeq(string(contents), "\n") {
			if !strings.Contains(line, "tailscale serve") {
				continue
			}
			assert.NotContains(t, line, readinessPort, "%s serves the readiness port: %q", path, line)
		}
	}
}

func readRepositoryFile(t *testing.T, name string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(repositoryRoot(t), name)) //nolint:gosec // a repository file, named by this test
	require.NoError(t, err)

	return string(contents)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	return root
}
