package main

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nobbs/domestique/internal/measure"
)

func month(n int) time.Time { return time.Date(2025, time.Month(n), 10, 7, 0, 0, 0, time.UTC) }

// sideOnLine is a month side whose blocks sit exactly on watts = intercept +
// slope × heart rate, with one best.
func sideOnLine(intercept, slope, best float64, rates ...float64) *monthSide {
	side := &monthSide{rides: 1, best: best, hasBest: best > 0}
	for _, rate := range rates {
		side.blocks = append(side.blocks, anchorBlock{heartRate: rate, watts: intercept + slope*rate, speed: rate / 20})
	}

	return side
}

func TestWattsAtHeartRateMovesTheMedianLevelToTheReference(t *testing.T) {
	t.Parallel()
	blocks := sideOnLine(-100, 2, 0, 120, 130, 140).blocks
	blocks = append(blocks, anchorBlock{heartRate: 130, watts: 5000}) // an unpaired strap

	got, ok := wattsAtHeartRate(blocks, 2, 125)

	require.True(t, ok)
	assert.InDelta(t, 150, got, 1e-9)
}

func TestWattsAtHeartRateRefusesNoBlocks(t *testing.T) {
	t.Parallel()
	_, ok := wattsAtHeartRate(nil, 2, 125)
	assert.False(t, ok)
}

func TestZoneTwoNeedsEnoughBlocksInsideTheBand(t *testing.T) {
	t.Parallel()
	statistics := anchorStatistics(2, 125, [2]float64{110, 130}, true)
	require.Equal(t, "zone-two", statistics[1].name)

	_, passingThrough := statistics[1].value(sideOnLine(0, 1, 0, 115, 125, 150, 160))
	watts, held := statistics[1].value(sideOnLine(0, 1, 0, 110, 115, 125, 130))

	assert.False(t, passingThrough, "two blocks in the band are a ride passing through it")
	require.True(t, held)
	assert.InDelta(t, (110+115+125)/3.0, watts, 1e-9, "the band's upper bound is exclusive")
}

func TestAnchorStatisticsOmitTheBandWithoutZones(t *testing.T) {
	t.Parallel()
	statistics := anchorStatistics(2, 125, [2]float64{}, false)

	names := make([]string, 0, len(statistics))
	for _, statistic := range statistics {
		names = append(names, statistic.name)
	}
	assert.Equal(t, []string{"hr-matched", "best-20min"}, names)
}

func TestAnchorVerdict(t *testing.T) {
	t.Parallel()
	assert.Contains(t, anchorVerdict([]float64{0.8, 0.8}, 3, 0.1), "too few months")
	assert.Contains(t, anchorVerdict([]float64{0.5, 0.8, 1.1}, 3, 0.1), "unstable")
	assert.Contains(t, anchorVerdict([]float64{0.79, 0.8, 0.81}, 3, 0.1), "stable: a per-rider scale of 1.25")
}

func TestTrackSpeedIsDistanceOverTimeWithinTheSpan(t *testing.T) {
	t.Parallel()
	var track []measure.Sample
	for second := range 100 {
		track = append(track, measure.Sample{At: start().Add(time.Duration(second) * time.Second), DistanceMetres: 5 * float64(second)})
	}

	speed, ok := trackSpeed(track, start().Add(10*time.Second), start().Add(40*time.Second))
	_, beyond := trackSpeed(track, start().Add(time.Hour), start().Add(2*time.Hour))

	require.True(t, ok)
	assert.InDelta(t, 5, speed, 1e-9)
	assert.False(t, beyond)
}

// The regression the speed split rests on: a block's start is where its own
// readings began, not the ride's.
func TestMeteredBlocksOfStampsEachBlocksStart(t *testing.T) {
	t.Parallel()
	var power, heartRate []measure.Reading
	for second := range 600 {
		at := start().Add(time.Duration(second) * time.Second)
		power = append(power, measure.Reading{At: at, Value: 200})
		heartRate = append(heartRate, measure.Reading{At: at, Value: 140})
	}

	blocks := meteredBlocksOf(power, heartRate, 5*time.Minute)

	require.Len(t, blocks, 2)
	assert.Equal(t, start(), blocks[0].Start)
	assert.Equal(t, start().Add(5*time.Minute), blocks[1].Start)
}

func TestAddOutdoorBlocksTheEstimateAndReadsItsSpeedAndBest(t *testing.T) {
	t.Parallel()
	var track []measure.Sample
	var estimates []measure.Estimate
	var heartRate []measure.Reading
	for second := range 1800 {
		at := start().Add(time.Duration(second) * time.Second)
		track = append(track, measure.Sample{At: at, DistanceMetres: 6 * float64(second)})
		estimates = append(estimates, measure.Estimate{Watts: 150, Known: second > 0})
		heartRate = append(heartRate, measure.Reading{At: at, Value: 130})
	}
	months := newAnchorMonths()

	months.addOutdoor(track, estimates, heartRate, 5*time.Minute)

	side := months.outdoor[start().Format("2006-01")]
	require.NotNil(t, side)
	assert.Equal(t, 1, side.rides)
	require.Len(t, side.blocks, 6)
	assert.InDelta(t, 6, side.blocks[0].speed, 1e-9)
	assert.InDelta(t, 150, side.blocks[0].watts, 1e-9)
	require.True(t, side.hasBest)
	assert.InDelta(t, 150, side.best, 1e-9)
}

func TestWriteAnchorReportsOnlyMonthsHoldingBothDomains(t *testing.T) {
	t.Parallel()
	months := newAnchorMonths()
	for _, n := range []int{1, 2, 3} {
		key := month(n).Format("2006-01")
		months.indoor[key] = sideOnLine(-100, 2, 260, 120, 130, 140)
		if n != 3 {
			months.outdoor[key] = sideOnLine(-120, 2, 208, 120, 130, 140)
		}
	}
	var b strings.Builder

	months.writeAnchor(&b, measure.DefaultCoefficients(), 2, [2]float64{}, false, 2, 0.1)

	out := b.String()
	assert.Contains(t, out, "overlap: 2 months, 2 indoor rides (6 blocks), 2 outdoor rides (6 blocks)")
	assert.NotContains(t, out, "2025-03", "a month with no outdoor ride is not overlap")
	assert.Contains(t, out, "2025-01    160.0    140.0   0.88", "hr-matched at the pooled 130 bpm")
	assert.Contains(t, out, "2025-02    260.0    208.0   0.80", "best twenty minutes")
	assert.Contains(t, out, "stable: a per-rider scale of 1.14")
	assert.Contains(t, out, "at or below 0.88 over 2 months, above 0.88 over 2 months")
}

// The regression: a nearest-rank median that lands on a tie must not leave
// every block on one side of the split.
func TestWriteSpeedSplitHalvesBlocksTiedAtTheMedian(t *testing.T) {
	t.Parallel()
	key := month(1).Format("2006-01")
	months := newAnchorMonths()
	months.indoor[key] = sideOnLine(0, 1, 0, 130)
	months.outdoor[key] = &monthSide{blocks: []anchorBlock{
		{heartRate: 130, watts: 100, speed: 5}, {heartRate: 130, watts: 100, speed: 5},
		{heartRate: 130, watts: 120, speed: 7}, {heartRate: 130, watts: 120, speed: 7},
	}}
	var b strings.Builder

	months.writeSpeedSplit(&b, months.overlap(), 1, 130)

	assert.Contains(t, b.String(), "split at 18.0 km/h: at or below 0.77 over 1 months, above 0.92 over 1 months")
}

func TestWriteSpeedSplitSaysSoWithoutSpeeds(t *testing.T) {
	t.Parallel()
	key := month(1).Format("2006-01")
	months := newAnchorMonths()
	months.indoor[key] = sideOnLine(0, 1, 0, 130)
	months.outdoor[key] = &monthSide{blocks: []anchorBlock{{heartRate: 130, watts: 100}}}
	var b strings.Builder

	months.writeSpeedSplit(&b, months.overlap(), 1, 130)

	assert.Contains(t, b.String(), "no outdoor block carried a speed")
}

func TestWriteAnchorSaysSoWithNoOverlap(t *testing.T) {
	t.Parallel()
	months := newAnchorMonths()
	months.indoor[month(1).Format("2006-01")] = sideOnLine(0, 1, 200, 130)
	var b strings.Builder

	months.writeAnchor(&b, measure.DefaultCoefficients(), 2, [2]float64{}, false, 2, 0.1)

	assert.Contains(t, b.String(), "nothing to anchor on")
}
