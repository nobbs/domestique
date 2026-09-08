package main

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/trainingload"
)

const (
	candidateNone    = "none"
	candidateWeather = "weather"
	candidateHRFit   = "hrfit"
)

func candidateOrder() []string { return []string{candidateNone, candidateWeather, candidateHRFit} }

const (
	metricMeanWatts    = "mean estimated watts"
	metricAutocorr     = "quality: lag-1 autocorrelation"
	metricMeanAbsDelta = "quality: mean |delta W|/s"
	metricClipBias     = "quality: clip bias (W)"
	metricHRFitRMS     = "HR-fit RMS residual (W)"
	metricEnergyError  = "energy vs device calories, relative error (%)"
)

// metricOrder is the fixed order the report prints its metric tables in.
func metricOrder() []string {
	return []string{metricMeanWatts, metricAutocorr, metricMeanAbsDelta, metricClipBias, metricHRFitRMS, metricEnergyError}
}

// metricTable is one metric's values, per candidate, over every ride that
// yielded one — mirroring dev/ascentstudy's report table, generalised to hold
// any metric rather than only a relative error against a device figure.
type metricTable struct {
	values map[string][]float64
	name   string
	order  []string
}

func newMetricTable(name string, order []string) *metricTable {
	values := make(map[string][]float64, len(order))
	for _, candidate := range order {
		values[candidate] = nil
	}

	return &metricTable{name: name, order: order, values: values}
}

func (m *metricTable) record(candidate string, value float64) {
	m.values[candidate] = append(m.values[candidate], value)
}

// summary is the median, first and third quartile of a metric's values, and
// how many rides contributed one.
type summary struct {
	rides                   int
	medianVal, q1Val, q3Val float64
}

func summarize(values []float64) summary {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	s := summary{rides: len(sorted)}
	if len(sorted) == 0 {
		return s
	}
	s.medianVal = percentileOf(sorted, 0.5)
	s.q1Val = percentileOf(sorted, 0.25)
	s.q3Val = percentileOf(sorted, 0.75)

	return s
}

// percentileOf reads a fractional position from an already-sorted slice by
// linear interpolation between the two nearest ranks.
func percentileOf(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := fraction * float64(len(sorted)-1)
	lower, upper := int(math.Floor(position)), int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}
	weight := position - float64(lower)

	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

func (m *metricTable) String() string {
	var b strings.Builder
	width := 14
	for _, name := range m.order {
		width = max(width, len(name))
	}
	fmt.Fprintf(&b, "%s\n", m.name)
	fmt.Fprintf(&b, "  %-*s %6s %10s %10s %10s\n", width, "candidate", "rides", "median", "q1", "q3")
	for _, name := range m.order {
		s := summarize(m.values[name])
		fmt.Fprintf(&b, "  %-*s %6d %10.2f %10.2f %10.2f\n", width, name, s.rides, s.medianVal, s.q1Val, s.q3Val)
	}

	return b.String()
}

// report is windstudy's aggregate result: one table per metric, the corpus
// counts a reader needs to judge them by, the wind fit's own agreement with
// the forecast, and the meter-ride comparison. No ride identifier, date,
// position, heading series or altitude value is ever held here.
type report struct {
	metrics              map[string]*metricTable
	meterRMS             map[string][]float64
	speedDiffMS          []float64
	angleDiffDeg         []float64
	totalRides           int
	skippedNoTrack       int
	skippedHasPowerMeter int
	skippedNoHeartRate   int
	skippedNoWeather     int
	skippedNoCalories    int
	meterRides           int
}

func newReport() *report {
	metrics := make(map[string]*metricTable, len(metricOrder()))
	for _, name := range metricOrder() {
		metrics[name] = newMetricTable(name, candidateOrder())
	}

	return &report{metrics: metrics, meterRMS: make(map[string][]float64, len(candidateOrder()))}
}

// recordCandidate folds one ride's one candidate series into every metric
// table it can feed: mean watts, its own quality diagnostics, the HR-fit RMS
// of its own series (comparable across every candidate on the handover's own
// criterion), and the energy cross-check where the ride's calories are known.
func (r *report) recordCandidate(
	name string, track []measure.Sample, estimates []measure.Estimate, quality measure.Quality,
	blocks []validBlock, kcal float64, hasCalories bool,
) {
	if mean, ok := measure.MeanEstimate(estimates); ok {
		r.metrics[metricMeanWatts].record(name, mean)
	}
	r.metrics[metricAutocorr].record(name, quality.Autocorrelation1)
	r.metrics[metricMeanAbsDelta].record(name, quality.MeanAbsDeltaWattsPerSecond)
	r.metrics[metricClipBias].record(name, quality.ClipBiasWatts)
	if rms, ok := hrFitRMS(estimates, blocks); ok {
		r.metrics[metricHRFitRMS].record(name, rms)
	}
	if hasCalories {
		if relError, ok := energyRelErrorPercent(kilojoules(track, estimates), kcal); ok {
			r.metrics[metricEnergyError].record(name, relError)
		}
	}
}

func (r *report) String() string {
	var b strings.Builder
	fmt.Fprintln(&b, "wind candidates: none, recorded weather, and a wind vector fitted from heart rate")
	for _, name := range metricOrder() {
		b.WriteString(r.metrics[name].String())
		fmt.Fprintln(&b)
	}
	fmt.Fprintf(&b, "rides: total=%d skipped_no_track=%d skipped_has_power_meter=%d skipped_no_heart_rate=%d skipped_no_weather=%d skipped_no_calories=%d\n",
		r.totalRides, r.skippedNoTrack, r.skippedHasPowerMeter, r.skippedNoHeartRate, r.skippedNoWeather, r.skippedNoCalories)

	fmt.Fprintln(&b)
	if len(r.speedDiffMS) == 0 {
		fmt.Fprintln(&b, "hrfit vs the weather forecast at ride midpoint: no ride had both a fit and a forecast")
	} else {
		speed := summarize(r.speedDiffMS)
		angle := summarize(r.angleDiffDeg)
		fmt.Fprintf(&b, "hrfit vs the weather forecast at ride midpoint (%d rides)\n", speed.rides)
		fmt.Fprintf(&b, "  speed diff (m/s):  median %6.2f q1 %6.2f q3 %6.2f\n", speed.medianVal, speed.q1Val, speed.q3Val)
		fmt.Fprintf(&b, "  angle diff (deg):  median %6.1f q1 %6.1f q3 %6.1f\n", angle.medianVal, angle.q1Val, angle.q3Val)
	}

	fmt.Fprintln(&b)
	fmt.Fprintf(&b, "rides with a meter: %d\n", r.meterRides)
	if r.meterRides > 0 {
		fmt.Fprintln(&b, "  RMS of estimate vs measured power (W), mean across those rides:")
		for _, name := range candidateOrder() {
			if mean, ok := meanOf(r.meterRMS[name]); ok {
				fmt.Fprintf(&b, "    %-8s %6.1f\n", name, mean)
			}
		}
	}

	return b.String()
}

func meanOf(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}

	return sum / float64(len(values)), true
}

// validBlock is one heart-rate block that held at least half its own length
// in known estimate samples: the pair (mean heart rate, indices to average an
// estimate series over) the wind fit and the HR-RMS diagnostic both need.
type validBlock struct {
	indices []int
	meanHR  float64
}

// buildBlocks cuts track into fixed-length blocks from its first sample's
// time, and keeps only those with at least half the block held by a known
// estimate and at least one heart-rate reading — one Sample per second is
// this service's own recording rate, so a count of known samples doubles as
// the seconds held.
func buildBlocks(track []measure.Sample, estimates []measure.Estimate, heartRate []trainingload.Sample, block time.Duration) []validBlock {
	if len(track) == 0 {
		return nil
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
	hrSumByBlock := map[int]float64{}
	hrCountByBlock := map[int]int{}
	for _, reading := range heartRate {
		key := int(reading.At.Sub(start) / block)
		hrSumByBlock[key] += reading.Value
		hrCountByBlock[key]++
	}

	keys := make([]int, 0, len(indicesByBlock))
	for key := range indicesByBlock {
		keys = append(keys, key)
	}
	sort.Ints(keys)

	halfBlockSeconds := block.Seconds() / 2
	blocks := make([]validBlock, 0, len(keys))
	for _, key := range keys {
		indices := indicesByBlock[key]
		if float64(len(indices)) < halfBlockSeconds {
			continue
		}
		count := hrCountByBlock[key]
		if count == 0 {
			continue
		}
		blocks = append(blocks, validBlock{indices: indices, meanHR: hrSumByBlock[key] / float64(count)})
	}

	return blocks
}

// blockMeans is one candidate series' mean heart rate and mean estimated
// watts over each of blocks, in the same order.
func blockMeans(estimates []measure.Estimate, blocks []validBlock) (heartRates, watts []float64) {
	heartRates = make([]float64, len(blocks))
	watts = make([]float64, len(blocks))
	for index, block := range blocks {
		sum := 0.0
		for _, sampleIndex := range block.indices {
			sum += estimates[sampleIndex].Watts
		}
		watts[index] = sum / float64(len(block.indices))
		heartRates[index] = block.meanHR
	}

	return heartRates, watts
}

// linearFit is the least-squares line P = a*HR + b through the given points,
// or the flat line at the mean P for points that share one heart rate.
func linearFit(heartRates, watts []float64) (a, b float64) {
	n := float64(len(heartRates))
	var sumX, sumY, sumXY, sumXX float64
	for index := range heartRates {
		sumX += heartRates[index]
		sumY += watts[index]
		sumXY += heartRates[index] * watts[index]
		sumXX += heartRates[index] * heartRates[index]
	}
	denominator := n*sumXX - sumX*sumX
	if denominator == 0 {
		return 0, sumY / n
	}
	a = (n*sumXY - sumX*sumY) / denominator
	b = (sumY - a*sumX) / n

	return a, b
}

// rmsResidual is the RMS of watts minus the fitted line's prediction from
// heartRates.
func rmsResidual(heartRates, watts []float64, a, b float64) float64 {
	sumSquares := 0.0
	for index := range heartRates {
		residual := watts[index] - (a*heartRates[index] + b)
		sumSquares += residual * residual
	}

	return math.Sqrt(sumSquares / float64(len(heartRates)))
}

// hrFitRMS is the linear HR->power fit's RMS residual over one candidate
// series' block means, or false for fewer than two blocks — nothing to fit a
// line through.
func hrFitRMS(estimates []measure.Estimate, blocks []validBlock) (float64, bool) {
	if len(blocks) < 2 {
		return 0, false
	}
	heartRates, watts := blockMeans(estimates, blocks)
	a, b := linearFit(heartRates, watts)

	return rmsResidual(heartRates, watts, a, b), true
}

// fitWindFromHeartRate grid-searches candidate wind vectors, scoring each by
// its own hrFitRMS, and returns the vector with the lowest RMS.
//
// See docs/references/power-estimation-handover.md §6.
func fitWindFromHeartRate(
	track []measure.Sample, headingDeg []float64, totalMassKG float64, blocks []validBlock,
	speedsMS []float64, directionStepDeg float64,
) (bestSpeedMS, bestDirectionDeg, bestRMS float64, ok bool) {
	if len(blocks) < 2 {
		return 0, 0, 0, false
	}
	bestRMS = math.Inf(1)
	headwind := make([]float64, len(track))
	for _, speed := range speedsMS {
		for direction := 0.0; direction < 360; direction += directionStepDeg {
			for index, heading := range headingDeg {
				headwind[index] = speed * math.Cos((direction-heading)*math.Pi/180)
			}
			estimates, _, estOK := measure.EstimateSeriesWithWind(track, totalMassKG, headwind)
			if !estOK {
				continue
			}
			rms, rmsOK := hrFitRMS(estimates, blocks)
			if rmsOK && rms < bestRMS {
				bestRMS, bestSpeedMS, bestDirectionDeg, ok = rms, speed, direction, true
			}
			// A calm candidate has no direction to search: every direction
			// gives the same headwind of zero.
			if speed == 0 {
				break
			}
		}
	}

	return bestSpeedMS, bestDirectionDeg, bestRMS, ok
}

// coordinateOf is a positioned track point's location, for a bearing between
// two of them.
func coordinateOf(point activity.TrackPoint) measure.Coordinate {
	return measure.Coordinate{Latitude: point.Latitude, Longitude: point.Longitude}
}

// bearingDegrees is the forward great-circle bearing from a to b, in degrees
// from north.
func bearingDegrees(a, b measure.Coordinate) float64 {
	lat1 := a.Latitude * math.Pi / 180
	lat2 := b.Latitude * math.Pi / 180
	deltaLongitude := (b.Longitude - a.Longitude) * math.Pi / 180
	y := math.Sin(deltaLongitude) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(deltaLongitude)

	return math.Mod(math.Atan2(y, x)*180/math.Pi+360, 360)
}

// headingSeries is the bearing at each of a ride's positioned track points:
// from the previous point to this one, with the first sample taking the
// second's, since it has no previous point of its own.
func headingSeries(track []activity.TrackPoint) []float64 {
	headings := make([]float64, len(track))
	for index := 1; index < len(track); index++ {
		headings[index] = bearingDegrees(coordinateOf(track[index-1]), coordinateOf(track[index]))
	}
	if len(track) > 1 {
		headings[0] = headings[1]
	}

	return headings
}

// absDuration is the non-negative magnitude of a duration.
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}

	return d
}

// alignHeadings assigns each of a ride's estimate samples the heading of the
// nearest positioned track point by time. Both slices are already
// time-ordered, so one forward-only pointer over positions finds each sample's
// nearest point without ever walking backward.
func alignHeadings(track []measure.Sample, positions []activity.TrackPoint, headings []float64) []float64 {
	aligned := make([]float64, len(track))
	position := 0
	for index, sample := range track {
		for position < len(positions)-1 &&
			absDuration(positions[position+1].Time.Sub(sample.At)) <= absDuration(positions[position].Time.Sub(sample.At)) {
			position++
		}
		aligned[index] = headings[position]
	}

	return aligned
}

// weatherAt is the weather step whose interval contains t, or the nearest one
// by time where none does. False only for an empty series.
func weatherAt(steps []activity.WeatherStep, t time.Time) (activity.WeatherStep, bool) {
	if len(steps) == 0 {
		return activity.WeatherStep{}, false
	}
	for _, step := range steps {
		if !t.Before(step.At) && t.Before(step.At.Add(step.Step)) {
			return step, true
		}
	}
	nearest := steps[0]
	nearestGap := absDuration(t.Sub(nearest.At))
	for _, step := range steps[1:] {
		if gap := absDuration(t.Sub(step.At)); gap < nearestGap {
			nearest, nearestGap = step, gap
		}
	}

	return nearest, true
}

// weatherHeadwind is the recorded weather's headwind component at each of a
// ride's samples: w = windSpeedMS * cos(windFrom - heading), positive into
// the wind. False for a ride with no recorded weather at all.
//
// See docs/references/power-estimation-handover.md §6.
func weatherHeadwind(track []measure.Sample, headingDeg []float64, steps []activity.WeatherStep) ([]float64, bool) {
	if len(steps) == 0 {
		return nil, false
	}
	headwind := make([]float64, len(track))
	for index, sample := range track {
		step, _ := weatherAt(steps, sample.At)
		windMS := step.WindSpeedKMH / 3.6
		headwind[index] = windMS * math.Cos((step.WindDirectionDegrees-headingDeg[index])*math.Pi/180)
	}

	return headwind, true
}

// angularDifference is the magnitude of the smallest signed angle from a to
// b, in degrees, wrapped into [0, 180].
func angularDifference(a, b float64) float64 {
	diff := math.Mod(b-a+540, 360) - 180

	return math.Abs(diff)
}

// kilojoules is the energy an estimated series spent, integrating each known
// sample's watts over the elapsed time since the sample before it.
func kilojoules(track []measure.Sample, estimates []measure.Estimate) float64 {
	kj := 0.0
	for index := 1; index < len(estimates); index++ {
		if !estimates[index].Known {
			continue
		}
		elapsedSeconds := track[index].At.Sub(track[index-1].At).Seconds()
		if elapsedSeconds <= 0 {
			continue
		}
		kj += estimates[index].Watts * elapsedSeconds / 1000
	}

	return kj
}

// energyRelErrorPercent is the estimated energy's relative error against the
// device's own reported calories, positive where the estimate over-reports.
// False where the device reported no calories to check against.
//
// See docs/references/power-estimation-handover.md §10.
func energyRelErrorPercent(kj, kcalDevice float64) (float64, bool) {
	if kcalDevice <= 0 {
		return 0, false
	}
	estimatedKcal := kj / 4.184 / 0.22

	return (estimatedKcal/kcalDevice - 1) * 100, true
}

// rmsVsMeasured is the RMS of a candidate estimate series against a ride's own
// measured power, matched by nearest time. False for a ride with no power
// series or no known estimate.
func rmsVsMeasured(track []measure.Sample, estimates []measure.Estimate, power []trainingload.Sample) (float64, bool) {
	if len(power) == 0 {
		return 0, false
	}
	sumSquares := 0.0
	count := 0
	position := 0
	for index, estimate := range estimates {
		if !estimate.Known {
			continue
		}
		at := track[index].At
		for position < len(power)-1 && absDuration(power[position+1].At.Sub(at)) <= absDuration(power[position].At.Sub(at)) {
			position++
		}
		residual := estimate.Watts - power[position].Value
		sumSquares += residual * residual
		count++
	}
	if count == 0 {
		return 0, false
	}

	return math.Sqrt(sumSquares / float64(count)), true
}

// massForTarget is one target's total system mass: its rider profile's, the
// way internal/activity/derive.go reads it, or the -mass flag where no
// profile gives one. Cached per target, since RecordedRides orders every
// target's rides together.
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

// midpoint is a track's midpoint in time, halfway between its first and last
// sample.
func midpoint(track []measure.Sample) time.Time {
	first, last := track[0].At, track[len(track)-1].At

	return first.Add(last.Sub(first) / 2)
}

// study reads every recorded ride the store holds and scores the three wind
// candidates (see the package doc comment) against the handover's own
// diagnostics.
func study(
	ctx context.Context, store *sqlite.Store, minSamples int, block time.Duration,
	speedsMS []float64, directionStepDeg, massFlagKG float64,
) (*report, error) {
	rides, err := store.RecordedRides(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing recorded rides: %w", err)
	}

	result := newReport()
	massCache := map[string]float64{}

	for _, ride := range rides {
		result.totalRides++
		samples, samplesErr := store.ActivityRideSamples(ctx, ride.TargetID, ride.WorkoutID)
		if samplesErr != nil {
			return nil, fmt.Errorf("reading a ride's samples: %w", samplesErr)
		}
		if len(samples.Track) < minSamples {
			result.skippedNoTrack++
			continue
		}

		positions, trackErr := store.ActivityTrack(ctx, ride.TargetID, ride.WorkoutID)
		if trackErr != nil {
			return nil, fmt.Errorf("reading a ride's track: %w", trackErr)
		}
		if len(positions) == 0 {
			result.skippedNoTrack++
			continue
		}

		mass, massErr := massForTarget(ctx, store, ride.TargetID, massFlagKG, massCache)
		if massErr != nil {
			return nil, massErr
		}

		noneEstimates, noneQuality, ok := measure.EstimateSeries(samples.Track, mass)
		if !ok {
			result.skippedNoTrack++
			continue
		}

		hasMeter := len(samples.Power) > 0
		if hasMeter {
			result.skippedHasPowerMeter++
		}

		weatherSteps, weatherErr := store.ActivityWeatherSteps(ctx, ride.TargetID, ride.WorkoutID)
		if weatherErr != nil {
			return nil, fmt.Errorf("reading a ride's weather: %w", weatherErr)
		}
		hasWeather := len(weatherSteps) > 0
		if !hasWeather {
			result.skippedNoWeather++
		}

		hasHeartRate := len(samples.HeartRate) > 0
		if !hasHeartRate {
			result.skippedNoHeartRate++
		}

		kcal, hasCalories, caloriesErr := store.ActivityCaloriesAccum(ctx, ride.TargetID, ride.WorkoutID)
		if caloriesErr != nil {
			return nil, fmt.Errorf("reading a ride's calories: %w", caloriesErr)
		}
		if !hasCalories {
			result.skippedNoCalories++
		}

		headings := alignHeadings(samples.Track, positions, headingSeries(positions))
		blocks := buildBlocks(samples.Track, noneEstimates, samples.HeartRate, block)

		type candidateResult struct {
			name      string
			estimates []measure.Estimate
			quality   measure.Quality
		}
		candidates := []candidateResult{{candidateNone, noneEstimates, noneQuality}}

		if hasWeather {
			if headwind, whOK := weatherHeadwind(samples.Track, headings, weatherSteps); whOK {
				if weatherEstimates, weatherQuality, wOK := measure.EstimateSeriesWithWind(samples.Track, mass, headwind); wOK {
					candidates = append(candidates, candidateResult{candidateWeather, weatherEstimates, weatherQuality})
				}
			}
		}

		if hasHeartRate {
			bestSpeed, bestDirection, _, fitOK := fitWindFromHeartRate(samples.Track, headings, mass, blocks, speedsMS, directionStepDeg)
			if fitOK {
				headwind := make([]float64, len(samples.Track))
				for index, heading := range headings {
					headwind[index] = bestSpeed * math.Cos((bestDirection-heading)*math.Pi/180)
				}
				if hrfitEstimates, hrfitQuality, hOK := measure.EstimateSeriesWithWind(samples.Track, mass, headwind); hOK {
					candidates = append(candidates, candidateResult{candidateHRFit, hrfitEstimates, hrfitQuality})
				}
				if hasWeather {
					if step, wOK := weatherAt(weatherSteps, midpoint(samples.Track)); wOK {
						result.speedDiffMS = append(result.speedDiffMS, bestSpeed-step.WindSpeedKMH/3.6)
						result.angleDiffDeg = append(result.angleDiffDeg, angularDifference(bestDirection, step.WindDirectionDegrees))
					}
				}
			}
		}

		if hasMeter {
			result.meterRides++
			for _, candidate := range candidates {
				if rms, ok := rmsVsMeasured(samples.Track, candidate.estimates, samples.Power); ok {
					result.meterRMS[candidate.name] = append(result.meterRMS[candidate.name], rms)
				}
			}

			continue
		}

		for _, candidate := range candidates {
			result.recordCandidate(candidate.name, samples.Track, candidate.estimates, candidate.quality, blocks, kcal, hasCalories)
		}
	}

	return result, nil
}
