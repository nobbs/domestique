// The heart-rate bridge that lets a rider's bicycle numbers be checked
// against power they really produced.
//
// A rider with no meter on their road bicycle may still own rides that
// measured power — on a trainer, where the speed and the distance are a
// simulation but the power and the heart rate are not. Heart rate is the only
// quantity both kinds of ride record honestly, so it is the bridge: what the
// rider's heart does at a known power indoors says what power they were
// producing at the same heart rate outdoors, and the model's coefficients are
// then whatever makes it agree.
package main

// A MeasuredBlock is one span of a ride that carried a power meter: the mean
// heart rate the rider held over it and the mean power they really produced.
// Long enough that the heart has caught up with the legs — a few minutes, not
// a few seconds.
type MeasuredBlock struct {
	HeartRateBPM  float64
	WattsMeasured float64
}
