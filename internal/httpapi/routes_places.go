package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

// The refusal only the place endpoint answers with.
const codeGeocodingFailed = "geocoding_failed"

// ReversePlace names the place at a coordinate, for a planner that would
// otherwise list a route as pairs of numbers. An unnamed place is a 200 with
// no name: open country is a legitimate answer, not a failure.
func (h *Handler) ReversePlace(writer http.ResponseWriter, request *http.Request) {
	latitude, ok := coordinate(h, writer, request, "latitude", 90)
	if !ok {
		return
	}
	longitude, ok := coordinate(h, writer, request, "longitude", 180)
	if !ok {
		return
	}
	name, err := h.places.Reverse(request.Context(), latitude, longitude)
	if err != nil {
		// The geocoder's own error stays in the log; the coordinate, being
		// geometry, is logged nowhere.
		slog.Error("naming a coordinate failed", "error", err)
		h.error(writer, http.StatusBadGateway, codeGeocodingFailed, "the geocoder could not name this coordinate")

		return
	}
	view := openapi.Place{}
	if name != "" {
		view.Name = &name
	}
	h.writeJSON(writer, http.StatusOK, view)
}

// SnapPlace moves a planned waypoint onto the nearest way the local map holds,
// answering the coordinate unmoved where there is none close enough.
func (h *Handler) SnapPlace(writer http.ResponseWriter, request *http.Request) {
	latitude, ok := coordinate(h, writer, request, "latitude", 90)
	if !ok {
		return
	}
	longitude, ok := coordinate(h, writer, request, "longitude", 180)
	if !ok {
		return
	}
	snapLatitude, snapLongitude, moved, err := h.snapper.Snap(request.Context(), latitude, longitude)
	if err != nil {
		slog.Error("snapping a waypoint failed", "error", err)
		h.unavailable(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, openapi.SnappedPlace{
		Latitude: snapLatitude, Longitude: snapLongitude, Snapped: moved,
	})
}

// coordinate reads one required query coordinate, bounded by limit in both
// directions, answering the refusal itself where it is missing or out of range.
func coordinate(
	h *Handler, writer http.ResponseWriter, request *http.Request, name string, limit float64,
) (float64, bool) {
	raw := request.URL.Query().Get(name)
	value, err := strconv.ParseFloat(raw, 64)
	// Asked as a range, so NaN, which no comparison holds for, is refused too.
	if raw == "" || err != nil || !(value >= -limit && value <= limit) {
		h.error(writer, http.StatusBadRequest, "invalid_request", name+" must be a coordinate")

		return 0, false
	}

	return value, true
}
