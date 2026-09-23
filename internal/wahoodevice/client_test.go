package wahoodevice

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)
	client, err := New(&Options{BaseURL: server.URL, Transport: server.Client().Transport})
	require.NoError(t, err)

	return client
}

func writeBody(t *testing.T, writer http.ResponseWriter, body string) {
	t.Helper()
	_, err := writer.Write([]byte(body))
	assert.NoError(t, err)
}

func TestClientSignsInWithFlatFormFields(t *testing.T) {
	client := newTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPost, request.Method)
		assert.Equal(t, "/api/v1/sessions/", request.URL.Path)
		assert.Equal(t, "1", request.Header.Get("WF-FITNESS-APP-ID"))
		assert.Empty(t, request.Header.Get("WF-USER-TOKEN"))
		assert.Contains(t, request.Header.Get("User-Agent"), "ELEMNT")
		assert.NoError(t, request.ParseForm())
		assert.Equal(t, "rider@example.com", request.PostForm.Get("email"))
		assert.Equal(t, "secret", request.PostForm.Get("password"))
		writeBody(t, writer, `{"id":7,"token":"session-token"}`)
	})

	token, err := client.SignIn(t.Context(), []byte("rider@example.com"), []byte("secret"))
	require.NoError(t, err)
	assert.Equal(t, "session-token", token)
}

func TestClientReportsARefusedSignInAsUnauthorized(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusUnprocessableEntity} {
		client := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(status)
			writeBody(t, writer, `{"error":"secret-bearing body"}`)
		})

		_, err := client.SignIn(t.Context(), []byte("rider@example.com"), []byte("wrong"))
		require.Error(t, err)
		assert.True(t, client.IsUnauthorized(err), "HTTP %d", status)
		assert.NotContains(t, err.Error(), "secret-bearing")
	}
}

func TestClientRefusesASignInWithoutAToken(t *testing.T) {
	client := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writeBody(t, writer, `{"id":7}`)
	})

	_, err := client.SignIn(t.Context(), []byte("rider@example.com"), []byte("secret"))
	require.ErrorContains(t, err, "no token")
}

func TestClientListsRoutesWithEveryProviderIDShape(t *testing.T) {
	client := newTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, "/api/v1/routes", request.URL.Path)
		assert.Equal(t, "0", request.URL.Query().Get("deleted"))
		assert.Equal(t, "session-token", request.Header.Get("WF-USER-TOKEN"))
		writeBody(t, writer, `[
			{"id":1,"external_id":"domestique:veloplanner:1:stage:0","provider_id":null},
			{"id":2,"external_id":null,"provider_id":12345},
			{"id":3,"external_id":"domestique:veloplanner:2:stage:0","provider_id":"3"},
			{"id":0,"external_id":"domestique:veloplanner:3:stage:0","provider_id":null}
		]`)
	})

	routes, err := client.Routes(t.Context(), "session-token")
	require.NoError(t, err)
	assert.Equal(t, []Route{
		{ID: 1, ExternalID: "domestique:veloplanner:1:stage:0"},
		{ID: 2, ProviderID: "12345"},
		{ID: 3, ExternalID: "domestique:veloplanner:2:stage:0", ProviderID: "3"},
	}, routes)
}

func TestClientReportsAnExpiredSessionAsUnauthorized(t *testing.T) {
	client := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	})

	_, err := client.Routes(t.Context(), "stale")
	assert.True(t, client.IsUnauthorized(err))
}

func TestClientSetsAProviderIDWithAMultipartPut(t *testing.T) {
	client := newTestClient(t, func(writer http.ResponseWriter, request *http.Request) {
		assert.Equal(t, http.MethodPut, request.Method)
		assert.Equal(t, "/api/v1/routes/42", request.URL.Path)
		assert.Equal(t, "session-token", request.Header.Get("WF-USER-TOKEN"))
		reader, err := request.MultipartReader()
		if !assert.NoError(t, err) {
			return
		}
		part, err := reader.NextPart()
		if !assert.NoError(t, err) {
			return
		}
		value, err := io.ReadAll(io.LimitReader(part, 64))
		assert.NoError(t, err)
		assert.Equal(t, "route[provider_id]", part.FormName())
		assert.Equal(t, "42", string(value))
		writer.WriteHeader(http.StatusOK)
	})

	require.NoError(t, client.SetProviderID(t.Context(), "session-token", 42, "42"))
}

func TestClientReportsAFailedWriteWithoutItsBody(t *testing.T) {
	client := newTestClient(t, func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnprocessableEntity)
		writeBody(t, writer, `secret-bearing body`)
	})

	err := client.SetProviderID(t.Context(), "session-token", 42, "42")
	require.ErrorContains(t, err, "HTTP 422")
	assert.False(t, client.IsUnauthorized(err), "a refused write is not a refused session")
	assert.NotContains(t, err.Error(), "secret-bearing")
}

func TestNewRefusesAPlaintextBaseURL(t *testing.T) {
	_, err := New(&Options{BaseURL: "http://www.wahooligan.com"})
	require.ErrorContains(t, err, "https origin")
}
