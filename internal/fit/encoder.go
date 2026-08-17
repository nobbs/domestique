// Package fit encodes validated route stages as deterministic FIT courses.
package fit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	fitencoder "github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/nobbs/domestique/internal/route"
)

const (
	minimumAltitude  = -500.0
	maximumAltitude  = 12_606.8
	earthRadiusMetre = 6_371_000.0
)

// Encoder creates device-ready FIT course files.
type Encoder struct{}

// New creates a FIT encoder.
func New() *Encoder {
	return &Encoder{}
}

// Encode returns a deterministic FIT course for one validated route stage.
//
//nolint:gocritic // This method conforms to the sync package's value contract.
func (e *Encoder) Encode(ctx context.Context, stage route.Stage) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("fit: encoding cancelled: %w", err)
	}

	createdAt := courseTimestamp()
	course := filedef.NewCourse()
	course.FileId.SetType(typedef.FileCourse).
		SetProductName("domestique").
		SetTimeCreated(createdAt)
	course.Course = mesgdef.NewCourse(nil).
		SetName(stage.Title()).
		SetSport(typedef.SportCycling)

	geometry := stage.Geometry()
	distance := 0.0
	for index, point := range geometry {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("fit: encoding cancelled: %w", err)
		}
		if index > 0 {
			distance += haversine(geometry[index-1], point)
		}

		record := mesgdef.NewRecord(nil).
			SetTimestamp(createdAt.Add(time.Duration(index) * time.Second)).
			SetPositionLatDegrees(point.Latitude).
			SetPositionLongDegrees(point.Longitude).
			SetDistanceScaled(distance)
		if point.Elevation != nil {
			if *point.Elevation < minimumAltitude || *point.Elevation > maximumAltitude {
				return nil, errors.New("fit: route elevation is outside the FIT encoding range")
			}
			record.SetAltitudeScaled(*point.Elevation)
		}
		course.Records = append(course.Records, record)
	}

	encoded := course.ToFIT(nil)
	var buffer bytes.Buffer
	if err := fitencoder.New(&buffer).Encode(&encoded); err != nil {
		return nil, fmt.Errorf("fit: encoding course: %w", err)
	}

	return buffer.Bytes(), nil
}

func courseTimestamp() time.Time {
	return time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
}

func haversine(left, right route.Point) float64 {
	latitudeDelta := (right.Latitude - left.Latitude) * math.Pi / 180
	longitudeDelta := (right.Longitude - left.Longitude) * math.Pi / 180
	leftLatitude := left.Latitude * math.Pi / 180
	rightLatitude := right.Latitude * math.Pi / 180
	a := math.Sin(latitudeDelta/2)*math.Sin(latitudeDelta/2) +
		math.Cos(leftLatitude)*math.Cos(rightLatitude)*math.Sin(longitudeDelta/2)*math.Sin(longitudeDelta/2)

	return earthRadiusMetre * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}
