// Package webui embeds the built browser UI and serves it from the binary.
//
// The bundle is compiled in rather than read from disk so the runtime keeps a
// read-only root filesystem and so the UI and the API can only ever ship as one
// signed artefact.
package webui

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
)

// bundleRoot is where the bundler writes its output.
const bundleRoot = "app/dist"

// ErrBundleMissing reports a binary built without its browser UI. The build
// fails loudly rather than serving a placeholder, so a released image can never
// be missing the UI silently.
var ErrBundleMissing = errors.New("browser UI bundle is missing: run `make ui-build` before building the binary")

// The all: prefix keeps the embed valid when the working tree holds only the
// committed .gitkeep, so the package compiles before the UI has been built.
//
//go:embed all:app/dist
var bundle embed.FS

// Assets serves the embedded browser UI build.
type Assets struct {
	files      http.Handler
	indexBytes []byte
}

// New opens the embedded bundle. It fails when the UI has not been built.
func New() (*Assets, error) {
	return newFromFS(bundle, bundleRoot)
}

func newFromFS(files fs.FS, root string) (*Assets, error) {
	rooted, err := fs.Sub(files, root)
	if err != nil {
		return nil, fmt.Errorf("opening embedded browser UI: %w", err)
	}
	indexBytes, err := fs.ReadFile(rooted, "index.html")
	if err != nil {
		return nil, ErrBundleMissing
	}

	return &Assets{files: http.FileServerFS(rooted), indexBytes: indexBytes}, nil
}

// Index writes the application entry document. Every UI route serves the same
// document so the client router can resolve deep links.
func (a *Assets) Index(writer http.ResponseWriter, _ *http.Request) {
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := writer.Write(a.indexBytes); err != nil {
		return
	}
}

// Static serves one hashed build artefact straight from the embedded bundle.
func (a *Assets) Static(writer http.ResponseWriter, request *http.Request) {
	a.files.ServeHTTP(writer, request)
}
