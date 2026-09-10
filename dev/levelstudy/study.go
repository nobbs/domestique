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

// A meteredBlock is one block of a ride that carried a meter, kept with the
// day it was ridden so a bridge can be drawn from the rider's own form at the
// time rather than from their form across all history.
type meteredBlock struct {
	at    time.Time
	block powerfit.MeasuredBlock
}

// An unmeteredRide is one road ride waiting for a bridge: the blocks are cut
// and the model can already run over them, but each block's target is only
// known once the rider's heart rate at that date has a line to read through.
type unmeteredRide struct {
	blockHeartRate []float64
	ride           powerfit.Ride
	// Seconds rather than a time.Time so the struct holds no pointer past its
	// slices, which is all fieldalignment is asking for.
	atUnix int64
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

// meteredBlocksOf cuts a metered ride into the blocks a bridge is fitted
// through: those holding at least half their own length in both readings.
// Only the power and the heart rate are read. A trainer ride's speed and
// distance are a simulation and no evidence of anything.
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

// unmeteredBlocksOf cuts a road ride into blocks of sample indices the model
// estimated across, each with the mean heart rate the rider held over it.
// Whether a sample is estimable turns on the recording's own gaps, not on any
// coefficient, so the default pair decides membership once for every
// candidate that follows.
func unmeteredBlocksOf(
	track []measure.Sample, heartRate []trainingload.Sample, massKG float64, block time.Duration,
) (blocks []powerfit.Block, meanHeartRate []float64) {
	estimates, _, ok := measure.EstimateSeries(track, massKG)
	if !ok || len(track) == 0 {
		return nil, nil
	}
	start := track[0].At
	indicesByBlock := map[int][]int{}
	for index, sample := range track {
		if !estimates[index].Known {
			continue
		}
		key := int(sample.At.Sub(start) / block)
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
		meanHeartRate = append(meanHeartRate, heartRateMeans[key])
	}

	return blocks, meanHeartRate
}

// bridgeNear fits the rider's heart-rate-to-power line from the metered
// blocks within window either side of a date, so a fit reads their form at
// the time rather than averaged across every season in the corpus.
func bridgeNear(blocks []meteredBlock, at time.Time, window time.Duration) (powerfit.Bridge, bool) {
	near := make([]powerfit.MeasuredBlock, 0, len(blocks))
	for _, held := range blocks {
		if held.at.Sub(at) <= window && at.Sub(held.at) <= window {
			near = append(near, held.block)
		}
	}

	return powerfit.FitBridge(near)
}

// A candidate is one way of choosing the coefficients, scored on rides its
// fit never saw.
type candidate struct {
	fit  func([]powerfit.Ride) (measure.Coefficients, powerfit.Result, bool)
	name string
}

// candidates are the built-in pair, which fits nothing and is the baseline,
// and the two fits #623 weighs against each other.
func candidates() []candidate {
	return []candidate{
		{name: "default", fit: func([]powerfit.Ride) (measure.Coefficients, powerfit.Result, bool) {
			return measure.DefaultCoefficients(), powerfit.Result{}, true
		}},
		{name: "scale", fit: powerfit.FitScale},
		{name: "pair", fit: powerfit.FitPair},
	}
}

// A report is levelstudy's aggregate result. No ride identifier, date,
// position or altitude value is ever held here.
type report struct {
	heldOut      map[string][]powerfit.Result
	fitted       map[string][]measure.Coefficients
	bridge       powerfit.Bridge
	rides        int
	meteredRides int
	blocks       int
	skipped      int
	noBridge     int
}

func (r *report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "bridge over every metered block: %.3f W/bpm, intercept %.1f W, residual RMS %.1f W, %d blocks\n",
		r.bridge.WattsPerBPM, r.bridge.InterceptWatts, r.bridge.ResidualRMSWatts, r.bridge.Blocks)
	fmt.Fprintf(&b, "corpus: %d unmetered rides (%d blocks), %d metered rides, %d skipped, %d without a bridge\n\n",
		r.rides, r.blocks, r.meteredRides, r.skipped, r.noBridge)

	fmt.Fprintln(&b, "held-out agreement with what the rider's heart rate says (lower is better)")
	fmt.Fprintf(&b, "  %-8s %10s %10s %10s\n", "candidate", "RMS W", "bias W", "blocks")
	for _, name := range []string{"default", "scale", "pair"} {
		rms, bias, blocks := 0.0, 0.0, 0
		for _, result := range r.heldOut[name] {
			rms += result.RMSWatts * float64(result.Blocks)
			bias += result.BiasWatts * float64(result.Blocks)
			blocks += result.Blocks
		}
		if blocks == 0 {
			continue
		}
		fmt.Fprintf(&b, "  %-8s %10.1f %10.1f %10d\n", name, rms/float64(blocks), bias/float64(blocks), blocks)
	}

	fmt.Fprintln(&b, "\ncoefficients each fold fitted, and how far they moved between folds")
	fmt.Fprintf(&b, "  %-8s %18s %18s\n", "candidate", "CdA (m²)", "Crr")
	for _, name := range []string{"scale", "pair"} {
		fitted := r.fitted[name]
		if len(fitted) == 0 {
			continue
		}
		drag, rolling := make([]float64, 0, len(fitted)), make([]float64, 0, len(fitted))
		for _, coefficients := range fitted {
			drag = append(drag, coefficients.DragArea)
			rolling = append(rolling, coefficients.RollingResistance)
		}
		fmt.Fprintf(&b, "  %-8s %8.3f ±%-8.3f %8.5f ±%-8.5f\n",
			name, mean(drag), spread(drag), mean(rolling), spread(rolling))
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

// study reads every recorded ride, bridges the metered ones to the unmetered
// ones by heart rate, and scores each candidate on folds its own fit never
// saw. Folds are cut by ride, never by block: two blocks of one ride share
// its weather, its bicycle and its rider's day, so splitting them would let a
// fit see its own test set.
func study(
	ctx context.Context, store *sqlite.Store, minSamples int,
	block, bridgeWindow time.Duration, folds int, massFlagKG float64,
) (*report, error) {
	rides, err := store.RecordedRides(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing recorded rides: %w", err)
	}

	result := &report{
		heldOut: map[string][]powerfit.Result{},
		fitted:  map[string][]measure.Coefficients{},
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

		if len(samples.Track) < minSamples || len(samples.HeartRate) == 0 {
			result.skipped++
			continue
		}
		blocks, heartRates := unmeteredBlocksOf(samples.Track, samples.HeartRate, mass, block)
		if len(blocks) == 0 {
			result.skipped++
			continue
		}
		unmetered = append(unmetered, unmeteredRide{
			atUnix:         samples.Track[0].At.Unix(),
			ride:           powerfit.Ride{Samples: samples.Track, Blocks: blocks, TotalMassKG: mass},
			blockHeartRate: heartRates,
		})
	}

	all := make([]powerfit.MeasuredBlock, 0, len(metered))
	for _, held := range metered {
		all = append(all, held.block)
	}
	result.bridge, _ = powerfit.FitBridge(all)

	// Each ride's targets come from the rider's form around the day they rode
	// it, and a ride with no metered rides near it cannot be scored at all.
	ready := make([]powerfit.Ride, 0, len(unmetered))
	for _, held := range unmetered {
		bridge, ok := bridgeNear(metered, time.Unix(held.atUnix, 0), bridgeWindow)
		if !ok {
			result.noBridge++
			continue
		}
		for index := range held.ride.Blocks {
			held.ride.Blocks[index].TargetWatts = bridge.WattsAt(held.blockHeartRate[index])
		}
		ready = append(ready, held.ride)
		result.blocks += len(held.ride.Blocks)
	}
	result.rides = len(ready)

	for fold := range folds {
		var train, test []powerfit.Ride
		for index, ride := range ready {
			if index%folds == fold {
				test = append(test, ride)
			} else {
				train = append(train, ride)
			}
		}
		if len(train) == 0 || len(test) == 0 {
			continue
		}
		for _, held := range candidates() {
			coefficients, _, ok := held.fit(train)
			if !ok {
				continue
			}
			scored, scoreOK := powerfit.Evaluate(test, coefficients)
			if !scoreOK {
				continue
			}
			result.heldOut[held.name] = append(result.heldOut[held.name], scored)
			if held.name != "default" {
				result.fitted[held.name] = append(result.fitted[held.name], coefficients)
			}
		}
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
