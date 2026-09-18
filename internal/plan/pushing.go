package plan

import "github.com/nobbs/domestique/internal/route"

// Routed is what the routing engine answers for a set of waypoints: the line,
// and the ways it runs along.
type Routed struct {
	Points []route.Point
	// Ways are the stretches of the line in order, each ending EndMetres along it
	// by the engine's own reckoning, with the tags of the way it runs on. Empty
	// where the engine says nothing about them.
	Ways []RoutedWay
}

// RoutedWay is one stretch of a routed line and the tags of the way under it.
type RoutedWay struct {
	Tags      map[string]string
	EndMetres float64
}

// Window is a stretch of a plan, in metres from its start.
type Window struct {
	StartMetres float64
	EndMetres   float64
}

// pushed reports whether a rider walks a way: the engine refuses it to bicycles
// and routes along it only because feet are allowed. It is BRouter's own
// bikeaccess and footaccess, so a stretch is pushed exactly when the engine
// priced it as pushing.
func pushed(tags map[string]string) bool {
	return !bikeAccess(tags) && footAccess(tags)
}

func defaultAccess(tags map[string]string) bool {
	switch tags["access"] {
	case "":
		return tags["motorroad"] != "yes"
	case "private", "no":
		return false
	default:
		return true
	}
}

func bikeAccess(tags map[string]string) bool {
	switch bicycle := tags["bicycle"]; {
	case bicycle != "":
		return bicycle != "private" && bicycle != "no" && bicycle != "dismount" && bicycle != "use_sidepath"
	case tags["bicycle_road"] == "yes":
		return true
	case tags["vehicle"] == "":
		// A footway with no word on bicycles is a footway: not for riding.
		return tags["highway"] != "footway" && defaultAccess(tags)
	default:
		return tags["vehicle"] != "private" && tags["vehicle"] != "no"
	}
}

func footAccess(tags map[string]string) bool {
	switch foot := tags["foot"]; {
	case tags["bicycle"] == "dismount":
		return true
	case foot == "":
		return defaultAccess(tags)
	default:
		return foot != "private" && foot != "no" && foot != "use_sidepath"
	}
}

// pushingOf folds the pushed ways into the windows a rider walks, scaled onto a
// line totalMetres long: the engine measures its own length, which the
// normalised line the plan reports differs from by a little.
func pushingOf(ways []RoutedWay, totalMetres float64) []Window {
	if len(ways) == 0 {
		return nil
	}
	engineMetres := ways[len(ways)-1].EndMetres
	if engineMetres <= 0 || totalMetres <= 0 {
		return nil
	}
	scale := totalMetres / engineMetres
	var windows []Window
	start := 0.0
	for _, way := range ways {
		if pushed(way.Tags) {
			if last := len(windows) - 1; last >= 0 && windows[last].EndMetres == start*scale {
				windows[last].EndMetres = way.EndMetres * scale
			} else {
				windows = append(windows, Window{StartMetres: start * scale, EndMetres: way.EndMetres * scale})
			}
		}
		start = way.EndMetres
	}

	return windows
}
