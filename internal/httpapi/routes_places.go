package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

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

// The query lengths a place search accepts, in characters once trimmed: fewer
// than three matches too much to be worth asking the geocoder.
const (
	minimumPlaceQuery = 3
	maximumPlaceQuery = 200
)

// SearchPlaces finds places by name for a planner adding waypoints. The query,
// like the coordinate it is biased towards, is logged nowhere.
func (h *Handler) SearchPlaces(writer http.ResponseWriter, request *http.Request) {
	query := strings.TrimSpace(request.URL.Query().Get("query"))
	if length := utf8.RuneCountInString(query); length < minimumPlaceQuery || length > maximumPlaceQuery {
		h.error(writer, http.StatusBadRequest, "invalid_request", "query must be 3 to 200 characters")

		return
	}
	near, ok := searchBias(h, writer, request)
	if !ok {
		return
	}
	matches, err := h.places.Search(request.Context(), query, near)
	if err != nil {
		slog.Error("searching for a place failed", "error", err)
		h.error(writer, http.StatusBadGateway, codeGeocodingFailed, "the geocoder could not search for this place")

		return
	}
	view := openapi.PlaceSearch{Places: make([]openapi.PlaceMatch, 0, len(matches))}
	for _, match := range matches {
		entry := openapi.PlaceMatch{
			Name:      match.Name,
			Kind:      openapi.PlaceMatch_Kind(match.Kind),
			Latitude:  match.Latitude,
			Longitude: match.Longitude,
		}
		if match.Context != "" {
			entry.Context = &match.Context
		}
		view.Places = append(view.Places, entry)
	}
	h.writeJSON(writer, http.StatusOK, view)
}

// searchBias reads the optional point a search leans towards: both coordinates
// or neither, answering the refusal itself where only one is given.
func searchBias(h *Handler, writer http.ResponseWriter, request *http.Request) (*PlaceNear, bool) {
	values := request.URL.Query()
	if !values.Has("latitude") && !values.Has("longitude") {
		return nil, true
	}
	latitude, ok := coordinate(h, writer, request, "latitude", 90)
	if !ok {
		return nil, false
	}
	longitude, ok := coordinate(h, writer, request, "longitude", 180)
	if !ok {
		return nil, false
	}

	return &PlaceNear{Latitude: latitude, Longitude: longitude}, true
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
