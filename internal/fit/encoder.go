// Package fit encodes validated route stages as deterministic FIT courses.
package fit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"time"

	fitencoder "github.com/muktihari/fit/encoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/mesgdef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/nobbs/domestique/internal/measure"
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
	return e.EncodeWithCues(ctx, stage, nil)
}

// EncodeWithCues is Encode with turn instructions as course points, each on
// the record nearest its distance along the route.
//
//nolint:gocritic // This method conforms to the sync package's value contract.
func (e *Encoder) EncodeWithCues(ctx context.Context, stage route.Route, cues []route.Cue) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("fit: encoding cancelled: %w", err)
	}

	createdAt := courseTimestamp(stage.Key())
	course := filedef.NewCourse()
	course.FileId.SetType(typedef.FileCourse).
		SetManufacturer(typedef.ManufacturerDevelopment).
		SetSerialNumber(courseSerial(stage.Key())).
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
			distance += measure.HaversineMetres(geometry[index-1].Coordinate(), point.Coordinate())
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
	course.CoursePoints = coursePoints(course.Records, cues)

	encoded := course.ToFIT(nil)
	var buffer bytes.Buffer
	if err := fitencoder.New(&buffer).Encode(&encoded); err != nil {
		return nil, fmt.Errorf("fit: encoding course: %w", err)
	}

	return buffer.Bytes(), nil
}

// coursePoints places each cue on the record nearest its distance, so a
// device reaches it at the same moment as that point of the line.
func coursePoints(records []*mesgdef.Record, cues []route.Cue) []*mesgdef.CoursePoint {
	if len(records) == 0 {
		return nil
	}
	points := make([]*mesgdef.CoursePoint, 0, len(cues))
	for _, cue := range cues {
		kind, name, known := cuePoint(cue)
		if !known {
			continue
		}
		index := sort.Search(len(records), func(index int) bool {
			return records[index].DistanceScaled() >= cue.Metres
		})
		if index == len(records) || (index > 0 &&
			cue.Metres-records[index-1].DistanceScaled() < records[index].DistanceScaled()-cue.Metres) {
			index--
		}
		record := records[index]
		points = append(points, mesgdef.NewCoursePoint(nil).
			SetTimestamp(record.Timestamp).
			SetPositionLat(record.PositionLat).
			SetPositionLong(record.PositionLong).
			SetDistance(record.Distance).
			SetType(kind).
			SetName(name))
	}

	return points
}

// cuePoint is the course point type a device draws a turn with, and the
// short name it shows beside it.
func cuePoint(cue route.Cue) (typedef.CoursePoint, string, bool) {
	switch cue.Turn {
	case route.TurnStraight:
		return typedef.CoursePointStraight, "Straight on", true
	case route.TurnLeft:
		return typedef.CoursePointLeft, "Left", true
	case route.TurnSlightLeft:
		return typedef.CoursePointSlightLeft, "Slight left", true
	case route.TurnSharpLeft:
		return typedef.CoursePointSharpLeft, "Sharp left", true
	case route.TurnRight:
		return typedef.CoursePointRight, "Right", true
	case route.TurnSlightRight:
		return typedef.CoursePointSlightRight, "Slight right", true
	case route.TurnSharpRight:
		return typedef.CoursePointSharpRight, "Sharp right", true
	case route.TurnKeepLeft:
		return typedef.CoursePointLeftFork, "Keep left", true
	case route.TurnKeepRight:
		return typedef.CoursePointRightFork, "Keep right", true
	case route.TurnUTurn:
		return typedef.CoursePointUTurn, "U-turn", true
	case route.TurnRoundabout:
		if cue.Exit > 0 {
			return typedef.CoursePointGeneric, fmt.Sprintf("Roundabout exit %d", cue.Exit), true
		}

		return typedef.CoursePointGeneric, "Roundabout", true
	default:
		return typedef.CoursePointGeneric, "", false
	}
}

// courseSerial tells one route's file_id from another's: a head unit that
// identifies courses by file_id keeps only one of several identical ones.
func courseSerial(key route.Key) uint32 {
	sum := sha256.Sum256([]byte(key.ExternalID()))

	return binary.BigEndian.Uint32(sum[:4])
}

// courseTimestamp gives each route a route-specific, past time_created: a head
// unit that keys courses on it alone keeps only one of several that share it.
func courseTimestamp(key route.Key) time.Time {
	const spreadSeconds = 1 << 28 // about 8.5 years, so every course stays in the past
	offset := time.Duration(courseSerial(key)%spreadSeconds) * time.Second

	return time.Date(2010, time.January, 1, 0, 0, 0, 0, time.UTC).Add(offset)
}
