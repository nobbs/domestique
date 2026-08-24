package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/surface"
)

// The physics half of the accepted hybrid model (#213's "Decisions already
// made") is fixed rather than fitted: independently defensible constants, not
// values the corpus is trusted to identify. Only the two route-level
// coefficients — seconds per kilometre and seconds per ascent metre — are
// calibrated, and only once, before the evaluation cutoff.
const (
	defaultBenchmarkWarmupFraction = 0.6
	defaultRouteCellDegrees        = 0.002
	defaultRouteJaccardThreshold   = 0.7
	referenceDriveEfficiency       = 0.975
	referenceCdA                   = 0.45
	referencePowerWatts            = 180.0
	referenceScalarCrr             = 0.012
	referenceDescentCutoffPercent  = -1.0
	observedDescentCapMPS          = 60.0 / 3.6
	hybridModelVersion             = "hybrid-v1"
)

// referenceCrrBySurface is the per-surface rolling resistance table the
// hybrid candidate compares against its own scalar assumption. #239 keeps it
// only if it materially improves the frozen candidate on route-disjoint
// evaluation rides; otherwise the runtime rework drops ETA's surface
// dependency entirely.
func referenceCrrBySurface() map[surface.Kind]float64 {
	return map[surface.Kind]float64{
		surface.KindAsphalt: referenceScalarCrr, surface.KindPaving: 0.014, surface.KindCompacted: 0.015,
		surface.KindGravel: 0.018, surface.KindGround: 0.025,
	}
}

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

// splitEvaluation is one calibrate-once, score-once pass: the models it fit
// and the per-model prediction errors scored against the route-disjoint
// evaluation rides. runETABenchmark builds one for the run's own configured
// parameters and several more, varying one parameter at a time, for the
// robustness section.
type splitEvaluation struct {
	calibrationCutoff time.Time
	errorsByModel     map[string][]float64
	physicsNote       string
	models            []benchmarkModel
	clusterCount      int
	repeatedRides     int
	largestCluster    int
	calibrateCount    int
	evaluateCount     int
	evaluateScored    int
	secondsPerKM      float64
	secondsPerAscentM float64
}

func runETABenchmark(
	groups []rideGroup, rides []rideRow, samplesByRide map[string][]sampleRow, cfg *runConfig,
) (string, error) {
	var report strings.Builder
	var benchmarkErrors []error
	fmt.Fprintln(&report, "ETA benchmark: one frozen calibration, scored once on route-disjoint first attempts")
	fmt.Fprintln(&report, "Errors are signed bias, mean absolute error and p90 absolute error, all as percentages of moving time.")
	fmt.Fprintf(&report, "Calibration is the oldest %.0f%% of rides; a repeat requires Jaccard overlap %.2f on a %.4f-degree coordinate grid.\n",
		cfg.etaWarmupFraction*100, cfg.etaRouteJaccard, cfg.etaRouteCellDegrees)

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

		eval, err := evaluateSplit(cleanRides, samplesByRide, cfg, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard, cfg.etaWarmupFraction)
		if err != nil {
			fmt.Fprintf(&report, "  skipped (%v)\n", err)
			benchmarkErrors = append(benchmarkErrors, fmt.Errorf("%s: %w", displayGear(group.Gear), err))

			continue
		}

		printSplitSummary(&report, &eval)
		printModelMetrics(&report, &eval)
		printCandidateComparison(&report, &eval)
		printSurfaceComparison(&report, &eval, cfg.osmIndexPath != "")
		printRobustness(&report, cleanRides, samplesByRide, cfg)
		printCopyReadyProfile(&report, cfg, &eval)
	}

	return report.String(), errors.Join(benchmarkErrors...)
}

// evaluateSplit runs the whole pipeline for one set of route-matching and
// warm-up parameters: cluster routes, split chronologically into calibration
// and evaluation rides, fit every model once on calibration alone, and score
// them once against the route-disjoint evaluation rides. Calling it again
// with different parameters — what the robustness section does — never
// reuses a fit from another parameterisation, so a robustness variation is as
// frozen as the primary run.
func evaluateSplit(
	cleanRides []rideRow, samplesByRide map[string][]sampleRow, cfg *runConfig,
	cellDegrees, jaccard, warmupFraction float64,
) (splitEvaluation, error) {
	clusters, clusterCount, repeatedRides, largestCluster := clusterRoutes(cleanRides, samplesByRide, cellDegrees, jaccard)
	calibrate, evaluate := routeDisjointSplit(cleanRides, clusters, warmupFraction)
	if len(calibrate) < minGroupRides {
		return splitEvaluation{}, errors.New("too few calibration rides")
	}
	if len(evaluate) == 0 {
		return splitEvaluation{}, errors.New("no route-disjoint evaluation rides")
	}

	models, secondsPerKM, secondsPerAscentM, physicsNote, err := fitBenchmarkModels(calibrate, samplesByRide, cfg)
	if err != nil {
		return splitEvaluation{}, err
	}

	errorsByModel, scored := scoreRides(models, evaluate, samplesByRide)
	if scored == 0 {
		return splitEvaluation{}, errors.New("no evaluation ride could be scored")
	}

	return splitEvaluation{
		clusterCount: clusterCount, repeatedRides: repeatedRides, largestCluster: largestCluster,
		calibrateCount: len(calibrate), evaluateCount: len(evaluate), evaluateScored: scored,
		calibrationCutoff: calibrate[len(calibrate)-1].Date,
		models:            models, errorsByModel: errorsByModel, physicsNote: physicsNote,
		secondsPerKM: secondsPerKM, secondsPerAscentM: secondsPerAscentM,
	}, nil
}

func printSplitSummary(report *strings.Builder, eval *splitEvaluation) {
	fmt.Fprintf(report, "  %d route clusters, %d repeats, largest cluster %d\n",
		eval.clusterCount, eval.repeatedRides, eval.largestCluster)
	fmt.Fprintf(report, "  calibration: %d rides through %s\n", eval.calibrateCount, eval.calibrationCutoff.Format(time.DateOnly))
	fmt.Fprintf(report, "  evaluation: %d/%d route-disjoint first attempts scored\n", eval.evaluateScored, eval.evaluateCount)
	if eval.physicsNote != "" {
		fmt.Fprintf(report, "  current physics: unavailable (%s)\n", eval.physicsNote)
	}
}

func printModelMetrics(report *strings.Builder, eval *splitEvaluation) {
	bestName, bestMAE := "", math.Inf(1)
	for _, model := range eval.models {
		metrics := summarizeBenchmarkErrors(eval.errorsByModel[model.name])
		fmt.Fprintf(report, "    %-24s bias %+6.2f  MAE %6.2f  p90 %6.2f", model.name, metrics.bias, metrics.mae, metrics.p90)
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

// printCandidateComparison reports the frozen hybrid candidate against both
// baselines the acceptance criteria name: the current fitted physics model
// (when this corpus could fit one) and the route-only baseline it averages
// with. A candidate that is not materially better than the fitted baseline,
// or not competitive with the route-only one, is a reason to stop rather than
// tune against these same evaluation routes — this section is what that
// decision reads off.
func printCandidateComparison(report *strings.Builder, eval *splitEvaluation) {
	candidate := eval.errorsByModel["hybrid (scalar Crr)"]
	if len(candidate) == 0 {
		return
	}
	fmt.Fprintln(report, "  hybrid candidate vs baselines (positive is the candidate doing better):")
	if current := eval.errorsByModel["current physics"]; len(current) > 0 {
		improvement, low, high := pairedMAEImprovement(current, candidate)
		fmt.Fprintf(report, "    vs current physics       %+6.2f pp  95%% bootstrap [%+6.2f, %+6.2f]\n", improvement, low, high)
	}
	if routeOnly := eval.errorsByModel["route-only"]; len(routeOnly) > 0 {
		improvement, low, high := pairedMAEImprovement(routeOnly, candidate)
		fmt.Fprintf(report, "    vs route-only            %+6.2f pp  95%% bootstrap [%+6.2f, %+6.2f]\n", improvement, low, high)
	}
}

// printSurfaceComparison records the surface-table decision from this run's
// own scored errors, per #239's acceptance criterion that the decision come
// from an OSM-labelled run rather than being assumed. Without -osm-index every
// sample reads as surface.KindUnknown, which Coefficients.crr already maps to
// asphalt — the same value the scalar variant uses — so the two candidates
// would score identically and the comparison is flagged as uninformative
// rather than silently printed as a real result.
func printSurfaceComparison(report *strings.Builder, eval *splitEvaluation, osmIndexProvided bool) {
	scalar, surfaceTable := eval.errorsByModel["hybrid (scalar Crr)"], eval.errorsByModel["hybrid (surface Crr)"]
	if len(scalar) == 0 || len(surfaceTable) == 0 {
		return
	}
	fmt.Fprintln(report, "  surface table vs scalar Crr (positive is the surface table doing better):")
	if !osmIndexProvided {
		fmt.Fprintln(report, "    no -osm-index supplied; every sample is unclassified and reads as asphalt either way — not informative")

		return
	}
	improvement, low, high := pairedMAEImprovement(scalar, surfaceTable)
	fmt.Fprintf(report, "    %+6.2f pp  95%% bootstrap [%+6.2f, %+6.2f]\n", improvement, low, high)
}

// printRobustness re-runs the whole calibrate-once/score-once pipeline at one
// alternate value for each of the three parameters the acceptance criteria
// name — the route-matching grid, the route-overlap threshold, and the
// warm-up fraction — and reports how the hybrid candidate's MAE moves. Each
// alternate run calibrates fresh from its own split; nothing here reuses the
// primary run's fit.
func printRobustness(
	report *strings.Builder, cleanRides []rideRow, samplesByRide map[string][]sampleRow, cfg *runConfig,
) {
	fmt.Fprintln(report, "  robustness (hybrid candidate MAE, scalar Crr):")
	variations := []struct {
		label                        string
		cellDegrees, jaccard, warmup float64
	}{
		{"default", cfg.etaRouteCellDegrees, cfg.etaRouteJaccard, cfg.etaWarmupFraction},
		{"coarser route grid", cfg.etaRouteCellDegrees * 2, cfg.etaRouteJaccard, cfg.etaWarmupFraction},
		{"looser route overlap", cfg.etaRouteCellDegrees, cfg.etaRouteJaccard - 0.1, cfg.etaWarmupFraction},
		{"shorter warm-up", cfg.etaRouteCellDegrees, cfg.etaRouteJaccard, cfg.etaWarmupFraction - 0.1},
	}
	for _, variation := range variations {
		eval, err := evaluateSplit(cleanRides, samplesByRide, cfg, variation.cellDegrees, variation.jaccard, variation.warmup)
		if err != nil {
			fmt.Fprintf(report, "    %-22s unavailable (%v)\n", variation.label, err)

			continue
		}
		metrics := summarizeBenchmarkErrors(eval.errorsByModel["hybrid (scalar Crr)"])
		fmt.Fprintf(report, "    %-22s MAE %6.2f (%d rides)\n", variation.label, metrics.mae, metrics.rides)
	}
}

// printCopyReadyProfile prints the values #240 needs to carry the frozen
// candidate into the runtime coefficient contract: the calibrated route
// coefficients, the fixed physics constants they were averaged against, the
// calibration cutoff, and the validation metrics that justify the choice.
// This is a report block, not a file writer — the production TOML schema is
// #240's decision, and this is what the operator pastes into it.
func printCopyReadyProfile(report *strings.Builder, cfg *runConfig, eval *splitEvaluation) {
	candidate := eval.errorsByModel["hybrid (scalar Crr)"]
	if len(candidate) == 0 {
		return
	}
	metrics := summarizeBenchmarkErrors(candidate)
	fmt.Fprintf(report, "  copy-ready profile (%s):\n", hybridModelVersion)
	fmt.Fprintf(report, "    calibration_cutoff = %s\n", eval.calibrationCutoff.Format(time.DateOnly))
	fmt.Fprintf(report, "    seconds_per_km = %.4f\n", eval.secondsPerKM)
	fmt.Fprintf(report, "    seconds_per_ascent_m = %.4f\n", eval.secondsPerAscentM)
	fmt.Fprintf(report, "    mass_kg = %.1f\n", cfg.massKG)
	fmt.Fprintf(report, "    cda_m2 = %.2f\n", referenceCdA)
	fmt.Fprintf(report, "    power_watts = %.0f\n", referencePowerWatts)
	fmt.Fprintf(report, "    drive_efficiency = %.3f\n", referenceDriveEfficiency)
	fmt.Fprintf(report, "    air_density_kg_per_m3 = %.3f\n", standardAirDensity)
	fmt.Fprintf(report, "    descent_cutoff_percent = %.1f\n", referenceDescentCutoffPercent)
	fmt.Fprintf(report, "    descent_cap_metres_per_second = %.2f\n", observedDescentCapMPS)
	fmt.Fprintf(report, "    crr = %.3f (scalar; use this unless the surface-table comparison above favours the table below)\n", referenceScalarCrr)
	fmt.Fprintln(report, "    crr_by_surface (per-surface alternative; paste this instead if the comparison favours it):")
	surfaceCrr := referenceCrrBySurface()
	for _, kind := range []surface.Kind{surface.KindAsphalt, surface.KindPaving, surface.KindCompacted, surface.KindGravel, surface.KindGround} {
		fmt.Fprintf(report, "      %s = %.3f\n", kind, surfaceCrr[kind])
	}
	fmt.Fprintf(report, "    validation: rides %d, bias %+.2f%%, MAE %.2f%%, p90 %.2f%%\n", metrics.rides, metrics.bias, metrics.mae, metrics.p90)
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

// routeDisjointSplit orders rides chronologically and draws one cutoff at
// warmupFraction: everything before it calibrates, and everything after it is
// scored — but only a ride whose route cluster never appeared in calibration
// and has not already been selected, so evaluation sees each unseen route
// exactly once. #239 forbids refitting per fold or per test route; a single
// cutoff is what makes that true by construction rather than by convention.
func routeDisjointSplit(rides []rideRow, clusters map[string]int, warmupFraction float64) (calibrate, evaluate []rideRow) {
	if len(rides) < minGroupRides {
		return nil, nil
	}
	warmup := int(float64(len(rides)) * warmupFraction)
	calibrate = rides[:warmup]

	seen := make(map[int]bool, warmup)
	for _, ride := range calibrate {
		seen[clusters[ride.RideID]] = true
	}
	selected := make(map[int]bool)
	for _, ride := range rides[warmup:] {
		cluster := clusters[ride.RideID]
		if seen[cluster] || selected[cluster] {
			continue
		}
		selected[cluster] = true
		evaluate = append(evaluate, ride)
	}

	return calibrate, evaluate
}

// fitBenchmarkModels fits every model once against the calibration rides: the
// route-only linear model (always attempted — it is half of the candidate),
// the frozen hybrid candidate in its scalar- and surface-Crr forms, and the
// current fitted physics model as a comparison baseline only. The physics fit
// is exactly the identification problem #213 exists to route around, so its
// failure does not fail the whole run — the candidate does not depend on it —
// it is reported as an unavailable baseline instead.
func fitBenchmarkModels(
	train []rideRow, samplesByRide map[string][]sampleRow, cfg *runConfig,
) (models []benchmarkModel, secondsPerKM, secondsPerAscentM float64, physicsNote string, err error) {
	linear, secondsPerKM, secondsPerAscentM, err := fitDistanceAscentModel(train, samplesByRide)
	if err != nil {
		return nil, 0, 0, "", err
	}

	hybridScalar := averageModels(fixedPhysicsModel(cfg.massKG, nil), linear, "hybrid (scalar Crr)")
	hybridSurface := averageModels(fixedPhysicsModel(cfg.massKG, referenceCrrBySurface()), linear, "hybrid (surface Crr)")
	models = []benchmarkModel{linear, hybridScalar, hybridSurface}

	if physics, physicsErr := fitPhysicsBenchmarkModel(train, samplesByRide, cfg); physicsErr == nil {
		models = append([]benchmarkModel{physics}, models...)
	} else {
		physicsNote = physicsErr.Error()
	}

	return models, secondsPerKM, secondsPerAscentM, physicsNote, nil
}

func averageModels(left, right benchmarkModel, name string) benchmarkModel {
	return benchmarkModel{
		name:   name,
		detail: "equal weights",
		predict: func(samples []sampleRow) float64 {
			return (left.predict(samples) + right.predict(samples)) / 2
		},
	}
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

func fitPhysicsBenchmarkModel(train []rideRow, samplesByRide map[string][]sampleRow, cfg *runConfig) (benchmarkModel, error) {
	var windows []coastingWindow
	var climbs []climbSample
	for _, ride := range train {
		windows = append(windows, coastingWindowsFor(samplesByRide[ride.RideID], &coastingFilterCounts{}, cfg.massKG)...)
		climbs = append(climbs, climbWindowsFor(samplesByRide[ride.RideID], cfg.climbThresholdPercent)...)
	}
	crr, cda, condition := irlsFit(observationsFor(windows, cfg.massKG))
	if !finitePositive(crr) || crr > plausibleMaxCrr || !finitePositive(cda) || condition > maxAcceptableConditionRatio {
		return benchmarkModel{}, fmt.Errorf("current physics fit is invalid (Crr %.5f, CdA %.3f, condition %.0f)", crr, cda, condition)
	}
	crrBySurface := crrPerSurface(windows, cfg.massKG, cda)
	power := sustainedPowerWatts(climbs, crrBySurface, crr, cda, cfg.massKG, cfg.driveEfficiency)
	if !finitePositive(power) {
		return benchmarkModel{}, errors.New("current physics fit has no valid climbing power")
	}
	result := &fitResult{
		MassKG: cfg.massKG, CrrOverall: crr, CrrBySurface: crrBySurface, CdA: cda,
		PowerWatts: power, MeanAirDensity: meanAirDensity(windows),
	}
	config := coefficientsConfig{
		DriveEfficiency: cfg.driveEfficiency, AirDensityKGPerM3: result.MeanAirDensity,
		DescentCutoffPercent: cfg.descentCutoffPercent, DescentCapMetresPerSecond: cfg.descentCapMPS,
	}

	return benchmarkModel{
		name:   "current physics",
		detail: fmt.Sprintf("Crr %.5f, CdA %.3f, %.0f W", crr, cda, power),
		predict: func(samples []sampleRow) float64 {
			return predictedMovingSeconds(samples, result, config, cfg.driveEfficiency, power)
		},
	}, nil
}

// fixedPhysicsModel is the settled physics half of the hybrid candidate: mass
// is the only input this fitter still takes from the operator, everything
// else is the constant #213 accepted. crrBySurface is nil for the scalar
// variant (fullCrrBySurface then falls every class back to CrrOverall) and
// the full per-surface table for the other.
func fixedPhysicsModel(massKG float64, crrBySurface map[surface.Kind]float64) benchmarkModel {
	result := &fitResult{
		MassKG: massKG, CrrOverall: referenceScalarCrr, CdA: referenceCdA,
		PowerWatts: referencePowerWatts, MeanAirDensity: standardAirDensity, CrrBySurface: crrBySurface,
	}
	config := coefficientsConfig{
		DriveEfficiency: referenceDriveEfficiency, AirDensityKGPerM3: standardAirDensity,
		DescentCutoffPercent: referenceDescentCutoffPercent, DescentCapMetresPerSecond: observedDescentCapMPS,
	}

	return benchmarkModel{
		name: "fixed physics",
		detail: fmt.Sprintf(
			"CdA %.2f, %.0f W, coast <= %.0f%%, %.0f km/h cap", referenceCdA, referencePowerWatts,
			referenceDescentCutoffPercent, observedDescentCapMPS*3.6,
		),
		predict: func(samples []sampleRow) float64 {
			return predictedMovingSeconds(samples, result, config, referenceDriveEfficiency, referencePowerWatts)
		},
	}
}

func fitDistanceAscentModel(
	train []rideRow, samplesByRide map[string][]sampleRow,
) (model benchmarkModel, secondsPerKM, secondsPerAscentM float64, err error) {
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
		return benchmarkModel{}, 0, 0, fmt.Errorf("route-only fit is invalid (%.2f s/km, %.3f s/m)", secondsPerKM, secondsPerAscentM)
	}

	return benchmarkModel{
		name:   "route-only",
		detail: fmt.Sprintf("%.1f s/km + %.2f s/m", secondsPerKM, secondsPerAscentM),
		predict: func(samples []sampleRow) float64 {
			distanceKM, ascentM := distanceAndAscent(samples)

			return secondsPerKM*distanceKM + secondsPerAscentM*ascentM
		},
	}, secondsPerKM, secondsPerAscentM, nil
}

func distanceAndAscent(samples []sampleRow) (distanceKM, ascentM float64) {
	stage, _, ok := normalizedRideStage(samples)
	if !ok {
		return 0, 0
	}

	return stage.DistanceMetres() / 1000, stage.ElevationGainMetres()
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
