package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

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

// parseThresholds splits a comma-separated hysteresis threshold list into
// metres, rejecting anything that does not parse as a number.
func parseThresholds(list string) ([]float64, error) {
	thresholds := make([]float64, 0)
	for part := range strings.SplitSeq(list, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing threshold %q: %w", trimmed, err)
		}
		thresholds = append(thresholds, value)
	}
	if len(thresholds) == 0 {
		return nil, errors.New("at least one threshold is required")
	}

	return thresholds, nil
}

func hystName(thresholdMetres float64) string {
	return "hyst" + strconv.FormatFloat(thresholdMetres, 'g', -1, 64)
}

func routeHystName(thresholdMetres float64) string {
	return "route+" + hystName(thresholdMetres)
}

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

// report accumulates one candidate's relative errors against the device
// ascent, and the corpus counts a reader needs to judge them by.
type report struct {
	errorsPercent map[string][]float64
	quantumCounts map[string]int
	order         []string
	totalRides    int
	skippedZeroAscent,
	skippedMinSamples,
	skippedNonMonotonic int
}

func newReport(thresholdsMetres []float64) *report {
	order := []string{candidateRaw, candidateRoute}
	for _, t := range thresholdsMetres {
		order = append(order, hystName(t))
	}
	for _, t := range thresholdsMetres {
		order = append(order, routeHystName(t))
	}
	errorsPercent := make(map[string][]float64, len(order))
	for _, name := range order {
		errorsPercent[name] = nil
	}

	return &report{order: order, errorsPercent: errorsPercent, quantumCounts: map[string]int{}}
}

func (r *report) record(name string, candidateAscent, deviceAscent float64) {
	r.errorsPercent[name] = append(r.errorsPercent[name], (candidateAscent/deviceAscent-1)*100)
}

func (r *report) addQuantum(altitudeMetres []float64) {
	if step, ok := smallestPositiveStep(altitudeMetres); ok {
		r.quantumCounts[quantumBucketLabel(step)]++
	}
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

func (r *report) String() string {
	var b strings.Builder
	fmt.Fprintln(&b, "ascent candidates vs device (relative error %, positive = candidate over-reports)")
	fmt.Fprintf(&b, "  %-14s %6s %10s %10s %10s %10s\n", "candidate", "rides", "median", "q1", "q3", "mean|err|")
	for _, name := range r.order {
		s := r.summarize(name)
		fmt.Fprintf(&b, "  %-14s %6d %9.1f%% %9.1f%% %9.1f%% %9.1f%%\n",
			s.name, s.rides, s.medianPercent, s.q1Percent, s.q3Percent, s.meanAbsPercent)
	}
	fmt.Fprintln(&b)
	fmt.Fprintln(&b, "altimeter quantum: smallest positive altitude step per ride, bucketed")
	for _, label := range quantumBucketLabels() {
		fmt.Fprintf(&b, "  %-5s %d\n", label, r.quantumCounts[label])
	}
	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "rides: total=%d skipped_zero_device_ascent=%d skipped_min_samples=%d skipped_route_non_monotonic=%d\n",
		r.totalRides, r.skippedZeroAscent, r.skippedMinSamples, r.skippedNonMonotonic)

	return b.String()
}

// study reads every recorded ride the store holds and scores each candidate
// ascent definition (see the package doc comment) against the ascent the
// device reported for it.
func study(ctx context.Context, store *sqlite.Store, thresholdsMetres []float64, minSamples int) (*report, error) {
	rides, err := store.RecordedRides(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing recorded rides: %w", err)
	}

	result := newReport(thresholdsMetres)
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
		result.record(candidateRaw, measure.AscentMetres(altitudeMetres), device)
		for _, threshold := range thresholdsMetres {
			result.record(hystName(threshold), measure.AscentWithHysteresisMetres(altitudeMetres, threshold), device)
		}

		profile, ok := measure.ProfileOf(distanceMetres, altitudeMetres)
		if !ok {
			result.skippedNonMonotonic++
			continue
		}
		smoothed := profile.Resample(routeIntervalMetres).MedianFiltered(routeIntervalMetres, routeWindowMetres)
		result.record(candidateRoute, smoothed.AscentMetres(), device)
		routeAltitudeMetres := smoothed.AltitudeMetres()
		for _, threshold := range thresholdsMetres {
			result.record(routeHystName(threshold), measure.AscentWithHysteresisMetres(routeAltitudeMetres, threshold), device)
		}
	}

	return result, nil
}
