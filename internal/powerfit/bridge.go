// Package powerfit calibrates the estimated-power model's two fittable
// coefficients against power a rider really produced.
//
// A rider with no meter on their road bicycle may still own rides that
// measured power — on a trainer, where the speed and the distance are a
// simulation but the power and the heart rate are not. Heart rate is the only
// quantity both kinds of ride record honestly, so it is the bridge: what the
// rider's heart does at a known power indoors says what power they were
// producing at the same heart rate outdoors, and the model's coefficients are
// then whatever makes it agree.
package powerfit

import "math"

// A MeasuredBlock is one span of a ride that carried a power meter: the mean
// heart rate the rider held over it and the mean power they really produced.
// Long enough that the heart has caught up with the legs — a few minutes, not
// a few seconds.
type MeasuredBlock struct {
	HeartRateBPM  float64
	WattsMeasured float64
}

// A Bridge is the rider's own heart-rate-to-power line, and how well their
// own measured rides actually sit on it. ResidualRMSWatts is the honest width
// of every conclusion drawn through it: a coefficient fitted against a target
// this noisy cannot be sharper than the target is.
type Bridge struct {
	WattsPerBPM      float64
	InterceptWatts   float64
	ResidualRMSWatts float64
	Blocks           int
}

// FitBridge is the least-squares line through the blocks a meter recorded.
// False for fewer than three blocks, or for blocks that share one heart rate
// and so name no slope.
func FitBridge(blocks []MeasuredBlock) (Bridge, bool) {
	if len(blocks) < 3 {
		return Bridge{}, false
	}
	n := float64(len(blocks))
	var sumX, sumY, sumXY, sumXX float64
	for _, block := range blocks {
		sumX += block.HeartRateBPM
		sumY += block.WattsMeasured
		sumXY += block.HeartRateBPM * block.WattsMeasured
		sumXX += block.HeartRateBPM * block.HeartRateBPM
	}
	denominator := n*sumXX - sumX*sumX
	if denominator == 0 {
		return Bridge{}, false
	}
	bridge := Bridge{Blocks: len(blocks)}
	bridge.WattsPerBPM = (n*sumXY - sumX*sumY) / denominator
	bridge.InterceptWatts = (sumY - bridge.WattsPerBPM*sumX) / n

	sumSquares := 0.0
	for _, block := range blocks {
		residual := block.WattsMeasured - bridge.WattsAt(block.HeartRateBPM)
		sumSquares += residual * residual
	}
	bridge.ResidualRMSWatts = math.Sqrt(sumSquares / n)

	return bridge, true
}

// WattsAt is the power this rider's heart rate says they were producing.
// Clamped at nought: the line is fitted over the heart rates a rider actually
// rides at and says nothing sensible below them.
func (b Bridge) WattsAt(heartRateBPM float64) float64 {
	return max(b.WattsPerBPM*heartRateBPM+b.InterceptWatts, 0)
}
