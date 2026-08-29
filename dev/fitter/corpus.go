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

// csvColumns maps a header name to its index for the corpus files this package
// reads. dev/ridemodel writes them in a fixed column order, so reading by name
// only tolerates it reordering its own columns.
type csvColumns map[string]int

func columnsByName(header []string) csvColumns {
	columns := make(csvColumns, len(header))
	for i, name := range header {
		columns[name] = i
	}

	return columns
}

func (c csvColumns) str(record []string, name string) string {
	if i, ok := c[name]; ok && i < len(record) {
		return record[i]
	}

	return ""
}

// float parses a numeric column, defaulting to zero for a missing or
// unparseable value — a corpus this package's own dev/ridemodel wrote is
// trusted, so an empty cell means "not applicable" (an unlabelled surface,
// an absent optional channel), not a reason to fail the whole read.
func (c csvColumns) float(record []string, name string) float64 {
	v, _ := strconv.ParseFloat(c.str(record, name), 64) //nolint:errcheck // zero is the correct default for a missing or blank cell

	return v
}

func (c csvColumns) boolean(record []string, name string) bool {
	v, _ := strconv.ParseBool(c.str(record, name)) //nolint:errcheck // false is the correct default for a missing or blank cell

	return v
}

func (c csvColumns) time(record []string, name string) time.Time {
	t, _ := time.Parse(time.RFC3339, c.str(record, name)) //nolint:errcheck // a zero time is the correct default for a missing or blank cell

	return t
}

// readCorpusCSV reads a corpus file dev/ridemodel wrote, calling build for every
// data row. Generic over the row type so three files share one reader.
// requiredColumns names what the contract cannot do without: a row missing one
// means the file is not the corpus this package expects, not an absent channel.
func readCorpusCSV[T any](path string, requiredColumns []string, build func(record []string, columns csvColumns) T) ([]T, error) {
	file, err := os.Open(path) //nolint:gosec // The path is the operator's own -corpus flag, joined with this package's fixed file names.
	if err != nil {
		return nil, fmt.Errorf("opening %s: %w", path, err)
	}
	defer closeFile(file)

	reader := csv.NewReader(file)
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("reading %s header: %w", path, err)
	}
	columns := columnsByName(header)

	var missing []string
	for _, name := range requiredColumns {
		if _, ok := columns[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("%s is missing required column(s): %s", path, strings.Join(missing, ", "))
	}

	var rows []T
	for {
		record, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("reading %s: %w", path, readErr)
		}
		rows = append(rows, build(record, columns))
	}

	return rows, nil
}

func readSamplesCSV(path string) ([]sampleRow, error) {
	// Everything the route-only fit and the hybrid benchmark key decisions on:
	// geometry, timing, and the moving flag. dev/ridemodel still writes cadence,
	// temperature and heart rate, but nothing here reads them.
	required := []string{
		"ride_id", "time", "delta_seconds", "interval_distance_m", "speed_mps", "gradient_percent",
		"altitude_m", "has_altitude", "latitude", "longitude", "has_position", "moving",
	}

	return readCorpusCSV(path, required, func(record []string, c csvColumns) sampleRow {
		return sampleRow{
			RideID:           c.str(record, "ride_id"),
			Time:             c.time(record, "time"),
			DeltaSeconds:     c.float(record, "delta_seconds"),
			IntervalDistance: c.float(record, "interval_distance_m"),
			SpeedMPS:         c.float(record, "speed_mps"),
			GradientPercent:  c.float(record, "gradient_percent"),
			AltitudeM:        c.float(record, "altitude_m"),
			HasAltitude:      c.boolean(record, "has_altitude"),
			Latitude:         c.float(record, "latitude"),
			Longitude:        c.float(record, "longitude"),
			HasPosition:      c.boolean(record, "has_position"),
			Moving:           c.boolean(record, "moving"),
		}
	})
}

func readRidesCSV(path string) ([]rideRow, error) {
	required := []string{"ride_id", "date", "gear", "moving_seconds"}

	return readCorpusCSV(path, required, func(record []string, c csvColumns) rideRow {
		return rideRow{
			RideID:        c.str(record, "ride_id"),
			Date:          c.time(record, "date"),
			Gear:          c.str(record, "gear"),
			MovingSeconds: c.float(record, "moving_seconds"),
		}
	})
}

//nolint:errcheck // A file opened for reading has nothing to report on close.
func closeFile(f *os.File) { _ = f.Close() }
