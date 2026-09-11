package main

import (
	"math"

	"github.com/nobbs/domestique/internal/measure"
)

// A Block is one span of an unmetered ride the fit is scored over: which of
// the ride's samples it holds, and the power the rider's own heart rate says
// they were producing across them.
type Block struct {
	Indices     []int
	TargetWatts float64
}

// A Ride is one unmetered ride as the fit sees it: the samples the model runs
// over, the mass it runs at, and the blocks it is scored against.
type Ride struct {
	Samples     []measure.Sample
	Blocks      []Block
	TotalMassKG float64
}

// A Result is how far a candidate pair of coefficients leaves the model from
// what the rider's heart rate says. BiasWatts is signed and is the figure
// that matters for a level: a fit can sit right on average and still be
// noisy, but one that is 30 W low on average is wrong.
type Result struct {
	RMSWatts  float64
	BiasWatts float64
	Blocks    int
}

// Evaluate scores one candidate pair over every block of every ride. False
// where no ride yielded a block the model could estimate across.
func Evaluate(rides []Ride, coefficients measure.Coefficients) (Result, bool) {
	var sumSquares, sumSigned float64
	count := 0
	for _, ride := range rides {
		estimates, ok := measure.EstimateSeries(ride.Samples, ride.TotalMassKG, coefficients)
		if !ok {
			continue
		}
		for _, block := range ride.Blocks {
			modelled, blockOK := meanOverBlock(estimates, block.Indices)
			if !blockOK {
				continue
			}
			residual := modelled - block.TargetWatts
			sumSquares += residual * residual
			sumSigned += residual
			count++
		}
	}
	if count == 0 {
		return Result{}, false
	}

	return Result{
		RMSWatts:  math.Sqrt(sumSquares / float64(count)),
		BiasWatts: sumSigned / float64(count),
		Blocks:    count,
	}, true
}

// meanOverBlock is the mean of the estimates the model worked out across a
// block, and false where it worked out none of them.
func meanOverBlock(estimates []measure.Estimate, indices []int) (float64, bool) {
	total, count := 0.0, 0
	for _, index := range indices {
		if index < 0 || index >= len(estimates) || !estimates[index].Known {
			continue
		}
		total += estimates[index].Watts
		count++
	}
	if count == 0 {
		return 0, false
	}

	return total / float64(count), true
}

// The search grid. Three rounds of a coarse sweep, each refining into the
// interval either side of the round before it, is enough for a surface this
// smooth, and unlike a gradient step it cannot walk off a flat spot.
const (
	searchSteps  = 16
	searchRounds = 3
)

// dragAreaBounds span the handover's own table, from a rider on aerobars to
// one sitting up. See docs/references/power-estimation-handover.md §2.
const (
	minDragArea = 0.15
	maxDragArea = 0.75
)

// FitDragArea fits the drag area at a stated rolling resistance. Crr is what a
// tyre and a surface set and can be looked up; CdA is posture, clothing and
// luggage, which nothing but the rider's own rides can name. Gravity and
// inertia are untouched: the mass is the rider's own and the grade and
// acceleration were recorded, so neither has anything to fit.
//
// This is the fit a corpus of one rider's rides can actually support. The two
// coefficients move the model along nearly the same direction — over the
// operator's own rides the rolling and aerodynamic bases correlate at 0.93 —
// so their sum is well determined and their split is not.
//
// A fit that lands on the edge of the range it searched is refused rather
// than reported: the handover's own recovery test found the HR objective
// collapses CdA to the scan's own edge whatever the rider's true value is
// (docs/references/power-estimation-handover.md §8), because the objective
// has no interior minimum to find. A boundary result is that failure's own
// signature, not a rider's number.
func FitDragArea(rollingResistance float64, rides []Ride) (measure.Coefficients, Result, bool) {
	best, ok := minimise1D(minDragArea, maxDragArea, func(dragArea float64) (float64, bool) {
		result, evalOK := Evaluate(rides, measure.Coefficients{
			DragArea: dragArea, RollingResistance: rollingResistance,
		})
		return result.RMSWatts, evalOK
	})
	if !ok || best <= minDragArea || best >= maxDragArea {
		return measure.Coefficients{}, Result{}, false
	}
	coefficients := measure.Coefficients{DragArea: best, RollingResistance: rollingResistance}
	result, evalOK := Evaluate(rides, coefficients)

	return coefficients, result, evalOK
}

// minimise1D sweeps a grid and refines into the interval around its best
// point, returning the argument that scored lowest. Every refinement stays
// inside the bounds it started from: those are what a bicycle can be, not a
// hint, and a search that leaves them has stopped answering the question.
func minimise1D(low, high float64, score func(float64) (float64, bool)) (float64, bool) {
	floor, ceiling := low, high
	best, found := 0.0, false
	for range searchRounds {
		// Reset each round: a refined round's grid sits inside the interval
		// the round before it narrowed to, so its own points must be judged
		// against each other, not held to the coarser grid's best.
		bestScore := math.Inf(1)
		roundFound := false
		step := (high - low) / searchSteps
		for index := range searchSteps + 1 {
			at := low + step*float64(index)
			if scored, ok := score(at); ok && scored < bestScore {
				best, bestScore, roundFound = at, scored, true
			}
		}
		if !roundFound {
			return 0, false
		}
		found = true
		low, high = max(best-step, floor), min(best+step, ceiling)
	}

	return best, found
}
