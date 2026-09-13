//go:build claude_acceptance

package claude_test

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/claude"
)

// TestClaudeAcceptance runs one real prompt through a real executable on the
// operator's subscription: the token, the flags and the result document are
// facts no fixture can vouch for. It prints neither the prompt nor the answer.
func TestClaudeAcceptance(t *testing.T) {
	token := os.Getenv("DOMESTIQUE_CLAUDE_TOKEN")
	if token == "" {
		t.Skip("DOMESTIQUE_CLAUDE_TOKEN is required for the Claude acceptance check")
	}
	executable := os.Getenv("DOMESTIQUE_CLAUDE_EXECUTABLE")
	if executable == "" {
		found, err := exec.LookPath("claude")
		require.NoError(t, err, "set DOMESTIQUE_CLAUDE_EXECUTABLE or put claude on PATH")
		executable = found
	}

	client, err := claude.New(claude.Options{Executable: executable, Home: t.TempDir(), Token: []byte(token)})
	require.NoError(t, err)

	answer, err := client.Ask(context.Background(), "Reply with one short sentence about cycling.")
	require.NoError(t, err)
	assert.NotEmpty(t, answer.Text)
}
