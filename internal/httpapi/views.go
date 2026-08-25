package httpapi

import (
	"encoding/json"

	"github.com/nobbs/domestique/internal/route"
)

// The v1 JSON contract uses camelCase throughout. These view types are
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
	LastRun       *targetRunView `json:"lastRun,omitempty"`
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
	CompletedAt string `json:"completedAt"`
	Result      string `json:"result"`
	// Failure is the safe failure category, present only when that run did not
	// succeed. It is never provider text.
	Failure string `json:"failure,omitempty"`
}

type stageView struct {
	// MovingSeconds is the predicted moving time from internal/ridemodel.
	// Absent — never zero — when no coefficient file is configured, the stage
	// has no usable elevation, or nothing has predicted this exact geometry
	// yet: a rider figure the service never asked for must not manufacture a
	// confident number.
	//
	MovingSeconds *float64 `json:"movingSeconds,omitempty"`
	// Validation is the frozen profile's measured unseen-route error —
	// present only alongside MovingSeconds, and only when the loaded
	// coefficient file itself carries a measured benchmark result. Every
	// stage on a deployment shares the same value: it describes the profile,
	// not this stage's own geometry.
	Validation         *stageValidation `json:"validation,omitempty"`
	Title              string           `json:"title"`
	StageName          string           `json:"stageName"`
	Provider           string           `json:"provider"`
	SourceRevision     string           `json:"sourceRevision"`
	ContentHash        string           `json:"contentHash"`
	RouteName          string           `json:"routeName"`
	DistanceMetres     float64          `json:"distanceMetres"`
	AscentMetres       float64          `json:"ascentMetres"`
	MaxGradientPercent float64          `json:"maxGradientPercent"`
	RouteID            int64            `json:"routeId"`
	PointCount         int              `json:"pointCount"`
	StageOrder         int              `json:"stageOrder"`
}

// stageValidation is the frozen coefficient profile's measured unseen-route
// error, from the same route-disjoint benchmark that froze it. It describes
// the profile as a whole, not any one stage's geometry.
type stageValidation struct {
	BiasPercent    float64 `json:"biasPercent"`
	MAEPercent     float64 `json:"maePercent"`
	P90Percent     float64 `json:"p90Percent"`
	EvaluatedRides int     `json:"evaluatedRides"`
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
	LastCompletedAt string `json:"lastCompletedAt"`
	LastResult      string `json:"lastResult"`
	LastFailure     string `json:"lastFailure,omitempty"`
	SourceStages    int    `json:"sourceStages"`
	Created         int    `json:"created"`
	Updated         int    `json:"updated"`
	Deleted         int    `json:"deleted"`
}

// syncRunView is one terminal run in the recorded history: which half it was,
// when it ended, how it ended, and what it moved. It names no route, carries no
// geometry, and quotes nothing a provider said — the failure is the same safe
// category the status response reports.
//
// Reference is the opaque name the run was recorded under. It is what a
// notification can say out loud, and what an operator matches this row against.
type syncRunView struct {
	Reference    string `json:"reference"`
	Phase        string `json:"phase"`
	CompletedAt  string `json:"completedAt"`
	Result       string `json:"result"`
	Failure      string `json:"failure,omitempty"`
	SourceStages int    `json:"sourceStages"`
	Created      int    `json:"created"`
	Updated      int    `json:"updated"`
	Deleted      int    `json:"deleted"`
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
	Phase    string `json:"phase,omitempty"`
	StartsAt string `json:"startsAt,omitempty"`
	// Targets is how many accounts are configured, which is what the stage
	// counts beside it are measured against.
	Targets int              `json:"targets"`
	Stages  targetStagesView `json:"stages"`
}

// wahooRateLimitView reports Wahoo's own most recently advertised request
// quota. It is a live reading of that response, not a count this service
// keeps itself — Wahoo's quota is shared across every configured target, so
// nothing local could total it correctly on its own.
type wahooRateLimitView struct {
	ResetsAt  string `json:"resetsAt,omitempty"`
	Remaining int    `json:"remaining"`
}

type syncView struct {
	Phases syncPhasesView `json:"phases"`
	// Active is present only while a run is queued, running, or held back —
	// the states in which no terminal result may be claimed.
	Active *activeView `json:"active,omitempty"`
	// WahooRateLimit is absent until a request has actually reached Wahoo and
	// carried a quota header back.
	WahooRateLimit *wahooRateLimitView `json:"wahooRateLimit,omitempty"`
	// TrustedInventory reports the age of the trusted source inventory against
	// the configured staleness bound. Absent when the deployment named no
	// bound.
	TrustedInventory *trustedInventoryView `json:"trustedInventory,omitempty"`
	State            string                `json:"state"`
	LastCompletedAt  string                `json:"lastCompletedAt,omitempty"`
	LastResult       string                `json:"lastResult,omitempty"`
	Surface          surfaceView           `json:"surface"`
	SourceStages     int                   `json:"sourceStages"`
	Created          int                   `json:"created"`
	Updated          int                   `json:"updated"`
	Deleted          int                   `json:"deleted"`
	Schedule         syncScheduleView      `json:"schedule"`
}

// trustedInventoryView reports the trusted source inventory's freshness. It is
// derived from local state alone — the last successful source phase completion
// against the configured bound — so reading it costs no provider call.
type trustedInventoryView struct {
	// LastSuccessAt is absent until a source phase has ever succeeded, which is
	// what distinguishes that case from a true age of zero: AgeSeconds is
	// always present and reads 0 in both, and cannot carry the distinction on
	// its own without the ",omitempty" on a plain int also dropping a
	// perfectly valid zero age read immediately after a successful refresh.
	LastSuccessAt string `json:"lastSuccessAt,omitempty"`
	AgeSeconds    int64  `json:"ageSeconds"`
	MaxAgeSeconds int64  `json:"maxAgeSeconds"`
	Fresh         bool   `json:"fresh"`
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
	BuiltAt    string `json:"builtAt,omitempty"`
	Classified int    `json:"classified"`
	Total      int    `json:"total"`
	// Incomplete is how many stages the most recently completed classification
	// pass could not classify. What is neither classified nor incomplete is
	// simply waiting its turn — the difference this field exists to draw: a
	// stage that keeps failing otherwise looks exactly like one nobody has
	// asked about yet.
	Incomplete int `json:"incomplete"`
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
	ImageDigest string `json:"imageDigest,omitempty"`
}

type webUIConfigView struct {
	// Each configured source's own web application, keyed by provider, from
	// which the page builds an outbound link to the source route a stage was
	// made from. A provider it cannot build a link for is omitted from the map
	// entirely, which is how the page knows to offer no link for that source
	// rather than a broken one. The whole field is omitted when no source names
	// one.
	//
	// A base URL only: the route identifier the link needs is already in the
	// stage the page is showing, and no route name, URL, or geometry is added
	// here.
	SourceBaseURLs map[route.Provider]string `json:"sourceBaseUrls,omitempty"`
	// The cartographies the page may switch between, in the configured order.
	// Never empty: the first entry is what a browser that has not chosen loads.
	Basemaps []basemapView `json:"basemaps"`
}

// basemapView is one cartography offered to the page. Every field is operator
// configuration the browser must know to render and label a map; none is a
// secret, for the reason webUIConfig gives.
type basemapView struct {
	// The label the picker shows, and the identity a browser remembers its
	// choice by.
	Name     string `json:"name"`
	StyleURL string `json:"styleUrl"`
	// Omitted when unconfigured, which is how the page knows to keep this
	// entry's one style in both colour schemes.
	StyleURLDark string `json:"styleUrlDark,omitempty"`
	// Omitted when false. True marks ground that is dark in either colour
	// scheme, so the page inks routes over it to match the map rather than the
	// system.
	DarkCartography bool `json:"darkCartography,omitempty"`
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
