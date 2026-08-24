package main

import (
	"errors"
	"fmt"
	"math"
	"math/rand"
	"sort"
	"strings"

	"github.com/nobbs/domestique/internal/surface"
)

const (
	defaultBenchmarkWarmupFraction = 0.6
	benchmarkFoldCount             = 4
	recentRideCount                = 20
	defaultRouteCellDegrees        = 0.002
	defaultRouteJaccardThreshold   = 0.7
	referenceDriveEfficiency       = 0.975
	observedDescentCapMPS          = 60.0 / 3.6
)

type benchmarkModel struct {
	predict func([]sampleRow) float64
	name    string
	detail  string
}

type benchmarkFold struct {
	train []rideRow
	test  []rideRow
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

func runETABenchmark(
	groups []rideGroup, rides []rideRow, samplesByRide map[string][]sampleRow, cfg *runConfig,
) (string, error) {
	var report strings.Builder
	var benchmarkErrors []error
	fmt.Fprintln(&report, "ETA benchmark: rolling-origin, route-disjoint first attempts")
	fmt.Fprintln(&report, "Errors are signed bias, mean absolute error and p90 absolute error, all as percentages of moving time.")
	fmt.Fprintf(&report, "Scoring starts after %.0f%% of rides; a repeat requires Jaccard overlap %.2f on a %.4f-degree coordinate grid.\n",
		cfg.etaWarmupFraction*100, cfg.etaRouteJaccard, cfg.etaRouteCellDegrees)

	for _, group := range groups {
		if group.Skipped {
			fmt.Fprintf(&report, "\n%s: skipped (%s)\n", displayGear(group.Gear), group.SkipReason)

			continue
		}

		groupRides := ridesInGroup(rides, group)
		cleanRides, exclusions := benchmarkEligibleRides(groupRides, samplesByRide)
		clusters, clusterCount, repeatedRides, largestCluster := clusterRoutes(
			cleanRides, samplesByRide, cfg.etaRouteCellDegrees, cfg.etaRouteJaccard,
		)
		folds := routeDisjointFolds(cleanRides, clusters, cfg.etaWarmupFraction)
		fmt.Fprintf(&report, "\n%s: %d/%d hygienic rides, %d route clusters, %d repeats, largest cluster %d\n",
			displayGear(group.Gear), len(cleanRides), len(groupRides), clusterCount, repeatedRides, largestCluster)
		if len(exclusions) > 0 {
			fmt.Fprintf(&report, "  excluded: %s\n", renderExclusions(exclusions))
		}

		aggregate := make(map[string][]float64)
		modelOrder := make([]string, 0, 15)
		scoredFolds := 0
		for index, fold := range folds {
			models, err := fitBenchmarkModels(fold.train, samplesByRide, cfg)
			if err != nil {
				fmt.Fprintf(&report, "  fold %d: skipped (%v)\n", index+1, err)

				continue
			}
			if len(modelOrder) == 0 {
				for _, model := range models {
					modelOrder = append(modelOrder, model.name)
				}
			}

			errorsByModel, scored := scoreBenchmarkFold(models, fold.test, samplesByRide)
			fmt.Fprintf(&report, "  fold %d: train %d, first-time routes %d, scored %d\n", index+1, len(fold.train), len(fold.test), scored)
			if scored == 0 {
				continue
			}
			scoredFolds++
			for _, model := range models {
				metrics := summarizeBenchmarkErrors(errorsByModel[model.name])
				fmt.Fprintf(&report, "    %-24s bias %+6.2f  MAE %6.2f  p90 %6.2f", model.name, metrics.bias, metrics.mae, metrics.p90)
				if model.detail != "" {
					fmt.Fprintf(&report, "  (%s)", model.detail)
				}
				fmt.Fprintln(&report)
				aggregate[model.name] = append(aggregate[model.name], errorsByModel[model.name]...)
			}
		}

		if scoredFolds == 0 {
			benchmarkErrors = append(benchmarkErrors, fmt.Errorf("%s: no benchmark fold could be scored", displayGear(group.Gear)))

			continue
		}
		fmt.Fprintln(&report, "  aggregate:")
		bestName, bestMAE := "", math.Inf(1)
		for _, name := range modelOrder {
			metrics := summarizeBenchmarkErrors(aggregate[name])
			fmt.Fprintf(&report, "    %-24s rides %3d  bias %+6.2f  MAE %6.2f  p90 %6.2f\n",
				name, metrics.rides, metrics.bias, metrics.mae, metrics.p90)
			if metrics.rides > 0 && metrics.mae < bestMAE {
				bestName, bestMAE = name, metrics.mae
			}
		}
		fmt.Fprintf(&report, "  lowest aggregate MAE: %s (%.2f%%)\n", bestName, bestMAE)
		if currentErrors := aggregate["current physics"]; len(currentErrors) > 0 {
			fmt.Fprintln(&report, "  paired improvement over current physics (positive is better):")
			for _, name := range modelOrder {
				if name == "current physics" {
					continue
				}
				improvement, low, high := pairedMAEImprovement(currentErrors, aggregate[name])
				fmt.Fprintf(&report, "    %-24s %+6.2f pp  95%% bootstrap [%+6.2f, %+6.2f]\n", name, improvement, low, high)
			}
		}
	}

	return report.String(), errors.Join(benchmarkErrors...)
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

func routeDisjointFolds(rides []rideRow, clusters map[string]int, warmupFraction float64) []benchmarkFold {
	if len(rides) < minGroupRides {
		return nil
	}
	warmup := int(float64(len(rides)) * warmupFraction)
	folds := make([]benchmarkFold, 0, benchmarkFoldCount)
	for index := range benchmarkFoldCount {
		start := warmup + (len(rides)-warmup)*index/benchmarkFoldCount
		end := warmup + (len(rides)-warmup)*(index+1)/benchmarkFoldCount
		if start == end {
			continue
		}
		seen := make(map[int]bool)
		for _, ride := range rides[:start] {
			seen[clusters[ride.RideID]] = true
		}
		selected := make(map[int]bool)
		test := make([]rideRow, 0, end-start)
		for _, ride := range rides[start:end] {
			cluster := clusters[ride.RideID]
			if seen[cluster] || selected[cluster] {
				continue
			}
			selected[cluster] = true
			test = append(test, ride)
		}
		folds = append(folds, benchmarkFold{train: rides[:start], test: test})
	}

	return folds
}

func fitBenchmarkModels(train []rideRow, samplesByRide map[string][]sampleRow, cfg *runConfig) ([]benchmarkModel, error) {
	if len(train) < minGroupRides {
		return nil, errors.New("too few training rides")
	}
	trainIDs := rideIDSet(train)
	flatSpeed, vam := baselineCoefficients(samplesByRide, trainIDs, cfg.climbThresholdPercent)
	if flatSpeed <= 0 {
		return nil, errors.New("trivial baseline could not be fitted")
	}
	baseline := benchmarkModel{
		name:   "trivial speed + VAM",
		detail: fmt.Sprintf("%.1f km/h, %.0f m/h", flatSpeed*3.6, vam),
		predict: func(samples []sampleRow) float64 {
			return baselineMovingSeconds(samples, flatSpeed, vam, cfg.climbThresholdPercent)
		},
	}

	physics, err := fitPhysicsBenchmarkModel(train, samplesByRide, cfg)
	if err != nil {
		return nil, err
	}
	linear, err := fitDistanceAscentModel(train, samplesByRide)
	if err != nil {
		return nil, err
	}
	physicsLinear := averageModels(physics, linear, "physics + linear average")
	referencePhysics := referencePhysicsBenchmarkModel(cfg.massKG, 0.40, 200)
	referenceLinear := averageModels(referencePhysics, linear, "reference + linear avg")
	midDragPhysics := referencePhysicsBenchmarkModel(cfg.massKG, 0.45, 200)
	midDragLinear := averageModels(midDragPhysics, linear, "mid-drag + linear avg")
	midDrag180Physics := referencePhysicsBenchmarkModel(cfg.massKG, 0.45, 180)
	midDrag180Linear := averageModels(midDrag180Physics, linear, "mid-drag 180 W + linear")
	highDragPhysics := referencePhysicsBenchmarkModel(cfg.massKG, 0.50, 200)
	highDragLinear := averageModels(highDragPhysics, linear, "high-drag + linear avg")

	return []benchmarkModel{
		baseline,
		physics,
		withRecentScale(physics, train, samplesByRide),
		linear,
		withRecentScale(linear, train, samplesByRide),
		physicsLinear,
		withRecentScale(physicsLinear, train, samplesByRide),
		referencePhysics,
		referenceLinear,
		midDragPhysics,
		midDragLinear,
		midDrag180Physics,
		midDrag180Linear,
		highDragPhysics,
		highDragLinear,
	}, nil
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

func referencePhysicsBenchmarkModel(massKG, cda, powerWatts float64) benchmarkModel {
	result := &fitResult{
		MassKG: massKG, CrrOverall: 0.012, CdA: cda,
		PowerWatts: powerWatts, MeanAirDensity: standardAirDensity,
		CrrBySurface: map[surface.Kind]float64{
			surface.KindAsphalt: 0.012, surface.KindPaving: 0.014, surface.KindCompacted: 0.015,
			surface.KindGravel: 0.018, surface.KindGround: 0.025,
		},
	}
	config := coefficientsConfig{
		DriveEfficiency: referenceDriveEfficiency, AirDensityKGPerM3: standardAirDensity,
		DescentCutoffPercent: -1, DescentCapMetresPerSecond: observedDescentCapMPS,
	}

	return benchmarkModel{
		name: fmt.Sprintf("reference %.2f / %.0f W", cda, powerWatts),
		detail: fmt.Sprintf(
			"Crr 0.012 asphalt, CdA %.2f, %.0f W, coast <= -1%%, 60 km/h cap",
			cda, powerWatts,
		),
		predict: func(samples []sampleRow) float64 {
			return predictedMovingSeconds(samples, result, config, referenceDriveEfficiency, powerWatts)
		},
	}
}

func fitDistanceAscentModel(train []rideRow, samplesByRide map[string][]sampleRow) (benchmarkModel, error) {
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
	secondsPerKM, secondsPerAscentM, _ := irlsFit(observations)
	if !finitePositive(secondsPerKM) || !finitePositive(secondsPerAscentM) {
		return benchmarkModel{}, fmt.Errorf("distance + ascent fit is invalid (%.2f s/km, %.3f s/m)", secondsPerKM, secondsPerAscentM)
	}

	return benchmarkModel{
		name:   "distance + ascent",
		detail: fmt.Sprintf("%.1f s/km + %.2f s/m", secondsPerKM, secondsPerAscentM),
		predict: func(samples []sampleRow) float64 {
			distanceKM, ascentM := distanceAndAscent(samples)

			return secondsPerKM*distanceKM + secondsPerAscentM*ascentM
		},
	}, nil
}

func distanceAndAscent(samples []sampleRow) (distanceKM, ascentM float64) {
	stage, _, ok := normalizedRideStage(samples)
	if !ok {
		return 0, 0
	}

	return stage.DistanceMetres() / 1000, stage.ElevationGainMetres()
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)

	return percentileOf(sorted, 0.5)
}

func withRecentScale(base benchmarkModel, train []rideRow, samplesByRide map[string][]sampleRow) benchmarkModel {
	start := max(0, len(train)-recentRideCount)
	ratios := make([]float64, 0, len(train)-start)
	for _, ride := range train[start:] {
		predicted := base.predict(samplesByRide[ride.RideID])
		if finitePositive(predicted) && finitePositive(ride.MovingSeconds) {
			ratios = append(ratios, ride.MovingSeconds/predicted)
		}
	}
	scale := median(ratios)
	if !finitePositive(scale) {
		scale = 1
	}

	return benchmarkModel{
		name:   base.name + " + recent",
		detail: fmt.Sprintf("x %.3f from %d rides", scale, len(ratios)),
		predict: func(samples []sampleRow) float64 {
			return base.predict(samples) * scale
		},
	}
}

func scoreBenchmarkFold(
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
