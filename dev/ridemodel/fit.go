package main

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/basetype"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
)

// decodeFITGZ reads a gzipped FIT file the way a Strava export stores one. Zwift
// writes FIT files whose CRC does not validate. A file is always decoded with the
// checksum ignored, but is first tried with it enforced, so a checksum failure is
// reported under its own name rather than as "unreadable".
func decodeFITGZ(path string) (decodedActivity, error) {
	raw, err := readGzip(path)
	if err != nil {
		return decodedActivity{}, err
	}

	checksumFailed := false
	if _, strictErr := decodeFIT(raw); strictErr != nil {
		if !errors.Is(strictErr, decoder.ErrCRCChecksumMismatch) {
			return decodedActivity{}, fmt.Errorf("decoding FIT: %w", strictErr)
		}
		checksumFailed = true
	}

	activity, err := decodeFIT(raw, decoder.WithIgnoreChecksum())
	if err != nil {
		return decodedActivity{}, fmt.Errorf("decoding FIT with checksum ignored: %w", err)
	}
	activity.ChecksumFailed = checksumFailed

	return activity, nil
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

func decodeFIT(raw []byte, opts ...decoder.Option) (decodedActivity, error) {
	listener := filedef.NewListener()
	// File() also closes the listener, on the path that reaches it; deferred
	// here too so every path does, the way internal/fit's own tests always
	// defer their listener's Close. A second Close is a no-op.
	defer listener.Close()
	dec := decoder.New(bytes.NewReader(raw), append([]decoder.Option{decoder.WithMesgListener(listener)}, opts...)...)
	if _, err := dec.Decode(); err != nil {
		return decodedActivity{}, fmt.Errorf("decoding: %w", err)
	}

	activity, ok := listener.File().(*filedef.Activity)
	if !ok || activity == nil {
		return decodedActivity{}, errors.New("no activity file type found")
	}

	return activityToDecoded(activity), nil
}

func activityToDecoded(activity *filedef.Activity) decodedActivity {
	records := make([]point, len(activity.Records))
	for i, record := range activity.Records {
		records[i] = fitPoint(record)
	}

	decoded := decodedActivity{Records: records, RecordingDevice: activity.FileId.Manufacturer.String()}
	if len(activity.Sessions) > 0 {
		session := activity.Sessions[0]
		if session.TotalTimerTime != basetype.Uint32Invalid {
			decoded.HasTotalTimerTime = true
			decoded.TotalTimerTime = time.Duration(session.TotalTimerTimeScaled() * float64(time.Second))
		}
		if session.TotalElapsedTime != basetype.Uint32Invalid {
			decoded.HasTotalElapsedTime = true
			decoded.TotalElapsedTime = time.Duration(session.TotalElapsedTimeScaled() * float64(time.Second))
		}
		if session.TotalAscent != basetype.Uint16Invalid {
			decoded.HasTotalAscent = true
			decoded.TotalAscentMetres = float64(session.TotalAscent)
		}
	}

	return decoded
}

// fitPoint reads the fields ridemodel uses from one FIT record, preferring
// the enhanced altitude and speed fields when a device wrote them — they
// carry more precision — and falling back to the plain ones otherwise.
func fitPoint(record *mesgdef.Record) point {
	p := point{Time: record.Timestamp}

	if record.PositionLat != basetype.Sint32Invalid && record.PositionLong != basetype.Sint32Invalid {
		p.HasPosition = true
		p.Latitude = record.PositionLatDegrees()
		p.Longitude = record.PositionLongDegrees()
	}
	if altitude, ok := fitAltitude(record); ok {
		p.HasAltitude = true
		p.AltitudeMetres = altitude
	}
	if record.Distance != basetype.Uint32Invalid {
		p.HasDistance = true
		p.DistanceMetres = record.DistanceScaled()
	}
	if record.Cadence != basetype.Uint8Invalid {
		p.HasCadence = true
		p.CadenceRPM = float64(record.Cadence)
	}
	if record.Temperature != basetype.Sint8Invalid {
		p.HasTemperatureCelsius = true
		p.TemperatureCelsius = float64(record.Temperature)
	}
	if record.Power != basetype.Uint16Invalid {
		p.HasPower = true
		p.PowerWatts = float64(record.Power)
	}
	if record.HeartRate != basetype.Uint8Invalid {
		p.HasHeartRate = true
		p.HeartRateBPM = float64(record.HeartRate)
	}

	return p
}

func fitAltitude(record *mesgdef.Record) (float64, bool) {
	if record.EnhancedAltitude != basetype.Uint32Invalid {
		return record.EnhancedAltitudeScaled(), true
	}
	if record.Altitude != basetype.Uint16Invalid {
		return record.AltitudeScaled(), true
	}

	return 0, false
}
