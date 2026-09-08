package activity

import "github.com/nobbs/domestique/internal/measure"

// splitAscentHysteresisMetres is the rise a climb must reach before a stretch
// counts it, and the fall that ends one: the walk a head unit does over the
// same barometric samples, to within a few per cent on the rider's own rides
// (docs/specs/measurement.md, Ascent and descent).
const splitAscentHysteresisMetres = 3

// Split is one fixed stretch of a ride, as a splits table reads it. Every
// figure is of that stretch alone. A ride's last split is whatever was left
// over, which is what DistanceMetres says. Its ascent is counted with
// hysteresis over the stretch's own samples, so a climb that straddles two
// stretches is counted in each from where that stretch found it, and a
// sample with no height breaks the series rather than bridging it.
type Split struct {
	DistanceMetres float64
	MovingSeconds  float64
	AscentMetres   float64
	HeartRateBPM   Reading
	PowerWatts     Reading
}

// Splits divides a ride into stretches of the given length, in the order they
// were ridden.
//
// The bicycle's own odometer decides where a stretch ends rather than the
// distance between positions: a table that disagreed with the ride's stated
// distance would be wrong in the only terms its reader has. A pair of samples
// the odometer did not advance over is a rider standing still, so its seconds
// are not counted as moving ones.
func Splits(rows []SampleRow, everyMetres float64) []Split {
	if everyMetres <= 0 {
		return nil
	}
	var (
		splits  []Split
		current splitParts
		origin  float64
		last    *SampleRow
		covered float64
	)
	for index := range rows {
		row := &rows[index]
		if !row.DistanceMetres.Known {
			continue
		}
		if last == nil {
			origin, last = row.DistanceMetres.Value, row

			continue
		}
		covered = row.DistanceMetres.Value - origin
		// A step belongs to the stretch it began in; a sample landing several
		// stretches on leaves the ones it skipped empty rather than merged.
		current.add(last, row)
		last = row
		for covered >= float64(len(splits)+1)*everyMetres {
			splits = append(splits, current.close(everyMetres))
			current = splitParts{}
		}
	}
	if rest := covered - float64(len(splits))*everyMetres; rest > 0 {
		splits = append(splits, current.close(rest))
	}

	return splits
}

// splitParts accumulates one stretch as the samples over it arrive.
type splitParts struct {
	// runs are the stretch's altitude series, one per unbroken run of samples
	// that carried a height, since a climb must not be read across a gap.
	runs          [][]float64
	movingSeconds float64
	heartRate     mean
	power         mean
}

// add folds the step from one sample to the next into the stretch it began in.
func (p *splitParts) add(previous, current *SampleRow) {
	// The same pair `speedSeries` refuses to report a speed for: a clock that
	// did not advance, or went backwards over a correction, times nothing.
	seconds := current.Time.Sub(previous.Time).Seconds()
	if seconds > 0 && current.DistanceMetres.Value > previous.DistanceMetres.Value {
		p.movingSeconds += seconds
	}
	// The first sample the stretch began from opens its series where it
	// carried a height; a sample without one ends the run, and the next that
	// carries one starts another.
	if len(p.runs) == 0 && previous.AltitudeMetres.Known {
		p.runs = append(p.runs, []float64{previous.AltitudeMetres.Value})
	}
	switch {
	case !current.AltitudeMetres.Known:
		if last := len(p.runs) - 1; last >= 0 && len(p.runs[last]) > 0 {
			p.runs = append(p.runs, nil)
		}
	case len(p.runs) == 0:
		p.runs = append(p.runs, []float64{current.AltitudeMetres.Value})
	default:
		last := len(p.runs) - 1
		p.runs[last] = append(p.runs[last], current.AltitudeMetres.Value)
	}
	p.heartRate.add(current.HeartRateBPM)
	p.power.add(current.PowerWatts)
}

func (p splitParts) close(distanceMetres float64) Split {
	return Split{
		DistanceMetres: distanceMetres,
		MovingSeconds:  p.movingSeconds,
		AscentMetres:   p.ascentMetres(),
		HeartRateBPM:   p.heartRate.reading(),
		PowerWatts:     p.power.reading(),
	}
}

// ascentMetres is the hysteresis walk over each unbroken run of heights,
// summed: a gap contributes nothing and hides nothing either side of it.
func (p splitParts) ascentMetres() float64 {
	total := 0.0
	for _, run := range p.runs {
		total += measure.AscentWithHysteresisMetres(run, splitAscentHysteresisMetres)
	}

	return total
}

// mean averages the samples of one series that carried a reading at all.
type mean struct {
	sum   float64
	count int
}

func (m *mean) add(reading Reading) {
	if reading.Known {
		m.sum += reading.Value
		m.count++
	}
}

// reading is the mean, or nothing at all where no sample carried the series.
func (m mean) reading() Reading {
	if m.count == 0 {
		return Reading{}
	}

	return Reading{Value: m.sum / float64(m.count), Known: true}
}
