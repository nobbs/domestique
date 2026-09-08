// Command windstudy answers #587: for every recorded ride with a track and no
// power meter, how three candidate treatments of wind compare against the
// handover's own diagnostics (docs/references/power-estimation-handover.md
// §6-§10) — none, the recorded weather injected as a signed term, and a wind
// vector fitted from the ride's own heart rate.
//
// Development tooling, not part of the shipped binary and never run in quick
// or check: it needs the operator's own snapshot of real rides. Its report is
// aggregate numbers only — no ride identifier, date, position, heading series
// or altitude value — and is safe to paste into an issue.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/sqlite"
)

func main() {
	database := flag.String("database", "", "state database to read recorded rides from")
	minSamples := flag.Int("min-samples", 60, "rides with fewer positioned track samples are skipped and counted")
	block := flag.Duration("block", 5*time.Minute, "heart-rate block length the wind fit scores blocks over")
	speeds := flag.String("speeds", "0,0.5,1,1.5,2,3,4,5,6", "comma-separated candidate wind speeds in m/s")
	directionsStep := flag.Float64("directions-step", 10, "degrees between candidate wind directions in the grid search")
	mass := flag.Float64("mass", 0, "total system mass in kg, used for a target with no rider profile")
	flag.Parse()

	if err := run(*database, *minSamples, *block, *speeds, *directionsStep, *mass); err != nil {
		fmt.Fprintf(os.Stderr, "windstudy: %v\n", err)
		os.Exit(1)
	}
}

func run(database string, minSamples int, block time.Duration, speedsList string, directionsStepDeg, massFlag float64) error {
	if database == "" {
		return errors.New("-database is required")
	}
	if minSamples <= 0 {
		return errors.New("-min-samples must be a positive number of samples")
	}
	if block <= 0 {
		return errors.New("-block must be a positive duration")
	}
	speedsMS, err := parseNonNegativeSpeeds(speedsList)
	if err != nil {
		return err
	}
	if directionsStepDeg <= 0 || directionsStepDeg >= 360 {
		return errors.New("-directions-step must be greater than 0 and less than 360")
	}
	if massFlag <= 0 {
		return errors.New("-mass must be a positive number of kilograms")
	}

	ctx := context.Background()
	// A placeholder key: this tool only reads, and the development snapshot's
	// stored credentials are already undecryptable.
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
			fmt.Fprintf(os.Stderr, "windstudy: closing state: %v\n", closeErr)
		}
	}()

	result, err := study(ctx, store, minSamples, block, speedsMS, directionsStepDeg, massFlag)
	if err != nil {
		return err
	}
	fmt.Print(result.String())

	return nil
}

// parseNonNegativeSpeeds splits a comma-separated list of non-negative,
// finite candidate wind speeds in m/s — zero is a valid speed here, unlike
// ascentstudy's distance lists, since "no wind" is one of the grid points.
func parseNonNegativeSpeeds(list string) ([]float64, error) {
	values := make([]float64, 0)
	for part := range strings.SplitSeq(list, ",") {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		value, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return nil, fmt.Errorf("parsing speed %q: %w", trimmed, err)
		}
		if value < 0 {
			return nil, fmt.Errorf("speed %q must not be negative", trimmed)
		}
		values = append(values, value)
	}
	if len(values) == 0 {
		return nil, errors.New("at least one candidate speed is required")
	}

	return values, nil
}
