package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/sqlite/internal/sqlcgen"
)

// LibraryRoutes returns every route a ride may be attributed to, along with a
// hash over the positions of the library as a whole. A match stored against a
// different hash was measured against ground that has since changed, and is
// worked out again; a route whose line moved is therefore re-matched without
// the rides having to notice which route it was. Renaming one, or a revision
// carrying the same line, leaves every stored match alone.
func (s *Store) LibraryRoutes(ctx context.Context) ([]activity.RouteCandidate, string, error) {
	rows, err := s.queries.ListLibraryStageGeometry(ctx)
	if err != nil {
		return nil, "", fmt.Errorf("reading the library geometry: %w", err)
	}

	digest := sha256.New()
	var position [8]byte
	candidates := make([]activity.RouteCandidate, 0, len(rows))
	for _, row := range rows {
		// The rows arrive in key order, so the digest is stable for a library
		// whatever order the rides are matched in. The stage's content hash is
		// deliberately not what is digested: it covers the title and the source
		// revision too, so a rename would owe every ride a fresh match.
		// hash.Hash.Write never returns an error — the interface carries one only
		// because it embeds io.Writer.
		_, _ = digest.Write(fmt.Appendf(nil, "%s\x00%d\x00%d\n",
			row.Provider, row.RouteID, row.StageOrder))
		points, decodeErr := decodeCoordinates(row.Coordinates)
		if decodeErr != nil {
			return nil, "", fmt.Errorf("reading the library geometry: %w", decodeErr)
		}
		line := make([]measure.Coordinate, 0, len(points))
		elevations := make([]float64, 0, len(points))
		everyPointHasHeight := true
		for _, point := range points {
			coordinate := point.Coordinate()
			// The ground the route covers and the height along it, because both
			// decide what this pass produces: the positions decide which route a
			// ride was on, and the heights decide where its climbs are. A route
			// whose profile is redrawn owes its rides a fresh pass just as one
			// whose line moved does.
			binary.LittleEndian.PutUint64(position[:], math.Float64bits(coordinate.Longitude))
			_, _ = digest.Write(position[:])
			binary.LittleEndian.PutUint64(position[:], math.Float64bits(coordinate.Latitude))
			_, _ = digest.Write(position[:])
			height, hasHeight := 0.0, point.Elevation != nil
			if hasHeight {
				height = *point.Elevation
			}
			binary.LittleEndian.PutUint64(position[:], math.Float64bits(height))
			_, _ = digest.Write(position[:])
			line = append(line, coordinate)
			elevations = append(elevations, height)
			everyPointHasHeight = everyPointHasHeight && hasHeight
		}
		candidate := activity.RouteCandidate{
			Key:      route.NewKey(route.Provider(row.Provider), row.RouteID, int(row.StageOrder)),
			Geometry: line,
		}
		// All of them or none: a profile with gaps in it would put climbs where
		// the gaps are rather than where the hills are.
		if everyPointHasHeight {
			candidate.Elevations = elevations
		}
		candidates = append(candidates, candidate)
	}

	return candidates, hex.EncodeToString(digest.Sum(nil)), nil
}

// ActivitiesAwaitingRouteMatch lists the rides owed a match against this
// library: those never matched, and those matched against a library that has
// since changed. A ride whose samples are not stored has no track to match.
func (s *Store) ActivitiesAwaitingRouteMatch(ctx context.Context, targetID, libraryHash string) ([]int64, error) {
	ids, err := s.queries.ListActivitiesAwaitingRouteMatch(ctx, sqlcgen.ListActivitiesAwaitingRouteMatchParams{
		TargetSlot: targetID, LibraryHash: libraryHash,
	})
	if err != nil {
		return nil, fmt.Errorf("listing activities awaiting a route match: %w", err)
	}

	return ids, nil
}

// StoreActivityRouteMatch records which route one ride was ridden on, or that
// it was ridden on none. A nil match is that second answer, and is stored just
// as deliberately: it is what stops the ride being matched again next run.
func (s *Store) StoreActivityRouteMatch(
	ctx context.Context, targetID string, id int64, match *activity.RouteMatch, libraryHash string, now time.Time,
) error {
	params := sqlcgen.UpsertActivityRouteMatchParams{
		TargetSlot: targetID, WorkoutID: id, LibraryHash: libraryHash, MatchedAtUnix: now.Unix(),
	}
	if match != nil {
		params.Provider = sql.NullString{String: string(match.Key.Provider()), Valid: true}
		params.RouteID = sql.NullInt64{Int64: match.Key.SourceRouteID(), Valid: true}
		params.StageOrder = sql.NullInt64{Int64: int64(match.Key.StageOrder()), Valid: true}
		params.RouteCoverage = sql.NullFloat64{Float64: match.RouteCoverage, Valid: true}
		params.RideCoverage = sql.NullFloat64{Float64: match.RideCoverage, Valid: true}
		// A direction that could not be told is absent rather than a word meaning
		// absence, which is what the column being nullable is for.
		if match.Direction != activity.DirectionUnknown {
			params.Direction = sql.NullString{String: match.Direction.String(), Valid: true}
		}
	}
	if err := s.queries.UpsertActivityRouteMatch(ctx, params); err != nil {
		return fmt.Errorf("storing an activity's route match: %w", err)
	}

	return nil
}

// StoreActivityClimbAttempts replaces one ride's attempts at its route's
// climbs. An empty set clears whatever was there, which is what a ride that has
// come to match a different route -- or none -- leaves behind.
func (s *Store) StoreActivityClimbAttempts(
	ctx context.Context, targetID string, id int64, attempts []activity.ClimbAttempt,
) error {
	// Whole, and so in one transaction: a delete followed by inserts that
	// stopped part way would leave the ride holding some of one derivation's
	// attempts and none of the rest, which is a set no derivation ever produced.
	transaction, beginErr := s.database.BeginTx(ctx, nil)
	if beginErr != nil {
		return fmt.Errorf("starting the climb attempts write: %w", beginErr)
	}
	defer rollback(transaction)
	queries := s.queries.WithTx(transaction)
	if err := queries.DeleteActivityClimbAttempts(ctx, sqlcgen.DeleteActivityClimbAttemptsParams{
		TargetSlot: targetID, WorkoutID: id,
	}); err != nil {
		return fmt.Errorf("clearing an activity's climb attempts: %w", err)
	}
	for index := range attempts {
		attempt := &attempts[index]
		if err := queries.InsertActivityClimbAttempt(ctx, sqlcgen.InsertActivityClimbAttemptParams{
			TargetSlot:          targetID,
			WorkoutID:           id,
			ClimbIndex:          int64(attempt.ClimbIndex),
			Seconds:             attempt.Seconds,
			HeartRateBpm:        nullFloat(attempt.HeartRateBPM, attempt.HasHeartRate),
			PowerWatts:          nullFloat(attempt.PowerWatts, attempt.HasPower),
			EstimatedPowerWatts: nullFloat(attempt.EstimatedPowerWatts, attempt.HasEstimatedPower),
		}); err != nil {
			return fmt.Errorf("storing an activity's climb attempt: %w", err)
		}
	}
	if err := transaction.Commit(); err != nil {
		return fmt.Errorf("committing the climb attempts write: %w", err)
	}

	return nil
}

// StageProfile is one stage's stored line and the height along it, for finding
// its climbs. found is false for a stage the library does not hold; the
// elevations are nil for one whose geometry carries no height at all, which is
// a route no climb can be found on rather than one with none.
func (s *Store) StageProfile(
	ctx context.Context, key route.Key,
) (line []measure.Coordinate, elevations []float64, found bool, err error) {
	row, err := s.queries.GetStageGeometry(ctx, sqlcgen.GetStageGeometryParams{
		Provider: string(key.Provider()), RouteID: key.SourceRouteID(), StageOrder: int64(key.StageOrder()),
	})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, false, nil
	}
	if err != nil {
		return nil, nil, false, fmt.Errorf("reading stage geometry: %w", err)
	}
	points, decodeErr := decodeCoordinates(row.Coordinates)
	if decodeErr != nil {
		return nil, nil, false, fmt.Errorf("reading stage geometry: %w", decodeErr)
	}
	line = make([]measure.Coordinate, 0, len(points))
	heights := make([]float64, 0, len(points))
	everyPointHasHeight := true
	for _, point := range points {
		line = append(line, point.Coordinate())
		if point.Elevation == nil {
			everyPointHasHeight = false

			continue
		}
		heights = append(heights, *point.Elevation)
	}
	if everyPointHasHeight {
		elevations = heights
	}

	return line, elevations, true, nil
}

// RouteClimbAttempts is every attempt one target's rides made at one route's
// climbs, newest ride first.
func (s *Store) RouteClimbAttempts(
	ctx context.Context, targetID string, key route.Key,
) ([]activity.StoredClimbAttempt, error) {
	rows, err := s.queries.ListRouteClimbAttempts(ctx, sqlcgen.ListRouteClimbAttemptsParams{
		TargetSlot: targetID,
		Provider:   sql.NullString{String: string(key.Provider()), Valid: true},
		RouteID:    sql.NullInt64{Int64: key.SourceRouteID(), Valid: true},
		StageOrder: sql.NullInt64{Int64: int64(key.StageOrder()), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("reading a route's climb attempts: %w", err)
	}
	attempts := make([]activity.StoredClimbAttempt, 0, len(rows))
	for index := range rows {
		row := &rows[index]
		attempt := activity.ClimbAttempt{
			ClimbIndex:          int(row.ClimbIndex),
			Seconds:             row.Seconds,
			HeartRateBPM:        row.HeartRateBpm.Float64,
			PowerWatts:          row.PowerWatts.Float64,
			EstimatedPowerWatts: row.EstimatedPowerWatts.Float64,
			HasHeartRate:        row.HeartRateBpm.Valid,
			HasPower:            row.PowerWatts.Valid,
			HasEstimatedPower:   row.EstimatedPowerWatts.Valid,
		}
		attempts = append(attempts, activity.StoredClimbAttempt{
			RiddenAt:     time.Unix(row.StartedAtUnix, 0).UTC(),
			ClimbAttempt: attempt,
			WorkoutID:    row.WorkoutID,
		})
	}

	return attempts, nil
}

// ClearActivityRouteMatches removes every match one target holds and reports
// how many went. A library holding no route leaves nothing for a match to name.
func (s *Store) ClearActivityRouteMatches(ctx context.Context, targetID string) (int, error) {
	if _, err := s.queries.ClearActivityClimbAttempts(ctx, targetID); err != nil {
		return 0, fmt.Errorf("clearing activity climb attempts: %w", err)
	}
	removed, err := s.queries.DeleteActivityRouteMatchesForTarget(ctx, targetID)
	if err != nil {
		return 0, fmt.Errorf("clearing activity route matches: %w", err)
	}

	return int(removed), nil
}

// ActivityRouteMatches returns the route each of one target's rides was ridden
// on, by ride. A ride that matched nothing is absent rather than present and
// empty, so a caller reads the map the same way it reads the metrics one.
//
// A row naming a route carries its coverage, which the table enforces rather
// than each writer remembering, so the figures are read as given.
func (s *Store) ActivityRouteMatches(ctx context.Context, targetID string) (map[int64]activity.RouteMatch, error) {
	rows, err := s.queries.ListActivityRouteMatches(ctx, targetID)
	if err != nil {
		return nil, fmt.Errorf("reading activity route matches: %w", err)
	}

	matches := make(map[int64]activity.RouteMatch, len(rows))
	for _, row := range rows {
		matches[row.WorkoutID] = activity.RouteMatch{
			Key:           route.NewKey(route.Provider(row.Provider.String), row.RouteID.Int64, int(row.StageOrder.Int64)),
			RouteCoverage: row.RouteCoverage.Float64,
			RideCoverage:  row.RideCoverage.Float64,
			Direction:     activity.ParseDirection(row.Direction.String),
		}
	}

	return matches, nil
}

// RouteActivities returns the rides one target rode on one route, newest first.
func (s *Store) RouteActivities(
	ctx context.Context, targetID string, key route.Key,
) ([]activity.RouteRide, error) {
	rows, err := s.queries.ListRouteActivities(ctx, sqlcgen.ListRouteActivitiesParams{
		TargetSlot: targetID,
		Provider:   sql.NullString{String: string(key.Provider()), Valid: true},
		RouteID:    sql.NullInt64{Int64: key.SourceRouteID(), Valid: true},
		StageOrder: sql.NullInt64{Int64: int64(key.StageOrder()), Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("reading a route's activities: %w", err)
	}

	rides := make([]activity.RouteRide, 0, len(rows))
	for _, row := range rows {
		rides = append(rides, activity.RouteRide{
			ID:            row.WorkoutID,
			RouteCoverage: row.RouteCoverage.Float64,
			RideCoverage:  row.RideCoverage.Float64,
			Direction:     activity.ParseDirection(row.Direction.String),
		})
	}

	return rides, nil
}
