// Package demo builds a synthetic route library: stages, surfaces, targets and
// runs that exercise the service the way a rider's own data would, computed from
// arithmetic instead of copied from anybody.
//
// It exists so that developing the browser UI, and testing it, needs neither a
// snapshot of a deployed database nor an account with a provider. Every value
// here is a function of the constants in this file, so the same library appears
// on every machine and in every test run, and a fixture can be asserted against
// by name rather than by whatever happened to be in somebody's SQLite file.
//
// The coordinates are placed over real ground, because a basemap under an empty
// ocean shows a developer nothing. They are still generated: no route in here
// was ever planned or ridden.
package demo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// Where the synthetic rides start, and the flat-earth scale used to walk away
// from it. Over the tens of kilometres a stage covers the error is metres, which
// is smaller than the spacing between two stored points and irrelevant to
// anything a fixture is asked to demonstrate.
const (
	originLatitude  = 48.40
	originLongitude = 8.10

	metresPerDegreeLatitude = 111_320.0
)

// How densely a stage is stored, and the bounds either side of it: a stage is
// sampled about every stored-point interval a planner emits, but a two-kilometre
// stage still needs enough points to draw a shape and a sixty-kilometre one does
// not need thousands to look like a ride.
const (
	pointSpacingMetres = 25.0
	minimumPoints      = 48
	maximumPoints      = 2_400
)

// shape is how a stage's ground plan is drawn.
type shape uint8

const (
	// shapeLine runs away from its start and finishes somewhere else.
	shapeLine shape = iota
	// shapeLoop returns to within a few metres of its start.
	shapeLoop
	// shapeOutAndBack rides out and retraces the same ground home.
	shapeOutAndBack
)

// elevationCoverage is how much of a stage carries a height.
type elevationCoverage uint8

const (
	// elevationEverywhere is the stage a source profiled from end to end.
	elevationEverywhere elevationCoverage = iota
	// elevationNowhere is the stage a source stored flat, with no height at all.
	// It is not a claim that the ground is flat, and the UI must not present it
	// as one.
	elevationNowhere
	// elevationPartial is the stage whose profile stops partway. It is the case
	// that breaks a naive gradient, because a height beside a hole is not a
	// slope.
	elevationPartial
)

// band is one run of ground, as a fraction of the stage, and what it is made of.
type band struct {
	untilFraction float64
	kind          surface.Kind
}

// stageSpec is one synthetic stage, before it becomes geometry.
//
// spanMetres is the ground the stage covers, not the distance a rider would
// record: the drawn line wanders either side of its bearing, so the ridden
// length comes out something like a tenth longer. That is the right way round —
// a fixture's distance should be whatever the geometry actually measures, since
// that is the number every reader of it derives.
type stageSpec struct {
	routeName      string
	stageName      string
	bands          []band
	routeID        int64
	spanMetres     float64
	bearingDegrees float64
	startEastwards float64
	startNorthward float64
	stageOrder     int
	climbMetres    float64
	climbCycles    float64
	shape          shape
	elevation      elevationCoverage
}

// specs is the library. Between them the stages carry a multi-stage route, a
// loop, an out-and-back, the longest and shortest rides the UI has to lay out,
// a stage with no profile and one with half a profile, gradients from flat to
// steep, every surface class including unknown, a stage that was never
// classified at all, and one whose classification covers only part of its
// length.
func specs() []stageSpec {
	return []stageSpec{
		{
			routeID: 4101, stageOrder: 1,
			routeName: "Synthetic Rhine Traverse", stageName: "Valley floor",
			shape: shapeLine, spanMetres: 58_400, bearingDegrees: 22,
			climbMetres: 240, climbCycles: 3.5, elevation: elevationEverywhere,
			bands: []band{
				{untilFraction: 0.62, kind: surface.KindAsphalt},
				{untilFraction: 0.78, kind: surface.KindPaving},
				{untilFraction: 1, kind: surface.KindCompacted},
			},
		},
		{
			routeID: 4101, stageOrder: 2,
			routeName: "Synthetic Rhine Traverse", stageName: "Forest ramps",
			shape: shapeLine, spanMetres: 31_900, bearingDegrees: 118,
			startEastwards: 21_800, startNorthward: 54_100,
			climbMetres: 780, climbCycles: 2, elevation: elevationEverywhere,
			bands: []band{
				{untilFraction: 0.35, kind: surface.KindAsphalt},
				{untilFraction: 0.7, kind: surface.KindGravel},
				{untilFraction: 1, kind: surface.KindGround},
			},
		},
		{
			routeID: 4101, stageOrder: 3,
			routeName: "Synthetic Rhine Traverse", stageName: "Run to the border",
			shape: shapeLine, spanMetres: 12_600, bearingDegrees: 205,
			startEastwards: 50_000, startNorthward: 39_100,
			climbMetres: 60, climbCycles: 1, elevation: elevationEverywhere,
			bands: []band{{untilFraction: 1, kind: surface.KindAsphalt}},
		},
		{
			routeID: 4102, stageOrder: 1,
			routeName: "Synthetic Kaiserstuhl Loop", stageName: "",
			shape: shapeLoop, spanMetres: 44_500,
			startEastwards: -18_400, startNorthward: 6_200,
			climbMetres: 910, climbCycles: 2.5, elevation: elevationEverywhere,
			bands: []band{
				{untilFraction: 0.28, kind: surface.KindAsphalt},
				{untilFraction: 0.44, kind: surface.KindCompacted},
				{untilFraction: 0.58, kind: surface.KindGravel},
				{untilFraction: 0.71, kind: surface.KindGround},
				{untilFraction: 0.86, kind: surface.KindPaving},
				{untilFraction: 1, kind: surface.KindAsphalt},
			},
		},
		{
			routeID: 4103, stageOrder: 1,
			routeName: "Synthetic Station Link", stageName: "",
			shape: shapeLine, spanMetres: 2_350, bearingDegrees: 267,
			startEastwards: 4_100, startNorthward: -7_600,
			elevation: elevationNowhere,
			// No bands at all: this stage has never been classified, which is a
			// different thing from being classified as unknown and has to look
			// different.
		},
		{
			routeID: 4104, stageOrder: 1,
			routeName: "Synthetic Summit Ascent", stageName: "",
			shape: shapeOutAndBack, spanMetres: 18_200, bearingDegrees: 74,
			startEastwards: -9_800, startNorthward: -14_500,
			climbMetres: 1_120, climbCycles: 0.5, elevation: elevationPartial,
			bands: []band{
				{untilFraction: 0.24, kind: surface.KindAsphalt},
				{untilFraction: 0.66, kind: surface.KindUnknown},
				{untilFraction: 1, kind: surface.KindCompacted},
			},
		},
	}
}

// Stages returns the synthetic source inventory.
func Stages() ([]route.Stage, error) {
	specs := specs()
	stages := make([]route.Stage, 0, len(specs))
	for index := range specs {
		spec := &specs[index]
		geometry := spec.geometry()
		stage, err := route.NewStage(
			route.ProviderVeloPlanner,
			spec.routeID,
			spec.stageOrder,
			Revision(spec.routeID, spec.stageOrder),
			spec.routeName,
			spec.stageName,
			geometry,
			contentHash(spec, geometry),
		)
		if err != nil {
			return nil, fmt.Errorf("demo: building stage %d/%d: %w", spec.routeID, spec.stageOrder, err)
		}
		stages = append(stages, stage)
	}

	return stages, nil
}

// Classification is one stage's stored surface, in the form the state database
// holds and the geometry endpoint serves.
type Classification struct {
	ContentHash   string
	Ranges        []byte
	MatchedMetres float64
	RouteID       int64
	StageOrder    int
}

// Classifications returns the stored surface for every stage that has one.
//
// The ranges are compressed and encoded by the same code that stores a real
// classification, and the matched length is measured the same way, so a fixture
// cannot drift into a shape the production reader would not accept.
func Classifications(stages []route.Stage) ([]Classification, error) {
	specs := specs()
	classifications := make([]Classification, 0, len(specs))
	for index := range specs {
		spec := &specs[index]
		if len(spec.bands) == 0 {
			continue
		}
		stage := &stages[index]
		geometry := stage.Geometry()
		kinds := spec.kinds(len(geometry))
		encoded, err := surface.EncodeRanges(surface.Compress(kinds))
		if err != nil {
			return nil, fmt.Errorf("demo: encoding surface for %d/%d: %w", spec.routeID, spec.stageOrder, err)
		}
		classifications = append(classifications, Classification{
			ContentHash:   stage.ContentHash(),
			Ranges:        encoded,
			MatchedMetres: surface.MatchedMetres(geometry, kinds),
			RouteID:       spec.routeID,
			StageOrder:    spec.stageOrder,
		})
	}

	return classifications, nil
}

// Revision is the source revision a stage is stored at. Convergence compares
// revisions and nothing else, so a fixture that wants a target to look behind
// writes an earlier one rather than inventing a mismatch the reader ignores.
func Revision(routeID int64, stageOrder int) string {
	return fmt.Sprintf("demo-%d-%d-r2", routeID, stageOrder)
}

// EarlierRevision is the same stage one planning revision ago.
func EarlierRevision(routeID int64, stageOrder int) string {
	return fmt.Sprintf("demo-%d-%d-r1", routeID, stageOrder)
}

// kinds assigns a class to every point from the spec's bands.
func (s *stageSpec) kinds(points int) []surface.Kind {
	kinds := make([]surface.Kind, points)
	for index := range kinds {
		fraction := 0.0
		if points > 1 {
			fraction = float64(index) / float64(points-1)
		}
		kinds[index] = s.bands[len(s.bands)-1].kind
		for _, band := range s.bands {
			if fraction <= band.untilFraction {
				kinds[index] = band.kind

				break
			}
		}
	}

	return kinds
}

// geometry draws the stage's ground plan and hangs a profile on it.
func (s *stageSpec) geometry() []route.Point {
	count := int(math.Round(s.spanMetres / pointSpacingMetres))
	count = max(min(count, maximumPoints), minimumPoints)

	points := make([]route.Point, count)
	for index := range points {
		fraction := float64(index) / float64(count-1)
		eastwards, northward := s.offsetMetres(fraction)
		points[index] = route.Point{
			Longitude: originLongitude + (s.startEastwards+eastwards)/metresPerDegreeLongitude(originLatitude),
			Latitude:  originLatitude + (s.startNorthward+northward)/metresPerDegreeLatitude,
			Elevation: s.elevationAt(fraction),
		}
	}

	return points
}

// offsetMetres is how far along and across the stage's start one point lies.
//
// A line leaves on its bearing and wanders either side of it, so a drawn route
// bends like a road rather than reading as a ruler. A loop is a closed curve
// whose circumference is the stage length. An out-and-back walks the line to
// halfway and retraces it, which is what makes its two ends the same point
// without its shape being a loop.
func (s *stageSpec) offsetMetres(fraction float64) (eastwards, northward float64) {
	if s.shape == shapeLoop {
		// A slightly lobed circle: a perfect one reads as a drawing rather than
		// as ground somebody routed over.
		radius := s.spanMetres / (2 * math.Pi)
		angle := 2 * math.Pi * fraction
		wobble := 1 + 0.18*math.Sin(3*angle)

		return radius * wobble * math.Sin(angle), radius * wobble * (1 - math.Cos(angle))
	}

	along := s.spanMetres * fraction
	if s.shape == shapeOutAndBack {
		along = s.spanMetres * (0.5 - math.Abs(fraction-0.5))
	}
	drift := 0.06 * s.spanMetres * math.Sin(4*math.Pi*along/s.spanMetres)
	radians := s.bearingDegrees * math.Pi / 180

	return along*math.Sin(radians) + drift*math.Cos(radians),
		along*math.Cos(radians) - drift*math.Sin(radians)
}

// elevationAt is the height at one point, or nil where the source stored none.
func (s *stageSpec) elevationAt(fraction float64) *float64 {
	switch s.elevation {
	case elevationNowhere:
		return nil
	case elevationPartial:
		// A hole in the middle third, which is where a consumer that assumes a
		// height beside a height notices and one that assumes a complete profile
		// does not.
		if fraction > 0.34 && fraction < 0.66 {
			return nil
		}
	case elevationEverywhere:
	}

	height := 180 + s.climbMetres*(0.5-0.5*math.Cos(2*math.Pi*s.climbCycles*fraction))

	return &height
}

// contentHash is a stable digest of everything a stage is made of, standing in
// for the one the source computes. Nothing reads it as anything but an opaque
// identity, but it still has to change when the stage does, or a re-seeded
// library would keep a surface measured against geometry that has moved.
func contentHash(spec *stageSpec, geometry []route.Point) string {
	// Assembled as bytes rather than written to the hash directly, so a fixture's
	// identity does not depend on an ignored write error.
	payload := fmt.Appendf(nil, "demo\x00%d\x00%d\x00%s\x00%s\x00",
		spec.routeID, spec.stageOrder, spec.routeName, spec.stageName)
	for index := range geometry {
		point := &geometry[index]
		elevation := math.NaN()
		if point.Elevation != nil {
			elevation = *point.Elevation
		}
		payload = fmt.Appendf(payload, "%.6f,%.6f,%.1f;",
			point.Longitude, point.Latitude, elevation)
	}
	digest := sha256.Sum256(payload)

	return hex.EncodeToString(digest[:])
}

// metresPerDegreeLongitude is the east-west scale at one latitude.
func metresPerDegreeLongitude(latitude float64) float64 {
	return metresPerDegreeLatitude * math.Cos(latitude*math.Pi/180)
}
