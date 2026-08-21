package httpapi

import "encoding/json"

// The v1 JSON contract uses snake_case throughout. These view types are
// deliberately separate from persistence and adapter structs so a storage
// change cannot silently alter the wire format.

// targetView is one configured Wahoo account: whether it is onboarded, how much
// of the stored library has been written to it, and how its own last
// reconciliation ended.
//
// Everything here is derived from local state. The counts say what this service
// has successfully applied to that Wahoo account; they are not a claim about
// what any physical head unit has downloaded, which this service cannot observe.
type targetView struct {
	// LastRun is absent until this slot has been reconciled once, which is a
	// different state from having been reconciled and failed.
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastRun       *targetRunView `json:"last_run,omitempty"`
	ID            string         `json:"id"`
	Authorization string         `json:"authorisation"`
	// Convergence is the one word for this target: "current", "lagging",
	// "failed", or "unauthorized". It is a summary of the fields beside it, not
	// an extra fact.
	Convergence string           `json:"convergence"`
	Stages      targetStagesView `json:"stages"`
}

// targetStagesView counts stored stages against one target. Counts only: naming
// the stages would put route names on a status response that exists to be safe
// to look at.
type targetStagesView struct {
	Current int `json:"current"`
	// Pending is everything this target still owes the stored library: a stage
	// never written, a stage written at an older revision, and a stage the
	// target still carries that the library no longer has.
	Pending int `json:"pending"`
}

// targetRunView is one target's own last reconciliation, in the same vocabulary
// a run uses.
type targetRunView struct {
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	CompletedAt string `json:"completed_at"`
	Result      string `json:"result"`
	// Failure is the safe failure category, present only when that run did not
	// succeed. It is never provider text.
	Failure string `json:"failure,omitempty"`
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

// syncScheduleView reports which halves of a synchronization the timer may
// start. They are separate switches because they fail, and are wanted, for
// unrelated reasons.
type syncScheduleView struct {
	Source  bool `json:"source"`
	Targets bool `json:"targets"`
}

// phaseRunView is the last recorded run of one phase. Absent means that phase
// has not finished a run since the service learned to record them separately.
type phaseRunView struct {
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastCompletedAt string `json:"last_completed_at"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastResult string `json:"last_result"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastFailure string `json:"last_failure,omitempty"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	SourceStages int `json:"source_stages"`
	Created      int `json:"created"`
	Updated      int `json:"updated"`
	Deleted      int `json:"deleted"`
}

// syncRunView is one terminal run in the recorded history: which half it was,
// when it ended, how it ended, and what it moved. It names no route, carries no
// geometry, and quotes nothing a provider said — the failure is the same safe
// category the status response reports.
//
// Reference is the opaque name the run was recorded under. It is what a
// notification can say out loud, and what an operator matches this row against.
type syncRunView struct {
	Reference string `json:"reference"`
	Phase     string `json:"phase"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	CompletedAt string `json:"completed_at"`
	Result      string `json:"result"`
	Failure     string `json:"failure,omitempty"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	SourceStages int `json:"source_stages"`
	Created      int `json:"created"`
	Updated      int `json:"updated"`
	Deleted      int `json:"deleted"`
}

// syncRunsView is one page of that history, newest first, with the cursor for
// the page after it. Next is absent when the history ends with this page.
type syncRunsView struct {
	Next string        `json:"next,omitempty"`
	Runs []syncRunView `json:"runs"`
}

type syncPhasesView struct {
	Source  *phaseRunView `json:"source,omitempty"`
	Targets *phaseRunView `json:"targets,omitempty"`
}

// activeView describes a run that has not finished: which half is in flight,
// when a held-back run is due to start, and how much of the library is already
// where it belongs. It is absent whenever nothing is under way.
//
// Aggregate counts only, and every one of them is read from the same local
// state the rest of this response is derived from. Watching a run therefore
// costs no provider call and names no route.
type activeView struct {
	// Phase is the half in flight, absent while a run has been accepted but no
	// half of it has started.
	Phase string `json:"phase,omitempty"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	StartsAt string `json:"starts_at,omitempty"`
	// Targets is how many accounts are configured, which is what the stage
	// counts beside it are measured against.
	Targets int              `json:"targets"`
	Stages  targetStagesView `json:"stages"`
}

type syncView struct {
	Phases syncPhasesView `json:"phases"`
	// Active is present only while a run is queued, running, or held back —
	// the states in which no terminal result may be claimed.
	Active *activeView `json:"active,omitempty"`
	State  string      `json:"state"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastCompletedAt string `json:"last_completed_at,omitempty"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	LastResult string      `json:"last_result,omitempty"`
	Surface    surfaceView `json:"surface"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	SourceStages int              `json:"source_stages"`
	Created      int              `json:"created"`
	Updated      int              `json:"updated"`
	Deleted      int              `json:"deleted"`
	Schedule     syncScheduleView `json:"schedule"`
}

// surfaceView reports how much of the library carries a usable classification.
// It is the difference between "this stage is waiting its turn" and "nothing has
// been classified in days", which is otherwise invisible: enrichment cannot fail
// a run, so a stage that fails every pass looks exactly like one nobody asked
// about.
//
// Generation and BuiltAt name the map build the classifications were read from.
// Both stay empty while surface classification is switched off, and until a
// first index has finished building.
type surfaceView struct {
	Generation string `json:"generation,omitempty"`
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	BuiltAt    string `json:"built_at,omitempty"`
	Classified int    `json:"classified"`
	Total      int    `json:"total"`
}

type statusView struct {
	// Build names the source this service was built from. Absent for a build
	// that was not made by CI, which is how a reader tells a development
	// process from a deployed one.
	Build   *buildView   `json:"build,omitempty"`
	Targets []targetView `json:"targets"`
	Sync    syncView     `json:"sync"`
	Ready   bool         `json:"ready"`
	// Converged is true only when every stored stage is current on every
	// configured target. It says the Wahoo accounts hold what the library holds,
	// and nothing about whether a device has since fetched it.
	Converged bool `json:"converged"`
}

// buildView says which public revision produced the running service, and which
// image carries it. Both are immutable facts about the build, so this group is
// safe to serve: it names no host, no path, no configuration, and no route.
type buildView struct {
	// Revision is a full commit object name in the public repository, so a
	// reader can address exactly the source they are running.
	Revision string `json:"revision"`
	// ImageDigest is the digest alone, without the registry or repository the
	// host pulls it from. Absent when the service was not told one.
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	ImageDigest string `json:"image_digest,omitempty"`
}

type webUIConfigView struct {
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	TileStyleURL string `json:"tile_style_url"`
	// Omitted when unconfigured, which is how the page knows to keep the one
	// style in both colour schemes.
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	TileStyleURLDark string `json:"tile_style_url_dark,omitempty"`
	// The provider's own web application, from which the page builds an outbound
	// link to the source route a stage was made from. Omitted when unconfigured,
	// which is how the page knows to offer no link rather than a broken one.
	//
	// A base URL only: the route identifier the link needs is already in the
	// stage the page is showing, and no route name, URL, or geometry is added
	// here.
	//nolint:tagliatelle // This v1 JSON contract uses snake_case.
	SourceBaseURL string `json:"source_base_url,omitempty"`
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
