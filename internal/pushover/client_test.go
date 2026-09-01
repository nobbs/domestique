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

func TestNewRequiresConfigurationAndDefaultsItsBaseURL(t *testing.T) {
	_, err := New(nil)
	require.EqualError(t, err, "pushover: options and configuration are required")

	client, err := New(&Options{Configuration: func() Configuration { return Configuration{} }})
	require.NoError(t, err)
	assert.NotNil(t, client)
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		Configuration: func() Configuration {
			return Configuration{
				BaseURL:          server.URL,
				ApplicationToken: []byte("application-token"),
				UserKey:          []byte("user-key"),
			}
		},
		Timeout:   time.Second,
		Transport: server.Client().Transport,
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

// One callback supplies the destination and both credentials so a save cannot
// split one notification across two settings generations.
func TestClientReadsOneConfigurationBeforeEachSend(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if !assert.NoError(t, request.ParseForm()) {
			return
		}
		assert.Equal(t, "new-token", request.Form.Get("token"))
		assert.Equal(t, "new-user", request.Form.Get("user"))
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"status":1}`)
	}))
	defer server.Close()

	reads := 0
	configuration := Configuration{
		BaseURL:          "https://api.pushover.net",
		ApplicationToken: []byte("old-token"),
		UserKey:          []byte("old-user"),
	}
	client, err := New(&Options{
		Configuration: func() Configuration {
			reads++

			return configuration
		},
		Timeout:   time.Second,
		Transport: server.Client().Transport,
	})
	require.NoError(t, err)

	configuration = Configuration{
		BaseURL:          server.URL,
		ApplicationToken: []byte("new-token"),
		UserKey:          []byte("new-user"),
	}
	require.NoError(t, client.Send(t.Context(), "Domestique sync", "succeeded"), "Send()")
	assert.Equal(t, 1, requests, "the send went somewhere other than the edited base url")
	assert.Equal(t, 2, reads, "a send read more than one settings generation")
}
