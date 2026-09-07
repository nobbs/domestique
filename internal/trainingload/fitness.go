package trainingload

import "time"

// The two time constants the fitness and fatigue averages are taken over, in
// days. They are the conventional pair every training application draws this
// chart with: six weeks of accumulated work against one week of it.
const (
	FitnessDays = 42
	FatigueDays = 7
)

// Day is one day of the timeline: what was ridden, and where that leaves the
// rider on each of the two scales this service keeps.
//
// Fitness is the long average of daily load, fatigue the short one, and form
// the difference: fresh above zero, buried below it.
type Day struct {
	Date time.Time
	// Load is the day's own total on each scale, zero on a day nobody rode.
	TRIMPLoad float64
	TSSLoad   float64

	TRIMPFitness float64
	TRIMPFatigue float64
	TRIMPForm    float64

	TSSFitness float64
	TSSFatigue float64
	TSSForm    float64
}

// RideLoad is one ride's contribution to the timeline: when it was, and what it
// cost on each scale.
type RideLoad struct {
	At    time.Time
	TRIMP float64
	TSS   float64
	Zones Zones
}

// LoadOf reads a ride's contribution out of its derived metrics.
//
// The stress score is the one measured from power where the ride carried a
// meter and the one measured from heart rate otherwise, so a rider's series
// stays continuous across the day a power meter arrives rather than stepping.
// An estimate worked out from the track is never either of them.
func LoadOf(at time.Time, metrics *Metrics) RideLoad {
	load := RideLoad{At: at, Zones: metrics.Zones}
	if metrics.HasTRIMP {
		load.TRIMP = metrics.TRIMP
	}
	switch {
	case metrics.HasPower:
		load.TSS = metrics.Power.TSS
	case metrics.HasHeartRateTSS:
		load.TSS = metrics.HeartRateTSS
	}

	return load
}

// Timeline folds a rider's rides into one row per day, from the day of their
// first ride to the last day asked for.
//
// A day nobody rode contributes nothing and still decays both averages, which
// is the whole point of the chart: rest is what turns work into fitness. The
// fold always starts at the first ride rather than at the start of the window
// asked about, so a window opening years into a rider's history opens at the
// fitness they had actually built rather than at nothing.
//
// rides need not be sorted. Days are the service's own, in the location given.
func Timeline(rides []RideLoad, until time.Time, location *time.Location) []Day {
	if len(rides) == 0 {
		return nil
	}
	daily := map[time.Time]RideLoad{}
	first := startOfDay(rides[0].At, location)
	for index := range rides {
		day := startOfDay(rides[index].At, location)
		if day.Before(first) {
			first = day
		}
		into := daily[day]
		into.TRIMP += rides[index].TRIMP
		into.TSS += rides[index].TSS
		for zone := range into.Zones {
			into.Zones[zone] += rides[index].Zones[zone]
		}
		daily[day] = into
	}

	last := startOfDay(until, location)
	if last.Before(first) {
		last = first
	}
	days := make([]Day, 0, int(last.Sub(first).Hours()/24)+1)
	current := Day{}
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		load := daily[day]
		current = Day{
			Date:      day,
			TRIMPLoad: load.TRIMP,
			TSSLoad:   load.TSS,

			TRIMPFitness: decay(current.TRIMPFitness, load.TRIMP, FitnessDays),
			TRIMPFatigue: decay(current.TRIMPFatigue, load.TRIMP, FatigueDays),
			TSSFitness:   decay(current.TSSFitness, load.TSS, FitnessDays),
			TSSFatigue:   decay(current.TSSFatigue, load.TSS, FatigueDays),
		}
		current.TRIMPForm = current.TRIMPFitness - current.TRIMPFatigue
		current.TSSForm = current.TSSFitness - current.TSSFatigue
		days = append(days, current)
	}

	return days
}

// decay moves an average one day towards the load that day carried, by the
// share of it one day is worth. A day of nothing therefore still moves the
// average, downwards.
func decay(average, load, days float64) float64 {
	return average + (load-average)/days
}

// WeeklyZones is how long a week held each heart-rate zone, easiest first.
type WeeklyZones struct {
	// WeekStart is the Monday the week began on, in the service's own zone.
	WeekStart time.Time
	Zones     Zones
}

// ZonesByWeek sums time in zone into the weeks the rides fell in, oldest first.
// Weeks with no ride are absent rather than present and empty: a bar chart of
// them has nothing to draw.
func ZonesByWeek(rides []RideLoad, location *time.Location) []WeeklyZones {
	weeks := map[time.Time]Zones{}
	order := []time.Time{}
	for index := range rides {
		start := startOfWeek(rides[index].At, location)
		if _, seen := weeks[start]; !seen {
			order = append(order, start)
		}
		zones := weeks[start]
		for zone := range zones {
			zones[zone] += rides[index].Zones[zone]
		}
		weeks[start] = zones
	}
	sortDays(order)
	weekly := make([]WeeklyZones, 0, len(order))
	for _, start := range order {
		weekly = append(weekly, WeeklyZones{WeekStart: start, Zones: weeks[start]})
	}

	return weekly
}

// startOfDay is midnight at the start of a moment's own local day.
func startOfDay(at time.Time, location *time.Location) time.Time {
	local := at.In(location)
	year, month, day := local.Date()

	return time.Date(year, month, day, 0, 0, 0, 0, location)
}

// startOfWeek is the Monday a moment's own local week began on, which is the
// week a training plan is written in.
func startOfWeek(at time.Time, location *time.Location) time.Time {
	day := startOfDay(at, location)
	// Go counts Sunday as zero; a training week starts on Monday.
	back := (int(day.Weekday()) + 6) % 7

	return day.AddDate(0, 0, -back)
}

func sortDays(days []time.Time) {
	for index := 1; index < len(days); index++ {
		for back := index; back > 0 && days[back].Before(days[back-1]); back-- {
			days[back], days[back-1] = days[back-1], days[back]
		}
	}
}
