package httpapi

import (
	"net/http"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
)

// GetWebUIConfig hands the page its runtime settings so the built assets stay
// static and fully cacheable. It exposes no secret: the tile style URLs are
// operator-chosen configuration that the browser must know to render a map.
//
// The whole list is sent rather than the service picking one, for two reasons
// that are really the same reason. Which cartography to show is the reader's
// choice, remembered in their browser; and which of an entry's two styles to
// load is their system's colour scheme. Both are properties of the browser the
// operator is sitting at, and this response is cached across every page in the
// session, so neither can be resolved here.
//
// Each configured source's base URL rides along for the same reason: the page
// builds the link back to a stage's source route from it, keyed by the
// provider that stage names, and the alternative — the service putting a route
// URL on every stage it serves — would be the same fact repeated once per
// stage.
func (h *Handler) GetWebUIConfig(writer http.ResponseWriter, _ *http.Request) {
	// An unset dark style and an unset dark-cartography flag are both absent
	// from the response rather than sent as an empty string or false, which is
	// how the page tells "keep this entry's one style in both colour schemes"
	// from a value it was given.
	basemaps := make([]openapi.BrowserBasemap, len(h.basemaps))
	for index, basemap := range h.basemaps {
		basemaps[index] = openapi.BrowserBasemap{
			Name:            basemap.Name,
			StyleURL:        basemap.StyleURL,
			StyleURLDark:    optionalString(basemap.StyleURLDark),
			DarkCartography: optionalBool(basemap.DarkCartography),
		}
	}

	config := openapi.WebUIConfig{Basemaps: basemaps}
	// Omitted entirely when no source named one, so the page offers no link at
	// all rather than building a broken one.
	if len(h.sourceBaseURLs) > 0 {
		config.SourceBaseUrls = &openapi.SourceBaseUrls{
			Komoot:      optionalString(h.sourceBaseURLs[route.ProviderKomoot]),
			Veloplanner: optionalString(h.sourceBaseURLs[route.ProviderVeloPlanner]),
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

// GetManifest serves the manifest that makes the UI installable, which is how
// the map runs edge to edge on a phone: iOS 26 Safari lays a tab out between
// its own chrome and reports no safe-area insets, while the same document added
// to the Home Screen gets the whole screen.
//
// The type is set here because Go's table does not know .webmanifest and the
// responses carry X-Content-Type-Options: nosniff. The caching is the stable
// kind for the reason below, and doubly so for this file: it decides how an
// installed copy launches.
func (h *Handler) GetManifest(writer http.ResponseWriter, request *http.Request) {
	writer.Header().Set("Content-Type", "application/manifest+json")
	h.stableAsset(writer, request)
}

// stableAsset serves a build artefact that is addressed by a fixed name rather
// than a content hash: the favicon, the icons an installed copy is given, the
// manifest that names them.
//
// The name staying the same is the whole point of these — a manifest may only
// point at a path it can rely on — and it is also why they cannot be cached the
// way the hashed output is. A new icon arrives at the URL the old one had, so a
// year of immutable caching is a year of the Home Screen showing the previous
// one. They revalidate instead, which costs a conditional request and keeps the
// installed copy honest.
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

// GetIndex serves the application document at the root. It and the three
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

// GetSettingsPage serves the application document for the settings view.
func (h *Handler) GetSettingsPage(writer http.ResponseWriter, request *http.Request) {
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
