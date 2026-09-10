package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/powerfit"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/trainingload"
)

// uprightPrior is the bicycle the scale multiplies: the handover's upright
// row with a wide-tyre Crr, for a flat-bar trekking bike. The corpus's speeds
// cannot split CdA from Crr, so the ratio is stated here rather than fitted.
var uprightPrior = measure.Coefficients{DragArea: 0.45, RollingResistance: 0.010} //nolint:gochecknoglobals // dev tool

// minHeartRateCoverage is the share of a ride's track samples that must carry
// a heart rate before the ride's mean heart rate is trusted as its target.
const minHeartRateCoverage = 0.8

// A meteredBlock is one block of a ride that carried a meter, kept with the
// day it was ridden so the bridge can read the rider's form at the time.
type meteredBlock struct {
	at    time.Time
	block powerfit.MeasuredBlock
}

// An unmeteredRide is one road ride as the study scores it: whole is the one
// block the yardstick judges, the model's pedalling samples against the power
// the ride's mean heart rate names; blocks are the five-minute cuts kept as a
// diagnostic of the shape.
type unmeteredRide struct {
	blockHeartRate []float64
	whole          powerfit.Ride
	blocks         powerfit.Ride
	meanHeartRate  float64
	windKMH        float64
	hasWind        bool
	atUnix         int64
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
// half their own length in both readings. Only the power and the heart rate
// are read. A trainer ride's speed and distance are a simulation.
func meteredBlocksOf(power, heartRate []trainingload.Sample, block time.Duration) []powerfit.MeasuredBlock {
	if len(power) == 0 || len(heartRate) == 0 {
		return nil
	}
	start := power[0].At
	powerMeans, powerCounts := blockMean(power, start, block)
	heartRateMeans, heartRateCounts := blockMean(heartRate, start, block)

	keys := make([]int, 0, len(powerMeans))
	for key := range powerMeans {
		keys = append(keys, key)
	}
	sort.Ints(keys)

	half := int(block.Seconds() / 2)
	blocks := make([]powerfit.MeasuredBlock, 0, len(keys))
	for _, key := range keys {
		if powerCounts[key] < half || heartRateCounts[key] < half {
			continue
		}
		blocks = append(blocks, powerfit.MeasuredBlock{
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

// unmeteredBlocksOf cuts a road ride into the whole-ride block the yardstick
// scores and the five-minute blocks kept as a diagnostic, each with the mean
// heart rate held over it. Whether a sample is estimable turns on the
// recording's own gaps, not on any coefficient, so the prior decides
// membership once for every candidate that follows.
func unmeteredBlocksOf(
	track []measure.Sample, heartRate []trainingload.Sample, massKG float64, block time.Duration,
) (whole powerfit.Block, blocks []powerfit.Block, blockHeartRate []float64) {
	estimates, _, ok := measure.EstimateSeriesWith(track, massKG, uprightPrior, nil)
	if !ok || len(track) == 0 {
		return powerfit.Block{}, nil, nil
	}
	start := track[0].At
	indicesByBlock := map[int][]int{}
	for index := range track {
		if !estimates[index].Known || !pedalling(&track[index]) {
			continue
		}
		whole.Indices = append(whole.Indices, index)
		key := int(track[index].At.Sub(start) / block)
		indicesByBlock[key] = append(indicesByBlock[key], index)
	}
	heartRateMeans, heartRateCounts := blockMean(heartRate, start, block)

	keys := make([]int, 0, len(indicesByBlock))
	for key := range indicesByBlock {
		keys = append(keys, key)
	}
	sort.Ints(keys)

	half := int(block.Seconds() / 2)
	for _, key := range keys {
		if len(indicesByBlock[key]) < half || heartRateCounts[key] < half {
			continue
		}
		blocks = append(blocks, powerfit.Block{Indices: indicesByBlock[key]})
		blockHeartRate = append(blockHeartRate, heartRateMeans[key])
	}

	return whole, blocks, blockHeartRate
}

// A monthAnchor is one calendar month of metered riding: when it was, and
// the mean heart rate and power its blocks held.
type monthAnchor struct {
	at        time.Time
	heartRate float64
	watts     float64
}

// A bridge is the rider's heart-rate-to-power relation with the level free
// to move month by month: one slope shared across every month, and a
// per-month level interpolated across the months that carried no meter.
// Pooling every block into one line flattens the slope to what fitness drift
// leaves of it; letting the level move per month keeps the slope effort's.
type bridge struct {
	anchors     []monthAnchor
	wattsPerBPM float64
	residualRMS float64
	blocks      int
}

// fitBridge fits the per-month levels and the shared slope, the slope by
// least squares over every block's deviation from its own month's means.
func fitBridge(blocks []meteredBlock) (bridge, bool) {
	type sum struct {
		at, heartRate, watts float64
		n                    int
	}
	months := map[string]*sum{}
	for _, held := range blocks {
		key := held.at.UTC().Format("2006-01")
		if months[key] == nil {
			months[key] = &sum{}
		}
		months[key].at += float64(held.at.Unix())
		months[key].heartRate += held.block.HeartRateBPM
		months[key].watts += held.block.WattsMeasured
		months[key].n++
	}
	anchorByMonth := map[string]monthAnchor{}
	fitted := bridge{blocks: len(blocks)}
	for key, held := range months {
		n := float64(held.n)
		anchor := monthAnchor{
			at: time.Unix(int64(held.at/n), 0).UTC(), heartRate: held.heartRate / n, watts: held.watts / n,
		}
		anchorByMonth[key] = anchor
		fitted.anchors = append(fitted.anchors, anchor)
	}
	sort.Slice(fitted.anchors, func(i, j int) bool { return fitted.anchors[i].at.Before(fitted.anchors[j].at) })

	var covariance, variance float64
	for _, held := range blocks {
		anchor := anchorByMonth[held.at.UTC().Format("2006-01")]
		dh, dw := held.block.HeartRateBPM-anchor.heartRate, held.block.WattsMeasured-anchor.watts
		covariance += dh * dw
		variance += dh * dh
	}
	if len(fitted.anchors) == 0 || variance == 0 {
		return bridge{}, false
	}
	fitted.wattsPerBPM = covariance / variance

	var sumSquares float64
	for _, held := range blocks {
		residual := held.block.WattsMeasured - fitted.wattsAt(held.at, held.block.HeartRateBPM)
		sumSquares += residual * residual
	}
	fitted.residualRMS = math.Sqrt(sumSquares / float64(len(blocks)))

	return fitted, true
}

// wattsAt is the power this rider's heart rate says they were producing on a
// date: the month level interpolated linearly between the metered months
// either side of it, or the nearest one beyond the ends, moved along the
// shared slope by how far the heart rate sits from that level's own.
func (b bridge) wattsAt(at time.Time, heartRateBPM float64) float64 {
	level := b.anchors[0]
	if after := sort.Search(len(b.anchors), func(i int) bool { return !b.anchors[i].at.Before(at) }); after > 0 {
		level = b.anchors[after-1]
		if after < len(b.anchors) {
			before, next := b.anchors[after-1], b.anchors[after]
			weight := at.Sub(before.at).Seconds() / next.at.Sub(before.at).Seconds()
			level = monthAnchor{
				heartRate: before.heartRate + weight*(next.heartRate-before.heartRate),
				watts:     before.watts + weight*(next.watts-before.watts),
			}
		}
	}

	return max(level.watts+b.wattsPerBPM*(heartRateBPM-level.heartRate), 0)
}

// A candidate is one way of choosing the coefficients, scored on rides its
// fit never saw.
type candidate struct {
	fit  func([]powerfit.Ride) (measure.Coefficients, bool)
	name string
}

// candidates are the built-in road bicycle, the upright prior as it stands,
// and the prior under the one scale a corpus of one rider can support.
func candidates() []candidate {
	fixed := func(coefficients measure.Coefficients) func([]powerfit.Ride) (measure.Coefficients, bool) {
		return func([]powerfit.Ride) (measure.Coefficients, bool) { return coefficients, true }
	}

	return []candidate{
		{name: "default", fit: fixed(measure.DefaultCoefficients())},
		{name: "prior", fit: fixed(uprightPrior)},
		{name: "scale", fit: func(rides []powerfit.Ride) (measure.Coefficients, bool) {
			coefficients, _, ok := powerfit.FitScale(uprightPrior, rides)
			return coefficients, ok
		}},
	}
}

// A rideScore is one held-out ride's whole-ride result: what the model
// produced over its pedalling samples against what its heart rate named.
type rideScore struct {
	modelled, target, windKMH float64
	hasWind                   bool
}

func (s rideScore) errorPercent() float64 { return 100 * (s.modelled - s.target) / s.target }

// A report is levelstudy's aggregate result. No ride identifier, date,
// position or altitude value is ever held here.
type report struct {
	rides        map[string][]rideScore
	blocks       map[string][]powerfit.Result
	fitted       map[string][]measure.Coefficients
	bridge       bridge
	shipped      measure.Coefficients
	rideCount    int
	blockCount   int
	meteredRides int
	skipped      int
}

func (r *report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "bridge: %.2f W/bpm shared slope, %d monthly levels, residual RMS %.1f W over %d metered blocks\n",
		r.bridge.wattsPerBPM, len(r.bridge.anchors), r.bridge.residualRMS, r.bridge.blocks)
	fmt.Fprintf(&b, "corpus: %d unmetered rides (%d diagnostic blocks), %d metered rides, %d skipped\n\n",
		r.rideCount, r.blockCount, r.meteredRides, r.skipped)

	fmt.Fprintln(&b, "held-out whole-ride agreement with what the rider's heart rate says")
	fmt.Fprintf(&b, "  %-8s %6s %10s %10s %10s %10s %10s %10s\n",
		"candidate", "rides", "MAE %", "bias %", "q1 %", "q3 %", "RMS W", "bias W")
	for _, name := range []string{"default", "prior", "scale"} {
		scores := r.rides[name]
		if len(scores) == 0 {
			continue
		}
		errors := make([]float64, 0, len(scores))
		var sumSquares, sumSigned float64
		for _, score := range scores {
			errors = append(errors, score.errorPercent())
			residual := score.modelled - score.target
			sumSquares += residual * residual
			sumSigned += residual
		}
		absolute := make([]float64, len(errors))
		for index, held := range errors {
			absolute[index] = math.Abs(held)
		}
		n := float64(len(scores))
		fmt.Fprintf(&b, "  %-8s %6d %10.1f %10.1f %10.1f %10.1f %10.1f %10.1f\n", name, len(scores),
			quantile(absolute, 0.5), mean(errors), quantile(errors, 0.25), quantile(errors, 0.75),
			math.Sqrt(sumSquares/n), sumSigned/n)
	}

	fmt.Fprintln(&b, "\nheld-out five-minute blocks, a diagnostic of the shape (lower is better)")
	fmt.Fprintf(&b, "  %-8s %10s %10s %10s\n", "candidate", "RMS W", "bias W", "blocks")
	for _, name := range []string{"default", "prior", "scale"} {
		rms, bias, blocks := 0.0, 0.0, 0
		for _, result := range r.blocks[name] {
			rms += result.RMSWatts * float64(result.Blocks)
			bias += result.BiasWatts * float64(result.Blocks)
			blocks += result.Blocks
		}
		if blocks == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %-8s %10.1f %10.1f %10d\n", name, rms/float64(blocks), bias/float64(blocks), blocks)
	}

	if fitted := r.fitted["scale"]; len(fitted) > 0 {
		drag, rolling := make([]float64, 0, len(fitted)), make([]float64, 0, len(fitted))
		for _, coefficients := range fitted {
			drag = append(drag, coefficients.DragArea)
			rolling = append(rolling, coefficients.RollingResistance)
		}
		fmt.Fprintf(&b, "\nscale over the prior (CdA %.2f, Crr %.3f), fold to fold: CdA %.3f ±%.3f, Crr %.5f ±%.5f\n",
			uprightPrior.DragArea, uprightPrior.RollingResistance, mean(drag), spread(drag), mean(rolling), spread(rolling))
		fmt.Fprintf(&b, "fitted over the whole corpus: scale %.3f, CdA %.3f, Crr %.5f\n",
			r.shipped.DragArea/uprightPrior.DragArea, r.shipped.DragArea, r.shipped.RollingResistance)
	}

	if scores := r.rides["scale"]; len(scores) > 0 {
		var errors, wind []float64
		for _, score := range scores {
			if score.hasWind {
				errors = append(errors, score.errorPercent())
				wind = append(wind, score.windKMH)
			}
		}
		if len(wind) >= 3 {
			median := quantile(wind, 0.5)
			var calm, windy []float64
			for index, held := range wind {
				if held <= median {
					calm = append(calm, errors[index])
				} else {
					windy = append(windy, errors[index])
				}
			}
			fmt.Fprintf(&b, "\nwind, a diagnostic only: %d rides with a forecast, error against wind speed r = %.2f, "+
				"mean error %.1f %% on the calmer half (≤ %.0f km/h) and %.1f %% on the windier half\n",
				len(wind), correlation(errors, wind), mean(calm), median, mean(windy))
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

// correlation is the Pearson correlation between two equal-length series.
func correlation(a, b []float64) float64 {
	meanA, meanB := mean(a), mean(b)
	var cov, varA, varB float64
	for index := range a {
		da, db := a[index]-meanA, b[index]-meanB
		cov += da * db
		varA += da * da
		varB += db * db
	}
	if varA == 0 || varB == 0 {
		return 0
	}

	return cov / math.Sqrt(varA*varB)
}

// study reads every recorded ride, bridges the metered ones to the unmetered
// ones by heart rate, and scores each candidate on folds its own fit never
// saw. Folds are cut by ride: two blocks of one ride share its weather, its
// bicycle and its rider's day, so splitting them would let a fit see its own
// test set.
func study(
	ctx context.Context, store *sqlite.Store, minSamples int, block time.Duration, folds int, massFlagKG float64,
) (*report, error) {
	rides, err := store.RecordedRides(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing recorded rides: %w", err)
	}

	result := &report{
		rides:  map[string][]rideScore{},
		blocks: map[string][]powerfit.Result{},
		fitted: map[string][]measure.Coefficients{},
	}
	massCache := map[string]float64{}
	var metered []meteredBlock
	var unmetered []unmeteredRide

	for _, ride := range rides {
		samples, samplesErr := store.ActivityRideSamples(ctx, ride.TargetID, ride.WorkoutID)
		if samplesErr != nil {
			return nil, fmt.Errorf("reading a ride's samples: %w", samplesErr)
		}
		mass, massErr := massForTarget(ctx, store, ride.TargetID, massFlagKG, massCache)
		if massErr != nil {
			return nil, massErr
		}

		if len(samples.Power) > 0 {
			blocks := meteredBlocksOf(samples.Power, samples.HeartRate, block)
			if len(blocks) == 0 {
				result.skipped++
				continue
			}
			result.meteredRides++
			for _, held := range blocks {
				metered = append(metered, meteredBlock{at: samples.Power[0].At, block: held})
			}

			continue
		}

		if len(samples.Track) < minSamples ||
			float64(len(samples.HeartRate)) < minHeartRateCoverage*float64(len(samples.Track)) {
			result.skipped++
			continue
		}
		whole, blocks, heartRates := unmeteredBlocksOf(samples.Track, samples.HeartRate, mass, block)
		if len(whole.Indices) == 0 {
			result.skipped++
			continue
		}
		heartRateSum := 0.0
		for _, reading := range samples.HeartRate {
			heartRateSum += reading.Value
		}
		held := unmeteredRide{
			atUnix:         samples.Track[0].At.Unix(),
			whole:          powerfit.Ride{Samples: samples.Track, Blocks: []powerfit.Block{whole}, TotalMassKG: mass},
			blocks:         powerfit.Ride{Samples: samples.Track, Blocks: blocks, TotalMassKG: mass},
			blockHeartRate: heartRates,
			meanHeartRate:  heartRateSum / float64(len(samples.HeartRate)),
		}
		steps, weatherErr := store.ActivityWeatherSteps(ctx, ride.TargetID, ride.WorkoutID)
		if weatherErr != nil {
			return nil, fmt.Errorf("reading a ride's weather: %w", weatherErr)
		}
		for _, step := range steps {
			held.windKMH += step.WindSpeedKMH / float64(len(steps))
			held.hasWind = true
		}
		unmetered = append(unmetered, held)
	}

	fitted, ok := fitBridge(metered)
	if !ok {
		return nil, fmt.Errorf("no bridge: %d metered blocks name no slope", len(metered))
	}
	result.bridge = fitted

	for index := range unmetered {
		held := &unmetered[index]
		at := time.Unix(held.atUnix, 0).UTC()
		held.whole.Blocks[0].TargetWatts = fitted.wattsAt(at, held.meanHeartRate)
		for blockIndex := range held.blocks.Blocks {
			held.blocks.Blocks[blockIndex].TargetWatts = fitted.wattsAt(at, held.blockHeartRate[blockIndex])
		}
		result.blockCount += len(held.blocks.Blocks)
	}
	result.rideCount = len(unmetered)

	for fold := range folds {
		var train []powerfit.Ride
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
		for _, held := range candidates() {
			coefficients, fitOK := held.fit(train)
			if !fitOK {
				continue
			}
			result.fitted[held.name] = append(result.fitted[held.name], coefficients)
			for _, ride := range test {
				if scored, scoreOK := powerfit.Evaluate([]powerfit.Ride{ride.blocks}, coefficients); scoreOK {
					result.blocks[held.name] = append(result.blocks[held.name], scored)
				}
				scored, scoreOK := powerfit.Evaluate([]powerfit.Ride{ride.whole}, coefficients)
				if !scoreOK {
					continue
				}
				target := ride.whole.Blocks[0].TargetWatts
				result.rides[held.name] = append(result.rides[held.name], rideScore{
					modelled: target + scored.BiasWatts, target: target, windKMH: ride.windKMH, hasWind: ride.hasWind,
				})
			}
		}
	}

	whole := make([]powerfit.Ride, 0, len(unmetered))
	for index := range unmetered {
		whole = append(whole, unmetered[index].whole)
	}
	if shipped, _, shipOK := powerfit.FitScale(uprightPrior, whole); shipOK {
		result.shipped = shipped
	}

	return result, nil
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
	cache[targetID] = mass

	return mass, nil
}
