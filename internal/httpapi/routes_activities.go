package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	activities "github.com/nobbs/domestique/internal/activity"
	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
)

const (
	// defaultActivityWindow is how far back a request that names no window
	// reads, and maximumActivityWindow the longest one it may name.
	defaultActivityWindow = 365 * 24 * time.Hour
	maximumActivityWindow = 2 * 365 * 24 * time.Hour

	// maximumActivities bounds one response. A rider who has ridden more than
	// this in the window sees the most recent of them.
	maximumActivities = 5000
)

// GetActivities serves one target's recorded activities, newest first. A
// non-admin reads only the target they own; naming another's is not found
// rather than forbidden, so the surface never confirms which targets exist.
func (h *Handler) GetActivities(writer http.ResponseWriter, request *http.Request) {
	query := request.URL.Query()
	from, to, ok := h.activityWindow(writer, query.Get("from"), query.Get("to"))
	if !ok {
		return
	}
	requested := query.Get("target")
	targetID, found, err := h.readableTarget(request.Context(), requested)
	if err != nil {
		h.unavailable(writer)

		return
	}
	// A named target the caller may not read is refused; a caller who simply has
	// no target of their own has an empty history, not a missing page.
	if !found && requested != "" {
		h.notFound(writer)

		return
	}
	view := openapi.ActivityList{Activities: []openapi.Activity{}}
	if found {
		stored, activitiesErr := h.state.ActivitiesBetween(request.Context(), targetID, from, to, maximumActivities)
		if activitiesErr != nil {
			h.unavailable(writer)

			return
		}
		view.Activities = make([]openapi.Activity, 0, len(stored))
		for _, recorded := range stored {
			view.Activities = append(view.Activities, openapi.Activity{
				ID:             recorded.ID,
				StartedAt:      wireTime(recorded.StartedAt),
				DistanceMetres: recorded.DistanceMetres,
				MovingSeconds:  recorded.MovingSeconds,
				ElapsedSeconds: recorded.ElapsedSeconds,
				AscentMetres:   recorded.AscentMetres,
				TypeID:         recorded.TypeID,
				LocationID:     recorded.LocationID,
			})
		}
	}
	h.writeJSON(writer, http.StatusOK, view)
}

// GetActivityTrack serves one activity's recorded track as a GeoJSON Feature,
// scoped exactly as the activity list is. An activity with fewer than two
// positioned samples has no track and is not found.
func (h *Handler) GetActivityTrack(writer http.ResponseWriter, request *http.Request) {
	// The served surface refuses a non-numeric id before it reaches here.
	id, idErr := strconv.ParseInt(request.PathValue("activityId"), 10, 64)
	if idErr != nil {
		h.notFound(writer)

		return
	}
	requested := request.URL.Query().Get("target")
	targetID, found, err := h.readableTarget(request.Context(), requested)
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !found {
		h.notFound(writer)

		return
	}
	track, trackErr := h.state.ActivityTrack(request.Context(), targetID, id)
	if trackErr != nil {
		h.unavailable(writer)

		return
	}
	if len(track) < 2 {
		h.notFound(writer)

		return
	}

	writer.Header().Set("Content-Type", "application/geo+json")
	h.writeJSON(writer, http.StatusOK, activityTrackFeature(track))
}

// activityTrackFeature draws the Feature one track is served as: the line, the
// box around it, and the altitudes beside it — null for a sample that recorded
// none, the property absent only when no sample recorded one at all.
func activityTrackFeature(track []activities.TrackPoint) activityTrackView {
	coordinates := make([][2]float64, 0, len(track))
	altitudes := make([]*float64, 0, len(track))
	// One backing array for every altitude, rather than one allocation per sample.
	recorded := make([]float64, len(track))
	anyAltitude := false
	west, south := track[0].Longitude, track[0].Latitude
	east, north := west, south
	for index, point := range track {
		coordinates = append(coordinates, [2]float64{point.Longitude, point.Latitude})
		if point.HasAltitude {
			recorded[index] = point.AltitudeMetres
			altitudes = append(altitudes, &recorded[index])
			anyAltitude = true
		} else {
			altitudes = append(altitudes, nil)
		}
		west, east = min(west, point.Longitude), max(east, point.Longitude)
		south, north = min(south, point.Latitude), max(north, point.Latitude)
	}
	view := activityTrackView{
		Type:     "Feature",
		BBox:     []float64{west, south, east, north},
		Geometry: trackLineStringView{Type: "LineString", Coordinates: coordinates},
	}
	if anyAltitude {
		view.Properties.AltitudeMetres = altitudes
	}

	return view
}

// activityWindow reads the requested window, defaulting to the last year and
// refusing one that is inverted or longer than this service will read.
func (h *Handler) activityWindow(writer http.ResponseWriter, rawFrom, rawTo string) (from, to time.Time, ok bool) {
	// Stored start times are whole UTC seconds, so the window is read as such:
	// a fractional second or an offset must not move a ride across its edge.
	to = h.now().UTC().Truncate(time.Second)
	if rawTo != "" {
		parsed, err := time.Parse(time.RFC3339Nano, rawTo)
		if err != nil {
			h.error(writer, http.StatusBadRequest, "invalid_request", "to is not an RFC3339 timestamp")

			return time.Time{}, time.Time{}, false
		}
		to = parsed.UTC().Truncate(time.Second)
	}
	from = to.Add(-defaultActivityWindow)
	if rawFrom != "" {
		parsed, err := time.Parse(time.RFC3339Nano, rawFrom)
		if err != nil {
			h.error(writer, http.StatusBadRequest, "invalid_request", "from is not an RFC3339 timestamp")

			return time.Time{}, time.Time{}, false
		}
		from = parsed.UTC().Truncate(time.Second)
	}
	if from.After(to) || to.Sub(from) > maximumActivityWindow {
		h.error(writer, http.StatusBadRequest, "invalid_request",
			"the window must end after it starts and span no more than two years")

		return time.Time{}, time.Time{}, false
	}

	return from, to, true
}

// readableTarget resolves the target a caller may read: the one they named
// when they may see it, otherwise their own. Ownership is the rule the status
// view uses — the subject the target records as its owner. found is false for
// a caller with no target of their own, and for a named target they may not
// read.
func (h *Handler) readableTarget(ctx context.Context, requested string) (targetID string, found bool, err error) {
	identity := identityOf(ctx)
	if visitErr := h.state.ForEachTarget(ctx, func(id, _, ownerSubject string) error {
		own := ownerSubject == identity.Subject
		if requested == "" && own && !found {
			targetID, found = id, true
		}
		if requested != "" && id == requested && (identity.Admin || own) {
			targetID, found = id, true
		}

		return nil
	}); visitErr != nil {
		return "", false, fmt.Errorf("listing targets: %w", visitErr)
	}

	return targetID, found, nil
}
