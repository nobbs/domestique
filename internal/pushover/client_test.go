package pushover

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientSendsFormNotification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		// The handler runs on the server's goroutine, where FailNow is unsafe,
		// so every check here is an assert and the parse failure returns early.
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/1/messages.json", request.URL.Path)
		if !assert.NoError(t, request.ParseForm()) {
			return
		}
		for key, want := range map[string]string{
			"token":   "application-token",
			"user":    "user-key",
			"title":   "Domestique sync",
			"message": "succeeded: 2 created, 0 updated, 1 deleted",
		} {
			assert.Equal(t, want, request.Form.Get(key), "form field %q", key)
		}
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"status":1}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	require.NoError(t, client.Send(t.Context(), "Domestique sync", "succeeded: 2 created, 0 updated, 1 deleted"))
}

func TestClientHidesProviderFailureDetails(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeResponse(t, writer, http.StatusBadRequest, "private provider failure")
	}))
	defer server.Close()

	err := newTestClient(t, server).Send(t.Context(), "Domestique sync", "failed: destination")
	require.Error(t, err)
	assert.NotContains(t, err.Error(), "private provider failure", "Send() repeated the provider's body")
	assert.NotContains(t, err.Error(), "application-token", "Send() exposed the application token")
}

func TestClientRejectsUnacceptedSuccessResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"status":0}`)
	}))
	defer server.Close()

	err := newTestClient(t, server).Send(t.Context(), "Domestique sync", "failed: destination")
	assert.EqualError(t, err, "pushover: notification was not accepted")
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		BaseURL:          func() string { return server.URL },
		ApplicationToken: func() []byte { return []byte("application-token") },
		UserKey:          func() []byte { return []byte("user-key") },
		Timeout:          time.Second,
		Transport:        server.Client().Transport,
	})
	require.NoError(t, err)

	return client
}

func writeResponse(t *testing.T, writer http.ResponseWriter, status int, body string) {
	t.Helper()
	writer.WriteHeader(status)
	_, err := writer.Write([]byte(body))
	assert.NoError(t, err, "writing the response")
}

// The base URL an operator edits takes effect on the next notification, not on
// the next restart, so it cannot be resolved once at construction.
func TestClientReadsItsBaseURLAgainBeforeEachSend(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests++
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"status":1}`)
	}))
	defer server.Close()

	baseURL := "https://api.pushover.net"
	client, err := New(&Options{
		BaseURL:          func() string { return baseURL },
		ApplicationToken: func() []byte { return []byte("application-token") },
		UserKey:          func() []byte { return []byte("user-key") },
		Timeout:          time.Second,
		Transport:        server.Client().Transport,
	})
	require.NoError(t, err)

	baseURL = server.URL
	require.NoError(t, client.Send(t.Context(), "Domestique sync", "succeeded"), "Send()")
	assert.Equal(t, 1, requests, "the send went somewhere other than the edited base url")
}
