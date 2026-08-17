package webui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestNewReportsAnUnbuiltBundle(t *testing.T) {
	_, err := newFromFS(fstest.MapFS{"app/dist/.gitkeep": {}}, bundleRoot)
	if !errors.Is(err, ErrBundleMissing) {
		t.Errorf("newFromFS() error = %v, want %v", err, ErrBundleMissing)
	}
}

func TestAssetsServeTheBundle(t *testing.T) {
	assets, err := newFromFS(fstest.MapFS{
		"app/dist/index.html":            {Data: []byte("<!doctype html><title>domestique</title>")},
		"app/dist/assets/app-abc123.js":  {Data: []byte("export default null;")},
		"app/dist/assets/app-abc123.css": {Data: []byte(":root{}")},
	}, bundleRoot)
	if err != nil {
		t.Fatalf("newFromFS() error = %v", err)
	}

	index := httptest.NewRecorder()
	assets.Index(index, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/routes/1/1", http.NoBody))
	if got := index.Body.String(); !strings.Contains(got, "<!doctype html>") {
		t.Errorf("Index() body = %q, want the entry document", got)
	}

	asset := httptest.NewRecorder()
	assets.Static(asset, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/assets/app-abc123.js", http.NoBody))
	if got, want := asset.Code, http.StatusOK; got != want {
		t.Errorf("Static() status = %d, want %d", got, want)
	}
	if got := asset.Body.String(); !strings.Contains(got, "export default null;") {
		t.Errorf("Static() body = %q, want the bundled asset", got)
	}

	missing := httptest.NewRecorder()
	assets.Static(missing, httptest.NewRequestWithContext(
		t.Context(), http.MethodGet, "/assets/absent.js", http.NoBody))
	if got, want := missing.Code, http.StatusNotFound; got != want {
		t.Errorf("Static() missing status = %d, want %d", got, want)
	}
}
