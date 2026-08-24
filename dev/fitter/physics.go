package main

import (
	"math"

	"github.com/nobbs/domestique/internal/route"
)

// This file is a self-contained copy of internal/ridemodel/model.go's
// physics branch as it stood before #240: fixed-mass, fixed-power physics
// with no route-level correction, forward-solved segment by segment. #240
// made internal/ridemodel.Predict inherently hybrid — it always blends in the
// route-calibrated linear half now — so it can no longer answer "what would
// pure physics alone predict," which "current physics" (the pre-#213 fitted
// model this benchmark evaluates the accepted hybrid against) still needs.
//
// This is a deliberate, short-lived duplication rather than a second
// exported entry point on internal/ridemodel: #241 deletes "current physics"
// and the sensor-level fitter feeding it, and this file along with them.
const (
	physicsEarthRadiusMetres    = 6_371_000.0
	physicsGradientWindowMetres = 100.0
	physicsGravityMPS2          = 9.80665
	physicsMinSolveSpeedMPS     = 0.01
	physicsMaxSolveSpeedMPS     = 30.0
	physicsBisectionIterations  = 60
	physicsMinCoastingSpeedMPS  = 1.0
)

// physicsOnlyMovingSeconds predicts a ride's moving time from fixed-mass,
// fixed-power physics alone, using the same forward-solve internal/ridemodel
// ran before #240 — see this file's own doc comment for why it is copied here
// rather than called there. crrPerSegment is the rolling resistance already
// resolved for each segment (aligned like points, one shorter): the caller
// picks the surface's Crr the same way fullCrrBySurface's callers already do,
// so this file carries no surface-classification logic of its own.
func physicsOnlyMovingSeconds(
	points []route.Point, crrPerSegment []float64, massKG, powerWatts, cdaM2 float64, config coefficientsConfig,
) float64 {
	if len(points) < 2 {
		return 0
	}

	distances := make([]float64, len(points))
	for index := 1; index < len(points); index++ {
		distances[index] = distances[index-1] + physicsHaversineMetres(points[index-1], points[index])
	}
	gradients := physicsWindowedGradientPercent(distances, points)

	var seconds float64
	for index := 1; index < len(points); index++ {
		span := distances[index] - distances[index-1]
		if span <= 0 {
			continue
		}
		crr := 0.0
		if index-1 < len(crrPerSegment) {
			crr = crrPerSegment[index-1]
		}
		speed := physicsSegmentSpeed(gradients[index], crr, massKG, powerWatts, cdaM2, config)
		seconds += span / speed
	}

	return seconds
}

func physicsHaversineMetres(left, right route.Point) float64 {
	latitudeDelta := (right.Latitude - left.Latitude) * math.Pi / 180
	longitudeDelta := (right.Longitude - left.Longitude) * math.Pi / 180
	leftLatitude := left.Latitude * math.Pi / 180
	rightLatitude := right.Latitude * math.Pi / 180
	chord := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*
			math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)

	return physicsEarthRadiusMetres * 2 * math.Atan2(math.Sqrt(chord), math.Sqrt(1-chord))
}

func physicsWindowedGradientPercent(distances []float64, points []route.Point) []float64 {
	gradients := make([]float64, len(points))
	trailing := 0
	for leading := 1; leading < len(points); leading++ {
		for distances[leading]-distances[trailing+1] >= physicsGradientWindowMetres {
			trailing++
		}
		span := distances[leading] - distances[trailing]
		if span <= 0 {
			continue
		}
		rise := *points[leading].Elevation - *points[trailing].Elevation
		gradients[leading] = rise / span * 100
	}

	return gradients
}

func physicsGradientTrig(gradientPercent float64) (sinTheta, cosTheta float64) {
	tanTheta := gradientPercent / 100
	cosTheta = 1 / math.Sqrt(1+tanTheta*tanTheta)
	sinTheta = tanTheta * cosTheta

	return sinTheta, cosTheta
}

func physicsSegmentSpeed(gradientPercent, crr, massKG, powerWatts, cdaM2 float64, config coefficientsConfig) float64 {
	sinTheta, cosTheta := physicsGradientTrig(gradientPercent)
	if gradientPercent <= config.DescentCutoffPercent {
		if speed, coasting := physicsCoastingSpeed(crr, sinTheta, cosTheta, massKG, cdaM2, config); coasting {
			return speed
		}
	}

	return physicsPoweredSpeed(crr, sinTheta, cosTheta, massKG, powerWatts, cdaM2, config)
}

func physicsPoweredSpeed(crr, sinTheta, cosTheta, massKG, powerWatts, cdaM2 float64, config coefficientsConfig) float64 {
	target := powerWatts * config.DriveEfficiency
	gravityTerm := massKG * physicsGravityMPS2 * (crr*cosTheta + sinTheta)
	residual := func(speed float64) float64 {
		return speed*(gravityTerm+0.5*config.AirDensityKGPerM3*cdaM2*speed*speed) - target
	}

	low, high := physicsMinSolveSpeedMPS, physicsMaxSolveSpeedMPS
	if residual(high) <= 0 || residual(low) >= 0 {
		return low
	}
	for range physicsBisectionIterations {
		mid := (low + high) / 2
		if residual(mid) > 0 {
			high = mid
		} else {
			low = mid
		}
	}

	return (low + high) / 2
}

func physicsCoastingSpeed(crr, sinTheta, cosTheta, massKG, cdaM2 float64, config coefficientsConfig) (speed float64, ok bool) {
	drivingForce := massKG * physicsGravityMPS2 * (-sinTheta - crr*cosTheta)
	if drivingForce <= 0 {
		return 0, false
	}

	speed = math.Sqrt(2 * drivingForce / (config.AirDensityKGPerM3 * cdaM2))
	if speed < physicsMinCoastingSpeedMPS {
		return 0, false
	}

	return min(speed, config.DescentCapMetresPerSecond), true
}
