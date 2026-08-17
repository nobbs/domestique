package httpapi

import (
	"net/http"
	"strconv"

	"github.com/nobbs/domestique/internal/route"
)

// stages lists every trusted source stage with its display metadata. It carries
// no geometry: geometry is served only by its own endpoint.
func (h *Handler) stages(writer http.ResponseWriter, request *http.Request, _ string) {
	views := make([]stageView, 0)
	if err := h.state.ForEachStageSummary(request.Context(), func(summary route.Summary) error {
		views = append(views, newStageView(&summary))

		return nil
	}); err != nil {
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, map[string][]stageView{"stages": views})
}

// stage returns one stage's stored metadata, not edit controls.
func (h *Handler) stage(writer http.ResponseWriter, request *http.Request, _ string) {
	routeID, stageOrder, ok := stageKey(request)
	if !ok {
		h.notFound(writer)

		return
	}

	var found *stageView
	if err := h.state.ForEachStageSummary(request.Context(), func(summary route.Summary) error {
		if summary.RouteID == routeID && summary.StageOrder == stageOrder {
			view := newStageView(&summary)
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

// stageGeometry returns one stage's cached geometry as a GeoJSON Feature. This
// is the only endpoint that serves geometry, and only to the gated identity.
func (h *Handler) stageGeometry(writer http.ResponseWriter, request *http.Request, _ string) {
	routeID, stageOrder, ok := stageKey(request)
	if !ok {
		h.notFound(writer)

		return
	}

	summary, coordinates, found, err := h.state.StageGeometry(request.Context(), routeID, stageOrder)
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !found {
		h.notFound(writer)

		return
	}

	h.writeJSON(writer, http.StatusOK, geometryView{
		Type:     "Feature",
		Geometry: lineStringView{Type: "LineString", Coordinates: coordinates},
		BBox: []float64{
			summary.Bounds.MinLongitude, summary.Bounds.MinLatitude,
			summary.Bounds.MaxLongitude, summary.Bounds.MaxLatitude,
		},
		Properties: geometryPropertyView{
			RouteID:            summary.RouteID,
			StageOrder:         summary.StageOrder,
			Title:              summary.Title(),
			RouteName:          summary.RouteName,
			StageName:          summary.StageName,
			DistanceMetres:     summary.DistanceMetres,
			AscentMetres:       summary.AscentMetres,
			MaxGradientPercent: summary.MaxGradientPercent,
			PointCount:         summary.PointCount,
		},
	})
}

func newStageView(summary *route.Summary) stageView {
	return stageView{
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
		PointCount:         summary.PointCount,
	}
}

// stageKey reads the positive route and stage identifiers from the path.
func stageKey(request *http.Request) (routeID int64, stageOrder int, ok bool) {
	routeID, routeErr := strconv.ParseInt(request.PathValue("routeID"), 10, 64)
	stageOrder, stageErr := strconv.Atoi(request.PathValue("stage"))

	return routeID, stageOrder, routeErr == nil && stageErr == nil && routeID > 0 && stageOrder > 0
}

func (h *Handler) notFound(writer http.ResponseWriter) {
	h.error(writer, http.StatusNotFound, "not_found", "resource was not found")
}
