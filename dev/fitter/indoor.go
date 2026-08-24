package main

import (
	"sort"

	"github.com/nobbs/domestique/internal/surface"
)

// minIndoorHoursPerYear flags a year's heart-rate-to-power relation as thin
// in the report rather than silently using it — the issue's own third
// caution: a year with few indoor hours gives a relation that should be said
// to be thin, not quietly relied on.
const minIndoorHoursPerYear = 10.0

// hrPowerRelation is one year's heart-rate-to-power line, fitted separately
// per year because pooling years is wrong: an improving power-at-heart-rate
// trend would then read as model error rather than fitness.
type hrPowerRelation struct {
	Slope, Intercept float64
	Hours            float64
	Thin             bool
}

// hrPowerRelationByYear fits one relation per calendar year from indoor
// samples carrying both channels.
func hrPowerRelationByYear(indoor []indoorRow) map[int]hrPowerRelation {
	byYear := make(map[int][]indoorRow)
	for _, r := range indoor {
		if r.HasPower && r.HasHeartRate {
			byYear[r.Time.Year()] = append(byYear[r.Time.Year()], r)
		}
	}

	relations := make(map[int]hrPowerRelation, len(byYear))
	for year, rows := range byYear {
		hrs := make([]float64, len(rows))
		powers := make([]float64, len(rows))
		weights := make([]float64, len(rows))
		hours := 0.0
		for i, r := range rows {
			hrs[i] = r.HeartRateBPM
			powers[i] = r.PowerWatts
			weights[i] = r.DeltaSeconds
			hours += r.DeltaSeconds / 3600
		}
		slope, intercept := linearRegression(hrs, powers, weights)
		relations[year] = hrPowerRelation{Slope: slope, Intercept: intercept, Hours: hours, Thin: hours < minIndoorHoursPerYear}
	}

	return relations
}

// impliedPower evaluates this year's relation at a heart rate. Whether a
// year has a relation at all is the caller's own map-lookup concern (see
// crossCheckRide), not something this method reports.
func (r hrPowerRelation) impliedPower(heartRateBPM float64) float64 {
	return r.Intercept + r.Slope*heartRateBPM
}

// rideCrossCheck is one outdoor ride's heart-rate-implied power against
// stage B's own physics-implied power for the same ride.
type rideCrossCheck struct {
	RideID       string
	HRPower      float64
	PhysicsPower float64
}

// crossCheckRide compares one ride's mean heart-rate-implied power (from the
// indoor calibration) against a full-ride mean of the same power equation
// stage B evaluates for climbing alone, applied here across every interval
// with a valid grade — a quasi-static estimate that, unlike a true
// instantaneous model, carries no acceleration term, so it understates power
// on a ride with heavy stop-and-go riding. Stated as a limitation in the
// report rather than silently assumed away.
func crossCheckRide(
	samples []sampleRow, relations map[int]hrPowerRelation,
	crrBySurface map[surface.Kind]float64, crrOverall, cda, massKG, driveEfficiency float64,
) (rideCrossCheck, bool) {
	var hrWeighted, hrWeight float64
	var physicsWeighted, physicsWeight float64
	for i := range samples {
		s := &samples[i]
		if s.HasHeartRate {
			if relation, ok := relations[s.Time.Year()]; ok {
				hrWeighted += relation.impliedPower(s.HeartRateBPM) * s.DeltaSeconds
				hrWeight += s.DeltaSeconds
			}
		}
		if s.HasAltitude && s.Moving {
			crr := crrOverall
			if fitted, ok := crrBySurface[s.Surface]; ok && fitted > 0 {
				crr = fitted
			}
			power := climbPowerWatts(climbSample{
				MeanSpeedMPS: s.SpeedMPS, GradePercent: s.GradientPercent, AirDensity: airDensityFor(s),
			}, crr, cda, massKG, driveEfficiency)
			physicsWeighted += power * s.DeltaSeconds
			physicsWeight += s.DeltaSeconds
		}
	}

	if hrWeight == 0 || physicsWeight == 0 {
		return rideCrossCheck{}, false
	}

	return rideCrossCheck{
		RideID:       samples[0].RideID,
		HRPower:      hrWeighted / hrWeight,
		PhysicsPower: physicsWeighted / physicsWeight,
	}, true
}

// indoorCrossCheckSummary is what the report names: the median and
// correlation of physics power against heart-rate-implied power across every
// ride both could be computed for.
type indoorCrossCheckSummary struct {
	ThinYears   []int
	Rides       int
	MedianRatio float64
	Correlation float64
}

// summarizeCrossCheck reduces every ride's cross-check to one report. A ride
// with a non-positive HRPower carries no usable ratio and would also skew
// the correlation, so it is dropped from all three figures together —
// Rides, MedianRatio and Correlation all describe the same subset, rather
// than Rides counting rides the other two silently excluded.
func summarizeCrossCheck(checks []rideCrossCheck, relations map[int]hrPowerRelation) indoorCrossCheckSummary {
	ratios := make([]float64, 0, len(checks))
	physics := make([]float64, 0, len(checks))
	hr := make([]float64, 0, len(checks))
	for _, c := range checks {
		if c.HRPower <= 0 {
			continue
		}
		ratios = append(ratios, c.PhysicsPower/c.HRPower)
		physics = append(physics, c.PhysicsPower)
		hr = append(hr, c.HRPower)
	}
	if len(ratios) == 0 {
		return indoorCrossCheckSummary{}
	}
	sort.Float64s(ratios)

	var thinYears []int
	for year, r := range relations {
		if r.Thin {
			thinYears = append(thinYears, year)
		}
	}
	sort.Ints(thinYears)

	return indoorCrossCheckSummary{
		Rides:       len(ratios),
		MedianRatio: percentileOf(ratios, 0.5),
		Correlation: pearsonCorrelation(physics, hr),
		ThinYears:   thinYears,
	}
}
