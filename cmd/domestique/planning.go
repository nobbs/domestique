package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/nobbs/domestique/internal/brouter"
	"github.com/nobbs/domestique/internal/config"
	"github.com/nobbs/domestique/internal/fit"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/photon"
	"github.com/nobbs/domestique/internal/route"

	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/surface"
	syncservice "github.com/nobbs/domestique/internal/sync"
)

// newLocalSource builds the plan service, which is also the sync source for
// [planning], when that section is configured. configured is false, with a
// nil service and error, when it is absent.
func newLocalSource(
	settings *config.Settings, store *sqlite.Store, pace plan.Pace,
) (service *plan.Service, configured bool, err error) {
	if !settings.Planning.Enabled() {
		return nil, false, nil
	}
	client, err := brouter.New(&brouter.Options{BaseURL: settings.Planning.BRouterURL})
	if err != nil {
		return nil, false, fmt.Errorf("creating BRouter client: %w", err)
	}

	return plan.NewService(
		planStore{store: store}, brouterRouter{client: client}, pace, time.Now, plan.RandomID,
	), true, nil
}

// modelPace adapts the ride model to plan.Pace: the same forward model a
// stage's moving time is predicted with, over whatever pair is in force now.
type modelPace struct{ model *rideModelProvider }

func (p modelPace) Predict(points []route.Point) (movingSeconds float64, cumulative []float64, ok bool) {
	result, ok := ridemodel.Predict(points, p.model.pair())
	if !ok {
		return 0, nil, false
	}

	return result.MovingSeconds, result.CumulativeSeconds, true
}

// newPlaceNamer builds the geocoder the planner names waypoints with, when
// planning.photon_url is set. A nil result is the shape a build without one
// takes: waypoints read as coordinates.
func newPlaceNamer(settings *config.Settings) (httpapi.Places, error) {
	if !settings.Planning.Enabled() || settings.Planning.PhotonURL == "" {
		return nil, nil //nolint:nilnil // an absent geocoder is a configuration, not a failure
	}
	client, err := photon.New(&photon.Options{BaseURL: settings.Planning.PhotonURL})
	if err != nil {
		return nil, fmt.Errorf("creating Photon client: %w", err)
	}

	return client, nil
}

// surfaceSnapper moves a planned waypoint onto the nearest way the surface map
// holds: the same index a plan's ground is classified from.
type surfaceSnapper struct{ source surface.Source }

var _ httpapi.Snapper = surfaceSnapper{}

func (s surfaceSnapper) Snap(
	ctx context.Context, latitude, longitude float64,
) (snapLatitude, snapLongitude float64, moved bool, err error) {
	at := measure.Coordinate{Longitude: longitude, Latitude: latitude}
	snapped, moved, err := surface.Snap(ctx, s.source, at, surface.WaypointRadiusMetres)
	if err != nil {
		return latitude, longitude, false, fmt.Errorf("snapping a waypoint: %w", err)
	}

	return snapped.Latitude, snapped.Longitude, moved, nil
}

type surfaceClassifier struct{ source surface.Source }

var _ httpapi.SurfaceClassifier = surfaceClassifier{}

func newSurfaceClassifier(source surface.Source) httpapi.SurfaceClassifier {
	return surfaceClassifier{source: source}
}

func (c surfaceClassifier) Classify(
	ctx context.Context, points []route.Point,
) (*httpapi.SurfaceClassification, error) {
	ranges, matchedMetres, err := surface.ClassifyGeometry(ctx, c.source, points)
	if err != nil {
		return nil, fmt.Errorf("classifying geometry: %w", err)
	}
	if ranges == nil {
		return nil, nil //nolint:nilnil // no map generation is an optional classification result
	}
	classification := &httpapi.SurfaceClassification{
		Ranges:        make([]httpapi.SurfaceRange, len(ranges)),
		MatchedMetres: matchedMetres,
	}
	for index, band := range ranges {
		classification.Ranges[index] = httpapi.SurfaceRange{
			Kind:       band.Kind.String(),
			StartIndex: band.StartIndex,
			EndIndex:   band.EndIndex,
		}
	}

	return classification, nil
}

// brouterRouter adapts *brouter.Client to plan.Router: the brouter package
// knows nothing of plan.Waypoint or plan.Profile, so this is the one place
// that converts between them.
type brouterRouter struct{ client *brouter.Client }

func (r brouterRouter) Route(ctx context.Context, waypoints []plan.Waypoint, profile plan.Profile) (plan.Routed, error) {
	converted := make([]brouter.Waypoint, len(waypoints))
	for index, waypoint := range waypoints {
		converted[index] = brouter.Waypoint{Longitude: waypoint.Longitude, Latitude: waypoint.Latitude}
	}

	answer, err := r.client.Route(ctx, converted, string(profile))
	if err != nil {
		return plan.Routed{}, fmt.Errorf("routing waypoints: %w", err)
	}
	ways := make([]plan.RoutedWay, len(answer.Ways))
	for index, way := range answer.Ways {
		ways[index] = plan.RoutedWay{EndMetres: way.EndMetres, Tags: way.Tags}
	}

	turns := make([]plan.RoutedTurn, len(answer.Turns))
	for index, turn := range answer.Turns {
		turns[index] = plan.RoutedTurn{Turn: turn.Turn, Index: turn.Index, Exit: turn.Exit}
	}

	return plan.Routed{Points: answer.Points, Ways: ways, Turns: turns}, nil
}

// wireLocalSource registers the plan service on cache when [planning] is
// configured, leaving cache untouched otherwise, and returns it so main can
// wire it into the HTTP surface's plan endpoints. configured is false, with
// a nil service, when the section is absent.
func wireLocalSource(
	settings *config.Settings, store *sqlite.Store, cache *sourceCache, pace plan.Pace,
) (service *plan.Service, configured bool, err error) {
	service, configured, err = newLocalSource(settings, store, pace)
	if err != nil {
		return nil, false, err
	}
	if !configured {
		return nil, false, nil
	}
	cache.setLocal(service)
	// No behaviour change: the segment refresh task is #758/#761's work. This
	// build routes with whatever the engine's own directory already holds.
	if len(settings.Planning.Segments) > 0 {
		slog.Warn("planning.segments named but this build registers no segment refresh; " +
			"the engine routes with what its directory holds")
	}

	return service, true, nil
}

// httpapiPlans adapts a possibly-nil *plan.Service to httpapi.Plans, so the
// composition root's Options literal never assigns a typed nil pointer to
// that interface field — which Go would treat as non-nil.
// httpapiSnapper offers snapping only where there is a planner to snap for; the
// map may still be unbuilt, which the snapper answers by moving nothing.
func httpapiSnapper(service *plan.Service, source surface.Source) httpapi.Snapper {
	if service == nil {
		return nil
	}

	return surfaceSnapper{source: source}
}

func httpapiPlans(service *plan.Service) httpapi.Plans {
	if service == nil {
		return nil
	}

	return service
}

// planGetter is the one question the course encoder asks of the planner.
type planGetter interface {
	Get(ctx context.Context, id int64) (plan.Plan, bool, error)
}

// cueEncoder encodes courses, adding a plan's turn instructions when the plan
// asks for them and the stage is the revision they were measured against.
type cueEncoder struct {
	fit   *fit.Encoder
	plans planGetter
}

// courseEncoder is the plain encoder without a planner, and one reading each
// plan's cues with it.
func courseEncoder(service *plan.Service) cueEncoder {
	if service == nil {
		return cueEncoder{fit: fit.New()}
	}

	return cueEncoder{fit: fit.New(), plans: service}
}

//nolint:gocritic // This method conforms to the sync package's value contract.
func (e cueEncoder) Encode(ctx context.Context, stage route.Route) ([]byte, error) {
	if e.plans == nil || stage.Key().Provider() != route.ProviderLocal {
		return e.fit.Encode(ctx, stage) //nolint:wrapcheck // the encoder already names what failed
	}
	stored, found, err := e.plans.Get(ctx, stage.Key().SourceRouteID())
	if err != nil {
		return nil, fmt.Errorf("reading the plan's cues: %w", err)
	}
	if !found || !stored.Cues || stored.Revision() != stage.Revision() {
		return e.fit.Encode(ctx, stage) //nolint:wrapcheck // the encoder already names what failed
	}

	return e.fit.EncodeWithCues(ctx, stage, stored.Turns) //nolint:wrapcheck // the encoder already names what failed
}

// planDeliveries adapts the sync service's report on one plan's copies to
// what the HTTP boundary reads.
type planDeliveries struct {
	reconciler interface {
		PlanDelivery(ctx context.Context, planID int64, revision string) ([]syncservice.Delivery, error)
	}
}

func (d planDeliveries) PlanDelivery(ctx context.Context, planID int64, revision string) ([]httpapi.PlanDelivery, error) {
	deliveries, err := d.reconciler.PlanDelivery(ctx, planID, revision)
	if err != nil {
		return nil, fmt.Errorf("reporting plan delivery: %w", err)
	}
	views := make([]httpapi.PlanDelivery, len(deliveries))
	for index, delivery := range deliveries {
		views[index] = httpapi.PlanDelivery{
			DeliveredAt: delivery.DeliveredAt,
			TargetID:    delivery.TargetID,
			State:       string(delivery.State),
			Failure:     string(delivery.Failure),
		}
	}

	return views, nil
}

// httpapiPlanDeliveries is nil without a planner, as httpapiPlans is.
func httpapiPlanDeliveries(service *plan.Service, reconciler *syncservice.Service) httpapi.PlanDeliveries {
	if service == nil {
		return nil
	}

	return planDeliveries{reconciler: reconciler}
}

// planStore adapts *sqlite.Store to plan.Store, converting between
// sqlite.PlanRecord and plan.Plan.
type planStore struct{ store *sqlite.Store }

func (s planStore) InsertPlan(ctx context.Context, p *plan.Plan) error {
	record := planRecordOf(p)
	if err := s.store.InsertPlan(ctx, &record); err != nil {
		return fmt.Errorf("storing plan: %w", err)
	}

	return nil
}

func (s planStore) GetPlan(ctx context.Context, id int64) (plan.Plan, bool, error) {
	record, found, err := s.store.GetPlan(ctx, id)
	if err != nil {
		return plan.Plan{}, false, fmt.Errorf("reading plan: %w", err)
	}
	if !found {
		return plan.Plan{}, false, nil
	}
	converted, err := planOf(&record)
	if err != nil {
		return plan.Plan{}, false, err
	}

	return converted, true, nil
}

func (s planStore) ListPlans(ctx context.Context) ([]plan.Plan, error) {
	records, err := s.store.ListPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing plans: %w", err)
	}

	return plansOf(records)
}

func (s planStore) ListPublishedPlans(ctx context.Context) ([]plan.Plan, error) {
	records, err := s.store.ListPublishedPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing published plans: %w", err)
	}

	return plansOf(records)
}

func (s planStore) ReplacePlan(ctx context.Context, p *plan.Plan, expectedVersion int64) (bool, error) {
	record := planRecordOf(p)
	ok, err := s.store.ReplacePlan(ctx, &record, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("replacing plan: %w", err)
	}

	return ok, nil
}

func (s planStore) DeletePlan(ctx context.Context, id, expectedVersion int64) (bool, error) {
	ok, err := s.store.DeletePlan(ctx, id, expectedVersion)
	if err != nil {
		return false, fmt.Errorf("deleting plan: %w", err)
	}

	return ok, nil
}

// planRecordOf converts a plan.Plan to the shape sqlite.Store persists,
// waypoints as [longitude, latitude] pairs.
func planRecordOf(p *plan.Plan) sqlite.PlanRecord {
	waypoints := make([][2]float64, len(p.Waypoints))
	for index, waypoint := range p.Waypoints {
		waypoints[index] = [2]float64{waypoint.Longitude, waypoint.Latitude}
	}

	pushing := make([][2]float64, len(p.Pushing))
	for index, window := range p.Pushing {
		pushing[index] = [2]float64{window.StartMetres, window.EndMetres}
	}

	return sqlite.PlanRecord{
		ID: p.ID, Name: p.Name, Profile: string(p.Profile), Waypoints: waypoints, Geometry: p.Geometry,
		Pushing: pushing, Turns: p.Turns, Cues: p.Cues,
		DistanceMetres: p.DistanceMetres, AscentMetres: p.AscentMetres, Published: p.Published,
		Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// planOf converts a stored record back to plan.Plan.
func planOf(record *sqlite.PlanRecord) (plan.Plan, error) {
	profile, err := plan.ParseProfile(record.Profile)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("stored plan %d: %w", record.ID, err)
	}
	waypoints := make([]plan.Waypoint, len(record.Waypoints))
	for index, coordinate := range record.Waypoints {
		waypoints[index] = plan.Waypoint{Longitude: coordinate[0], Latitude: coordinate[1]}
	}

	pushing := make([]plan.Window, len(record.Pushing))
	for index, pair := range record.Pushing {
		pushing[index] = plan.Window{StartMetres: pair[0], EndMetres: pair[1]}
	}

	return plan.Plan{
		ID: record.ID, Name: record.Name, Profile: profile, Waypoints: waypoints, Geometry: record.Geometry,
		Pushing: pushing, Turns: record.Turns, Cues: record.Cues,
		DistanceMetres: record.DistanceMetres, AscentMetres: record.AscentMetres, Published: record.Published,
		Version: record.Version, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func plansOf(records []sqlite.PlanRecord) ([]plan.Plan, error) {
	plans := make([]plan.Plan, len(records))
	for index := range records {
		converted, err := planOf(&records[index])
		if err != nil {
			return nil, err
		}
		plans[index] = converted
	}

	return plans, nil
}
