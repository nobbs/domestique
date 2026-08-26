package webui

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewReportsAnUnbuiltBundle(t *testing.T) {
	// The tree as a fresh clone has it: the committed placeholder, and no
	// bundle directory for the bundler to have written yet.
	_, err := newFromFS(fstest.MapFS{"app/dist/.gitkeep": {}}, bundleRoot)
	require.ErrorIs(t, err, ErrBundleMissing)
}

func TestAssetsServeTheBundle(t *testing.T) {
	assets, err := newFromFS(fstest.MapFS{
		"app/dist/bundle/index.html":            {Data: []byte("<!doctype html><title>domestique</title>")},
		"app/dist/bundle/assets/app-abc123.js":  {Data: []byte("export default null;")},
		"app/dist/bundle/assets/app-abc123.css": {Data: []byte(":root{}")},
	}, bundleRoot)
	require.NoError(t, err)

	index := httptest.NewRecorder()
	assets.Index(index, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/routes/1/1", http.NoBody))
	assert.Contains(t, index.Body.String(), "<!doctype html>", "Index() did not serve the entry document")

	asset := httptest.NewRecorder()
	assets.Static(asset, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/assets/app-abc123.js", http.NoBody))
	assert.Equal(t, http.StatusOK, asset.Code)
	assert.Contains(t, asset.Body.String(), "export default null;", "Static() did not serve the bundled asset")

	missing := httptest.NewRecorder()
	assets.Static(missing, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/assets/absent.js", http.NoBody))
	assert.Equal(t, http.StatusNotFound, missing.Code)
}
