package main

import "sort"

// minGroupRides is the fewest rides a gear group needs before this package will
// fit coefficients for it. Below it a fit mostly overfits whatever few rides were
// on that bike; such a group is reported and skipped rather than merged into a
// neighbour. A fixed threshold, not derived from the regression's standard error.
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

// groupRidesByGear partitions rides by their gear identifier, never by name or
// date, which go stale the next time equipment changes. Groups are returned in a
// stable order — tagged gears alphabetically, then untagged — so a run's report
// and output files are reproducible.
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
