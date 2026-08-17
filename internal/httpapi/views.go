package httpapi

import "encoding/json"

// The v1 JSON contract uses snake_case throughout. These view types are
// deliberately separate from persistence and adapter structs so a storage
// change cannot silently alter the wire format.

type targetView struct {
	ID            string `json:"id"`
	Authorization string `json:"authorisation"`
}

type stageView struct {
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	RouteName string `json:"route_name"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	StageName string `json:"stage_name"`
	Title     string `json:"title"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	SourceRevision string `json:"source_revision"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	ContentHash string `json:"content_hash"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	DistanceMetres float64 `json:"distance_metres"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	AscentMetres float64 `json:"ascent_metres"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	MaxGradientPercent float64 `json:"max_gradient_percent"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	RouteID int64 `json:"route_id"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	PointCount int `json:"point_count"`
	StageOrder int `json:"stage"`
}

type syncView struct {
	State string `json:"state"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastCompletedAt string `json:"last_completed_at,omitempty"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastResult string `json:"last_result,omitempty"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	SourceStages int `json:"source_stages"`
	Created      int `json:"created"`
	Updated      int `json:"updated"`
	Deleted      int `json:"deleted"`
}

type statusView struct {
	Targets []targetView `json:"targets"`
	Sync    syncView     `json:"sync"`
	Ready   bool         `json:"ready"`
}

type webUIConfigView struct {
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	TileStyleURL string `json:"tile_style_url"`
}

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
	Ranges json.RawMessage `json:"ranges"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	MatchedMetres float64 `json:"matched_metres"`
}

type geometryPropertyView struct {
	Surface *geometrySurfaceView `json:"surface,omitempty"`
	Title   string               `json:"title"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	RouteName string `json:"route_name"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	StageName string `json:"stage_name"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	DistanceMetres float64 `json:"distance_metres"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	AscentMetres float64 `json:"ascent_metres"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	MaxGradientPercent float64 `json:"max_gradient_percent"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	RouteID int64 `json:"route_id"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	PointCount int `json:"point_count"`
	StageOrder int `json:"stage"`
}
