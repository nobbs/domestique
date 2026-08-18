package httpapi

import "net/http"

// webUIConfig hands the page its runtime settings so the built assets stay
// static and fully cacheable. It exposes no secret: the tile style URLs are
// operator-chosen configuration that the browser must know to render a map.
//
// Both styles are sent rather than the service picking one, because the colour
// scheme is a property of the browser the operator is sitting at and this
// response is cached across every page in the session.
func (h *Handler) webUIConfig(writer http.ResponseWriter, _ *http.Request, _ string) {
	h.writeJSON(writer, http.StatusOK, webUIConfigView{
		TileStyleURL:     h.tileStyleURL,
		TileStyleURLDark: h.tileStyleURLDark,
	})
}

// index serves the application entry document for every UI route, so a deep
// link such as /routes/12/1 loads the app and lets it route client-side.
func (h *Handler) index(writer http.ResponseWriter, request *http.Request, _ string) {
	writer.Header().Set("Cache-Control", cacheDocument)
	h.assets.Index(writer, request)
}

// staticAsset serves a build artefact. Names are content-hashed by the bundler,
// so they may be cached indefinitely.
func (h *Handler) staticAsset(writer http.ResponseWriter, request *http.Request, _ string) {
	writer.Header().Set("Cache-Control", cacheImmutable)
	h.assets.Static(writer, request)
}
