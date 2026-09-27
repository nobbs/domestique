package main

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
)

// line is n points about 36 m apart, starting at (lat, lon) and stepping by
// the given degrees each point.
func line(lat, lon, dLat, dLon float64, n int) []measure.Coordinate {
	points := make([]measure.Coordinate, n)
	for index := range points {
		points[index] = measure.Coordinate{Latitude: lat + dLat*float64(index), Longitude: lon + dLon*float64(index)}
	}

	return points
}

func library() []activity.RouteCandidate {
	return []activity.RouteCandidate{
		{Key: route.NewKey("veloplanner", 1, 0), Geometry: line(50, 8, 0, 0.0005, 60)},
		{Key: route.NewKey("veloplanner", 2, 0), Geometry: line(50.5, 8, 0.0003, 0, 60)},
		// The first 45 points of route 1 again: a shorter stage over the same road.
		{Key: route.NewKey("veloplanner", 3, 0), Geometry: line(50, 8, 0, 0.0005, 45)},
	}
}

func tracks() map[string][]measure.Coordinate {
	whole := line(50, 8, 0, 0.0005, 60)
	reversed := slices.Clone(whole)
	slices.Reverse(reversed)

	return map[string][]measure.Coordinate{
		"whole":     whole,
		"reversed":  reversed,
		"partial":   line(50, 8, 0, 0.0005, 38),
		"detour":    append(slices.Clone(whole), line(50.0003, 8.0295, 0.0003, 0, 25)...),
		"elsewhere": line(49, 7, 0, 0.0005, 60),
	}
}

// The copy of the matcher's measurement must decide every track as
// production does at production's own band, direction included.
func TestTheCopyAgreesWithProductionAtItsBand(t *testing.T) {
	t.Parallel()
	candidates := library()
	index := measure.NewSnapIndex(geometries(candidates), corridorMetres)
	for name, track := range tracks() {
		want, wantOK := activity.MatchRoute(track, candidates)

		best, cleared := winner(coveragesOf(track, index, candidates), candidates, productionBand, productionBand)

		require.Equal(t, wantOK, cleared > 0, name)
		if !wantOK {
			continue
		}
		assert.Equal(t, want.Key, candidates[best.candidate].Key, name)
		assert.InDelta(t, want.RouteCoverage, best.route, 1e-12, name)
		assert.InDelta(t, want.RideCoverage, best.ride, 1e-12, name)
		assert.Equal(t, want.Direction, directionOf(track, candidates[best.candidate].Geometry), name)
	}
}

func TestALooserBandMatchesAPartialRideAndContestsIt(t *testing.T) {
	t.Parallel()
	candidates := library()
	index := measure.NewSnapIndex(geometries(candidates), corridorMetres)
	coverages := coveragesOf(tracks()["partial"], index, candidates)

	_, atProduction := winner(coverages, candidates, productionBand, productionBand)
	best, loosened := winner(coverages, candidates, 0.65, 0.65)

	assert.Zero(t, atProduction, "most of the shorter stage is not the stage at the production band")
	assert.Equal(t, 2, loosened, "at 0.65 it clears both routes over the same road")
	assert.Equal(t, route.NewKey("veloplanner", 3, 0), candidates[best.candidate].Key,
		"the whole ride lies on both, so the route it covered more of wins")
}

func TestVariantsLoosenOneShareAndHoldTheOther(t *testing.T) {
	t.Parallel()
	byName := map[string]bandVariant{}
	for _, variant := range variants() {
		byName[variant.name] = variant
	}

	routeBand, rideBand := byName["route only"].shares(0.7)
	assert.InDelta(t, 0.7, routeBand, 1e-12)
	assert.InDelta(t, productionBand, rideBand, 1e-12)
	routeBand, rideBand = byName["ride only"].shares(0.7)
	assert.InDelta(t, productionBand, routeBand, 1e-12)
	assert.InDelta(t, 0.7, rideBand, 1e-12)
}

func TestReportCountsWithoutNamingRidesOrRoutes(t *testing.T) {
	t.Parallel()
	candidates := library()
	index := measure.NewSnapIndex(geometries(candidates), corridorMetres)
	r := report{candidates: candidates, bands: []float64{0.65}, storedMatches: 1}
	for _, name := range []string{"whole", "partial", "elsewhere"} {
		track := tracks()[name]
		production, matched := activity.MatchRoute(track, candidates)
		r.rides = append(r.rides, ride{
			track: track, coverages: coveragesOf(track, index, candidates),
			production: production, matched: matched, directions: map[int]activity.Direction{},
		})
	}

	out := r.String()

	assert.Contains(t, out, "matched: 1 stored, 1 by the production matcher against this library, 2 unmatched")
	assert.Contains(t, out, "decides 3 of 3 rides as production does")
	assert.Contains(t, out, "(1 came within 40 m of no route)")
	assert.Contains(t, out, "  0.8-0.9          1        1        0")
	assert.Contains(t, out, "  0.65  both               2        1         2        0          2")
	assert.NotContains(t, out, "veloplanner")
}

func TestBinOfPlacesEdgesInTheBinTheyOpen(t *testing.T) {
	t.Parallel()
	assert.Equal(t, 0, binOf(1))
	assert.Equal(t, 0, binOf(0.9))
	assert.Equal(t, 1, binOf(0.8999))
	assert.Equal(t, len(coverageBins)-1, binOf(0.05))
}

func TestParseBands(t *testing.T) {
	t.Parallel()
	bands, err := parseBands("0.9, 0.75")
	require.NoError(t, err)
	assert.Equal(t, []float64{0.9, 0.75}, bands)

	_, err = parseBands("0.9,1.5")
	assert.ErrorContains(t, err, "1.5")
}
