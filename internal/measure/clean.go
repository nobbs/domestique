package measure

// CapHeartRate replaces readings above maxBPM with a line interpolated
// between the plausible readings either side, the way Intervals.icu does: a
// spike past the rider's maximum is a sensor fault, and the rate either side
// is what the heart was doing. A run of spikes reaching either end of the
// series takes its one plausible neighbour's value. A maxBPM that is not
// positive, or a series with no plausible reading at all, comes back
// unchanged. Always returns a new slice; the input is never mutated.
//
// See docs/specs/measurement.md §Sensor cleaning.
func CapHeartRate(readings []Reading, maxBPM float64) []Reading {
	cleaned := make([]Reading, len(readings))
	copy(cleaned, readings)
	if maxBPM <= 0 {
		return cleaned
	}

	lastPlausible := -1
	for index := 0; index < len(cleaned); index++ {
		if cleaned[index].Value <= maxBPM {
			lastPlausible = index

			continue
		}
		runStart := index
		for index+1 < len(cleaned) && cleaned[index+1].Value > maxBPM {
			index++
		}
		nextPlausible := index + 1
		if nextPlausible == len(cleaned) {
			nextPlausible = -1
		}
		fillRun(cleaned, runStart, index, lastPlausible, nextPlausible)
	}

	return cleaned
}

// fillRun sets the value of every reading in [runStart, runEnd] to the
// interpolation between the plausible readings at before and after; either
// bound may be absent (-1), in which case the run copies the other one.
func fillRun(readings []Reading, runStart, runEnd, before, after int) {
	switch {
	case before == -1 && after == -1:
		return
	case before == -1:
		for index := runStart; index <= runEnd; index++ {
			readings[index].Value = readings[after].Value
		}
	case after == -1:
		for index := runStart; index <= runEnd; index++ {
			readings[index].Value = readings[before].Value
		}
	default:
		span := readings[after].At.Sub(readings[before].At).Seconds()
		if span <= 0 {
			// A clock that did not advance gives no line to draw; the earlier reading stands.
			for index := runStart; index <= runEnd; index++ {
				readings[index].Value = readings[before].Value
			}

			return
		}
		for index := runStart; index <= runEnd; index++ {
			fraction := readings[index].At.Sub(readings[before].At).Seconds() / span
			readings[index].Value = readings[before].Value + (readings[after].Value-readings[before].Value)*fraction
		}
	}
}

// ClampPower holds readings above maxWatts at maxWatts and reports how many
// it touched. A maxWatts that is not positive clamps nothing. Always returns
// a new slice.
//
// See docs/specs/measurement.md §Sensor cleaning.
func ClampPower(readings []Reading, maxWatts float64) (cleaned []Reading, touched int) {
	cleaned = make([]Reading, len(readings))
	copy(cleaned, readings)
	if maxWatts <= 0 {
		return cleaned, 0
	}

	for index := range cleaned {
		if cleaned[index].Value > maxWatts {
			cleaned[index].Value = maxWatts
			touched++
		}
	}

	return cleaned, touched
}
