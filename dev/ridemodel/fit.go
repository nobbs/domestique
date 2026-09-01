package main

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"

	"github.com/nobbs/domestique/internal/activity"
)

// decodeFITGZ reads a gzipped FIT file the way a Strava export stores one.
func decodeFITGZ(path string) (decodedActivity, error) {
	raw, err := readGzip(path)
	if err != nil {
		return decodedActivity{}, err
	}

	decoded, err := activity.DecodeFIT(raw)
	if err != nil {
		return decodedActivity{}, fmt.Errorf("decoding activity FIT: %w", err)
	}

	return decodedFIT(decoded), nil
}

func readGzip(path string) ([]byte, error) {
	file, err := os.Open(path) //nolint:gosec // The path is composed from the operator's own -export flag and activities.csv's Filename column.
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer closeFile(file)

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("opening gzip stream in %s: %w", path, err)
	}
	defer closeFile(gzipReader)

	raw, err := io.ReadAll(gzipReader)
	if err != nil {
		return nil, fmt.Errorf("decompressing %s: %w", path, err)
	}

	return raw, nil
}

func decodedFIT(fit activity.FIT) decodedActivity {
	records := make([]point, len(fit.Records))
	for i, record := range fit.Records {
		records[i] = point{
			Time: record.Time, CadenceRPM: record.CadenceRPM, HeartRateBPM: record.HeartRateBPM,
			Latitude: record.Latitude, Longitude: record.Longitude, PowerWatts: record.PowerWatts,
			AltitudeMetres: record.AltitudeMetres, TemperatureCelsius: record.TemperatureCelsius,
			DistanceMetres: record.DistanceMetres, HasCadence: record.HasCadence,
			HasTemperatureCelsius: record.HasTemperatureCelsius, HasDistance: record.HasDistance,
			HasPower: record.HasPower, HasAltitude: record.HasAltitude, HasHeartRate: record.HasHeartRate,
			HasPosition: record.HasPosition,
		}
	}

	return decodedActivity{
		RecordingDevice: fit.RecordingDevice, Records: records, TotalTimerTime: fit.TotalTimerTime,
		TotalElapsedTime: fit.TotalElapsedTime, TotalAscentMetres: fit.TotalAscentMetres,
		ChecksumFailed: fit.ChecksumFailed, HasTotalTimerTime: fit.HasTotalTimerTime,
		HasTotalElapsedTime: fit.HasTotalElapsedTime, HasTotalAscent: fit.HasTotalAscent,
	}
}
