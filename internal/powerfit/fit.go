package powerfit

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
		estimates, _, ok := measure.EstimateSeriesWith(ride.Samples, ride.TotalMassKG, coefficients, nil)
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

// scaleBounds is how far from the built-in road bicycle a scale is allowed to
// travel. A rider on gravel is roughly 1.5 times draggier; anything beyond
// this range is a fit that has run away rather than a bicycle.
const (
	minScale = 0.4
	maxScale = 3.0
)

// dragAreaBounds and rollingBounds span the handover's own tables, from a
// rider on aerobars to one sitting up, and from slicks on asphalt to loose
// gravel. See docs/references/power-estimation-handover.md §2.
const (
	minDragArea = 0.15
	maxDragArea = 0.75
	minRolling  = 0.002
	maxRolling  = 0.025
)

// FitScale fits the one factor that scales both coefficients together,
// leaving the ratio between them at base's. Gravity and inertia are
// untouched: the mass is the rider's own and the grade and acceleration were
// recorded, so neither has anything to fit.
//
// This is the fit a corpus of one rider's rides can actually support. The two
// coefficients move the model along nearly the same direction — over the
// operator's own rides the rolling and aerodynamic bases correlate at 0.93 —
// so their sum is well determined and their split is not.
func FitScale(base measure.Coefficients, rides []Ride) (measure.Coefficients, Result, bool) {
	best, ok := minimise1D(minScale, maxScale, func(scale float64) (float64, bool) {
		result, evalOK := Evaluate(rides, scaledBy(base, scale))
		return result.RMSWatts, evalOK
	})
	if !ok {
		return measure.Coefficients{}, Result{}, false
	}
	coefficients := scaledBy(base, best)
	result, evalOK := Evaluate(rides, coefficients)

	return coefficients, result, evalOK
}

// FitPair fits the drag area and the rolling resistance independently.
func FitPair(rides []Ride) (measure.Coefficients, Result, bool) {
	drag, rolling, ok := minimise2D(
		minDragArea, maxDragArea, minRolling, maxRolling,
		func(dragArea, rollingResistance float64) (float64, bool) {
			result, evalOK := Evaluate(rides, measure.Coefficients{
				DragArea: dragArea, RollingResistance: rollingResistance,
			})
			return result.RMSWatts, evalOK
		})
	if !ok {
		return measure.Coefficients{}, Result{}, false
	}
	coefficients := measure.Coefficients{DragArea: drag, RollingResistance: rolling}
	result, evalOK := Evaluate(rides, coefficients)

	return coefficients, result, evalOK
}

// scaledBy moves both coefficients by one factor, keeping the ratio between
// them.
func scaledBy(base measure.Coefficients, scale float64) measure.Coefficients {
	return measure.Coefficients{
		DragArea:          base.DragArea * scale,
		RollingResistance: base.RollingResistance * scale,
	}
}

// minimise1D sweeps a grid and refines into the interval around its best
// point, returning the argument that scored lowest. Every refinement stays
// inside the bounds it started from: those are what a bicycle can be, not a
// hint, and a search that leaves them has stopped answering the question.
func minimise1D(low, high float64, score func(float64) (float64, bool)) (float64, bool) {
	floor, ceiling := low, high
	best, bestScore, found := 0.0, math.Inf(1), false
	for range searchRounds {
		step := (high - low) / searchSteps
		for index := range searchSteps + 1 {
			at := low + step*float64(index)
			if scored, ok := score(at); ok && scored < bestScore {
				best, bestScore, found = at, scored, true
			}
		}
		if !found {
			return 0, false
		}
		low, high = max(best-step, floor), min(best+step, ceiling)
	}

	return best, found
}

// minimise2D is minimise1D over two arguments at once.
func minimise2D(
	lowA, highA, lowB, highB float64, score func(a, b float64) (float64, bool),
) (bestA, bestB float64, found bool) {
	floorA, ceilingA, floorB, ceilingB := lowA, highA, lowB, highB
	bestScore := math.Inf(1)
	for range searchRounds {
		stepA, stepB := (highA-lowA)/searchSteps, (highB-lowB)/searchSteps
		for indexA := range searchSteps + 1 {
			for indexB := range searchSteps + 1 {
				a, b := lowA+stepA*float64(indexA), lowB+stepB*float64(indexB)
				if scored, ok := score(a, b); ok && scored < bestScore {
					bestA, bestB, bestScore, found = a, b, scored, true
				}
			}
		}
		if !found {
			return 0, 0, false
		}
		lowA, highA = max(bestA-stepA, floorA), min(bestA+stepA, ceilingA)
		lowB, highB = max(bestB-stepB, floorB), min(bestB+stepB, ceilingB)
	}

	return bestA, bestB, found
}
