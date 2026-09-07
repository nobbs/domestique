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

// weatherSummary is the wire form of what a ride's steps came to. A ride nobody
// has asked about, and one that was asked and had nothing to answer, carry none
// at all rather than a row of zeroes.
func weatherSummary(summary activities.WeatherSummary) *openapi.ActivityWeatherSummary {
	return &openapi.ActivityWeatherSummary{
		TemperatureMinCelsius:    summary.TemperatureMinCelsius,
		TemperatureMaxCelsius:    summary.TemperatureMaxCelsius,
		WindSpeedKmh:             summary.WindSpeedKMH,
		PrecipitationMillimetres: summary.PrecipitationMillimetres,
		WeatherCode:              summary.WeatherCode,
	}
}

// rideWeatherSteps is the wire form of one ride's steps.
func rideWeatherSteps(steps []activities.WeatherStep) []openapi.RideWeatherStep {
	if len(steps) == 0 {
		return nil
	}
	view := make([]openapi.RideWeatherStep, 0, len(steps))
	for index := range steps {
		step := &steps[index]
		one := openapi.RideWeatherStep{
			Time:                       wireTime(step.At),
			StepSeconds:                int(step.Step / time.Second),
			TemperatureCelsius:         step.TemperatureCelsius,
			ApparentTemperatureCelsius: step.ApparentTemperatureCelsius,
			PrecipitationMillimetres:   step.PrecipitationMillimetres,
			WindSpeedKmh:               step.WindSpeedKMH,
			WindDirectionDegrees:       step.WindDirectionDegrees,
			WeatherCode:                step.WeatherCode,
			CloudCoverPercent:          step.CloudCoverPercent,
		}
		if step.HasPrecipitationProbability {
			one.PrecipitationProbabilityPercent = &steps[index].PrecipitationProbabilityPercent
		}
		view = append(view, one)
	}

	return view
}

// activityMetrics is the wire form of one ride's derived numbers. Each part is
// omitted where the ride or the profile did not allow it, rather than sent as a
// zero the page would have to read as "not worked out".
//
//nolint:gocritic // value param: metrics are plain numbers, copied as cheaply as a pointer.
func activityMetrics(stored activities.RideMetrics) *openapi.ActivityMetrics {
	metrics, averages := stored.Load, stored.Averages
	view := &openapi.ActivityMetrics{}
	if metrics.HasZones {
		view.ZoneSeconds = metrics.Zones[:]
	}
	if metrics.HasTRIMP {
		view.Trimp = &metrics.TRIMP
	}
	if metrics.HasHeartRateTSS {
		view.HeartRateTss = &metrics.HeartRateTSS
	}
	if metrics.HasPower {
		view.NormalizedPowerWatts = &metrics.Power.NormalizedWatts
		view.IntensityFactor = &metrics.Power.IntensityFactor
		view.PowerTss = &metrics.Power.TSS
	}
	if metrics.HasEstimatedPower {
		view.EstimatedPowerWatts = &metrics.EstimatedPowerWatts
	}
	if averages.HasHeartRate {
		view.AverageHeartRateBpm = &averages.HeartRateBPM
		view.MaxHeartRateBpm = &averages.MaxHeartRateBPM
	}
	if averages.HasCadence {
		view.AverageCadenceRpm = &averages.CadenceRPM
	}
	if averages.HasPower {
		view.AveragePowerWatts = &averages.PowerWatts
	}

	return view
}

// maximumActivities bounds one response. A rider who has ridden more than
// this in the window sees the most recent of them.
const maximumActivities = 5000

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
		// One read for the whole target rather than one per ride: the rows are
		// small, and the list is what the ride page and the volume page both work
		// from.
		derived, metricsErr := h.state.ActivityMetrics(request.Context(), targetID)
		if metricsErr != nil {
			h.unavailable(writer)

			return
		}
		summaries, weatherErr := h.state.ActivityWeatherSummaries(request.Context(), targetID)
		if weatherErr != nil {
			h.unavailable(writer)

			return
		}
		view.Activities = make([]openapi.Activity, 0, len(stored))
		for _, recorded := range stored {
			activity := openapi.Activity{
				ID:             recorded.ID,
				StartedAt:      wireTime(recorded.StartedAt),
				DistanceMetres: recorded.DistanceMetres,
				MovingSeconds:  recorded.MovingSeconds,
				ElapsedSeconds: recorded.ElapsedSeconds,
				AscentMetres:   recorded.AscentMetres,
				TypeID:         recorded.TypeID,
				LocationID:     recorded.LocationID,
			}
			if metrics, ok := derived[recorded.ID]; ok {
				activity.Metrics = activityMetrics(metrics)
			}
			if summary, ok := summaries[recorded.ID]; ok {
				activity.Weather = weatherSummary(summary)
			}
			view.Activities = append(view.Activities, activity)
		}
	}
	h.writeJSON(writer, http.StatusOK, view)
}

// GetActivityTrack serves one activity's recorded track as a GeoJSON Feature,
// scoped exactly as the activity list is. An activity with fewer than two
// positioned samples is served with a null geometry and the state that says
// why; only one this target has no summary for is not found.
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
	recordsState, stored, stateErr := h.state.ActivityRecordsState(request.Context(), targetID, id)
	if stateErr != nil {
		h.unavailable(writer)

		return
	}
	// Only a ride this service holds no summary for is missing: one whose
	// samples are merely absent answers with the state that says so.
	if !stored {
		h.notFound(writer)

		return
	}
	track, trackErr := h.state.ActivityTrack(request.Context(), targetID, id)
	if trackErr != nil {
		h.unavailable(writer)

		return
	}
	steps, weatherErr := h.state.ActivityWeatherSteps(request.Context(), targetID, id)
	if weatherErr != nil {
		h.unavailable(writer)

		return
	}

	writer.Header().Set("Content-Type", "application/geo+json")
	h.writeJSON(writer, http.StatusOK, activityTrackFeature(track, recordsState, steps))
}

// GetActivitySeries serves one named series of one activity's samples, scoped
// exactly as its track is and indexed 1:1 with the coordinates that track
// carries. A series no sample of the ride recorded is not found, so a page can
// tell a bicycle with no meter from one whose meter dropped out.
func (h *Handler) GetActivitySeries(writer http.ResponseWriter, request *http.Request) {
	// The served surface refuses a non-numeric id and an unnamed series before
	// they reach here.
	id, idErr := strconv.ParseInt(request.PathValue("activityId"), 10, 64)
	if idErr != nil {
		h.notFound(writer)

		return
	}
	name := activities.SeriesName(request.PathValue("series"))
	targetID, found, err := h.readableTarget(request.Context(), request.URL.Query().Get("target"))
	if err != nil {
		h.unavailable(writer)

		return
	}
	if !found {
		h.notFound(writer)

		return
	}
	_, stored, stateErr := h.state.ActivityRecordsState(request.Context(), targetID, id)
	if stateErr != nil {
		h.unavailable(writer)

		return
	}
	if !stored {
		h.notFound(writer)

		return
	}
	rows, rowsErr := h.state.ActivitySeries(request.Context(), targetID, id)
	if rowsErr != nil {
		h.unavailable(writer)

		return
	}
	readings, present := activities.Series(rows, name)
	if !present {
		h.notFound(writer)

		return
	}
	h.writeJSON(writer, http.StatusOK, activitySeriesView{Series: string(name), Values: seriesValues(readings)})
}

// seriesValues is the wire form of one series: a value where the sample carried
// one, null where it carried none.
func seriesValues(readings []activities.Reading) []*float64 {
	values := make([]*float64, len(readings))
	// One backing array for the whole series, rather than one allocation per
	// sample; a ride can hold twenty thousand of them.
	known := make([]float64, len(readings))
	for index := range readings {
		if readings[index].Known {
			known[index] = readings[index].Value
			values[index] = &known[index]
		}
	}

	return values
}

// activityTrackFeature draws the Feature one track is served as: the line, the
// box around it, and the altitudes beside it — null where a sample recorded
// none, absent only when none did. A ride with no line carries only its state.
func activityTrackFeature(
	track []activities.TrackPoint, state activities.RecordsState, steps []activities.WeatherStep,
) activityTrackView {
	if len(track) < 2 {
		// No line to draw, but a ride sampled at one point still has weather, and
		// the page can still say what it was ridden through.
		return activityTrackView{
			Type: "Feature",
			Properties: activityTrackPropertyView{
				State:   absentTrackState(state),
				Weather: rideWeatherSteps(steps),
			},
		}
	}
	coordinates := make([][2]float64, 0, len(track))
	altitudes := make([]*float64, 0, len(track))
	estimates := make([]*float64, 0, len(track))
	// One backing array per series, rather than one allocation per sample.
	recorded := make([]float64, len(track))
	estimated := make([]float64, len(track))
	anyAltitude, anyEstimate := false, false
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
		if point.HasEstimatedPower {
			estimated[index] = point.EstimatedPowerWatts
			estimates = append(estimates, &estimated[index])
			anyEstimate = true
		} else {
			estimates = append(estimates, nil)
		}
		west, east = min(west, point.Longitude), max(east, point.Longitude)
		south, north = min(south, point.Latitude), max(north, point.Latitude)
	}
	view := activityTrackView{
		Type:       "Feature",
		BBox:       []float64{west, south, east, north},
		Geometry:   &trackLineStringView{Type: "LineString", Coordinates: coordinates},
		Properties: activityTrackPropertyView{State: trackStateStored},
	}
	if anyAltitude {
		view.Properties.AltitudeMetres = altitudes
	}
	if anyEstimate {
		view.Properties.EstimatedPowerWatts = estimates
	}
	view.Properties.Weather = rideWeatherSteps(steps)

	return view
}

// absentTrackState tells the three reasons a stored ride has no line apart: its
// samples are still awaited, its file did not decode, or too few of them
// carried a position to draw one.
func absentTrackState(state activities.RecordsState) string {
	switch state {
	case activities.RecordsPending:
		return trackStatePending
	case activities.RecordsUnreadable:
		return trackStateUnreadable
	case activities.RecordsStored:
	}

	return trackStateEmpty
}

// activityWindow reads the requested window. An unset from is no lower bound
// at all; an unset to defaults to now.
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
	if rawFrom != "" {
		parsed, err := time.Parse(time.RFC3339Nano, rawFrom)
		if err != nil {
			h.error(writer, http.StatusBadRequest, "invalid_request", "from is not an RFC3339 timestamp")

			return time.Time{}, time.Time{}, false
		}
		from = parsed.UTC().Truncate(time.Second)
	}
	if from.After(to) {
		h.error(writer, http.StatusBadRequest, "invalid_request", "the window must not end before it starts")

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
