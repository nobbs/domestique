package httpapi

import (
	"net/http"
	"strconv"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/route"
)

// GetRoutes lists every trusted route with its display metadata. It carries
// no geometry: geometry is served only by its own endpoint.
func (h *Handler) GetRoutes(writer http.ResponseWriter, request *http.Request) {
	validation := h.routeValidationView()
	views := make([]openapi.Route, 0)
	if err := h.state.ForEachStageSummary(request.Context(), func(summary route.Summary) error {
		views = append(views, newRouteView(&summary, validation))

		return nil
	}); err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, openapi.RouteList{Routes: views})
}

// GetRoute returns one route's stored metadata, not edit controls.
func (h *Handler) GetRoute(writer http.ResponseWriter, request *http.Request) {
	provider, sourceRouteID, stageOrder, ok := routeKey(request)
	if !ok {
		h.notFound(writer)

		return
	}

	validation := h.routeValidationView()
	var found *openapi.Route
	if err := h.state.ForEachStageSummary(request.Context(), func(summary route.Summary) error {
		if summary.Provider == provider && summary.SourceRouteID == sourceRouteID && summary.StageOrder == stageOrder {
			view := newRouteView(&summary, validation)
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

// GetRouteGeometry returns one route's cached geometry as a GeoJSON Feature. This
// is the only endpoint that serves geometry, and only to the gated identity.
func (h *Handler) GetRouteGeometry(writer http.ResponseWriter, request *http.Request) {
	provider, sourceRouteID, stageOrder, ok := routeKey(request)
	if !ok {
		h.notFound(writer)

		return
	}

	summary, coordinates, cumulativeSeconds, found, err := h.state.StageGeometry(request.Context(), provider, sourceRouteID, stageOrder)
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !found {
		h.notFound(writer)

		return
	}
	surface, readable := h.routeSurface(request, &summary)
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
			SourceRouteID:      summary.SourceRouteID,
			StageOrder:         summary.StageOrder,
			Title:              summary.Title(),
			SourceRouteName:    summary.SourceRouteName,
			RouteName:          summary.RouteName,
			DistanceMetres:     summary.DistanceMetres,
			AscentMetres:       summary.AscentMetres,
			MaxGradientPercent: summary.MaxGradientPercent,
			PointCount:         summary.PointCount,
			Surface:            surface,
			CumulativeSeconds:  cumulativeSeconds,
		},
	})
}

// ReprocessRoute asks for one route to be worked out again from scratch, and
// starts the synchronization that will do it. The request is recorded before the
// run is asked for and survives a refused start, so a busy service still answers
// `202`: a run already in flight may be past this stage or not include it.
func (h *Handler) ReprocessRoute(writer http.ResponseWriter, request *http.Request) {
	provider, sourceRouteID, stageOrder, ok := routeKey(request)
	if !ok {
		h.notFound(writer)

		return
	}

	found, err := h.state.RequestStageReprocess(request.Context(), provider, sourceRouteID, stageOrder)
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

// routeSurface reads the classification stored for this exact geometry, nil when
// none is recorded and unreadable when the store failed. The content hash is part
// of the lookup: ranges index the stored coordinates, so a classification from an
// earlier plan describes positions that no longer exist.
func (h *Handler) routeSurface(request *http.Request, summary *route.Summary) (view *geometrySurfaceView, readable bool) {
	ranges, matchedMetres, found, err := h.state.StageSurface(
		request.Context(),
		summary.Provider,
		summary.SourceRouteID,
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

func newRouteView(summary *route.Summary, validation *openapi.RouteValidation) openapi.Route {
	view := openapi.Route{
		Provider:           string(summary.Provider),
		SourceRouteID:      summary.SourceRouteID,
		StageOrder:         summary.StageOrder,
		Title:              summary.Title(),
		SourceRouteName:    summary.SourceRouteName,
		RouteName:          summary.RouteName,
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

// routeValidationView reports the loaded coefficient profile's measured
// unseen-route error as a route field, or nil when no profile is
// configured or its file carries no measured benchmark result.
func (h *Handler) routeValidationView() *openapi.RouteValidation {
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

// routeKey reads the provider and the route and stage identifiers from the path.
// The contract declares them and the validator refuses anything else, so the
// parse is repeated only because the values arrive as path text. The provider is
// not checked against a known set: state is keyed by all three, so one naming
// nothing stored is refused downstream as not found.
func routeKey(request *http.Request) (provider route.Provider, sourceRouteID int64, stageOrder int, ok bool) {
	provider = route.Provider(request.PathValue("provider"))
	sourceRouteID, routeErr := strconv.ParseInt(request.PathValue("sourceRouteId"), 10, 64)
	stageOrder, stageErr := strconv.Atoi(request.PathValue("stageOrder"))

	return provider, sourceRouteID, stageOrder, routeErr == nil && stageErr == nil
}

func (h *Handler) notFound(writer http.ResponseWriter) {
	h.error(writer, http.StatusNotFound, "not_found", "resource was not found")
}
