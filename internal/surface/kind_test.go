package surface

import "testing"

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
			if got := Classify(map[string]string{"surface": test.surface}); got != test.want {
				t.Errorf("Classify(surface=%q) = %v, want %v", test.surface, got, test.want)
			}
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
			if got := Classify(test.tags); got != test.want {
				t.Errorf("Classify(%v) = %v, want %v", test.tags, got, test.want)
			}
		})
	}
}

// TestClassifyInfersNothingFromTheHighwayTag guards the deliberate refusal to
// guess: a street is usually sealed and a footpath usually is not, but neither
// has been surveyed here and both must read as unsurveyed.
func TestClassifyInfersNothingFromTheHighwayTag(t *testing.T) {
	for _, highway := range []string{"residential", "primary", "cycleway", "footway", "path", "service"} {
		t.Run(highway, func(t *testing.T) {
			if got := Classify(map[string]string{"highway": highway}); got != KindUnknown {
				t.Errorf("Classify(highway=%q) = %v, want %v", highway, got, KindUnknown)
			}
		})
	}
}

func TestClassifyHandlesAbsentTags(t *testing.T) {
	if got := Classify(nil); got != KindUnknown {
		t.Errorf("Classify(nil) = %v, want %v", got, KindUnknown)
	}
	if got := Classify(map[string]string{}); got != KindUnknown {
		t.Errorf("Classify(empty) = %v, want %v", got, KindUnknown)
	}
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
			if got := test.kind.String(); got != test.want {
				t.Errorf("Kind(%d).String() = %q, want %q", test.kind, got, test.want)
			}
		})
	}
}
