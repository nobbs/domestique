package measure

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The barometric-pressure and ideal-gas formula against hand-computed values:
// density falls with altitude and rises with cold.
func TestAirDensityFallsWithAltitudeAndRisesWithCold(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, 1.225, airDensity(0, 15), 0.001, "sea level, fifteen degrees")
	// The handover's own "≈1.112" for this case assumes the standard
	// atmosphere's falling temperature at altitude; the formula it actually
	// specifies holds temperature at the given 15°C and comes to ~1.087.
	assert.InDelta(t, 1.087, airDensity(1000, 15), 0.001, "a thousand metres up, thinner air")
	assert.InDelta(t, 1.164, airDensity(0, 30), 0.001, "sea level, hot air is less dense")
}

func gwmStart() time.Time { return time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC) }

// gwmRamp records one sample a second at a constant speed up a constant
// grade, the fixture gradeWindowMetres' own tests derive a window from.
func gwmRamp(n int, speedMS, grade float64) []Sample {
	samples := make([]Sample, n)
	for index := range samples {
		distance := speedMS * float64(index)
		samples[index] = Sample{
			At:             gwmStart().Add(time.Duration(index) * time.Second),
			DistanceMetres: distance,
			AltitudeMetres: 100 + distance*grade,
		}
	}

	return samples
}

func gwmFlat(n int, speedMS float64) []Sample {
	return gwmRamp(n, speedMS, 0)
}

// gwmNoisy layers deterministic jitter onto a clean track's distance and
// altitude, the way GPS and barometric noise do on a real recording.
func gwmNoisy(base []Sample) []Sample {
	distanceCycle := []float64{0, 1, -1, 0.5, -0.5, 1, -1}
	altitudeCycle := []float64{0, 0.2, -0.2, 0.2, 0, -0.2, 0.4}
	samples := make([]Sample, len(base))
	for index, sample := range base {
		sample.DistanceMetres += distanceCycle[index%len(distanceCycle)]
		sample.AltitudeMetres += altitudeCycle[index%len(altitudeCycle)]
		samples[index] = sample
	}

	return samples
}

// The window is derived per ride from the altimeter's own resolution:
// window = clamp(quantum / 0.002, 30, 300). See docs/specs/measurement.md
// §Gradient.
func TestGradeWindowMetresDerivesFromTheAltimetersResolution(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		samples []Sample
		want    float64
	}{
		"a barometer that steps in fifths of a metre gets 100 m": {
			samples: gwmNoisy(gwmFlat(1200, 7.0)),
			want:    100,
		},
		"a ramp rising 0.35 m a second gets 175 m": {
			samples: gwmRamp(10, 7, 0.05),
			want:    175,
		},
		"metre-scale steps clamp to the 300 m ceiling": {
			samples: gwmRamp(10, 1, 1),
			want:    300,
		},
		"two-centimetre steps clamp to the 30 m floor": {
			samples: gwmRamp(10, 1, 0.02),
			want:    30,
		},
		"a flat ride with no positive step takes the floor": {
			samples: gwmFlat(300, 7.5),
			want:    30,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.InDelta(t, testCase.want, gradeWindowMetres(testCase.samples), 0.001)
		})
	}
}

// A step across a pause is drift, not a reading of the altimeter's own
// resolution: the only positive altitude change in this fixture sits between
// two flat stretches either side of a stop longer than DefaultMaxGap, so it
// is ignored and the flat stretches leave the ride at the floor.
func TestGradeWindowMetresIgnoresAnAltitudeStepAcrossAPause(t *testing.T) {
	t.Parallel()
	before := gwmFlat(120, 7.5)
	after := gwmFlat(120, 7.5)
	for index := range after {
		after[index].At = before[len(before)-1].At.Add(time.Hour + time.Duration(index)*time.Second)
		after[index].DistanceMetres += before[len(before)-1].DistanceMetres
		after[index].AltitudeMetres += 40
	}
	samples := append(append([]Sample{}, before...), after...)

	assert.InDelta(t, 30, gradeWindowMetres(samples), 0.001)
}

// A clock that did not advance is no step the series measures over, so an
// altitude change across one is no reading of the altimeter's resolution.
func TestGradeWindowMetresIgnoresAnAltitudeStepAcrossANonAdvancingClock(t *testing.T) {
	t.Parallel()
	samples := gwmNoisy(gwmFlat(600, 7))
	last := samples[len(samples)-1]
	last.AltitudeMetres += 0.01
	samples = append(samples, last)

	assert.InDelta(t, 100, gradeWindowMetres(samples), 0.001)
}

// A ride with no positive altitude step at all, or fewer than two samples,
// has nothing the resolution can be read from and takes the floor.
func TestGradeWindowMetresRequires(t *testing.T) {
	t.Parallel()
	assert.InDelta(t, minWindowMetres, gradeWindowMetres(nil), 0.001)
	require.Len(t, gwmFlat(2, 7.5), 2, "sanity: the fixture is as short as the caller intends")
	assert.InDelta(t, minWindowMetres, gradeWindowMetres(gwmFlat(2, 7.5)), 0.001)
}
