package main

import (
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strings"
)

// gearStat is one gear identifier's contribution to the ingested corpus.
type gearStat struct {
	rides int
	hours float64
}

// deviceCadenceStat is one recording device's cadence coverage.
type deviceCadenceStat struct {
	rides       int
	withCadence int
}

// reportData is every figure the console report names, aggregated once over
// every ride this run touched.
type reportData struct {
	gear                       map[string]gearStat
	excludedByReason           map[exclusionReason]int
	cadenceByDevice            map[string]deviceCadenceStat
	timerDivergentRides        []string
	altitudeRatios             []float64
	stopAllowancePerMovingHour []float64
	samplesEmitted             int
	indoorHours                float64
	indoorRides                int
	checksumFailed             int
	totalRows                  int
	ingestedHours              float64
	ingested                   int
	ingestedDistanceMetres     float64
	ingestedMovingSeconds      float64
}

// timerDivergenceThreshold names how far a ride's own device timer time may
// differ from its elapsed time before it is worth a second look. Both are the
// device's own two answers to "how long was this ride", disagreeing only
// because one subtracts auto-paused time and the other does not; this is not
// the moving-time threshold above; it exists purely to flag rides worth a
// human's attention, and feeds no computation.
const timerDivergenceThreshold = 0.05

// buildReport aggregates every ride this run touched into the figures the
// console report names. It takes the summaries alone — never a sample or an
// indoor row — because every number here is already carried on the summary
// each ride produced.
func buildReport(rides []rideSummary) reportData {
	data := reportData{
		totalRows:        len(rides),
		excludedByReason: map[exclusionReason]int{},
		gear:             map[string]gearStat{},
		cadenceByDevice:  map[string]deviceCadenceStat{},
	}

	for i := range rides {
		ride := &rides[i]
		if ride.ChecksumFailed {
			data.checksumFailed++
		}
		if ride.Indoor {
			data.indoorRides++
			data.indoorHours += ride.ElapsedSeconds / 3600

			continue
		}
		if ride.Excluded {
			data.excludedByReason[ride.Reason]++

			continue
		}

		data.ingested++
		data.ingestedHours += ride.ElapsedSeconds / 3600
		data.samplesEmitted += ride.SampleCount
		data.ingestedDistanceMetres += ride.TotalDistanceMetres
		data.ingestedMovingSeconds += ride.MovingSeconds

		gear := data.gear[ride.Gear]
		gear.rides++
		gear.hours += ride.ElapsedSeconds / 3600
		data.gear[ride.Gear] = gear

		device := deviceKey(ride.RecordingDevice)
		stat := data.cadenceByDevice[device]
		stat.rides++
		if ride.HasCadence {
			stat.withCadence++
		}
		data.cadenceByDevice[device] = stat

		if ride.MovingSeconds > 0 {
			data.stopAllowancePerMovingHour = append(data.stopAllowancePerMovingHour,
				ride.StopAllowanceMinutesPerMovingHour)
		}
		if ride.HasAltitudeQuality && ride.DeviceAscentMetres > 0 {
			data.altitudeRatios = append(data.altitudeRatios, ride.RawRiseMetres/ride.DeviceAscentMetres)
		}
		if ride.HasDeviceTimerTime && ride.ElapsedSeconds > 0 {
			divergence := math.Abs(ride.ElapsedSeconds-ride.DeviceTimerTime.Seconds()) / ride.ElapsedSeconds
			if divergence > timerDivergenceThreshold {
				data.timerDivergentRides = append(data.timerDivergentRides, ride.RideID)
			}
		}
	}

	return data
}

func deviceKey(recordingDevice string) string {
	if recordingDevice == "" {
		return "unknown"
	}

	return recordingDevice
}

func renderReport(data *reportData) string {
	var b strings.Builder
	fmt.Fprintf(&b, "activities.csv rows:  %d\n", data.totalRows)
	fmt.Fprintf(&b, "ingested (outdoor):    %d rides, %.1f h, %d samples\n",
		data.ingested, data.ingestedHours, data.samplesEmitted)
	if data.ingestedMovingSeconds > 0 {
		// The operator's own recollection check the issue asks for: this
		// should read like their riding, not like a bug in the ingest.
		meanSpeedKMH := (data.ingestedDistanceMetres / data.ingestedMovingSeconds) * 3.6
		fmt.Fprintf(&b, "  corpus-wide weighted mean speed (moving): %.1f km/h (%.0f km over %.1f moving h)\n",
			meanSpeedKMH, data.ingestedDistanceMetres/1000, data.ingestedMovingSeconds/3600)
	}
	fmt.Fprintf(&b, "indoor (reference):    %d rides, %.1f h\n", data.indoorRides, data.indoorHours)
	fmt.Fprintf(&b, "checksum failures:     %d (still decoded, not counted as unreadable)\n", data.checksumFailed)

	b.WriteString("\nexcluded:\n")
	for _, reason := range slices.Sorted(maps.Keys(data.excludedByReason)) {
		fmt.Fprintf(&b, "  %-24s %d\n", reason, data.excludedByReason[reason])
	}

	b.WriteString("\ngear (ingested rides):\n")
	for _, name := range sortedGearNames(data.gear) {
		label := name
		if label == "" {
			label = "(untagged)"
		}
		stat := data.gear[name]
		fmt.Fprintf(&b, "  %-24s %d rides, %.1f h\n", label, stat.rides, stat.hours)
	}

	b.WriteString("\ncadence coverage by recording device:\n")
	for _, device := range slices.Sorted(maps.Keys(data.cadenceByDevice)) {
		stat := data.cadenceByDevice[device]
		percent := 0.0
		if stat.rides > 0 {
			percent = 100 * float64(stat.withCadence) / float64(stat.rides)
		}
		fmt.Fprintf(&b, "  %-24s %d/%d rides (%.0f%%)\n", device, stat.withCadence, stat.rides, percent)
	}

	fmt.Fprintf(&b, "\nstop allowance, minutes of (elapsed - moving) per moving hour, over %d rides:\n",
		len(data.stopAllowancePerMovingHour))
	b.WriteString(renderDistribution(data.stopAllowancePerMovingHour, "min/h"))

	fmt.Fprintf(&b, "\naltitude quality, raw rise / device's own reported ascent, over %d rides:\n", len(data.altitudeRatios))
	b.WriteString(renderDistribution(data.altitudeRatios, "ratio"))

	fmt.Fprintf(&b, "\nrides whose device timer time diverges from elapsed time by more than %.0f%%: %d\n",
		timerDivergenceThreshold*100, len(data.timerDivergentRides))
	if len(data.timerDivergentRides) > 0 {
		fmt.Fprintf(&b, "  %s\n", strings.Join(data.timerDivergentRides, ", "))
	}

	return b.String()
}

func sortedGearNames(gear map[string]gearStat) []string {
	names := slices.Collect(maps.Keys(gear))
	sort.Strings(names)

	return names
}

// renderDistribution reports a spread, never a single mean: a stop allowance
// or an altitude-quality ratio varies enough by ride that one figure would
// mislead about what the corpus actually looks like.
func renderDistribution(values []float64, unit string) string {
	if len(values) == 0 {
		return fmt.Sprintf("  (no rides carried a %s)\n", unit)
	}
	sorted := slices.Clone(values)
	slices.Sort(sorted)

	return fmt.Sprintf("  %s=%.3f  p25=%.3f  median=%.3f  p75=%.3f  max=%.3f\n",
		unit, sorted[0], percentile(sorted, 0.25), percentile(sorted, 0.5), percentile(sorted, 0.75), sorted[len(sorted)-1])
}

// percentile linearly interpolates between the two nearest ranks. sorted must
// already be sorted ascending and non-empty.
func percentile(sorted []float64, fraction float64) float64 {
	if len(sorted) == 1 {
		return sorted[0]
	}
	position := fraction * float64(len(sorted)-1)
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return sorted[lower]
	}

	return sorted[lower] + (sorted[upper]-sorted[lower])*(position-float64(lower))
}
