package httpapi

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/nobbs/domestique/internal/route"
)

// Both redirects here build the current address inline rather than through a
// shared helper. The duplication is deliberate: gosec's taint analysis cannot
// see through a call, so a helper reads as an open redirect whether or not the
// value reaching it came from the request.

// redirectStagePath sends an address from before *route* named the unit to its
// current equivalent, preserving suffix and method. A 308 is used rather than a
// 301 or 302 because the reprocess route is a POST, and only a 308 is defined
// to keep a redirected request's method and body intact.
//
// The identifiers are re-rendered from parsed integers. The provider cannot be —
// it is free text the caller chose — so it is escaped into exactly one path
// segment rather than trusted to stay one.
func (h *Handler) redirectStagePath(writer http.ResponseWriter, request *http.Request, suffix string) {
	sourceRouteID, stageOrder, ok := legacyStagePathValues(request)
	if !ok {
		h.notFound(writer)

		return
	}

	provider := url.PathEscape(request.PathValue("provider"))
	target := "/v1/providers/" + provider + "/sourceRoutes/" +
		strconv.FormatInt(sourceRouteID, 10) + "/routes/" + strconv.Itoa(stageOrder) + suffix
	http.Redirect(writer, request, target, http.StatusPermanentRedirect)
}

// RedirectStageRoute sends a provider-qualified address that still spells the
// unit as a stage to the same route under its current address. It and the two
// methods below differ only in the suffix they preserve.
func (h *Handler) RedirectStageRoute(writer http.ResponseWriter, request *http.Request) {
	h.redirectStagePath(writer, request, "")
}

// RedirectStageGeometry does the same for that route's geometry.
func (h *Handler) RedirectStageGeometry(writer http.ResponseWriter, request *http.Request) {
	h.redirectStagePath(writer, request, "/geometry")
}

// RedirectStageReprocess does the same for that route's reprocess request.
func (h *Handler) RedirectStageReprocess(writer http.ResponseWriter, request *http.Request) {
	h.redirectStagePath(writer, request, "/reprocess")
}

// redirectLegacyStagePath sends an address from before a second provider
// existed straight to the current one, on the same terms. It skips the
// provider-qualified generation above rather than bouncing through it, so an
// old bookmark still costs one hop.
func (h *Handler) redirectLegacyStagePath(writer http.ResponseWriter, request *http.Request, suffix string) {
	sourceRouteID, stageOrder, ok := legacyStagePathValues(request)
	if !ok {
		h.notFound(writer)

		return
	}

	target := "/v1/providers/" + string(route.ProviderVeloPlanner) + "/sourceRoutes/" +
		strconv.FormatInt(sourceRouteID, 10) + "/routes/" + strconv.Itoa(stageOrder) + suffix
	http.Redirect(writer, request, target, http.StatusPermanentRedirect)
}

// RedirectLegacyRoute sends a provider-less address to the same route under the
// only provider it could ever have meant. It and the two methods below differ
// only in the suffix they preserve.
func (h *Handler) RedirectLegacyRoute(writer http.ResponseWriter, request *http.Request) {
	h.redirectLegacyStagePath(writer, request, "")
}

// RedirectLegacyGeometry does the same for that route's geometry.
func (h *Handler) RedirectLegacyGeometry(writer http.ResponseWriter, request *http.Request) {
	h.redirectLegacyStagePath(writer, request, "/geometry")
}

// RedirectLegacyReprocess does the same for that route's reprocess request.
func (h *Handler) RedirectLegacyReprocess(writer http.ResponseWriter, request *http.Request) {
	h.redirectLegacyStagePath(writer, request, "/reprocess")
}

// RedirectLegacyRoutePage sends a browser address from before a second
// provider existed to its provider-qualified equivalent, so a bookmark or
// share link keeps resolving.
func (h *Handler) RedirectLegacyRoutePage(writer http.ResponseWriter, request *http.Request) {
	sourceRouteID, stageOrder, ok := legacyStagePathValues(request)
	if !ok {
		h.notFound(writer)

		return
	}

	target := "/routes/" + string(route.ProviderVeloPlanner) +
		"/" + strconv.FormatInt(sourceRouteID, 10) + "/" + strconv.Itoa(stageOrder)
	http.Redirect(writer, request, target, http.StatusPermanentRedirect)
}

// legacyStagePathValues reads the source route and stage identifiers from a
// superseded path, which names them `routeId` and `stage`. Their shape is
// already settled by the contract; the parse is repeated only because the
// values arrive as path text.
func legacyStagePathValues(request *http.Request) (sourceRouteID int64, stageOrder int, ok bool) {
	sourceRouteID, routeErr := strconv.ParseInt(request.PathValue("routeId"), 10, 64)
	stageOrder, stageErr := strconv.Atoi(request.PathValue("stage"))

	return sourceRouteID, stageOrder, routeErr == nil && stageErr == nil
}
