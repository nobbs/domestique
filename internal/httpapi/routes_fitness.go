package httpapi

import (
	"net/http"
	"time"

	openapi "github.com/nobbs/domestique/internal/httpapi/contract"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
)

// GetFitness serves the rider's fitness, fatigue and form over time, one row
// per day, on both of the scales a ride's load is measured on.
//
// Scoped exactly as the activity list is: the caller's own target, or one an
// administrator names. The series is folded at read time rather than stored —
// a rider's whole history is a few hundred rides, and a fold over them is
// cheaper than a table that would have to be kept true.
func (h *Handler) GetFitness(writer http.ResponseWriter, request *http.Request) {
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
	if !found && requested != "" {
		h.notFound(writer)

		return
	}
	view := openapi.Fitness{Days: []openapi.FitnessDay{}, Weeks: []openapi.FitnessWeek{}}
	if !found {
		// A rider with no target of their own has an empty timeline, not a
		// missing page, exactly as they have an empty history.
		h.writeJSON(writer, http.StatusOK, view)

		return
	}
	loads, loadsErr := h.state.ActivityRideLoads(request.Context(), targetID)
	if loadsErr != nil {
		h.unavailable(writer)

		return
	}
	location := h.timezone()
	// Folded from the rider's first ride rather than from the window, then cut
	// to the window: a window opening years into a history opens at the fitness
	// that history had actually built.
	// A day is in the window when any of it is, not only when it begins inside:
	// the window a page asks for starts at whatever moment the reader opened it,
	// and a day it half covers is a day it covers.
	for _, day := range trainingload.Timeline(loads, to, location) {
		if !overlaps(day.Date, day.Date.AddDate(0, 0, 1), from, to) {
			continue
		}
		view.Days = append(view.Days, fitnessDay(&day))
	}
	for _, week := range trainingload.ZonesByWeek(loads, location) {
		if !overlaps(week.WeekStart, week.WeekStart.AddDate(0, 0, 7), from, to) {
			continue
		}
		view.Weeks = append(view.Weeks, openapi.FitnessWeek{
			WeekStart:   week.WeekStart.Format(time.DateOnly),
			ZoneSeconds: week.Zones[:],
		})
	}
	// The curve is the rider's own over the window, folded from what each ride
	// was derived to hold. A read that fails costs the page its curve, not its
	// timeline: the two answer different questions from different rows.
	if curve, curveErr := h.state.PowerCurve(
		request.Context(), []string{targetID}, from, to,
	); curveErr == nil {
		view.PowerCurve = powerCurvePoints(curve)
	}
	h.writeJSON(writer, http.StatusOK, view)
}

// powerCurvePoints is the wire form of a folded curve: one point per duration
// the rides actually reached, shortest first, and nil for a window whose rides
// carried no meter at all.
func powerCurvePoints(curve rider.PowerCurve) []openapi.PowerCurvePoint {
	if !curve.Any() {
		return nil
	}
	points := make([]openapi.PowerCurvePoint, 0, rider.PowerCurvePoints)
	for point, window := range rider.PowerCurveDurations() {
		if curve.Held[point] {
			points = append(points, openapi.PowerCurvePoint{
				Seconds: int(window.Seconds()),
				Watts:   curve.Watts[point],
			})
		}
	}

	return points
}

// overlaps reports whether a period shares any moment with the half-open window
// [from, to).
func overlaps(start, end, from, to time.Time) bool {
	return end.After(from) && start.Before(to)
}

// timezone is the zone the service counts days in, which is the one the volume
// page buckets by. A zone this build cannot load leaves the days in UTC rather
// than failing a read: a timeline in the wrong zone is a smaller problem than
// no timeline at all.
func (h *Handler) timezone() *time.Location {
	location, err := time.LoadLocation(h.settings.Values().Timezone)
	if err != nil {
		return time.UTC
	}

	return location
}

func fitnessDay(day *trainingload.Day) openapi.FitnessDay {
	return openapi.FitnessDay{
		Date:         day.Date.Format(time.DateOnly),
		TrimpLoad:    day.TRIMPLoad,
		TrimpFitness: day.TRIMPFitness,
		TrimpFatigue: day.TRIMPFatigue,
		TrimpForm:    day.TRIMPForm,
		TssLoad:      day.TSSLoad,
		TssFitness:   day.TSSFitness,
		TssFatigue:   day.TSSFatigue,
		TssForm:      day.TSSForm,
	}
}
