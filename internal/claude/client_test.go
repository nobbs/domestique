package claude

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The test binary stands in for the executable: run with the adapter's first
// argument, it plays the scenario written into its working directory.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == "--print" {
		os.Exit(playScenario())
	}
	os.Exit(m.Run())
}

type scenario struct {
	Stdout string        `json:"stdout"`
	Exit   int           `json:"exit"`
	Sleep  time.Duration `json:"sleep"`
}

type invocation struct {
	Stdin string   `json:"stdin"`
	Args  []string `json:"args"`
	Env   []string `json:"env"`
}

func playScenario() int {
	contents, err := os.ReadFile("scenario.json")
	if err != nil {
		return 99
	}
	var played scenario
	if json.Unmarshal(contents, &played) != nil {
		return 98
	}
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		return 97
	}
	seen, err := json.Marshal(invocation{Args: os.Args[1:], Env: os.Environ(), Stdin: string(stdin)})
	if err != nil || os.WriteFile("invocation.json", seen, 0o600) != nil {
		return 96
	}
	time.Sleep(played.Sleep)
	if _, err := os.Stdout.WriteString(played.Stdout); err != nil {
		return 95
	}

	return played.Exit
}

func newTestClient(t *testing.T, played scenario, timeout time.Duration) (client *Client, home string) {
	t.Helper()

	home = t.TempDir()
	contents, err := json.Marshal(played)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(home, "scenario.json"), contents, 0o600))
	executable, err := os.Executable()
	require.NoError(t, err)
	client, err = New(Options{Executable: executable, Home: home, Token: []byte("oauth-token"), Timeout: timeout})
	require.NoError(t, err)

	return client, home
}

func readInvocation(t *testing.T, home string) invocation {
	t.Helper()

	contents, err := os.ReadFile(filepath.Join(home, "invocation.json")) //nolint:gosec // G304: a test's own temporary directory.
	require.NoError(t, err)
	var seen invocation
	require.NoError(t, json.Unmarshal(contents, &seen))

	return seen
}

func resultDocument(t *testing.T, fields map[string]any) string {
	t.Helper()

	fields["type"] = "result"
	contents, err := json.Marshal(fields)
	require.NoError(t, err)

	return string(contents)
}

func TestAskReturnsTheAnswerAndRunsTheExecutableConfined(t *testing.T) {
	t.Setenv("DOMESTIQUE_INHERITED_SENTINEL", "leaked")
	client, home := newTestClient(t, scenario{
		Stdout: resultDocument(t, map[string]any{"is_error": false, "result": "  A steady endurance ride.\n"}),
	}, 0)

	answer, err := client.Ask(context.Background(), "What about this ride?")
	require.NoError(t, err)
	assert.Equal(t, Answer{Text: "A steady endurance ride.", Model: Model}, answer)

	seen := readInvocation(t, home)
	assert.Equal(t, "What about this ride?", seen.Stdin, "the prompt travels on standard input")
	assert.Equal(t, []string{
		"--print", "--output-format", "json", "--model", Model,
		"--tools", "", "--no-session-persistence", "--strict-mcp-config",
	}, seen.Args)
	assert.ElementsMatch(t, []string{
		"HOME=" + home,
		"CLAUDE_CODE_OAUTH_TOKEN=oauth-token",
		"DISABLE_AUTOUPDATER=1",
		"DISABLE_TELEMETRY=1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1",
	}, withoutPlatformEnvironment(seen.Env), "the child inherits nothing from the service")
}

// macOS adds its own variables to every process; none of them is inherited.
func withoutPlatformEnvironment(env []string) []string {
	kept := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, "__CF") && !strings.HasPrefix(entry, "PWD=") {
			kept = append(kept, entry)
		}
	}

	return kept
}

func TestAskMapsFailuresToACategory(t *testing.T) {
	tests := []struct {
		name   string
		played func(t *testing.T) scenario
		want   Category
	}{
		{"a refused token", failedWith(401), CategoryToken},
		{"a forbidden token", failedWith(403), CategoryToken},
		{"an exhausted allowance", failedWith(429), CategoryAllowance},
		{"a server error", failedWith(529), CategoryExecutable},
		{"an error with no status", failedWith(0), CategoryExecutable},
		{"no document", func(*testing.T) scenario { return scenario{Exit: 1} }, CategoryExecutable},
		{"a document of another type", func(*testing.T) scenario {
			return scenario{Stdout: `{"type":"system","result":"hello"}`}
		}, CategoryExecutable},
		{"a non-zero exit beside a result", func(t *testing.T) scenario {
			return scenario{Stdout: resultDocument(t, map[string]any{"result": "hello"}), Exit: 2}
		}, CategoryExecutable},
		{"a blank answer", func(t *testing.T) scenario {
			return scenario{Stdout: resultDocument(t, map[string]any{"result": " \n"})}
		}, CategoryUnusable},
		{"output past the bound", func(t *testing.T) scenario {
			return scenario{Stdout: resultDocument(t, map[string]any{"result": strings.Repeat("x", maximumOutputBytes)})}
		}, CategoryExecutable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client, _ := newTestClient(t, test.played(t), 0)

			_, err := client.Ask(context.Background(), "prompt")
			var failure *Error
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, test.want, failure.Category)
		})
	}
}

func failedWith(status int) func(t *testing.T) scenario {
	return func(t *testing.T) scenario {
		t.Helper()

		fields := map[string]any{"is_error": true, "result": "API Error"}
		if status != 0 {
			fields["api_error_status"] = status
		}

		return scenario{Stdout: resultDocument(t, fields), Exit: 1}
	}
}

func TestAskErrorCarriesNothingTheExecutablePrinted(t *testing.T) {
	client, _ := newTestClient(t, scenario{
		Stdout: resultDocument(t, map[string]any{"is_error": true, "result": "private-answer-text", "api_error_status": 500}),
		Exit:   1,
	}, 0)

	_, err := client.Ask(context.Background(), "private-prompt-text")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private-answer-text")
	assert.NotContains(t, err.Error(), "private-prompt-text")
	assert.NotContains(t, err.Error(), "oauth-token")
}

func TestAskGivesUpAtTheTimeout(t *testing.T) {
	client, _ := newTestClient(t, scenario{
		Stdout: resultDocument(t, map[string]any{"result": "too late"}),
		Sleep:  time.Minute,
	}, 200*time.Millisecond)

	_, err := client.Ask(context.Background(), "prompt")
	var failure *Error
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, CategoryExecutable, failure.Category)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestNewRejectsIncompleteOptions(t *testing.T) {
	valid := Options{Executable: "/usr/local/bin/claude", Home: "/var/lib/domestique/claude", Token: []byte("token")}
	tests := map[string]func(*Options){
		"a relative executable": func(o *Options) { o.Executable = "claude" },
		"a relative home":       func(o *Options) { o.Home = "claude" },
		"no token":              func(o *Options) { o.Token = nil },
		"a negative timeout":    func(o *Options) { o.Timeout = -time.Second },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			options := valid
			mutate(&options)

			_, err := New(options)
			assert.Error(t, err)
		})
	}

	_, err := New(valid)
	assert.NoError(t, err)
}
