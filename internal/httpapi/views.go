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

// optionalIntervalSeconds omits a task with no fixed gap between runs rather
// than serving a zero that would read as one running every instant.
func optionalIntervalSeconds(interval time.Duration) *int {
	seconds := int(interval.Round(time.Second) / time.Second)
	if seconds <= 0 {
		return nil
	}

	return &seconds
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
// package. What stays here is the geometry Feature, whose coordinates, surface
// ranges and cumulative seconds are served as json.RawMessage straight from
// storage rather than decoded and re-encoded on every request.

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
	// stored. They tile the whole geometry rather than only its surveyed parts: an
	// unsurveyed stretch travels as an `unknown` range in its own place.
	// MatchedMetres alone answers how much of the stage the classes describe.
	Ranges        json.RawMessage `json:"ranges"`
	MatchedMetres float64         `json:"matchedMetres"`
}

type geometryPropertyView struct {
	Surface         *geometrySurfaceView `json:"surface,omitempty"`
	Provider        string               `json:"provider"`
	Title           string               `json:"title"`
	SourceRouteName string               `json:"sourceRouteName"`
	RouteName       string               `json:"routeName"`
	// CumulativeSeconds is the predicted moving time at each coordinate, indexed
	// 1:1 with the feature's coordinates and passed through as stored. Absent —
	// never zero-filled — whenever nothing has predicted this exact geometry.
	CumulativeSeconds  json.RawMessage `json:"cumulativeSeconds,omitempty"`
	DistanceMetres     float64         `json:"distanceMetres"`
	AscentMetres       float64         `json:"ascentMetres"`
	DescentMetres      float64         `json:"descentMetres"`
	MaxGradientPercent float64         `json:"maxGradientPercent"`
	SourceRouteID      int64           `json:"sourceRouteId"`
	PointCount         int             `json:"pointCount"`
	StageOrder         int             `json:"stageOrder"`
}
