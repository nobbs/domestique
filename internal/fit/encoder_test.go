package fit

import (
	"bytes"
	"context"
	"testing"

	"github.com/muktihari/fit/decoder"
	"github.com/muktihari/fit/profile/filedef"
	"github.com/muktihari/fit/profile/typedef"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/route"
)

func TestEncoderEncodeCreatesDecodableCourse(t *testing.T) {
	encoded, err := New().Encode(t.Context(), testStage(t))
	require.NoError(t, err)
	require.NotEmpty(t, encoded, "Encode() returned no FIT data")

	course := decodeCourse(t, encoded)
	assert.Equal(t, typedef.FileCourse, course.FileId.Type)
	assert.Equal(t, "domestique", course.FileId.ProductName)
	require.NotNil(t, course.Course, "course message is missing")
	assert.Equal(t, "Morning ride — Climb", course.Course.Name)
	assert.Equal(t, typedef.SportCycling, course.Course.Sport)
	require.Len(t, course.Records, 2)

	first := course.Records[0]
	assert.InDelta(t, 49.0, first.PositionLatDegrees(), 0.000001, "first latitude")
	assert.InDelta(t, 8.4, first.PositionLongDegrees(), 0.000001, "first longitude")
	assert.InDelta(t, 321.4, first.AltitudeScaled(), 0.001, "first altitude")
	assert.InDelta(t, 0, first.DistanceScaled(), 0.001, "the first record starts the distance at zero")
	assert.Positive(t, course.Records[1].DistanceScaled(), "the second record carries no cumulative metres")
	assert.InDelta(t, 1.0, course.Records[1].Timestamp.Sub(first.Timestamp).Seconds(), 0,
		"records must be one second apart")
}

func TestEncoderEncodeIsDeterministic(t *testing.T) {
	encoder := New()
	first, err := encoder.Encode(t.Context(), testStage(t))
	require.NoError(t, err, "first Encode()")
	second, err := encoder.Encode(t.Context(), testStage(t))
	require.NoError(t, err, "second Encode()")
	assert.Equal(t, first, second, "Encode() produced different bytes for the same stage")
}

func TestEncoderEncodeRespectsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	_, err := New().Encode(ctx, testStage(t))
	require.ErrorIs(t, err, context.Canceled)
}

func TestEncoderEncodeRejectsUnsupportedElevation(t *testing.T) {
	elevation := 13_000.0
	stage, err := route.NewStage(
		route.ProviderVeloPlanner,
		1,
		1,
		"2026-08-17T07:00:00",
		"High route",
		"",
		[]route.Point{{Longitude: 8.4, Latitude: 49.0, Elevation: &elevation}, {Longitude: 8.5, Latitude: 49.1}},
		"hash",
	)
	require.NoError(t, err)

	_, err = New().Encode(t.Context(), stage)
	require.Error(t, err, "an elevation outside the FIT range must be refused")
}

func testStage(t *testing.T) route.Stage {
	t.Helper()
	elevation := 321.4
	stage, err := route.NewStage(
		route.ProviderVeloPlanner,
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
	require.NoError(t, err)

	return stage
}

func decodeCourse(t *testing.T, encoded []byte) *filedef.Course {
	t.Helper()
	listener := filedef.NewListener()
	defer listener.Close()

	fitDecoder := decoder.New(bytes.NewReader(encoded), decoder.WithMesgListener(listener), decoder.WithBroadcastOnly())
	_, err := fitDecoder.Decode()
	require.NoError(t, err)

	decoded := listener.File()
	file, ok := decoded.(*filedef.Course)
	require.Truef(t, ok, "decoded file = %T, want *filedef.Course", decoded)

	return file
}
