// Command ascentstudy answers #608's second bullet: for every recorded ride
// in a state database, how each candidate ascent definition computed over
// the ride's own samples compares with the ascent the device reported.
//
// Development tooling, not part of the shipped binary and never run in quick
// or check: it needs the operator's own snapshot of real rides. Its report is
// aggregate numbers only — no ride identifier, date, position, slot or
// altitude value — and is safe to paste into an issue.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nobbs/domestique/internal/sqlite"
)

func main() {
	database := flag.String("database", "", "state database to read recorded rides from")
	thresholds := flag.String("thresholds", "1.2,2,3", "comma-separated hysteresis thresholds in metres")
	grids := flag.String("grids", "10,20,50", "comma-separated distance-grid resample intervals in metres")
	stillSpeed := flag.Float64("still-speed", 1.0, "minimum metres per second a sample must move at to count as moving")
	minSamples := flag.Int("min-samples", 60, "rides with fewer positioned track samples are skipped and counted")
	splits := flag.Bool("splits", true, "print the weather and moving-speed split tables")
	flag.Parse()

	if err := run(*database, *thresholds, *grids, *stillSpeed, *minSamples, *splits); err != nil {
		fmt.Fprintf(os.Stderr, "ascentstudy: %v\n", err)
		os.Exit(1)
	}
}

func run(database, thresholdList, gridList string, stillSpeedMS float64, minSamples int, splitsEnabled bool) error {
	if minSamples <= 0 {
		return errors.New("-min-samples must be a positive number of samples")
	}
	if database == "" {
		return errors.New("-database is required")
	}
	thresholds, err := parseThresholds(thresholdList)
	if err != nil {
		return err
	}
	grids, err := parseGrids(gridList)
	if err != nil {
		return err
	}
	if speedErr := validateStillSpeed(stillSpeedMS); speedErr != nil {
		return speedErr
	}

	ctx := context.Background()
	// A placeholder key: this tool only reads, and the development snapshot's
	// stored credentials are already undecryptable.
	// sqlite.Open takes an absolute path; the task's example passes a relative one.
	databasePath, err := filepath.Abs(database)
	if err != nil {
		return fmt.Errorf("resolving the database path: %w", err)
	}
	var key [32]byte
	store, err := sqlite.Open(ctx, databasePath, key)
	if err != nil {
		return fmt.Errorf("opening state: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "ascentstudy: closing state: %v\n", closeErr)
		}
	}()

	report, err := study(ctx, store, thresholds, grids, stillSpeedMS, minSamples, splitsEnabled)
	if err != nil {
		return err
	}
	fmt.Print(report.String())

	return nil
}
