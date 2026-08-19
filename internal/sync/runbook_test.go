package sync

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The runbook is written against the categories this package emits, and those
// are the only words an operator ever sees for a run that did not succeed. A
// category added here without an entry there leaves the one document that
// explains it silently incomplete, which is exactly the failure a runbook exists
// to prevent.
func TestTheRunbookExplainsEveryFailureCategory(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	contents, err := os.ReadFile(filepath.Join(root, "docs", "runbook.md")) //nolint:gosec // a repository document, named by this test
	require.NoError(t, err)
	runbook := string(contents)

	for _, category := range []FailureCategory{
		FailureState,
		FailureSource,
		FailureAuthorization,
		FailureDestination,
		FailureCourse,
		FailureEmptySource,
		FailureDeletionLimit,
	} {
		assert.Contains(t, runbook, "`"+string(category)+"`",
			"docs/runbook.md does not explain the %q failure category", category)
	}
}
