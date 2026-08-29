// Package surface classifies the ground each part of a route stage is ridden
// on, from OpenStreetMap way tagging.
package surface

// Kind is a rideability class for one stretch of route. The classes are few:
// OpenStreetMap distinguishes dozens of surface values, but a rider picking tyres
// cares whether the ground is sealed, firm, loose or soft. Collapsing them makes
// an unfamiliar tag degrade to the nearest class.
type Kind uint8

const (
	// KindUnknown means the ground was never recorded. It does not mean smooth.
	KindUnknown Kind = iota
	// KindAsphalt is sealed and fast.
	KindAsphalt
	// KindPaving is sealed but rough or slippery: paving stones, sett, bricks.
	KindPaving
	// KindCompacted is unpaved but firm enough to ride on road tyres.
	KindCompacted
	// KindGravel is unpaved and loose.
	KindGravel
	// KindGround is unpaved and soft: bare earth, grass, sand, mud.
	KindGround
)

// String returns the stable wire name of the class. It is the name the JSON
// contract carries, so it may not be changed to suit a display language.
func (k Kind) String() string {
	switch k {
	case KindAsphalt:
		return "asphalt"
	case KindPaving:
		return "paving"
	case KindCompacted:
		return "compacted"
	case KindGravel:
		return "gravel"
	case KindGround:
		return "ground"
	case KindUnknown:
		return "unknown"
	}

	return "unknown"
}

// Classify maps one way's OpenStreetMap tags to a rideability class. The surface
// tag decides wherever present; where absent, tracktype fills in for tracks only.
// Nothing is inferred from the highway tag: an unsurveyed stretch stays
// KindUnknown rather than being painted a confident colour.
func Classify(tags map[string]string) Kind {
	if kind := classifySurface(tags["surface"]); kind != KindUnknown {
		return kind
	}
	if tags["highway"] == "track" {
		return classifyTrackType(tags["tracktype"])
	}

	return KindUnknown
}

// classifySurface maps the surface tag's value. Two are judgement calls: generic
// "paved" maps to KindAsphalt, at the cost of calling a cobbled lane smooth, and
// generic "unpaved" to KindGravel as the middle of its range.
func classifySurface(value string) Kind {
	switch value {
	case "asphalt", "chipseal", "concrete", "concrete:lanes", "concrete:plates", "paved":
		return KindAsphalt
	case "paving_stones", "paving_stones:lanes", "sett", "cobblestone",
		"cobblestone:flattened", "unhewn_cobblestone", "bricks", "brick",
		"grass_paver", "metal", "wood":
		return KindPaving
	case "compacted", "fine_gravel":
		return KindCompacted
	case "gravel", "pebblestone", "rock", "stone", "shells", "unpaved":
		return KindGravel
	case "ground", "dirt", "earth", "grass", "mud", "sand", "woodchips", "snow", "ice":
		return KindGround
	}

	return KindUnknown
}

// classifyTrackType maps the tracktype grades onto firmness. grade1 is solid
// but, being untagged for surface here, is taken as well-compacted rather than
// sealed. The upper grades describe an increasing share of soft ground, and
// grade3 is already where a road bike stops being the right tool.
func classifyTrackType(value string) Kind {
	switch value {
	case "grade1":
		return KindCompacted
	case "grade2":
		return KindGravel
	case "grade3", "grade4", "grade5":
		return KindGround
	}

	return KindUnknown
}
