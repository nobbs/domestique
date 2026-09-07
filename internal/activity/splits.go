package activity

// Split is one fixed stretch of a ride, as a splits table reads it. Every
// figure is of that stretch alone. A ride's last split is whatever was left
// over, which is what DistanceMetres says.
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
	movingSeconds float64
	ascentMetres  float64
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
	if current.AltitudeMetres.Known && previous.AltitudeMetres.Known {
		// Raw positive steps, unsmoothed, exactly as a route's own gain is cut.
		if climb := current.AltitudeMetres.Value - previous.AltitudeMetres.Value; climb > 0 {
			p.ascentMetres += climb
		}
	}
	p.heartRate.add(current.HeartRateBPM)
	p.power.add(current.PowerWatts)
}

func (p splitParts) close(distanceMetres float64) Split {
	return Split{
		DistanceMetres: distanceMetres,
		MovingSeconds:  p.movingSeconds,
		AscentMetres:   p.ascentMetres,
		HeartRateBPM:   p.heartRate.reading(),
		PowerWatts:     p.power.reading(),
	}
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
