package main

import (
	"context"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/brouter"
	"github.com/nobbs/domestique/internal/httpapi"
	"github.com/nobbs/domestique/internal/plan"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/sqlite"
	"github.com/nobbs/domestique/internal/surface"
)

const demoBRouterURL = "https://brouter.de"

type demoSurfaceClassifier struct{}

var _ httpapi.SurfaceClassifier = demoSurfaceClassifier{}

func (demoSurfaceClassifier) Classify(
	ctx context.Context, points []route.Point,
) (*httpapi.SurfaceClassification, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("classifying demo surface: %w", err)
	}
	if len(points) == 0 {
		return nil, fmt.Errorf("classifying demo surface: geometry is empty")
	}

	kinds := make([]surface.Kind, len(points))
	classes := []surface.Kind{surface.KindAsphalt, surface.KindGravel, surface.KindGround, surface.KindPaving}
	for index := range kinds {
		classIndex := index * len(classes) / len(kinds)
		if classIndex >= len(classes) {
			classIndex = len(classes) - 1
		}
		kinds[index] = classes[classIndex]
	}
	ranges := surface.Compress(kinds)
	classification := &httpapi.SurfaceClassification{
		Ranges:        make([]httpapi.SurfaceRange, len(ranges)),
		MatchedMetres: surface.MatchedMetres(points, kinds),
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

// newDemoPlanService builds the one planner used by the demo API and its
// reseed path. BRouter's client owns the 20-second timeout and context flow.
func newDemoPlanService(store *sqlite.Store) (*plan.Service, error) {
	client, err := brouter.New(&brouter.Options{BaseURL: demoBRouterURL})
	if err != nil {
		return nil, fmt.Errorf("creating demo BRouter client: %w", err)
	}

	return plan.NewService(planStore{store: store}, brouterRouter{client: client}, time.Now, plan.RandomID), nil
}

// brouterRouter adapts *brouter.Client to plan.Router: the brouter package
// knows nothing about plan.Waypoint or plan.Profile, so this is the one place
// that converts between them.
type brouterRouter struct{ client *brouter.Client }

var _ plan.Router = brouterRouter{}

func (r brouterRouter) Route(ctx context.Context, waypoints []plan.Waypoint, profile plan.Profile) ([]route.Point, error) {
	converted := make([]brouter.Waypoint, len(waypoints))
	for index, waypoint := range waypoints {
		converted[index] = brouter.Waypoint{Longitude: waypoint.Longitude, Latitude: waypoint.Latitude}
	}

	points, err := r.client.Route(ctx, converted, string(profile))
	if err != nil {
		return nil, fmt.Errorf("routing waypoints: %w", err)
	}

	return points, nil
}

// planStore adapts the SQLite plan records to the plan service used by the
// demo. The shipped composition root has the same adapter for its BRouter
// source; this one keeps the demo independent of that production binary.
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

func planRecordOf(p *plan.Plan) sqlite.PlanRecord {
	waypoints := make([][2]float64, len(p.Waypoints))
	for index, waypoint := range p.Waypoints {
		waypoints[index] = [2]float64{waypoint.Longitude, waypoint.Latitude}
	}

	return sqlite.PlanRecord{
		ID: p.ID, Name: p.Name, Profile: string(p.Profile), Waypoints: waypoints, Geometry: p.Geometry,
		DistanceMetres: p.DistanceMetres, AscentMetres: p.AscentMetres, Published: p.Published,
		Version: p.Version, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func planOf(record *sqlite.PlanRecord) (plan.Plan, error) {
	profile, err := plan.ParseProfile(record.Profile)
	if err != nil {
		return plan.Plan{}, fmt.Errorf("stored plan %d: %w", record.ID, err)
	}
	waypoints := make([]plan.Waypoint, len(record.Waypoints))
	for index, coordinate := range record.Waypoints {
		waypoints[index] = plan.Waypoint{Longitude: coordinate[0], Latitude: coordinate[1]}
	}

	return plan.Plan{
		ID: record.ID, Name: record.Name, Profile: profile, Waypoints: waypoints, Geometry: record.Geometry,
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
