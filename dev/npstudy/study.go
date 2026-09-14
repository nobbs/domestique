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

// smoothingWindows are the spans the estimate is averaged over before its
// normalized power is taken again, to see how much of its excess is noise.
var smoothingWindows = [...]time.Duration{10 * time.Second, 60 * time.Second} //nolint:gochecknoglobals // the study's fixed design, read-only.

// Drag areas the trainer fit may choose between: the handover table's aerobars
// to sitting up. A fit on either edge is refused, since it found no minimum.
const (
	minDragArea = 0.15
	maxDragArea = 0.75
)

// stravaMatchWindow is how far a Strava activity's start may sit from a
// ride's first sample and still be the same ride.
const stravaMatchWindow = 2 * time.Minute

func openStore(ctx context.Context, path string) (*sqlite.Store, error) {
	var key [32]byte // this tool writes no encrypted column
	store, err := sqlite.Open(ctx, path, key)
	if err != nil {
		return nil, fmt.Errorf("opening state: %w", err)
	}

	return store, nil
}

// normalizedPower is the production definition, trainingload.PowerLoad's,
// applied to any series: the threshold it is given does not touch NP.
func normalizedPower(readings []measure.Reading) (float64, bool) {
	power, ok := trainingload.PowerLoad(readings, 1)
	return power.NormalizedWatts, ok
}

// smoothed is each reading replaced by the mean of the readings within window
// behind it, never reaching across a recording gap.
func smoothed(readings []measure.Reading, window time.Duration) []measure.Reading {
	out := make([]measure.Reading, len(readings))
	start, sum := 0, 0.0
	for index, reading := range readings {
		if index > 0 && reading.At.Sub(readings[index-1].At) > measure.DefaultMaxGap {
			start, sum = index, 0
		}
		sum += reading.Value
		for reading.At.Sub(readings[start].At) >= window {
			sum -= readings[start].Value
			start++
		}
		out[index] = measure.Reading{At: reading.At, Value: sum / float64(index-start+1)}
	}

	return out
}

// estimatedReadings is the estimate as a power series over the samples it
// was worked out for, coasting zeros included, the way a meter records them.
func estimatedReadings(track []measure.Sample, estimates []measure.Estimate) []measure.Reading {
	readings := make([]measure.Reading, 0, len(track))
	for index := range track {
		if estimates[index].Known {
			readings = append(readings, measure.Reading{At: track[index].At, Value: estimates[index].Watts})
		}
	}

	return readings
}

// seriesFigures are one power series' plain and normalized figures.
type seriesFigures struct {
	mean     float64 // over held seconds, coasting included
	np       float64
	smoothNP [len(smoothingWindows)]float64
}

func figuresOf(readings []measure.Reading) (seriesFigures, bool) {
	figures := seriesFigures{}
	figures.mean, _ = measure.MeanHeld(readings, measure.DefaultMaxGap)
	np, ok := normalizedPower(readings)
	if !ok || figures.mean <= 0 {
		return figures, false
	}
	figures.np = np
	for index, window := range smoothingWindows {
		if figures.smoothNP[index], ok = normalizedPower(smoothed(readings, window)); !ok {
			return figures, false
		}
	}

	return figures, true
}

// An outdoorRide is a road ride's estimate at the rider's saved bicycle.
type outdoorRide struct {
	start          time.Time
	estimate       seriesFigures
	pedallingWatts float64
}

// A trainerRide is a metered ride with a virtual track under it: the meter's
// figures, and the samples the drag area is fitted over.
type trainerRide struct {
	start time.Time
	track []measure.Sample
	meter seriesFigures
	mass  float64
}

// A stravaActivity is one row of a Strava export: when it started and the
// average power Strava gives it, its own estimate where no meter rode.
type stravaActivity struct {
	start        time.Time
	averageWatts float64
}

// A report holds ratios only; no ride identifier, date or position.
type report struct {
	ratios         map[string][]float64
	order          []string
	bicycle        measure.Coefficients
	trainerBicycle measure.Coefficients
	outdoor        int
	trainer        int
	skipped        int
	stravaRows     int
	trainerFitOK   bool
}

func (r *report) add(name string, numerator, denominator float64) {
	if denominator <= 0 || numerator <= 0 {
		return
	}
	if _, seen := r.ratios[name]; !seen {
		r.order = append(r.order, name)
	}
	r.ratios[name] = append(r.ratios[name], numerator/denominator)
}

func quantile(values []float64, q float64) float64 {
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	index := min(int(math.Ceil(q*float64(len(sorted))))-1, len(sorted)-1)

	return sorted[max(index, 0)]
}

func (r *report) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "corpus: %d outdoor rides, %d trainer rides with a track, %d skipped", r.outdoor, r.trainer, r.skipped)
	if r.stravaRows > 0 {
		fmt.Fprintf(&b, ", %d Strava activities read", r.stravaRows)
	}
	fmt.Fprintf(&b, "\noutdoor estimated at the saved bicycle: CdA %.3f, Crr %.4f\n", r.bicycle.DragArea, r.bicycle.RollingResistance)
	if r.trainerFitOK {
		fmt.Fprintf(&b, "trainer estimated at the fitted bicycle: CdA %.3f, Crr %.4f\n", r.trainerBicycle.DragArea, r.trainerBicycle.RollingResistance)
	} else {
		fmt.Fprintln(&b, "trainer bicycle: unavailable (no ride to fit, or the fit reached its bound)")
	}
	fmt.Fprintf(&b, "\n  %-44s %6s %7s %7s %7s\n", "ratio", "rides", "q1", "median", "q3")
	for _, name := range r.order {
		values := r.ratios[name]
		fmt.Fprintf(&b, "  %-44s %6d %7.3f %7.3f %7.3f\n", name, len(values),
			quantile(values, 0.25), quantile(values, 0.5), quantile(values, 0.75))
	}

	return b.String()
}

// study reads every recorded ride, works out the estimate's figures over the
// outdoor ones, fits a drag area to the trainer rides' meter, and joins both
// to the Strava export where one was given.
func study(
	ctx context.Context, store *sqlite.Store, target string, massFlag float64, minSamples int, zwiftCrr float64,
	strava []stravaActivity,
) (*report, error) {
	rides, err := store.RecordedRides(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing recorded rides: %w", err)
	}
	filtered := rides[:0]
	for _, ride := range rides {
		if target == "" || ride.TargetID == target {
			filtered = append(filtered, ride)
		}
	}
	rides = filtered
	result := &report{ratios: map[string][]float64{}, stravaRows: len(strava)}
	if len(rides) == 0 {
		return result, nil
	}
	for _, ride := range rides {
		if ride.TargetID != rides[0].TargetID {
			return nil, errors.New("the database holds several targets: name one with -target")
		}
	}
	mass, bicycle, err := riderOf(ctx, store, rides[0].TargetID, massFlag)
	if err != nil {
		return nil, err
	}
	result.bicycle = bicycle

	var outdoor []outdoorRide
	var trainer []trainerRide
	for _, ride := range rides {
		samples, samplesErr := store.ActivityRideSamples(ctx, ride.TargetID, ride.WorkoutID)
		if samplesErr != nil {
			return nil, fmt.Errorf("reading a ride's samples: %w", samplesErr)
		}
		if len(samples.Track) < minSamples {
			result.skipped++
			continue
		}
		if len(samples.Power) > 0 {
			meter, ok := figuresOf(samples.Power)
			if !ok {
				result.skipped++
				continue
			}
			trainer = append(trainer, trainerRide{start: samples.Track[0].At, track: samples.Track, meter: meter, mass: mass})
			continue
		}
		estimates, ok := measure.EstimateSeries(samples.Track, mass, bicycle)
		if !ok {
			result.skipped++
			continue
		}
		figures, figuresOK := figuresOf(estimatedReadings(samples.Track, estimates))
		pedalling, _, pedallingOK := measure.PedallingMean(samples.Track, estimates)
		if !figuresOK || !pedallingOK {
			result.skipped++
			continue
		}
		outdoor = append(outdoor, outdoorRide{start: samples.Track[0].At, estimate: figures, pedallingWatts: pedalling})
	}
	result.outdoor, result.trainer = len(outdoor), len(trainer)

	for _, ride := range outdoor {
		result.add("outdoor estimate NP / pedalling mean", ride.estimate.np, ride.pedallingWatts)
		result.add("outdoor estimate NP / all-seconds mean", ride.estimate.np, ride.estimate.mean)
		result.add("outdoor estimate NP 10 s smoothed / mean", ride.estimate.smoothNP[0], ride.estimate.mean)
		result.add("outdoor estimate NP 60 s smoothed / mean", ride.estimate.smoothNP[1], ride.estimate.mean)
	}

	result.trainerBicycle, result.trainerFitOK = fitTrainerDragArea(trainer, zwiftCrr)
	for _, ride := range trainer {
		result.add("trainer meter NP / meter mean", ride.meter.np, ride.meter.mean)
		result.add("trainer meter NP 60 s smoothed / meter mean", ride.meter.smoothNP[1], ride.meter.mean)
		if !result.trainerFitOK {
			continue
		}
		estimates, ok := measure.EstimateSeries(ride.track, ride.mass, result.trainerBicycle)
		if !ok {
			continue
		}
		figures, figuresOK := figuresOf(estimatedReadings(ride.track, estimates))
		if !figuresOK {
			continue
		}
		result.add("trainer estimate mean / meter mean (fit)", figures.mean, ride.meter.mean)
		result.add("trainer estimate NP / estimate mean", figures.np, figures.mean)
		result.add("trainer estimate NP / meter NP", figures.np, ride.meter.np)
		result.add("trainer estimate NP 10 s smoothed / meter NP", figures.smoothNP[0], ride.meter.np)
		result.add("trainer estimate NP 60 s smoothed / meter NP", figures.smoothNP[1], ride.meter.np)
	}

	for _, ride := range trainer {
		if watts, ok := stravaWattsAt(strava, ride.start); ok {
			result.add("strava average / trainer meter mean (join)", watts, ride.meter.mean)
		}
	}
	for _, ride := range outdoor {
		if watts, ok := stravaWattsAt(strava, ride.start); ok {
			result.add("strava average / outdoor estimate NP", watts, ride.estimate.np)
			result.add("strava average / outdoor estimate mean", watts, ride.estimate.mean)
			result.add("strava average / outdoor pedalling mean", watts, ride.pedallingWatts)
		}
	}

	return result, nil
}

// stravaWattsAt is the average power of the Strava activity that started
// nearest a ride's first sample, within stravaMatchWindow.
func stravaWattsAt(activities []stravaActivity, start time.Time) (float64, bool) {
	best, found := stravaMatchWindow+1, false
	watts := 0.0
	for _, activity := range activities {
		offset := activity.start.Sub(start).Abs()
		if offset <= stravaMatchWindow && offset < best && activity.averageWatts > 0 {
			best, watts, found = offset, activity.averageWatts, true
		}
	}

	return watts, found
}

// fitTrainerDragArea is the drag area at a fixed rolling resistance that
// brings the estimate's all-seconds mean nearest the meter's over every
// trainer ride, by a grid refined three times.
func fitTrainerDragArea(rides []trainerRide, rollingResistance float64) (measure.Coefficients, bool) {
	if len(rides) == 0 {
		return measure.Coefficients{}, false
	}
	score := func(dragArea float64) float64 {
		coefficients := measure.Coefficients{DragArea: dragArea, RollingResistance: rollingResistance}
		total := 0.0
		for _, ride := range rides {
			estimates, ok := measure.EstimateSeries(ride.track, ride.mass, coefficients)
			if !ok {
				continue
			}
			mean, _ := measure.MeanHeld(estimatedReadings(ride.track, estimates), measure.DefaultMaxGap)
			total += (mean - ride.meter.mean) * (mean - ride.meter.mean)
		}
		return total
	}
	const steps, rounds = 16, 3
	low, high, best := minDragArea, maxDragArea, 0.0
	for range rounds {
		step, bestScore := (high-low)/steps, math.Inf(1)
		for index := range steps + 1 {
			at := low + step*float64(index)
			if scored := score(at); scored < bestScore {
				best, bestScore = at, scored
			}
		}
		low, high = max(best-step, minDragArea), min(best+step, maxDragArea)
	}
	if best <= minDragArea || best >= maxDragArea {
		return measure.Coefficients{}, false
	}

	return measure.Coefficients{DragArea: best, RollingResistance: rollingResistance}, true
}

// riderOf is one target's total system mass and saved bicycle, read the way
// internal/activity/derive.go reads them; -mass stands in for a missing mass.
func riderOf(ctx context.Context, store *sqlite.Store, targetID string, massFlag float64) (float64, measure.Coefficients, error) {
	mass, bicycle := massFlag, measure.DefaultCoefficients()
	subject, err := store.TargetOwner(ctx, targetID)
	if err != nil {
		return 0, bicycle, fmt.Errorf("reading a target's owner: %w", err)
	}
	if subject != "" {
		profile, profileErr := store.RiderProfile(ctx, subject)
		if profileErr != nil {
			return 0, bicycle, fmt.Errorf("reading a rider profile: %w", profileErr)
		}
		if inputs := trainingload.InputsOf(&profile); inputs.TotalMassKG > 0 {
			mass = inputs.TotalMassKG
		}
		if profile.DragAreaM2.Set && profile.RollingResistance.Set {
			bicycle = measure.Coefficients{DragArea: profile.DragAreaM2.Number, RollingResistance: profile.RollingResistance.Number}
		}
	}
	if mass <= 0 {
		return 0, bicycle, errors.New("the target has neither a profile mass nor a -mass flag")
	}

	return mass, bicycle, nil
}
