package activity

import (
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/trainingload"
	"github.com/stretchr/testify/assert"
)

func promptFixture() (rider.Profile, RideMetrics, trainingload.Day, []Analysis) {
	profile := rider.Profile{
		MaxHeartRateBPM:               rider.Value{Number: 190, Set: true},
		ThresholdHeartRateBPM:         rider.Value{Number: 170, Set: true},
		FunctionalThresholdPowerWatts: rider.Value{Number: 280, Set: true},
		RiderMassKG:                   rider.Value{Number: 72.5, Set: true},
	}
	metrics := RideMetrics{
		Load: trainingload.Metrics{
			Zones: trainingload.Zones{600, 1800, 900, 300, 60}, HasZones: true,
			TRIMP: 95, HasTRIMP: true,
			HeartRateTSS: 74, HasHeartRateTSS: true,
			HeartRateCoverage: 0.9, HasHeartRateCoverage: true,
			Power: trainingload.Power{NormalizedWatts: 231, IntensityFactor: 0.825, TSS: 78}, HasPower: true,
			PowerCoverage: 0.98, HasPowerCoverage: true,
		},
		Averages: RideAverages{
			HeartRateBPM: 142, MaxHeartRateBPM: 176, HasHeartRate: true, PowerWatts: 205, HasPower: true,
			CadenceRPM: 88, HasCadence: true, MaxSpeedKmh: 54.6, HasSpeed: true,
		},
		Decoupling: Decoupling{Percent: 3.4, Known: true},
		HeatDrift:  HeatDrift{HeartRateBPM: 131, TemperatureCelsius: 24, Samples: 900, Known: true},
		PowerBests: rider.PowerCurve{Watts: [rider.PowerCurvePoints]float64{820, 0, 390}, Held: [rider.PowerCurvePoints]bool{true, false, true}},
	}
	day := trainingload.Day{Date: time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC),
		TSSFitness: 62, TSSFatigue: 71, TSSForm: -9, TRIMPFitness: 70, TRIMPFatigue: 80, TRIMPForm: -10}
	earlier := []Analysis{{Text: "A steady endurance ride.\nKeep it easy."}}

	return profile, metrics, day, earlier
}

// The prompt is the whole of what leaves the host: figures, profile, load and
// earlier analyses, and nothing a fixture of those could not have produced.
func TestComposePromptCarriesTheFiguresAndNothingElse(t *testing.T) {
	t.Parallel()
	profile, metrics, day, earlier := promptFixture()

	assert.Equal(t, promptInstruction+`

Rider profile:
- maximum heart rate 190 bpm
- threshold heart rate 170 bpm
- functional threshold power 280 W
- rider mass 72.5 kg
- heart-rate zone bounds: 138, 153, 160 and 170 bpm

This ride:
- time in heart-rate zones: zone 1 10 min, zone 2 30 min, zone 3 15 min, zone 4 5 min, zone 5 1 min
- TRIMP 95
- heart-rate TSS 74
- normalized power 231 W, intensity factor 0.82, power TSS 78
- average heart rate 142 bpm, maximum 176 bpm
- average power 205 W
- average cadence 88 rpm
- maximum speed 55 km/h
- aerobic decoupling 3.4%
- endurance-band heart rate 131 bpm at 24 °C
- best power: 5 s 820 W, 1 min 390 W
- heart-rate sensor covered 90% of the ride
- power meter covered 98% of the ride

The rider's training load now:
- TSS scale: fitness 62, fatigue 71, form -9
- TRIMP scale: fitness 70, fatigue 80, form -10

What was said about this rider's earlier rides, newest first:
- A steady endurance ride. Keep it easy.`, composePrompt(&profile, &metrics, &day, earlier))
}

func TestComposePromptLeavesOutWhatIsAbsent(t *testing.T) {
	t.Parallel()

	assert.Equal(t, promptInstruction+"\n\nThis ride:\n- no power meter; estimated power while pedalling 180 W, pedalling 85% of the ride",
		composePrompt(&rider.Profile{}, &RideMetrics{
			Load:                       trainingload.Metrics{EstimatedPowerWatts: 180, HasEstimatedPower: true},
			EstimatedPedallingShare:    0.85,
			HasEstimatedPedallingShare: true,
		}, nil, nil))
}
