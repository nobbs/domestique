package surface

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyReadsTheSurfaceTag(t *testing.T) {
	tests := []struct {
		name    string
		surface string
		want    Kind
	}{
		{name: "asphalt is sealed", surface: "asphalt", want: KindAsphalt},
		{name: "concrete is sealed", surface: "concrete", want: KindAsphalt},
		{name: "the generic paved is taken as asphalt", surface: "paved", want: KindAsphalt},
		{name: "sett is rough sealed", surface: "sett", want: KindPaving},
		{name: "paving stones are rough sealed", surface: "paving_stones", want: KindPaving},
		{name: "wood is rough sealed", surface: "wood", want: KindPaving},
		{name: "compacted is firm unpaved", surface: "compacted", want: KindCompacted},
		{name: "fine gravel is firm unpaved", surface: "fine_gravel", want: KindCompacted},
		{name: "gravel is loose", surface: "gravel", want: KindGravel},
		{name: "the generic unpaved is taken as loose", surface: "unpaved", want: KindGravel},
		{name: "dirt is soft", surface: "dirt", want: KindGround},
		{name: "sand is soft", surface: "sand", want: KindGround},
		{name: "an unrecognised value stays unknown", surface: "moon_dust", want: KindUnknown},
		{name: "an empty value stays unknown", surface: "", want: KindUnknown},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, Classify(map[string]string{"surface": test.surface}),
				"Classify(surface=%q)", test.surface)
		})
	}
}

func TestClassifyFallsBackToTrackTypeForTracksOnly(t *testing.T) {
	tests := []struct {
		tags map[string]string
		name string
		want Kind
	}{
		{
			name: "a track graded solid is firm",
			tags: map[string]string{"highway": "track", "tracktype": "grade1"},
			want: KindCompacted,
		},
		{
			name: "a track graded mostly solid is loose",
			tags: map[string]string{"highway": "track", "tracktype": "grade2"},
			want: KindGravel,
		},
		{
			name: "a track graded soft is soft",
			tags: map[string]string{"highway": "track", "tracktype": "grade4"},
			want: KindGround,
		},
		{
			name: "the surface tag outranks the grade",
			tags: map[string]string{"highway": "track", "tracktype": "grade5", "surface": "asphalt"},
			want: KindAsphalt,
		},
		{
			name: "a grade on something that is not a track is ignored",
			tags: map[string]string{"highway": "path", "tracktype": "grade2"},
			want: KindUnknown,
		},
		{
			name: "an ungraded track stays unknown",
			tags: map[string]string{"highway": "track"},
			want: KindUnknown,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, Classify(test.tags), "Classify(%v)", test.tags)
		})
	}
}

// TestClassifyInfersNothingFromTheHighwayTag guards the deliberate refusal to
// guess: a street is usually sealed and a footpath usually is not, but neither
// has been surveyed here and both must read as unsurveyed.
func TestClassifyInfersNothingFromTheHighwayTag(t *testing.T) {
	for _, highway := range []string{"residential", "primary", "cycleway", "footway", "path", "service"} {
		t.Run(highway, func(t *testing.T) {
			assert.Equal(t, KindUnknown, Classify(map[string]string{"highway": highway}),
				"Classify(highway=%q)", highway)
		})
	}
}

func TestClassifyHandlesAbsentTags(t *testing.T) {
	assert.Equal(t, KindUnknown, Classify(nil), "Classify(nil)")
	assert.Equal(t, KindUnknown, Classify(map[string]string{}), "Classify(empty)")
}

// TestKindStringIsStable pins the wire names. Changing one of these silently
// changes the JSON contract, so the expected values are written out in full
// rather than derived from the constants.
func TestKindStringIsStable(t *testing.T) {
	tests := []struct {
		want string
		kind Kind
	}{
		{kind: KindUnknown, want: "unknown"},
		{kind: KindAsphalt, want: "asphalt"},
		{kind: KindPaving, want: "paving"},
		{kind: KindCompacted, want: "compacted"},
		{kind: KindGravel, want: "gravel"},
		{kind: KindGround, want: "ground"},
		{kind: Kind(200), want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			assert.Equal(t, test.want, test.kind.String(), "Kind(%d).String()", test.kind)
		})
	}
}
