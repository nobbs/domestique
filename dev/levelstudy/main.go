// Command levelstudy answers #623: whether the estimated-power model's level
// should be fitted as one scale over both coefficients or as the two
// coefficients apart, judged on rides the fit never saw.
//
// The rider's own trainer rides carry a measured power and a heart rate; their
// road rides carry a track and a heart rate. Heart rate is the only quantity
// both record honestly, so it bridges them (see internal/powerfit).
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
	bridgeWindow := flag.Duration("bridge-window", 90*24*time.Hour,
		"how far either side of a ride its bridge may draw measured blocks from")
	folds := flag.Int("folds", 5, "how many held-out folds the candidates are scored over")
	mass := flag.Float64("mass", 0, "total system mass in kg, used for a target with no rider profile")
	flag.Parse()

	if err := run(*database, *minSamples, *block, *bridgeWindow, *folds, *mass); err != nil {
		fmt.Fprintf(os.Stderr, "levelstudy: %v\n", err)
		os.Exit(1)
	}
}

func run(database string, minSamples int, block, bridgeWindow time.Duration, folds int, massFlag float64) error {
	switch {
	case database == "":
		return errors.New("-database is required")
	case minSamples <= 0:
		return errors.New("-min-samples must be a positive number of samples")
	case block <= 0:
		return errors.New("-block must be a positive duration")
	case bridgeWindow <= 0:
		return errors.New("-bridge-window must be a positive duration")
	case folds < 2:
		return errors.New("-folds must be at least two")
	case massFlag <= 0:
		return errors.New("-mass must be a positive number of kilograms")
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

	result, err := study(ctx, store, minSamples, block, bridgeWindow, folds, massFlag)
	if err != nil {
		return err
	}
	fmt.Print(result.String())

	return nil
}
