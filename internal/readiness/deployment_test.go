package readiness

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readinessPort is the port the shipped container and the deployment files agree
// on. It is restated here rather than imported, because the point of these tests
// is that the image, the compose file, and the deploy script all still name the
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

// Loopback only, on the file an operator starts from. A readiness probe
// published on a public address would be the one unauthenticated endpoint on the
// internet.
func TestTheComposeFilePublishesReadinessToLoopbackOnly(t *testing.T) {
	contents := readRepositoryFile(t, "docs/compose.example.yml")

	assert.Contains(t, contents, `"127.0.0.1:`+readinessPort+`:`+readinessPort+`"`)
	for line := range strings.SplitSeq(contents, "\n") {
		if !strings.Contains(line, ":"+readinessPort+":"+readinessPort) {
			continue
		}
		assert.Contains(t, line, "127.0.0.1:", "line %q publishes readiness off loopback", line)
	}
}

// Docker kills a container that outstays its grace period, and its default of
// ten seconds is shorter than the fifteen the service spends draining its
// listeners: a recreate can cut the shutdown off before the service has even
// reached the part it does after that. The two numbers sit in different files
// and neither names the other, so this is what keeps a change to either from
// quietly reintroducing the kill.
//
// It is only the bounded part of the shutdown that can be compared. The
// scheduler drain and the wait for an in-flight manual sync run with no deadline
// at all, and a reconciliation long enough to outlast any grace period is still
// cut off — safely, because the store is in WAL mode, but at an arbitrary point
// rather than at a boundary the service chose.
func TestTheComposeFileOutwaitsTheServiceShutdown(t *testing.T) {
	grace := composeStopGracePeriod(t)
	drain := serviceShutdownTimeout(t)

	assert.Greater(t, grace, drain,
		"docs/compose.example.yml gives the container %s to stop, and cmd/domestique/main.go "+
			"spends up to %s draining its listeners before it starts waiting on anything else",
		grace, drain)
}

// composeStopGracePeriod returns how long the documented deployment lets the
// container take to stop.
func composeStopGracePeriod(t *testing.T) time.Duration {
	t.Helper()

	compose := readRepositoryFile(t, "docs/compose.example.yml")

	match := regexp.MustCompile(`(?m)^\s*stop_grace_period: (\S+)$`).FindStringSubmatch(compose)
	require.Len(t, match, 2, "the compose file must declare a stop_grace_period")

	period, err := time.ParseDuration(match[1])
	require.NoError(t, err)

	return period
}

// serviceShutdownTimeout returns the budget the service gives its own listeners.
func serviceShutdownTimeout(t *testing.T) time.Duration {
	t.Helper()

	main := readRepositoryFile(t, "cmd/domestique/main.go")

	match := regexp.MustCompile(`shutdownTimeout\s+= (\d+) \* time\.Second`).FindStringSubmatch(main)
	require.Len(t, match, 2, "cmd/domestique/main.go must state its shutdown timeout in whole seconds")

	seconds, err := strconv.Atoi(match[1])
	require.NoError(t, err)

	return time.Duration(seconds) * time.Second
}

// The deploy gate has to ask whether the new image can read its state, not only
// whether it answers HTTP, and it has to ask over loopback.
func TestTheDeployScriptGatesOnReadiness(t *testing.T) {
	script := readRepositoryFile(t, "deploy/domestique-deploy.sh")

	assert.Contains(t, script, "READY_URL:-http://127.0.0.1:"+readinessPort+"/readyz")
	assert.Contains(t, script, "wait_healthy && wait_ready && loopback_only")
}

// Superseded images are pruned by the digest each image carries, not by the one
// the listing prints. `docker images` leaves that column empty for an image
// whose tag has moved on, which is every image the prune is for, so a prune
// reading the column removed nothing and the host accumulated a deployment's
// image on every deploy.
func TestTheDeployScriptPrunesByRepositoryDigest(t *testing.T) {
	script := readRepositoryFile(t, "deploy/domestique-deploy.sh")

	assert.Contains(t, script, "{{range .RepoDigests}}")
	assert.NotContains(t, script, "{{.Repository}} {{.Digest}}")
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

// The container smoke test is only worth running while it runs the container an
// operator deploys. Each pair below is one line of the documented runtime and the
// flag that reproduces it, so relaxing either file fails here rather than quietly
// leaving the smoke test asserting a healthy service inside a softer container
// than the one it stands for. It is asserted here because this is where the
// deployment files are held to each other, and because the probe the smoke test
// waits for is this package's.
func TestTheContainerSmokeTestRunsTheDocumentedRuntime(t *testing.T) {
	// The compose file is compared with its whitespace collapsed, so a pair can
	// name a clause that spans two lines of YAML — dropping capabilities is one —
	// without depending on how deeply it happens to be indented.
	compose := strings.Join(strings.Fields(readRepositoryFile(t, "docs/compose.example.yml")), " ")
	smoke := readRepositoryFile(t, "dev/container-smoke.sh")

	for name, pair := range map[string]struct{ documented, asserted string }{
		"unprivileged user":   {`user: "65532:65532"`, `IMAGE_USER="65532:65532"`},
		"read-only root":      {"read_only: true", "--read-only"},
		"no capabilities":     {"cap_drop: - ALL", "--cap-drop ALL"},
		"no new privileges":   {"no-new-privileges:true", "--security-opt no-new-privileges"},
		"temporary directory": {"/tmp:mode=1777,nosuid,nodev,noexec", "--tmpfs /tmp:mode=1777,nosuid,nodev,noexec"},
		"writable state":      {"/var/lib/domestique", `STATE_PATH="/var/lib/domestique"`},
		"read-only config":    {"/etc/domestique/config.toml:ro", `--volume "${CONFIG_FILE}:${CONFIG_PATH}:ro"`},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Contains(t, compose, pair.documented, "the documented deployment no longer says this")
			assert.Contains(t, smoke, pair.asserted, "the smoke test no longer runs the container this way")
		})
	}
}

// Both probes, on both listeners. A smoke test that asked only for liveness would
// pass a container that answers HTTP and cannot read its own state, which is the
// deployment failure the readiness probe exists to catch.
func TestTheContainerSmokeTestProbesBothListeners(t *testing.T) {
	smoke := readRepositoryFile(t, "dev/container-smoke.sh")

	assert.Contains(t, smoke, "/healthz")
	assert.Contains(t, smoke, "/readyz")
	assert.Contains(t, smoke, `readiness_address = ":`+readinessPort+`"`)
	assert.Contains(t, smoke, `:${READINESS_PORT}:`+readinessPort)
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
