package httpapi

import (
	"encoding/json"
	"time"
)

// wireTime renders an instant the way the contract declares every timestamp:
// UTC, at second resolution, so one instant has one spelling regardless of the
// timezone the service runs in.
func wireTime(at time.Time) time.Time {
	return at.UTC().Truncate(time.Second)
}

// optionalTime is wireTime for a field the contract marks optional. A zero
// instant is the absent case, which is a different fact from an instant that
// happens to be the epoch.
func optionalTime(at time.Time) *time.Time {
	if at.IsZero() {
		return nil
	}
	rendered := wireTime(at)

	return &rendered
}

// optionalBool omits a false rather than serving it, which is what the
// ",omitempty" on a plain bool field used to do. Absent and false mean the
// same thing to the page in every place this is used.
func optionalBool(value bool) *bool {
	if !value {
		return nil
	}

	return &value
}

// optionalString omits an empty string rather than serving one, which is what
// the ",omitempty" on a plain string field used to do before these shapes came
// from the contract.
func optionalString(value string) *string {
	if value == "" {
		return nil
	}

	return &value
}

// boolValue and stringValue read an omitted field back as the value its
// absence stands for, which is the direction a submitted body travels.
func boolValue(value *bool) bool {
	return value != nil && *value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}

	return *value
}

// The wire shapes come from api/openapi.yaml through the generated contract
// package, so there is one definition of every response body. What stays here
// is the geometry Feature, which cannot: its coordinates, surface ranges, and
// cumulative seconds are served as json.RawMessage straight from storage, and
// the generated shapes would decode and re-encode every one of them on every
// request.

// geometryView is a GeoJSON Feature carrying one stage's stored geometry. The
// coordinates pass through as stored, so no decode and re-encode is needed.
type geometryView struct {
	Type       string               `json:"type"`
	BBox       []float64            `json:"bbox"`
	Geometry   lineStringView       `json:"geometry"`
	Properties geometryPropertyView `json:"properties"`
}

type lineStringView struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

// geometrySurfaceView reports what is known about the ground a stage covers. It
// is one nullable group rather than loose fields so a client can tell "not
// classified" from "classified and nothing matched": the group is absent in the
// first case, and present with a matched length of zero in the second.
type geometrySurfaceView struct {
	// Ranges are index spans into this feature's coordinates, passed through as
	// stored so serving them costs no decode and re-encode.
	//
	// They tile the whole geometry rather than only its surveyed parts: an
	// unsurveyed stretch is a fact about the ground worth drawing, so it travels
	// as an `unknown` range in its own place along the line. What was surveyed
	// is MatchedMetres, and it alone answers how much of the stage the classes
	// actually describe.
	Ranges        json.RawMessage `json:"ranges"`
	MatchedMetres float64         `json:"matchedMetres"`
}

type geometryPropertyView struct {
	Surface   *geometrySurfaceView `json:"surface,omitempty"`
	Provider  string               `json:"provider"`
	Title     string               `json:"title"`
	RouteName string               `json:"routeName"`
	StageName string               `json:"stageName"`
	// CumulativeSeconds is the predicted moving time in seconds at each
	// coordinate, indexed 1:1 with the feature's coordinates and passed through
	// as stored so serving it costs no decode and re-encode. It is absent —
	// never a zero-filled array — whenever nothing has predicted this exact
	// geometry: a deployment with no coefficient file configured, a stage with
	// no usable elevation, or a prediction measured against geometry that has
	// since changed.
	CumulativeSeconds  json.RawMessage `json:"cumulativeSeconds,omitempty"`
	DistanceMetres     float64         `json:"distanceMetres"`
	AscentMetres       float64         `json:"ascentMetres"`
	MaxGradientPercent float64         `json:"maxGradientPercent"`
	RouteID            int64           `json:"routeId"`
	PointCount         int             `json:"pointCount"`
	StageOrder         int             `json:"stageOrder"`
}
