package route

// Turn is what a rider does at a cue.
type Turn string

// The turns a cue can ask for. Continuing straight on is one of them: a
// routing engine names it only where the way ahead is not obvious.
const (
	TurnStraight    Turn = "straight"
	TurnLeft        Turn = "left"
	TurnSlightLeft  Turn = "slight_left"
	TurnSharpLeft   Turn = "sharp_left"
	TurnRight       Turn = "right"
	TurnSlightRight Turn = "slight_right"
	TurnSharpRight  Turn = "sharp_right"
	TurnKeepLeft    Turn = "keep_left"
	TurnKeepRight   Turn = "keep_right"
	TurnUTurn       Turn = "u_turn"
	TurnRoundabout  Turn = "roundabout"
)

// Cue is one turn instruction along a route.
type Cue struct {
	Turn Turn
	// Metres is how far along the route the turn is.
	Metres float64
	// Exit is the roundabout exit to take, counted from the entry; zero for
	// every other turn.
	Exit int
}
