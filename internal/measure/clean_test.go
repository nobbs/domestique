package measure_test

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/stretchr/testify/assert"
)

func TestCapHeartRateInterpolatesASingleSpike(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 150},
		{At: start().Add(time.Second), Value: 220},
		{At: start().Add(2 * time.Second), Value: 152},
	}

	cleaned := measure.CapHeartRate(readings, 200)

	assert.InDelta(t, 151, cleaned[1].Value, 1e-9)
}

func TestCapHeartRateFollowsTimeNotIndexAcrossAnUnevenRun(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 100},
		{At: start().Add(time.Second), Value: 220},
		{At: start().Add(2 * time.Second), Value: 230},
		{At: start().Add(10 * time.Second), Value: 240},
		{At: start().Add(11 * time.Second), Value: 200},
	}

	cleaned := measure.CapHeartRate(readings, 200)

	// interpolate 100 -> 200 over the 11 second span between the plausible neighbours.
	assert.InDelta(t, 100+100*1.0/11, cleaned[1].Value, 1e-9)
	assert.InDelta(t, 100+100*2.0/11, cleaned[2].Value, 1e-9)
	assert.InDelta(t, 100+100*10.0/11, cleaned[3].Value, 1e-9)
}

func TestCapHeartRateCopiesTheNeighbourForARunAtTheStart(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 220},
		{At: start().Add(time.Second), Value: 150},
	}

	cleaned := measure.CapHeartRate(readings, 200)

	assert.InDelta(t, 150, cleaned[0].Value, 1e-9)
}

func TestCapHeartRateCopiesTheNeighbourForARunAtTheEnd(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 150},
		{At: start().Add(time.Second), Value: 220},
	}

	cleaned := measure.CapHeartRate(readings, 200)

	assert.InDelta(t, 150, cleaned[1].Value, 1e-9)
}

func TestCapHeartRateReturnsUnchangedWhenNoneArePlausible(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 220},
		{At: start().Add(time.Second), Value: 230},
	}

	cleaned := measure.CapHeartRate(readings, 200)

	assert.Equal(t, readings, cleaned)
}

func TestCapHeartRateReturnsUnchangedForANonPositiveMax(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{{At: start(), Value: 220}}

	cleaned := measure.CapHeartRate(readings, 0)

	assert.Equal(t, readings, cleaned)
}

func TestCapHeartRateDoesNotMutateTheInput(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 150},
		{At: start().Add(time.Second), Value: 220},
		{At: start().Add(2 * time.Second), Value: 152},
	}
	before := append([]measure.Reading(nil), readings...)

	measure.CapHeartRate(readings, 200)

	assert.Equal(t, before, readings)
}

func TestCapHeartRateHandlesEmptyAndSingleReadingSeries(t *testing.T) {
	t.Parallel()
	assert.Empty(t, measure.CapHeartRate(nil, 200))

	single := []measure.Reading{{At: start(), Value: 150}}
	assert.Equal(t, single, measure.CapHeartRate(single, 200))
}

func TestClampPowerHoldsReadingsAboveTheThreshold(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{
		{At: start(), Value: 150},
		{At: start().Add(time.Second), Value: 300},
		{At: start().Add(2 * time.Second), Value: 301},
	}

	cleaned, touched := measure.ClampPower(readings, 300)

	assert.InDelta(t, 150.0, cleaned[0].Value, 1e-9, "below the threshold is untouched")
	assert.InDelta(t, 300.0, cleaned[1].Value, 1e-9, "at the threshold is untouched")
	assert.InDelta(t, 300.0, cleaned[2].Value, 1e-9, "above the threshold is clamped")
	assert.Equal(t, 1, touched)
}

func TestClampPowerClampsNothingForANonPositiveThreshold(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{{At: start(), Value: 300}}

	cleaned, touched := measure.ClampPower(readings, 0)

	assert.Equal(t, readings, cleaned)
	assert.Equal(t, 0, touched)
}

func TestClampPowerDoesNotMutateTheInput(t *testing.T) {
	t.Parallel()
	readings := []measure.Reading{{At: start(), Value: 400}}
	before := append([]measure.Reading(nil), readings...)

	measure.ClampPower(readings, 300)

	assert.Equal(t, before, readings)
}

func TestClampPowerHandlesEmptyAndSingleReadingSeries(t *testing.T) {
	t.Parallel()
	cleaned, touched := measure.ClampPower(nil, 300)
	assert.Empty(t, cleaned)
	assert.Equal(t, 0, touched)

	single := []measure.Reading{{At: start(), Value: 150}}
	cleaned, touched = measure.ClampPower(single, 300)
	assert.Equal(t, single, cleaned)
	assert.Equal(t, 0, touched)
}

func TestCapHeartRateTakesTheEarlierReadingWhenTheClockDidNotAdvance(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 8, 24, 6, 0, 0, 0, time.UTC)
	readings := []measure.Reading{{At: at, Value: 120}, {At: at, Value: 250}, {At: at, Value: 130}}

	cleaned := measure.CapHeartRate(readings, 200)

	assert.InDelta(t, 120, cleaned[1].Value, 1e-9)
}
