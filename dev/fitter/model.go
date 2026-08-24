// Command fitter benchmarks the accepted hybrid ETA model — internal/
// ridemodel's own equation, never a second implementation of it — against a
// ride corpus, and can recalibrate its two route coefficients,
// seconds_per_km and seconds_per_ascent_m, when explicitly asked.
//
// It reads samples.csv and rides.csv from dev/ridemodel's corpus. See
// dev/fitter/main.go for the CLI.
package main

import "time"

// sampleRow is one row of samples.csv, holding only the columns this
// package reads. It carries no gear or ride type — those live on rideRow,
// and gear partitioning (groupRidesByGear) works from rides.csv directly
// rather than joining them onto every sample.
type sampleRow struct {
	Time             time.Time
	RideID           string
	AltitudeM        float64
	Latitude         float64
	Longitude        float64
	SpeedMPS         float64
	GradientPercent  float64
	IntervalDistance float64
	DeltaSeconds     float64
	HasPosition      bool
	HasAltitude      bool
	Moving           bool
}

// rideRow is one row of rides.csv, holding only the columns this package
// reads.
type rideRow struct {
	Date          time.Time
	RideID        string
	Gear          string
	MovingSeconds float64
}
