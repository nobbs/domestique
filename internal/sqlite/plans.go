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
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Name           string
	Profile        string
	Waypoints      [][2]float64
	Geometry       []route.Point
	DistanceMetres float64
	AscentMetres   float64
	ID             int64
	Version        int64
	Published      bool
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
	if err := s.queries.InsertPlan(ctx, sqlcgen.InsertPlanParams{
		ID: record.ID, Name: record.Name, Profile: record.Profile, Waypoints: string(waypoints),
		Coordinates: coordinates, DistanceMetres: record.DistanceMetres, AscentMetres: record.AscentMetres,
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
	updated, err := s.queries.UpdatePlan(ctx, sqlcgen.UpdatePlanParams{
		Name: record.Name, Profile: record.Profile, Waypoints: string(waypoints), Coordinates: coordinates,
		DistanceMetres: record.DistanceMetres, AscentMetres: record.AscentMetres,
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

	return PlanRecord{
		ID: row.ID, Name: row.Name, Profile: row.Profile, Waypoints: waypoints, Geometry: geometry,
		DistanceMetres: row.DistanceMetres, AscentMetres: row.AscentMetres,
		Published: row.Published != 0, Version: row.Version,
		CreatedAt: time.Unix(0, row.CreatedAtUnixNano).UTC(),
		UpdatedAt: time.Unix(0, row.UpdatedAtUnixNano).UTC(),
	}, nil
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

// decodeWaypoints reads back what encodeWaypoints wrote.
func decodeWaypoints(encoded []byte) ([][2]float64, error) {
	var waypoints [][2]float64
	if err := json.Unmarshal(encoded, &waypoints); err != nil {
		return nil, fmt.Errorf("decoding plan waypoints: %w", err)
	}

	return waypoints, nil
}
