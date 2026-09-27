package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/wahoo"
)

// A ride is one outdoor ride as the study holds it: its track, what it had in
// common with every route it came near, and what production decides for it.
type ride struct {
	directions map[int]activity.Direction // by candidate, filled as asked
	track      []measure.Coordinate
	coverages  []coverage
	production activity.RouteMatch
	matched    bool
}

func (r *ride) direction(candidate int, candidates []activity.RouteCandidate) activity.Direction {
	if held, ok := r.directions[candidate]; ok {
		return held
	}
	direction := directionOf(r.track, candidates[candidate].Geometry)
	r.directions[candidate] = direction

	return direction
}

// A bandVariant is which of the two shares a candidate band loosens.
type bandVariant struct {
	name      string
	routeOnly bool
	rideOnly  bool
}

func (v bandVariant) shares(band float64) (routeBand, rideBand float64) {
	routeBand, rideBand = band, band
	if v.routeOnly {
		rideBand = productionBand
	}
	if v.rideOnly {
		routeBand = productionBand
	}

	return routeBand, rideBand
}

func variants() []bandVariant {
	return []bandVariant{
		{name: "both"},
		{name: "route only", routeOnly: true},
		{name: "ride only", rideOnly: true},
	}
}

// A report is counts and distributions only: no ride, route or position.
type report struct {
	rides         []ride
	candidates    []activity.RouteCandidate
	bands         []float64
	storedMatches int
}

// study measures every outdoor ride that has a track against the whole
// library, without the acceptance band, beside what production records.
func study(ctx context.Context, store *sqlite.Store, target string, bands []float64) (*report, error) {
	target, err := targetOf(ctx, store, target)
	if err != nil {
		return nil, err
	}
	candidates, _, err := store.LibraryRoutes(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading the library: %w", err)
	}
	activities, err := store.ActivitiesBetween(ctx, target, time.Unix(0, 0), time.Now().Add(24*time.Hour), math.MaxInt32)
	if err != nil {
		return nil, fmt.Errorf("listing rides: %w", err)
	}
	stored, err := store.ActivityRouteMatches(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("reading stored matches: %w", err)
	}

	result := &report{candidates: candidates, bands: bands, storedMatches: len(stored)}
	library := measure.NewSnapIndex(geometries(candidates), corridorMetres)
	matcher := activity.NewRouteMatcher(candidates)
	for index := range activities {
		if slices.Contains(wahoo.IndoorWorkoutTypes(), activities[index].TypeID) {
			continue
		}
		points, trackErr := store.ActivityTrack(ctx, target, activities[index].ID)
		if trackErr != nil {
			return nil, fmt.Errorf("reading a ride's track: %w", trackErr)
		}
		track := make([]measure.Coordinate, 0, len(points))
		for _, point := range points {
			track = append(track, measure.Coordinate{Latitude: point.Latitude, Longitude: point.Longitude})
		}
		if len(track) < 2 {
			continue
		}
		production, matched := matcher.Match(track)
		result.rides = append(result.rides, ride{
			track: track, coverages: coveragesOf(track, library, candidates),
			production: production, matched: matched, directions: map[int]activity.Direction{},
		})
	}

	return result, nil
}

func geometries(candidates []activity.RouteCandidate) [][]measure.Coordinate {
	lines := make([][]measure.Coordinate, 0, len(candidates))
	for _, candidate := range candidates {
		lines = append(lines, candidate.Geometry)
	}

	return lines
}

// targetOf is the target named, or the only one the database holds.
func targetOf(ctx context.Context, store *sqlite.Store, named string) (string, error) {
	if named != "" {
		return named, nil
	}
	rides, err := store.RecordedRides(ctx)
	if err != nil {
		return "", fmt.Errorf("listing recorded rides: %w", err)
	}
	target := ""
	for _, recorded := range rides {
		if target != "" && recorded.TargetID != target {
			return "", errors.New("the database holds several targets: name one with -target")
		}
		target = recorded.TargetID
	}
	if target == "" {
		return "", errors.New("the database holds no recorded ride")
	}

	return target, nil
}

// coverageBins are the lower edges the unmatched rides' best shares are
// counted under, highest first.
var coverageBins = [...]float64{0.9, 0.8, 0.7, 0.6, 0.5, 0.4, 0.3, 0.2, 0.1, 0} //nolint:gochecknoglobals // the report's fixed layout, read-only.

func binOf(share float64) int {
	for index, edge := range coverageBins {
		if share >= edge {
			return index
		}
	}

	return len(coverageBins) - 1
}

func (r *report) String() string {
	var b strings.Builder
	agree, matched := 0, 0
	for index := range r.rides {
		held := &r.rides[index]
		best, cleared := winner(held.coverages, r.candidates, productionBand, productionBand)
		if held.matched {
			matched++
		}
		if held.matched == (cleared > 0) && (!held.matched || r.candidates[best.candidate].Key == held.production.Key) {
			agree++
		}
	}
	fmt.Fprintf(&b, "corpus: %d outdoor rides with a track, %d library stages\n", len(r.rides), len(r.candidates))
	fmt.Fprintf(&b, "matched: %d stored, %d by the production matcher against this library, %d unmatched\n",
		r.storedMatches, matched, len(r.rides)-matched)
	fmt.Fprintf(&b, "agreement: this study's copy at the %.2f band decides %d of %d rides as production does\n",
		productionBand, agree, len(r.rides))

	r.writeUnmatched(&b)
	r.writeBands(&b)

	return b.String()
}

// writeUnmatched is the best each production-unmatched ride could have done
// against any one route: both shares held together, and each alone.
func (r *report) writeUnmatched(b *strings.Builder) {
	var joint, routeShare, rideShare [len(coverageBins)]int
	untouched := 0
	var joints []float64
	for index := range r.rides {
		held := &r.rides[index]
		if held.matched {
			continue
		}
		if len(held.coverages) == 0 {
			untouched++
			continue
		}
		bestJoint, bestRoute, bestRide := 0.0, 0.0, 0.0
		for _, share := range held.coverages {
			bestJoint = math.Max(bestJoint, share.joint())
			bestRoute = math.Max(bestRoute, share.route)
			bestRide = math.Max(bestRide, share.ride)
		}
		joint[binOf(bestJoint)]++
		routeShare[binOf(bestRoute)]++
		rideShare[binOf(bestRide)]++
		joints = append(joints, bestJoint)
	}
	fmt.Fprintf(b, "\nunmatched rides, best share against any one route (%d came within %.0f m of no route)\n",
		untouched, corridorMetres)
	fmt.Fprintf(b, "  %-10s %8s %8s %8s\n", "share", "joint", "route", "ride")
	for index, edge := range coverageBins {
		upper := 1.0
		if index > 0 {
			upper = coverageBins[index-1]
		}
		fmt.Fprintf(b, "  %.1f-%.1f   %8d %8d %8d\n", edge, upper, joint[index], routeShare[index], rideShare[index])
	}
	if len(joints) > 0 {
		sort.Float64s(joints)
		fmt.Fprintf(b, "  best joint share: median %.2f, q3 %.2f, max %.2f over %d rides\n",
			joints[(len(joints)-1)/2], joints[(3*len(joints)-1)/4], joints[len(joints)-1], len(joints))
	}
}

// writeBands is what each candidate band does: rides it newly matches, rides
// it leaves clearing more than one route, production matches it moves to
// another route, and how many of its matches have a direction.
func (r *report) writeBands(b *strings.Builder) {
	fmt.Fprintf(b, "\ncandidate bands (production %.2f on both shares)\n", productionBand)
	fmt.Fprintf(b, "  %-5s %-11s %8s %8s %9s %8s %10s\n", "band", "loosens", "matched", "new", "ambiguous", "moved", "direction")
	for _, band := range append([]float64{productionBand}, r.bands...) {
		for _, variant := range variants() {
			if band == productionBand && variant.name != "both" {
				continue
			}
			routeBand, rideBand := variant.shares(band)
			var matched, fresh, ambiguous, moved, directed int
			for index := range r.rides {
				held := &r.rides[index]
				best, cleared := winner(held.coverages, r.candidates, routeBand, rideBand)
				if cleared == 0 {
					continue
				}
				matched++
				if cleared > 1 {
					ambiguous++
				}
				switch {
				case !held.matched:
					fresh++
				case r.candidates[best.candidate].Key != held.production.Key:
					moved++
				}
				if held.direction(best.candidate, r.candidates) != activity.DirectionUnknown {
					directed++
				}
			}
			fmt.Fprintf(b, "  %.2f  %-11s %8d %8d %9d %8d %10d\n", band, variant.name, matched, fresh, ambiguous, moved, directed)
		}
	}
}
