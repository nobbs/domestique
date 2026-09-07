package main

import (
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CI restores the directories the Go toolchain writes its caches to, and two
// files must agree on where those are: `.mise.toml` points the toolchain at them
// and ci.yml names them in a cache step. Drift fails nothing and is invisible in
// a green run, so the agreement is asserted here.
func TestCICachesTheDirectoriesTheToolchainWritesTo(t *testing.T) {
	t.Parallel()

	cached := cachedPaths(t)

	for _, name := range []string{"GOCACHE", "GOLANGCI_LINT_CACHE"} {
		assert.Contains(t, cached, miseCacheDir(t, name),
			"ci.yml must cache the directory .mise.toml sets %s to", name)
	}
}

// miseCacheDir returns one cache directory from .mise.toml's [env] table, as the
// repository-relative path a workflow names. Each variable is a template that
// resolves to the main checkout's root, followed by that path.
func miseCacheDir(t *testing.T, name string) string {
	t.Helper()

	pattern := regexp.MustCompile(fmt.Sprintf(`(?m)^%s = ['"](.*)['"]$`, regexp.QuoteMeta(name)))

	match := pattern.FindStringSubmatch(repositoryFile(t, ".mise.toml"))
	require.Len(t, match, 2, ".mise.toml sets no %s", name)

	// Against the main checkout, never `config_root`: a linked worktree's
	// config_root is the worktree, and each would grow its own copy of the cache.
	assert.Contains(t, match[1], "--git-common-dir",
		"%s must resolve against the main checkout's git directory", name)

	_, path, found := strings.Cut(match[1], "}}/")
	require.True(t, found, "%s must be a template resolving to the repository root, then the path, not %q", name, match[1])

	return path
}

// cachedPaths returns every directory ci.yml's cache steps name in a `path:`
// list. Reading the lists rather than the whole file is what keeps a failure
// readable, and means a directory mentioned in a comment does not count as
// cached.
func cachedPaths(t *testing.T) []string {
	t.Helper()

	lists := regexp.MustCompile(`(?m)^ {10}path: \|\n((?: {12}\S.*\n)+)`).
		FindAllStringSubmatch(repositoryFile(t, ".github/workflows/ci.yml"), -1)
	require.NotEmpty(t, lists, "ci.yml declares no cache step with a path list")

	var paths []string
	for _, list := range lists {
		paths = append(paths, strings.Fields(list[1])...)
	}

	return paths
}
