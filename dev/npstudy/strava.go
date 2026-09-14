package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// stravaDateLayout is how a Strava bulk export writes Activity Date, in UTC.
const stravaDateLayout = "Jan 2, 2006, 3:04:05 PM"

// readStravaActivities reads the start and average power of every activity in
// a Strava export's activities.csv. Columns are found by header name, the last
// occurrence winning, since the export repeats some headers and reorders them
// between versions. A row without a parseable date or power is skipped.
func readStravaActivities(path string) ([]stravaActivity, error) {
	file, err := os.Open(path) //nolint:gosec // The operator's own -strava flag.
	if err != nil {
		return nil, fmt.Errorf("opening the Strava export: %w", err)
	}
	defer file.Close() //nolint:errcheck // read-only

	return parseStravaActivities(file)
}

func parseStravaActivities(source io.Reader) ([]stravaActivity, error) {
	reader := csv.NewReader(source)
	reader.FieldsPerRecord = -1
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("reading the Strava export header: %w", err)
	}
	date, watts := -1, -1
	for index, name := range header {
		switch strings.TrimSpace(name) {
		case "Activity Date":
			date = index
		case "Average Watts":
			watts = index
		}
	}
	if date < 0 || watts < 0 {
		return nil, errors.New(`the Strava export has no "Activity Date" or "Average Watts" column`)
	}

	var activities []stravaActivity
	for {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			return activities, nil
		}
		if readErr != nil {
			return nil, fmt.Errorf("reading the Strava export: %w", readErr)
		}
		if max(date, watts) >= len(record) {
			continue
		}
		start, dateErr := time.Parse(stravaDateLayout, record[date])
		average, wattsErr := strconv.ParseFloat(record[watts], 64)
		if dateErr != nil || wattsErr != nil {
			continue
		}
		activities = append(activities, stravaActivity{start: start, averageWatts: average})
	}
}
