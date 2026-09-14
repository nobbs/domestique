package activity

import (
	"cmp"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"slices"
	"sort"

	"github.com/nobbs/domestique/internal/measure"
)

// routeClockSpacingMetres is how far along its route a ride goes between two
// readings of its clock. See docs/specs/measurement.md §Ahead of prediction.
const routeClockSpacingMetres = 100

// RouteClock is a ride's moving time read along the route it was matched to.
type RouteClock struct {
	// Line fingerprints the route line the readings were placed along, whose
	// projected frame their metres are in.
	Line     string            `json:"line"`
	Readings []RouteClockPoint `json:"readings"`
}

// RouteClockPoint is one reading of a ride's clock against its route: the
// ride's own moving time on reaching a place along the route.
type RouteClockPoint struct {
	// Sample is the index of the positioned sample the reading was taken at.
	Sample int `json:"i"`
	// AlongMetres is in the route's snap frame; see measure.SnapHit.
	AlongMetres   float64 `json:"a"`
	MovingSeconds float64 `json:"s"`
}

// ReadRouteClock reads a ride's moving time every routeClockSpacingMetres of
// progress along the route it was matched to, in the order ridden.
//
// Only a ride that ran the way the route is stored has one: a route's predicted
// time is not symmetric, so a ride the other way round is not compared against
// it. track and series are the same ride's positioned samples, indexed 1:1.
func ReadRouteClock(
	line []measure.Coordinate, track []TrackPoint, series []SampleRow, direction Direction,
) *RouteClock {
	if direction != DirectionForward || len(track) < 2 || len(series) != len(track) {
		return nil
	}
	index := measure.NewSnapIndex([][]measure.Coordinate{line}, corridorMetres)
	var (
		readings []RouteClockPoint
		moving   float64
		next     float64
		placed   = -1.0
		odometer *SampleRow
	)
	for sample := range track {
		if row := &series[sample]; row.DistanceMetres.Known {
			// A step longer than a recording gap is a pause, however far the
			// odometer crept across it.
			if odometer != nil && row.Time.Sub(odometer.Time) <= measure.DefaultMaxGap {
				moving += movingSecondsBetween(odometer, row)
			}
			odometer = row
		}
		at, onRoute := placeAlong(index, &track[sample], placed)
		if !onRoute {
			continue
		}
		placed = at
		if at < next {
			continue
		}
		readings = append(readings, RouteClockPoint{Sample: sample, AlongMetres: at, MovingSeconds: moving})
		next = (math.Floor(at/routeClockSpacingMetres) + 1) * routeClockSpacingMetres
	}
	// Without an odometer there is no telling moving from standing still.
	if odometer == nil || len(readings) == 0 {
		return nil
	}

	return &RouteClock{Line: lineFingerprint(line), Readings: readings}
}

// placeAlong is where along the route one sample fell. Where the corridor holds
// the route more than once, as a loop's finish beside its start does, each
// pass offers its nearest point, and the one nearest the last place is taken,
// or before any the earliest.
func placeAlong(index *measure.SnapIndex, point *TrackPoint, placed float64) (float64, bool) {
	hits := index.Near(measure.Coordinate{Latitude: point.Latitude, Longitude: point.Longitude})
	slices.SortFunc(hits, func(one, other measure.SnapHit) int {
		return cmp.Compare(one.DistanceMetres, other.DistanceMetres)
	})
	var passes []float64
	for _, hit := range hits {
		// Neighbouring segments of one pass lie within the corridor of the same
		// point; a place further along than twice its width is another pass.
		if !slices.ContainsFunc(passes, func(along float64) bool {
			return math.Abs(along-hit.AlongMetres) <= 2*corridorMetres
		}) {
			passes = append(passes, hit.AlongMetres)
		}
	}
	if len(passes) == 0 {
		return 0, false
	}
	if placed < 0 {
		return slices.Min(passes), true
	}

	return slices.MinFunc(passes, func(one, other float64) int {
		return cmp.Compare(math.Abs(one-placed), math.Abs(other-placed))
	}), true
}

// AheadOfPrediction is how far ahead of a route's predicted moving time a ride
// was, in seconds, at each of samples positioned samples: the predicted time
// from where the ride joined the route to where it had reached, less its own
// moving time over the same. Samples before the first reading and after the
// last read nothing.
//
// cumulative is the predicted moving time at each coordinate of line; ok is
// false where there is no prediction for this line, or no clock read along it.
func AheadOfPrediction(
	line []measure.Coordinate, cumulative []float64, clock *RouteClock, samples int,
) (readings []Reading, ok bool) {
	if len(line) < 2 || len(cumulative) != len(line) || clock == nil || len(clock.Readings) == 0 ||
		clock.Line != lineFingerprint(line) {
		return nil, false
	}
	index := measure.NewSnapIndex([][]measure.Coordinate{line}, corridorMetres)
	vertexAlong := make([]float64, len(line))
	for vertex := 1; vertex < len(line); vertex++ {
		east, north := index.Offset(line[vertex-1], line[vertex])
		vertexAlong[vertex] = vertexAlong[vertex-1] + math.Hypot(east, north)
	}
	readings = make([]Reading, samples)
	// Both clocks start where the ride joined the route, so riding to its start
	// is not read as time lost on it.
	first := clock.Readings[0]
	predictedFrom := interpolate(vertexAlong, cumulative, first.AlongMetres)
	previous, previousAhead := -1, 0.0
	for _, point := range clock.Readings {
		if point.Sample < 0 || point.Sample >= samples || point.Sample <= previous {
			return nil, false
		}
		ahead := interpolate(vertexAlong, cumulative, point.AlongMetres) - predictedFrom -
			(point.MovingSeconds - first.MovingSeconds)
		readings[point.Sample] = Reading{Value: ahead, Known: true}
		for sample := previous + 1; previous >= 0 && sample < point.Sample; sample++ {
			share := float64(sample-previous) / float64(point.Sample-previous)
			readings[sample] = Reading{Value: previousAhead + share*(ahead-previousAhead), Known: true}
		}
		previous, previousAhead = point.Sample, ahead
	}

	return readings, true
}

// lineFingerprint identifies a route line by its positions alone.
func lineFingerprint(line []measure.Coordinate) string {
	digest := sha256.New()
	var buffer [16]byte
	for _, coordinate := range line {
		binary.LittleEndian.PutUint64(buffer[:8], math.Float64bits(coordinate.Longitude))
		binary.LittleEndian.PutUint64(buffer[8:], math.Float64bits(coordinate.Latitude))
		// hash.Hash.Write never returns an error.
		_, _ = digest.Write(buffer[:])
	}

	return hex.EncodeToString(digest.Sum(nil)[:16])
}

// interpolate reads ys at x along the ascending xs, holding the end values
// beyond either end.
func interpolate(xs, ys []float64, x float64) float64 {
	upper := min(max(sort.SearchFloat64s(xs, x), 1), len(xs)-1)
	lower := upper - 1
	span := xs[upper] - xs[lower]
	if span <= 0 {
		return ys[upper]
	}
	share := math.Min(1, math.Max(0, (x-xs[lower])/span))

	return ys[lower] + share*(ys[upper]-ys[lower])
}
