package measure_test

import (
	"math/rand/v2"
	"time"

	"github.com/nobbs/domestique/internal/measure"
)

func start() time.Time { return time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC) }

// flat records one sample a second at a constant speed on flat ground.
func flat(n int, speedMS float64) []measure.Sample {
	return ramp(n, speedMS, 0)
}

// ramp records one sample a second at a constant speed up a constant grade.
func ramp(n int, speedMS, grade float64) []measure.Sample {
	samples := make([]measure.Sample, n)
	for index := range samples {
		distance := speedMS * float64(index)
		samples[index] = measure.Sample{
			At:             start().Add(time.Duration(index) * time.Second),
			DistanceMetres: distance,
			AltitudeMetres: 100 + distance*grade,
		}
	}

	return samples
}

// outAndBack rides a ramp out and descends the same grade back down, so the
// profile has one climb and one equal descent either side of a summit.
func outAndBack(n int, speedMS, grade float64) []measure.Sample {
	out := ramp(n, speedMS, grade)
	samples := make([]measure.Sample, 0, 2*n-1)
	samples = append(samples, out...)
	summitDistance := out[n-1].DistanceMetres
	summitAltitude := out[n-1].AltitudeMetres
	for index := 1; index < n; index++ {
		distance := summitDistance + speedMS*float64(index)
		samples = append(samples, measure.Sample{
			At:             start().Add(time.Duration(n-1+index) * time.Second),
			DistanceMetres: distance,
			AltitudeMetres: summitAltitude - speedMS*float64(index)*grade,
		})
	}

	return samples
}

// noisy layers deterministic jitter onto an otherwise clean track's distance
// and altitude, the way GPS and barometric noise do on a real recording.
func noisy(base []measure.Sample) []measure.Sample {
	// Cyclic jitter applied to distance and altitude in turn, so noisy tracks
	// are reproducible without a random source.
	distanceCycle := []float64{0, 1, -1, 0.5, -0.5, 1, -1}
	altitudeCycle := []float64{0, 0.2, -0.2, 0.2, 0, -0.2, 0.4}
	samples := make([]measure.Sample, len(base))
	for index, sample := range base {
		sample.DistanceMetres += distanceCycle[index%len(distanceCycle)]
		sample.AltitudeMetres += altitudeCycle[index%len(altitudeCycle)]
		samples[index] = sample
	}

	return samples
}

// paused records `before` samples, a silence of `gap`, then `after` more
// samples, all at the same speed and grade either side of the gap.
func paused(before, after int, gap time.Duration) []measure.Sample {
	first := ramp(before, 5, 0.02)
	samples := make([]measure.Sample, 0, before+after)
	samples = append(samples, first...)
	resumeAt := first[before-1].At.Add(gap)
	resumeDistance := first[before-1].DistanceMetres
	resumeAltitude := first[before-1].AltitudeMetres
	for index := range after {
		distance := resumeDistance + 5*float64(index)
		samples = append(samples, measure.Sample{
			At:             resumeAt.Add(time.Duration(index) * time.Second),
			DistanceMetres: distance,
			AltitudeMetres: resumeAltitude + (distance-resumeDistance)*0.02,
		})
	}

	return samples
}

// stationary records n samples one second apart with no distance or altitude
// change at all, as a rider stopped at the roadside would.
func stationary(n int) []measure.Sample {
	samples := make([]measure.Sample, n)
	for index := range samples {
		samples[index] = measure.Sample{
			At:             start().Add(time.Duration(index) * time.Second),
			DistanceMetres: 0,
			AltitudeMetres: 50,
		}
	}

	return samples
}

// randomCorpus produces a few thousand points with non-decreasing distance,
// jittered altitude, one-second timestamps and occasional gaps over ten
// seconds, from a fixed seed so the corpus is reproducible.
func randomCorpus() []measure.Sample {
	//nolint:gosec // A fixed seed keeps a failure reproducible; nothing here is a secret.
	source := rand.New(rand.NewPCG(1, 2))
	const n = 3000
	samples := make([]measure.Sample, n)
	at := start()
	distance := 0.0
	altitude := 100.0
	for index := range samples {
		if index > 0 {
			at = at.Add(time.Second)
			if source.IntN(200) == 0 {
				at = at.Add(time.Duration(11+source.IntN(120)) * time.Second)
			}
			distance += source.Float64() * 8
			altitude += (source.Float64() - 0.5) * 2
		}
		samples[index] = measure.Sample{At: at, DistanceMetres: distance, AltitudeMetres: altitude}
	}

	return samples
}

// randomCoordinates produces n coordinate pairs within valid latitude and
// longitude ranges, from a fixed seed so the corpus is reproducible.
func randomCoordinates(n int) []measure.Coordinate {
	//nolint:gosec // A fixed seed keeps a failure reproducible; nothing here is a secret.
	source := rand.New(rand.NewPCG(3, 4))
	coordinates := make([]measure.Coordinate, n)
	for index := range coordinates {
		coordinates[index] = measure.Coordinate{
			Latitude:  source.Float64()*180 - 90,
			Longitude: source.Float64()*360 - 180,
		}
	}

	return coordinates
}
