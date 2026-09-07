package measure

import (
	"math"
	"sort"
)

// Profile is a track's altitude against distance along it, in step order.
type Profile struct {
	distanceMetres []float64
	altitudeMetres []float64
}

// ProfileOf builds a profile over a distance and an altitude series, copying
// both. It reports false for unequal lengths, fewer than two points, which
// have no step to measure a gradient over, or a distance that goes backwards,
// which is not a track in step order.
//
// See docs/specs/measurement.md §Profiles.
func ProfileOf(distanceMetres, altitudeMetres []float64) (Profile, bool) {
	if len(distanceMetres) != len(altitudeMetres) || len(distanceMetres) < 2 {
		return Profile{}, false
	}
	for index := 1; index < len(distanceMetres); index++ {
		if distanceMetres[index] < distanceMetres[index-1] {
			return Profile{}, false
		}
	}

	return Profile{
		distanceMetres: append([]float64(nil), distanceMetres...),
		altitudeMetres: append([]float64(nil), altitudeMetres...),
	}, true
}

// Len is the number of points the profile holds.
//
// See docs/specs/measurement.md §Profiles.
func (p Profile) Len() int {
	return len(p.distanceMetres)
}

// LengthMetres is the distance the profile covers: its last distance minus
// its first. Zero for the zero value.
//
// See docs/specs/measurement.md §Profiles.
func (p Profile) LengthMetres() float64 {
	if len(p.distanceMetres) == 0 {
		return 0
	}

	return p.distanceMetres[len(p.distanceMetres)-1] - p.distanceMetres[0]
}

// DistanceMetres is a copy of the profile's distance series.
//
// See docs/specs/measurement.md §Profiles.
func (p Profile) DistanceMetres() []float64 {
	return append([]float64(nil), p.distanceMetres...)
}

// AltitudeMetres is a copy of the profile's altitude series.
//
// See docs/specs/measurement.md §Profiles.
func (p Profile) AltitudeMetres() []float64 {
	return append([]float64(nil), p.altitudeMetres...)
}

// AscentMetres is the total ascent along the profile, summing the positive
// adjacent steps as stored. Raw satellite altitude noise summed over
// thousands of points inflates the total badly; only meaningful on a smoothed
// profile.
//
// See docs/specs/measurement.md §Ascent and descent.
func (p Profile) AscentMetres() float64 {
	gain := 0.0
	for index := 1; index < len(p.altitudeMetres); index++ {
		if step := p.altitudeMetres[index] - p.altitudeMetres[index-1]; step > 0 {
			gain += step
		}
	}

	return gain
}

// DescentMetres is the total descent along the profile, summing the negative
// adjacent steps as stored. Same profile, same caveat as AscentMetres.
//
// See docs/specs/measurement.md §Ascent and descent.
func (p Profile) DescentMetres() float64 {
	loss := 0.0
	for index := 1; index < len(p.altitudeMetres); index++ {
		if step := p.altitudeMetres[index] - p.altitudeMetres[index-1]; step < 0 {
			loss -= step
		}
	}

	return loss
}

// GradientsPercent measures each point's signed gradient back over the
// shortest span of at least windowMetres the profile holds before it, and
// reports that span. Index 0 is zero over a zero span.
//
// See docs/specs/measurement.md §Gradient.
func (p Profile) GradientsPercent(windowMetres float64) (gradientPercent, spanMetres []float64) {
	count := len(p.distanceMetres)
	gradientPercent = make([]float64, count)
	spanMetres = make([]float64, count)
	behind := 0
	for index := 1; index < count; index++ {
		// distances is non-decreasing, so behind+1 stays in range: the
		// difference reaches zero and the loop stops.
		for behind+1 < index && p.distanceMetres[index]-p.distanceMetres[behind+1] >= windowMetres {
			behind++
		}
		run := p.distanceMetres[index] - p.distanceMetres[behind]
		rise := p.altitudeMetres[index] - p.altitudeMetres[behind]
		if run > 0 {
			gradientPercent[index] = rise / run * 100
		}
		spanMetres[index] = run
	}

	return gradientPercent, spanMetres
}

// MaxGradientPercent is the steepest gradient in either direction over any
// span of at least windowMetres; zero where no span reaches the window.
//
// See docs/specs/measurement.md §Gradient.
func (p Profile) MaxGradientPercent(windowMetres float64) float64 {
	gradientPercent, spanMetres := p.GradientsPercent(windowMetres)
	steepest := 0.0
	for index, span := range spanMetres {
		if span < windowMetres {
			continue
		}
		steepest = max(steepest, math.Abs(gradientPercent[index]))
	}

	return steepest
}

// Resample rebuilds the profile at a fixed distance interval, linearly
// interpolating altitude between the points either side of each new sample.
// The grid starts at the profile's own first distance, and the final point
// always closes its exact length. An interval that is not positive, or the
// zero value, returns the profile as it is.
//
// See docs/specs/measurement.md §Profiles.
func (p Profile) Resample(intervalMetres float64) Profile {
	if intervalMetres <= 0 || len(p.distanceMetres) == 0 {
		return p.clone()
	}
	origin := p.distanceMetres[0]
	distances := []float64{origin}
	altitudes := []float64{p.altitudeMetres[0]}
	nextSample := origin + intervalMetres
	distanceSoFar := origin
	for index := 1; index < len(p.distanceMetres); index++ {
		segmentDistance := p.distanceMetres[index] - p.distanceMetres[index-1]
		for segmentDistance > 0 && distanceSoFar+segmentDistance >= nextSample {
			ratio := (nextSample - distanceSoFar) / segmentDistance
			distances = append(distances, nextSample)
			altitudes = append(altitudes, p.altitudeMetres[index-1]+ratio*(p.altitudeMetres[index]-p.altitudeMetres[index-1]))
			nextSample += intervalMetres
		}
		distanceSoFar += segmentDistance
	}
	if distances[len(distances)-1] != distanceSoFar {
		distances = append(distances, distanceSoFar)
		altitudes = append(altitudes, p.altitudeMetres[len(p.altitudeMetres)-1])
	}

	return Profile{distanceMetres: distances, altitudeMetres: altitudes}
}

// MedianFiltered removes isolated altitude spikes with a centred moving
// median over a window of windowMetres, on a profile already sampled every
// intervalMetres. An interval that is not positive returns the profile as it
// is.
//
// See docs/specs/measurement.md §Profiles.
func (p Profile) MedianFiltered(intervalMetres, windowMetres float64) Profile {
	if intervalMetres <= 0 {
		return p.clone()
	}
	radius := int(windowMetres / intervalMetres / 2)
	altitudes := make([]float64, len(p.altitudeMetres))
	for index := range p.altitudeMetres {
		start, end := max(0, index-radius), min(len(p.altitudeMetres), index+radius+1)
		window := append([]float64(nil), p.altitudeMetres[start:end]...)
		sort.Float64s(window)
		altitudes[index] = window[len(window)/2]
	}

	return Profile{distanceMetres: append([]float64(nil), p.distanceMetres...), altitudeMetres: altitudes}
}

func (p Profile) clone() Profile {
	return Profile{
		distanceMetres: append([]float64(nil), p.distanceMetres...),
		altitudeMetres: append([]float64(nil), p.altitudeMetres...),
	}
}

// AltitudeAt is the altitude at one distance along the profile: the last
// sample at or before it, taken as is if it is the last sample overall, else
// linearly interpolated to the next. A query before the first sample takes
// the first altitude; the zero value answers zero.
//
// See docs/specs/measurement.md §Profiles.
func (p Profile) AltitudeAt(distanceMetres float64) float64 {
	last := len(p.distanceMetres) - 1
	if last < 0 {
		return 0
	}
	if distanceMetres <= p.distanceMetres[0] {
		return p.altitudeMetres[0]
	}
	// The last sample at or before the query, found by bisection: a caller
	// walking a whole geometry asks this once per point.
	left := sort.Search(len(p.distanceMetres), func(index int) bool {
		return p.distanceMetres[index] > distanceMetres
	}) - 1
	if left == last {
		return p.altitudeMetres[last]
	}
	ratio := (distanceMetres - p.distanceMetres[left]) / (p.distanceMetres[left+1] - p.distanceMetres[left])

	return p.altitudeMetres[left] + ratio*(p.altitudeMetres[left+1]-p.altitudeMetres[left])
}
