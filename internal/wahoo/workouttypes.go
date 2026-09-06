package wahoo

// Wahoo's workout_type_id enumeration for the BIKING family
// (workout_type_family_id 0), from its published API reference. A type outside
// this list is a run, a swim, a strength session or another sport entirely.
const (
	WorkoutTypeBiking                   = 0
	WorkoutTypeBikingCyclecross         = 11
	WorkoutTypeBikingIndoor             = 12
	WorkoutTypeBikingMountain           = 13
	WorkoutTypeBikingRecumbent          = 14
	WorkoutTypeBikingRoad               = 15
	WorkoutTypeBikingTrack              = 16
	WorkoutTypeBikingMotocycling        = 17
	WorkoutTypeBikingIndoorCyclingClass = 49
	WorkoutTypeBikingIndoorTrainer      = 61
	WorkoutTypeEBiking                  = 64
	WorkoutTypeBikingIndoorVirtual      = 68
	WorkoutTypeHandcycling              = 70
)

// IsBikingWorkout reports whether a workout type belongs to Wahoo's BIKING
// family. Motorcycling rides under that family in Wahoo's own vocabulary, so it
// is one here too; what a caller may fit a ride model from is a narrower
// question that OutdoorHumanPoweredWorkoutTypes answers.
func IsBikingWorkout(workoutTypeID int) bool {
	switch workoutTypeID {
	case WorkoutTypeBiking, WorkoutTypeBikingCyclecross, WorkoutTypeBikingIndoor,
		WorkoutTypeBikingMountain, WorkoutTypeBikingRecumbent, WorkoutTypeBikingRoad,
		WorkoutTypeBikingTrack, WorkoutTypeBikingMotocycling, WorkoutTypeBikingIndoorCyclingClass,
		WorkoutTypeBikingIndoorTrainer, WorkoutTypeEBiking, WorkoutTypeBikingIndoorVirtual,
		WorkoutTypeHandcycling:
		return true
	}

	return false
}

// OutdoorHumanPoweredWorkoutTypes are the biking types ridden outdoors under
// the rider's own power. The indoor types report a virtual distance over no
// real ground, and motorcycling and e-biking carry a motor, so none of them
// price a rider's own pace against distance and ascent.
func OutdoorHumanPoweredWorkoutTypes() []int {
	return []int{
		WorkoutTypeBiking,
		WorkoutTypeBikingCyclecross,
		WorkoutTypeBikingMountain,
		WorkoutTypeBikingRecumbent,
		WorkoutTypeBikingRoad,
		WorkoutTypeBikingTrack,
	}
}
