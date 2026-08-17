package fit

import (
	"bytes"
	"context"
	"errors"
	"math"
	"testing"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/nobbs/domestique/internal/route"
)

func TestEncoderEncodeCreatesDecodableCourse(t *testing.T) {
	encoded, err := New().Encode(t.Context(), testStage(t))
	if err != nil {
		t.Fatalf("Encode() error = %v", err)
	}
	if len(encoded) == 0 {
		t.Fatal("Encode() returned no FIT data")
	}

	course := decodeCourse(t, encoded)
	if got, want := course.FileId.Type, typedef.FileCourse; got != want {
		t.Errorf("file type = %v, want %v", got, want)
	}
	if got, want := course.FileId.ProductName, "domestique"; got != want {
		t.Errorf("product name = %q, want %q", got, want)
	}
	if course.Course == nil {
		t.Fatal("course message is missing")
	}
	if got, want := course.Course.Name, "Morning ride — Climb"; got != want {
		t.Errorf("course name = %q, want %q", got, want)
	}
	if got, want := course.Course.Sport, typedef.SportCycling; got != want {
		t.Errorf("course sport = %v, want %v", got, want)
	}
	if got, want := len(course.Records), 2; got != want {
		t.Fatalf("record count = %d, want %d", got, want)
	}

	first := course.Records[0]
	if got, want := first.PositionLatDegrees(), 49.0; math.Abs(got-want) > 0.000001 {
		t.Errorf("first latitude = %v, want %v", got, want)
	}
	if got, want := first.PositionLongDegrees(), 8.4; math.Abs(got-want) > 0.000001 {
		t.Errorf("first longitude = %v, want %v", got, want)
	}
	if got, want := first.AltitudeScaled(), 321.4; math.Abs(got-want) > 0.001 {
		t.Errorf("first altitude = %v, want %v", got, want)
	}
	if got, want := course.Records[1].Timestamp.Sub(first.Timestamp).Seconds(), 1.0; got != want {
		t.Errorf("record timestamp interval = %v, want %v", got, want)
	}
}

func TestEncoderEncodeIsDeterministic(t *testing.T) {
	encoder := New()
	first, err := encoder.Encode(t.Context(), testStage(t))
	if err != nil {
		t.Fatalf("first Encode() error = %v", err)
	}
	second, err := encoder.Encode(t.Context(), testStage(t))
	if err != nil {
		t.Fatalf("second Encode() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("Encode() produced different bytes for the same stage")
	}
}

func TestEncoderEncodeRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := New().Encode(ctx, testStage(t))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Encode() error = %v, want context.Canceled", err)
	}
}

func TestEncoderEncodeRejectsUnsupportedElevation(t *testing.T) {
	elevation := 13_000.0
	stage, err := route.NewStage(
		1,
		1,
		"2026-08-17T07:00:00",
		"High route",
		"",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation}, {Longitude: 8.5, Latitude: 49.1}},
		"hash",
	)
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	_, err = New().Encode(t.Context(), stage)
	if err == nil {
		t.Fatal("Encode() error = nil, want an elevation range error")
	}
}

func testStage(t *testing.T) route.Stage {
	t.Helper()
	elevation := 321.4
	stage, err := route.NewStage(
		100,
		2,
		"2026-08-17T07:00:00",
		"Morning ride",
		"Climb",
		[]route.Point{
			{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation},
			{Longitude: 8.5, Latitude: 49.1},
		},
		"hash",
	)
	if err != nil {
		t.Fatalf("NewStage() error = %v", err)
	}

	return stage
}

func decodeCourse(t *testing.T, encoded []byte) *filedef.Course {
	t.Helper()
	listener := filedef.NewListener()
	defer listener.Close()

	fitDecoder := decoder.New(bytes.NewReader(encoded), decoder.WithMesgListener(listener), decoder.WithBroadcastOnly())
	if _, err := fitDecoder.Decode(); err != nil {
		t.Fatalf("Decode() error = %v", err)
	}
	file, ok := listener.File().(*filedef.Course)
	if !ok {
		t.Fatalf("decoded file = %T, want *filedef.Course", file)
	}

	return file
}
