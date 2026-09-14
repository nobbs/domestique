// Command matchstudy measures what the route matcher's acceptance band costs:
// how close the rides it leaves unmatched came to a library route, and what a
// looser band would match, contest between two routes, or move to another.
//
// Development tooling, not part of the shipped binary: it needs the operator's
// own snapshot of real rides and library. Its report is counts and
// distributions only — no ride or route identifier, name or position — and is
// safe to paste into an issue. Nothing it measures is written back.
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

	"github.com/nobbs/domestique/internal/sqlite"
)

func main() {
	database := flag.String("database", "", "state database to read rides and the library from; a copy, since opening migrates it")
	target := flag.String("target", "", "the target slot to read rides from; required where the database holds several")
	bands := flag.String("bands", "0.9,0.8,0.7,0.6", "comma-separated candidate bands to compare with production's")
	flag.Parse()

	if err := run(*database, *target, *bands); err != nil {
		fmt.Fprintf(os.Stderr, "matchstudy: %v\n", err)
		os.Exit(1)
	}
}

func run(database, target, bandList string) error {
	if database == "" {
		return errors.New("-database is required")
	}
	bands, err := parseBands(bandList)
	if err != nil {
		return err
	}

	ctx := context.Background()
	path, err := filepath.Abs(database)
	if err != nil {
		return fmt.Errorf("resolving the database path: %w", err)
	}
	var key [32]byte // this tool writes no encrypted column
	store, err := sqlite.Open(ctx, path, key)
	if err != nil {
		return fmt.Errorf("opening state: %w", err)
	}
	defer func() {
		if closeErr := store.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "matchstudy: closing state: %v\n", closeErr)
		}
	}()

	result, err := study(ctx, store, target, bands)
	if err != nil {
		return err
	}
	fmt.Print(result.String())

	return nil
}

func parseBands(list string) ([]float64, error) {
	var bands []float64
	for field := range strings.SplitSeq(list, ",") {
		band, err := strconv.ParseFloat(strings.TrimSpace(field), 64)
		if err != nil || band <= 0 || band > 1 {
			return nil, fmt.Errorf("-bands: %q is not a share between 0 and 1", field)
		}
		bands = append(bands, band)
	}

	return bands, nil
}
