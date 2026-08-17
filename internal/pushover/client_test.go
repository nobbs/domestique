package pushover

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientSendsFormNotification(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if got, want := request.Method, http.MethodPost; got != want {
			t.Errorf("method = %q, want %q", got, want)
		}
		if got, want := request.URL.Path, "/1/messages.json"; got != want {
			t.Errorf("path = %q, want %q", got, want)
		}
		if err := request.ParseForm(); err != nil {
			t.Errorf("ParseForm() error = %v", err)
			return
		}
		for key, want := range map[string]string{
			"token":   "application-token",
			"user":    "user-key",
			"title":   "Domestique sync",
			"message": "succeeded: 2 created, 0 updated, 1 deleted",
		} {
			if got := request.Form.Get(key); got != want {
				t.Errorf("form %q = %q, want %q", key, got, want)
			}
		}
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"status":1}`)
	}))
	defer server.Close()

	client := newTestClient(t, server)
	if err := client.Send(t.Context(), "Domestique sync", "succeeded: 2 created, 0 updated, 1 deleted"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
}

func TestClientHidesProviderFailureDetails(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writeResponse(t, writer, http.StatusBadRequest, "private provider failure")
	}))
	defer server.Close()

	err := newTestClient(t, server).Send(t.Context(), "Domestique sync", "failed: destination")
	if err == nil {
		t.Fatal("Send() error = nil, want failure")
	}
	if strings.Contains(err.Error(), "private provider failure") || strings.Contains(err.Error(), "application-token") {
		t.Errorf("Send() exposed sensitive detail: %q", err)
	}
}

func TestClientRejectsUnacceptedSuccessResponse(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		writeResponse(t, writer, http.StatusOK, `{"status":0}`)
	}))
	defer server.Close()

	err := newTestClient(t, server).Send(t.Context(), "Domestique sync", "failed: destination")
	if got, want := err.Error(), "pushover: notification was not accepted"; got != want {
		t.Errorf("Send() error = %v, want rejected notification", err)
	}
}

func newTestClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	client, err := New(&Options{
		BaseURL:          server.URL,
		ApplicationToken: []byte("application-token"),
		UserKey:          []byte("user-key"),
		Timeout:          time.Second,
		Transport:        server.Client().Transport,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	return client
}

func writeResponse(t *testing.T, writer http.ResponseWriter, status int, body string) {
	t.Helper()
	writer.WriteHeader(status)
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Errorf("writing response: %v", err)
	}
}
