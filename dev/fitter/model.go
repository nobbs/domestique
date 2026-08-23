// Command fitter turns the corpus dev/ridemodel emits into fitted physical
// coefficients: effective drag area, rolling resistance per surface class,
// and sustained climbing power.
//
// It owns the regression and nothing else. It reads samples.csv, rides.csv
// and indoor.csv and writes one coefficient file per fitted gear group, plus
// a diagnostics report. See dev/fitter/main.go for the CLI.
package main

import (
	"time"

	"github.com/nobbs/domestique/internal/surface"
)

// gravityMetresPerSecondSquared is standard gravity, the one physical
// constant every stage of this fit shares.
const gravityMetresPerSecondSquared = 9.80665

// sampleRow is one row of samples.csv, joined against its ride's gear and
// type from rides.csv. Only the columns this package reads.
type sampleRow struct {
	Time             time.Time
	RideID           string
	Gear             string
	TemperatureC     float64
	AltitudeM        float64
	Latitude         float64
	Longitude        float64
	CadenceRPM       float64
	SpeedMPS         float64
	GradientPercent  float64
	IntervalDistance float64
	DeltaSeconds     float64
	HeartRateBPM     float64
	Surface          surface.Kind
	HasCadence       bool
	HasTemperature   bool
	HasPosition      bool
	HasAltitude      bool
	HasHeartRate     bool
	Moving           bool
}

// rideRow is one row of rides.csv, holding only the columns this package
// reads.
type rideRow struct {
	Date          time.Time
	RideID        string
	Gear          string
	Type          string
	MovingSeconds float64
}

// indoorRow is one row of indoor.csv, holding only the columns this package
// reads.
type indoorRow struct {
	Time         time.Time
	RideID       string
	PowerWatts   float64
	HeartRateBPM float64
	DeltaSeconds float64
	HasPower     bool
	HasHeartRate bool
}

// coastingWindow is one fixed-duration slice of a sustained coasting run: the
// regression's unit of observation. Every quantity is a window average or
// endpoint delta rather than a single record, so a noisy instantaneous
// dv/dt never enters the fit directly.
type coastingWindow struct {
	RideID          string
	Surface         surface.Kind
	DeltaSpeedMPS   float64 // v_end - v_start
	MeanSpeedMPS    float64 // distance / duration
	DurationSeconds float64
	GradePercent    float64 // mean rise/run over the window
	AirDensity      float64
}

// climbSample is one sustained above-threshold climbing window: stage B's
// unit of observation.
type climbSample struct {
	Date         time.Time
	RideID       string
	MeanSpeedMPS float64
	GradePercent float64
	AirDensity   float64
	Surface      surface.Kind
}

// fitResult is one gear group's fitted coefficients plus the diagnostics the
// report names.
type fitResult struct {
	CrrBySurface       map[surface.Kind]float64
	SkipReason         string
	Group              string
	SurvivingWindows   []coastingWindow
	QuarterlyIntercept []quarterlyCrr
	CrossCheck         indoorCrossCheckSummary
	HeldOut            heldOutValidation
	MeanAirDensity     float64
	PowerWatts         float64
	ClimbHoursAbove    float64
	MassKG             float64
	CorneringRejected  int
	PlausibilityReject int
	ConditionRatio     float64
	CrrOverall         float64
	CdA                float64
	ClimbThresholdPct  float64
	Skipped            bool
	UntaggedAttributed bool
	TyrePlausible      bool
	IllConditioned     bool
	RejectedCrrBounds  bool
}
