package trainingload

import (
	"math"
	"time"
)

// The outlook's own constants: how far it projects, how wide a window counts
// as habitual, and how the build plan scales that window's daily load.
const (
	OutlookDays       = 21
	HabitualDays      = 28
	BuildFactor       = 1.2
	BuildingShareLow  = 0.03
	BuildingShareHigh = 0.08
)

// ProjectedDay is one day of a plan's projection under a constant daily load.
type ProjectedDay struct {
	Date                   time.Time
	Fitness, Fatigue, Form float64
}

// ScaleOutlook is where one scale's last day leaves the rider: how fast
// fitness is moving, the daily load that has become habitual, the week's load
// that would keep it rising, and the three plans projected from there.
type ScaleOutlook struct {
	Rest, Habitual, Build []ProjectedDay

	RampPerWeek       float64
	HabitualDailyLoad float64
	WeekLoadLow       float64
	WeekLoadHigh      float64
}

// Outlook is both scales' outlook from a timeline's last day.
type Outlook struct {
	Date       time.Time
	TSS, TRIMP ScaleOutlook
}

// OutlookOf projects a timeline's last day forward under three plans, on both
// scales. False for an empty timeline, which has no last day to project from.
func OutlookOf(days []Day) (Outlook, bool) {
	if len(days) == 0 {
		return Outlook{}, false
	}

	return Outlook{
		Date: days[len(days)-1].Date,
		TSS: scaleOutlookOf(days,
			func(day Day) float64 { return day.TSSLoad },
			func(day Day) float64 { return day.TSSFitness },
			func(day Day) float64 { return day.TSSFatigue }),
		TRIMP: scaleOutlookOf(days,
			func(day Day) float64 { return day.TRIMPLoad },
			func(day Day) float64 { return day.TRIMPFitness },
			func(day Day) float64 { return day.TRIMPFatigue }),
	}, true
}

// scaleOutlookOf reads one scale's outlook through its own load, fitness and fatigue.
func scaleOutlookOf(days []Day, load, fitness, fatigue func(Day) float64) ScaleOutlook {
	last := days[len(days)-1]
	lastFitness, lastFatigue := fitness(last), fatigue(last)

	// The fold starts at zero before the first ride, so a timeline with no day
	// seven before its last had no fitness then.
	earlierFitness := 0.0
	if len(days) >= 8 {
		earlierFitness = fitness(days[len(days)-8])
	}

	habitualLoad := 0.0
	for _, day := range days[max(0, len(days)-HabitualDays):] {
		habitualLoad += load(day)
	}
	habitualLoad /= HabitualDays

	// keep is the share of fitness a constant load leaves untouched after
	// seven days; weekly solves f7 = f0*(1+share) for the load that gets there.
	keep := math.Pow(1-1/float64(FitnessDays), 7)
	weekly := func(share float64) float64 {
		return 7 * lastFitness * (1 + share - keep) / (1 - keep)
	}

	return ScaleOutlook{
		RampPerWeek:       lastFitness - earlierFitness,
		HabitualDailyLoad: habitualLoad,
		WeekLoadLow:       weekly(BuildingShareLow),
		WeekLoadHigh:      weekly(BuildingShareHigh),
		Rest:              project(last.Date, lastFitness, lastFatigue, 0),
		Habitual:          project(last.Date, lastFitness, lastFatigue, habitualLoad),
		Build:             project(last.Date, lastFitness, lastFatigue, habitualLoad*BuildFactor),
	}
}

// project decays fitness and fatigue forward under one constant daily load,
// one day at a time, starting the day after from.
func project(from time.Time, fitness, fatigue, dailyLoad float64) []ProjectedDay {
	projected := make([]ProjectedDay, OutlookDays)
	for index := range OutlookDays {
		fitness = decay(fitness, dailyLoad, FitnessDays)
		fatigue = decay(fatigue, dailyLoad, FatigueDays)
		projected[index] = ProjectedDay{
			Date: from.AddDate(0, 0, index+1), Fitness: fitness, Fatigue: fatigue,
			Form: fitness - fatigue,
		}
	}

	return projected
}
