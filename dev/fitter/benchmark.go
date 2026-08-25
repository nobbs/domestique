package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/elevation"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

const (
	defaultRouteCellDegrees      = 0.002
	defaultRouteJaccardThreshold = 0.7
	// defaultTrainingWindowMonths bounds how far back a fit reaches. #251
	// measured the choice on the operator's corpus: accuracy is flat from six
	// months to all history, so the window buys no error today. It is here so
	// a profile can follow a rider whose form actually moves — an all-history
	// fit takes years to forget a season — and twelve months is the shortest
	// window that still spans a full year of weather and daylight.
	defaultTrainingWindowMonths = 12
	// rollingHorizonMonths is how much of the future one fold scores. It
	// matches the interval a profile is expected to be refit at, so a fold
	// measures a profile over the same span of staleness it will really serve.
	rollingHorizonMonths = 1
	// modelVersionLabel mirrors internal/ridemodel/model.go's unexported
	// modelVersion constant, for display only: this package never loads or
	// computes it, it just needs the same label on the profile it prints.
	modelVersionLabel = "hybrid-v2"
	// physicsOnlyScaleFactor recovers the physics-only prediction from
	// internal/ridemodel.Predict's own equal-weight blend: with the route
	// coefficients zeroed, every segment's blended time is exactly half its
	// physics time (#213's settled 50/50 split), so doubling Predict's own
	// output — the real equation, with the route half silenced, not a second
	// implementation of it — recovers the physics-only prediction exactly.
	physicsOnlyScaleFactor = 2.0
	// maxMissingGeometryFraction is how much of a ride's moving time may come
	// from samples missing altitude or position (a GPS/barometer dropout)
	// before the ride is unscorable here.
	maxMissingGeometryFraction = 0.05
)

type benchmarkModel struct {
	predict func([]sampleRow) float64
	name    string
	detail  string
}

type benchmarkMetrics struct {
	rides int
	bias  float64
	mae   float64
	p90   float64
}

type routeCell struct {
	latitude  int
	longitude int
}

// splitEvaluation is one calibrate-once, score-once pass: the models it built
// and the per-model prediction errors scored against the route-disjoint
// evaluation rides.
type splitEvaluation struct {
	calibrationCutoff time.Time
	errorsByModel     map[string][]float64
	models            []benchmarkModel
	clusterCount      int
	repeatedRides     int
	largestCluster    int
	seenCount         int
	evaluateCount     int
	evaluateScored    int
	secondsPerKM      float64
	secondsPerAscentM float64
}

// recalibration is one walk-forward run: the coefficients it ends up
// recommending, and the out-of-sample errors pooled across every fold that
// produced them. The two are deliberately not the same fit — see
// runRecalibration.
type recalibration struct {
	errorsByModel     map[string][]float64
	firstOrigin       time.Time
	lastOrigin        time.Time
	calibrationCutoff time.Time
	modelNames        []string
	folds             int
	scored            int
	trainRides        int
	windowMonths      int
	secondsPerKM      float64
	secondsPerAscentM float64
}

// runETABenchmark implements the repeat protocol: by default it evaluates
// the loaded, still-frozen profile against rides after its own
// calibration_cutoff — no fitting at all — and under -recalibrate it walks a
// monthly origin across the corpus, refitting the two route coefficients per
// fold, and prints a copy-ready profile carrying the pooled out-of-sample
// error. Every model it scores — hybrid, physics-only, route-only — is built
// from a ridemodel.Coefficients value, and the hybrid model calls
// internal/ridemodel.Predict directly: the real production equation, never a
// second implementation of it.
func runETABenchmark(
	groups []rideGroup, rides []rideRow, samplesByRide map[string][]sampleRow,
	coefficients *ridemodel.Coefficients, cfg *runConfig,
) (string, error) {
	var report strings.Builder
	var benchmarkErrors []error

	cutoff, err := time.Parse(time.DateOnly, coefficients.CalibrationCutoff)
	if err != nil {
		return "", fmt.Errorf("coefficient file's calibration_cutoff: %w", err)
	}

	if cfg.recalibrate {
		fmt.Fprintf(&report, "ETA recalibration: walking a monthly origin across the corpus, each fold fit on the %d months before it and scored on the unseen routes of the month after; the shipped fit then covers the newest %d months\n",
			cfg.etaTrainingMonths, cfg.etaTrainingMonths)
	} else {
		fmt.Fprintf(&report, "ETA evaluation: the frozen profile calibrated through %s, scored on route-disjoint first attempts after that date\n", cutoff.Format(time.DateOnly))
	}
	fmt.Fprintf(&report, "Errors are signed bias, mean absolute error and p90 absolute error, all as percentages of moving time. A repeat requires Jaccard overlap %.2f on a %.4f-degree coordinate grid.\n",
		cfg.etaRouteJaccard, cfg.etaRouteCellDegrees)

	for _, group := range groups {
		if group.Skipped {
			fmt.Fprintf(&report, "\n%s: skipped (%s)\n", displayGear(group.Gear), group.SkipReason)

			continue
		}

		groupRides := ridesInGroup(rides, group)
		cleanRides, exclusions := benchmarkEligibleRides(groupRides, samplesByRide)
		fmt.Fprintf(&report, "\n%s: %d/%d hygienic rides\n", displayGear(group.Gear), len(cleanRides), len(groupRides))
		if len(exclusions) > 0 {
			fmt.Fprintf(&report, "  excluded: %s\n", renderExclusions(exclusions))
		}

		if cfg.recalibrate {
			clusters, clusterCount, repeatedRides, largestCluster := clusterRoutes(
				cleanRides, samplesByRide, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard)
			fmt.Fprintf(&report, "  %d route clusters, %d repeats, largest cluster %d\n",
				clusterCount, repeatedRides, largestCluster)

			eval, recalibrateErr := runRecalibration(cleanRides, samplesByRide, clusters, coefficients, cfg)
			if recalibrateErr != nil {
				fmt.Fprintf(&report, "  skipped (%v)\n", recalibrateErr)
				benchmarkErrors = append(benchmarkErrors, fmt.Errorf("%s: %w", displayGear(group.Gear), recalibrateErr))

				continue
			}
			printRecalibration(&report, &eval)
			printCopyReadyProfile(&report, coefficients, &eval)

			continue
		}

		eval, err := evaluateSplit(cleanRides, samplesByRide, coefficients, cutoff, cfg)
		if err != nil {
			fmt.Fprintf(&report, "  skipped (%v)\n", err)
			benchmarkErrors = append(benchmarkErrors, fmt.Errorf("%s: %w", displayGear(group.Gear), err))

			continue
		}

		printSplitSummary(&report, &eval)
		printModelMetrics(&report, &eval)
		printCandidateComparison(&report, &eval)
	}

	return report.String(), errors.Join(benchmarkErrors...)
}

// evaluateSplit scores the loaded, still-frozen profile: it clusters routes,
// splits chronologically at the profile's own calibration_cutoff, and scores
// every model once against the route-disjoint rides after it. Nothing is
// fitted here — this is the "how is the file I am running doing" pass, and
// -recalibrate takes runRecalibration's road instead.
//
// Evaluation rides are additionally confined to the trailing window, because
// a profile is a claim about how the rider rides *now*: scoring it against
// everything since its cutoff let a year-old ride offset a present-day error
// and report the average as if it described today.
func evaluateSplit(
	cleanRides []rideRow, samplesByRide map[string][]sampleRow,
	coefficients *ridemodel.Coefficients, cutoff time.Time, cfg *runConfig,
) (splitEvaluation, error) {
	clusters, clusterCount, repeatedRides, largestCluster := clusterRoutes(cleanRides, samplesByRide, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard)

	// calibration_cutoff is a date, not an instant — "the last calibration
	// ride's date" — so every ride on that calendar date counts as seen,
	// whatever time of day it was recorded. Comparing against the start of
	// the following day, rather than cutoff itself, is what keeps a
	// same-day ride from being misclassified as unseen.
	seenCount := sort.Search(len(cleanRides), func(i int) bool { return !cleanRides[i].Date.Before(cutoff.AddDate(0, 0, 1)) })
	seen, evaluate := routeDisjointSplit(cleanRides, clusters, seenCount)
	if len(evaluate) == 0 {
		return splitEvaluation{}, errors.New("no route-disjoint evaluation rides after the cutoff")
	}
	newest := cleanRides[len(cleanRides)-1].Date
	evaluate = ridesBetween(evaluate, newest.AddDate(0, -cfg.etaTrainingMonths, 0), newest.AddDate(0, 0, 1))
	if len(evaluate) == 0 {
		return splitEvaluation{}, fmt.Errorf(
			"no route-disjoint evaluation ride in the %d months before %s",
			cfg.etaTrainingMonths, newest.Format(time.DateOnly))
	}

	active := *coefficients
	models := benchmarkModels(&active)
	errorsByModel, scored := scoreRides(models, evaluate, samplesByRide)
	if scored == 0 {
		return splitEvaluation{}, errors.New("no evaluation ride could be scored")
	}

	return splitEvaluation{
		clusterCount: clusterCount, repeatedRides: repeatedRides, largestCluster: largestCluster,
		seenCount: len(seen), evaluateCount: len(evaluate), evaluateScored: scored,
		calibrationCutoff: cutoff, models: models, errorsByModel: errorsByModel,
		secondsPerKM: active.SecondsPerKM, secondsPerAscentM: active.SecondsPerAscentM,
	}, nil
}

func printSplitSummary(report *strings.Builder, eval *splitEvaluation) {
	fmt.Fprintf(report, "  %d route clusters, %d repeats, largest cluster %d\n",
		eval.clusterCount, eval.repeatedRides, eval.largestCluster)
	fmt.Fprintf(report, "  frozen profile calibrated through %s; %d rides already seen by then\n",
		eval.calibrationCutoff.Format(time.DateOnly), eval.seenCount)
	fmt.Fprintf(report, "  evaluation: %d/%d route-disjoint first attempts scored\n", eval.evaluateScored, eval.evaluateCount)
}

func printModelMetrics(report *strings.Builder, eval *splitEvaluation) {
	bestName, bestMAE := "", math.Inf(1)
	for _, model := range eval.models {
		metrics := summarizeBenchmarkErrors(eval.errorsByModel[model.name])
		fmt.Fprintf(report, "    %-14s bias %+6.2f  MAE %6.2f  p90 %6.2f", model.name, metrics.bias, metrics.mae, metrics.p90)
		if model.detail != "" {
			fmt.Fprintf(report, "  (%s)", model.detail)
		}
		fmt.Fprintln(report)
		if metrics.mae < bestMAE {
			bestName, bestMAE = model.name, metrics.mae
		}
	}
	fmt.Fprintf(report, "  lowest MAE: %s (%.2f%%)\n", bestName, bestMAE)
}

// printCandidateComparison reports the hybrid model against the physics-only
// and route-only diagnostics the issue's "Keep" list names, so a reader can
// see whether the hybrid is earning its blend rather than being carried by
// one half.
func printCandidateComparison(report *strings.Builder, eval *splitEvaluation) {
	candidate := eval.errorsByModel["hybrid"]
	if len(candidate) == 0 {
		return
	}
	fmt.Fprintln(report, "  hybrid vs diagnostic baselines (positive is the hybrid doing better):")
	if physicsOnly := eval.errorsByModel["physics-only"]; len(physicsOnly) > 0 {
		improvement, low, high := pairedMAEImprovement(physicsOnly, candidate)
		fmt.Fprintf(report, "    vs physics-only   %+6.2f pp  95%% bootstrap [%+6.2f, %+6.2f]\n", improvement, low, high)
	}
	if routeOnly := eval.errorsByModel["route-only"]; len(routeOnly) > 0 {
		improvement, low, high := pairedMAEImprovement(routeOnly, candidate)
		fmt.Fprintf(report, "    vs route-only     %+6.2f pp  95%% bootstrap [%+6.2f, %+6.2f]\n", improvement, low, high)
	}
}

// printCopyReadyProfile prints the values an operator pastes into
// ridemodel.toml after an explicitly requested recalibration: the newly
// fitted route coefficients, the profile's unchanged physics inputs, the
// window and cutoff that bound the data they came from, and the pooled
// out-of-sample error that justifies adopting them.
//
// Those metrics measure the *procedure*, not these exact numbers — the
// shipped fit covers the newest window and so has no held-out future left to
// score against. See runRecalibration for why that is the honest arrangement
// rather than a gap.
func printCopyReadyProfile(report *strings.Builder, coefficients *ridemodel.Coefficients, eval *recalibration) {
	candidate := eval.errorsByModel["hybrid"]
	if len(candidate) == 0 {
		return
	}
	metrics := summarizeBenchmarkErrors(candidate)
	fmt.Fprintf(report, "  copy-ready profile (%s):\n", modelVersionLabel)
	fmt.Fprintf(report, "    calibration_cutoff = \"%s\"\n", eval.calibrationCutoff.Format(time.DateOnly))
	fmt.Fprintf(report, "    mass_kg = %.1f\n", coefficients.MassKG)
	fmt.Fprintf(report, "    power_watts = %.0f\n", coefficients.PowerWatts)
	fmt.Fprintf(report, "    cda_m2 = %.2f\n", coefficients.CdAM2)
	fmt.Fprintf(report, "    crr = %.3f\n", coefficients.CrrBySurface[surface.KindAsphalt])
	fmt.Fprintf(report, "    seconds_per_km = %.4f\n", eval.secondsPerKM)
	fmt.Fprintf(report, "    seconds_per_ascent_m = %.4f\n", eval.secondsPerAscentM)
	fmt.Fprintf(report, "    evaluated_rides = %d\n", metrics.rides)
	fmt.Fprintf(report, "    bias_percent = %.2f\n", metrics.bias)
	fmt.Fprintf(report, "    mae_percent = %.2f\n", metrics.mae)
	fmt.Fprintf(report, "    p90_percent = %.2f\n", metrics.p90)
	fmt.Fprintf(report, "    training_window_months = %d\n", eval.windowMonths)
	fmt.Fprintf(report, "    validation: %d unseen-route scorings over %d folds, bias %+.2f%%, MAE %.2f%%, p90 %.2f%%\n",
		metrics.rides, eval.folds, metrics.bias, metrics.mae, metrics.p90)
}

func displayGear(gear string) string {
	if gear == untaggedGear {
		return "untagged gear"
	}

	return gear
}

func ridesInGroup(rides []rideRow, group rideGroup) []rideRow {
	result := make([]rideRow, 0, len(group.RideIDs))
	for _, ride := range rides {
		if group.RideIDs[ride.RideID] {
			result = append(result, ride)
		}
	}

	return result
}

func benchmarkEligibleRides(
	rides []rideRow, samplesByRide map[string][]sampleRow,
) (eligible []rideRow, exclusions map[string]int) {
	eligible = make([]rideRow, 0, len(rides))
	exclusions = make(map[string]int)
	for _, ride := range rides {
		reason := benchmarkExclusionReason(ride, samplesByRide[ride.RideID])
		if reason != "" {
			exclusions[reason]++

			continue
		}
		eligible = append(eligible, ride)
	}
	sortRides(eligible)

	return eligible, exclusions
}

func benchmarkExclusionReason(ride rideRow, samples []sampleRow) string {
	if !finitePositive(ride.MovingSeconds) {
		return "invalid moving time"
	}
	if len(samples) == 0 {
		return "missing samples"
	}

	var movingSeconds, missingGeometrySeconds float64
	for i := range samples {
		sample := &samples[i]
		if !sample.Moving {
			continue
		}
		if !finitePositive(sample.DeltaSeconds) || !finiteNonNegative(sample.IntervalDistance) ||
			!isFinite(sample.SpeedMPS) || !isFinite(sample.GradientPercent) {
			return "invalid moving sample"
		}
		calculatedSpeed := sample.IntervalDistance / sample.DeltaSeconds
		if math.Abs(sample.SpeedMPS-calculatedSpeed) > 1e-6*math.Max(1, math.Abs(calculatedSpeed)) {
			return "inconsistent speed"
		}
		movingSeconds += sample.DeltaSeconds
		if !sample.HasPosition || !sample.HasAltitude {
			missingGeometrySeconds += sample.DeltaSeconds
		}
		if sample.HasPosition && (!isFinite(sample.Latitude) || !isFinite(sample.Longitude) ||
			math.Abs(sample.Latitude) > 90 || math.Abs(sample.Longitude) > 180) {
			return "invalid position"
		}
		if sample.HasAltitude && !isFinite(sample.AltitudeM) {
			return "invalid altitude"
		}
	}
	if movingSeconds == 0 {
		return "no moving samples"
	}
	if math.Abs(movingSeconds-ride.MovingSeconds)/ride.MovingSeconds > 0.01 {
		return "moving-time mismatch"
	}
	if missingGeometrySeconds/movingSeconds > maxMissingGeometryFraction {
		return "incomplete geometry"
	}
	if len(routeSignature(samples, defaultRouteCellDegrees)) == 0 {
		return "missing route geometry"
	}

	return ""
}

func isFinite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }

func finitePositive(value float64) bool { return isFinite(value) && value > 0 }

func finiteNonNegative(value float64) bool { return isFinite(value) && value >= 0 }

func renderExclusions(exclusions map[string]int) string {
	reasons := make([]string, 0, len(exclusions))
	for reason := range exclusions {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	parts := make([]string, len(reasons))
	for i, reason := range reasons {
		parts[i] = fmt.Sprintf("%s=%d", reason, exclusions[reason])
	}

	return strings.Join(parts, ", ")
}

func sortRides(rides []rideRow) {
	sort.Slice(rides, func(i, j int) bool {
		if rides[i].Date.Equal(rides[j].Date) {
			return rides[i].RideID < rides[j].RideID
		}

		return rides[i].Date.Before(rides[j].Date)
	})
}

func routeSignature(samples []sampleRow, cellDegrees float64) map[routeCell]struct{} {
	signature := make(map[routeCell]struct{})
	for i := range samples {
		if !samples[i].Moving || !samples[i].HasPosition {
			continue
		}
		signature[routeCell{
			latitude:  int(math.Round(samples[i].Latitude / cellDegrees)),
			longitude: int(math.Round(samples[i].Longitude / cellDegrees)),
		}] = struct{}{}
	}

	return signature
}

func clusterRoutes(
	rides []rideRow, samplesByRide map[string][]sampleRow, cellDegrees, jaccardThreshold float64,
) (clusterByRide map[string]int, clusterCount, repeatedRides, largestCluster int) {
	signatures := make([]map[routeCell]struct{}, len(rides))
	parents := make([]int, len(rides))
	for i, ride := range rides {
		signatures[i] = routeSignature(samplesByRide[ride.RideID], cellDegrees)
		parents[i] = i
	}

	// ponytail: O(n²) is simpler and takes milliseconds for a personal corpus.
	// Replace it with an inverted cell index only if the corpus reaches thousands of rides.
	for left := range rides {
		for right := left + 1; right < len(rides); right++ {
			if routeJaccard(signatures[left], signatures[right], jaccardThreshold) >= jaccardThreshold {
				unionClusters(parents, left, right)
			}
		}
	}

	clusterByRoot := make(map[int]int)
	clusterByRide = make(map[string]int, len(rides))
	clusterSizes := make(map[int]int)
	for i, ride := range rides {
		root := clusterRoot(parents, i)
		cluster, ok := clusterByRoot[root]
		if !ok {
			cluster = len(clusterByRoot)
			clusterByRoot[root] = cluster
		}
		clusterByRide[ride.RideID] = cluster
		clusterSizes[cluster]++
	}

	for _, size := range clusterSizes {
		repeatedRides += size - 1
		largestCluster = max(largestCluster, size)
	}

	return clusterByRide, len(clusterByRoot), repeatedRides, largestCluster
}

func routeJaccard(left, right map[routeCell]struct{}, threshold float64) float64 {
	if len(left) == 0 || len(right) == 0 {
		return 0
	}
	if len(left) > len(right) {
		left, right = right, left
	}
	if float64(len(left))/float64(len(right)) < threshold {
		return 0
	}
	intersection := 0
	for cell := range left {
		if _, ok := right[cell]; ok {
			intersection++
		}
	}

	return float64(intersection) / float64(len(left)+len(right)-intersection)
}

func clusterRoot(parents []int, index int) int {
	for parents[index] != index {
		parents[index] = parents[parents[index]]
		index = parents[index]
	}

	return index
}

func unionClusters(parents []int, left, right int) {
	leftRoot, rightRoot := clusterRoot(parents, left), clusterRoot(parents, right)
	if leftRoot != rightRoot {
		parents[rightRoot] = leftRoot
	}
}

// routeDisjointSplit orders rides chronologically and draws one cutoff at the
// given index: everything before it is "seen", and everything after it is
// scored — but only a ride whose route cluster never appeared in "seen" and
// has not already been selected, so evaluation sees each unseen route
// exactly once. seenCount comes from a date search against the loaded
// profile's own calibration_cutoff; the rolling-origin path uses
// unseenRoutesIn instead, which applies the same rule per fold.
func routeDisjointSplit(rides []rideRow, clusters map[string]int, seenCount int) (seen, evaluate []rideRow) {
	if len(rides) < minGroupRides {
		return nil, nil
	}
	seen = rides[:seenCount]

	seenClusters := make(map[int]bool, seenCount)
	for _, ride := range seen {
		seenClusters[clusters[ride.RideID]] = true
	}
	selected := make(map[int]bool)
	for _, ride := range rides[seenCount:] {
		cluster := clusters[ride.RideID]
		if seenClusters[cluster] || selected[cluster] {
			continue
		}
		selected[cluster] = true
		evaluate = append(evaluate, ride)
	}

	return seen, evaluate
}

// fitRouteCoefficients fits seconds_per_km and seconds_per_ascent_m by
// weighted least squares against a set of rides' distance, ascent and
// moving time — the only recalibration this package ever performs; mass,
// power, drag area and rolling resistance are never re-derived from ride
// data.
func fitRouteCoefficients(train []rideRow, samplesByRide map[string][]sampleRow) (secondsPerKM, secondsPerAscentM float64, err error) {
	observations := make([]coastingObservation, 0, len(train))
	for _, ride := range train {
		distanceKM, ascentM := distanceAndAscent(samplesByRide[ride.RideID])
		if distanceKM <= 0 || ride.MovingSeconds <= 0 {
			continue
		}
		observations = append(observations, coastingObservation{
			Y: ride.MovingSeconds, X1: distanceKM, X2: ascentM, Weight: 1,
		})
	}
	secondsPerKM, secondsPerAscentM, _ = irlsFit(observations)
	if !finitePositive(secondsPerKM) || !finitePositive(secondsPerAscentM) {
		return 0, 0, fmt.Errorf("route coefficient fit is invalid (%.2f s/km, %.3f s/m)", secondsPerKM, secondsPerAscentM)
	}

	return secondsPerKM, secondsPerAscentM, nil
}

// benchmarkModels builds the three models every evaluation scores from one
// coefficient set: the production hybrid and the two halves it blends, so a
// reader can see whether the hybrid is earning its blend.
func benchmarkModels(coefficients *ridemodel.Coefficients) []benchmarkModel {
	return []benchmarkModel{
		physicsOnlyModel(coefficients),
		routeOnlyModel(coefficients.SecondsPerKM, coefficients.SecondsPerAscentM),
		hybridModel(coefficients),
	}
}

// hybridModel is the exact production predictor: internal/ridemodel.Predict,
// called directly with the loaded (or recalibrated) coefficients. Never a
// second implementation of the equation.
func hybridModel(coefficients *ridemodel.Coefficients) benchmarkModel {
	return benchmarkModel{
		name:   "hybrid",
		detail: fmt.Sprintf("%.4f s/km + %.4f s/m", coefficients.SecondsPerKM, coefficients.SecondsPerAscentM),
		predict: func(samples []sampleRow) float64 {
			return predictHybrid(samples, coefficients)
		},
	}
}

// physicsOnlyModel isolates the hybrid's physics half by zeroing the route
// coefficients and doubling internal/ridemodel.Predict's own output — see
// physicsOnlyScaleFactor's doc comment. Still the real equation, not a copy
// of it.
func physicsOnlyModel(coefficients *ridemodel.Coefficients) benchmarkModel {
	physicsOnly := *coefficients
	physicsOnly.SecondsPerKM, physicsOnly.SecondsPerAscentM = 0, 0

	return benchmarkModel{
		name:   "physics-only",
		detail: fmt.Sprintf("CdA %.2f, %.0f W", coefficients.CdAM2, coefficients.PowerWatts),
		predict: func(samples []sampleRow) float64 {
			return physicsOnlyScaleFactor * predictHybrid(samples, &physicsOnly)
		},
	}
}

// routeOnlyModel is the linear route correction alone, with no physics
// contribution at all — a pure arithmetic diagnostic, never routed through
// internal/ridemodel.Predict, since Predict has no way to weight the physics
// half at zero.
func routeOnlyModel(secondsPerKM, secondsPerAscentM float64) benchmarkModel {
	return benchmarkModel{
		name:   "route-only",
		detail: fmt.Sprintf("%.4f s/km + %.4f s/m", secondsPerKM, secondsPerAscentM),
		predict: func(samples []sampleRow) float64 {
			distanceKM, ascentM := distanceAndAscent(samples)

			return secondsPerKM*distanceKM + secondsPerAscentM*ascentM
		},
	}
}

func predictHybrid(samples []sampleRow, coefficients *ridemodel.Coefficients) float64 {
	stage, ok := normalizedRideStage(samples)
	if !ok {
		return 0
	}

	result, ok := ridemodel.Predict(stage.Geometry(), nil, *coefficients)
	if !ok {
		return 0
	}

	return result.MovingSeconds
}

func distanceAndAscent(samples []sampleRow) (distanceKM, ascentM float64) {
	stage, ok := normalizedRideStage(samples)
	if !ok {
		return 0, 0
	}

	return stage.DistanceMetres() / 1000, stage.ElevationGainMetres()
}

// normalizedRideStage converts a ridden trace into the same elevation
// profile production stores and predicts: the normalizer preserves the GPS
// line while resampling and median-filtering its altitude channel. A moving
// sample missing altitude or position (a GPS/barometer dropout) is dropped
// from the point sequence rather than making the ride unscorable outright —
// see maxMissingGeometryFraction for why a small amount of this is
// tolerated.
func normalizedRideStage(samples []sampleRow) (route.Stage, bool) {
	points := make([]route.Point, 0, len(samples))
	var movingSeconds, missingGeometrySeconds float64
	for i := range samples {
		s := &samples[i]
		if !s.Moving {
			continue
		}
		movingSeconds += s.DeltaSeconds
		if !s.HasAltitude || !s.HasPosition {
			missingGeometrySeconds += s.DeltaSeconds

			continue
		}
		altitude := s.AltitudeM
		points = append(points, route.Point{Latitude: s.Latitude, Longitude: s.Longitude, Elevation: &altitude})
	}
	if movingSeconds > 0 && missingGeometrySeconds/movingSeconds > maxMissingGeometryFraction {
		return route.Stage{}, false
	}
	stage, err := route.NewStage(
		route.ProviderVeloPlanner, 1, 1, "benchmark", "benchmark", "", points, "benchmark",
	)
	if err != nil {
		return route.Stage{}, false
	}
	normalized, err := elevation.New().Process(&stage)
	if err != nil {
		return route.Stage{}, false
	}

	return normalized, true
}

func scoreRides(
	models []benchmarkModel, rides []rideRow, samplesByRide map[string][]sampleRow,
) (errorsByModel map[string][]float64, scored int) {
	errorsByModel = make(map[string][]float64, len(models))
	for _, ride := range rides {
		predictions := make([]float64, len(models))
		valid := true
		for i, model := range models {
			predictions[i] = model.predict(samplesByRide[ride.RideID])
			if !finitePositive(predictions[i]) {
				valid = false
			}
		}
		if !valid {
			continue
		}
		for i, model := range models {
			errorPercent := 100 * (predictions[i] - ride.MovingSeconds) / ride.MovingSeconds
			errorsByModel[model.name] = append(errorsByModel[model.name], errorPercent)
		}
	}

	if len(models) == 0 {
		return errorsByModel, 0
	}

	return errorsByModel, len(errorsByModel[models[0].name])
}

func summarizeBenchmarkErrors(errorPercentages []float64) benchmarkMetrics {
	if len(errorPercentages) == 0 {
		return benchmarkMetrics{}
	}
	absolute := make([]float64, len(errorPercentages))
	for i, value := range errorPercentages {
		absolute[i] = math.Abs(value)
	}
	sort.Float64s(absolute)

	return benchmarkMetrics{
		rides: len(errorPercentages),
		bias:  meanOf(errorPercentages),
		mae:   meanOf(absolute),
		p90:   percentileOf(absolute, 0.9),
	}
}

func meanOf(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}

	return sum / float64(len(values))
}

func pairedMAEImprovement(current, candidate []float64) (mean, low, high float64) {
	if len(current) == 0 || len(current) != len(candidate) {
		return 0, 0, 0
	}
	differences := make([]float64, len(current))
	for i := range current {
		differences[i] = math.Abs(current[i]) - math.Abs(candidate[i])
	}
	mean = meanOf(differences)

	const bootstrapSamples = 10_000
	bootstrapMeans := make([]float64, bootstrapSamples)
	random := rand.New(rand.NewSource(1)) //nolint:gosec // Fixed-seed resampling is reproducible statistics, not security.
	for sample := range bootstrapSamples {
		var sum float64
		for range differences {
			sum += differences[random.Intn(len(differences))]
		}
		bootstrapMeans[sample] = sum / float64(len(differences))
	}
	sort.Float64s(bootstrapMeans)

	return mean, percentileOf(bootstrapMeans, 0.025), percentileOf(bootstrapMeans, 0.975)
}

// trainingWindow returns the rides a fit calibrates from: those inside the
// trailing window of the given length ending at origin. Bounding it is what
// lets a profile follow the rider's current form instead of averaging over
// every season on record.
//
// The window is extended backwards when it holds too few rides to fit from,
// so an injury or a quiet winter narrows the training set rather than
// emptying it. rides must be sorted by date, which benchmarkEligibleRides
// guarantees.
func trainingWindow(rides []rideRow, origin time.Time, windowMonths int) []rideRow {
	end := sort.Search(len(rides), func(i int) bool { return !rides[i].Date.Before(origin) })
	start := sort.Search(len(rides), func(i int) bool {
		return !rides[i].Date.Before(origin.AddDate(0, -windowMonths, 0))
	})
	if end-start < minGroupRides {
		start = max(0, end-minGroupRides)
	}

	return rides[start:end]
}

// unseenRoutesIn selects the rides of horizon whose route cluster never
// appears in train, keeping each cluster only once, so a fold scores every
// route its fit had not been calibrated on exactly one time.
func unseenRoutesIn(horizon, train []rideRow, clusters map[string]int) []rideRow {
	trained := make(map[int]bool, len(train))
	for _, ride := range train {
		trained[clusters[ride.RideID]] = true
	}
	selected := make(map[int]bool)
	unseen := make([]rideRow, 0, len(horizon))
	for _, ride := range horizon {
		cluster := clusters[ride.RideID]
		if trained[cluster] || selected[cluster] {
			continue
		}
		selected[cluster] = true
		unseen = append(unseen, ride)
	}

	return unseen
}

// ridesBetween returns the rides dated on or after start and before end.
// rides must be sorted by date.
func ridesBetween(rides []rideRow, start, end time.Time) []rideRow {
	from := sort.Search(len(rides), func(i int) bool { return !rides[i].Date.Before(start) })
	to := sort.Search(len(rides), func(i int) bool { return !rides[i].Date.Before(end) })

	return rides[from:to]
}

// monthStart truncates a date to the first of its month, which is where every
// rolling origin sits.
func monthStart(date time.Time) time.Time {
	return time.Date(date.Year(), date.Month(), 1, 0, 0, 0, 0, date.Location())
}

// runRecalibration is the calibration protocol #251 replaced the single
// oldest-share split with. It does two separate things, and keeping them
// separate is the whole point:
//
// The folds measure the *procedure*. Walking a monthly origin across the
// corpus, each fold fits from the training window behind its origin and
// scores the unseen routes of the month after it. No fold ever sees a ride
// dated at or after the rides it is scored on, so the pooled error is honest
// in a way the old single split was not: that one drew its evaluation rides
// from across the whole corpus, so a profile could be badly wrong about the
// present and still record a flattering average.
//
// The shipped coefficients are then fit over the newest window, including the
// rides the final folds scored. Withholding them would deploy a profile
// deliberately blinded to the most recent months, which is the opposite of
// what a rider wants; the pooled fold error still describes it, because it
// was produced by this same procedure applied to less data.
func runRecalibration(
	rides []rideRow, samplesByRide map[string][]sampleRow, clusters map[string]int,
	coefficients *ridemodel.Coefficients, cfg *runConfig,
) (recalibration, error) {
	if len(rides) < minGroupRides {
		return recalibration{}, errors.New("too few hygienic rides to recalibrate from")
	}

	pooled := make(map[string][]float64)
	var modelNames []string
	var folds, scored int
	var firstOrigin, lastOrigin time.Time

	// Folds start one window back rather than at the first ride, so the
	// pooled error describes the same span of riding the shipped fit is
	// calibrated on. Reaching further back only averages the present
	// together with a fitness, a bike or a model the profile no longer
	// represents — the failure that let a live bias record itself as its
	// own opposite.
	newest := rides[len(rides)-1].Date
	firstFold := monthStart(newest.AddDate(0, -cfg.etaTrainingMonths, 0))
	for origin := firstFold; !origin.After(newest); origin = origin.AddDate(0, 1, 0) {
		train := trainingWindow(rides, origin, cfg.etaTrainingMonths)
		if len(train) < minGroupRides {
			continue
		}
		horizon := ridesBetween(rides, origin, origin.AddDate(0, rollingHorizonMonths, 0))
		evaluate := unseenRoutesIn(horizon, train, clusters)
		if len(evaluate) == 0 {
			continue
		}
		secondsPerKM, secondsPerAscentM, err := fitRouteCoefficients(train, samplesByRide)
		if err != nil {
			continue
		}
		active := *coefficients
		active.SecondsPerKM, active.SecondsPerAscentM = secondsPerKM, secondsPerAscentM

		models := benchmarkModels(&active)
		errorsByModel, foldScored := scoreRides(models, evaluate, samplesByRide)
		if foldScored == 0 {
			continue
		}
		if modelNames == nil {
			for _, model := range models {
				modelNames = append(modelNames, model.name)
			}
		}
		for name, errorPercentages := range errorsByModel {
			pooled[name] = append(pooled[name], errorPercentages...)
		}
		if folds == 0 {
			firstOrigin = origin
		}
		lastOrigin = origin
		folds++
		scored += foldScored
	}

	if folds == 0 {
		return recalibration{}, errors.New("no rolling-origin fold could be both fitted and scored")
	}

	// The shipped fit: the newest window, ending a day past the last ride so
	// that ride is inside it rather than just outside.
	train := trainingWindow(rides, newest.AddDate(0, 0, 1), cfg.etaTrainingMonths)
	secondsPerKM, secondsPerAscentM, err := fitRouteCoefficients(train, samplesByRide)
	if err != nil {
		return recalibration{}, err
	}

	return recalibration{
		errorsByModel: pooled, modelNames: modelNames,
		firstOrigin: firstOrigin, lastOrigin: lastOrigin, calibrationCutoff: newest,
		folds: folds, scored: scored, trainRides: len(train), windowMonths: cfg.etaTrainingMonths,
		secondsPerKM: secondsPerKM, secondsPerAscentM: secondsPerAscentM,
	}, nil
}

func printRecalibration(report *strings.Builder, eval *recalibration) {
	fmt.Fprintf(report, "  %d rolling-origin folds, origins %s .. %s, %d unseen-route scorings pooled\n",
		eval.folds, eval.firstOrigin.Format("2006-01"), eval.lastOrigin.Format("2006-01"), eval.scored)
	fmt.Fprintf(report, "  each fold fit on the %d months before its origin, scored on the %d month(s) after it\n",
		eval.windowMonths, rollingHorizonMonths)

	bestName, bestMAE := "", math.Inf(1)
	for _, name := range eval.modelNames {
		metrics := summarizeBenchmarkErrors(eval.errorsByModel[name])
		fmt.Fprintf(report, "    %-14s bias %+6.2f  MAE %6.2f  p90 %6.2f\n", name, metrics.bias, metrics.mae, metrics.p90)
		if metrics.mae < bestMAE {
			bestName, bestMAE = name, metrics.mae
		}
	}
	fmt.Fprintf(report, "  lowest MAE: %s (%.2f%%)\n", bestName, bestMAE)
	fmt.Fprintf(report, "  shipped fit: %d rides in the %d months through %s\n",
		eval.trainRides, eval.windowMonths, eval.calibrationCutoff.Format(time.DateOnly))
}
