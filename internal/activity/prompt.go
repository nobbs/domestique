package activity

import (
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/trainingload"
)

// promptInstruction is revision PromptRevision's framing; change the two together.
const promptInstruction = `You are a cycling coach reading one ride a rider has finished, with the rider's recent history and the ride's own recording.
Answer with the JSON document the schema describes and nothing else. Plain text inside every string: no Markdown, no lists inside strings. The summary is two or three short paragraphs. Use only the figures below; where a figure you would want is absent, say so under data_gaps rather than guessing. Timestamps are in the rider's local time zone unless a column says otherwise.`

// The two ways a prompt names the training load it carries.
const (
	loadNow       = "The rider's training load now"
	loadOnRideDay = "The rider's training load at the end of this ride's day"
)

// maximumTimeseriesRows bounds the timeseries section to six hours of 5 s buckets.
const (
	maximumTimeseriesRows = 4320
	timeseriesStep        = 5 * time.Second
	minimumWorkoutSeconds = 30
	targetWattTolerance   = 1.0
	minimumSplitRows      = 2
)

// bundle is everything one ride's prompt is built from, gathered by
// analyseOne before it asks. Never the track's positions, the weather
// provider's raw document, or who the rider is.
type bundle struct {
	at            time.Time
	thisAttempts  map[int]ClimbAttempt
	location      *time.Location
	day           *trainingload.Day
	routeNames    map[route.Key]string
	match         *RouteMatch
	matches       map[int64]RouteMatch
	weather       *WeatherSummary
	metrics       *RideMetrics
	recentByID    map[int64]RideMetrics
	routeName     string
	loadLabel     string
	climbs        []measure.Climb
	series        []SampleRow
	otherAttempts []StoredClimbAttempt
	recent        []Stored
	weatherSteps  []WeatherStep
	track         []TrackPoint
	indoorTypes   []int
	earlier       []Analysis
	loads         []trainingload.RideLoad
	ride          Stored
	session       Session
	profile       rider.Profile
	powerCurve    rider.PowerCurve
	indoor        bool
}

// compose renders the bundle as the plain-text sections the instruction
// refers to, in the fixed order the sections are numbered in.
func (b *bundle) compose() string {
	var prompt strings.Builder
	prompt.WriteString(promptInstruction)

	writeSection(&prompt, "Rider", b.riderLines())
	writeSection(&prompt, "This ride", b.rideSectionLines())
	writeSection(&prompt, "Derived figures", rideLines(b.metrics))
	writeCSV(&prompt, "Splits", "split_km,distance_km,moving_min,ascent_m,avg_hr,avg_power", b.splitRows())
	writeCSV(&prompt, "Climbs", "climb,length_km,gain_m,this_ride_min,this_ride_hr,this_ride_power,best_min,median_min,attempts", b.climbRows())
	writeCSV(&prompt, "Structured workout", "start_min,duration_min,target_w,actual_w,delta_pct,avg_hr", b.workoutRows())
	writeCSV(&prompt, "Weather during the ride", "time,temp_c,feels_c,wind_kmh,wind_dir_deg,precip_mm,cloud_pct,code", b.weatherStepRows())
	b.writeTimeseries(&prompt)
	writeCSV(&prompt, "Recent rides (newest first)",
		"date,indoor,distance_km,moving_h,ascent_m,tss,trimp,intensity_factor,avg_hr,avg_power,route", b.recentRows())
	b.writeTrainingLoad(&prompt)
	if len(b.earlier) > 0 {
		lines := make([]string, 0, len(b.earlier))
		for index := range b.earlier {
			lines = append(lines, earlierLine(&b.earlier[index]))
		}
		writeSection(&prompt, "What was said about this rider's earlier rides, newest first", lines)
	}

	return prompt.String()
}

// earlierLine is how one earlier analysis is quoted back to the model: its own
// headline and summary for a revision-3 row, its plain text for an older one.
func earlierLine(analysis *Analysis) string {
	date := analysis.StartedAt.Format("2006-01-02")
	if analysis.Document.Headline == "" && analysis.Document.Summary == "" {
		return date + ": " + strings.ReplaceAll(analysis.Text, "\n", " ")
	}
	summary := strings.ReplaceAll(analysis.Document.Summary, "\n", " ")
	line := fmt.Sprintf("%s: %s — %s", date, analysis.Document.Headline, summary)
	if advice := analysis.Document.NextSession.Advice; advice != "" {
		line += " Next: " + strings.ReplaceAll(advice, "\n", " ")
	}

	return line
}

func writeSection(prompt *strings.Builder, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	prompt.WriteString("\n\n" + title + ":")
	for _, line := range lines {
		prompt.WriteString("\n- " + line)
	}
}

// writeCSV renders a table section: one header row, then one row per line. A
// table with no rows is left out entirely, never emitted with a bare header.
func writeCSV(prompt *strings.Builder, title, header string, rows []string) {
	if len(rows) == 0 {
		return
	}
	prompt.WriteString("\n\n" + title + ":\n" + header)
	for _, row := range rows {
		prompt.WriteString("\n" + row)
	}
}

func profileLines(profile *rider.Profile) []string {
	var lines []string
	add := func(label string, value rider.Value, format string) {
		if value.Set {
			lines = append(lines, label+" "+fmt.Sprintf(format, value.Number))
		}
	}
	add("maximum heart rate", profile.MaxHeartRateBPM, "%.0f bpm")
	add("resting heart rate", profile.RestingHeartRateBPM, "%.0f bpm")
	add("threshold heart rate", profile.ThresholdHeartRateBPM, "%.0f bpm")
	add("functional threshold power", profile.FunctionalThresholdPowerWatts, "%.0f W")
	add("rider mass", profile.RiderMassKG, "%.1f kg")
	if bounds, ok := trainingload.BoundsFrom(profile.ThresholdHeartRateBPM.Number, profile.MaxHeartRateBPM.Number); ok {
		lines = append(lines, fmt.Sprintf("heart-rate zone bounds: %.0f, %.0f, %.0f and %.0f bpm",
			bounds[0], bounds[1], bounds[2], bounds[3]))
	}

	return lines
}

// riderLines is section 1: the profile and the all-time power curve.
func (b *bundle) riderLines() []string {
	lines := profileLines(&b.profile)
	if best := powerBestLines(&b.powerCurve); best != "" {
		lines = append(lines, "best power ever: "+best)
	}

	return lines
}

// rideSectionLines is section 2: this ride, from its listing, device session,
// route match and weather summary. Never latitude, longitude or the raw
// provider document.
func (b *bundle) rideSectionLines() []string {
	var lines []string
	add := func(line string) { lines = append(lines, line) }

	add("started at " + b.ride.StartedAt.In(b.location).Format("2006-01-02T15:04:05"))
	kind := "outdoor"
	if b.indoor {
		kind = "indoor"
	}
	add("type: " + kind)
	add("provider: " + b.ride.Provider)
	if b.ride.HasWorkout {
		add(fmt.Sprintf("Zwift workout/route: %s, completion %.0f%%", b.ride.WorkoutName, b.ride.WorkoutCompletion*100))
	}
	add(fmt.Sprintf("distance %.1f km", b.ride.DistanceMetres/1000))
	add(fmt.Sprintf("moving time %s, elapsed time %s", hoursMinutes(b.ride.MovingSeconds), hoursMinutes(b.ride.ElapsedSeconds)))
	add(fmt.Sprintf("ascent %.0f m", b.ride.AscentMetres))

	s := &b.session
	addReading := func(label string, r Reading, format string) {
		if r.Known {
			add(label + " " + fmt.Sprintf(format, r.Value))
		}
	}
	addReading("calories", s.CaloriesKcal, "%.0f kcal")
	if s.Sport != "" {
		add("device sport: " + s.Sport + "/" + s.SubSport)
	}
	addReading("average speed", s.AverageSpeedKmh, "%.1f km/h")
	addReading("maximum speed", s.MaxSpeedKmh, "%.1f km/h")
	addReading("average power", s.AveragePowerWatts, "%.0f W")
	addReading("maximum power", s.MaxPowerWatts, "%.0f W")
	addReading("normalized power (device)", s.NormalizedPowerWatts, "%.0f W")
	addReading("minimum heart rate", s.MinHeartRateBPM, "%.0f bpm")
	addReading("average heart rate", s.AverageHeartRateBPM, "%.0f bpm")
	addReading("maximum heart rate", s.MaxHeartRateBPM, "%.0f bpm")
	addReading("average cadence", s.AverageCadenceRPM, "%.0f rpm")
	addReading("maximum cadence", s.MaxCadenceRPM, "%.0f rpm")
	addReading("average temperature", s.AverageTemperatureCelsius, "%.1f °C")
	addReading("maximum temperature", s.MaxTemperatureCelsius, "%.1f °C")
	addReading("average grade", s.AverageGradePercent, "%.1f%%")
	addReading("maximum positive grade", s.MaxPositiveGradePercent, "%.1f%%")
	addReading("maximum negative grade", s.MaxNegativeGradePercent, "%.1f%%")
	addReading("minimum altitude", s.MinAltitudeMetres, "%.0f m")
	addReading("maximum altitude", s.MaxAltitudeMetres, "%.0f m")
	if line := zoneTableLine("device heart-rate zones", s.HeartRateZoneSeconds, s.HeartRateZoneHighBPM, "bpm"); line != "" {
		add(line)
	}
	if line := zoneTableLine("device power zones", s.PowerZoneSeconds, s.PowerZoneHighWatts, "W"); line != "" {
		add(line)
	}

	if b.match != nil {
		direction := b.match.Direction.String()
		name := b.routeName
		if name == "" {
			name = "unnamed route"
		}
		add(fmt.Sprintf("library route: %s, direction %s, route coverage %.0f%%, ride coverage %.0f%%",
			name, direction, b.match.RouteCoverage*100, b.match.RideCoverage*100))
	}
	if b.weather != nil {
		add(fmt.Sprintf("weather: %.1f to %.1f °C, mean wind %.1f km/h, %.1f mm precipitation, %s",
			b.weather.TemperatureMinCelsius, b.weather.TemperatureMaxCelsius,
			b.weather.WindSpeedKMH, b.weather.PrecipitationMillimetres, weatherCodeWord(b.weather.WeatherCode)))
	}

	return lines
}

// zoneTableLine renders one device-declared zone table as a single line, or
// "" when the device declared none.
func zoneTableLine(label string, seconds, highs []float64, unit string) string {
	if len(seconds) == 0 {
		return ""
	}
	parts := make([]string, 0, len(seconds))
	for index, value := range seconds {
		bound := ""
		if index < len(highs) {
			bound = fmt.Sprintf(" (≤%.0f %s)", highs[index], unit)
		}
		parts = append(parts, fmt.Sprintf("zone %d%s %.0f min", index+1, bound, value/60))
	}

	return label + ": " + strings.Join(parts, ", ")
}

func rideLines(metrics *RideMetrics) []string {
	load, averages := &metrics.Load, &metrics.Averages
	var lines []string
	if load.HasZones {
		minutes := make([]string, len(load.Zones))
		for index, seconds := range load.Zones {
			minutes[index] = fmt.Sprintf("zone %d %.0f min", index+1, seconds/60)
		}
		lines = append(lines, "time in heart-rate zones: "+strings.Join(minutes, ", "))
	}
	if load.HasTRIMP {
		lines = append(lines, fmt.Sprintf("TRIMP %.0f", load.TRIMP))
	}
	if load.HasHeartRateTSS {
		lines = append(lines, fmt.Sprintf("heart-rate TSS %.0f", load.HeartRateTSS))
	}
	if load.HasPower {
		lines = append(lines, fmt.Sprintf("normalized power %.0f W, intensity factor %.2f, power TSS %.0f",
			load.Power.NormalizedWatts, load.Power.IntensityFactor, load.Power.TSS))
	}
	if load.HasEstimatedPower && metrics.HasEstimatedPedallingShare {
		lines = append(lines, fmt.Sprintf("no power meter; estimated power while pedalling %.0f W, pedalling %.0f%% of the ride",
			load.EstimatedPowerWatts, metrics.EstimatedPedallingShare*100))
	}
	if averages.HasHeartRate {
		lines = append(lines, fmt.Sprintf("average heart rate %.0f bpm, maximum %.0f bpm",
			averages.HeartRateBPM, averages.MaxHeartRateBPM))
	}
	if averages.HasPower {
		lines = append(lines, fmt.Sprintf("average power %.0f W", averages.PowerWatts))
	}
	if averages.HasCadence {
		lines = append(lines, fmt.Sprintf("average cadence %.0f rpm", averages.CadenceRPM))
	}
	if averages.HasSpeed {
		lines = append(lines, fmt.Sprintf("maximum speed %.0f km/h", averages.MaxSpeedKmh))
	}
	if metrics.Decoupling.Known {
		lines = append(lines, fmt.Sprintf("aerobic decoupling %.1f%%", metrics.Decoupling.Percent))
	}
	if metrics.HeatDrift.Known {
		lines = append(lines, fmt.Sprintf("endurance-band heart rate %.0f bpm at %.0f °C",
			metrics.HeatDrift.HeartRateBPM, metrics.HeatDrift.TemperatureCelsius))
	}
	if bests := powerBestLines(&metrics.PowerBests); bests != "" {
		lines = append(lines, "best power this ride: "+bests)
	}
	if load.HasHeartRateCoverage {
		lines = append(lines, fmt.Sprintf("heart-rate sensor covered %.0f%% of the ride", load.HeartRateCoverage*100))
	}
	if load.HasPowerCoverage {
		lines = append(lines, fmt.Sprintf("power meter covered %.0f%% of the ride", load.PowerCoverage*100))
	}

	return lines
}

func powerBestLines(curve *rider.PowerCurve) string {
	durations := rider.PowerCurveDurations()
	parts := make([]string, 0, len(durations))
	for index, duration := range durations {
		if curve.Held[index] {
			parts = append(parts, fmt.Sprintf("%s %.0f W", formatDuration(duration), curve.Watts[index]))
		}
	}

	return strings.Join(parts, ", ")
}

func formatDuration(duration time.Duration) string {
	if duration < time.Minute {
		return fmt.Sprintf("%.0f s", duration.Seconds())
	}

	return fmt.Sprintf("%.0f min", duration.Minutes())
}

func hoursMinutes(seconds float64) string {
	total := int(seconds) / 60
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// weatherCodeWord maps a WMO weather code to the word the prompt names it by.
func weatherCodeWord(code int) string {
	switch {
	case code >= 0 && code <= 3:
		return [...]string{"clear", "mainly clear", "partly cloudy", "overcast"}[code]
	case code >= 45 && code <= 48:
		return "fog"
	case code >= 51 && code <= 67:
		return "drizzle/rain"
	case code >= 71 && code <= 77:
		return "snow"
	case code >= 80 && code <= 82:
		return "showers"
	case code >= 95 && code <= 99:
		return "thunderstorm"
	default:
		return fmt.Sprintf("code %d", code)
	}
}

// splitRows is section 4, skipped for fewer than two splits.
func (b *bundle) splitRows() []string {
	splits := Splits(b.series, analysisSplitMetres)
	if len(splits) < minimumSplitRows {
		return nil
	}
	rows := make([]string, 0, len(splits))
	cumulative := 0.0
	for _, split := range splits {
		cumulative += split.DistanceMetres
		rows = append(rows, fmt.Sprintf("%.1f,%.1f,%.1f,%.0f,%s,%s",
			cumulative/1000, split.DistanceMetres/1000, split.MovingSeconds/60, split.AscentMetres,
			readingCell(split.HeartRateBPM, "%.0f"), readingCell(split.PowerWatts, "%.0f")))
	}

	return rows
}

// climbRows is section 5: one row per route climb this ride has an attempt
// on, with best and median drawn from every other stored attempt on it.
func (b *bundle) climbRows() []string {
	if len(b.climbs) == 0 || len(b.thisAttempts) == 0 {
		return nil
	}
	byClimb := map[int][]StoredClimbAttempt{}
	for _, attempt := range b.otherAttempts {
		byClimb[attempt.ClimbIndex] = append(byClimb[attempt.ClimbIndex], attempt)
	}

	var rows []string
	for index := 0; index < len(b.climbs); index++ {
		mine, attempted := b.thisAttempts[index]
		if !attempted {
			continue
		}
		climb := &b.climbs[index]
		best, median, count := climbStats(byClimb[index])
		rows = append(rows, fmt.Sprintf("%d,%.1f,%.0f,%.1f,%s,%s,%s,%s,%d",
			index+1, climb.DistanceMetres/1000, climb.AscentMetres, mine.Seconds/60,
			climbReadingCell(mine.HasHeartRate, mine.HeartRateBPM, "%.0f"),
			climbReadingCell(mine.HasPower, mine.PowerWatts, "%.0f"),
			minutesCell(best), minutesCell(median), count))
	}

	return rows
}

func climbStats(attempts []StoredClimbAttempt) (best, median float64, count int) {
	if len(attempts) == 0 {
		return 0, 0, 0
	}
	seconds := make([]float64, len(attempts))
	for index, attempt := range attempts {
		seconds[index] = attempt.Seconds
	}
	sort.Float64s(seconds)
	middle := len(seconds) / 2
	median = seconds[middle]
	if len(seconds)%2 == 0 {
		median = (seconds[middle-1] + seconds[middle]) / 2
	}

	return seconds[0], median, len(seconds)
}

func minutesCell(seconds float64) string {
	if seconds <= 0 {
		return ""
	}

	return fmt.Sprintf("%.1f", seconds/60)
}

func climbReadingCell(known bool, value float64, format string) string {
	if !known {
		return ""
	}

	return fmt.Sprintf(format, value)
}

func readingCell(r Reading, format string) string {
	if !r.Known {
		return ""
	}

	return fmt.Sprintf(format, r.Value)
}

// workoutRows is section 6: consecutive samples grouped by their target
// power, dropped under thirty seconds.
func (b *bundle) workoutRows() []string {
	var hasTarget bool
	for index := range b.series {
		if b.series[index].TargetPowerWatts.Known {
			hasTarget = true

			break
		}
	}
	if !hasTarget {
		return nil
	}

	type interval struct {
		startIndex, endIndex int
		target               float64
	}
	var intervals []interval
	for index := range b.series {
		row := &b.series[index]
		if !row.TargetPowerWatts.Known {
			continue
		}
		if len(intervals) > 0 {
			last := &intervals[len(intervals)-1]
			if last.endIndex == index-1 && withinTolerance(b.series[last.endIndex].TargetPowerWatts.Value, row.TargetPowerWatts.Value) {
				last.endIndex = index

				continue
			}
		}
		intervals = append(intervals, interval{startIndex: index, endIndex: index, target: row.TargetPowerWatts.Value})
	}

	origin := b.series[0].Time
	var rows []string
	for _, in := range intervals {
		start, end := b.series[in.startIndex].Time, b.series[in.endIndex].Time
		duration := end.Sub(start).Seconds()
		if duration < minimumWorkoutSeconds {
			continue
		}
		var power, heartRate mean
		for index := in.startIndex; index <= in.endIndex; index++ {
			power.add(b.series[index].PowerWatts)
			heartRate.add(b.series[index].HeartRateBPM)
		}
		actual := power.reading()
		delta := ""
		actualCell := ""
		if actual.Known {
			actualCell = fmt.Sprintf("%.0f", actual.Value)
			if in.target != 0 {
				delta = fmt.Sprintf("%.0f", (actual.Value-in.target)/in.target*100)
			}
		}
		rows = append(rows, fmt.Sprintf("%.1f,%.1f,%.0f,%s,%s,%s",
			start.Sub(origin).Minutes(), duration/60, in.target, actualCell, delta, readingCell(heartRate.reading(), "%.0f")))
	}

	return rows
}

func withinTolerance(a, b float64) bool {
	delta := a - b
	if delta < 0 {
		delta = -delta
	}

	return delta <= targetWattTolerance
}

// weatherStepRows is section 7.
func (b *bundle) weatherStepRows() []string {
	rows := make([]string, 0, len(b.weatherSteps))
	for _, step := range b.weatherSteps {
		rows = append(rows, fmt.Sprintf("%s,%.1f,%.1f,%.1f,%.0f,%.1f,%.0f,%s",
			step.At.In(b.location).Format("15:04"), step.TemperatureCelsius, step.ApparentTemperatureCelsius,
			step.WindSpeedKMH, step.WindDirectionDegrees, step.PrecipitationMillimetres,
			step.CloudCoverPercent, weatherCodeWord(step.WeatherCode)))
	}

	return rows
}

// timeseriesBucket is one 5 s bucket of the ride's samples.
type timeseriesBucket struct {
	distanceKM, altitudeM                                           float64
	hasDistance, hasAltitude                                        bool
	grade, speed, heartRate, power, estPower, cadence, temp, target mean
}

// writeTimeseries is section 8: 5 s buckets, mean of known readings, last
// known value for distance and altitude, truncated at six hours.
func (b *bundle) writeTimeseries(prompt *strings.Builder) {
	if len(b.series) == 0 {
		return
	}
	speeds, _ := Series(b.series, SeriesSpeed)
	origin := b.series[0].Time
	buckets := map[int]*timeseriesBucket{}
	var order []int
	truncated := false
	limit := min(len(b.series), len(b.track))
	for index := range limit {
		row := &b.series[index]
		bucketIndex := int(row.Time.Sub(origin) / timeseriesStep)
		if bucketIndex >= maximumTimeseriesRows {
			truncated = true

			break
		}
		bucket, seen := buckets[bucketIndex]
		if !seen {
			bucket = &timeseriesBucket{}
			buckets[bucketIndex] = bucket
			order = append(order, bucketIndex)
		}
		if row.DistanceMetres.Known {
			bucket.distanceKM, bucket.hasDistance = row.DistanceMetres.Value/1000, true
		}
		track := &b.track[index]
		if track.HasAltitude {
			bucket.altitudeM, bucket.hasAltitude = track.AltitudeMetres, true
		}
		bucket.grade.add(row.GradePercent)
		if index < len(speeds) {
			bucket.speed.add(speeds[index])
		}
		bucket.heartRate.add(row.HeartRateBPM)
		bucket.power.add(row.PowerWatts)
		bucket.estPower.add(Reading{Value: track.EstimatedPowerWatts, Known: track.HasEstimatedPower})
		bucket.cadence.add(row.CadenceRPM)
		bucket.temp.add(row.TemperatureCelsius)
		bucket.target.add(row.TargetPowerWatts)
	}
	sort.Ints(order)

	var rows []string
	for _, index := range order {
		bucket := buckets[index]
		if !bucket.hasDistance && !bucket.hasAltitude && bucket.grade.count == 0 && bucket.speed.count == 0 &&
			bucket.heartRate.count == 0 && bucket.power.count == 0 && bucket.estPower.count == 0 &&
			bucket.cadence.count == 0 && bucket.temp.count == 0 && bucket.target.count == 0 {
			continue
		}
		rows = append(rows, fmt.Sprintf("%d,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s",
			index*int(timeseriesStep/time.Second),
			numberCell(bucket.hasDistance, bucket.distanceKM, "%.1f"),
			numberCell(bucket.hasAltitude, bucket.altitudeM, "%.0f"),
			readingCell(bucket.grade.reading(), "%.1f"),
			readingCell(bucket.speed.reading(), "%.1f"),
			readingCell(bucket.heartRate.reading(), "%.0f"),
			readingCell(bucket.power.reading(), "%.0f"),
			readingCell(bucket.estPower.reading(), "%.0f"),
			readingCell(bucket.cadence.reading(), "%.0f"),
			readingCell(bucket.temp.reading(), "%.1f"),
			readingCell(bucket.target.reading(), "%.0f")))
	}
	writeCSV(prompt, "Timeseries",
		"t_s,dist_km,alt_m,grade_pct,speed_kmh,hr,power,est_power,cadence,temp_c,target_w", rows)
	if truncated {
		prompt.WriteString("\n(timeseries truncated at six hours)")
	}
}

func numberCell(known bool, value float64, format string) string {
	if !known {
		return ""
	}

	return fmt.Sprintf(format, value)
}

// recentRows is section 9.
func (b *bundle) recentRows() []string {
	rows := make([]string, 0, len(b.recent))
	for index := range b.recent {
		ride := &b.recent[index]
		if ride.ID == b.ride.ID {
			continue
		}
		metrics := b.recentByID[ride.ID]
		indoor := "false"
		if isIndoorType(ride.TypeID, b.indoorTypes) {
			indoor = "true"
		}
		name := ""
		if match, matched := b.matches[ride.ID]; matched {
			name = b.routeNames[match.Key]
		}
		rows = append(rows, fmt.Sprintf("%s,%s,%.1f,%.1f,%.0f,%s,%s,%s,%s,%s,%s",
			ride.StartedAt.In(b.location).Format("2006-01-02"), indoor,
			ride.DistanceMetres/1000, ride.MovingSeconds/3600, ride.AscentMetres,
			readingCell(Reading{Value: metrics.Load.Power.TSS, Known: metrics.Load.HasPower}, "%.0f"),
			readingCell(Reading{Value: metrics.Load.TRIMP, Known: metrics.Load.HasTRIMP}, "%.0f"),
			readingCell(Reading{Value: metrics.Load.Power.IntensityFactor, Known: metrics.Load.HasPower}, "%.2f"),
			readingCell(Reading{Value: metrics.Averages.HeartRateBPM, Known: metrics.Averages.HasHeartRate}, "%.0f"),
			readingCell(Reading{Value: metrics.Averages.PowerWatts, Known: metrics.Averages.HasPower}, "%.0f"),
			name))
	}

	return rows
}

func isIndoorType(typeID int, indoorTypes []int) bool {
	return slices.Contains(indoorTypes, typeID)
}

// writeTrainingLoad is section 10: the load lines already used by revision 2,
// then the last 42 days, the last 8 weeks of zones, and the outlook.
func (b *bundle) writeTrainingLoad(prompt *strings.Builder) {
	if b.day != nil {
		writeSection(prompt, b.loadLabel, []string{
			fmt.Sprintf("TSS scale: fitness %.0f, fatigue %.0f, form %.0f", b.day.TSSFitness, b.day.TSSFatigue, b.day.TSSForm),
			fmt.Sprintf("TRIMP scale: fitness %.0f, fatigue %.0f, form %.0f", b.day.TRIMPFitness, b.day.TRIMPFatigue, b.day.TRIMPForm),
		})
	}
	days := trainingload.Timeline(b.loads, b.at, b.location)
	if len(days) > timelineWindowDays {
		days = days[len(days)-timelineWindowDays:]
	}
	timelineRows := make([]string, 0, len(days))
	for _, day := range days {
		timelineRows = append(timelineRows, fmt.Sprintf("%s,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f,%.0f",
			day.Date.Format("2006-01-02"), day.TSSLoad, day.TRIMPLoad,
			day.TSSFitness, day.TSSFatigue, day.TSSForm, day.TRIMPFitness, day.TRIMPFatigue, day.TRIMPForm))
	}
	writeCSV(prompt, "Training load, last 42 days",
		"date,day_tss,day_trimp,tss_fitness,tss_fatigue,tss_form,trimp_fitness,trimp_fatigue,trimp_form", timelineRows)

	weeks := trainingload.ZonesByWeek(b.loads, b.location)
	if len(weeks) > zonesWindowWeeks {
		weeks = weeks[len(weeks)-zonesWindowWeeks:]
	}
	weekRows := make([]string, 0, len(weeks))
	for _, week := range weeks {
		weekRows = append(weekRows, fmt.Sprintf("%s,%.0f,%.0f,%.0f,%.0f,%.0f",
			week.WeekStart.Format("2006-01-02"), week.Zones[0]/60, week.Zones[1]/60, week.Zones[2]/60,
			week.Zones[3]/60, week.Zones[4]/60))
	}
	writeCSV(prompt, "Time in zone, last 8 weeks", "week_of,z1_min,z2_min,z3_min,z4_min,z5_min", weekRows)

	if outlook, ok := trainingload.OutlookOf(days); ok {
		lines := []string{
			fmt.Sprintf("TSS: ramp %.0f/week, habitual daily load %.0f, building week %.0f-%.0f",
				outlook.TSS.RampPerWeek, outlook.TSS.HabitualDailyLoad, outlook.TSS.WeekLoadLow, outlook.TSS.WeekLoadHigh),
			fmt.Sprintf("TRIMP: ramp %.0f/week, habitual daily load %.0f, building week %.0f-%.0f",
				outlook.TRIMP.RampPerWeek, outlook.TRIMP.HabitualDailyLoad, outlook.TRIMP.WeekLoadLow, outlook.TRIMP.WeekLoadHigh),
		}
		writeSection(prompt, "Outlook", lines)
	}
}

const (
	timelineWindowDays = 42
	zonesWindowWeeks   = 8
)
