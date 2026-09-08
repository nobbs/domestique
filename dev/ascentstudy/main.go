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

	"github.com/nobbs/domestique/internal/sqlite"
)

func main() {
	database := flag.String("database", "", "state database to read recorded rides from")
	thresholds := flag.String("thresholds", "1.2,2,3", "comma-separated hysteresis thresholds in metres")
	minSamples := flag.Int("min-samples", 60, "rides with fewer track samples are skipped and counted")
	flag.Parse()

	if err := run(*database, *thresholds, *minSamples); err != nil {
		fmt.Fprintf(os.Stderr, "ascentstudy: %v\n", err)
		os.Exit(1)
	}
}

func run(database, thresholdList string, minSamples int) error {
	if database == "" {
		return errors.New("-database is required")
	}
	thresholds, err := parseThresholds(thresholdList)
	if err != nil {
		return err
	}

	ctx := context.Background()
	// A placeholder key: this tool only reads, and the development snapshot's
	// stored credentials are already undecryptable.
	var key [32]byte
	store, err := sqlite.Open(ctx, database, key)
	if err != nil {
		return fmt.Errorf("opening state: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "ascentstudy: closing state: %v\n", closeErr)
		}
	}()

	report, err := study(ctx, store, thresholds, minSamples)
	if err != nil {
		return err
	}
	fmt.Print(report.String())

	return nil
}
