package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/route"
)

// The two refusals only the plan endpoints answer with.
const (
	codePreconditionFailed = "precondition_failed"
	codeRoutingFailed      = "routing_failed"
)

// PreviewPlanRoute routes a set of waypoints over the configured engine and
// measures the result, storing nothing: the same path a save routes and
// measures through, so the two agree.
func (h *Handler) PreviewPlanRoute(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.PlanRouteRequest](h, writer, request)
	if !ok {
		return
	}
	measured, err := h.plans.Route(request.Context(), waypointsOf(body.Waypoints), plan.Profile(body.Profile))
	if h.planFailed(writer, err) {
		return
	}
	h.writeJSON(writer, http.StatusOK, openapi.PlanRoutePreview{
		Geometry:         lineStringOf(measured.Geometry),
		DistanceMetres:   measured.DistanceMetres,
		AscentMetres:     measured.AscentMetres,
		DescentMetres:    &measured.DescentMetres,
		MovingSeconds:    optionalSeconds(measured.MovingSeconds),
		WaypointProgress: progressOf(measured.Progress),
		Pushing:          windowsOf(measured.Pushing),
		Surface:          h.planSurface(request.Context(), measured.Geometry),
	})
}

// ListPlans lists every plan, draft and published, with no geometry: a
// plan's geometry is served only by GetPlan.
func (h *Handler) ListPlans(writer http.ResponseWriter, request *http.Request) {
	plans, err := h.plans.List(request.Context())
	if err != nil {
		h.unavailable(writer)

		return
	}
	views := make([]openapi.PlanSummary, len(plans))
	for index := range plans {
		views[index] = planSummaryOf(&plans[index])
	}
	h.writeJSON(writer, http.StatusOK, openapi.PlanList{Plans: views})
}

// CreatePlan routes the waypoints server-side and stores a new plan as a
// draft, regardless of the published field the request carries.
func (h *Handler) CreatePlan(writer http.ResponseWriter, request *http.Request) {
	body, ok := settingsBody[openapi.PlanWrite](h, writer, request)
	if !ok {
		return
	}
	created, err := h.plans.Create(request.Context(), body.Name, plan.Profile(body.Profile), waypointsOf(body.Waypoints))
	if h.planFailed(writer, err) {
		return
	}
	h.writeJSON(writer, http.StatusCreated, h.planOf(request.Context(), &created))
}

// GetPlan returns one plan whole, including its stored geometry: a draft is
// in no inventory for the route geometry endpoint to serve it instead.
func (h *Handler) GetPlan(writer http.ResponseWriter, request *http.Request) {
	id, ok := planID(request)
	if !ok {
		h.notFound(writer)

		return
	}
	found, exists, err := h.plans.Get(request.Context(), id)
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !exists {
		h.notFound(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, h.planOf(request.Context(), &found))
}

// ReplacePlan overwrites one plan whole, published state included, routing
// the waypoints server-side again. A stale If-Match is refused with 412 so
// one admin cannot overwrite what another has just changed.
func (h *Handler) ReplacePlan(writer http.ResponseWriter, request *http.Request) {
	id, ok := planID(request)
	if !ok {
		h.notFound(writer)

		return
	}
	version, ok := ifMatch(request)
	if !ok {
		h.error(writer, http.StatusBadRequest, "invalid_request", "If-Match must be the plan's version, as a plain decimal string")

		return
	}
	body, ok := settingsBody[openapi.PlanWrite](h, writer, request)
	if !ok {
		return
	}
	replaced, err := h.plans.Replace(
		request.Context(), id, version, body.Name, plan.Profile(body.Profile), waypointsOf(body.Waypoints), body.Published,
	)
	if h.planFailed(writer, err) {
		return
	}
	h.writeJSON(writer, http.StatusOK, h.planOf(request.Context(), &replaced))
}

// DeletePlan removes one plan. A stale If-Match is refused with 412 on the
// same terms as ReplacePlan.
func (h *Handler) DeletePlan(writer http.ResponseWriter, request *http.Request) {
	id, ok := planID(request)
	if !ok {
		h.notFound(writer)

		return
	}
	version, ok := ifMatch(request)
	if !ok {
		h.error(writer, http.StatusBadRequest, "invalid_request", "If-Match must be the plan's version, as a plain decimal string")

		return
	}
	if err := h.plans.Delete(request.Context(), id, version); h.planFailed(writer, err) {
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

// planID reads the path's planId. A value the contract validator did not
// already refuse but this cannot parse is answered not found, the same as an
// address naming a plan that was never stored.
func planID(request *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(request.PathValue("planId"), 10, 64)

	return id, err == nil
}

// ifMatch reads the required If-Match header as the plain decimal version it
// carries.
func ifMatch(request *http.Request) (int64, bool) {
	version, err := strconv.ParseInt(request.Header.Get("If-Match"), 10, 64)
	if err != nil || version < 1 {
		return 0, false
	}

	return version, true
}

// planFailed answers a plan operation's error in the shape the contract
// names, and reports whether it already wrote a response. Every sentinel the
// plan package declares maps to its own status; anything else is unavailable.
func (h *Handler) planFailed(writer http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, plan.ErrNotFound):
		h.notFound(writer)
	case errors.Is(err, plan.ErrVersionMismatch):
		h.error(writer, http.StatusPreconditionFailed, codePreconditionFailed, "the plan has changed since it was last read")
	case errors.Is(err, plan.ErrRouting):
		// The engine's own error, and any coordinate, stay in the log: nothing
		// about what was asked to route ever reaches a caller.
		slog.Error("routing a plan failed", "error", err)
		h.error(writer, http.StatusBadGateway, codeRoutingFailed, "the routing engine could not route these waypoints")
	case errors.Is(err, plan.ErrInvalid):
		h.error(writer, http.StatusBadRequest, "invalid_request", err.Error())
	default:
		h.unavailable(writer)
	}

	return true
}

// waypointsOf converts the wire waypoints to the plan package's own type.
func waypointsOf(waypoints []openapi.PlanWaypoint) []plan.Waypoint {
	converted := make([]plan.Waypoint, len(waypoints))
	for index, waypoint := range waypoints {
		converted[index] = plan.Waypoint{Longitude: waypoint.Longitude, Latitude: waypoint.Latitude}
	}

	return converted
}

// openapiWaypointsOf is waypointsOf's inverse, for serving a stored plan back.
func openapiWaypointsOf(waypoints []plan.Waypoint) []openapi.PlanWaypoint {
	converted := make([]openapi.PlanWaypoint, len(waypoints))
	for index, waypoint := range waypoints {
		converted[index] = openapi.PlanWaypoint{Longitude: waypoint.Longitude, Latitude: waypoint.Latitude}
	}

	return converted
}

// lineStringOf renders a routed geometry as a GeoJSON LineString, elevation
// included only where the point carries one — the same shape the stored
// library's own geometry is served in.
func lineStringOf(points []route.Point) openapi.GeoJSONLineString {
	coordinates := make([][]float64, len(points))
	for index, point := range points {
		if point.Elevation == nil {
			coordinates[index] = []float64{point.Longitude, point.Latitude}

			continue
		}
		coordinates[index] = []float64{point.Longitude, point.Latitude, *point.Elevation}
	}

	return openapi.GeoJSONLineString{Type: "LineString", Coordinates: coordinates}
}

// planOf renders a stored plan whole, waypoints and geometry included.
func (h *Handler) planOf(ctx context.Context, p *plan.Plan) openapi.Plan {
	return openapi.Plan{
		ID: p.ID, Name: p.Name, Profile: openapi.PlanProfile(p.Profile), Published: p.Published, Version: p.Version,
		Waypoints: openapiWaypointsOf(p.Waypoints), Geometry: lineStringOf(p.Geometry),
		DistanceMetres: p.DistanceMetres, AscentMetres: p.AscentMetres,
		DescentMetres: &p.DescentMetres,
		MovingSeconds: optionalSeconds(p.MovingSeconds), WaypointProgress: progressOf(p.Progress),
		Pushing:   windowsOf(p.Pushing),
		Surface:   h.planSurface(ctx, p.Geometry),
		CreatedAt: wireTime(p.CreatedAt), UpdatedAt: wireTime(p.UpdatedAt),
	}
}

// optionalSeconds omits an unpredicted time rather than reporting it as zero,
// which is a plan of no length rather than one the model cannot read.
func optionalSeconds(seconds float64) *float64 {
	if seconds <= 0 {
		return nil
	}

	return &seconds
}

// windowsOf renders stretches of a plan, absent where there are none.
func windowsOf(windows []plan.Window) []openapi.PlanWindow {
	if len(windows) == 0 {
		return nil
	}
	views := make([]openapi.PlanWindow, len(windows))
	for index, window := range windows {
		views[index] = openapi.PlanWindow{StartMetres: window.StartMetres, EndMetres: window.EndMetres}
	}

	return views
}

// progressOf renders each waypoint's place along the line, leaving its time
// absent where the line carries no prediction.
func progressOf(progress []plan.Progress) []openapi.PlanProgress {
	if len(progress) == 0 {
		return nil
	}
	views := make([]openapi.PlanProgress, len(progress))
	for index, at := range progress {
		views[index] = openapi.PlanProgress{
			DistanceMetres: at.DistanceMetres,
			MovingSeconds:  optionalSeconds(at.MovingSeconds),
		}
	}

	return views
}

func (h *Handler) planSurface(ctx context.Context, geometry []route.Point) *openapi.SurfaceClassification {
	if h.surface == nil {
		return nil
	}
	classification, err := h.surface.Classify(ctx, geometry)
	if err != nil || classification == nil {
		return nil
	}
	ranges := make([]openapi.SurfaceRange, len(classification.Ranges))
	for index, band := range classification.Ranges {
		ranges[index] = openapi.SurfaceRange{
			Kind:       openapi.SurfaceRange_Kind(band.Kind),
			StartIndex: band.StartIndex,
			EndIndex:   band.EndIndex,
		}
	}

	return &openapi.SurfaceClassification{Ranges: ranges, MatchedMetres: classification.MatchedMetres}
}

// planSummaryOf renders a plan's summary, carrying no geometry.
func planSummaryOf(p *plan.Plan) openapi.PlanSummary {
	return openapi.PlanSummary{
		ID: p.ID, Name: p.Name, Profile: openapi.PlanProfile(p.Profile), Published: p.Published, Version: p.Version,
		DistanceMetres: p.DistanceMetres, AscentMetres: p.AscentMetres, WaypointCount: len(p.Waypoints),
		UpdatedAt: wireTime(p.UpdatedAt),
	}
}
