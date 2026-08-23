package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// writeCorpus writes the three output tables under dir: the outdoor sample
// table, the per-ride summary, and the indoor reference table — kept in its
// own file the outdoor corpus is never joined to.
func writeCorpus(dir string, samples []sample, indoor []indoorSample, rides []rideSummary) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("creating output directory: %w", err)
	}
	if err := writeSamples(filepath.Join(dir, "samples.csv"), samples); err != nil {
		return err
	}
	if err := writeIndoorSamples(filepath.Join(dir, "indoor.csv"), indoor); err != nil {
		return err
	}
	if err := writeRideSummaries(filepath.Join(dir, "rides.csv"), rides); err != nil {
		return err
	}

	return nil
}

func writeSamples(path string, samples []sample) error {
	return writeCSVFile(path, []string{
		"ride_id", "time", "delta_seconds", "interval_distance_m", "speed_mps", "gradient_percent",
		"altitude_m", "has_altitude", "cadence_rpm", "has_cadence", "temperature_c", "has_temperature",
		"latitude", "longitude", "has_position", "moving", "derived",
	}, len(samples), func(i int) []string {
		s := samples[i]

		return []string{
			s.RideID, s.Time.Format(time.RFC3339), formatFloat(s.DeltaSeconds), formatFloat(s.IntervalDistanceMetres),
			formatFloat(s.SpeedMetresPerSecond), formatFloat(s.GradientPercent),
			formatFloat(s.AltitudeMetres), formatBool(s.HasAltitude),
			formatFloat(s.CadenceRPM), formatBool(s.HasCadence),
			formatFloat(s.TemperatureCelsius), formatBool(s.HasTemperatureCelsius),
			formatFloat(s.Latitude), formatFloat(s.Longitude), formatBool(s.HasPosition),
			formatBool(s.MovingFilter), formatBool(s.Derived),
		}
	})
}

func writeIndoorSamples(path string, samples []indoorSample) error {
	return writeCSVFile(path, []string{
		"ride_id", "time", "delta_seconds", "power_w", "has_power",
		"heart_rate_bpm", "has_heart_rate", "cadence_rpm", "has_cadence",
	}, len(samples), func(i int) []string {
		s := samples[i]

		return []string{
			s.RideID, s.Time.Format(time.RFC3339), formatFloat(s.DeltaSeconds),
			formatFloat(s.PowerWatts), formatBool(s.HasPower),
			formatFloat(s.HeartRateBPM), formatBool(s.HasHeartRate),
			formatFloat(s.CadenceRPM), formatBool(s.HasCadence),
		}
	})
}

func writeRideSummaries(path string, rides []rideSummary) error {
	return writeCSVFile(path, []string{
		"ride_id", "date", "type", "gear", "source_format", "recording_device",
		"indoor", "excluded", "reason",
		"sample_count", "moving_seconds", "elapsed_seconds", "total_distance_m", "has_cadence",
		"checksum_failed", "derived",
		"raw_rise_m", "device_ascent_m", "has_altitude_quality",
		"stop_allowance_min_per_moving_hour",
		"strava_moving_seconds", "device_timer_seconds", "has_device_timer_time",
	}, len(rides), func(i int) []string {
		r := rides[i]

		return []string{
			r.RideID, r.Date.Format(time.RFC3339), r.Type, r.Gear, r.Device, r.RecordingDevice,
			formatBool(r.Indoor), formatBool(r.Excluded), string(r.Reason),
			strconv.Itoa(r.SampleCount), formatFloat(r.MovingSeconds), formatFloat(r.ElapsedSeconds),
			formatFloat(r.TotalDistanceMetres), formatBool(r.HasCadence),
			formatBool(r.ChecksumFailed), formatBool(r.Derived),
			formatFloat(r.RawRiseMetres), formatFloat(r.DeviceAscentMetres), formatBool(r.HasAltitudeQuality),
			formatFloat(r.StopAllowanceMinutesPerMovingHour),
			formatFloat(r.StravaMovingTime.Seconds()), formatFloat(r.DeviceTimerTime.Seconds()), formatBool(r.HasDeviceTimerTime),
		}
	})
}

// writeCSVFile writes a header and n rows, each rendered by row(i). Writing
// always happens, even for zero rows, so a corpus with an empty half — no
// indoor rides in an export, say — still leaves a file the fitter's tooling
// can open rather than one it has to first check for.
func writeCSVFile(path string, header []string, n int, row func(i int) []string) (err error) {
	// The corpus carries a raw GPS position and time series, so it is written
	// readable to the operator alone rather than at os.Create's umask-subject
	// default.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600) //nolint:gosec // The path is composed from the operator's own -out flag and this package's own fixed table names.
	if err != nil {
		return fmt.Errorf("creating %s: %w", path, err)
	}
	defer func() {
		err = errors.Join(err, file.Close())
	}()

	writer := csv.NewWriter(file)
	if err := writer.Write(header); err != nil {
		return fmt.Errorf("writing %s header: %w", path, err)
	}
	for i := range n {
		if err := writer.Write(row(i)); err != nil {
			return fmt.Errorf("writing %s row %d: %w", path, i, err)
		}
	}
	writer.Flush()
	if flushErr := writer.Error(); flushErr != nil {
		return fmt.Errorf("flushing %s: %w", path, flushErr)
	}

	return nil
}

func formatFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

func formatBool(v bool) string {
	return strconv.FormatBool(v)
}
