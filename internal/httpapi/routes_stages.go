package httpapi

import (
	"net/http"
	"strconv"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
)

// GetRoutes lists every trusted source stage with its display metadata. It carries
// no geometry: geometry is served only by its own endpoint.
func (h *Handler) GetRoutes(writer http.ResponseWriter, request *http.Request) {
	validation := h.stageValidationView()
	views := make([]openapi.Route, 0)
	if err := h.state.ForEachStageSummary(request.Context(), func(summary route.Summary) error {
		views = append(views, newStageView(&summary, validation))

		return nil
	}); err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, openapi.RouteList{Stages: views})
}

// GetRoute returns one stage's stored metadata, not edit controls.
func (h *Handler) GetRoute(writer http.ResponseWriter, request *http.Request) {
	provider, routeID, stageOrder, ok := stageKey(request)
	if !ok {
		h.notFound(writer)

		return
	}

	validation := h.stageValidationView()
	var found *openapi.Route
	if err := h.state.ForEachStageSummary(request.Context(), func(summary route.Summary) error {
		if summary.Provider == provider && summary.RouteID == routeID && summary.StageOrder == stageOrder {
			view := newStageView(&summary, validation)
			found = &view
		}

		return nil
	}); err != nil {
		h.unavailable(writer)

		return
	}
	if found == nil {
		h.notFound(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, found)
}

// GetRouteGeometry returns one stage's cached geometry as a GeoJSON Feature. This
// is the only endpoint that serves geometry, and only to the gated identity.
func (h *Handler) GetRouteGeometry(writer http.ResponseWriter, request *http.Request) {
	provider, routeID, stageOrder, ok := stageKey(request)
	if !ok {
		h.notFound(writer)

		return
	}

	summary, coordinates, cumulativeSeconds, found, err := h.state.StageGeometry(request.Context(), provider, routeID, stageOrder)
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !found {
		h.notFound(writer)

		return
	}
	surface, readable := h.stageSurface(request, &summary)
	if !readable {
		h.unavailable(writer)

		return
	}

	writer.Header().Set("Content-Type", "application/geo+json")
	h.writeJSON(writer, http.StatusOK, geometryView{
		Type:     "Feature",
		Geometry: lineStringView{Type: "LineString", Coordinates: coordinates},
		BBox: []float64{
			summary.Bounds.MinLongitude, summary.Bounds.MinLatitude,
			summary.Bounds.MaxLongitude, summary.Bounds.MaxLatitude,
		},
		Properties: geometryPropertyView{
			Provider:           string(summary.Provider),
			RouteID:            summary.RouteID,
			StageOrder:         summary.StageOrder,
			Title:              summary.Title(),
			RouteName:          summary.RouteName,
			StageName:          summary.StageName,
			DistanceMetres:     summary.DistanceMetres,
			AscentMetres:       summary.AscentMetres,
			MaxGradientPercent: summary.MaxGradientPercent,
			PointCount:         summary.PointCount,
			Surface:            surface,
			CumulativeSeconds:  cumulativeSeconds,
		},
	})
}

// ReprocessRoute asks for one stage to be worked out again from scratch, and
// starts the synchronization that will do it.
//
// The request is recorded before the run is asked for, and deliberately survives
// a refused start: a run already in flight may be past this stage, or may not
// include it at all, so the mark waits for a pass that will honour it rather
// than being dropped on the floor. That is why a busy service still answers
// `202` here — the operator's request has been taken either way.
func (h *Handler) ReprocessRoute(writer http.ResponseWriter, request *http.Request) {
	provider, routeID, stageOrder, ok := stageKey(request)
	if !ok {
		h.notFound(writer)

		return
	}

	found, err := h.state.RequestStageReprocess(request.Context(), provider, routeID, stageOrder)
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !found {
		h.notFound(writer)

		return
	}
	// Both halves, in order: the stage is read and derived again, then written
	// to every target. Asking for only one would leave the request half met.
	h.syncRuns.Trigger(SyncPhaseAll)
	h.writeJSON(writer, http.StatusAccepted, openapi.Accepted{Status: "accepted"})
}

// stageSurface reads the classification stored for this exact geometry. It
// returns a nil view when none has been recorded yet, and reports the state as
// unreadable when the store itself failed.
//
// The content hash is part of the lookup because the ranges index the stored
// coordinates: a classification measured against an earlier plan of the same
// stage describes positions that no longer exist, so it is treated as absent
// rather than served against the wrong line.
func (h *Handler) stageSurface(request *http.Request, summary *route.Summary) (view *geometrySurfaceView, readable bool) {
	ranges, matchedMetres, found, err := h.state.StageSurface(
		request.Context(),
		summary.Provider,
		summary.RouteID,
		summary.StageOrder,
		summary.ContentHash,
	)
	if err != nil {
		return nil, false
	}
	if !found {
		return nil, true
	}

	return &geometrySurfaceView{Ranges: ranges, MatchedMetres: matchedMetres}, true
}

func newStageView(summary *route.Summary, validation *openapi.RouteValidation) openapi.Route {
	view := openapi.Route{
		Provider:           string(summary.Provider),
		RouteID:            summary.RouteID,
		StageOrder:         summary.StageOrder,
		Title:              summary.Title(),
		RouteName:          summary.RouteName,
		StageName:          summary.StageName,
		SourceRevision:     summary.SourceRevision,
		ContentHash:        summary.ContentHash,
		DistanceMetres:     summary.DistanceMetres,
		AscentMetres:       summary.AscentMetres,
		MaxGradientPercent: summary.MaxGradientPercent,
		MovingSeconds:      summary.MovingSeconds,
		PointCount:         summary.PointCount,
	}
	// A stage with no prediction of its own has nothing for the profile's
	// uncertainty to qualify.
	if summary.MovingSeconds != nil {
		view.Validation = validation
	}

	return view
}

// stageValidationView reports the loaded coefficient profile's measured
// unseen-route error as a route field, or nil when no profile is
// configured or its file carries no measured benchmark result.
func (h *Handler) stageValidationView() *openapi.RouteValidation {
	if h.rideModelValidation == nil {
		return nil
	}
	validation := h.rideModelValidation()
	if validation == nil {
		return nil
	}

	return &openapi.RouteValidation{
		BiasPercent:    validation.BiasPercent,
		MaePercent:     validation.MAEPercent,
		P90percent:     validation.P90Percent,
		EvaluatedRides: validation.EvaluatedRides,
	}
}

// stageKey reads the provider and the route and stage identifiers from the
// path. Their shape is already settled: the contract declares them as a
// non-empty string and two integers of minimum 1, and the request validator
// refuses anything else before this runs. The parse is repeated rather than
// trusted only because the values arrive as path text.
//
// The provider is not checked against a known set here: state is keyed by
// provider, routeID and stageOrder together, so a provider naming nothing
// stored is already refused downstream as not found, the same way a well-formed
// but absent routeID is.
func stageKey(request *http.Request) (provider route.Provider, routeID int64, stageOrder int, ok bool) {
	provider = route.Provider(request.PathValue("provider"))
	routeID, routeErr := strconv.ParseInt(request.PathValue("routeId"), 10, 64)
	stageOrder, stageErr := strconv.Atoi(request.PathValue("stage"))

	return provider, routeID, stageOrder, routeErr == nil && stageErr == nil
}

func (h *Handler) notFound(writer http.ResponseWriter) {
	h.error(writer, http.StatusNotFound, "not_found", "resource was not found")
}

// redirectLegacyStagePath sends a stage URL from before a second provider
// existed to its provider-qualified equivalent, preserving suffix and method.
// A 308 is used rather than a 301 or 302 because the reprocess route is a
// POST, and only a 308 is defined to keep a redirected request's method and
// body intact. The route and stage identifiers are re-rendered from parsed
// integers, rather than carried over as raw path text, so the redirect target
// is never built from unvalidated request input.
func (h *Handler) redirectLegacyStagePath(writer http.ResponseWriter, request *http.Request, suffix string) {
	routeID, stageOrder, ok := legacyStagePathValues(request)
	if !ok {
		h.notFound(writer)

		return
	}

	target := "/v1/providers/" + string(route.ProviderVeloPlanner) + "/routes/" +
		strconv.FormatInt(routeID, 10) + "/stages/" + strconv.Itoa(stageOrder) + suffix
	http.Redirect(writer, request, target, http.StatusPermanentRedirect)
}

// RedirectLegacyRoute sends a provider-less stage address to the same stage
// under the only provider it could ever have meant. It and the two methods
// below differ only in the suffix they preserve.
func (h *Handler) RedirectLegacyRoute(writer http.ResponseWriter, request *http.Request) {
	h.redirectLegacyStagePath(writer, request, "")
}

// RedirectLegacyGeometry does the same for that stage's geometry.
func (h *Handler) RedirectLegacyGeometry(writer http.ResponseWriter, request *http.Request) {
	h.redirectLegacyStagePath(writer, request, "/geometry")
}

// RedirectLegacyReprocess does the same for that stage's reprocess request.
func (h *Handler) RedirectLegacyReprocess(writer http.ResponseWriter, request *http.Request) {
	h.redirectLegacyStagePath(writer, request, "/reprocess")
}

// RedirectLegacyRoutePage sends a browser address from before a second
// provider existed to its provider-qualified equivalent, so a bookmark or
// share link keeps resolving.
func (h *Handler) RedirectLegacyRoutePage(writer http.ResponseWriter, request *http.Request) {
	routeID, stageOrder, ok := legacyStagePathValues(request)
	if !ok {
		h.notFound(writer)

		return
	}

	target := "/routes/" + string(route.ProviderVeloPlanner) +
		"/" + strconv.FormatInt(routeID, 10) + "/" + strconv.Itoa(stageOrder)
	http.Redirect(writer, request, target, http.StatusPermanentRedirect)
}

// legacyStagePathValues reads the route and stage identifiers from a legacy,
// provider-less path, on the same terms as stageKey above.
func legacyStagePathValues(request *http.Request) (routeID int64, stageOrder int, ok bool) {
	routeID, routeErr := strconv.ParseInt(request.PathValue("routeId"), 10, 64)
	stageOrder, stageErr := strconv.Atoi(request.PathValue("stage"))

	return routeID, stageOrder, routeErr == nil && stageErr == nil
}
