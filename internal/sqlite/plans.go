package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// PlanRecord is one admin-composed plan as SQLite stores it: a draft or
// published local route, its routed geometry, and the version a replace or
// delete must present as If-Match.
type PlanRecord struct {
	CreatedAt time.Time
	UpdatedAt time.Time
	Name      string
	Profile   string
	Waypoints [][2]float64
	// Pushing is [start, end] pairs in metres where a rider walks.
	Pushing  [][2]float64
	Geometry []route.Point
	// Turns are the routing engine's turn instructions along Geometry.
	Turns []route.Cue
	// Straight are the indices of the waypoints reached by a straight leg.
	Straight []int
	// Avoid are circles as [longitude, latitude, radius in metres].
	Avoid          [][3]float64
	DistanceMetres float64
	AscentMetres   float64
	ID             int64
	Version        int64
	Published      bool
	Cues           bool
}

// InsertPlan stores a newly created plan.
func (s *Store) InsertPlan(ctx context.Context, record *PlanRecord) error {
	waypoints, err := encodeWaypoints(record.Waypoints)
	if err != nil {
		return err
	}
	coordinates, err := encodeCoordinates(record.Geometry)
	if err != nil {
		return err
	}
	pushing, err := encodePairs(record.Pushing)
	if err != nil {
		return err
	}
	turns, err := encodeTurns(record.Turns)
	if err != nil {
		return err
	}
	straight, avoid, err := encodeRoutingOptions(record)
	if err != nil {
		return err
	}
	if err := s.queries.InsertPlan(ctx, sqlcgen.InsertPlanParams{
		ID: record.ID, Name: record.Name, Profile: record.Profile, Waypoints: string(waypoints),
		Coordinates: coordinates, DistanceMetres: record.DistanceMetres, AscentMetres: record.AscentMetres,
		Pushing: string(pushing), Turns: string(turns), Cues: boolToInt(record.Cues),
		Straight: string(straight), Avoid: string(avoid),
		Published: boolToInt(record.Published), Version: record.Version,
		CreatedAtUnixNano: record.CreatedAt.UnixNano(), UpdatedAtUnixNano: record.UpdatedAt.UnixNano(),
	}); err != nil {
		return fmt.Errorf("storing plan: %w", err)
	}

	return nil
}

// GetPlan reads one plan by ID, found reporting whether it exists.
func (s *Store) GetPlan(ctx context.Context, id int64) (record PlanRecord, found bool, err error) {
	row, err := s.queries.GetPlan(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return PlanRecord{}, false, nil
	}
	if err != nil {
		return PlanRecord{}, false, fmt.Errorf("reading plan: %w", err)
	}
	record, err = planRecordFromRow(&row)
	if err != nil {
		return PlanRecord{}, false, err
	}

	return record, true, nil
}

// ListPlans returns every plan, draft and published, ordered by id.
func (s *Store) ListPlans(ctx context.Context) ([]PlanRecord, error) {
	rows, err := s.queries.ListPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing plans: %w", err)
	}

	return planRecordsFromRows(rows)
}

// ListPublishedPlans returns only published plans, ordered by id: the local
// source's inventory.
func (s *Store) ListPublishedPlans(ctx context.Context) ([]PlanRecord, error) {
	rows, err := s.queries.ListPublishedPlans(ctx)
	if err != nil {
		return nil, fmt.Errorf("listing published plans: %w", err)
	}

	return planRecordsFromRows(rows)
}

// ReplacePlan overwrites a plan's fields, reporting false without writing
// anything when expectedVersion no longer matches what is stored.
func (s *Store) ReplacePlan(ctx context.Context, record *PlanRecord, expectedVersion int64) (bool, error) {
	waypoints, err := encodeWaypoints(record.Waypoints)
	if err != nil {
		return false, err
	}
	coordinates, err := encodeCoordinates(record.Geometry)
	if err != nil {
		return false, err
	}
	pushing, err := encodePairs(record.Pushing)
	if err != nil {
		return false, err
	}
	turns, err := encodeTurns(record.Turns)
	if err != nil {
		return false, err
	}
	straight, avoid, err := encodeRoutingOptions(record)
	if err != nil {
		return false, err
	}
	updated, err := s.queries.UpdatePlan(ctx, sqlcgen.UpdatePlanParams{
		Name: record.Name, Profile: record.Profile, Waypoints: string(waypoints), Coordinates: coordinates,
		DistanceMetres: record.DistanceMetres, AscentMetres: record.AscentMetres, Pushing: string(pushing),
		Turns: string(turns), Cues: boolToInt(record.Cues), Straight: string(straight), Avoid: string(avoid),
		Published: boolToInt(record.Published), Version: record.Version,
		UpdatedAtUnixNano: record.UpdatedAt.UnixNano(),
		ID:                record.ID, Version_2: expectedVersion,
	})
	if err != nil {
		return false, fmt.Errorf("replacing plan: %w", err)
	}

	return updated > 0, nil
}

// DeletePlan removes a plan, reporting false without writing anything when
// expectedVersion no longer matches what is stored.
func (s *Store) DeletePlan(ctx context.Context, id, expectedVersion int64) (bool, error) {
	deleted, err := s.queries.DeletePlan(ctx, sqlcgen.DeletePlanParams{ID: id, Version: expectedVersion})
	if err != nil {
		return false, fmt.Errorf("deleting plan: %w", err)
	}

	return deleted > 0, nil
}

func planRecordsFromRows(rows []sqlcgen.Plan) ([]PlanRecord, error) {
	records := make([]PlanRecord, 0, len(rows))
	for index := range rows {
		record, err := planRecordFromRow(&rows[index])
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}

	return records, nil
}

func planRecordFromRow(row *sqlcgen.Plan) (PlanRecord, error) {
	waypoints, err := decodeWaypoints([]byte(row.Waypoints))
	if err != nil {
		return PlanRecord{}, err
	}
	geometry, err := decodeCoordinates(row.Coordinates)
	if err != nil {
		return PlanRecord{}, fmt.Errorf("decoding plan geometry: %w", err)
	}
	pushing, err := decodeWaypoints([]byte(row.Pushing))
	if err != nil {
		return PlanRecord{}, fmt.Errorf("decoding plan pushing: %w", err)
	}
	if len(pushing) == 0 {
		pushing = nil
	}
	turns, err := decodeTurns(row.Turns)
	if err != nil {
		return PlanRecord{}, err
	}
	var straight []int
	if err := json.Unmarshal([]byte(row.Straight), &straight); err != nil {
		return PlanRecord{}, fmt.Errorf("decoding plan straight legs: %w", err)
	}
	var avoid [][3]float64
	if err := json.Unmarshal([]byte(row.Avoid), &avoid); err != nil {
		return PlanRecord{}, fmt.Errorf("decoding plan avoided areas: %w", err)
	}
	if len(straight) == 0 {
		straight = nil
	}
	if len(avoid) == 0 {
		avoid = nil
	}

	return PlanRecord{
		ID: row.ID, Name: row.Name, Profile: row.Profile, Waypoints: waypoints, Geometry: geometry,
		Pushing: pushing, Turns: turns, Cues: row.Cues != 0, Straight: straight, Avoid: avoid,
		DistanceMetres: row.DistanceMetres, AscentMetres: row.AscentMetres,
		Published: row.Published != 0, Version: row.Version,
		CreatedAt: time.Unix(0, row.CreatedAtUnixNano).UTC(),
		UpdatedAt: time.Unix(0, row.UpdatedAtUnixNano).UTC(),
	}, nil
}

// encodeRoutingOptions renders a plan's straight legs and avoided areas as
// JSON, empty lists rather than null.
func encodeRoutingOptions(record *PlanRecord) (straight, avoid []byte, err error) {
	legs, areas := record.Straight, record.Avoid
	if legs == nil {
		legs = []int{}
	}
	if areas == nil {
		areas = [][3]float64{}
	}
	if straight, err = json.Marshal(legs); err != nil {
		return nil, nil, fmt.Errorf("encoding plan straight legs: %w", err)
	}
	if avoid, err = json.Marshal(areas); err != nil {
		return nil, nil, fmt.Errorf("encoding plan avoided areas: %w", err)
	}

	return straight, avoid, nil
}

// storedTurn is one turn as the turns column holds it.
type storedTurn struct {
	Turn   route.Turn `json:"turn"`
	Metres float64    `json:"metres"`
	Exit   int        `json:"exit,omitempty"`
}

// encodeTurns renders a plan's turns as JSON, an empty list rather than null.
func encodeTurns(turns []route.Cue) ([]byte, error) {
	stored := make([]storedTurn, len(turns))
	for index, turn := range turns {
		stored[index] = storedTurn{Turn: turn.Turn, Metres: turn.Metres, Exit: turn.Exit}
	}
	encoded, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encoding plan turns: %w", err)
	}

	return encoded, nil
}

func decodeTurns(raw string) ([]route.Cue, error) {
	var stored []storedTurn
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		return nil, fmt.Errorf("decoding plan turns: %w", err)
	}
	if len(stored) == 0 {
		return nil, nil
	}
	turns := make([]route.Cue, len(stored))
	for index, turn := range stored {
		turns[index] = route.Cue{Turn: turn.Turn, Metres: turn.Metres, Exit: turn.Exit}
	}

	return turns, nil
}

func boolToInt(value bool) int64 {
	if value {
		return 1
	}

	return 0
}

// encodeWaypoints renders plan waypoints as a JSON [longitude, latitude] array.
func encodeWaypoints(waypoints [][2]float64) ([]byte, error) {
	encoded, err := json.Marshal(waypoints)
	if err != nil {
		return nil, fmt.Errorf("encoding plan waypoints: %w", err)
	}

	return encoded, nil
}

// encodePairs renders [start, end] pairs as JSON, an empty list rather than
// null, since the column holds one for every plan.
func encodePairs(pairs [][2]float64) ([]byte, error) {
	if pairs == nil {
		pairs = [][2]float64{}
	}

	return encodeWaypoints(pairs)
}

// decodeWaypoints reads back what encodeWaypoints wrote.
func decodeWaypoints(encoded []byte) ([][2]float64, error) {
	var waypoints [][2]float64
	if err := json.Unmarshal(encoded, &waypoints); err != nil {
		return nil, fmt.Errorf("decoding plan waypoints: %w", err)
	}

	return waypoints, nil
}
