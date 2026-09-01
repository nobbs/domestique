// Package fit encodes validated route stages as deterministic FIT courses.
package fit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	fitencoder "github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/nobbs/domestique/internal/route"
)

const (
	minimumAltitude = -500.0
	maximumAltitude = 12_606.8
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
func (e *Encoder) Encode(ctx context.Context, stage route.Route) ([]byte, error) {
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
			distance += route.HaversineMetres(geometry[index-1], point)
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
