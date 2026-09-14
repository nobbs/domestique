// Package plan owns the admin-composed local route source: waypoints and a
// routing profile, measured through the same path a preview and a save both
// use, and the published subset that becomes this service's own route
// inventory. It never persists anything itself; a Store implementation does.
package plan

import (
	"context"
	cryptorand "crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/elevation"
	"github.com/nobbs/domestique/internal/route"
)

// Profile is a routing engine profile a plan is drawn against.
type Profile string

// The routing profiles the engine accepts; no others are valid.
const (
	Trekking Profile = "trekking"
	Fastbike Profile = "fastbike"
	Gravel   Profile = "gravel"
)

// ParseProfile validates a stored or submitted profile string.
func ParseProfile(value string) (Profile, error) {
	switch profile := Profile(value); profile {
	case Trekking, Fastbike, Gravel:
		return profile, nil
	default:
		return "", fmt.Errorf("plan: unknown profile %q", value)
	}
}

const (
	minWaypoints  = 2
	maxWaypoints  = 50
	maxNameLength = 120
)

// ErrVersionMismatch reports a replace or delete whose expected version no
// longer matches what is stored.
var ErrVersionMismatch = errors.New("plan: version does not match")

// ErrNotFound reports an operation against a plan ID that does not exist.
var ErrNotFound = errors.New("plan: not found")

// Waypoint is one point an admin placed while drawing a plan.
type Waypoint struct {
	Longitude float64
	Latitude  float64
}

// Plan is an admin-composed route, from its waypoints to its routed geometry,
// exactly as a Store persists it.
type Plan struct {
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Name           string
	Profile        Profile
	Waypoints      []Waypoint
	Geometry       []route.Point
	DistanceMetres float64
	AscentMetres   float64
	ID             int64
	Version        int64
	Published      bool
}

// Router routes an ordered set of waypoints over a bike-preferring road
// network for one profile, returning the snapped line with an elevation per
// point.
type Router interface {
	Route(ctx context.Context, waypoints []Waypoint, profile Profile) ([]route.Point, error)
}

// Store owns plan persistence. ReplacePlan and DeletePlan report false,
// without writing anything, when expectedVersion no longer matches what is
// stored.
type Store interface {
	InsertPlan(ctx context.Context, plan *Plan) error
	GetPlan(ctx context.Context, id int64) (plan Plan, found bool, err error)
	ListPlans(ctx context.Context) ([]Plan, error)
	ListPublishedPlans(ctx context.Context) ([]Plan, error)
	ReplacePlan(ctx context.Context, plan *Plan, expectedVersion int64) (bool, error)
	DeletePlan(ctx context.Context, id, expectedVersion int64) (bool, error)
}

// Measured is a plan's routed and measured geometry, produced by the one path
// both a preview and a save use.
type Measured struct {
	Geometry       []route.Point
	DistanceMetres float64
	AscentMetres   float64
}

// Service composes a Router and a Store into plan validation, routing,
// measurement, and persistence.
type Service struct {
	store  Store
	router Router
	now    func() time.Time
	newID  func() (int64, error)
}

// NewService builds a Service. now and newID are clock and ID seams: a
// caller running the service for real passes time.Now and RandomID.
func NewService(store Store, router Router, now func() time.Time, newID func() (int64, error)) *Service {
	return &Service{store: store, router: router, now: now, newID: newID}
}

// RandomID draws a plan identifier at random from the positive int63 range,
// never in sequence, so a database rebuilt after state loss cannot reissue a
// deleted plan's identity.
func RandomID() (int64, error) {
	drawn, err := cryptorand.Int(cryptorand.Reader, big.NewInt(math.MaxInt64))
	if err != nil {
		return 0, fmt.Errorf("plan: drawing a plan id: %w", err)
	}

	return drawn.Int64() + 1, nil
}

// previewRevision is a non-empty placeholder for route.NewRoute: Route builds
// no stored identity, so any fixed value satisfies it.
const previewRevision = "preview"

// Route is the one measuring path: it asks the router for the snapped line,
// then normalizes and measures it exactly as a stored stage is.
func (s *Service) Route(ctx context.Context, waypoints []Waypoint, profile Profile) (Measured, error) {
	points, err := s.router.Route(ctx, waypoints, profile)
	if err != nil {
		return Measured{}, fmt.Errorf("plan: routing waypoints: %w", err)
	}
	built, err := route.NewRoute(route.ProviderLocal, 1, 1, previewRevision, "", "", points, previewRevision)
	if err != nil {
		return Measured{}, fmt.Errorf("plan: building routed geometry: %w", err)
	}
	normalized, err := elevation.New().Process(&built)
	if err != nil {
		return Measured{}, fmt.Errorf("plan: normalizing elevation: %w", err)
	}

	return Measured{
		Geometry:       normalized.Geometry(),
		DistanceMetres: normalized.DistanceMetres(),
		AscentMetres:   normalized.ElevationGainMetres(),
	}, nil
}

// Create validates, routes, and stores a new draft plan.
func (s *Service) Create(ctx context.Context, name string, profile Profile, waypoints []Waypoint) (Plan, error) {
	trimmedName, err := validate(name, profile, waypoints)
	if err != nil {
		return Plan{}, err
	}
	measured, err := s.Route(ctx, waypoints, profile)
	if err != nil {
		return Plan{}, err
	}
	id, err := s.newID()
	if err != nil {
		return Plan{}, fmt.Errorf("plan: generating a plan id: %w", err)
	}
	now := s.now().UTC()
	created := Plan{
		ID: id, Name: trimmedName, Profile: profile, Waypoints: waypoints,
		Geometry: measured.Geometry, DistanceMetres: measured.DistanceMetres, AscentMetres: measured.AscentMetres,
		Published: false, Version: 1, CreatedAt: now, UpdatedAt: now,
	}
	if err := s.store.InsertPlan(ctx, &created); err != nil {
		return Plan{}, fmt.Errorf("plan: storing plan: %w", err)
	}

	return created, nil
}

// Replace validates, routes, and overwrites an existing plan's waypoints,
// profile, name, and published state. expectedVersion must match the
// currently stored version, or nothing is stored and ErrVersionMismatch is
// returned; a missing plan returns ErrNotFound.
func (s *Service) Replace(
	ctx context.Context, id, expectedVersion int64, name string, profile Profile, waypoints []Waypoint, published bool,
) (Plan, error) {
	trimmedName, err := validate(name, profile, waypoints)
	if err != nil {
		return Plan{}, err
	}
	existing, found, err := s.store.GetPlan(ctx, id)
	if err != nil {
		return Plan{}, fmt.Errorf("plan: reading plan: %w", err)
	}
	if !found {
		return Plan{}, ErrNotFound
	}
	if existing.Version != expectedVersion {
		return Plan{}, ErrVersionMismatch
	}
	measured, err := s.Route(ctx, waypoints, profile)
	if err != nil {
		return Plan{}, err
	}
	replaced := Plan{
		ID: id, Name: trimmedName, Profile: profile, Waypoints: waypoints,
		Geometry: measured.Geometry, DistanceMetres: measured.DistanceMetres, AscentMetres: measured.AscentMetres,
		Published: published, Version: expectedVersion + 1, CreatedAt: existing.CreatedAt, UpdatedAt: s.now().UTC(),
	}
	ok, err := s.store.ReplacePlan(ctx, &replaced, expectedVersion)
	if err != nil {
		return Plan{}, fmt.Errorf("plan: replacing plan: %w", err)
	}
	if !ok {
		return Plan{}, ErrVersionMismatch
	}

	return replaced, nil
}

// Delete removes a plan whose expectedVersion still matches what is stored;
// otherwise it returns ErrVersionMismatch, or ErrNotFound for an unknown ID,
// and stores nothing.
func (s *Service) Delete(ctx context.Context, id, expectedVersion int64) error {
	existing, found, err := s.store.GetPlan(ctx, id)
	if err != nil {
		return fmt.Errorf("plan: reading plan: %w", err)
	}
	if !found {
		return ErrNotFound
	}
	if existing.Version != expectedVersion {
		return ErrVersionMismatch
	}
	ok, err := s.store.DeletePlan(ctx, id, expectedVersion)
	if err != nil {
		return fmt.Errorf("plan: deleting plan: %w", err)
	}
	if !ok {
		return ErrVersionMismatch
	}

	return nil
}

// Get returns one plan, found reporting whether it exists.
func (s *Service) Get(ctx context.Context, id int64) (result Plan, found bool, err error) {
	result, found, err = s.store.GetPlan(ctx, id)
	if err != nil {
		return Plan{}, false, fmt.Errorf("plan: reading plan: %w", err)
	}
	if !found {
		return Plan{}, false, nil
	}

	return result, true, nil
}

// List returns every plan, draft and published, ordered by id.
func (s *Service) List(ctx context.Context) ([]Plan, error) {
	plans, err := s.store.ListPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("plan: listing plans: %w", err)
	}

	return plans, nil
}

// Provider names the local source's provider, for a caller wiring plan into
// the multi-source inventory alongside veloplanner and komoot.
func (s *Service) Provider() route.Provider {
	return route.ProviderLocal
}

// Inventory returns one route.Route per published plan, built from the
// geometry each plan stored when it was saved: it never asks the router. A
// draft contributes nothing.
func (s *Service) Inventory(ctx context.Context) ([]route.Route, error) {
	plans, err := s.store.ListPublishedPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("plan: listing published plans: %w", err)
	}
	routes := make([]route.Route, 0, len(plans))
	for index := range plans {
		plan := &plans[index]
		revision := plan.UpdatedAt.UTC().Format(time.RFC3339Nano)
		built, err := route.NewRoute(
			route.ProviderLocal, plan.ID, 1, revision, plan.Name, "",
			plan.Geometry, contentHash(plan.ID, plan.Name, plan.Geometry),
		)
		if err != nil {
			return nil, fmt.Errorf("plan: building route for plan %d: %w", plan.ID, err)
		}
		routes = append(routes, built)
	}

	return routes, nil
}

func validate(name string, profile Profile, waypoints []Waypoint) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", errors.New("plan: name is required")
	}
	if len(trimmed) > maxNameLength {
		return "", fmt.Errorf("plan: name exceeds %d characters", maxNameLength)
	}
	if _, err := ParseProfile(string(profile)); err != nil {
		return "", err
	}
	if len(waypoints) < minWaypoints || len(waypoints) > maxWaypoints {
		return "", fmt.Errorf("plan: waypoints must number between %d and %d", minWaypoints, maxWaypoints)
	}
	for index, waypoint := range waypoints {
		if waypoint.Longitude < -180 || waypoint.Longitude > 180 {
			return "", fmt.Errorf("plan: waypoint %d longitude is out of range", index)
		}
		if waypoint.Latitude < -90 || waypoint.Latitude > 90 {
			return "", fmt.Errorf("plan: waypoint %d latitude is out of range", index)
		}
	}

	return trimmed, nil
}

// contentHash is a stable digest of everything a published plan's route is
// made of, in the style of internal/demo's, but over a plan's own identity.
func contentHash(id int64, name string, geometry []route.Point) string {
	payload := fmt.Appendf(nil, "local\x00%d\x00%s\x00", id, name)
	for index := range geometry {
		point := &geometry[index]
		height := math.NaN()
		if point.Elevation != nil {
			height = *point.Elevation
		}
		payload = fmt.Appendf(payload, "%.6f,%.6f,%.1f;", point.Longitude, point.Latitude, height)
	}
	digest := sha256.Sum256(payload)

	return hex.EncodeToString(digest[:])
}
