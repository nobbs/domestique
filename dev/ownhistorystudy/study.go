// Package main answers #584: for a route the rider has already ridden more
// than once, does a recency-weighted median of their own earlier attempts
// predict the next one's moving time better than the pooled ride model —
// and from how many attempts?
//
// Development tooling, not part of the shipped binary and never run in quick
// or check: it needs the operator's own snapshot of real rides. Its report is
// aggregate numbers only — no ride identifier, date, position or route
// name — and is safe to paste into an issue.
package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/wahoo"
)

// corpus is the store surface this study reads. A subset of *sqlite.Store,
// named here so the pure walking logic below never imports sqlite itself.
type corpus interface {
	RecordedRides(ctx context.Context) ([]recordedRide, error)
	ActivityListings(ctx context.Context, targetID string) (listings []activity.Listing, readAt time.Time, err error)
	ActivityRouteMatches(ctx context.Context, targetID string) (map[int64]activity.RouteMatch, error)
	ActivityRides(ctx context.Context, since time.Time, workoutTypeIDs []int) ([]ridemodel.Ride, error)
}

// recordedRide mirrors sqlite.RecordedRide without importing it, so this
// package's own type is what the interface above speaks.
type recordedRide struct {
	TargetID       string
	WorkoutID      int64
	AscentMetres   float64
	DistanceMetres float64
	MovingSeconds  float64
}

// datedRide is one matched ride on one route, sorted set built per (target,
// route) group.
type datedRide struct {
	StartedAt      time.Time
	MovingSeconds  float64
	DistanceMetres float64
	AscentMetres   float64
}

// routeGroupKey scopes own-history to one rider's own attempts at one route
// ridden the same way round: two targets riding the same route never share a
// history, and neither do a forward and a reverse attempt — climbs become
// descents and the other way round, so the two are not one distribution.
type routeGroupKey struct {
	TargetID  string
	Route     route.Key
	Direction activity.Direction
}

// buildGroups joins a target's recorded totals against its route matches and
// listing dates, keeping only rides the matcher already scored as a full
// match — RouteCoverage's own threshold, not repeated here.
func buildGroups(ctx context.Context, store corpus) (map[routeGroupKey][]datedRide, error) {
	rides, err := store.RecordedRides(ctx)
	if err != nil {
		return nil, fmt.Errorf("reading recorded rides: %w", err)
	}
	byTarget := map[string][]recordedRide{}
	for _, ride := range rides {
		byTarget[ride.TargetID] = append(byTarget[ride.TargetID], ride)
	}

	groups := map[routeGroupKey][]datedRide{}
	for targetID, targetRides := range byTarget {
		listings, _, err := store.ActivityListings(ctx, targetID)
		if err != nil {
			return nil, fmt.Errorf("reading activity listings: %w", err)
		}
		startedAt := make(map[int64]time.Time, len(listings))
		for _, listing := range listings {
			startedAt[listing.ID] = listing.Starts
		}

		matches, err := store.ActivityRouteMatches(ctx, targetID)
		if err != nil {
			return nil, fmt.Errorf("reading route matches: %w", err)
		}

		for _, ride := range targetRides {
			match, matched := matches[ride.WorkoutID]
			when, dated := startedAt[ride.WorkoutID]
			if !matched || !dated {
				continue
			}
			key := routeGroupKey{TargetID: targetID, Route: match.Key, Direction: match.Direction}
			groups[key] = append(groups[key], datedRide{
				StartedAt: when, MovingSeconds: ride.MovingSeconds,
				DistanceMetres: ride.DistanceMetres, AscentMetres: ride.AscentMetres,
			})
		}
	}
	for key := range groups {
		slices.SortFunc(groups[key], func(a, b datedRide) int { return a.StartedAt.Compare(b.StartedAt) })
	}

	return groups, nil
}

// weightedMedianSeconds halves a ride's weight in the median every
// halfLifeDays before asOf, so a rider's pace this season outweighs one from
// long before without being dropped outright.
func weightedMedianSeconds(rides []datedRide, asOf time.Time, halfLifeDays float64) float64 {
	type weighted struct{ value, weight float64 }
	items := make([]weighted, len(rides))
	total := 0.0
	for index, ride := range rides {
		days := asOf.Sub(ride.StartedAt).Hours() / 24
		weight := math.Pow(0.5, days/halfLifeDays)
		items[index] = weighted{ride.MovingSeconds, weight}
		total += weight
	}
	slices.SortFunc(items, func(a, b weighted) int { return cmp.Compare(a.value, b.value) })
	cumulative := 0.0
	for _, item := range items {
		cumulative += item.weight
		if cumulative >= total/2 {
			return item.value
		}
	}

	return items[len(items)-1].value
}

// pooledPredict fits the pooled model from every ride recorded before asOf —
// the same corpus calibrate() draws from, unwindowed here since Fit's own
// trainingWindow applies the trailing-window-with-extension rule against
// whatever it is given. A fold too thin, or too degenerate, to fit predicts
// from Default() instead, standing in for "leaves the pair in force" without
// tracking a running coefficient state across folds.
func pooledPredict(pool []ridemodel.Ride, asOf time.Time, distanceMetres, ascentMetres float64) float64 {
	coefficients, err := ridemodel.Fit(pool, asOf)
	if err != nil {
		coefficients = ridemodel.Default()
	}

	return coefficients.SecondsPerKM*(distanceMetres/1000) + coefficients.SecondsPerAscentM*ascentMetres
}

// fold is one held-out ride scored against both methods.
type fold struct {
	priorRideCount    int
	ownHistoryPercent float64
	pooledPercent     float64
}

// walk scores every ride with at least one earlier attempt on the same
// route, by the rider who rode it, against both the pooled model as of that
// ride's own date and a recency-weighted median of the rides before it.
func walk(groups map[routeGroupKey][]datedRide, pool []ridemodel.Ride, halfLifeDays float64) []fold {
	var folds []fold
	for _, rides := range groups {
		for index := 1; index < len(rides); index++ {
			held := rides[index]
			prior := rides[:index]

			ownHistory := weightedMedianSeconds(prior, held.StartedAt, halfLifeDays)
			pooled := pooledPredict(pool, held.StartedAt, held.DistanceMetres, held.AscentMetres)

			folds = append(folds, fold{
				priorRideCount:    len(prior),
				ownHistoryPercent: (ownHistory - held.MovingSeconds) / held.MovingSeconds * 100,
				pooledPercent:     (pooled - held.MovingSeconds) / held.MovingSeconds * 100,
			})
		}
	}

	return folds
}

// bucket summarises one prior-ride-count group's errors for both methods.
type bucket struct {
	priorRideCount                               int
	folds                                        int
	ownHistoryBias, ownHistoryMAE, ownHistoryP90 float64
	pooledBias, pooledMAE, pooledP90             float64
}

// summarize buckets folds by how many prior rides own-history had, capping
// the bucket key at 5-or-more so a long tail of well-ridden routes does not
// spread the report across dozens of near-empty rows.
func summarize(folds []fold) []bucket {
	const capAt = 5
	byCount := map[int][]fold{}
	for _, f := range folds {
		key := min(f.priorRideCount, capAt)
		byCount[key] = append(byCount[key], f)
	}

	buckets := make([]bucket, 0, len(byCount))
	for count, group := range byCount {
		own := make([]float64, len(group))
		pooled := make([]float64, len(group))
		var ownBias, pooledBias float64
		for index, f := range group {
			own[index], pooled[index] = f.ownHistoryPercent, f.pooledPercent
			ownBias += f.ownHistoryPercent / float64(len(group))
			pooledBias += f.pooledPercent / float64(len(group))
		}
		buckets = append(buckets, bucket{
			priorRideCount: count,
			folds:          len(group),
			ownHistoryBias: ownBias, ownHistoryMAE: mae(own), ownHistoryP90: p90(own),
			pooledBias: pooledBias, pooledMAE: mae(pooled), pooledP90: p90(pooled),
		})
	}
	slices.SortFunc(buckets, func(a, b bucket) int { return cmp.Compare(a.priorRideCount, b.priorRideCount) })

	return buckets
}

func mae(percentErrors []float64) float64 {
	sum := 0.0
	for _, v := range percentErrors {
		sum += math.Abs(v)
	}
	if len(percentErrors) == 0 {
		return 0
	}

	return sum / float64(len(percentErrors))
}

func p90(percentErrors []float64) float64 {
	absolute := make([]float64, len(percentErrors))
	for index, v := range percentErrors {
		absolute[index] = math.Abs(v)
	}
	slices.Sort(absolute)
	if len(absolute) == 0 {
		return 0
	}
	position := 0.9 * float64(len(absolute)-1)
	lower, upper := int(math.Floor(position)), int(math.Ceil(position))
	if lower == upper {
		return absolute[lower]
	}
	weight := position - float64(lower)

	return absolute[lower]*(1-weight) + absolute[upper]*weight
}

func report(buckets []bucket, totalGroups, eligibleGroups int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "routes with >=2 matched rides by the same target: %d of %d route/target pairs ridden at all\n", eligibleGroups, totalGroups)
	fmt.Fprintf(&b, "%-18s %6s | %8s %8s %8s | %8s %8s %8s\n",
		"prior own rides", "folds", "own bias", "own MAE", "own p90", "pool bias", "pool MAE", "pool p90")
	for _, bucket := range buckets {
		label := fmt.Sprintf("%d", bucket.priorRideCount)
		if bucket.priorRideCount == 5 {
			label = "5+"
		}
		fmt.Fprintf(&b, "%-18s %6d | %7.1f%% %7.1f%% %7.1f%% | %7.1f%% %7.1f%% %7.1f%%\n",
			label, bucket.folds,
			bucket.ownHistoryBias, bucket.ownHistoryMAE, bucket.ownHistoryP90,
			bucket.pooledBias, bucket.pooledMAE, bucket.pooledP90)
	}

	return b.String()
}

var errNoEligibleRoutes = errors.New("ownhistorystudy: no route was ridden more than once by the rider who rode it")

func study(ctx context.Context, store corpus, halfLifeDays float64) (string, error) {
	groups, err := buildGroups(ctx, store)
	if err != nil {
		return "", err
	}
	eligible := 0
	for _, rides := range groups {
		if len(rides) >= 2 {
			eligible++
		}
	}
	if eligible == 0 {
		return "", errNoEligibleRoutes
	}

	pool, err := store.ActivityRides(ctx, time.Time{}, wahoo.OutdoorHumanPoweredWorkoutTypes())
	if err != nil {
		return "", fmt.Errorf("reading the pooled corpus: %w", err)
	}

	folds := walk(groups, pool, halfLifeDays)
	buckets := summarize(folds)

	return report(buckets, len(groups), eligible), nil
}
