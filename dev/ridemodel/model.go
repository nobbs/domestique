// Command ridemodel turns a Strava bulk export into a flat table of ride
// samples the drag and power fitter can regress over.
//
// It owns no physics and no fitting: it reads activities.csv and the files
// under activities/, and emits a sample table, a per-ride summary, and a
// reference table of indoor rides. See dev/ridemodel/main.go for the CLI, and
// the mise task "ridemodel" for how it is invoked.
package main

import (
	"io"
	"time"
)

// closeFile ignores the error of closing a file this package only ever
// reads: nothing this tool does with an export directory depends on a close
// error, the way internal/osmindex's own closeFile does not either.
//
//nolint:errcheck // A file opened for reading has nothing to report on close.
func closeFile(closer io.Closer) { _ = closer.Close() }

// movingSpeedThresholdMetresPerSecond is the one definition of "moving" this
// pipeline uses, applied in exactly one place (sample.MovingFilter, set in
// ingestActivity). Below it a bike is being walked or is stationary at a
// light, not rolling in any sense a drag or rolling-resistance fit should
// weight; above it, wheel speed dominates any GPS or auto-pause noise.
// Roughly a brisk walking pace (1.8 km/h), well under the ~3 m/s a coasting
// downhill roll starts at, so it never clips genuine low-speed riding.
//
// Strava's own moving_time column and a device's total_timer_time each apply
// their own undocumented heuristic and disagree with this and each other by
// one to three percent; both are reported as a sanity check, but this
// constant is what every downstream sample carries as its own filter, so a
// figure computed from filtered samples means the same thing everywhere it is
// used.
const movingSpeedThresholdMetresPerSecond = 0.5

// gradientWindowMetres matches internal/route's own constant (and
// lib/profile.ts's), so a gradient in this corpus means the same thing as one
// in the model later fitted to it. Kept as this package's own constant,
// rather than imported, because dev/ tooling stays decoupled from the
// service's internal packages.
const gradientWindowMetres = 100.0

// exclusionReason names why an activity did not enter the outdoor sample
// corpus. It is its own type so a report can name every reason it saw without
// a call site inventing a new string.
type exclusionReason string

const (
	// exclusionNotCycling marks a row this tool never treats as a ride at
	// all: a real bulk export holds every activity type an athlete logged,
	// and Strava's own Activity Type is the only signal that distinguishes
	// them — a run or a climb has no drag or rolling-resistance regime to
	// fit. Counted rather than dropped silently, so the corpus total still
	// reconciles against the CSV's own row count.
	exclusionNotCycling      exclusionReason = "not_a_ride"
	exclusionIndoor          exclusionReason = "indoor"
	exclusionUnreadable      exclusionReason = "unreadable"
	exclusionUnsupportedFile exclusionReason = "unsupported_file_format"
	exclusionNoSourceFile    exclusionReason = "no_source_file"
	exclusionUnsafeFilename  exclusionReason = "unsafe_filename"
	exclusionNoAltitude      exclusionReason = "no_altitude_channel"
	// exclusionNoSamples marks a decoded file that yielded fewer than two
	// usable record intervals — under two records, or every consecutive pair
	// sharing a non-positive Δt — so buildSamples has nothing to emit. Without
	// this, such a ride would count as ingested while contributing zero rows
	// to samples.csv, leaving the corpus's own ride count inconsistent with
	// what it actually holds.
	exclusionNoSamples exclusionReason = "no_samples"
)

// activityRow is one row of activities.csv, holding only the columns this
// tool reads. Strava's own export carries many more; the rest are ignored.
type activityRow struct {
	Date                time.Time
	ID                  string
	Type                string
	Gear                string
	Filename            string
	DistanceMetres      float64
	ElapsedTime         time.Duration
	StravaMovingTime    time.Duration
	ElevationGainMetres float64
}

// point is one decoded record, in the vocabulary both the FIT and the GPX
// source produce. A "Has" flag distinguishes a channel that was absent from
// one that happened to read as the zero value.
type point struct {
	Time                  time.Time
	CadenceRPM            float64
	HeartRateBPM          float64
	Latitude              float64
	Longitude             float64
	PowerWatts            float64
	AltitudeMetres        float64
	TemperatureCelsius    float64
	DistanceMetres        float64
	HasCadence            bool
	HasTemperatureCelsius bool
	HasDistance           bool
	HasPower              bool
	HasAltitude           bool
	HasHeartRate          bool
	HasPosition           bool
}

// decodedActivity is one source file's records plus whatever session-level
// totals it carried, in the vocabulary this tool works in — not the FIT or
// GPX wire format either source read.
type decodedActivity struct {
	RecordingDevice     string
	Records             []point
	TotalTimerTime      time.Duration
	TotalElapsedTime    time.Duration
	TotalAscentMetres   float64
	Derived             bool
	ChecksumFailed      bool
	HasTotalTimerTime   bool
	HasTotalElapsedTime bool
	HasTotalAscent      bool
}

// sample is one row of the outdoor sample table: one record interval, ending
// at Time.
type sample struct {
	Time                   time.Time
	RideID                 string
	TemperatureCelsius     float64
	DeltaSeconds           float64
	IntervalDistanceMetres float64
	SpeedMetresPerSecond   float64
	GradientPercent        float64
	Longitude              float64
	AltitudeMetres         float64
	CadenceRPM             float64
	Latitude               float64
	HasCadence             bool
	HasPosition            bool
	HasTemperatureCelsius  bool
	HasAltitude            bool
	MovingFilter           bool
	Derived                bool
}

// indoorSample is one row of the indoor reference table: one record
// interval's power, heart rate and cadence, kept in a file the outdoor
// corpus is never joined to.
type indoorSample struct {
	Time         time.Time
	RideID       string
	DeltaSeconds float64
	PowerWatts   float64
	HeartRateBPM float64
	CadenceRPM   float64
	HasPower     bool
	HasHeartRate bool
	HasCadence   bool
}

// rideSummary is one row of the per-ride summary table.
type rideSummary struct {
	Date                              time.Time
	Reason                            exclusionReason
	Type                              string
	Gear                              string
	Device                            string
	RideID                            string
	RecordingDevice                   string
	DeviceTimerTime                   time.Duration
	SampleCount                       int
	MovingSeconds                     float64
	ElapsedSeconds                    float64
	TotalDistanceMetres               float64
	StravaMovingTime                  time.Duration
	RawRiseMetres                     float64
	StopAllowanceMinutesPerMovingHour float64
	DeviceAscentMetres                float64
	Indoor                            bool
	HasAltitudeQuality                bool
	Derived                           bool
	ChecksumFailed                    bool
	HasCadence                        bool
	Excluded                          bool
	HasDeviceTimerTime                bool
}
