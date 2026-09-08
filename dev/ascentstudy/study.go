package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
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

// validateCoverage rejects a -route-coverage or -ride-coverage that could not
// be a real share of a length: RouteMatch coverages live in (0, 1].
func validateCoverage(share float64, flagName string) error {
	if share <= 0 || share > 1 || math.IsNaN(share) {
		return fmt.Errorf("-%s must be a share greater than 0 and at most 1", flagName)
	}

	return nil
}

func hystName(thresholdMetres float64) string {
	return "hyst" + strconv.FormatFloat(thresholdMetres, 'g', -1, 64)
}

func routeHystName(thresholdMetres float64) string {
	return "route+" + hystName(thresholdMetres)
}

// routeProfileName is the matched-route candidate read straight off the stored
// library geometry, which the sync stored already resampled and median
// filtered for export: its raw steps are what route.Route.ElevationGainMetres
// and the stored Summary price today.
func routeProfileName() string { return "routeprofile" }

func routeProfileHystName(thresholdMetres float64) string {
	return routeProfileName() + "+" + hystName(thresholdMetres)
}

// crossRideHyst3VsRouteProfileHyst2 and crossRideHyst3VsRouteProfile are the
// two columns comparing a ride's own hysteresis-3 ascent against the route's
// figure instead of the device's, fixed at these two thresholds regardless of
// -thresholds: they answer how a route summary would compare with what the
// rider's own ride already says, not how either compares with the device.
const (
	crossRideHyst3VsRouteProfileHyst2 = "ride hyst3 vs routeprofile+hyst2"
	crossRideHyst3VsRouteProfile      = "ride hyst3 vs routeprofile"
)

// routeCandidateOrder is the fixed order the matched-routes report prints its
// columns in.
func routeCandidateOrder(thresholdsMetres []float64) []string {
	order := []string{routeProfileName()}
	for _, t := range thresholdsMetres {
		order = append(order, routeProfileHystName(t))
	}

	return append(order, crossRideHyst3VsRouteProfileHyst2, crossRideHyst3VsRouteProfile)
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
		run := track[index].DistanceMetres - track[index-1].DistanceMetres
		// A clock or an odometer that went backwards is a glitch, not a
		// standstill: the sample is kept as it is.
		if dtSeconds <= 0 || run < 0 {
			kept = append(kept, track[index].AltitudeMetres)

			continue
		}
		if run/dtSeconds < minSpeedMS {
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
// skippedRouteUnmatched and skippedRouteBelowCoverage are only counted for a
// ride that already passed the zero-ascent and min-samples gates above.
type report struct {
	errorsPercent                map[string][]float64
	quantumCounts                map[string]int
	routeReport                  *report
	order                        []string
	driftPerHourMetres           []float64
	weatherSplits                []namedReport
	speedSplits                  []namedReport
	skippedZeroAscent            int
	totalRides                   int
	skippedMinSamples            int
	skippedNonMonotonic          int
	skippedRouteUnmatched        int
	skippedRouteBelowCoverage    int
	skippedRouteMissingElevation int
	splitsEnabled                bool
	routesEnabled                bool
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
// first and last samples that carry a height, for a ride whose last positioned
// sample lies within loopEndsWithinMetres of its first; not a loop for any
// other ride, and an error only where the track could not be read.
func loopDrift(
	ctx context.Context, store *sqlite.Store, ride sqlite.RecordedRide,
) (residualMetres float64, loop bool, err error) {
	track, err := store.ActivityTrack(ctx, ride.TargetID, ride.WorkoutID)
	if err != nil {
		return 0, false, fmt.Errorf("reading a ride's track: %w", err)
	}
	if len(track) < 2 {
		return 0, false, nil
	}
	apart := measure.HaversineMetres(
		measure.Coordinate{Latitude: track[0].Latitude, Longitude: track[0].Longitude},
		measure.Coordinate{Latitude: track[len(track)-1].Latitude, Longitude: track[len(track)-1].Longitude})
	if apart > loopEndsWithinMetres {
		return 0, false, nil
	}
	// The height comes from the samples that carry one: a positioned sample
	// with no altitude says nothing about the barometer.
	firstIndex := slices.IndexFunc(track, func(point activity.TrackPoint) bool { return point.HasAltitude })
	if firstIndex < 0 {
		return 0, false, nil
	}
	lastIndex := len(track) - 1
	for lastIndex > firstIndex && !track[lastIndex].HasAltitude {
		lastIndex--
	}
	if lastIndex == firstIndex {
		return 0, false, nil
	}

	return track[lastIndex].AltitudeMetres - track[firstIndex].AltitudeMetres, true, nil
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
	fmt.Fprintf(&b, "rides: total=%d skipped_zero_device_ascent=%d skipped_short_or_unpositioned_track=%d skipped_route_non_monotonic=%d skipped_route_below_coverage=%d skipped_route_unmatched=%d\n",
		r.totalRides, r.skippedZeroAscent, r.skippedMinSamples, r.skippedNonMonotonic,
		r.skippedRouteBelowCoverage, r.skippedRouteUnmatched)

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

	if r.routesEnabled {
		fmt.Fprintln(&b)
		fmt.Fprintln(&b, "matched routes vs device (relative error %, positive = route over-reports)")
		b.WriteString(r.routeReport.tableString())
		fmt.Fprintf(&b, "  missing elevation: %d\n", r.skippedRouteMissingElevation)
	}

	fmt.Fprintln(&b)
	if len(r.driftPerHourMetres) == 0 {
		fmt.Fprintln(&b, "drift: no ride returned to its start")
	} else {
		median, q1, q3 := r.driftSummary()
		fmt.Fprintf(&b, "drift: median %.1f m/h (%.1f, %.1f) over %d rides that returned to their start\n",
			median, q1, q3, len(r.driftPerHourMetres))
	}

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

// routeProfile is one library route's geometry, decoded once and cached by
// key: distance and altitude are in the route's own stored (forward) order.
// hasElevation is false for a route missing any point's elevation or whose
// geometry could not be read, and its series are then unset.
type routeProfile struct {
	distanceMetres []float64
	altitudeMetres []float64
	hasElevation   bool
}

// loadRouteProfile reads and decodes one matched route's cached geometry, or
// returns the cached result of an earlier ride's match against the same
// route: the geometry cost is paid once per route, not once per ride.
func loadRouteProfile(
	ctx context.Context, store *sqlite.Store, cache map[route.Key]routeProfile, key route.Key,
) (routeProfile, error) {
	if cached, ok := cache[key]; ok {
		return cached, nil
	}
	_, coordinates, _, found, err := store.StageGeometry(ctx, key.Provider(), key.SourceRouteID(), key.StageOrder())
	if err != nil {
		return routeProfile{}, fmt.Errorf("reading a matched route's geometry: %w", err)
	}
	if !found {
		cache[key] = routeProfile{}

		return routeProfile{}, nil
	}
	var positions [][]float64
	if err := json.Unmarshal(coordinates, &positions); err != nil {
		return routeProfile{}, fmt.Errorf("decoding a matched route's geometry: %w", err)
	}
	altitudeMetres := make([]float64, len(positions))
	coordinatesForDistance := make([]measure.Coordinate, len(positions))
	for index, position := range positions {
		if len(position) < 3 {
			cache[key] = routeProfile{}

			return routeProfile{}, nil
		}
		coordinatesForDistance[index] = measure.Coordinate{Longitude: position[0], Latitude: position[1]}
		altitudeMetres[index] = position[2]
	}
	profile := routeProfile{
		distanceMetres: measure.CumulativeMetres(coordinatesForDistance),
		altitudeMetres: altitudeMetres,
		hasElevation:   true,
	}
	cache[key] = profile

	return profile, nil
}

// orientedAltitudes is a route's altitude series as the ride rode it: reversed
// for a ride that went the route backwards, since its descents were the
// route's ascents, and left as stored for a forward or unknown direction — an
// out-and-back ascends as much either way.
func orientedAltitudes(altitudeMetres []float64, direction activity.Direction) []float64 {
	oriented := append([]float64(nil), altitudeMetres...)
	if direction == activity.DirectionReverse {
		slices.Reverse(oriented)
	}

	return oriented
}

// scoreRouteMatch records one qualifying ride's matched-route ascent
// candidates against its device ascent, and the two cross columns that need
// no device figure at all.
func scoreRouteMatch(
	routeReport *report, profile routeProfile, direction activity.Direction,
	thresholdsMetres []float64, rideAltitudeMetres []float64, device float64,
) {
	routeAltitudes := orientedAltitudes(profile.altitudeMetres, direction)
	routeProfileAscent := measure.AscentMetres(routeAltitudes)
	routeReport.record(routeProfileName(), routeProfileAscent, device)
	for _, threshold := range thresholdsMetres {
		routeReport.record(routeProfileHystName(threshold), measure.AscentWithHysteresisMetres(routeAltitudes, threshold), device)
	}

	rideHyst3 := measure.AscentWithHysteresisMetres(rideAltitudeMetres, 3)
	routeProfileHyst2 := measure.AscentWithHysteresisMetres(routeAltitudes, 2)
	routeReport.record(crossRideHyst3VsRouteProfileHyst2, rideHyst3, routeProfileHyst2)
	routeReport.record(crossRideHyst3VsRouteProfile, rideHyst3, routeProfileAscent)
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
	routeCoverageMin, rideCoverageMin float64, routesEnabled bool,
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
	result.routesEnabled = routesEnabled
	if routesEnabled {
		result.routeReport = newReportWithOrder(routeCandidateOrder(thresholdsMetres))
	}
	weatherCache := map[string]map[int64]activity.WeatherSummary{}
	routeMatchCache := map[string]map[int64]activity.RouteMatch{}
	routeProfileCache := map[route.Key]routeProfile{}

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

		matches, cached := routeMatchCache[ride.TargetID]
		if !cached {
			matches, err = store.ActivityRouteMatches(ctx, ride.TargetID)
			if err != nil {
				return nil, fmt.Errorf("reading a target's route matches: %w", err)
			}
			routeMatchCache[ride.TargetID] = matches
		}
		switch match, found := matches[ride.WorkoutID]; {
		case !found:
			result.skippedRouteUnmatched++
		case match.RouteCoverage < routeCoverageMin || match.RideCoverage < rideCoverageMin:
			result.skippedRouteBelowCoverage++
		case routesEnabled:
			profile, profileErr := loadRouteProfile(ctx, store, routeProfileCache, match.Key)
			if profileErr != nil {
				return nil, profileErr
			}
			if !profile.hasElevation {
				result.skippedRouteMissingElevation++
			} else {
				scoreRouteMatch(result.routeReport, profile, match.Direction, thresholdsMetres, altitudeMetres, device)
			}
		}

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
			residual, ok, err := loopDrift(ctx, store, ride)
			if err != nil {
				return nil, err
			}
			if ok {
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
