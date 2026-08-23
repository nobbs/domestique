package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

func main() {
	exportDir := flag.String("export", "",
		"path to the Strava bulk export directory (containing activities.csv and activities/)")
	outDir := flag.String("out", ".local/ridemodel",
		"directory to write samples.csv, rides.csv and indoor.csv to")
	flag.Parse()

	if err := run(*exportDir, *outDir); err != nil {
		fmt.Fprintf(os.Stderr, "ridemodel: %v\n", err)
		os.Exit(1)
	}
}

// run reads the export, ingests every cycling activity in it, writes the
// three output tables, and prints the corpus report. No network access and
// no module dependency beyond what go.mod already carries: the whole ingest
// is encoding/csv, encoding/xml, compress/gzip and github.com/muktihari/fit.
func run(exportDir, outDir string) error {
	if exportDir == "" {
		return errors.New("-export is required")
	}

	rows, err := readActivitiesCSV(filepath.Join(exportDir, "activities.csv"))
	if err != nil {
		return err
	}

	var samples []sample
	var indoor []indoorSample
	summaries := make([]rideSummary, 0, len(rows))
	for i := range rows {
		summary, rideSamples, indoorRows := ingestActivity(exportDir, &rows[i])
		summaries = append(summaries, summary)
		samples = append(samples, rideSamples...)
		indoor = append(indoor, indoorRows...)
	}

	if err := writeCorpus(outDir, samples, indoor, summaries); err != nil {
		return err
	}

	report := buildReport(summaries)
	fmt.Println(renderReport(&report))
	fmt.Printf("wrote %s/samples.csv, %s/rides.csv, %s/indoor.csv\n", outDir, outDir, outDir)

	return nil
}
