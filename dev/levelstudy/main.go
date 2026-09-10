// Command levelstudy measures how far a drag area fitted to the rider brings
// the estimated-power model's level to what their own heart rate says, judged
// whole ride by whole ride on rides the fit never saw, and how the model then
// sits against a real meter on the trainer rides that had one.
//
// The rider's own trainer rides carry a measured power and a heart rate; their
// road rides carry a track and a heart rate. Heart rate is the only quantity
// both record honestly, so it bridges them, with the bridge's level read from
// the metered rides before each road ride so a winter's fitness is not read
// as a summer's.
//
// Development tooling, not part of the shipped binary and never run in quick
// or check: it needs the operator's own snapshot of real rides. Its report is
// aggregate numbers only — no ride identifier, date, position or altitude
// value — and is safe to paste into an issue.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/nobbs/domestique/internal/sqlite"
)

func main() {
	database := flag.String("database", "", "state database to read recorded rides from")
	minSamples := flag.Int("min-samples", 60, "unmetered rides with fewer track samples are skipped")
	block := flag.Duration("block", 5*time.Minute, "block length both sides of the bridge are averaged over")
	folds := flag.Int("folds", 5, "how many held-out folds the candidates are scored over")
	window := flag.Int("window", 30, "how many metered rides before a road ride its bridge level is the median of")
	checkYear := flag.Int("check-year", 0, "hold the fitted model against the meter on that year's metered rides (0: skip)")
	mass := flag.Float64("mass", 0, "total system mass in kg, used for a target with no rider profile")
	flag.Parse()

	if err := run(*database, *minSamples, *block, *folds, *window, *checkYear, *mass); err != nil {
		fmt.Fprintf(os.Stderr, "levelstudy: %v\n", err)
		os.Exit(1)
	}
}

func run(database string, minSamples int, block time.Duration, folds, window, checkYear int, massFlag float64) error {
	switch {
	case database == "":
		return errors.New("-database is required")
	case minSamples <= 0:
		return errors.New("-min-samples must be a positive number of samples")
	case block <= 0:
		return errors.New("-block must be a positive duration")
	case folds < 2:
		return errors.New("-folds must be at least two")
	case window < 1:
		return errors.New("-window must be at least one ride")
	}

	ctx := context.Background()
	databasePath, err := filepath.Abs(database)
	if err != nil {
		return fmt.Errorf("resolving the database path: %w", err)
	}
	// A placeholder key: this tool writes no encrypted column. Opening still
	// migrates the file, so point it at a copy of a snapshot, never at the
	// deployed state.
	var key [32]byte
	store, err := sqlite.Open(ctx, databasePath, key)
	if err != nil {
		return fmt.Errorf("opening state: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "levelstudy: closing state: %v\n", closeErr)
		}
	}()

	result, err := study(ctx, store, minSamples, block, folds, window, checkYear, massFlag)
	if err != nil {
		return err
	}
	fmt.Print(result.String())

	return nil
}
