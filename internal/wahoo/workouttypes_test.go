package wahoo_test

import (
	"testing"

	"github.com/nobbs/domestique/internal/wahoo"
	"github.com/stretchr/testify/assert"
)

// The family is what a poll keeps, so a type Wahoo files under biking counts
// even where it is not something anyone would fit a model from.
func TestIsBikingWorkoutHoldsTheWholeFamily(t *testing.T) {
	t.Parallel()

	for _, workoutTypeID := range []int{0, 11, 12, 13, 14, 15, 16, 17, 49, 61, 64, 68, 70} {
		assert.True(t, wahoo.IsBikingWorkout(workoutTypeID), "type %d is in Wahoo's biking family", workoutTypeID)
	}
}

// A run, a swim or a strength session is not a ride, and neither is a type
// Wahoo has never issued.
func TestIsBikingWorkoutRefusesAnotherSport(t *testing.T) {
	t.Parallel()

	for _, workoutTypeID := range []int{1, 2, 3, 10, 18, 48, 50, 60, 62, 63, 65, 69, 71, 999, -1} {
		assert.False(t, wahoo.IsBikingWorkout(workoutTypeID), "type %d is not in Wahoo's biking family", workoutTypeID)
	}
}

// What prices a rider's pace is narrower than what a poll stores: an indoor
// distance is virtual, and a motor is not the rider.
func TestOutdoorHumanPoweredWorkoutTypesExcludeIndoorAndMotorAssisted(t *testing.T) {
	t.Parallel()

	types := wahoo.OutdoorHumanPoweredWorkoutTypes()
	assert.Equal(t, []int{0, 11, 13, 14, 15, 16}, types)

	for _, workoutTypeID := range types {
		assert.True(t, wahoo.IsBikingWorkout(workoutTypeID), "type %d must also be a biking type", workoutTypeID)
	}
	for _, excluded := range []int{
		wahoo.WorkoutTypeBikingIndoor, wahoo.WorkoutTypeBikingMotocycling,
		wahoo.WorkoutTypeBikingIndoorCyclingClass, wahoo.WorkoutTypeBikingIndoorTrainer,
		wahoo.WorkoutTypeEBiking, wahoo.WorkoutTypeBikingIndoorVirtual, wahoo.WorkoutTypeHandcycling,
	} {
		assert.NotContains(t, types, excluded)
	}
}

// The caller owns what it is given: a mutated result must not reach the next
// calibration as a different corpus.
func TestOutdoorHumanPoweredWorkoutTypesHandsOutAFreshSlice(t *testing.T) {
	t.Parallel()

	first := wahoo.OutdoorHumanPoweredWorkoutTypes()
	first[0] = 999

	assert.Equal(t, 0, wahoo.OutdoorHumanPoweredWorkoutTypes()[0])
}
