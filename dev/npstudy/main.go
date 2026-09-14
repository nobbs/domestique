// Command npstudy asks what a normalized power of the estimated power series
// would say, before deciding whether it is a figure worth serving: how far it
// sits above the plain means, how much of that excess survives smoothing, and,
// on trainer rides whose virtual track the model can run over, how it compares
// with the meter's own normalized power.
//
// Development tooling, not part of the shipped binary: it needs the operator's
// own snapshot of real rides, and optionally their Strava bulk export's
// activities.csv. Its report is aggregate numbers only — no ride identifier,
// date, position or altitude value — and is safe to paste into an issue.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	database := flag.String("database", "", "state database to read recorded rides from; a copy, since opening migrates it")
	target := flag.String("target", "", "the target slot to read rides from; required where the database holds several")
	mass := flag.Float64("mass", 0, "total system mass in kg, used for a target with no rider profile")
	minSamples := flag.Int("min-samples", 600, "rides with fewer track samples are skipped")
	zwiftCrr := flag.Float64("zwift-crr", 0.004, "rolling resistance the trainer rides' drag area is fitted at")
	strava := flag.String("strava", "", "optional Strava export activities.csv, joined to rides by start time")
	flag.Parse()

	if err := run(*database, *target, *mass, *minSamples, *zwiftCrr, *strava); err != nil {
		fmt.Fprintf(os.Stderr, "npstudy: %v\n", err)
		os.Exit(1)
	}
}

func run(database, target string, mass float64, minSamples int, zwiftCrr float64, strava string) error {
	switch {
	case database == "":
		return errors.New("-database is required")
	case minSamples <= 0:
		return errors.New("-min-samples must be a positive number of samples")
	case zwiftCrr <= 0 || zwiftCrr >= 0.1:
		return errors.New("-zwift-crr must be a rolling resistance a tyre could have")
	}
	var stravaRows []stravaActivity
	if strava != "" {
		rows, err := readStravaActivities(strava)
		if err != nil {
			return err
		}
		stravaRows = rows
	}

	ctx := context.Background()
	databasePath, err := filepath.Abs(database)
	if err != nil {
		return fmt.Errorf("resolving the database path: %w", err)
	}
	store, err := openStore(ctx, databasePath)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "npstudy: closing state: %v\n", closeErr)
		}
	}()

	result, err := study(ctx, store, target, mass, minSamples, zwiftCrr, stravaRows)
	if err != nil {
		return err
	}
	fmt.Print(result.String())

	return nil
}
