package main

import (
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// earthRadiusMetres matches internal/route's own spherical Earth model, kept
// as this package's own constant for the same reason gradientWindowMetres is.
const earthRadiusMetres = 6_371_000.0

// errUnsupportedFile marks a source file whose extension names no parser this
// tool has — TCX, most often, which the export's own table of inputs does not
// list. Distinct from a decode failure: the file was never opened.
var errUnsupportedFile = errors.New("no parser for this file format")

// ingestActivity turns one activities.csv row into a per-ride summary, and
// either outdoor sample rows or an indoor reference row, never both. Every
// exclusion is recorded on the summary rather than returned as an error: a
// run over hundreds of activities must finish and report on all of them, not
// stop at the first one a file format or a missing channel does not like.
func ingestActivity(exportDir string, row *activityRow) (rideSummary, []sample, []indoorSample) {
	summary := rideSummary{
		RideID:           row.ID,
		Date:             row.Date,
		Type:             row.Type,
		Gear:             row.Gear,
		StravaMovingTime: row.StravaMovingTime,
	}

	if !isCyclingType(row.Type) {
		summary.Excluded = true
		summary.Reason = exclusionNotCycling

		return summary, nil, nil
	}
	if row.Filename == "" {
		summary.Excluded = true
		summary.Reason = exclusionNoSourceFile

		return summary, nil, nil
	}

	path, pathErr := resolveActivityFile(exportDir, row.Filename)
	if pathErr != nil {
		summary.Excluded = true
		summary.Reason = exclusionUnsafeFilename

		return summary, nil, nil
	}

	decoded, device, err := decodeActivityFile(path)
	summary.Device = device
	if err != nil {
		summary.Excluded = true
		summary.Reason = exclusionReasonFor(err)

		return summary, nil, nil
	}

	summary.ChecksumFailed = decoded.ChecksumFailed
	summary.Derived = decoded.Derived
	summary.RecordingDevice = decoded.RecordingDevice
	if decoded.HasTotalTimerTime {
		summary.HasDeviceTimerTime = true
		summary.DeviceTimerTime = decoded.TotalTimerTime
	}

	if len(decoded.Records) >= 2 {
		summary.ElapsedSeconds = decoded.Records[len(decoded.Records)-1].Time.Sub(decoded.Records[0].Time).Seconds()
	}

	summary.Indoor = isIndoorType(row.Type) || allRecordsLackPosition(decoded.Records)
	if summary.Indoor {
		indoorRows := buildIndoorSamples(row.ID, decoded.Records)
		summary.SampleCount = len(indoorRows)
		summary.Excluded = true
		summary.Reason = exclusionIndoor

		return summary, nil, indoorRows
	}

	if !anyRecordHasAltitude(decoded.Records) {
		summary.Excluded = true
		summary.Reason = exclusionNoAltitude

		return summary, nil, nil
	}

	samples := buildSamples(row.ID, decoded.Records, decoded.Derived)
	summary.SampleCount = len(samples)
	for i := range samples {
		summary.TotalDistanceMetres += samples[i].IntervalDistanceMetres
		if samples[i].MovingFilter {
			summary.MovingSeconds += samples[i].DeltaSeconds
		}
		if samples[i].HasCadence {
			summary.HasCadence = true
		}
	}
	if summary.MovingSeconds > 0 {
		stopSeconds := summary.ElapsedSeconds - summary.MovingSeconds
		summary.StopAllowanceMinutesPerMovingHour = (stopSeconds / 60) / (summary.MovingSeconds / 3600)
	}

	summary.RawRiseMetres = sumRawRise(decoded.Records)
	if decoded.HasTotalAscent {
		summary.HasAltitudeQuality = true
		summary.DeviceAscentMetres = decoded.TotalAscentMetres
	}

	return summary, samples, nil
}

// resolveActivityFile joins exportDir with the Filename column, and refuses
// the result if it does not stay inside exportDir. Filename is data this
// tool did not write — an absolute path, or one built from "..", would let a
// malformed or tampered export point this tool at an arbitrary local file;
// this is what stops that regardless of whether Filename could also just be
// an honest mistake.
func resolveActivityFile(exportDir, filename string) (string, error) {
	if filepath.IsAbs(filename) {
		return "", fmt.Errorf("filename %q must be relative", filename)
	}
	joined := filepath.Join(exportDir, filename)
	rel, err := filepath.Rel(exportDir, joined)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("filename %q escapes the export directory", filename)
	}

	return joined, nil
}

// decodeActivityFile picks the decoder by file extension and reports which
// device kind it used, so the summary can name it even when decoding failed.
func decodeActivityFile(path string) (decodedActivity, string, error) {
	switch sourceExtension(path) {
	case "fit.gz":
		decoded, err := decodeFITGZ(path)

		return decoded, "fit", err
	case "gpx.gz":
		decoded, err := decodeGPX(path, true)

		return decoded, "gpx", err
	case "gpx":
		decoded, err := decodeGPX(path, false)

		return decoded, "gpx", err
	default:
		return decodedActivity{}, "", errUnsupportedFile
	}
}

func exclusionReasonFor(err error) exclusionReason {
	if errors.Is(err, errUnsupportedFile) {
		return exclusionUnsupportedFile
	}

	return exclusionUnreadable
}

func sourceExtension(filename string) string {
	lower := strings.ToLower(filename)
	switch {
	case strings.HasSuffix(lower, ".fit.gz"):
		return "fit.gz"
	case strings.HasSuffix(lower, ".gpx.gz"):
		return "gpx.gz"
	case strings.HasSuffix(lower, ".gpx"):
		return "gpx"
	default:
		return ""
	}
}

func isCyclingType(activityType string) bool {
	return strings.Contains(strings.ToLower(activityType), "ride")
}

// isIndoorType matches Strava's "Virtual Ride" activity type, tolerant of the
// space its real exports carry even though the value read from Strava's API
// (and the issue this tool was written against) is unspaced "VirtualRide".
func isIndoorType(activityType string) bool {
	return strings.EqualFold(strings.ReplaceAll(activityType, " ", ""), "VirtualRide")
}

func allRecordsLackPosition(records []point) bool {
	for i := range records {
		if records[i].HasPosition {
			return false
		}
	}

	return true
}

func anyRecordHasAltitude(records []point) bool {
	for i := range records {
		if records[i].HasAltitude {
			return true
		}
	}

	return false
}

// buildSamples turns a decoded record stream into one row per interval,
// starting at the second record: the first has no predecessor to form an
// interval with. A pair whose Δt is not strictly positive — a clock that
// jumped backward, or a duplicate timestamp — is skipped rather than
// producing a divide-by-zero or negative speed.
func buildSamples(rideID string, records []point, derived bool) []sample {
	if len(records) < 2 {
		return nil
	}

	cumulative := cumulativeDistances(records)
	gradients := windowedGradients(records, cumulative)

	samples := make([]sample, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		prev, cur := records[i-1], records[i]
		dt := cur.Time.Sub(prev.Time).Seconds()
		if dt <= 0 {
			continue
		}
		intervalDistance := cumulative[i] - cumulative[i-1]
		s := sample{
			RideID:                 rideID,
			Time:                   cur.Time,
			DeltaSeconds:           dt,
			IntervalDistanceMetres: intervalDistance,
			SpeedMetresPerSecond:   intervalDistance / dt,
			GradientPercent:        gradients[i],
			MovingFilter:           intervalDistance/dt >= movingSpeedThresholdMetresPerSecond,
			Derived:                derived,
		}
		if cur.HasAltitude {
			s.HasAltitude = true
			s.AltitudeMetres = cur.AltitudeMetres
		}
		if cur.HasCadence {
			s.HasCadence = true
			s.CadenceRPM = cur.CadenceRPM
		}
		if cur.HasTemperatureCelsius {
			s.HasTemperatureCelsius = true
			s.TemperatureCelsius = cur.TemperatureCelsius
		}
		if cur.HasPosition {
			s.HasPosition = true
			s.Latitude = cur.Latitude
			s.Longitude = cur.Longitude
		}
		samples = append(samples, s)
	}

	return samples
}

func buildIndoorSamples(rideID string, records []point) []indoorSample {
	if len(records) < 2 {
		return nil
	}

	samples := make([]indoorSample, 0, len(records)-1)
	for i := 1; i < len(records); i++ {
		prev, cur := records[i-1], records[i]
		dt := cur.Time.Sub(prev.Time).Seconds()
		if dt <= 0 {
			continue
		}
		s := indoorSample{RideID: rideID, Time: cur.Time, DeltaSeconds: dt}
		if cur.HasPower {
			s.HasPower = true
			s.PowerWatts = cur.PowerWatts
		}
		if cur.HasHeartRate {
			s.HasHeartRate = true
			s.HeartRateBPM = cur.HeartRateBPM
		}
		if cur.HasCadence {
			s.HasCadence = true
			s.CadenceRPM = cur.CadenceRPM
		}
		samples = append(samples, s)
	}

	return samples
}

// cumulativeDistances returns, for each record, the distance travelled since
// the first one. A pair that both carry the device's own odometer reading
// uses its delta; otherwise it falls back to the great-circle distance
// between their positions, which is 0 when either lacks one.
func cumulativeDistances(records []point) []float64 {
	cumulative := make([]float64, len(records))
	for i := 1; i < len(records); i++ {
		cumulative[i] = cumulative[i-1] + intervalDistance(&records[i-1], &records[i])
	}

	return cumulative
}

func intervalDistance(prev, cur *point) float64 {
	if prev.HasDistance && cur.HasDistance {
		return cur.DistanceMetres - prev.DistanceMetres
	}
	if prev.HasPosition && cur.HasPosition {
		return haversineMetres(prev, cur)
	}

	return 0
}

func haversineMetres(left, right *point) float64 {
	latitudeDelta := (right.Latitude - left.Latitude) * math.Pi / 180
	longitudeDelta := (right.Longitude - left.Longitude) * math.Pi / 180
	leftLatitude := left.Latitude * math.Pi / 180
	rightLatitude := right.Latitude * math.Pi / 180
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)

	return earthRadiusMetres * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// windowedGradients returns, for each record, the gradient measured over a
// window of at least gradientWindowMetres ending at it — never between two
// adjacent records, which can be metres apart and dominated by altitude
// error rather than terrain. Doubling how densely a ride is sampled leaves
// these materially unchanged, because the window is a physical distance, not
// a point count: the same expanding-trailing-window technique
// internal/route.Stage.MaxGradientPercent uses, adapted to report every
// point's own value instead of only the steepest.
func windowedGradients(records []point, cumulative []float64) []float64 {
	gradients := make([]float64, len(records))
	trailing := 0
	for leading := 1; leading < len(records); leading++ {
		for trailing+1 < leading && cumulative[leading]-cumulative[trailing+1] >= gradientWindowMetres {
			trailing++
		}
		if !records[leading].HasAltitude || !records[trailing].HasAltitude {
			continue
		}
		span := cumulative[leading] - cumulative[trailing]
		if span < gradientWindowMetres {
			continue
		}
		rise := records[leading].AltitudeMetres - records[trailing].AltitudeMetres
		gradients[leading] = rise / span * 100
	}

	return gradients
}

// sumRawRise sums every positive altitude step between consecutive records
// that both carry one, ignoring any without — the same "sum of positive
// steps" internal/route.Stage.AscentMetres computes, over whichever records
// this ride actually has altitude for.
func sumRawRise(records []point) float64 {
	rise := 0.0
	havePrevious := false
	previous := 0.0
	for i := range records {
		if !records[i].HasAltitude {
			continue
		}
		if havePrevious {
			if delta := records[i].AltitudeMetres - previous; delta > 0 {
				rise += delta
			}
		}
		previous = records[i].AltitudeMetres
		havePrevious = true
	}

	return rise
}
