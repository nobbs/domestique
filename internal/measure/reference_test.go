package measure_test

import (
	"math"
	"sort"
	"time"
)

// referencePoint is route.Point trimmed to what referenceHaversineMetres
// reads.
type referencePoint struct {
	Latitude, Longitude float64
}

// internal/route/route.go HaversineMetres, copied verbatim.
func referenceHaversineMetres(left, right referencePoint) float64 {
	latitudeDelta := (right.Latitude - left.Latitude) * math.Pi / 180
	longitudeDelta := (right.Longitude - left.Longitude) * math.Pi / 180
	leftLatitude := left.Latitude * math.Pi / 180
	rightLatitude := right.Latitude * math.Pi / 180
	chord := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*
			math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)

	const earthRadiusMetres = 6_371_000.0

	return earthRadiusMetres * 2 * math.Atan2(math.Sqrt(chord), math.Sqrt(1-chord))
}

// internal/route/route.go ElevationGainMetres, copied verbatim over a plain
// altitude slice in place of route geometry.
func referenceElevationGainMetres(altitudeMetres []float64) float64 {
	gain := 0.0
	for index := 1; index < len(altitudeMetres); index++ {
		step := altitudeMetres[index] - altitudeMetres[index-1]
		if step > 0 {
			gain += step
		}
	}

	return gain
}

// internal/route/route.go ElevationLossMetres, copied verbatim over a plain
// altitude slice in place of route geometry.
func referenceElevationLossMetres(altitudeMetres []float64) float64 {
	loss := 0.0
	for index := 1; index < len(altitudeMetres); index++ {
		step := altitudeMetres[index] - altitudeMetres[index-1]
		if step < 0 {
			loss -= step
		}
	}

	return loss
}

// internal/route/route.go MaxGradientPercent, copied verbatim over plain
// distance/altitude slices in place of route geometry (the cumulative
// distance loop included).
func referenceMaxGradientPercent(coordinates []referencePoint, altitudeMetres []float64, gradientWindowMetres float64) float64 {
	distances := make([]float64, len(coordinates))
	for index := 1; index < len(coordinates); index++ {
		distances[index] = distances[index-1] + referenceHaversineMetres(coordinates[index-1], coordinates[index])
	}

	steepest := 0.0
	trailing := 0
	for leading := 1; leading < len(coordinates); leading++ {
		for distances[leading]-distances[trailing+1] >= gradientWindowMetres {
			trailing++
		}
		span := distances[leading] - distances[trailing]
		if span < gradientWindowMetres {
			continue
		}
		rise := altitudeMetres[leading] - altitudeMetres[trailing]
		gradient := math.Abs(rise) / span * 100
		steepest = max(steepest, gradient)
	}

	return steepest
}

// referenceSample is elevation.sample, copied verbatim.
type referenceSample struct {
	distance  float64
	elevation float64
}

// internal/elevation/normalizer.go resampleElevations, adapted to take step
// distances directly instead of re-deriving them by haversine.
func referenceResampleElevations(stepDistances, altitudeMetres []float64, sampleIntervalMetres float64) []referenceSample {
	result := []referenceSample{{elevation: altitudeMetres[0]}}
	nextSample := sampleIntervalMetres
	distanceSoFar := 0.0
	for index := 1; index < len(altitudeMetres); index++ {
		segmentDistance := stepDistances[index]
		for segmentDistance > 0 && distanceSoFar+segmentDistance >= nextSample {
			ratio := (nextSample - distanceSoFar) / segmentDistance
			result = append(result, referenceSample{
				distance:  nextSample,
				elevation: altitudeMetres[index-1] + ratio*(altitudeMetres[index]-altitudeMetres[index-1]),
			})
			nextSample += sampleIntervalMetres
		}
		distanceSoFar += segmentDistance
	}

	if resultLast := result[len(result)-1]; resultLast.distance != distanceSoFar {
		result = append(result, referenceSample{distance: distanceSoFar, elevation: altitudeMetres[len(altitudeMetres)-1]})
	}

	return result
}

// internal/elevation/normalizer.go applyMovingMedian, copied verbatim.
func referenceApplyMovingMedian(samples []referenceSample, medianWindowMetres, sampleIntervalMetres float64) {
	radius := int(medianWindowMetres / sampleIntervalMetres / 2)
	elevations := make([]float64, len(samples))
	for index, sample := range samples {
		elevations[index] = sample.elevation
	}
	for index := range samples {
		start, end := max(0, index-radius), min(len(elevations), index+radius+1)
		window := append([]float64(nil), elevations[start:end]...)
		sort.Float64s(window)
		elevation := window[len(window)/2]
		samples[index].elevation = elevation
	}
}

// internal/elevation/normalizer.go applyElevations, adapted to take step
// distances directly instead of re-deriving them by haversine.
func referenceApplyElevations(stepDistances []float64, samples []referenceSample) []float64 {
	altitudes := make([]float64, len(stepDistances))
	distanceSoFar := 0.0
	sampleIndex := 0
	for index := range stepDistances {
		if index > 0 {
			distanceSoFar += stepDistances[index]
		}
		for sampleIndex+1 < len(samples) && samples[sampleIndex+1].distance <= distanceSoFar {
			sampleIndex++
		}
		left := samples[sampleIndex]
		if sampleIndex == len(samples)-1 {
			altitudes[index] = left.elevation

			continue
		}
		right := samples[sampleIndex+1]
		ratio := (distanceSoFar - left.distance) / (right.distance - left.distance)
		altitudes[index] = left.elevation + ratio*(right.elevation-left.elevation)
	}

	return altitudes
}

// internal/trainingload/load.go rollingFourthPowerMean, copied verbatim over
// a plain (time, value) series in place of trainingload.Sample.
func referenceRollingFourthPowerMean(times []time.Time, values []float64, normalizedPowerWindow, maxSampleGap time.Duration) (mean, seconds float64, ok bool) {
	total := 0.0
	weight := 0.0
	start := 0
	integral := make([]float64, len(times))
	for index := 1; index < len(times); index++ {
		held := times[index].Sub(times[index-1])
		if held <= 0 || held > maxSampleGap {
			integral[index] = integral[index-1]
			start = index

			continue
		}
		seconds += held.Seconds()
		integral[index] = integral[index-1] + values[index-1]*held.Seconds()
		for start+1 < index &&
			times[index].Sub(times[start+1]) >= normalizedPowerWindow {
			start++
		}
		span := times[index].Sub(times[start])
		if span < normalizedPowerWindow {
			continue
		}
		rolling := (integral[index] - integral[start]) / span.Seconds()
		total += math.Pow(rolling, 4) * held.Seconds()
		weight += held.Seconds()
	}
	if weight <= 0 {
		return 0, seconds, false
	}

	return total / weight, seconds, true
}

// internal/rider/best.go BestAverage, copied verbatim.
func referenceBestAverage(times []time.Time, values []float64, window, maxSampleGap time.Duration) (float64, bool) {
	if len(times) != len(values) {
		return 0, false
	}
	best, found := 0.0, false
	for begin := 0; begin < len(times); {
		end := begin + 1
		for end < len(times) && times[end].Sub(times[end-1]) <= maxSampleGap {
			end++
		}
		if mean, ok := referenceBestUnbroken(times[begin:end], values[begin:end], window); ok && (!found || mean > best) {
			best, found = mean, true
		}
		begin = end
	}

	return best, found
}

// internal/rider/best.go bestUnbroken, copied verbatim.
func referenceBestUnbroken(times []time.Time, values []float64, window time.Duration) (float64, bool) {
	if len(times) < 2 {
		return 0, false
	}
	integral := make([]float64, len(times))
	for index := 1; index < len(times); index++ {
		integral[index] = integral[index-1] + values[index-1]*times[index].Sub(times[index-1]).Seconds()
	}

	best, found := 0.0, false
	start := 0
	for end := 1; end < len(times); end++ {
		for start+1 < end && times[end].Sub(times[start+1]) >= window {
			start++
		}
		span := times[end].Sub(times[start])
		if span < window || span <= 0 {
			continue
		}
		if mean := (integral[end] - integral[start]) / span.Seconds(); !found || mean > best {
			best, found = mean, true
		}
	}

	return best, found
}
