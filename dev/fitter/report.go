package main

import (
	"fmt"
	"sort"
	"strings"
)

// speedHistogramBucketWidth is the bucket width for the report's speed
// histogram of surviving coasting windows. The issue asks for this on every
// run, "the one output that shows whether the fit was identifiable" — coarse
// enough to read at a glance, fine enough to show a narrow band as narrow.
const speedHistogramBucketWidth = 5.0

func renderReport(results []fitResult) string {
	var b strings.Builder
	for i := range results {
		renderGroup(&b, &results[i])
	}

	return b.String()
}

func renderGroup(b *strings.Builder, r *fitResult) {
	fmt.Fprintf(b, "== gear group: %s ==\n", groupLabel(r))
	if r.Skipped {
		fmt.Fprintf(b, "  skipped: %s\n\n", r.SkipReason)

		return
	}
	if r.UntaggedAttributed {
		fmt.Fprintln(b, "  untagged: the only group in this corpus, attributed to the corpus rather than a named bike")
	}

	fmt.Fprintf(b, "  coasting windows: %d surviving, %d rejected cornering, %d rejected plausibility\n",
		len(r.SurvivingWindows), r.CorneringRejected, r.PlausibilityReject)
	fmt.Fprintf(b, "  speed histogram of surviving windows:\n%s", renderSpeedHistogram(r.SurvivingWindows))
	fmt.Fprintf(b, "  condition ratio: %.1f%s\n", r.ConditionRatio, illConditionedNote(r.IllConditioned))

	if r.RejectedCrrBounds {
		fmt.Fprintln(b, "  REJECTED: fitted Crr is outside the tyre-relative plausibility band — remedy: pin Crr to a literature value for the configured tyre and refit CdA alone")

		return
	}
	if r.IllConditioned {
		fmt.Fprintln(b, "  REJECTED: coasting speeds are too narrowly clustered to separate Crr from CdA — remedy: pin Crr to a literature value and refit CdA alone, or gather coasting at a wider range of speeds")

		return
	}

	fmt.Fprintf(b, "  CdA: %.3f m^2\n", r.CdA)
	fmt.Fprintf(b, "  Crr (overall): %.5f\n", r.CrrOverall)
	renderCrrBySurface(b, r)
	fmt.Fprintf(b, "  climb threshold: %.1f%% (%.1f h of the corpus above it)\n", r.ClimbThresholdPct, r.ClimbHoursAbove)
	fmt.Fprintf(b, "  sustained power: %.0f W (partly a surface distribution, not purely a fitness one — see the corpus's own surface labelling)\n", r.PowerWatts)

	renderQuarterlyIntercept(b, r.QuarterlyIntercept)
	renderCrossCheck(b, r.CrossCheck)
	renderHeldOut(b, r.HeldOut)
	fmt.Fprintln(b)
}

func groupLabel(r *fitResult) string {
	if r.Group == untaggedGear {
		return "(untagged)"
	}

	return r.Group
}

func illConditionedNote(illConditioned bool) string {
	if illConditioned {
		return " (ill-conditioned)"
	}

	return ""
}

func renderCrrBySurface(b *strings.Builder, r *fitResult) {
	if len(r.CrrBySurface) == 0 {
		return
	}

	type entry struct {
		name  string
		value float64
	}
	entries := make([]entry, 0, len(r.CrrBySurface))
	for kind, value := range r.CrrBySurface {
		entries = append(entries, entry{kind.String(), value})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].name < entries[j].name })

	fmt.Fprintln(b, "  Crr by surface (fitted classes only; others fall back to the overall figure):")
	for _, e := range entries {
		fmt.Fprintf(b, "    %-10s %.5f\n", e.name, e.value)
	}
}

func renderSpeedHistogram(windows []coastingWindow) string {
	if len(windows) == 0 {
		return "    (no surviving windows)\n"
	}

	buckets := make(map[int]int)
	maxBucket := 0
	for _, w := range windows {
		bucket := int(w.MeanSpeedMPS*3.6/speedHistogramBucketWidth) * int(speedHistogramBucketWidth)
		buckets[bucket]++
		if bucket > maxBucket {
			maxBucket = bucket
		}
	}

	var b strings.Builder
	for lower := 0; lower <= maxBucket; lower += int(speedHistogramBucketWidth) {
		if buckets[lower] == 0 {
			continue
		}
		fmt.Fprintf(&b, "    %3d-%3d km/h: %d\n", lower, lower+int(speedHistogramBucketWidth), buckets[lower])
	}

	return b.String()
}

// renderQuarterlyIntercept prints the per-ride rolling-resistance intercept
// by quarter for a human to read a step in — this fitter itself never
// infers one, per the issue's own warning that an automatic split invents
// regimes on a thin corpus that are not there.
func renderQuarterlyIntercept(b *strings.Builder, quarters []quarterlyCrr) {
	if len(quarters) == 0 {
		fmt.Fprintln(b, "  per-ride Crr intercept by quarter: no quarter had enough rides to report")

		return
	}
	fmt.Fprintln(b, "  per-ride Crr intercept by quarter (median; read for a step yourself, this run never infers one):")
	for _, q := range quarters {
		fmt.Fprintf(b, "    %-8s %.5f (%d rides)\n", q.Quarter, q.Median, q.Rides)
	}
}

func renderCrossCheck(b *strings.Builder, c indoorCrossCheckSummary) {
	if c.Rides == 0 {
		fmt.Fprintln(b, "  indoor heart-rate cross-check: no rides carried both channels")

		return
	}
	fmt.Fprintf(b, "  indoor heart-rate cross-check: %d rides, median ratio %.2f, correlation %.2f\n", c.Rides, c.MedianRatio, c.Correlation)
	fmt.Fprintln(b, "    caveats: indoor heart rate runs high for a given power (biases the check toward understating outdoor power);")
	fmt.Fprintln(b, "    ERG-mode workouts quantise the relation; a thin year's relation is unreliable")
	if len(c.ThinYears) > 0 {
		fmt.Fprintf(b, "    thin years (under %.0f indoor hours): %v\n", minIndoorHoursPerYear, c.ThinYears)
	}
}

func renderHeldOut(b *strings.Builder, h heldOutValidation) {
	if h.Rides == 0 {
		fmt.Fprintln(b, "  held-out validation: no held-out rides had a computable prediction")

		return
	}
	fmt.Fprintf(b, "  held-out validation (%d rides, split by date): model MAE %.1f%%, trivial-baseline MAE %.1f%%\n",
		h.Rides, h.ModelMAE, h.BaselineMAE)
}
