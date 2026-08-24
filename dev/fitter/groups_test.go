package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func ridesFor(gear string, n int) []rideRow {
	rides := make([]rideRow, n)
	for i := range n {
		rides[i] = rideRow{RideID: gear + string(rune('a'+i)), Gear: gear}
	}

	return rides
}

func TestGroupRidesByGearFitsEachTaggedGroupSeparately(t *testing.T) {
	rides := append(ridesFor("Bike A", 12), ridesFor("Bike B", 15)...)

	groups := groupRidesByGear(rides)
	require.Len(t, groups, 2)
	for _, g := range groups {
		assert.False(t, g.Skipped)
		assert.False(t, g.UntaggedAttributed)
	}
}

func TestGroupRidesByGearSkipsAGroupThatIsTooSmall(t *testing.T) {
	rides := append(ridesFor("Bike A", 12), ridesFor("Bike B", 3)...)

	groups := groupRidesByGear(rides)
	var small *rideGroup
	for i := range groups {
		if groups[i].Gear == "Bike B" {
			small = &groups[i]
		}
	}
	require.NotNil(t, small)
	assert.True(t, small.Skipped)
	assert.NotEmpty(t, small.SkipReason)
}

func TestGroupRidesByGearSkipsUntaggedWhenAnotherGroupExists(t *testing.T) {
	rides := append(ridesFor("Bike A", 12), ridesFor(untaggedGear, 12)...)

	groups := groupRidesByGear(rides)
	var untagged *rideGroup
	for i := range groups {
		if groups[i].Gear == untaggedGear {
			untagged = &groups[i]
		}
	}
	require.NotNil(t, untagged)
	assert.True(t, untagged.Skipped)
	assert.False(t, untagged.UntaggedAttributed)
}

func TestGroupRidesByGearFitsUntaggedWhenItIsTheOnlyGroup(t *testing.T) {
	rides := ridesFor(untaggedGear, 12)

	groups := groupRidesByGear(rides)
	require.Len(t, groups, 1)
	assert.False(t, groups[0].Skipped)
	assert.True(t, groups[0].UntaggedAttributed)
}
