package httpapi

import (
	"net/http"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
)

// GetWebUIConfig hands the page its runtime settings so the built assets stay
// static and cacheable. It exposes no secret. The whole basemap list is sent
// rather than one: the reader's choice and their colour scheme both belong to
// the browser, and this response is cached across the session. Each source's
// base URL rides along so the page can link a stage back to its source route.
func (h *Handler) GetWebUIConfig(writer http.ResponseWriter, request *http.Request) {
	// An unset dark style and an unset dark-cartography flag are both absent
	// from the response rather than sent as an empty string or false, which is
	// how the page tells "keep this entry's one style in both colour schemes"
	// from a value it was given.
	live := h.settings.Values().Basemaps
	basemaps := make([]openapi.BrowserBasemap, len(live))
	for index, basemap := range live {
		basemaps[index] = openapi.BrowserBasemap{
			Name:            basemap.Name,
			StyleURL:        basemap.StyleURL,
			StyleURLDark:    optionalString(basemap.StyleURLDark),
			DarkCartography: optionalBool(basemap.DarkCartography),
		}
	}

	// Who the gate admitted, so a reader can see which session they are in.
	identity := identityOf(request.Context())
	config := openapi.WebUIConfig{
		Basemaps: basemaps,
		Identity: openapi.BrowserIdentity{
			Display: identity.Display,
			Admin:   identity.Admin,
		},
	}
	// Omitted entirely when no source named one, so the page offers no link at
	// all rather than building a broken one.
	if sources := h.settings.Values().Sources; len(sources) > 0 {
		config.SourceBaseUrls = &openapi.SourceBaseUrls{
			Komoot:      optionalString(sourceBaseURL(sources, route.ProviderKomoot)),
			Veloplanner: optionalString(sourceBaseURL(sources, route.ProviderVeloPlanner)),
		}
	}
	h.writeJSON(writer, http.StatusOK, config)
}

// index serves the application entry document for every UI route, so a deep
// link such as /routes/12/1 loads the app and lets it route client-side.
func (h *Handler) index(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", cacheDocument)
	h.assets.Index(writer, request)
}

// GetManifest serves the manifest that makes the UI installable, which is how the
// map runs edge to edge on a phone: iOS 26 Safari lays a tab out between its own
// chrome and reports no safe-area insets, while a Home Screen copy gets the whole
// screen. The type is set here because Go's table does not know .webmanifest and
// responses carry nosniff.
func (h *Handler) GetManifest(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/manifest+json")
	h.stableAsset(writer, request)
}

// stableAsset serves a build artefact addressed by a fixed name rather than a
// content hash: the favicon, the installed copy's icons, the manifest naming
// them. A new icon arrives at the URL the old one had, so these revalidate rather
// than being cached the way hashed output is.
func (h *Handler) stableAsset(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", cacheDocument)
	h.assets.Static(writer, request)
}

// GetAsset serves a build artefact the bundler content-hashed. A change of
// content is a change of name, so these may be cached indefinitely; anything
// served under a name the bundler did not hash belongs on stableAsset above.
func (h *Handler) GetAsset(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", cacheImmutable)
	h.assets.Static(writer, request)
}

// GetWorkerAsset serves the map's worker bundle, content-hashed and cached like
// anything under /assets/ but served from its own directory because that one is
// public.
//
// A worker is governed by the Content-Security-Policy on its own response, not
// by the document's. Served as a public asset it is handed a policy naming no
// tile origin, and every tile fetch it makes — which is all of them, because
// MapLibre loads tiles in the worker — is refused, leaving a map that draws its
// ground and nothing on it. Behind the identity gate it is sent the same policy
// the page is.
// It keeps the blanket Vary: Cookie that every gated answer carries, unlike the
// public assets that drop it. The bytes are the same for every caller, but the
// answer is not: without an identity this path is a 401, and a cache free to
// ignore the cookie could serve either one to the wrong caller. The cost is a
// re-fetch when the session cookie changes, which is once per sign-in.
func (h *Handler) GetWorkerAsset(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Cache-Control", cacheImmutableGated)
	h.assets.Static(writer, request)
}

// GetIndex serves the application document at the root. It and the four
// methods below are separate operations because each is a page a reader can be
// linked straight to; they serve the same document because the routing that
// follows is the application's own.
func (h *Handler) GetIndex(writer http.ResponseWriter, request *http.Request) {
	h.index(writer, request)
}

// GetRoutePage serves the application document for one stage's address.
func (h *Handler) GetRoutePage(writer http.ResponseWriter, request *http.Request) {
	h.index(writer, request)
}

// GetCataloguePage serves the application document for the catalogue view.
func (h *Handler) GetCataloguePage(writer http.ResponseWriter, request *http.Request) {
	h.index(writer, request)
}

// GetSettingsPage serves the application document for the settings view.
func (h *Handler) GetSettingsPage(writer http.ResponseWriter, request *http.Request) {
	h.index(writer, request)
}

// GetTasksPage serves the application document for the task history view.
func (h *Handler) GetTasksPage(writer http.ResponseWriter, request *http.Request) {
	h.index(writer, request)
}

// GetAdminPage serves the application document for the service administration
// view. A non-admin is answered not found: a document is not an API operation.
func (h *Handler) GetAdminPage(writer http.ResponseWriter, request *http.Request) {
	h.adminPage(writer, request)
}

// GetAdminTasksPage serves the application document for the task
// administration view.
func (h *Handler) GetAdminTasksPage(writer http.ResponseWriter, request *http.Request) {
	h.adminPage(writer, request)
}

func (h *Handler) adminPage(writer http.ResponseWriter, request *http.Request) {
	if !identityOf(request.Context()).Admin {
		h.notFound(writer)

		return
	}
	h.index(writer, request)
}

// GetSyncPage serves the application document for the synchronization view.
func (h *Handler) GetSyncPage(writer http.ResponseWriter, request *http.Request) {
	h.index(writer, request)
}

// GetFavicon serves the tab icon. It and the two methods below are separate
// operations because the contract names each file the manifest may point at.
func (h *Handler) GetFavicon(writer http.ResponseWriter, request *http.Request) {
	h.stableAsset(writer, request)
}

// GetIcon256 serves the smaller installed-application icon.
func (h *Handler) GetIcon256(writer http.ResponseWriter, request *http.Request) {
	h.stableAsset(writer, request)
}

// GetIcon512 serves the larger installed-application icon.
func (h *Handler) GetIcon512(writer http.ResponseWriter, request *http.Request) {
	h.stableAsset(writer, request)
}
