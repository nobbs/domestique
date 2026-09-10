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

// zwiftRollingResistance is what Zwift's own simulation rolls a road bike at,
// so a drag area fitted on its rides is read against its physics, not ours.
const zwiftRollingResistance = 0.004

// minHeartRateCoverage is the share of a ride's track samples that must carry
// a heart rate before the ride's mean heart rate is trusted as its target.
const minHeartRateCoverage = 0.8

// A meteredRide is one ride that carried a meter: its blocks for the bridge,
// and its own track and power series for checking the model against a power
// that was measured, second by second.
type meteredRide struct {
	at     time.Time
	track  []measure.Sample
	power  []trainingload.Sample
	blocks []MeasuredBlock
	massKG float64
}

// An unmeteredRide is one road ride as the study scores it: whole is the one
// block the yardstick judges, the model's pedalling samples against the power
// the ride's mean heart rate names; blocks are the five-minute cuts kept as a
// diagnostic of the shape.
type unmeteredRide struct {
	blockHeartRate []float64
	whole          Ride
	blocks         Ride
	meanHeartRate  float64
	windKMH        float64
	hasWind        bool
	atUnix         int64
}

// positive is a series without the readings a sensor never took: a strap
// that was not paired writes nought, and nought is not a heart rate.
func positive(readings []trainingload.Sample) []trainingload.Sample {
	kept := make([]trainingload.Sample, 0, len(readings))
	for _, reading := range readings {
		if reading.Value > 0 {
			kept = append(kept, reading)
		}
	}

	return kept
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
func meteredBlocksOf(power, heartRate []trainingload.Sample, block time.Duration) []MeasuredBlock {
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
	blocks := make([]MeasuredBlock, 0, len(keys))
	for _, key := range keys {
		if powerCounts[key] < half || heartRateCounts[key] < half {
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

// unmeteredBlocksOf cuts a road ride into the whole-ride block the yardstick
// scores and the five-minute blocks kept as a diagnostic, each with the mean
// heart rate held over it. Whether a sample is estimable turns on the
// recording's own gaps, not on any coefficient, so the prior decides
// membership once for every candidate that follows.
func unmeteredBlocksOf(
	track []measure.Sample, heartRate []trainingload.Sample, massKG float64, block time.Duration,
) (whole Block, blocks []Block, blockHeartRate []float64) {
	estimates, ok := measure.EstimateSeries(track, massKG, statedBicycle())
	if !ok || len(track) == 0 {
		return Block{}, nil, nil
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
		blocks = append(blocks, Block{Indices: indicesByBlock[key]})
		blockHeartRate = append(blockHeartRate, heartRateMeans[key])
	}

	return whole, blocks, blockHeartRate
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
// shared slope. A median so one race or one unpaired strap moves nothing.
func (b bridge) wattsAt(at time.Time, heartRateBPM float64) float64 {
	end := sort.Search(len(b.levels), func(i int) bool { return b.levels[i].at.After(at) })
	end = min(max(end, b.window), len(b.levels))
	start := max(end-b.window, 0)
	intercepts := make([]float64, 0, end-start)
	for _, level := range b.levels[start:end] {
		intercepts = append(intercepts, level.watts-b.wattsPerBPM*level.heartRate)
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
// samples against what it is judged by, a heart rate's word or a meter's.
type rideScore struct {
	modelled, target, windKMH float64
	hasWind                   bool
}

func (s rideScore) errorPercent() float64 { return 100 * (s.modelled - s.target) / s.target }

// A meteredCheck is the model held against a meter on the rides that had
// one: per ride, per block and per second.
type meteredCheck struct {
	name              string
	rides             []rideScore
	blocks            Result
	coefficients      measure.Coefficients
	secondCorrelation float64
	secondRMS         float64
	// The same two figures with both series smoothed over smoothingSeconds,
	// which is the resolution the model's own windowing works at.
	smoothCorrelation float64
	smoothRMS         float64
	seconds           int
}

// smoothingSeconds is the rolling mean the per-second check is also read
// at: the model measures speed and grade over a 100 m window, a quarter of a
// minute at road speeds, so a meter's second-to-second jitter is beneath it.
const smoothingSeconds = 30

// smoothed is values under a centred rolling mean of width samples.
func smoothed(values []float64, width int) []float64 {
	out := make([]float64, len(values))
	half := width / 2
	for index := range values {
		low, high := max(index-half, 0), min(index+half, len(values)-1)
		total := 0.0
		for held := low; held <= high; held++ {
			total += values[held]
		}
		out[index] = total / float64(high-low+1)
	}

	return out
}

// A report is levelstudy's aggregate result. No ride identifier, date,
// position or altitude value is ever held here.
type report struct {
	rides        map[string][]rideScore
	blocks       map[string][]Result
	fitted       map[string][]measure.Coefficients
	checks       []meteredCheck
	bridge       bridge
	shipped      measure.Coefficients
	rideCount    int
	blockCount   int
	meteredRides int
	checkedRides int
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
	fmt.Fprintf(&b, "corpus: %d unmetered rides (%d diagnostic blocks), %d metered rides, %d skipped\n\n",
		r.rideCount, r.blockCount, r.meteredRides, r.skipped)

	fmt.Fprintln(&b, "held-out whole-ride agreement with what the rider's heart rate says")
	fmt.Fprintf(&b, "  %-8s %6s %10s %10s %10s %10s %10s %10s\n",
		"candidate", "rides", "MAE %", "bias %", "q1 %", "q3 %", "RMS W", "bias W")
	for _, name := range []string{"default", "prior", "profile", "cda"} {
		if scores := r.rides[name]; len(scores) > 0 {
			fmt.Fprintf(&b, "  %-8s ", name)
			writeRideTable(&b, scores)
		}
	}

	fmt.Fprintln(&b, "\nheld-out five-minute blocks, a diagnostic of the shape (lower is better)")
	fmt.Fprintf(&b, "  %-8s %10s %10s %10s\n", "candidate", "RMS W", "bias W", "blocks")
	for _, name := range []string{"default", "prior", "profile", "cda"} {
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

	if fitted := r.fitted["cda"]; len(fitted) > 0 {
		drag := make([]float64, 0, len(fitted))
		for _, coefficients := range fitted {
			drag = append(drag, coefficients.DragArea)
		}
		fmt.Fprintf(&b, "\ndrag area at Crr %.3f, fold to fold: CdA %.3f ±%.3f\n",
			statedBicycle().RollingResistance, mean(drag), spread(drag))
		fmt.Fprintf(&b, "fitted over the whole corpus: CdA %.3f, Crr %.3f\n", r.shipped.DragArea, r.shipped.RollingResistance)
	}

	if scores := r.rides["cda"]; len(scores) > 0 {
		var percents, wind []float64
		for _, score := range scores {
			if score.hasWind {
				percents = append(percents, score.errorPercent())
				wind = append(wind, score.windKMH)
			}
		}
		if len(wind) >= 3 {
			median := quantile(wind, 0.5)
			var calm, windy []float64
			for index, held := range wind {
				if held <= median {
					calm = append(calm, percents[index])
				} else {
					windy = append(windy, percents[index])
				}
			}
			fmt.Fprintf(&b, "\nwind, a diagnostic only: %d rides with a forecast, error against wind speed r = %.2f",
				len(wind), correlation(percents, wind))
			if len(calm) > 0 && len(windy) > 0 {
				fmt.Fprintf(&b, ", mean error %.1f %% on the calmer half (<= %.0f km/h) and %.1f %% on the windier half",
					mean(calm), median, mean(windy))
			}
			fmt.Fprintln(&b)
		}
	}

	if len(r.checks) > 0 {
		fmt.Fprintf(&b, "\nthe model against a meter, on %d metered rides with a track\n", r.checkedRides)
		fmt.Fprintf(&b, "  %-14s %6s %10s %10s %10s %10s %10s %10s\n",
			"coefficients", "rides", "MAE %", "bias %", "q1 %", "q3 %", "RMS W", "bias W")
		for _, check := range r.checks {
			if len(check.rides) == 0 {
				continue
			}
			fmt.Fprintf(&b, "  %-14s ", check.name)
			writeRideTable(&b, check.rides)
		}
		fmt.Fprintf(&b, "  %-14s %10s %10s %10s %10s %10s %10s %10s\n", "",
			"blk RMS W", "blk bias W", "blocks", "sec r", "sec RMS W", "30s r", "30s RMS W")
		for _, check := range r.checks {
			if len(check.rides) == 0 {
				continue
			}
			fmt.Fprintf(&b, "  %-14s %10.1f %10.1f %10d %10.2f %10.1f %10.2f %10.1f\n", check.name,
				check.blocks.RMSWatts, check.blocks.BiasWatts, check.blocks.Blocks,
				check.secondCorrelation, check.secondRMS, check.smoothCorrelation, check.smoothRMS)
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

// meteredRideOf pairs a metered ride's track with its power second by second
// and cuts it into blocks the model can be scored on, the same way an
// unmetered ride is, with the meter's own mean as each block's target.
func meteredRideOf(
	ride *meteredRide, block time.Duration,
) (whole, blocks Ride, indices []int, measured []float64) {
	wattsAt := make(map[int64]float64, len(ride.power))
	for _, reading := range ride.power {
		wattsAt[reading.At.Unix()] = reading.Value
	}
	estimates, ok := measure.EstimateSeries(ride.track, ride.massKG, statedBicycle())
	if !ok {
		return Ride{}, Ride{}, nil, nil
	}
	start := ride.track[0].At
	all := Block{}
	byBlock, sumByBlock := map[int][]int{}, map[int]float64{}
	for index := range ride.track {
		watts, present := wattsAt[ride.track[index].At.Unix()]
		if !present || !estimates[index].Known || !pedalling(&ride.track[index]) {
			continue
		}
		indices = append(indices, index)
		measured = append(measured, watts)
		all.Indices = append(all.Indices, index)
		all.TargetWatts += watts
		key := int(ride.track[index].At.Sub(start) / block)
		byBlock[key] = append(byBlock[key], index)
		sumByBlock[key] += watts
	}
	if len(all.Indices) == 0 {
		return Ride{}, Ride{}, nil, nil
	}
	all.TargetWatts /= float64(len(all.Indices))
	keys := make([]int, 0, len(byBlock))
	for key := range byBlock {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	var cuts []Block
	half := int(block.Seconds() / 2)
	for _, key := range keys {
		if len(byBlock[key]) < half {
			continue
		}
		cuts = append(cuts, Block{Indices: byBlock[key], TargetWatts: sumByBlock[key] / float64(len(byBlock[key]))})
	}

	return Ride{Samples: ride.track, Blocks: []Block{all}, TotalMassKG: ride.massKG},
		Ride{Samples: ride.track, Blocks: cuts, TotalMassKG: ride.massKG}, indices, measured
}

// checkAgainstMeter runs the model at one pair of coefficients over every
// metered ride with a track and reports how it sits against the meter per
// ride, per block and per second.
func checkAgainstMeter(
	name string, coefficients measure.Coefficients, wholes, blocks []Ride, indices [][]int, measured [][]float64,
) meteredCheck {
	check := meteredCheck{name: name, coefficients: coefficients}
	var modelledSeconds, measuredSeconds, modelledSmooth, measuredSmooth []float64
	for index := range wholes {
		scored, ok := Evaluate(wholes[index:index+1], coefficients)
		if !ok {
			continue
		}
		target := wholes[index].Blocks[0].TargetWatts
		check.rides = append(check.rides, rideScore{modelled: target + scored.BiasWatts, target: target})
		estimates, estimateOK := measure.EstimateSeries(wholes[index].Samples, wholes[index].TotalMassKG, coefficients)
		if !estimateOK {
			continue
		}
		modelledRide := make([]float64, 0, len(indices[index]))
		for _, sampleIndex := range indices[index] {
			modelledRide = append(modelledRide, estimates[sampleIndex].Watts)
		}
		modelledSeconds = append(modelledSeconds, modelledRide...)
		measuredSeconds = append(measuredSeconds, measured[index]...)
		modelledSmooth = append(modelledSmooth, smoothed(modelledRide, smoothingSeconds)...)
		measuredSmooth = append(measuredSmooth, smoothed(measured[index], smoothingSeconds)...)
	}
	check.blocks, _ = Evaluate(blocks, coefficients)
	check.seconds = len(modelledSeconds)
	if check.seconds >= 3 {
		check.secondCorrelation, check.secondRMS = agreement(modelledSeconds, measuredSeconds)
		check.smoothCorrelation, check.smoothRMS = agreement(modelledSmooth, measuredSmooth)
	}

	return check
}

// agreement is the Pearson correlation and the RMS difference between two
// equal-length series.
func agreement(a, b []float64) (r, rms float64) {
	var sumSquares float64
	for index := range a {
		residual := a[index] - b[index]
		sumSquares += residual * residual
	}

	return correlation(a, b), math.Sqrt(sumSquares / float64(len(a)))
}

// study reads every recorded ride, bridges the metered ones to the unmetered
// ones by heart rate, and scores each candidate on folds its own fit never
// saw. Folds are cut by ride: two blocks of one ride share its weather, its
// bicycle and its rider's day, so splitting them would let a fit see its own
// test set. With checkYear set, the metered rides of that year are also
// held against their own meter at the coefficients the corpus fitted.
func study(
	ctx context.Context, store *sqlite.Store, minSamples int, block time.Duration, folds, window, checkYear int, massFlagKG float64, target string,
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
		blocks: map[string][]Result{},
		fitted: map[string][]measure.Coefficients{},
	}
	massCache := map[string]float64{}
	bicycleCache := map[string]measure.Coefficients{}
	profile := measure.DefaultCoefficients()
	if len(rides) > 0 {
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
		mass, massErr := massForTarget(ctx, store, ride.TargetID, massFlagKG, massCache)
		if massErr != nil {
			return nil, massErr
		}
		heartRate := positive(samples.HeartRate)

		if len(samples.Power) > 0 {
			blocks := meteredBlocksOf(samples.Power, heartRate, block)
			if len(blocks) == 0 {
				result.skipped++
				continue
			}
			result.meteredRides++
			metered = append(metered, meteredRide{
				at: samples.Power[0].At, blocks: blocks, track: samples.Track, power: samples.Power, massKG: mass,
			})

			continue
		}

		if len(samples.Track) < minSamples || float64(len(heartRate)) < minHeartRateCoverage*float64(len(samples.Track)) {
			result.skipped++
			continue
		}
		whole, blocks, heartRates := unmeteredBlocksOf(samples.Track, heartRate, mass, block)
		if len(whole.Indices) == 0 {
			result.skipped++
			continue
		}
		heartRateSum := 0.0
		for _, reading := range heartRate {
			heartRateSum += reading.Value
		}
		held := unmeteredRide{
			atUnix:         samples.Track[0].At.Unix(),
			whole:          Ride{Samples: samples.Track, Blocks: []Block{whole}, TotalMassKG: mass},
			blocks:         Ride{Samples: samples.Track, Blocks: blocks, TotalMassKG: mass},
			blockHeartRate: heartRates,
			meanHeartRate:  heartRateSum / float64(len(heartRate)),
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

	fitted, ok := fitBridge(metered, window)
	if !ok {
		return nil, fmt.Errorf("no bridge: %d metered rides name no slope", len(metered))
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
				if scored, scoreOK := Evaluate([]Ride{ride.blocks}, coefficients); scoreOK {
					result.blocks[held.name] = append(result.blocks[held.name], scored)
				}
				scored, scoreOK := Evaluate([]Ride{ride.whole}, coefficients)
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

	whole := make([]Ride, 0, len(unmetered))
	for index := range unmetered {
		whole = append(whole, unmetered[index].whole)
	}
	shipped, _, shipOK := FitDragArea(statedBicycle().RollingResistance, whole)
	if shipOK {
		result.shipped = shipped
	}

	if checkYear > 0 {
		var wholes, blocks []Ride
		var indices [][]int
		var measured [][]float64
		for index := range metered {
			ride := &metered[index]
			if ride.at.UTC().Year() != checkYear || len(ride.track) < minSamples {
				continue
			}
			rideWhole, rideBlocks, rideIndices, rideMeasured := meteredRideOf(ride, block)
			if len(rideIndices) == 0 {
				continue
			}
			wholes = append(wholes, rideWhole)
			blocks = append(blocks, rideBlocks)
			indices = append(indices, rideIndices)
			measured = append(measured, rideMeasured)
		}
		result.checkedRides = len(wholes)
		if len(wholes) > 0 {
			zwiftFit, _, zwiftOK := FitDragArea(zwiftRollingResistance, wholes)
			if shipOK {
				result.checks = append(result.checks,
					checkAgainstMeter("learned", result.shipped, wholes, blocks, indices, measured))
			}
			result.checks = append(result.checks,
				checkAgainstMeter("default", measure.DefaultCoefficients(), wholes, blocks, indices, measured),
				checkAgainstMeter("profile", profile, wholes, blocks, indices, measured))
			if zwiftOK {
				result.checks = append(result.checks, checkAgainstMeter(
					fmt.Sprintf("zwift-fit %.2f", zwiftFit.DragArea), zwiftFit, wholes, blocks, indices, measured))
			}
		}
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
