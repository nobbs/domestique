// Same-month anchoring: the outdoor estimate set beside the indoor meter over
// the calendar months that hold both, by statistics either domain can express.
// Never the estimate worked out on an indoor ride: a trainer's speed and
// gradient are a simulation, so that comparison would look like validation
// and not be one.
package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/rider"
)

// bestWindow is the matched duration the best-effort statistic reads.
const bestWindow = 20 * time.Minute

// minBandBlocks is how many blocks a month's side must hold inside the band
// before its band mean is read; one block is a ride passing through.
const minBandBlocks = 3

// An anchorBlock is one steady span of either domain: its mean heart rate,
// its mean power (measured indoors, estimated outdoors) and, outdoors only,
// its mean ground speed in m/s.
type anchorBlock struct {
	heartRate, watts, speed float64
}

// A monthSide is one domain's rides within one calendar month.
type monthSide struct {
	blocks  []anchorBlock
	rides   int
	best    float64
	hasBest bool
}

func (s *monthSide) add(blocks []anchorBlock, best float64, hasBest bool) {
	s.rides++
	s.blocks = append(s.blocks, blocks...)
	if hasBest && (!s.hasBest || best > s.best) {
		s.best, s.hasBest = best, true
	}
}

// anchorMonths holds both domains per calendar month, keyed "2006-01".
type anchorMonths struct {
	indoor, outdoor map[string]*monthSide
}

func newAnchorMonths() anchorMonths {
	return anchorMonths{indoor: map[string]*monthSide{}, outdoor: map[string]*monthSide{}}
}

func sideOf(sides map[string]*monthSide, at time.Time) *monthSide {
	key := at.UTC().Format("2006-01")
	if sides[key] == nil {
		sides[key] = &monthSide{}
	}

	return sides[key]
}

// addIndoor adds one metered ride's contribution: its bridge blocks and its
// best measured power over bestWindow, coasting included.
func (m anchorMonths) addIndoor(at time.Time, blocks []MeasuredBlock, times []time.Time, watts []float64) {
	converted := make([]anchorBlock, 0, len(blocks))
	for _, block := range blocks {
		converted = append(converted, anchorBlock{heartRate: block.HeartRateBPM, watts: block.WattsMeasured})
	}
	best, ok := rider.BestAverage(times, watts, bestWindow)
	sideOf(m.indoor, at).add(converted, best, ok)
}

// addOutdoor cuts a road ride's estimate the way a metered ride's power is
// cut, so both sides of a month are blocked by one rule.
func (m anchorMonths) addOutdoor(track []measure.Sample, estimates []measure.Estimate, heartRate []measure.Reading, block time.Duration) {
	power := make([]measure.Reading, 0, len(track))
	for index := range track {
		if estimates[index].Known {
			power = append(power, measure.Reading{At: track[index].At, Value: estimates[index].Watts})
		}
	}
	if len(power) == 0 {
		return
	}
	times, watts := make([]time.Time, len(power)), make([]float64, len(power))
	for index, reading := range power {
		times[index], watts[index] = reading.At, reading.Value
	}
	best, ok := rider.BestAverage(times, watts, bestWindow)

	measured := meteredBlocksOf(power, heartRate, block)
	blocks := make([]anchorBlock, 0, len(measured))
	for _, cut := range measured {
		speed, _ := trackSpeed(track, cut.Start, cut.Start.Add(block))
		blocks = append(blocks, anchorBlock{heartRate: cut.HeartRateBPM, watts: cut.WattsMeasured, speed: speed})
	}
	sideOf(m.outdoor, track[0].At).add(blocks, best, ok)
}

// trackSpeed is the mean ground speed across the track samples within
// [from, to], in m/s.
func trackSpeed(track []measure.Sample, from, to time.Time) (float64, bool) {
	first := sort.Search(len(track), func(i int) bool { return !track[i].At.Before(from) })
	last := sort.Search(len(track), func(i int) bool { return track[i].At.After(to) }) - 1
	if first >= last {
		return 0, false
	}
	seconds := track[last].At.Sub(track[first].At).Seconds()

	return (track[last].DistanceMetres - track[first].DistanceMetres) / seconds, seconds > 0
}

// An anchorStatistic is one way of reading a month's side as one number.
type anchorStatistic struct {
	value func(side *monthSide) (float64, bool)
	name  string
}

// anchorStatistics are power at a matched heart rate along the bridge's own
// slope, the mean over blocks held inside the rider's zone two, and the best
// bestWindow of the month. band is absent where the profile cuts no zones.
func anchorStatistics(slope, referenceBPM float64, band [2]float64, bandOK bool) []anchorStatistic {
	statistics := []anchorStatistic{{
		name: "hr-matched",
		value: func(side *monthSide) (float64, bool) {
			return wattsAtHeartRate(side.blocks, slope, referenceBPM)
		},
	}}
	if bandOK {
		statistics = append(statistics, anchorStatistic{
			name: "zone-two",
			value: func(side *monthSide) (float64, bool) {
				var total float64
				var count int
				for _, block := range side.blocks {
					if block.heartRate >= band[0] && block.heartRate < band[1] {
						total += block.watts
						count++
					}
				}
				return total / float64(max(count, 1)), count >= minBandBlocks
			},
		})
	}

	return append(statistics, anchorStatistic{
		name:  "best-20min",
		value: func(side *monthSide) (float64, bool) { return side.best, side.hasBest },
	})
}

// wattsAtHeartRate is the median of each block's level along the slope,
// moved to the reference rate: a median so one unpaired strap moves nothing.
func wattsAtHeartRate(blocks []anchorBlock, slope, referenceBPM float64) (float64, bool) {
	if len(blocks) == 0 {
		return 0, false
	}
	intercepts := make([]float64, 0, len(blocks))
	for _, block := range blocks {
		intercepts = append(intercepts, block.watts-slope*block.heartRate)
	}

	return quantile(intercepts, 0.5) + slope*referenceBPM, true
}

// overlap is the months holding both domains, oldest first.
func (m anchorMonths) overlap() []string {
	var keys []string
	for key := range m.indoor {
		if m.outdoor[key] != nil {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)

	return keys
}

// referenceHeartRate is the median heart rate over every block of the
// overlapping months, both sides pooled, so the matched rate is one each
// domain actually rode at.
func (m anchorMonths) referenceHeartRate() (float64, bool) {
	var rates []float64
	for _, key := range m.overlap() {
		for _, side := range []*monthSide{m.indoor[key], m.outdoor[key]} {
			for _, block := range side.blocks {
				rates = append(rates, block.heartRate)
			}
		}
	}
	if len(rates) == 0 {
		return 0, false
	}

	return quantile(rates, 0.5), true
}

// anchorVerdict says whether a statistic's month ratios settle a scale: at
// least minMonths of them, with an interquartile range no wider than
// maxSpread of their median.
func anchorVerdict(ratios []float64, minMonths int, maxSpread float64) string {
	if len(ratios) < minMonths {
		return fmt.Sprintf("too few months to anchor on (%d of %d)", len(ratios), minMonths)
	}
	median := quantile(ratios, 0.5)
	spread := (quantile(ratios, 0.75) - quantile(ratios, 0.25)) / median
	if spread > maxSpread {
		return fmt.Sprintf("unstable: month-to-month IQR %.0f %% of the median, above %.0f %%", 100*spread, 100*maxSpread)
	}

	return fmt.Sprintf("stable: a per-rider scale of %.2f would bring the estimate to the meter", 1/median)
}

// writeAnchor reports every statistic month by month and its verdict, then
// whether the hr-matched ratio moves with outdoor speed.
func (m anchorMonths) writeAnchor(b *strings.Builder, bicycle measure.Coefficients, slope float64, band [2]float64, bandOK bool, minMonths int, maxSpread float64) {
	keys := m.overlap()
	fmt.Fprintf(b, "\nsame-month anchoring, outdoor estimated at the saved bicycle (CdA %.3f, Crr %.4f)\n",
		bicycle.DragArea, bicycle.RollingResistance)
	reference, ok := m.referenceHeartRate()
	if !ok {
		fmt.Fprintf(b, "  overlap: %d months hold both domains with a block to compare; nothing to anchor on\n", len(keys))
		return
	}
	var indoorRides, outdoorRides, indoorBlocks, outdoorBlocks int
	for _, key := range keys {
		indoorRides += m.indoor[key].rides
		outdoorRides += m.outdoor[key].rides
		indoorBlocks += len(m.indoor[key].blocks)
		outdoorBlocks += len(m.outdoor[key].blocks)
	}
	fmt.Fprintf(b, "  overlap: %d months, %d indoor rides (%d blocks), %d outdoor rides (%d blocks)\n",
		len(keys), indoorRides, indoorBlocks, outdoorRides, outdoorBlocks)
	fmt.Fprintf(b, "  hr-matched at %.0f bpm along %.2f W/bpm", reference, slope)
	if bandOK {
		fmt.Fprintf(b, "; zone two %.0f-%.0f bpm", band[0], band[1])
	}
	fmt.Fprintln(b)

	for _, statistic := range anchorStatistics(slope, reference, band, bandOK) {
		fmt.Fprintf(b, "\n  %s: indoor measured W, outdoor estimated W, outdoor/indoor\n", statistic.name)
		var ratios []float64
		for _, key := range keys {
			indoor, indoorOK := statistic.value(m.indoor[key])
			outdoor, outdoorOK := statistic.value(m.outdoor[key])
			if !indoorOK || !outdoorOK || indoor <= 0 || outdoor <= 0 {
				fmt.Fprintf(b, "    %s %8s\n", key, "-")
				continue
			}
			ratios = append(ratios, outdoor/indoor)
			fmt.Fprintf(b, "    %s %8.1f %8.1f %6.2f\n", key, indoor, outdoor, outdoor/indoor)
		}
		if len(ratios) > 0 {
			fmt.Fprintf(b, "    median %.2f, q1 %.2f, q3 %.2f over %d months\n",
				quantile(ratios, 0.5), quantile(ratios, 0.25), quantile(ratios, 0.75), len(ratios))
		}
		fmt.Fprintf(b, "    verdict: %s\n", anchorVerdict(ratios, minMonths, maxSpread))
	}

	m.writeSpeedSplit(b, keys, slope, reference)
}

// writeSpeedSplit halves every outdoor block at the overlap's median block
// speed, ties going below, and reads the hr-matched ratio for each half. A
// plain scale error leaves both halves alike; a coefficient error or
// unmodelled wind opens a gap between them.
func (m anchorMonths) writeSpeedSplit(b *strings.Builder, keys []string, slope, reference float64) {
	var speeds []float64
	for _, key := range keys {
		for _, block := range m.outdoor[key].blocks {
			if block.speed > 0 {
				speeds = append(speeds, block.speed)
			}
		}
	}
	if len(speeds) == 0 {
		fmt.Fprintln(b, "\n  hr-matched by outdoor block speed: no outdoor block carried a speed")
		return
	}
	split := quantile(speeds, 0.5)
	var slow, fast []float64
	for _, key := range keys {
		indoor, ok := wattsAtHeartRate(m.indoor[key].blocks, slope, reference)
		if !ok || indoor <= 0 {
			continue
		}
		var below, above []anchorBlock
		for _, block := range m.outdoor[key].blocks {
			if block.speed <= 0 {
				continue
			}
			if block.speed <= split {
				below = append(below, block)
			} else {
				above = append(above, block)
			}
		}
		if watts, belowOK := wattsAtHeartRate(below, slope, reference); belowOK {
			slow = append(slow, watts/indoor)
		}
		if watts, aboveOK := wattsAtHeartRate(above, slope, reference); aboveOK {
			fast = append(fast, watts/indoor)
		}
	}
	if len(slow) == 0 || len(fast) == 0 {
		fmt.Fprintf(b, "\n  hr-matched by outdoor block speed: every block sits on one side of %.1f km/h\n", split*3.6)
		return
	}
	fmt.Fprintf(b, "\n  hr-matched by outdoor block speed, split at %.1f km/h: at or below %.2f over %d months, above %.2f over %d months\n",
		split*3.6, quantile(slow, 0.5), len(slow), quantile(fast, 0.5), len(fast))
}
