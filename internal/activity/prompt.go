package activity

import (
	"fmt"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
)

// promptInstruction is revision PromptRevision's framing; change the two together.
const promptInstruction = `You are a cycling coach reading one ride a rider has just finished.
Answer in plain text without Markdown, headings or lists, in two or three short paragraphs of at most 1500 characters in total:
what kind of ride it was, what it did to the rider's fitness, fatigue and form, and one recommendation for the next session.
Use only the figures below. Where a figure you would want is absent, say so rather than guessing.`

// composePrompt is the whole of what leaves the host about one ride: its
// derived figures and sensor means, the rider's profile and zone bounds, the
// training load after the ride's day and earlier analyses. Never the track,
// the weather, the provider's document or who the rider is.
func composePrompt(profile *rider.Profile, metrics *RideMetrics, day *trainingload.Day, earlier []Analysis) string {
	var prompt strings.Builder
	prompt.WriteString(promptInstruction)

	writeSection(&prompt, "Rider profile", profileLines(profile))
	writeSection(&prompt, "This ride", rideLines(metrics))
	if day != nil {
		writeSection(&prompt, "Training load at the end of the ride's day", []string{
			fmt.Sprintf("TSS scale: fitness %.0f, fatigue %.0f, form %.0f", day.TSSFitness, day.TSSFatigue, day.TSSForm),
			fmt.Sprintf("TRIMP scale: fitness %.0f, fatigue %.0f, form %.0f", day.TRIMPFitness, day.TRIMPFatigue, day.TRIMPForm),
		})
	}
	if len(earlier) > 0 {
		lines := make([]string, 0, len(earlier))
		for _, analysis := range earlier {
			lines = append(lines, strings.ReplaceAll(analysis.Text, "\n", " "))
		}
		writeSection(&prompt, "What was said about this rider's earlier rides, newest first", lines)
	}

	return prompt.String()
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
		lines = append(lines, "best power: "+bests)
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
