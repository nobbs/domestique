package main

import (
	"compress/gzip"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"time"
)

// gpxDocument is the minimal GPX 1.1 shape this tool reads: one or more
// tracks, each one or more segments, each a list of points. A Strava export
// writes exactly one track and one segment per file, but nothing here
// depends on that.
type gpxDocument struct {
	Tracks []gpxTrack `xml:"trk"`
}

type gpxTrack struct {
	Segments []gpxSegment `xml:"trkseg"`
}

type gpxSegment struct {
	Points []gpxPoint `xml:"trkpt"`
}

type gpxPoint struct {
	Extensions gpxExtensions `xml:"extensions"`
	Elevation  *float64      `xml:"ele"`
	Time       string        `xml:"time"`
	Lat        float64       `xml:"lat,attr"`
	Lon        float64       `xml:"lon,attr"`
}

type gpxExtensions struct {
	TrackPoint gpxTrackPointExtension `xml:"TrackPointExtension"`
}

type gpxTrackPointExtension struct {
	Cadence     *float64 `xml:"cad"`
	HeartRate   *float64 `xml:"hr"`
	Temperature *float64 `xml:"atemp"`
}

// decodeGPX reads a Strava GPX export, gzipped or not. GPX carries no speed
// channel — Strava differentiates it from position and time, which is noisier
// than a device's own field — so every record is marked Derived and
// ingestActivity carries that through to every sample row.
func decodeGPX(path string, gzipped bool) (decodedActivity, error) {
	file, err := os.Open(path) //nolint:gosec // The path is composed from the operator's own -export flag and activities.csv's Filename column.
	if err != nil {
		return decodedActivity{}, fmt.Errorf("opening %s: %w", path, err)
	}
	defer closeFile(file)

	var reader io.Reader = file
	if gzipped {
		gzipReader, gzErr := gzip.NewReader(file)
		if gzErr != nil {
			return decodedActivity{}, fmt.Errorf("opening gzip stream in %s: %w", path, gzErr)
		}
		defer closeFile(gzipReader)
		reader = gzipReader
	}

	var doc gpxDocument
	if err := xml.NewDecoder(reader).Decode(&doc); err != nil {
		return decodedActivity{}, fmt.Errorf("decoding GPX: %w", err)
	}

	var records []point
	for _, track := range doc.Tracks {
		for _, segment := range track.Segments {
			for _, raw := range segment.Points {
				p, ok := gpxPointToPoint(raw)
				if !ok {
					continue
				}
				records = append(records, p)
			}
		}
	}

	return decodedActivity{Records: records, Derived: true}, nil
}

// gpxPointToPoint converts one trkpt. A point with an unparseable timestamp
// is dropped: every downstream computation reads timestamps, not record
// index, so a point this tool cannot place in time is not one it can use.
func gpxPointToPoint(raw gpxPoint) (point, bool) {
	at, err := parseGPXTime(raw.Time)
	if err != nil {
		return point{}, false
	}
	p := point{
		Time:        at,
		HasPosition: true,
		Latitude:    raw.Lat,
		Longitude:   raw.Lon,
	}
	if raw.Elevation != nil {
		p.HasAltitude = true
		p.AltitudeMetres = *raw.Elevation
	}
	if raw.Extensions.TrackPoint.Cadence != nil {
		p.HasCadence = true
		p.CadenceRPM = *raw.Extensions.TrackPoint.Cadence
	}
	if raw.Extensions.TrackPoint.HeartRate != nil {
		p.HasHeartRate = true
		p.HeartRateBPM = *raw.Extensions.TrackPoint.HeartRate
	}
	if raw.Extensions.TrackPoint.Temperature != nil {
		p.HasTemperatureCelsius = true
		p.TemperatureCelsius = *raw.Extensions.TrackPoint.Temperature
	}

	return p, true
}

// parseGPXTime reads a GPX trkpt's <time>, which Strava always writes as
// RFC3339 UTC ("Z").
func parseGPXTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parsing GPX time: %w", err)
	}

	return parsed, nil
}
