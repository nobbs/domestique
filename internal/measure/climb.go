package measure

// Climb is one sustained ascent within a track: the ground it covers and what it costs to ride.
//
// See docs/specs/measurement.md §Sustained climbs.
type Climb struct {
	StartMetres, EndMetres, DistanceMetres float64
	AscentMetres                           float64 // the rises within the climb only
	AverageGradePercent                    float64 // net rise over the climb's length, which a dip inside it can only lower
	MaxGradePercent                        float64 // the steepest window inside the climb
}

// climbRun is a stretch of the profile classified climbing or not, by index range.
type climbRun struct {
	climbing             bool
	startIndex, endIndex int
}

// Climbs finds the profile's sustained climbs in the order they are ridden: runs whose
// gradient, measured back over windowMetres, holds at or above minGradientPercent, with a run
// shorter than the window absorbed into the run before it, and a climb reported where a
// climbing run is at least a window long. Empty for a profile of no length.
//
// The browser's findClimbs (internal/webui/app/src/lib/climbs.ts) applies the same rule; the
// two are held together by a shared table of vectors in climbs.test.ts and climb_test.go.
//
// See docs/specs/measurement.md §Sustained climbs.
func Climbs(p Profile, windowMetres, minGradientPercent float64) []Climb {
	if p.LengthMetres() <= 0 || windowMetres <= 0 {
		return nil
	}
	distanceMetres, altitudeMetres := p.distanceMetres, p.altitudeMetres
	lastIndex := len(distanceMetres) - 1

	runs, gradients := signedRuns(p, windowMetres, minGradientPercent, lastIndex)
	runs = mergedRuns(runs, distanceMetres, windowMetres)

	var climbs []Climb
	for _, run := range runs {
		if run.climbing && distanceMetres[run.endIndex+1]-distanceMetres[run.startIndex] >= windowMetres {
			climbs = append(climbs, toClimb(run, distanceMetres, altitudeMetres, gradients))
		}
	}

	return climbs
}

// signedRuns classifies the profile into runs of climbing and non-climbing ground, by the
// gradient measured back over windowMetres — the same backward window GradientsPercent walks.
func signedRuns(p Profile, windowMetres, minGradientPercent float64, lastIndex int) (runs []climbRun, gradientPercent []float64) {
	gradientPercent, _ = p.GradientsPercent(windowMetres)

	for index := 1; index <= lastIndex; index++ {
		climbing := gradientPercent[index] >= minGradientPercent
		if last := len(runs) - 1; last >= 0 && runs[last].climbing == climbing {
			runs[last].endIndex = index - 1

			continue
		}
		runs = append(runs, climbRun{climbing: climbing, startIndex: index - 1, endIndex: index - 1})
	}

	return runs, gradientPercent
}

// mergedRuns absorbs runs too short to be sustained into the run before them.
func mergedRuns(runs []climbRun, distanceMetres []float64, windowMetres float64) []climbRun {
	var kept []climbRun
	for _, run := range runs {
		length := distanceMetres[run.endIndex+1] - distanceMetres[run.startIndex]
		last := len(kept) - 1
		if last >= 0 && length < windowMetres {
			kept[last].endIndex = run.endIndex

			continue
		}
		if last >= 0 && kept[last].climbing == run.climbing {
			kept[last].endIndex = run.endIndex

			continue
		}
		kept = append(kept, run)
	}

	return kept
}

func toClimb(run climbRun, distanceMetres, altitudeMetres, gradientPercent []float64) Climb {
	startMetres := distanceMetres[run.startIndex]
	endMetres := distanceMetres[run.endIndex+1]
	distanceCovered := endMetres - startMetres

	ascentMetres := 0.0
	for index := run.startIndex; index <= run.endIndex; index++ {
		if rise := altitudeMetres[index+1] - altitudeMetres[index]; rise > 0 {
			ascentMetres += rise
		}
	}

	maxGradePercent := 0.0
	for index := run.startIndex + 1; index <= run.endIndex+1; index++ {
		maxGradePercent = max(maxGradePercent, gradientPercent[index])
	}

	averageGradePercent := 0.0
	if distanceCovered > 0 {
		averageGradePercent = (altitudeMetres[run.endIndex+1] - altitudeMetres[run.startIndex]) / distanceCovered * 100
	}

	return Climb{
		StartMetres:         startMetres,
		EndMetres:           endMetres,
		DistanceMetres:      distanceCovered,
		AscentMetres:        ascentMetres,
		AverageGradePercent: averageGradePercent,
		MaxGradePercent:     maxGradePercent,
	}
}
