package main

import "sort"

// minGroupRides is the fewest rides a gear group needs before this package
// will fit coefficients for it. Below it, a fit exists mostly to overfit
// whatever few rides happened to be on that bike — reported and skipped
// rather than merged into a neighbouring group, which would attribute
// coefficients to equipment that did not produce them.
//
// Note: a fixed threshold, not derived from the regression's own standard
// error; revisit if a real corpus reports a group just above it with a
// visibly unstable fit.
const minGroupRides = 10

// untaggedGear is the gear partition key for a ride whose export row carries
// no gear at all — Strava writes an empty field, not a sentinel string.
const untaggedGear = ""

// rideGroup is one gear partition: the rides in it, and why it will or will
// not be fitted.
type rideGroup struct {
	RideIDs            map[string]bool
	Gear               string
	SkipReason         string
	Skipped            bool
	UntaggedAttributed bool
}

// groupRidesByGear partitions rides by their gear identifier — never by name
// or date, which the issue calls out as exactly the constant that goes stale
// the next time equipment changes. Groups are returned in a stable order:
// tagged gears first (alphabetically), then untagged, so a run's report and
// its output files are reproducible.
func groupRidesByGear(rides []rideRow) []rideGroup {
	counts := make(map[string]int)
	rideIDs := make(map[string]map[string]bool)
	for _, r := range rides {
		counts[r.Gear]++
		if rideIDs[r.Gear] == nil {
			rideIDs[r.Gear] = make(map[string]bool)
		}
		rideIDs[r.Gear][r.RideID] = true
	}

	var gears []string
	for gear := range counts {
		gears = append(gears, gear)
	}
	sort.Slice(gears, func(i, j int) bool {
		if gears[i] == untaggedGear {
			return false
		}
		if gears[j] == untaggedGear {
			return true
		}

		return gears[i] < gears[j]
	})

	onlyGroup := len(gears) == 1

	groups := make([]rideGroup, 0, len(gears))
	for _, gear := range gears {
		group := rideGroup{Gear: gear, RideIDs: rideIDs[gear]}
		switch {
		case counts[gear] < minGroupRides:
			group.Skipped = true
			group.SkipReason = "fewer than the minimum rides for a group fit"
		case gear == untaggedGear && !onlyGroup:
			group.Skipped = true
			group.SkipReason = "untagged rides are not attributed to a specific bike"
		case gear == untaggedGear && onlyGroup:
			group.UntaggedAttributed = true
		}
		groups = append(groups, group)
	}

	return groups
}
