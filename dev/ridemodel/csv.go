package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

// stravaDateLayout is the layout Strava's bulk export writes Activity Date
// in, e.g. "Aug 24, 2026, 6:00:00 AM". A row whose date does not parse in
// this layout keeps a zero Date rather than failing the row: nothing this
// tool computes depends on it beyond chronological report ordering.
const stravaDateLayout = "Jan 2, 2006, 3:04:05 PM"

// activitiesCSVColumns names the activities.csv columns this tool reads, by
// header name rather than position: Strava's own export carries many more
// columns, in an order that has changed between export versions.
type activitiesCSVColumns struct {
	id, date, activityType, distance, elapsedTime, movingTime, elevationGain, gear, filename int
}

// readActivitiesCSV reads activities.csv, tolerant of extra or reordered
// columns. Only a row missing its ID is skipped here: an ID names the
// activity a report can name back, even when its Filename is empty —
// ingestActivity is what turns a missing Filename into a reported
// exclusionNoSourceFile rather than a row silently absent from every count.
func readActivitiesCSV(path string) ([]activityRow, error) {
	file, err := os.Open(path) //nolint:gosec // The path is the operator's own -export flag, joined with the fixed activities.csv name.
	if err != nil {
		return nil, fmt.Errorf("opening activities.csv: %w", err)
	}
	defer closeFile(file)

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("reading activities.csv header: %w", err)
	}
	columns, err := columnsOf(header)
	if err != nil {
		return nil, err
	}

	var rows []activityRow
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("reading activities.csv: %w", readErr)
		}
		row, ok := parseActivityRow(record, columns)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}

	return rows, nil
}

// columnsOf finds each required column by header name. Strava's own export
// repeats several headers — "Distance" and "Elapsed Time" each appear twice,
// once in an early summary block and again in a later detailed one — and the
// two are not always the same unit: the first "Distance" is kilometres, the
// second is metres. The last occurrence is always the detailed, metric-base
// column this tool wants, so that is what wins rather than the first.
func columnsOf(header []string) (activitiesCSVColumns, error) {
	find := func(name string) int {
		found := -1
		for i, cell := range header {
			if strings.EqualFold(cell, name) {
				found = i
			}
		}

		return found
	}
	columns := activitiesCSVColumns{
		id:            find("Activity ID"),
		date:          find("Activity Date"),
		activityType:  find("Activity Type"),
		distance:      find("Distance"),
		elapsedTime:   find("Elapsed Time"),
		movingTime:    find("Moving Time"),
		elevationGain: find("Elevation Gain"),
		gear:          find("Activity Gear"),
		filename:      find("Filename"),
	}
	if columns.id < 0 || columns.filename < 0 {
		return columns, fmt.Errorf("activities.csv: missing required column %q or %q", "Activity ID", "Filename")
	}

	return columns, nil
}

// parseActivityRow reads one row by the columns found in the header. Only a
// missing ID drops the row here: a missing Filename or an unparseable field
// still names a real activity, and ingestActivity is where that becomes a
// reported exclusion rather than a row silently absent from every count.
func parseActivityRow(record []string, columns activitiesCSVColumns) (activityRow, bool) {
	id := cell(record, columns.id)
	if id == "" {
		return activityRow{}, false
	}

	row := activityRow{
		ID:                  id,
		Type:                cell(record, columns.activityType),
		DistanceMetres:      parseFloat(cell(record, columns.distance)),
		ElapsedTime:         parseSeconds(cell(record, columns.elapsedTime)),
		StravaMovingTime:    parseSeconds(cell(record, columns.movingTime)),
		ElevationGainMetres: parseFloat(cell(record, columns.elevationGain)),
		Gear:                cell(record, columns.gear),
		Filename:            cell(record, columns.filename),
	}
	if parsed, err := time.Parse(stravaDateLayout, cell(record, columns.date)); err == nil {
		row.Date = parsed
	}

	return row, true
}

func cell(record []string, index int) string {
	if index < 0 || index >= len(record) {
		return ""
	}

	return strings.TrimSpace(record[index])
}

func parseFloat(value string) float64 {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}

	return parsed
}

// parseSeconds reads Strava's elapsed/moving time columns, which are plain
// integer seconds.
func parseSeconds(value string) time.Duration {
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}

	return time.Duration(parsed * float64(time.Second))
}
