package main

import (
	"bytes"
	"log/slog"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstallLoggingWritesJSONAtTheLevelItIsGiven(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	var output bytes.Buffer

	level := installLogging(&output)
	slog.Debug("hidden")
	level.Set(slog.LevelDebug)
	slog.Debug("shown", "count", 1)

	assert.NotContains(t, output.String(), "hidden")
	assert.Contains(t, output.String(), `"msg":"shown","count":1}`)
}

func TestNewServerRoutesItsOwnComplaintsThroughTheDefaultLogger(t *testing.T) {
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	var output bytes.Buffer
	installLogging(&output)

	server := newServer(":0", http.NotFoundHandler())
	require.NotNil(t, server.ErrorLog)
	server.ErrorLog.Print("handshake failed")

	assert.Equal(t, ":0", server.Addr)
	assert.Equal(t, httpReadHeaderTimeout, server.ReadHeaderTimeout)
	assert.Contains(t, output.String(), `"level":"WARN","msg":"handshake failed"`)
}
