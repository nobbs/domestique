package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nobbs/domestique/internal/ridemodel"
)

func main() {
	corpusDir := flag.String("corpus", "", "directory holding samples.csv and rides.csv from dev/ridemodel")
	coefficientsPath := flag.String("coefficients", "", "path to the ridemodel.toml being evaluated, or recalibrated from")
	recalibrate := flag.Bool(
		"recalibrate", false,
		"refit seconds_per_km and seconds_per_ascent_m over the whole corpus and print a copy-ready profile; "+
			"the default evaluates the loaded profile's own frozen cutoff against rides after it, with no fitting",
	)
	etaRouteCellDegrees := flag.Float64("eta-route-cell-degrees", defaultRouteCellDegrees, "coordinate grid used to identify repeated routes")
	etaRouteJaccard := flag.Float64("eta-route-jaccard", defaultRouteJaccardThreshold, "minimum route-cell Jaccard overlap considered a repeat")
	etaWarmupFraction := flag.Float64(
		"eta-warmup-fraction", defaultBenchmarkWarmupFraction, "oldest share of the corpus recalibrated from, under -recalibrate",
	)
	flag.Parse()

	if err := run(&runConfig{
		corpusDir: *corpusDir, coefficientsPath: *coefficientsPath, recalibrate: *recalibrate,
		etaRouteCellDegrees: *etaRouteCellDegrees, etaRouteJaccard: *etaRouteJaccard, etaWarmupFraction: *etaWarmupFraction,
	}); err != nil {
		fmt.Fprintf(os.Stderr, "fitter: %v\n", err)
		os.Exit(1)
	}
}

type runConfig struct {
	corpusDir           string
	coefficientsPath    string
	etaRouteCellDegrees float64
	etaRouteJaccard     float64
	etaWarmupFraction   float64
	recalibrate         bool
}

func (cfg *runConfig) validate() error {
	if cfg.corpusDir == "" {
		return errors.New("-corpus is required")
	}
	if cfg.coefficientsPath == "" {
		return errors.New("-coefficients is required")
	}
	if cfg.etaRouteCellDegrees <= 0 {
		return errors.New("-eta-route-cell-degrees must be positive")
	}
	if cfg.etaRouteJaccard <= 0 || cfg.etaRouteJaccard > 1 {
		return errors.New("-eta-route-jaccard must be greater than 0 and at most 1")
	}
	if cfg.etaWarmupFraction <= 0 || cfg.etaWarmupFraction >= 1 {
		return errors.New("-eta-warmup-fraction must be greater than 0 and less than 1")
	}

	return nil
}

func run(cfg *runConfig) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	coefficients, err := ridemodel.Load(cfg.coefficientsPath)
	if err != nil {
		return fmt.Errorf("loading coefficients: %w", err)
	}

	samples, err := readSamplesCSV(filepath.Join(cfg.corpusDir, "samples.csv"))
	if err != nil {
		return err
	}
	rides, err := readRidesCSV(filepath.Join(cfg.corpusDir, "rides.csv"))
	if err != nil {
		return err
	}

	samplesByRide := make(map[string][]sampleRow)
	for i := range samples {
		samplesByRide[samples[i].RideID] = append(samplesByRide[samples[i].RideID], samples[i])
	}

	ridesWithSamples := make([]rideRow, 0, len(rides))
	for _, r := range rides {
		if _, ok := samplesByRide[r.RideID]; ok {
			ridesWithSamples = append(ridesWithSamples, r)
		}
	}

	groups := groupRidesByGear(ridesWithSamples)
	report, benchmarkErr := runETABenchmark(groups, ridesWithSamples, samplesByRide, &coefficients, cfg)
	fmt.Print(report)

	return benchmarkErr
}
