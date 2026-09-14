package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(offset int) time.Time {
	return time.Date(2026, 1, 1, 6, 0, 0, 0, time.UTC).AddDate(0, 0, offset)
}

func TestWeightedMedianSecondsFavoursTheMostRecentRideAsHalfLifeShrinks(t *testing.T) {
	t.Parallel()
	// The most recent ride is an outlier: a short half-life should track it
	// alone, a long one should barely weigh recency and land on the classic
	// (unweighted) median of the three, 600.
	rides := []datedRide{
		{StartedAt: day(0), MovingSeconds: 500},
		{StartedAt: day(50), MovingSeconds: 600},
		{StartedAt: day(200), MovingSeconds: 5000},
	}

	short := weightedMedianSeconds(rides, day(200), 7)
	long := weightedMedianSeconds(rides, day(200), 36500)
	assert.InDelta(t, 5000, short, 1, "a short half-life should read as the recent outlier alone")
	assert.InDelta(t, 600, long, 1, "a long half-life should barely weigh recency, landing on the classic median")
}

func TestWeightedMedianSecondsOfOneRideIsThatRide(t *testing.T) {
	t.Parallel()
	got := weightedMedianSeconds([]datedRide{{StartedAt: day(0), MovingSeconds: 900}}, day(30), 90)
	assert.InDelta(t, 900, got, 1e-9)
}

// fakeCorpus is the smallest corpus a test can hand walk() through
// buildGroups: two targets, one route each, one shared with both.
type fakeCorpus struct {
	rides    map[string][]recordedRide
	listings map[string][]activity.Listing
	matches  map[string]map[int64]activity.RouteMatch
	pool     []ridemodel.Ride
}

func (f fakeCorpus) RecordedRides(context.Context) ([]recordedRide, error) {
	var all []recordedRide
	for _, rides := range f.rides {
		all = append(all, rides...)
	}

	return all, nil
}

func (f fakeCorpus) ActivityListings(
	_ context.Context, targetID string,
) ([]activity.Listing, time.Time, error) {
	return f.listings[targetID], time.Time{}, nil
}

func (f fakeCorpus) ActivityRouteMatches(
	_ context.Context, targetID string,
) (map[int64]activity.RouteMatch, error) {
	return f.matches[targetID], nil
}

func (f fakeCorpus) ActivityRides(context.Context, time.Time, []int) ([]ridemodel.Ride, error) {
	return f.pool, nil
}

func TestBuildGroupsScopesHistoryToOneTargetsOwnRoute(t *testing.T) {
	t.Parallel()
	key := route.NewKey("veloplanner", 12, 1)
	corpus := fakeCorpus{
		rides: map[string][]recordedRide{
			"rider-a": {
				{TargetID: "rider-a", WorkoutID: 1, DistanceMetres: 20000, MovingSeconds: 3600, AscentMetres: 200},
				{TargetID: "rider-a", WorkoutID: 2, DistanceMetres: 20000, MovingSeconds: 3400, AscentMetres: 200},
			},
			"rider-b": {
				{TargetID: "rider-b", WorkoutID: 3, DistanceMetres: 20000, MovingSeconds: 4000, AscentMetres: 200},
			},
		},
		listings: map[string][]activity.Listing{
			"rider-a": {{ID: 1, Starts: day(0)}, {ID: 2, Starts: day(10)}},
			"rider-b": {{ID: 3, Starts: day(5)}},
		},
		matches: map[string]map[int64]activity.RouteMatch{
			"rider-a": {1: {Key: key}, 2: {Key: key}},
			"rider-b": {3: {Key: key}},
		},
	}

	groups, err := buildGroups(t.Context(), corpus)
	require.NoError(t, err)
	require.Len(t, groups, 2, "one group per target on the shared route, not pooled across targets")
	riderA := groups[routeGroupKey{TargetID: "rider-a", Route: key}]
	require.Len(t, riderA, 2)
	assert.True(t, riderA[0].StartedAt.Before(riderA[1].StartedAt), "sorted oldest first")
}

// A climb ridden forward is a descent ridden in reverse: the two directions'
// moving times are not one distribution, and must not share a history.
func TestBuildGroupsSeparatesAForwardAttemptFromAReverseOne(t *testing.T) {
	t.Parallel()
	key := route.NewKey("veloplanner", 12, 1)
	corpus := fakeCorpus{
		rides: map[string][]recordedRide{
			"rider-a": {
				{TargetID: "rider-a", WorkoutID: 1, DistanceMetres: 20000, MovingSeconds: 3600},
				{TargetID: "rider-a", WorkoutID: 2, DistanceMetres: 20000, MovingSeconds: 3000},
			},
		},
		listings: map[string][]activity.Listing{
			"rider-a": {{ID: 1, Starts: day(0)}, {ID: 2, Starts: day(10)}},
		},
		matches: map[string]map[int64]activity.RouteMatch{
			"rider-a": {
				1: {Key: key, Direction: activity.DirectionForward},
				2: {Key: key, Direction: activity.DirectionReverse},
			},
		},
	}

	groups, err := buildGroups(t.Context(), corpus)
	require.NoError(t, err)
	require.Len(t, groups, 2, "forward and reverse attempts form separate groups")
	forward := groups[routeGroupKey{TargetID: "rider-a", Route: key, Direction: activity.DirectionForward}]
	reverse := groups[routeGroupKey{TargetID: "rider-a", Route: key, Direction: activity.DirectionReverse}]
	require.Len(t, forward, 1)
	require.Len(t, reverse, 1)
	assert.Equal(t, 3600.0, forward[0].MovingSeconds)
	assert.Equal(t, 3000.0, reverse[0].MovingSeconds)
}

func TestBuildGroupsSkipsAnUnmatchedOrUndatedRide(t *testing.T) {
	t.Parallel()
	corpus := fakeCorpus{
		rides: map[string][]recordedRide{
			"rider-a": {
				{TargetID: "rider-a", WorkoutID: 1, DistanceMetres: 20000, MovingSeconds: 3600},
				{TargetID: "rider-a", WorkoutID: 2, DistanceMetres: 20000, MovingSeconds: 3600},
			},
		},
		listings: map[string][]activity.Listing{
			"rider-a": {{ID: 1, Starts: day(0)}}, // workout 2 never listed
		},
		matches: map[string]map[int64]activity.RouteMatch{
			"rider-a": {1: {Key: route.NewKey("veloplanner", 1, 1)}}, // workout 2 never matched
		},
	}

	groups, err := buildGroups(t.Context(), corpus)
	require.NoError(t, err)
	total := 0
	for _, rides := range groups {
		total += len(rides)
	}
	assert.Equal(t, 1, total, "only the matched, dated ride is kept")
}

func TestWalkProducesOneFoldPerRideBeyondTheFirst(t *testing.T) {
	t.Parallel()
	key := route.NewKey("veloplanner", 1, 1)
	groups := map[routeGroupKey][]datedRide{
		{TargetID: "rider-a", Route: key}: {
			{StartedAt: day(0), MovingSeconds: 3600, DistanceMetres: 20000, AscentMetres: 200},
			{StartedAt: day(10), MovingSeconds: 3500, DistanceMetres: 20000, AscentMetres: 200},
			{StartedAt: day(20), MovingSeconds: 3400, DistanceMetres: 20000, AscentMetres: 200},
		},
	}

	folds := walk(groups, nil, 90)

	require.Len(t, folds, 2, "the first ride has no earlier attempt to fold on")
	assert.Equal(t, 1, folds[0].priorRideCount)
	assert.Equal(t, 2, folds[1].priorRideCount)
}

func TestStudyReportsNoEligibleRoutesWhenNoRouteWasRiddenTwice(t *testing.T) {
	t.Parallel()
	corpus := fakeCorpus{
		rides: map[string][]recordedRide{
			"rider-a": {{TargetID: "rider-a", WorkoutID: 1, DistanceMetres: 20000, MovingSeconds: 3600}},
		},
		listings: map[string][]activity.Listing{"rider-a": {{ID: 1, Starts: day(0)}}},
		matches: map[string]map[int64]activity.RouteMatch{
			"rider-a": {1: {Key: route.NewKey("veloplanner", 1, 1)}},
		},
	}

	_, err := study(t.Context(), corpus, 90)
	require.ErrorIs(t, err, errNoEligibleRoutes)
}

func TestReportNamesBothMethodsForEveryBucket(t *testing.T) {
	t.Parallel()
	buckets := []bucket{
		{priorRideCount: 1, folds: 4, ownHistoryMAE: 12.5, pooledMAE: 9.1},
		{priorRideCount: 5, folds: 2, ownHistoryMAE: 3.2, pooledMAE: 9.1},
	}

	got := report(buckets, 10, 6)

	assert.True(t, strings.Contains(got, "own bias") && strings.Contains(got, "pool bias"))
	assert.True(t, strings.Contains(got, "5+"), "the capped bucket reads as 5+, not 5")
}
