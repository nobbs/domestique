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

// minHeartRateCoverage is the share of the track's own elapsed span a strap
// must hold a reading for, within the track's span alone, before the ride's
// mean heart rate is trusted as its target: a duration, not a sample count,
// so two sensors sampling at different rates are judged the same way.
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
// that was not paired writes nought, and nought is not a heart rate. For a
// mean alone: held duration must treat a nought as invalid instead, see
// heldSecondsAcrossRuns, or a dropout it wrote this way bridges across as
// continuously covered.
func positive(readings []trainingload.Sample) []trainingload.Sample {
	kept := make([]trainingload.Sample, 0, len(readings))
	for _, reading := range readings {
		if reading.Value > 0 {
			kept = append(kept, reading)
		}
	}

	return kept
}

// heldSecondsAcrossRuns is how long readings held within each block, counted
// from start: a step no wider than measure.DefaultMaxGap is credited to the
// block its own start falls in. Duration, not a reading count, is what a
// block's own coverage gate must be judged against -- a coarser sensor's
// ordinary sampling interval leaves it fewer readings without leaving it any
// less covered. A step whose reading on either end invalid reports true for
// is never counted: an excluded reading -- a dropout's nought, a coast, one
// outside the span being judged -- must break held time there, not let the
// ordinary gap tolerance bridge across it the way a reading simply never
// taken would.
func heldSecondsAcrossRuns(
	readings []trainingload.Sample, invalid func(trainingload.Sample) bool, start time.Time, block time.Duration,
) map[int]float64 {
	held := map[int]float64{}
	for index := 0; index < len(readings)-1; index++ {
		if invalid(readings[index]) || invalid(readings[index+1]) {
			continue
		}
		step := readings[index+1].At.Sub(readings[index].At)
		if step <= 0 || step > measure.DefaultMaxGap {
			continue
		}
		key := int(readings[index].At.Sub(start) / block)
		held[key] += step.Seconds()
	}

	return held
}

// totalHeldSeconds is heldSecondsAcrossRuns summed over the whole series
// rather than bucketed into blocks.
func totalHeldSeconds(readings []trainingload.Sample, invalid func(trainingload.Sample) bool) float64 {
	held := 0.0
	for index := 0; index < len(readings)-1; index++ {
		if invalid(readings[index]) || invalid(readings[index+1]) {
			continue
		}
		step := readings[index+1].At.Sub(readings[index].At)
		if step > 0 && step <= measure.DefaultMaxGap {
			held += step.Seconds()
		}
	}

	return held
}

// heldSecondsAcrossTrackRuns is heldSecondsAcrossRuns for a track series,
// where invalid is precomputed once per sample rather than read off the
// sample itself: a track sample's validity turns on the estimate and cadence
// beside it, not on any field of its own.
func heldSecondsAcrossTrackRuns(track []measure.Sample, invalid []bool, start time.Time, block time.Duration) map[int]float64 {
	held := map[int]float64{}
	for index := 0; index < len(track)-1; index++ {
		if invalid[index] || invalid[index+1] {
			continue
		}
		step := track[index+1].At.Sub(track[index].At)
		if step <= 0 || step > measure.DefaultMaxGap {
			continue
		}
		key := int(track[index].At.Sub(start) / block)
		held[key] += step.Seconds()
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

// cadenceMatchTolerance is how far a power reading may sit from the nearest
// cadence reading and still be judged a coast by it: half a typical sensor's
// broadcast interval, wide enough that the two independent series' ordinary
// sampling offset never leaves a coast unmatched.
const cadenceMatchTolerance = 2 * time.Second

// coastingAt is whether the cadence reading nearest at, if one stands close
// enough to trust, names a coast. A power reading with no cadence series
// beside it at all is never judged a coast this way.
func coastingAt(at time.Time, cadence []trainingload.Sample) bool {
	index := sort.Search(len(cadence), func(i int) bool { return !cadence[i].At.Before(at) })
	nearest := (*trainingload.Sample)(nil)
	if index < len(cadence) {
		nearest = &cadence[index]
	}
	if index > 0 {
		if before := &cadence[index-1]; nearest == nil || at.Sub(before.At) < nearest.At.Sub(at) {
			nearest = before
		}
	}
	if nearest == nil {
		return false
	}
	gap := nearest.At.Sub(at)
	if gap < 0 {
		gap = -gap
	}

	return gap <= cadenceMatchTolerance && nearest.Value == 0
}

// meteredBlocksOf cuts a metered ride into blocks: those holding at least
// half their own length in both readings, over the samples the rider was
// pedalling through -- the unmetered side is scored the same way, and a
// coasting-heavy trainer ride must not lower the bridge target against a
// model that never sees its zero-power samples. Only the power and the heart
// rate are read; a trainer ride's speed and distance are a simulation.
func meteredBlocksOf(power, heartRate, cadence []trainingload.Sample, block time.Duration) []MeasuredBlock {
	if len(power) == 0 || len(heartRate) == 0 {
		return nil
	}
	pedallingPower := make([]trainingload.Sample, 0, len(power))
	for _, reading := range power {
		if !coastingAt(reading.At, cadence) {
			pedallingPower = append(pedallingPower, reading)
		}
	}
	if len(pedallingPower) == 0 {
		return nil
	}
	start, end := power[0].At, power[len(power)-1].At
	// The same two restrictions the power side already applies: a reading
	// from outside the powered ride's own span, or one recorded while the
	// rider coasted, must not shift the level the bridge learns from a heart
	// rate the pedalling watts beside it never produced.
	pedallingHeartRate := make([]trainingload.Sample, 0, len(heartRate))
	for _, reading := range heartRate {
		if reading.At.Before(start) || reading.At.After(end) || coastingAt(reading.At, cadence) {
			continue
		}
		pedallingHeartRate = append(pedallingHeartRate, reading)
	}
	powerMeans, _ := blockMean(pedallingPower, start, block)
	heartRateMeans, _ := blockMean(positive(pedallingHeartRate), start, block)
	// Held duration is judged over the ORIGINAL series, coasting/window/nought
	// marked invalid rather than spliced out first: two readings either side
	// of an excluded one must not pair across it just because the gap between
	// them, once it is gone, sits within the ordinary tolerance.
	powerHeld := heldSecondsAcrossRuns(power, func(r trainingload.Sample) bool {
		return coastingAt(r.At, cadence)
	}, start, block)
	heartRateHeld := heldSecondsAcrossRuns(heartRate, func(r trainingload.Sample) bool {
		return r.At.Before(start) || r.At.After(end) || coastingAt(r.At, cadence) || r.Value <= 0
	}, start, block)

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
	start, trackEnd := track[0].At, track[len(track)-1].At
	trackInvalid := make([]bool, len(track))
	indicesByBlock := map[int][]int{}
	for index := range track {
		trackInvalid[index] = !estimates[index].Known || !pedalling(&track[index])
		if trackInvalid[index] {
			continue
		}
		whole.Indices = append(whole.Indices, index)
		key := int(track[index].At.Sub(start) / block)
		indicesByBlock[key] = append(indicesByBlock[key], index)
	}
	pedallingHeartRate := heartRateOverTrack(heartRate, track)
	heartRateMeans, _ := blockMean(positive(pedallingHeartRate), start, block)
	// Held duration reads the ORIGINAL series, invalid readings marked rather
	// than spliced out first: see heldSecondsAcrossRuns.
	heartRateHeld := heldSecondsAcrossRuns(heartRate, func(r trainingload.Sample) bool {
		return r.At.Before(start) || r.At.After(trackEnd) || !pedallingAt(r.At, track) || r.Value <= 0
	}, start, block)
	trackHeld := heldSecondsAcrossTrackRuns(track, trackInvalid, start, block)

	keys := make([]int, 0, len(indicesByBlock))
	for key := range indicesByBlock {
		keys = append(keys, key)
	}
	sort.Ints(keys)

	halfBlockSeconds := block.Seconds() / 2
	for _, key := range keys {
		if trackHeld[key] < halfBlockSeconds || heartRateHeld[key] < halfBlockSeconds {
			continue
		}
		blocks = append(blocks, Block{Indices: indicesByBlock[key]})
		blockHeartRate = append(blockHeartRate, heartRateMeans[key])
	}

	return whole, blocks, blockHeartRate
}

// heartRateOverTrack is the heart-rate readings recorded within the track's
// own span, while the track says the rider was pedalling: a FIT file's
// readings from before or after the stretch that carries a position (while
// GPS or altitude was unavailable, say), or recorded through a coast the
// model's own samples already exclude, must not enter a target the model is
// scored against by time or effort it does not cover.
func heartRateOverTrack(heartRate []trainingload.Sample, track []measure.Sample) []trainingload.Sample {
	if len(track) == 0 {
		return nil
	}
	trackStart, trackEnd := track[0].At, track[len(track)-1].At
	kept := make([]trainingload.Sample, 0, len(heartRate))
	for _, reading := range heartRate {
		if reading.At.Before(trackStart) || reading.At.After(trackEnd) || !pedallingAt(reading.At, track) {
			continue
		}
		kept = append(kept, reading)
	}

	return kept
}

// pedallingAt is whether the track sample nearest at, within
// cadenceMatchTolerance, says the rider was pedalling. A heart-rate reading
// with no track sample close enough to trust is kept rather than guessed at:
// the track's own span and coverage requirements already bound how far that
// can drift.
func pedallingAt(at time.Time, track []measure.Sample) bool {
	index := sort.Search(len(track), func(i int) bool { return !track[i].At.Before(at) })
	nearest := (*measure.Sample)(nil)
	if index < len(track) {
		nearest = &track[index]
	}
	if index > 0 {
		if before := &track[index-1]; nearest == nil || at.Sub(before.At) < nearest.At.Sub(at) {
			nearest = before
		}
	}
	if nearest == nil {
		return true
	}
	gap := nearest.At.Sub(at)
	if gap < 0 {
		gap = -gap
	}
	if gap > cadenceMatchTolerance {
		return true
	}

	return pedalling(nearest)
}

// meanHeartRateOverTrack is the mean of the heart-rate readings recorded
// within the track's own span. False for an empty track or one with no heart
// rate over it.
func meanHeartRateOverTrack(heartRate []trainingload.Sample, track []measure.Sample) (mean float64, ok bool) {
	kept := positive(heartRateOverTrack(heartRate, track))
	if len(kept) == 0 {
		return 0, false
	}
	sum := 0.0
	for _, reading := range kept {
		sum += reading.Value
	}

	return sum / float64(len(kept)), true
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
// samples against what it is judged by, a heart rate's word or a meter's.
type rideScore struct {
	modelled, target, windKMH float64
	hasWind                   bool
}

func (s rideScore) errorPercent() float64 { return 100 * (s.modelled - s.target) / s.target }

// scorable is whether a ride can be judged in per cent at all: a target of
// nought names no scale to judge against.
func (s rideScore) scorable() bool { return s.target > 0 }

// A meteredCheck is the model held against a meter on the rides that had
// one: per ride, per block and per second.
type meteredCheck struct {
	name              string
	rides             []rideScore
	blocks            Result
	coefficients      measure.Coefficients
	secondCorrelation float64
	secondRMS         float64
	// The same two figures with both series smoothed over smoothingWindow,
	// which is the resolution the model's own windowing works at.
	smoothCorrelation float64
	smoothRMS         float64
	seconds           int
}

// smoothingWindow is the rolling mean the per-second check is also read
// at: the model measures speed and grade over a 100 m window, a quarter of a
// minute at road speeds, so a meter's second-to-second jitter is beneath it.
const smoothingWindow = 30 * time.Second

// smoothed is values under a centred rolling mean spanning window seconds
// either side of each sample's own timestamp -- named in time, not a count
// of retained samples: coasting removed from between them leaves the series
// unevenly spaced, so a fixed sample count would not name the seconds its
// own label claims.
func smoothed(times []time.Time, values []float64, window time.Duration) []float64 {
	out := make([]float64, len(values))
	half := window / 2
	for index, at := range times {
		total := 0.0
		low := index
		for low > 0 && at.Sub(times[low-1]) <= half {
			low--
		}
		high := index
		for high < len(times)-1 && times[high+1].Sub(at) <= half {
			high++
		}
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
	shippedOK    bool
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
		"candidate", "rides", "MdAE %", "bias %", "q1 %", "q3 %", "RMS W", "bias W")
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
		if r.shippedOK {
			fmt.Fprintf(&b, "fitted over the whole corpus: CdA %.3f, Crr %.3f\n",
				r.shipped.DragArea, r.shipped.RollingResistance)
		} else {
			fmt.Fprintln(&b, "fitted over the whole corpus: unavailable (the fit collapsed to its search bound)")
		}
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
			"coefficients", "rides", "MdAE %", "bias %", "q1 %", "q3 %", "RMS W", "bias W")
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
	trackInvalid := make([]bool, len(ride.track))
	byBlock, sumByBlock := map[int][]int{}, map[int]float64{}
	for index := range ride.track {
		watts, present := wattsAt[ride.track[index].At.Unix()]
		trackInvalid[index] = !present || !estimates[index].Known || !pedalling(&ride.track[index])
		if trackInvalid[index] {
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
	// Held duration reads the ORIGINAL track, invalid samples marked rather
	// than spliced out first: see heldSecondsAcrossRuns.
	held := heldSecondsAcrossTrackRuns(ride.track, trackInvalid, start, block)
	keys := make([]int, 0, len(byBlock))
	for key := range byBlock {
		keys = append(keys, key)
	}
	sort.Ints(keys)
	var cuts []Block
	halfBlockSeconds := block.Seconds() / 2
	for _, key := range keys {
		if held[key] < halfBlockSeconds {
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
		if score := (rideScore{modelled: target + scored.BiasWatts, target: target}); score.scorable() {
			check.rides = append(check.rides, score)
		}
		estimates, estimateOK := measure.EstimateSeries(wholes[index].Samples, wholes[index].TotalMassKG, coefficients)
		if !estimateOK {
			continue
		}
		rideTimes := make([]time.Time, 0, len(indices[index]))
		modelledRide := make([]float64, 0, len(indices[index]))
		for _, sampleIndex := range indices[index] {
			rideTimes = append(rideTimes, wholes[index].Samples[sampleIndex].At)
			modelledRide = append(modelledRide, estimates[sampleIndex].Watts)
		}
		modelledSeconds = append(modelledSeconds, modelledRide...)
		measuredSeconds = append(measuredSeconds, measured[index]...)
		modelledSmooth = append(modelledSmooth, smoothed(rideTimes, modelledRide, smoothingWindow)...)
		measuredSmooth = append(measuredSmooth, smoothed(rideTimes, measured[index], smoothingWindow)...)
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
		mass, massErr := massForTarget(ctx, store, ride.TargetID, massFlagKG, massCache)
		if massErr != nil {
			return nil, massErr
		}
		// Raw, noughts included: the functions below split on a nought
		// themselves where held duration is at stake, rather than having it
		// stripped here and silently bridged across as continuously held.
		heartRate := samples.HeartRate

		if len(samples.Power) > 0 {
			blocks := meteredBlocksOf(samples.Power, heartRate, samples.Cadence, block)
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

		if len(samples.Track) < minSamples {
			result.skipped++
			continue
		}
		trackStart, trackEnd := samples.Track[0].At, samples.Track[len(samples.Track)-1].At
		elapsedSeconds := trackEnd.Sub(trackStart).Seconds()
		// Read over the ORIGINAL heart-rate series, invalid readings marked
		// rather than spliced out first: see heldSecondsAcrossRuns.
		heldSeconds := totalHeldSeconds(heartRate, func(r trainingload.Sample) bool {
			return r.At.Before(trackStart) || r.At.After(trackEnd) || !pedallingAt(r.At, samples.Track) || r.Value <= 0
		})
		if elapsedSeconds <= 0 || heldSeconds < minHeartRateCoverage*elapsedSeconds {
			result.skipped++
			continue
		}
		whole, blocks, heartRates := unmeteredBlocksOf(samples.Track, heartRate, mass, block)
		if len(whole.Indices) == 0 {
			result.skipped++
			continue
		}
		meanHeartRate, meanOK := meanHeartRateOverTrack(heartRate, samples.Track)
		if !meanOK {
			result.skipped++
			continue
		}
		held := unmeteredRide{
			atUnix:         samples.Track[0].At.Unix(),
			whole:          Ride{Samples: samples.Track, Blocks: []Block{whole}, TotalMassKG: mass},
			blocks:         Ride{Samples: samples.Track, Blocks: blocks, TotalMassKG: mass},
			blockHeartRate: heartRates,
			meanHeartRate:  meanHeartRate,
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
				score := rideScore{
					modelled: target + scored.BiasWatts, target: target, windKMH: ride.windKMH, hasWind: ride.hasWind,
				}
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
