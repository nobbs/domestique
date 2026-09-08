package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/sqlite"
)

// See docs/specs/measurement.md §Ascent and descent for why 25 m and 100 m
// are the route definition's own resample interval and median window.
const (
	routeIntervalMetres = 25.0
	routeWindowMetres   = 100.0
)

const (
	candidateRaw   = "raw"
	candidateRoute = "route"
)

// parsePositiveMetresList splits a comma-separated list of positive, finite,
// non-repeating metre values (hysteresis thresholds, distance grids),
// labelling any rejection with what kind of value it was parsing.
func parsePositiveMetresList(list, label string) ([]float64, error) {
	values := make([]float64, 0)
	for part := range strings.SplitSeq(list, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing %s %q: %w", label, trimmed, err)
		}
		if value <= 0 || math.IsInf(value, 0) || math.IsNaN(value) {
			return nil, fmt.Errorf("%s %q is not a positive number of metres", label, trimmed)
		}
		if slices.Contains(values, value) {
			return nil, fmt.Errorf("%s %q is listed twice", label, trimmed)
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("at least one %s is required", label)
	}

	return values, nil
}

func parseThresholds(list string) ([]float64, error) {
	return parsePositiveMetresList(list, "threshold")
}

func parseGrids(list string) ([]float64, error) {
	return parsePositiveMetresList(list, "grid")
}

// validateStillSpeed rejects a -still-speed that could not be a real minimum
// moving speed.
func validateStillSpeed(metresPerSecond float64) error {
	if metresPerSecond <= 0 || math.IsInf(metresPerSecond, 0) || math.IsNaN(metresPerSecond) {
		return errors.New("-still-speed must be a positive number of metres per second")
	}

	return nil
}

func hystName(thresholdMetres float64) string {
	return "hyst" + strconv.FormatFloat(thresholdMetres, 'g', -1, 64)
}

func routeHystName(thresholdMetres float64) string {
	return "route+" + hystName(thresholdMetres)
}

func gridName(gridMetres float64) string {
	return "grid" + strconv.FormatFloat(gridMetres, 'g', -1, 64)
}

func gridHystName(gridMetres, thresholdMetres float64) string {
	return gridName(gridMetres) + "+" + hystName(thresholdMetres)
}

func stillHystName(thresholdMetres float64) string {
	return "still+" + hystName(thresholdMetres)
}

// fullCandidateOrder is every candidate the main table prints, in the fixed
// order it prints them: the existing candidates first, unchanged, so a run
// against the same flags stays comparable with one from before this file
// grew grid and stationary candidates, then those two families after.
func fullCandidateOrder(thresholdsMetres, gridsMetres []float64) []string {
	order := []string{candidateRaw, candidateRoute}
	for _, t := range thresholdsMetres {
		order = append(order, hystName(t))
	}
	for _, t := range thresholdsMetres {
		order = append(order, routeHystName(t))
	}
	for _, g := range gridsMetres {
		for _, t := range thresholdsMetres {
			order = append(order, gridHystName(g, t))
		}
	}
	for _, t := range thresholdsMetres {
		order = append(order, stillHystName(t))
	}

	return order
}

// splitCandidateOrder is the fixed, small candidate subset the split tables
// print so they stay readable, keeping only the ones the run's own flags
// actually produced.
func splitCandidateOrder(fullOrder []string) []string {
	wanted := []string{candidateRaw, candidateRoute, hystName(3), routeHystName(2), stillHystName(3), gridHystName(20, 2)}
	order := make([]string, 0, len(wanted))
	for _, name := range wanted {
		if slices.Contains(fullOrder, name) {
			order = append(order, name)
		}
	}

	return order
}

func weatherSplitLabels() []string { return []string{"dry", "wet", "no weather"} }

func speedSplitLabels() []string { return []string{"<5 m/s", "5-7 m/s", ">=7 m/s"} }

// quantumBucketLabels is the fixed, ordered set a smallest-positive-step
// reading is bucketed into.
func quantumBucketLabels() []string {
	return []string{"<=0.1", "0.2", "0.5", "1", ">1"}
}

func quantumBucketLabel(stepMetres float64) string {
	switch {
	case stepMetres <= 0.1:
		return "<=0.1"
	case stepMetres <= 0.2:
		return "0.2"
	case stepMetres <= 0.5:
		return "0.5"
	case stepMetres <= 1.0:
		return "1"
	default:
		return ">1"
	}
}

// smallestPositiveStep is the smallest strictly-positive adjacent step in an
// altitude series, which is what an altimeter's own quantum shows up as. The
// second value is false when the series never rises.
func smallestPositiveStep(altitudeMetres []float64) (float64, bool) {
	smallest, found := math.Inf(1), false
	for index := 1; index < len(altitudeMetres); index++ {
		if step := altitudeMetres[index] - altitudeMetres[index-1]; step > 0 && step < smallest {
			smallest, found = step, true
		}
	}

	return smallest, found
}

// stillFilteredAltitudes drops every sample whose speed since the previous
// sample (distance moved over time elapsed) falls below minSpeedMS, along
// with any sample whose step took non-positive time — a repeated or
// reordered timestamp, not a rest the rider took. The first sample always
// survives: it has no previous step to judge it by.
func stillFilteredAltitudes(track []measure.Sample, minSpeedMS float64) []float64 {
	if len(track) == 0 {
		return nil
	}
	kept := []float64{track[0].AltitudeMetres}
	for index := 1; index < len(track); index++ {
		dtSeconds := track[index].At.Sub(track[index-1].At).Seconds()
		if dtSeconds <= 0 {
			continue
		}
		speed := (track[index].DistanceMetres - track[index-1].DistanceMetres) / dtSeconds
		if speed < minSpeedMS {
			continue
		}
		kept = append(kept, track[index].AltitudeMetres)
	}

	return kept
}

// namedReport pairs one split's label with the report scoring only that
// split's rides.
type namedReport struct {
	report *report
	label  string
}

// report accumulates one candidate's relative errors against the device
// ascent, and the corpus counts a reader needs to judge them by.
type report struct {
	errorsPercent       map[string][]float64
	quantumCounts       map[string]int
	order               []string
	driftPerHourMetres  []float64
	weatherSplits       []namedReport
	speedSplits         []namedReport
	totalRides          int
	skippedZeroAscent   int
	skippedMinSamples   int
	skippedNonMonotonic int
	splitsEnabled       bool
}

// newReportWithOrder is a report scoring exactly the named candidates, in the
// order given — the full candidate table, or a split's smaller subset.
func newReportWithOrder(order []string) *report {
	errorsPercent := make(map[string][]float64, len(order))
	for _, name := range order {
		errorsPercent[name] = nil
	}

	return &report{order: order, errorsPercent: errorsPercent, quantumCounts: map[string]int{}}
}

func (r *report) record(name string, candidateAscent, deviceAscent float64) {
	r.errorsPercent[name] = append(r.errorsPercent[name], (candidateAscent/deviceAscent-1)*100)
}

// wants reports whether name is one of this report's own candidates, so a
// caller scoring several reports at once from one candidate list can skip the
// ones a smaller split table left out.
func (r *report) wants(name string) bool {
	_, ok := r.errorsPercent[name]

	return ok
}

// recordAll scores one candidate's ascent into every report that wants it —
// the main table always does; a split report only for the handful of names
// splitCandidateOrder kept, and a nil report (splits disabled, or a ride with
// no moving time to bucket by speed) is skipped.
func recordAll(name string, candidateAscent, deviceAscent float64, reports ...*report) {
	for _, r := range reports {
		if r != nil && r.wants(name) {
			r.record(name, candidateAscent, deviceAscent)
		}
	}
}

func (r *report) addQuantum(altitudeMetres []float64) {
	if step, ok := smallestPositiveStep(altitudeMetres); ok {
		r.quantumCounts[quantumBucketLabel(step)]++
	}
}

// addDrift records one ride's barometer drift, in metres per hour of moving
// time: the height the barometer says a ride that returned to its start gained
// or lost between leaving and coming back, which the ground says is nought.
func (r *report) addDrift(residualMetres, movingSeconds float64) {
	r.driftPerHourMetres = append(r.driftPerHourMetres, residualMetres/(movingSeconds/3600))
}

// loopEndsWithinMetres is how close a ride's last position must be to its
// first for the ride to count as returning to its start.
const loopEndsWithinMetres = 200

// loopDrift is the altitude the barometer gained or lost between a ride's
// first and last positioned samples, for a ride whose last position lies
// within loopEndsWithinMetres of its first; not ok for any other ride.
func loopDrift(ctx context.Context, store *sqlite.Store, ride sqlite.RecordedRide) (float64, bool) {
	track, err := store.ActivityTrack(ctx, ride.TargetID, ride.WorkoutID)
	if err != nil || len(track) < 2 {
		return 0, false
	}
	first, last := track[0], track[len(track)-1]
	if !first.HasAltitude || !last.HasAltitude {
		return 0, false
	}
	apart := measure.HaversineMetres(
		measure.Coordinate{Latitude: first.Latitude, Longitude: first.Longitude},
		measure.Coordinate{Latitude: last.Latitude, Longitude: last.Longitude})
	if apart > loopEndsWithinMetres {
		return 0, false
	}

	return last.AltitudeMetres - first.AltitudeMetres, true
}

// candidateSummary is one candidate's statistics against the device ascent:
// its ride count, the median/quartiles of its signed relative error, and the
// mean of that error's magnitude.
type candidateSummary struct {
	name                                                string
	rides                                               int
	medianPercent, q1Percent, q3Percent, meanAbsPercent float64
}

func (r *report) summarize(name string) candidateSummary {
	values := append([]float64(nil), r.errorsPercent[name]...)
	sort.Float64s(values)
	summary := candidateSummary{name: name, rides: len(values)}
	if len(values) == 0 {
		return summary
	}
	summary.medianPercent = percentileOf(values, 0.5)
	summary.q1Percent = percentileOf(values, 0.25)
	summary.q3Percent = percentileOf(values, 0.75)
	sumAbs := 0.0
	for _, v := range values {
		sumAbs += math.Abs(v)
	}
	summary.meanAbsPercent = sumAbs / float64(len(values))

	return summary
}

// percentileOf reads a fractional position from an already-sorted slice by
// linear interpolation between the two nearest ranks.
func percentileOf(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := fraction * float64(len(sorted)-1)
	lower, upper := int(math.Floor(position)), int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)

	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// tableString renders this report's candidate table alone: the header and one
// row per candidate, in this report's own order. Shared by the main report
// and every split's smaller table.
func (r *report) tableString() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  %-14s %6s %10s %10s %10s %10s\n", "candidate", "rides", "median", "q1", "q3", "mean|err|")
	for _, name := range r.order {
		s := r.summarize(name)
		fmt.Fprintf(&b, "  %-14s %6d %9.1f%% %9.1f%% %9.1f%% %9.1f%%\n",
			s.name, s.rides, s.medianPercent, s.q1Percent, s.q3Percent, s.meanAbsPercent)
	}

	return b.String()
}

// driftSummary is the median and quartiles of every ride's barometer drift.
func (r *report) driftSummary() (median, q1, q3 float64) {
	values := append([]float64(nil), r.driftPerHourMetres...)
	sort.Float64s(values)

	return percentileOf(values, 0.5), percentileOf(values, 0.25), percentileOf(values, 0.75)
}

func (r *report) String() string {
	var b strings.Builder
	fmt.Fprintln(&b, "ascent candidates vs device (relative error %, positive = candidate over-reports)")
	b.WriteString(r.tableString())
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "altimeter quantum: smallest positive altitude step per ride, bucketed")
	for _, label := range quantumBucketLabels() {
		fmt.Fprintf(&b, "  %-5s %d\n", label, r.quantumCounts[label])
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "rides: total=%d skipped_zero_device_ascent=%d skipped_short_or_unpositioned_track=%d skipped_route_non_monotonic=%d\n",
		r.totalRides, r.skippedZeroAscent, r.skippedMinSamples, r.skippedNonMonotonic)

	if r.splitsEnabled {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "split: weather (dry vs wet vs no weather)")
		for _, entry := range r.weatherSplits {
			fmt.Fprintf(&b, "  %s\n", entry.label)
			b.WriteString(entry.report.tableString())
		}
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "split: mean moving speed")
		for _, entry := range r.speedSplits {
			fmt.Fprintf(&b, "  %s\n", entry.label)
			b.WriteString(entry.report.tableString())
		}
	}

	median, q1, q3 := r.driftSummary()
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "drift: median %.1f m/h (%.1f, %.1f)\n", median, q1, q3)

	return b.String()
}

// weatherSplitFor is the split report for one ride's weather group — wet if
// its summary recorded any precipitation, dry if it recorded a summary with
// none, and "no weather" if the target never got one for this ride. Summaries
// are read once per target and cached, since RecordedRides orders every
// target's rides together.
func weatherSplitFor(
	ctx context.Context, store *sqlite.Store, cache map[string]map[int64]activity.WeatherSummary,
	weatherSplits []namedReport, ride sqlite.RecordedRide,
) (*report, error) {
	summaries, ok := cache[ride.TargetID]
	if !ok {
		var err error
		summaries, err = store.ActivityWeatherSummaries(ctx, ride.TargetID)
		if err != nil {
			return nil, fmt.Errorf("reading a target's weather summaries: %w", err)
		}
		cache[ride.TargetID] = summaries
	}
	label := "no weather"
	if summary, found := summaries[ride.WorkoutID]; found {
		label = "dry"
		if summary.PrecipitationMillimetres > 0 {
			label = "wet"
		}
	}
	for _, entry := range weatherSplits {
		if entry.label == label {
			return entry.report, nil
		}
	}

	return nil, fmt.Errorf("no %q weather split report was built", label)
}

// speedSplitFor is the split report for one ride's mean moving speed bucket,
// or nil for a ride with no moving time to divide by.
func speedSplitFor(speedSplits []namedReport, ride sqlite.RecordedRide) *report {
	if ride.MovingSeconds <= 0 {
		return nil
	}
	speedMS := ride.DistanceMetres / ride.MovingSeconds
	label := ">=7 m/s"
	switch {
	case speedMS < 5:
		label = "<5 m/s"
	case speedMS < 7:
		label = "5-7 m/s"
	}
	for _, entry := range speedSplits {
		if entry.label == label {
			return entry.report
		}
	}

	return nil
}

// study reads every recorded ride the store holds and scores each candidate
// ascent definition (see the package doc comment) against the ascent the
// device reported for it.
func study(
	ctx context.Context, store *sqlite.Store,
	thresholdsMetres, gridsMetres []float64, stillSpeedMS float64,
	minSamples int, splitsEnabled bool,
) (*report, error) {
	rides, err := store.RecordedRides(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing recorded rides: %w", err)
	}

	fullOrder := fullCandidateOrder(thresholdsMetres, gridsMetres)
	result := newReportWithOrder(fullOrder)
	result.splitsEnabled = splitsEnabled
	if splitsEnabled {
		splitOrder := splitCandidateOrder(fullOrder)
		for _, label := range weatherSplitLabels() {
			result.weatherSplits = append(result.weatherSplits, namedReport{label: label, report: newReportWithOrder(splitOrder)})
		}
		for _, label := range speedSplitLabels() {
			result.speedSplits = append(result.speedSplits, namedReport{label: label, report: newReportWithOrder(splitOrder)})
		}
	}
	weatherCache := map[string]map[int64]activity.WeatherSummary{}

	for _, ride := range rides {
		result.totalRides++
		if ride.AscentMetres <= 0 {
			result.skippedZeroAscent++
			continue
		}
		samples, err := store.ActivityRideSamples(ctx, ride.TargetID, ride.WorkoutID)
		if err != nil {
			return nil, fmt.Errorf("reading a ride's samples: %w", err)
		}
		// A track is the positioned samples: a trainer ride has none, and no
		// altitude of its own to compare, so it lands here rather than being
		// studied as a flat ride.
		if len(samples.Track) < minSamples {
			result.skippedMinSamples++
			continue
		}

		altitudeMetres := make([]float64, len(samples.Track))
		distanceMetres := make([]float64, len(samples.Track))
		for index, sample := range samples.Track {
			altitudeMetres[index] = sample.AltitudeMetres
			distanceMetres[index] = sample.DistanceMetres
		}
		result.addQuantum(altitudeMetres)

		device := ride.AscentMetres

		var weatherReport, speedReport *report
		if splitsEnabled {
			weatherReport, err = weatherSplitFor(ctx, store, weatherCache, result.weatherSplits, ride)
			if err != nil {
				return nil, err
			}
			speedReport = speedSplitFor(result.speedSplits, ride)
		}

		recordAll(candidateRaw, measure.AscentMetres(altitudeMetres), device, result, weatherReport, speedReport)
		for _, threshold := range thresholdsMetres {
			recordAll(hystName(threshold), measure.AscentWithHysteresisMetres(altitudeMetres, threshold), device,
				result, weatherReport, speedReport)
		}

		stillAltitudeMetres := stillFilteredAltitudes(samples.Track, stillSpeedMS)
		for _, threshold := range thresholdsMetres {
			recordAll(stillHystName(threshold), measure.AscentWithHysteresisMetres(stillAltitudeMetres, threshold), device,
				result, weatherReport, speedReport)
		}

		if ride.MovingSeconds > 0 {
			if residual, ok := loopDrift(ctx, store, ride); ok {
				result.addDrift(residual, ride.MovingSeconds)
			}
		}

		profile, ok := measure.ProfileOf(distanceMetres, altitudeMetres)
		if !ok {
			result.skippedNonMonotonic++
			continue
		}
		smoothed := profile.Resample(routeIntervalMetres).MedianFiltered(routeIntervalMetres, routeWindowMetres)
		recordAll(candidateRoute, smoothed.AscentMetres(), device, result, weatherReport, speedReport)
		routeAltitudeMetres := smoothed.AltitudeMetres()
		for _, threshold := range thresholdsMetres {
			recordAll(routeHystName(threshold), measure.AscentWithHysteresisMetres(routeAltitudeMetres, threshold), device,
				result, weatherReport, speedReport)
		}
		for _, grid := range gridsMetres {
			gridAltitudeMetres := profile.Resample(grid).AltitudeMetres()
			for _, threshold := range thresholdsMetres {
				recordAll(gridHystName(grid, threshold), measure.AscentWithHysteresisMetres(gridAltitudeMetres, threshold), device,
					result, weatherReport, speedReport)
			}
		}
	}

	return result, nil
}
