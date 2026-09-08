package sqlite

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
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
		for _, point := range points {
			coordinate := point.Coordinate()
			// What a match is measured against is the ground the route covers, so
			// that and nothing else decides when one is owed again. Altitude is
			// left out with the rest: no match has ever read it.
			binary.LittleEndian.PutUint64(position[:], math.Float64bits(coordinate.Longitude))
			_, _ = digest.Write(position[:])
			binary.LittleEndian.PutUint64(position[:], math.Float64bits(coordinate.Latitude))
			_, _ = digest.Write(position[:])
			line = append(line, coordinate)
		}
		candidates = append(candidates, activity.RouteCandidate{
			Key:      route.NewKey(route.Provider(row.Provider), row.RouteID, int(row.StageOrder)),
			Geometry: line,
		})
	}

	return candidates, hex.EncodeToString(digest.Sum(nil)), nil
}

// ActivitiesAwaitingRouteMatch lists the rides whose track could be attributed
// to a route and has not been against this library: those never matched, and
// those matched against a library that has since changed.
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
