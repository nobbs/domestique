package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/trainingload"
)

// statedBicycle is the bicycle the fit starts from: a drop-bar gravel bike
// ridden on the hoods, on 47 mm WTB Byway tyres, whose Crr is the drum
// figure Bicycle Rolling Resistance measured. Its Crr is what the fit holds
// fixed; its CdA is only the baseline candidate.
func statedBicycle() measure.Coefficients {
	return measure.Coefficients{DragArea: 0.40, RollingResistance: 0.008}
}

// minHeartRateCoverage is the share of the track's own elapsed span a strap
// must hold a reading for, within the track's span alone, before the ride's
// mean heart rate is trusted as its target: a duration, not a sample count,
// so two sensors sampling at different rates are judged the same way.
const minHeartRateCoverage = 0.8

// A meteredRide is one ride that carried a power meter: its blocks for the
// bridge, and the day it was ridden.
type meteredRide struct {
	at     time.Time
	blocks []MeasuredBlock
}

// An unmeteredRide is one road ride as the study scores it: the model's
// whole-ride pedalling block against the power the ride's mean heart rate
// names.
type unmeteredRide struct {
	whole         Ride
	meanHeartRate float64
	atUnix        int64
}

// heldSecondsByBlock is how long a series held a reading within each block,
// counted from start, using measure.MeanHeld over the readings that fall in
// each: a step no wider than measure.DefaultMaxGap counts, one wider does not.
func heldSecondsByBlock(readings []trainingload.Sample, start time.Time, block time.Duration) map[int]float64 {
	buckets := map[int][]trainingload.Sample{}
	for _, reading := range readings {
		key := int(reading.At.Sub(start) / block)
		buckets[key] = append(buckets[key], reading)
	}
	held := make(map[int]float64, len(buckets))
	for key, samples := range buckets {
		_, seconds := measure.MeanHeld(samples, measure.DefaultMaxGap)
		held[key] = seconds
	}

	return held
}

// blockMean averages a sensor series into blocks of the given length, counted
// from start. Returns the mean and how many readings it held, per block index.
func blockMean(readings []trainingload.Sample, start time.Time, block time.Duration) (means map[int]float64, counts map[int]int) {
	sums, counts := map[int]float64{}, map[int]int{}
	for _, reading := range readings {
		key := int(reading.At.Sub(start) / block)
		sums[key] += reading.Value
		counts[key]++
	}
	means = make(map[int]float64, len(sums))
	for key, sum := range sums {
		means[key] = sum / float64(counts[key])
	}

	return means, counts
}

// meteredBlocksOf cuts a metered ride into blocks: those holding at least
// half their own length in both readings, over the samples the rider was
// pedalling through -- a power meter reads nought while coasting, so that
// alone marks it, and a coasting-heavy trainer ride must not lower the
// bridge target against a model that never sees its zero-power samples.
// Only the power and the heart rate are read; a trainer ride's speed and
// distance are a simulation.
func meteredBlocksOf(power, heartRate []trainingload.Sample, block time.Duration) []MeasuredBlock {
	if len(power) == 0 || len(heartRate) == 0 {
		return nil
	}
	pedallingPower := make([]trainingload.Sample, 0, len(power))
	for _, reading := range power {
		if reading.Value > 0 {
			pedallingPower = append(pedallingPower, reading)
		}
	}
	if len(pedallingPower) == 0 {
		return nil
	}
	start, end := power[0].At, power[len(power)-1].At
	pedallingHeartRate := make([]trainingload.Sample, 0, len(heartRate))
	for _, reading := range heartRate {
		if !reading.At.Before(start) && !reading.At.After(end) {
			pedallingHeartRate = append(pedallingHeartRate, reading)
		}
	}
	powerMeans, _ := blockMean(pedallingPower, start, block)
	heartRateMeans, _ := blockMean(pedallingHeartRate, start, block)
	powerHeld := heldSecondsByBlock(pedallingPower, start, block)
	heartRateHeld := heldSecondsByBlock(pedallingHeartRate, start, block)

	keys := make([]int, 0, len(powerMeans))
	for key := range powerMeans {
		keys = append(keys, key)
	}
	sort.Ints(keys)

	halfBlockSeconds := block.Seconds() / 2
	blocks := make([]MeasuredBlock, 0, len(keys))
	for _, key := range keys {
		if powerHeld[key] < halfBlockSeconds || heartRateHeld[key] < halfBlockSeconds {
			continue
		}
		blocks = append(blocks, MeasuredBlock{
			HeartRateBPM: heartRateMeans[key], WattsMeasured: powerMeans[key],
		})
	}

	return blocks
}

// pedalling is whether a sample is one the rider was pedalling through: any
// sample not known to be a coast. A sample with no cadence reading is
// estimated by the model and counts.
func pedalling(sample *measure.Sample) bool {
	return !sample.HasCadence || sample.CadenceRPM > 0
}

// unmeteredBlocksOf is the whole-ride block the yardstick scores: every track
// sample the model can estimate across while the rider was pedalling.
func unmeteredBlocksOf(track []measure.Sample, massKG float64) Block {
	whole := Block{}
	estimates, ok := measure.EstimateSeries(track, massKG, statedBicycle())
	if !ok {
		return whole
	}
	for index := range track {
		if estimates[index].Known && pedalling(&track[index]) {
			whole.Indices = append(whole.Indices, index)
		}
	}

	return whole
}

// meanOf is the plain mean of a series of readings, false for none.
func meanOf(readings []trainingload.Sample) (mean float64, ok bool) {
	if len(readings) == 0 {
		return 0, false
	}
	sum := 0.0
	for _, reading := range readings {
		sum += reading.Value
	}

	return sum / float64(len(readings)), true
}

// A rideLevel is one metered ride's place on the rider's heart-rate-to-power
// relation: the mean heart rate and power over its blocks, and the day.
type rideLevel struct {
	at        time.Time
	heartRate float64
	watts     float64
}

// A bridge is the rider's heart-rate-to-power relation with its level free
// to move over time: one slope shared across every month, and a level read
// from the rides before a date. Pooling every block into one line flattens
// the slope to what fitness drift leaves of it; taking the slope within
// months keeps it effort's.
type bridge struct {
	levels      []rideLevel // sorted by date
	window      int
	wattsPerBPM float64
	residualRMS float64
	blocks      int
}

// fitBridge fits the shared slope by least squares over every block's
// deviation from its own calendar month's means, and keeps one level per
// metered ride for the window to read.
func fitBridge(rides []meteredRide, window int) (bridge, bool) {
	type sum struct {
		heartRate, watts float64
		n                int
	}
	months := map[string]*sum{}
	fitted := bridge{window: window}
	for index := range rides {
		ride := &rides[index]
		key := ride.at.UTC().Format("2006-01")
		if months[key] == nil {
			months[key] = &sum{}
		}
		level := rideLevel{at: ride.at}
		for _, block := range ride.blocks {
			months[key].heartRate += block.HeartRateBPM
			months[key].watts += block.WattsMeasured
			months[key].n++
			level.heartRate += block.HeartRateBPM / float64(len(ride.blocks))
			level.watts += block.WattsMeasured / float64(len(ride.blocks))
		}
		fitted.levels = append(fitted.levels, level)
		fitted.blocks += len(ride.blocks)
	}
	sort.Slice(fitted.levels, func(i, j int) bool { return fitted.levels[i].at.Before(fitted.levels[j].at) })

	var covariance, variance float64
	for index := range rides {
		ride := &rides[index]
		month := months[ride.at.UTC().Format("2006-01")]
		meanHeartRate, meanWatts := month.heartRate/float64(month.n), month.watts/float64(month.n)
		for _, block := range ride.blocks {
			dh, dw := block.HeartRateBPM-meanHeartRate, block.WattsMeasured-meanWatts
			covariance += dh * dw
			variance += dh * dh
		}
	}
	if len(fitted.levels) == 0 || variance == 0 {
		return bridge{}, false
	}
	fitted.wattsPerBPM = covariance / variance

	var sumSquares float64
	for index := range rides {
		ride := &rides[index]
		for _, block := range ride.blocks {
			residual := block.WattsMeasured - fitted.wattsAt(ride.at, block.HeartRateBPM)
			sumSquares += residual * residual
		}
	}
	fitted.residualRMS = math.Sqrt(sumSquares / float64(fitted.blocks))

	return fitted, true
}

// wattsAt is the power this rider's heart rate says they were producing on a
// date: the median level of the last window metered rides before it, or the
// first window rides where the date comes before them, moved along the
// shared slope, never the ride on that date itself. A median so one race or
// one unpaired strap moves nothing.
func (b bridge) wattsAt(at time.Time, heartRateBPM float64) float64 {
	// The window rides strictly before at, or the first window rides where
	// at comes before them; the ride on that date itself is never among them.
	before := sort.Search(len(b.levels), func(i int) bool { return !b.levels[i].at.Before(at) })
	start, limit := max(before-b.window, 0), before
	if before < b.window {
		// Too early for a window of its own: read the first window's rides,
		// less the one being scored where it is among them, never reaching
		// past the window into rides that came after at to make up the count.
		start, limit = 0, min(b.window, len(b.levels))
	}
	intercepts := make([]float64, 0, b.window)
	for index := start; index < limit && len(intercepts) < b.window; index++ {
		if b.levels[index].at.Equal(at) {
			continue
		}
		intercepts = append(intercepts, b.levels[index].watts-b.wattsPerBPM*b.levels[index].heartRate)
	}
	if len(intercepts) == 0 {
		return 0
	}

	return max(quantile(intercepts, 0.5)+b.wattsPerBPM*heartRateBPM, 0)
}

// A candidate is one way of choosing the coefficients, scored on rides its
// fit never saw.
type candidate struct {
	fit  func([]Ride) (measure.Coefficients, bool)
	name string
}

// candidates are the built-in road bicycle, the upright prior as it stands,
// the rider's own saved bicycle, and the drag area fitted at the prior's
// rolling resistance.
func candidates(profile measure.Coefficients) []candidate {
	fixed := func(coefficients measure.Coefficients) func([]Ride) (measure.Coefficients, bool) {
		return func([]Ride) (measure.Coefficients, bool) { return coefficients, true }
	}

	return []candidate{
		{name: "default", fit: fixed(measure.DefaultCoefficients())},
		{name: "prior", fit: fixed(statedBicycle())},
		{name: "profile", fit: fixed(profile)},
		{name: "cda", fit: func(rides []Ride) (measure.Coefficients, bool) {
			coefficients, _, ok := FitDragArea(statedBicycle().RollingResistance, rides)
			return coefficients, ok
		}},
	}
}

// A rideScore is one ride's result: what the model produced over its
// samples against what the rider's heart rate says.
type rideScore struct {
	modelled, target float64
}

func (s rideScore) errorPercent() float64 { return 100 * (s.modelled - s.target) / s.target }

// scorable is whether a ride can be judged in per cent at all: a target of
// nought names no scale to judge against.
func (s rideScore) scorable() bool { return s.target > 0 }

// A report is levelstudy's aggregate result. No ride identifier, date,
// position or altitude value is ever held here.
type report struct {
	rides        map[string][]rideScore
	fitted       map[string][]measure.Coefficients
	bridge       bridge
	shipped      measure.Coefficients
	shippedOK    bool
	rideCount    int
	meteredRides int
	skipped      int
}

func writeRideTable(b *strings.Builder, scores []rideScore) {
	percents := make([]float64, 0, len(scores))
	var sumSquares, sumSigned float64
	for _, score := range scores {
		percents = append(percents, score.errorPercent())
		residual := score.modelled - score.target
		sumSquares += residual * residual
		sumSigned += residual
	}
	absolute := make([]float64, len(percents))
	for index, held := range percents {
		absolute[index] = math.Abs(held)
	}
	n := float64(len(scores))
	fmt.Fprintf(b, "%6d %10.1f %10.1f %10.1f %10.1f %10.1f %10.1f\n", len(scores),
		quantile(absolute, 0.5), mean(percents), quantile(percents, 0.25), quantile(percents, 0.75),
		math.Sqrt(sumSquares/n), sumSigned/n)
}

func (r *report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "bridge: %.2f W/bpm shared slope, median level over the last %d metered rides, "+
		"residual RMS %.1f W over %d metered blocks\n", r.bridge.wattsPerBPM, r.bridge.window, r.bridge.residualRMS, r.bridge.blocks)
	fmt.Fprintf(&b, "corpus: %d unmetered rides, %d metered rides, %d skipped\n\n",
		r.rideCount, r.meteredRides, r.skipped)

	fmt.Fprintln(&b, "held-out whole-ride agreement with what the rider's heart rate says")
	fmt.Fprintf(&b, "  %-8s %6s %10s %10s %10s %10s %10s %10s\n",
		"candidate", "rides", "MdAE %", "bias %", "q1 %", "q3 %", "RMS W", "bias W")
	for _, name := range []string{"default", "prior", "profile", "cda"} {
		if scores := r.rides[name]; len(scores) > 0 {
			fmt.Fprintf(&b, "  %-8s ", name)
			writeRideTable(&b, scores)
		}
	}

	if fitted := r.fitted["cda"]; len(fitted) > 0 {
		drag := make([]float64, 0, len(fitted))
		for _, coefficients := range fitted {
			drag = append(drag, coefficients.DragArea)
		}
		fmt.Fprintf(&b, "\ndrag area at Crr %.3f, fold to fold: CdA %.3f ±%.3f\n",
			statedBicycle().RollingResistance, mean(drag), spread(drag))
		if r.shippedOK {
			fmt.Fprintf(&b, "fitted over the whole corpus: CdA %.3f, Crr %.3f\n",
				r.shipped.DragArea, r.shipped.RollingResistance)
		} else {
			fmt.Fprintln(&b, "fitted over the whole corpus: unavailable (the fit collapsed to its search bound)")
		}
	}

	return b.String()
}

func mean(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

// spread is the sample standard deviation, which is how a fold-to-fold fit
// says whether it found one answer or several.
func spread(values []float64) float64 {
	if len(values) < 2 {
		return 0
	}
	average, sumSquares := mean(values), 0.0
	for _, value := range values {
		sumSquares += (value - average) * (value - average)
	}

	return math.Sqrt(sumSquares / float64(len(values)-1))
}

// quantile is the nearest-rank quantile of values, which it sorts a copy of.
func quantile(values []float64, q float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := min(int(math.Ceil(q*float64(len(sorted))))-1, len(sorted)-1)

	return sorted[max(index, 0)]
}

// study reads every recorded ride, bridges the metered ones to the unmetered
// ones by heart rate, and scores each candidate on folds its own fit never
// saw. Folds are cut by ride: two rides share a metered ride's bridge level
// only through the fold that trained on it, so splitting a ride's own blocks
// across folds would let a fit see its own test set.
func study(
	ctx context.Context, store *sqlite.Store, minSamples int, block time.Duration, folds, window int, massFlagKG float64, target string,
) (*report, error) {
	rides, err := store.RecordedRides(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing recorded rides: %w", err)
	}
	if target != "" {
		filtered := rides[:0]
		for _, ride := range rides {
			if ride.TargetID == target {
				filtered = append(filtered, ride)
			}
		}
		rides = filtered
	}

	result := &report{
		rides:  map[string][]rideScore{},
		fitted: map[string][]measure.Coefficients{},
	}
	massCache := map[string]float64{}
	bicycleCache := map[string]measure.Coefficients{}
	maxHeartRateCache := map[string]float64{}
	profile := measure.DefaultCoefficients()
	if len(rides) > 0 {
		for _, ride := range rides {
			if ride.TargetID != rides[0].TargetID {
				return nil, errors.New("the database holds several targets: name one with -target")
			}
		}
		profile, err = bicycleForTarget(ctx, store, rides[0].TargetID, bicycleCache)
		if err != nil {
			return nil, err
		}
	}
	var metered []meteredRide
	var unmetered []unmeteredRide

	for _, ride := range rides {
		samples, samplesErr := store.ActivityRideSamples(ctx, ride.TargetID, ride.WorkoutID)
		if samplesErr != nil {
			return nil, fmt.Errorf("reading a ride's samples: %w", samplesErr)
		}
		maxHeartRate, maxHeartRateErr := maxHeartRateForTarget(ctx, store, ride.TargetID, maxHeartRateCache)
		if maxHeartRateErr != nil {
			return nil, maxHeartRateErr
		}
		// Capped at the rider's own maximum, the same as live derivation: a
		// strap spike above it is a sensor fault, not a rider.
		heartRate := measure.CapHeartRate(samples.HeartRate, maxHeartRate)

		if len(samples.Power) > 0 {
			blocks := meteredBlocksOf(samples.Power, heartRate, block)
			if len(blocks) == 0 {
				result.skipped++
				continue
			}
			result.meteredRides++
			metered = append(metered, meteredRide{at: samples.Power[0].At, blocks: blocks})

			continue
		}

		if len(samples.Track) < minSamples {
			result.skipped++
			continue
		}
		mass, massErr := massForTarget(ctx, store, ride.TargetID, massFlagKG, massCache)
		if massErr != nil {
			return nil, massErr
		}
		trackStart, trackEnd := samples.Track[0].At, samples.Track[len(samples.Track)-1].At
		elapsedSeconds := trackEnd.Sub(trackStart).Seconds()
		withinTrack := make([]trainingload.Sample, 0, len(heartRate))
		for _, reading := range heartRate {
			if !reading.At.Before(trackStart) && !reading.At.After(trackEnd) {
				withinTrack = append(withinTrack, reading)
			}
		}
		_, heldSeconds := measure.MeanHeld(withinTrack, measure.DefaultMaxGap)
		if elapsedSeconds <= 0 || heldSeconds < minHeartRateCoverage*elapsedSeconds {
			result.skipped++
			continue
		}
		whole := unmeteredBlocksOf(samples.Track, mass)
		if len(whole.Indices) == 0 {
			result.skipped++
			continue
		}
		meanHeartRate, meanOK := meanOf(withinTrack)
		if !meanOK {
			result.skipped++
			continue
		}
		unmetered = append(unmetered, unmeteredRide{
			atUnix:        samples.Track[0].At.Unix(),
			whole:         Ride{Samples: samples.Track, Blocks: []Block{whole}, TotalMassKG: mass},
			meanHeartRate: meanHeartRate,
		})
	}

	fitted, ok := fitBridge(metered, window)
	if !ok {
		return nil, fmt.Errorf("no bridge: %d metered rides name no slope", len(metered))
	}
	result.bridge = fitted

	for index := range unmetered {
		held := &unmetered[index]
		at := time.Unix(held.atUnix, 0).UTC()
		held.whole.Blocks[0].TargetWatts = fitted.wattsAt(at, held.meanHeartRate)
	}
	result.rideCount = len(unmetered)

	for fold := range folds {
		var train []Ride
		var test []*unmeteredRide
		for index := range unmetered {
			if index%folds == fold {
				test = append(test, &unmetered[index])
			} else {
				train = append(train, unmetered[index].whole)
			}
		}
		if len(train) == 0 || len(test) == 0 {
			continue
		}
		for _, held := range candidates(profile) {
			coefficients, fitOK := held.fit(train)
			if !fitOK {
				continue
			}
			result.fitted[held.name] = append(result.fitted[held.name], coefficients)
			for _, ride := range test {
				scored, scoreOK := Evaluate([]Ride{ride.whole}, coefficients)
				if !scoreOK {
					continue
				}
				target := ride.whole.Blocks[0].TargetWatts
				score := rideScore{modelled: target + scored.BiasWatts, target: target}
				if score.scorable() {
					result.rides[held.name] = append(result.rides[held.name], score)
				}
			}
		}
	}

	whole := make([]Ride, 0, len(unmetered))
	for index := range unmetered {
		whole = append(whole, unmetered[index].whole)
	}
	shipped, _, shipOK := FitDragArea(statedBicycle().RollingResistance, whole)
	if shipOK {
		result.shipped, result.shippedOK = shipped, true
	}

	return result, nil
}

// bicycleForTarget is one target's saved bicycle, the way
// internal/activity/derive.go bicycleOf reads it: both numbers set names that
// pair, else the built-in default.
func bicycleForTarget(
	ctx context.Context, store *sqlite.Store, targetID string, cache map[string]measure.Coefficients,
) (measure.Coefficients, error) {
	if cached, ok := cache[targetID]; ok {
		return cached, nil
	}
	coefficients := measure.DefaultCoefficients()
	subject, err := store.TargetOwner(ctx, targetID)
	if err != nil {
		return measure.Coefficients{}, fmt.Errorf("reading a target's owner: %w", err)
	}
	if subject != "" {
		profile, profileErr := store.RiderProfile(ctx, subject)
		if profileErr != nil {
			return measure.Coefficients{}, fmt.Errorf("reading a rider profile: %w", profileErr)
		}
		if profile.DragAreaM2.Set && profile.RollingResistance.Set {
			coefficients = measure.Coefficients{DragArea: profile.DragAreaM2.Number, RollingResistance: profile.RollingResistance.Number}
		}
	}
	cache[targetID] = coefficients

	return coefficients, nil
}

// massForTarget is one target's total system mass: its rider profile's, the
// way internal/activity/derive.go reads it, or the -mass flag where no profile
// gives one.
func massForTarget(
	ctx context.Context, store *sqlite.Store, targetID string, massFlagKG float64, cache map[string]float64,
) (float64, error) {
	if cached, ok := cache[targetID]; ok {
		return cached, nil
	}
	mass := massFlagKG
	subject, err := store.TargetOwner(ctx, targetID)
	if err != nil {
		return 0, fmt.Errorf("reading a target's owner: %w", err)
	}
	if subject != "" {
		profile, profileErr := store.RiderProfile(ctx, subject)
		if profileErr != nil {
			return 0, fmt.Errorf("reading a rider profile: %w", profileErr)
		}
		if inputs := trainingload.InputsOf(&profile); inputs.TotalMassKG > 0 {
			mass = inputs.TotalMassKG
		}
	}
	if mass <= 0 {
		return 0, errors.New("a target has neither a profile mass nor a -mass flag")
	}
	cache[targetID] = mass

	return mass, nil
}

// maxHeartRateForTarget is one target's own configured maximum heart rate, or
// zero where its rider has entered none: measure.CapHeartRate reads that as
// no ceiling to cap against, the same as a rider who never set one sees live.
func maxHeartRateForTarget(
	ctx context.Context, store *sqlite.Store, targetID string, cache map[string]float64,
) (float64, error) {
	if cached, ok := cache[targetID]; ok {
		return cached, nil
	}
	var maxHeartRate float64
	subject, err := store.TargetOwner(ctx, targetID)
	if err != nil {
		return 0, fmt.Errorf("reading a target's owner: %w", err)
	}
	if subject != "" {
		profile, profileErr := store.RiderProfile(ctx, subject)
		if profileErr != nil {
			return 0, fmt.Errorf("reading a rider profile: %w", profileErr)
		}
		maxHeartRate = profile.MaxHeartRateBPM.Number
	}
	cache[targetID] = maxHeartRate

	return maxHeartRate, nil
}
