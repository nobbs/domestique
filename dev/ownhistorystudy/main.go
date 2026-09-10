package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/sqlite"
)

func main() {
	database := flag.String("database", "", "state database to read recorded rides from")
	halfLifeDays := flag.Float64("half-life-days", 90, "recency half-life, in days, for the own-history median's weights")
	flag.Parse()
	if err := run(*database, *halfLifeDays); err != nil {
		fmt.Fprintf(os.Stderr, "ownhistorystudy: %v\n", err)
		os.Exit(1)
	}
}

// storeAdapter narrows *sqlite.Store to the corpus interface, converting its
// own RecordedRide into this package's, so the study logic depends on
// neither the store nor its row type.
type storeAdapter struct{ store *sqlite.Store }

func (a storeAdapter) RecordedRides(ctx context.Context) ([]recordedRide, error) {
	rows, err := a.store.RecordedRides(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]recordedRide, len(rows))
	for i, row := range rows {
		out[i] = recordedRide{
			TargetID: row.TargetID, WorkoutID: row.WorkoutID,
			AscentMetres: row.AscentMetres, DistanceMetres: row.DistanceMetres, MovingSeconds: row.MovingSeconds,
		}
	}

	return out, nil
}

func (a storeAdapter) ActivityListings(
	ctx context.Context, targetID string,
) ([]activity.Listing, time.Time, error) {
	return a.store.ActivityListings(ctx, targetID)
}

func (a storeAdapter) ActivityRouteMatches(
	ctx context.Context, targetID string,
) (map[int64]activity.RouteMatch, error) {
	return a.store.ActivityRouteMatches(ctx, targetID)
}

func (a storeAdapter) ActivityRides(
	ctx context.Context, since time.Time, workoutTypeIDs []int,
) ([]ridemodel.Ride, error) {
	return a.store.ActivityRides(ctx, since, workoutTypeIDs)
}

func run(database string, halfLifeDays float64) error {
	if database == "" {
		return errors.New("-database is required")
	}
	if halfLifeDays <= 0 {
		return errors.New("-half-life-days must be positive")
	}

	ctx := context.Background()
	databasePath, err := filepath.Abs(database)
	if err != nil {
		return fmt.Errorf("resolving the database path: %w", err)
	}
	// A placeholder key: this tool decrypts nothing. Opening still migrates
	// the file and sets its journal mode, so point it at a copy of a
	// snapshot, never at the deployed state.
	var key [32]byte
	store, err := sqlite.Open(ctx, databasePath, key)
	if err != nil {
		return fmt.Errorf("opening state: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "ownhistorystudy: closing state: %v\n", closeErr)
		}
	}()

	result, err := study(ctx, storeAdapter{store}, halfLifeDays)
	if err != nil {
		return err
	}
	fmt.Print(result)

	return nil
}
