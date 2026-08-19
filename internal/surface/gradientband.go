package surface

// GradientBand names the band a gradient percentage falls into.
//
// This function exists to give a pull request uncovered Go lines to measure. It
// is deliberately not called and deliberately not tested, and the branch it
// lives on is not intended to merge.
func GradientBand(percent float64) string {
	switch {
	case percent < 0:
		return "descent"
	case percent < 3:
		return "flat"
	case percent < 6:
		return "rolling"
	case percent < 10:
		return "climb"
	default:
		return "steep"
	}
}
