package main

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/surface"
)

// climbingSamples builds n samples 1 s apart at a fixed grade and speed, all
// above the climbing threshold.
func climbingSamples(n int, speed, gradePercent float64) []sampleRow {
	start := time.Date(2026, 8, 1, 6, 0, 0, 0, time.UTC)
	samples := make([]sampleRow, n)
	for i := range n {
		samples[i] = sampleRow{
			RideID:           "r1",
			Time:             start.Add(time.Duration(i) * time.Second),
			DeltaSeconds:     1.0,
			IntervalDistance: speed,
			SpeedMPS:         speed,
			GradientPercent:  gradePercent,
			HasAltitude:      true,
			Moving:           true,
		}
	}

	return samples
}

func TestClimbWindowsForRequiresASustainedRun(t *testing.T) {
	short := climbingSamples(10, 4.0, 6.0) // under minClimbRunSeconds
	assert.Empty(t, climbWindowsFor(short, defaultClimbThresholdPercent))

	long := climbingSamples(40, 4.0, 6.0)
	windows := climbWindowsFor(long, defaultClimbThresholdPercent)
	require.NotEmpty(t, windows)
	for _, c := range windows {
		assert.InDelta(t, 6.0, c.GradePercent, 0.01)
	}
}

func TestClimbWindowsForExcludesBelowThreshold(t *testing.T) {
	samples := climbingSamples(40, 4.0, 2.0) // below the 4% default
	assert.Empty(t, climbWindowsFor(samples, defaultClimbThresholdPercent))
}

func TestSustainedPowerWattsMatchesTheExplicitEquation(t *testing.T) {
	climbs := []climbSample{
		{MeanSpeedMPS: 4.0, GradePercent: 6.0, AirDensity: 1.2},
	}
	const crr, cda, massKG, efficiency = 0.006, 0.45, 90.0, 0.975

	got := sustainedPowerWatts(climbs, nil, crr, cda, massKG, efficiency)

	grade := 0.06
	denom := math.Sqrt(1 + grade*grade)
	want := 4.0 * (crr*massKG*gravityMetresPerSecondSquared*(1/denom) +
		massKG*gravityMetresPerSecondSquared*(grade/denom) +
		0.5*1.2*cda*4.0*4.0) / efficiency
	assert.InDelta(t, want, got, 1e-9)
}

func TestSustainedPowerWattsPrefersThePerSurfaceCrrWhenFitted(t *testing.T) {
	climb := climbSample{MeanSpeedMPS: 4.0, GradePercent: 6.0, AirDensity: 1.2, Surface: surface.KindGravel}
	const cda, massKG, efficiency = 0.45, 90.0, 0.975

	withOverallOnly := sustainedPowerWatts([]climbSample{climb}, nil, 0.006, cda, massKG, efficiency)
	withPerSurface := sustainedPowerWatts(
		[]climbSample{climb}, map[surface.Kind]float64{surface.KindGravel: 0.012}, 0.006, cda, massKG, efficiency,
	)
	assert.Greater(t, withPerSurface, withOverallOnly, "gravel's higher Crr should raise the implied power")
}

func TestTrimmedMeanDropsBothTails(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 100}
	got := trimmedMean(values, 0.2)
	assert.InDelta(t, 5, got, 0.6)
}

func TestTrimmedMeanHandlesEmpty(t *testing.T) {
	assert.InDelta(t, 0, trimmedMean(nil, 0.2), 1e-9)
}
