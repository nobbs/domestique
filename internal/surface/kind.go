// Package surface classifies the ground each part of a route stage is ridden
// on, from OpenStreetMap way tagging.
package surface

// Kind is a rideability class for one stretch of route.
//
// The classes are deliberately few. OpenStreetMap distinguishes several dozen
// surface values, most of which change nothing about how a stage rides: a rider
// picking tyres cares whether the ground is sealed, firm, loose, or soft, not
// whether the loose stuff was tagged gravel or pebblestone. Collapsing the
// values here keeps the legend small enough to hold in your head, and makes an
// unfamiliar tag degrade to the nearest class rather than to a colour nobody
// has seen before.
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

// Classify maps one way's OpenStreetMap tags to a rideability class.
//
// The surface tag decides wherever it is present. Where it is absent, tracktype
// fills in for tracks only: it grades how firm a track is without naming its
// material, which is the distinction that matters here and no help at all on a
// road.
//
// Nothing is inferred from the highway tag. A residential street is usually
// sealed and a woodland path usually is not, but "usually" would paint a
// confident colour over ground nobody has surveyed. An unsurveyed stretch is
// worth showing as unsurveyed, so it stays KindUnknown.
func Classify(tags map[string]string) Kind {
	if kind := classifySurface(tags["surface"]); kind != KindUnknown {
		return kind
	}
	if tags["highway"] == "track" {
		return classifyTrackType(tags["tracktype"])
	}

	return KindUnknown
}

// classifySurface maps the surface tag's value.
//
// Two of these are judgement calls rather than translations. The generic value
// "paved" means sealed without saying with what, and is mapped to KindAsphalt
// because that is what it nearly always turns out to be — at the cost of
// occasionally calling a cobbled lane smooth. The generic "unpaved" is mapped to
// KindGravel as the middle of the range it spans, so it errs towards loose
// rather than promising a firm surface that may be mud.
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
