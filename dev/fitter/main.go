package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nobbs/domestique/internal/osmindex"
)

func main() {
	corpusDir := flag.String("corpus", "", "directory holding samples.csv, rides.csv and indoor.csv from dev/ridemodel")
	outDir := flag.String("out", ".local/fitter", "directory to write coefficients-<gear>.toml files to")
	massKG := flag.Float64("mass-kg", 0, "total riding mass in kilograms — weighed, not estimated")
	tyreCrrBench := flag.Float64("tyre-crr-bench", 0, "bench-measured Crr for the configured tyre; 0 skips the tyre-relative plausibility check")
	tyreCrrToleranceLow := flag.Float64("tyre-crr-tolerance-low", 1.0, "fitted Crr must be at least this multiple of -tyre-crr-bench")
	tyreCrrToleranceHigh := flag.Float64("tyre-crr-tolerance-high", 1.5, "fitted Crr must be at most this multiple of -tyre-crr-bench")
	driveEfficiency := flag.Float64("drive-efficiency", 0.975, "drivetrain mechanical efficiency, 0-1")
	descentCutoffPercent := flag.Float64("descent-cutoff-percent", -1.0, "grade at or below which a ride is assumed to coast rather than pedal")
	descentCapMPS := flag.Float64("descent-cap-mps", 22.0, "speed a coasted descent is assumed to reach")
	climbThresholdPercent := flag.Float64("climb-threshold-percent", defaultClimbThresholdPercent, "grade above which a window counts as sustained climbing")
	osmIndexPath := flag.String("osm-index", "", "path to an osmindex database; empty skips surface labelling and per-surface Crr")
	flag.Parse()

	if err := run(&runConfig{
		corpusDir: *corpusDir, outDir: *outDir, massKG: *massKG,
		tyreCrrBench: *tyreCrrBench, tyreCrrToleranceLow: *tyreCrrToleranceLow, tyreCrrToleranceHigh: *tyreCrrToleranceHigh,
		driveEfficiency: *driveEfficiency, descentCutoffPercent: *descentCutoffPercent, descentCapMPS: *descentCapMPS,
		climbThresholdPercent: *climbThresholdPercent, osmIndexPath: *osmIndexPath,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "fitter: %v\n", err)
		os.Exit(1)
	}
}

type runConfig struct {
	corpusDir             string
	outDir                string
	osmIndexPath          string
	massKG                float64
	tyreCrrBench          float64
	tyreCrrToleranceLow   float64
	tyreCrrToleranceHigh  float64
	driveEfficiency       float64
	descentCutoffPercent  float64
	descentCapMPS         float64
	climbThresholdPercent float64
}

// validate fails fast on a flag combination that would otherwise reach a
// division, a solver, or the plausibility gate in a state it cannot give a
// sensible answer from — a zero drive-efficiency dividing by zero in
// climbPowerWatts, a non-positive descent cap making every capped descent
// prediction nonsense, or a tyre tolerance band whose low bound exceeds its
// high one silently disabling the plausibility gate it is meant to run.
func (cfg *runConfig) validate() error {
	if cfg.corpusDir == "" {
		return errors.New("-corpus is required")
	}
	if cfg.massKG <= 0 {
		return errors.New("-mass-kg is required and must be positive: an input this fit is run against, not something it estimates")
	}
	if cfg.driveEfficiency <= 0 || cfg.driveEfficiency > 1 {
		return errors.New("-drive-efficiency must be greater than 0 and at most 1")
	}
	// Matches internal/ridemodel/coefficients.go's own validation of the
	// same field (PR #232): a positive cutoff would make predictedSpeed
	// treat flat or uphill grades as a descent to coast, returning 0 for
	// most of a ride's samples.
	if cfg.descentCutoffPercent > 0 {
		return errors.New("-descent-cutoff-percent must not be positive")
	}
	if cfg.descentCapMPS <= 0 {
		return errors.New("-descent-cap-mps must be positive")
	}
	if cfg.climbThresholdPercent <= 0 {
		return errors.New("-climb-threshold-percent must be positive")
	}
	if cfg.tyreCrrBench > 0 && cfg.tyreCrrToleranceLow > cfg.tyreCrrToleranceHigh {
		return errors.New("-tyre-crr-tolerance-low must not exceed -tyre-crr-tolerance-high")
	}

	return nil
}

func run(cfg *runConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	samples, err := readSamplesCSV(filepath.Join(cfg.corpusDir, "samples.csv"))
	if err != nil {
		return err
	}
	rides, err := readRidesCSV(filepath.Join(cfg.corpusDir, "rides.csv"))
	if err != nil {
		return err
	}
	indoor, err := readIndoorCSV(filepath.Join(cfg.corpusDir, "indoor.csv"))
	if err != nil {
		return err
	}

	samplesByRide := make(map[string][]sampleRow)
	for i := range samples {
		samplesByRide[samples[i].RideID] = append(samplesByRide[samples[i].RideID], samples[i])
	}

	ctx := context.Background()
	if cfg.osmIndexPath != "" {
		index, openErr := osmindex.Open(ctx, cfg.osmIndexPath)
		if openErr != nil {
			return fmt.Errorf("opening osm index: %w", openErr)
		}
		defer closeIndex(index)
		attempted, failed := labelSurfaces(ctx, index, samplesByRide)
		if attempted > 0 && failed == attempted {
			return fmt.Errorf(
				"surface labelling: all %d ride lookups against -osm-index failed; check the index path and permissions",
				attempted,
			)
		}
		if failed > 0 {
			fmt.Fprintf(os.Stderr, "fitter: surface labelling: %d/%d ride lookups failed; those rides fall back to unknown surface\n", failed, attempted)
		}
	}

	ridesWithSamples := make([]rideRow, 0, len(rides))
	for _, r := range rides {
		if _, ok := samplesByRide[r.RideID]; ok {
			ridesWithSamples = append(ridesWithSamples, r)
		}
	}

	relations := hrPowerRelationByYear(indoor)

	if err := os.MkdirAll(cfg.outDir, 0o750); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}

	groups := groupRidesByGear(ridesWithSamples)
	if err := checkTOMLStemCollisions(groups); err != nil {
		return err
	}

	anyRejected := false
	var results []fitResult
	for _, group := range groups {
		result := fitGroup(group, ridesWithSamples, samplesByRide, relations, cfg)
		results = append(results, result)
		if result.RejectedCrrBounds || result.IllConditioned || result.NoClimbingData {
			anyRejected = true

			continue
		}
		if result.Skipped {
			continue
		}

		path := filepath.Join(cfg.outDir, fmt.Sprintf("coefficients-%s.toml", tomlFileStem(result.Group)))
		if writeErr := writeCoefficientsTOML(path, &result, coefficientsConfig{
			DriveEfficiency: cfg.driveEfficiency, AirDensityKGPerM3: result.MeanAirDensity,
			DescentCutoffPercent: cfg.descentCutoffPercent, DescentCapMetresPerSecond: cfg.descentCapMPS,
		}); writeErr != nil {
			return writeErr
		}
	}

	fmt.Print(renderReport(results))
	if anyRejected {
		return errors.New("one or more groups failed the plausibility check; see the report above")
	}

	return nil
}

// checkTOMLStemCollisions fails fast when two distinct gear names would
// normalize to the same output file — "Bike A" and "Bike-A" both become
// "Bike-A" — rather than letting the second group's write silently
// overwrite the first's. Only groups that will actually reach the write
// step are checked; a skipped group never claims a file name.
func checkTOMLStemCollisions(groups []rideGroup) error {
	gearByStem := make(map[string]string)
	for _, group := range groups {
		if group.Skipped {
			continue
		}
		stem := tomlFileStem(group.Gear)
		if existing, ok := gearByStem[stem]; ok && existing != group.Gear {
			return fmt.Errorf(
				"gear %q and %q both normalize to the output file name %q; rename one in the export to avoid overwriting the other's coefficients",
				existing, group.Gear, stem,
			)
		}
		gearByStem[stem] = group.Gear
	}

	return nil
}

// tomlFileStem turns a gear identifier into a filesystem-safe file name
// component. Strava's own gear names carry spaces and punctuation a path
// segment should not.
func tomlFileStem(gear string) string {
	if gear == untaggedGear {
		return "untagged"
	}

	stem := make([]rune, 0, len(gear))
	for _, r := range gear {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			stem = append(stem, r)
		default:
			stem = append(stem, '-')
		}
	}

	return string(stem)
}

func closeIndex(index *osmindex.Index) { _ = index.Close() } //nolint:errcheck // A read-only index has nothing to report on close.

// fitGroup runs stages A and B for one gear group's rides: a chronological
// train/held-out split, the coasting and climbing fits over the training
// rides only, the tyre-relative and conditioning plausibility checks, the
// indoor heart-rate cross-check, and held-out validation against the
// trivial baseline.
func fitGroup(
	group rideGroup, rides []rideRow, samplesByRide map[string][]sampleRow,
	relations map[int]hrPowerRelation, cfg *runConfig,
) fitResult {
	result := fitResult{Group: group.Gear, MassKG: cfg.massKG, ClimbThresholdPct: cfg.climbThresholdPercent, UntaggedAttributed: group.UntaggedAttributed}
	if group.Skipped {
		result.Skipped = true
		result.SkipReason = group.SkipReason

		return result
	}

	var groupRides []rideRow
	for _, r := range rides {
		if group.RideIDs[r.RideID] {
			groupRides = append(groupRides, r)
		}
	}
	train, heldOut := splitByDate(groupRides)
	trainRideIDs := rideIDSet(train)
	heldOutRideIDs := rideIDSet(heldOut)

	counts := &coastingFilterCounts{}
	var allWindows []coastingWindow
	var allClimbs []climbSample
	var climbHoursAboveThreshold float64
	for _, r := range train {
		samples := samplesByRide[r.RideID]
		allWindows = append(allWindows, coastingWindowsFor(samples, counts, cfg.massKG)...)
		allClimbs = append(allClimbs, climbWindowsFor(samples, cfg.climbThresholdPercent)...)
		climbHoursAboveThreshold += hoursAboveThreshold(samples, cfg.climbThresholdPercent)
	}

	result.SurvivingWindows = allWindows
	result.CorneringRejected = counts.Cornering
	result.PlausibilityReject = counts.Plausibility
	result.ClimbHoursAbove = climbHoursAboveThreshold
	result.MeanAirDensity = meanAirDensity(allWindows)

	observations := observationsFor(allWindows, cfg.massKG)
	crrOverall, cda, conditionRatio := irlsFit(observations)
	result.CrrOverall = crrOverall
	result.CdA = cda
	result.ConditionRatio = conditionRatio
	result.IllConditioned = conditionRatio > maxAcceptableConditionRatio

	if cfg.tyreCrrBench > 0 {
		lower, upper := cfg.tyreCrrBench*cfg.tyreCrrToleranceLow, cfg.tyreCrrBench*cfg.tyreCrrToleranceHigh
		result.TyrePlausible = crrOverall >= lower && crrOverall <= upper
	} else {
		result.TyrePlausible = crrOverall > 0 && crrOverall <= plausibleMaxCrr
	}
	result.RejectedCrrBounds = !result.TyrePlausible

	if result.IllConditioned || result.RejectedCrrBounds {
		return result
	}

	result.NoClimbingData = len(allClimbs) == 0
	if result.NoClimbingData {
		return result
	}

	result.CrrBySurface = crrPerSurface(allWindows, cfg.massKG, cda)
	result.PowerWatts = sustainedPowerWatts(allClimbs, result.CrrBySurface, crrOverall, cda, cfg.massKG, cfg.driveEfficiency)

	// The quarterly intercept is a diagnostic over the group's whole ridden
	// history, not the fit itself, so it is built over every ride — held-out
	// ones included — rather than truncating the very recent quarter a real
	// equipment change would show up in.
	rideDates := make(map[string]time.Time, len(groupRides))
	var allGroupWindows []coastingWindow
	for _, r := range groupRides {
		rideDates[r.RideID] = r.Date
		if trainRideIDs[r.RideID] {
			continue
		}
		allGroupWindows = append(allGroupWindows, coastingWindowsFor(samplesByRide[r.RideID], &coastingFilterCounts{}, cfg.massKG)...)
	}
	allGroupWindows = append(allGroupWindows, allWindows...)
	result.QuarterlyIntercept = medianByQuarter(quarterlyIntercepts(allGroupWindows, rideDates, cfg.massKG, cda))

	var checks []rideCrossCheck
	for _, r := range groupRides {
		if check, ok := crossCheckRide(samplesByRide[r.RideID], relations, result.CrrBySurface, crrOverall, cda, cfg.massKG, cfg.driveEfficiency); ok {
			checks = append(checks, check)
		}
	}
	result.CrossCheck = summarizeCrossCheck(checks, relations)

	flatSpeedMPS, vamMetresPerHour := baselineCoefficients(samplesByRide, trainRideIDs, cfg.climbThresholdPercent)
	actualMovingSeconds := make(map[string]float64, len(groupRides))
	for _, r := range groupRides {
		actualMovingSeconds[r.RideID] = r.MovingSeconds
	}
	result.HeldOut = validateHeldOut(heldOutRideIDs, samplesByRide, actualMovingSeconds, &result, coefficientsConfig{
		DriveEfficiency: cfg.driveEfficiency, DescentCutoffPercent: cfg.descentCutoffPercent, DescentCapMetresPerSecond: cfg.descentCapMPS,
	}, flatSpeedMPS, vamMetresPerHour, cfg.climbThresholdPercent)

	return result
}

func rideIDSet(rides []rideRow) map[string]bool {
	set := make(map[string]bool, len(rides))
	for _, r := range rides {
		set[r.RideID] = true
	}

	return set
}

func hoursAboveThreshold(samples []sampleRow, thresholdPercent float64) float64 {
	var seconds float64
	for i := range samples {
		if samples[i].Moving && samples[i].HasAltitude && samples[i].GradientPercent >= thresholdPercent {
			seconds += samples[i].DeltaSeconds
		}
	}

	return seconds / 3600
}
