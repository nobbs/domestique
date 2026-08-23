package main

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testMassKG = 90.0

// straightLineSamples builds n coasting-eligible samples, 1 s apart, heading
// due north at speed with small realistic GPS jitter (a few centimetres) and
// a physically ordinary coasting deceleration — comfortably inside the
// plausibility bounds at testMassKG, so a real cornering filter is what these
// tests exercise, not the plausibility filter.
func straightLineSamples(n int, speed float64) []sampleRow {
	const decelPerSecond = 0.35 // within [minPlausible, maxPlausible] at 10 m/s, testMassKG

	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	lat, lon := 50.0, 8.0
	samples := make([]sampleRow, n)
	for i := range n {
		v := math.Max(1.0, speed-decelPerSecond*float64(i))
		jitter := 0.0000003 * math.Sin(float64(i)) // ~3 cm, GPS-grade noise
		samples[i] = sampleRow{
			RideID:           "r1",
			Time:             start.Add(time.Duration(i) * time.Second),
			DeltaSeconds:     1.0,
			IntervalDistance: v,
			SpeedMPS:         v,
			GradientPercent:  0,
			HasAltitude:      true,
			CadenceRPM:       0,
			HasCadence:       true,
			Latitude:         lat + float64(i)*0.000009 + jitter,
			Longitude:        lon,
			HasPosition:      true,
			Moving:           true,
		}
	}

	return samples
}

// curvingPathSamples builds a path that heads due north, bends through a
// quarter turn over turnSamples starting at turnStart, then continues
// straight east — a real turn a cornering filter must reject, wherever it is
// tested inside the bend.
func curvingPathSamples(n, turnStart, turnSamples int, speed float64) []sampleRow {
	const metresPerDegreeLat = 111_320.0

	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	lat, lon := 50.0, 8.0
	samples := make([]sampleRow, n)
	for i := range n {
		heading := 0.0
		switch {
		case i >= turnStart+turnSamples:
			heading = math.Pi / 2
		case i >= turnStart:
			heading = (math.Pi / 2) * float64(i-turnStart) / float64(turnSamples)
		}

		metresPerDegreeLon := metresPerDegreeLat * math.Cos(lat*math.Pi/180)
		lat += speed * math.Cos(heading) / metresPerDegreeLat
		lon += speed * math.Sin(heading) / metresPerDegreeLon

		samples[i] = sampleRow{
			RideID:           "r1",
			Time:             start.Add(time.Duration(i) * time.Second),
			DeltaSeconds:     1.0,
			IntervalDistance: speed,
			SpeedMPS:         speed,
			HasAltitude:      true,
			CadenceRPM:       0,
			HasCadence:       true,
			Latitude:         lat,
			Longitude:        lon,
			HasPosition:      true,
			Moving:           true,
		}
	}

	return samples
}

// realisticTurningSamples decelerates at a physically ordinary rate (so
// plausible() has no reason to reject anything) while turning sharply
// starting at turnStart — a synthetic run with a real, unambiguous
// cornering event embedded in it, rather than a discontinuity too subtle
// for corneringPass to see.
func realisticTurningSamples(n, turnStart, turnSamples int, speed, turnAngle float64) []sampleRow {
	const decelPerSecond = 0.35
	const metresPerDegreeLat = 111_320.0

	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	lat, lon := 50.0, 8.0
	samples := make([]sampleRow, n)
	for i := range n {
		heading := 0.0
		switch {
		case i >= turnStart+turnSamples:
			heading = turnAngle
		case i >= turnStart:
			heading = turnAngle * float64(i-turnStart) / float64(turnSamples)
		}
		v := math.Max(1.0, speed-decelPerSecond*float64(i))

		metresPerDegreeLon := metresPerDegreeLat * math.Cos(lat*math.Pi/180)
		lat += v * math.Cos(heading) / metresPerDegreeLat
		lon += v * math.Sin(heading) / metresPerDegreeLon

		samples[i] = sampleRow{
			RideID: "r1", Time: start.Add(time.Duration(i) * time.Second), DeltaSeconds: 1.0,
			IntervalDistance: v, SpeedMPS: v, HasAltitude: true, CadenceRPM: 0, HasCadence: true,
			Latitude: lat, Longitude: lon, HasPosition: true, Moving: true,
		}
	}

	return samples
}

// A cornering sample must break the run rather than being spliced out of
// it: deleting it and windowing over what remains can join an interval
// from before the turn to one from after it into a single "window" whose
// duration and distance come from ten records while its speed delta comes
// from two records that are not adjacent in time — this run is
// constructed so that never happens (16 s pre-turn, too short alone for a
// second window; 20 s post-turn, good for exactly two), and a splice bug
// would report one window more than the run actually contains.
func TestWindowsInRunBreaksAtACorneringSampleRatherThanSplicingAroundIt(t *testing.T) {
	samples := realisticTurningSamples(16+1+20, 16, 1, 10.0, math.Pi)
	counts := &coastingFilterCounts{}

	windows := windowsInRun(samples, counts, testMassKG)
	assert.Len(t, windows, 2)
	assert.Equal(t, len(windows), counts.SurvivingWindows)
	assert.Zero(t, counts.Plausibility, "the deceleration here is ordinary; nothing should fail on physical grounds")
}

func TestCoastingWindowsForBuildsWindowsFromASustainedRun(t *testing.T) {
	samples := straightLineSamples(41, 10.0)
	counts := &coastingFilterCounts{}

	windows := coastingWindowsFor(samples, counts, testMassKG)
	require.NotEmpty(t, windows)
	assert.Equal(t, len(windows), counts.SurvivingWindows)
	for _, w := range windows {
		assert.InDelta(t, windowDurationSeconds, w.DurationSeconds, 0.01)
		assert.Equal(t, "r1", w.RideID)
	}
}

func TestCoastingWindowsForSkipsARunShorterThanOneWindow(t *testing.T) {
	samples := straightLineSamples(5, 10.0) // 5 s, under one window
	counts := &coastingFilterCounts{}

	windows := coastingWindowsFor(samples, counts, testMassKG)
	assert.Empty(t, windows)
}

func TestCoastingWindowsForBreaksARunOnCadenceOrAGap(t *testing.T) {
	samples := straightLineSamples(30, 10.0)
	samples[15].CadenceRPM = 60 // rider resumes pedalling mid-run
	counts := &coastingFilterCounts{}

	windows := coastingWindowsFor(samples, counts, testMassKG)
	for _, w := range windows {
		assert.LessOrEqual(t, w.DurationSeconds, 15.0)
	}
}

func TestCorneringPassAcceptsAStraightLineWithGPSJitter(t *testing.T) {
	samples := straightLineSamples(41, 10.0)
	for i := corneringChordIndexMargin; i < len(samples)-corneringChordIndexMargin; i++ {
		assert.True(t, corneringPass(samples, i), "sample %d", i)
	}
}

func TestCorneringPassRejectsARealTurn(t *testing.T) {
	samples := curvingPathSamples(30, 10, 6, 10.0)
	assert.False(t, corneringPass(samples, 13))
}

func TestCorneringPassAcceptsStraightSectionsBeforeAndAfterATurn(t *testing.T) {
	samples := curvingPathSamples(30, 10, 6, 10.0)
	assert.True(t, corneringPass(samples, 4))
	assert.True(t, corneringPass(samples, 25))
}

func TestPlausibleRejectsImplausiblyHighDissipation(t *testing.T) {
	// A sharp brake tap: far more deceleration than any plausible Crr/CdA at
	// this speed and grade could produce.
	w := coastingWindow{
		DeltaSpeedMPS:   -20.0,
		MeanSpeedMPS:    10.0,
		DurationSeconds: 10.0,
		GradePercent:    0,
		AirDensity:      1.2,
	}
	assert.False(t, plausible(w, testMassKG))
}

func TestPlausibleRejectsImplausiblyLowDissipation(t *testing.T) {
	// Barely slowing at all while flat and slow: less dissipation than even
	// the lower bound of Crr and CdA could produce — a push, a draft, or a
	// gust, not a clean coast.
	w := coastingWindow{
		DeltaSpeedMPS:   0.5,
		MeanSpeedMPS:    5.0,
		DurationSeconds: 10.0,
		GradePercent:    0,
		AirDensity:      1.2,
	}
	assert.False(t, plausible(w, testMassKG))
}

func TestPlausibleAcceptsAnOrdinaryCoast(t *testing.T) {
	w := coastingWindow{
		DeltaSpeedMPS:   -3.5, // matches straightLineSamples' own decel
		MeanSpeedMPS:    10.0,
		DurationSeconds: 10.0,
		GradePercent:    0,
		AirDensity:      1.2,
	}
	assert.True(t, plausible(w, testMassKG))
}

// corneringChordIndexMargin is a generous index margin (in samples, at 1 Hz)
// to stay clear of straightLineSamples' own start and end, where
// corneringPass cannot build a full chord and always passes regardless of
// geometry — testing there would prove nothing about the filter itself.
const corneringChordIndexMargin = int(corneringChordSeconds) + 2
