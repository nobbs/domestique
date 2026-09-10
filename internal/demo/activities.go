package demo

import (
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/nobbs/domestique/internal/activity"
	"github.com/nobbs/domestique/internal/measure"
	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/rider"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/wahoo"
)

// Ride is one synthetic recorded activity: the listing an account would carry,
// its totals, and the samples its FIT file would have held.
type Ride struct {
	Listing activity.Listing
	Summary activity.Summary
	FIT     activity.FIT
}

// carried is which sensors a ride's recorder had fitted.
type carried uint8

const (
	carriesHeartRate carried = 1 << iota
	carriesCadence
	carriesPower
	carriesTemperature
)

func (c carried) has(sensor carried) bool {
	return c&sensor != 0
}

// Wahoo's workout_type_location_id, which is all the served view says about
// where a ride happened.
const (
	locationOutdoors = 0
	locationIndoors  = 1
)

const (
	// barometricWarmUpRecords is how many samples one ride's altimeter takes to
	// settle: before them it has a position and no height.
	barometricWarmUpRecords = 90

	// sampleIntervalSeconds is how often one of these fixtures records: a real
	// head unit's own rough cadence, and comfortably under
	// measure.DefaultMaxGap so no ordinary stretch of riding reads as a
	// recording gap. A stage's geometry is spaced by distance, which a slow
	// climb would otherwise turn into a time gap between samples wide enough
	// to be read as one.
	sampleIntervalSeconds = 5.0

	// riddenSlowerThanPredicted stretches the forward model's timing into a
	// recorded one, so a demo ride is not its own prediction to the second.
	riddenSlowerThanPredicted = 1.04

	// stoppedFraction is how much longer a ride's elapsed time runs than its
	// moving time.
	stoppedFraction = 0.06

	// effortCycles is how many times the rider's effort rises and falls over a
	// whole ride, before the ground is taken into account.
	effortCycles = 2.5
)

// rideSpec is one synthetic ride before it becomes samples. A spec naming no
// stage was ridden on a trainer: every sensor, and no ground at all.
type rideSpec struct {
	// provider is which upstream listed this ride. Empty means Wahoo.
	provider       string
	workoutID      int64
	routeID        int64
	stageOrder     int
	typeID         int
	locationID     int
	daysAgo        int
	startHour      int
	altitudeFrom   int
	trainerMinutes int
	carries        carried
}

// rideSpecs is the recorded history: every sensor, an altimeter that needed
// warming up, no meter for the estimate to stand in for, and no position.
func rideSpecs() []rideSpec {
	return []rideSpec{
		{
			workoutID: 90_101, routeID: 4102, stageOrder: 1,
			typeID: wahoo.WorkoutTypeBikingRoad, locationID: locationOutdoors,
			daysAgo: 1, startHour: 8,
			carries: carriesHeartRate | carriesCadence | carriesPower | carriesTemperature,
		},
		{
			workoutID: 90_102, routeID: 4101, stageOrder: 1,
			typeID: wahoo.WorkoutTypeBikingRoad, locationID: locationOutdoors,
			daysAgo: 4, startHour: 7, altitudeFrom: barometricWarmUpRecords,
			carries: carriesHeartRate | carriesCadence | carriesPower | carriesTemperature,
		},
		{
			workoutID: 90_103, routeID: 4101, stageOrder: 2,
			typeID: wahoo.WorkoutTypeBikingMountain, locationID: locationOutdoors,
			daysAgo: 9, startHour: 14,
			carries: carriesHeartRate | carriesCadence | carriesTemperature,
		},
		{
			workoutID: 90_104,
			typeID:    wahoo.WorkoutTypeBikingIndoorTrainer, locationID: locationIndoors,
			daysAgo: 2, startHour: 19, trainerMinutes: 50,
			carries: carriesHeartRate | carriesCadence | carriesPower,
		},
		{
			workoutID: 90_105,
			typeID:    wahoo.WorkoutTypeBikingIndoorVirtual, locationID: locationIndoors,
			daysAgo: 6, startHour: 18, trainerMinutes: 40,
			carries:  carriesHeartRate | carriesCadence | carriesPower,
			provider: activity.ProviderZwift,
		},
	}
}

// Profile is the demo rider's own parameters. Without them no ride has a
// training load to show; they are a plausible profile, not anybody's real one.
func Profile() rider.Profile {
	return rider.Profile{
		MaxHeartRateBPM:               rider.Set(188),
		RestingHeartRateBPM:           rider.Set(48),
		ThresholdHeartRateBPM:         rider.Set(168),
		FunctionalThresholdPowerWatts: rider.Set(265),
		RiderMassKG:                   rider.Set(74),
		BikeMassKG:                    rider.Set(9),
	}
}

// Rides builds the synthetic recorded activities, dated against now so a demo
// always shows a fortnight that has just been ridden.
func Rides(now time.Time) ([]Ride, error) {
	stages, err := Routes()
	if err != nil {
		return nil, err
	}

	specs := rideSpecs()
	rides := make([]Ride, 0, len(specs))
	for index := range specs {
		spec := &specs[index]
		ride, buildErr := spec.ride(stages, now)
		if buildErr != nil {
			return nil, buildErr
		}
		rides = append(rides, ride)
	}

	return rides, nil
}

// ride assembles one spec into the listing, totals and samples a poll would
// have stored.
func (s *rideSpec) ride(stages []route.Route, now time.Time) (Ride, error) {
	start := s.startedAt(now)
	records, err := s.records(stages, start)
	if err != nil {
		return Ride{}, err
	}
	summary, err := s.summary(records)
	if err != nil {
		return Ride{}, err
	}

	return Ride{
		Listing: activity.Listing{
			Starts: start, ID: s.workoutID, TypeID: s.typeID, LocationID: s.locationID,
			Provider: s.provider,
		},
		Summary: summary,
		FIT: activity.FIT{
			RecordingDevice: "Demo Head Unit",
			Records:         records,
			Session: activity.Session{
				TimerSeconds:   activity.Reading{Value: summary.MovingSeconds, Known: true},
				ElapsedSeconds: activity.Reading{Value: summary.ElapsedSeconds, Known: true},
				DistanceMetres: activity.Reading{Value: summary.DistanceMetres, Known: true},
				AscentMetres:   activity.Reading{Value: summary.AscentMetres, Known: true},
				MaxSpeedKmh:    activity.Reading{Value: peakSpeedKmh(records), Known: true},
			},
		},
	}, nil
}

// peakSpeedKmh is the fastest stretch between two of the synthetic records,
// standing in for the maximum a head unit would have declared.
func peakSpeedKmh(records []activity.Record) float64 {
	peak := 0.0
	for index := 1; index < len(records); index++ {
		seconds := records[index].Time.Sub(records[index-1].Time).Seconds()
		if seconds <= 0 {
			continue
		}
		if kmh := (records[index].DistanceMetres - records[index-1].DistanceMetres) / seconds * 3.6; kmh > peak {
			peak = kmh
		}
	}

	return peak
}

// startedAt is when the ride set off: a whole hour of a past day, so a fixture
// reads as a ride rather than as whenever the demo was seeded.
func (s *rideSpec) startedAt(now time.Time) time.Time {
	day := now.UTC().AddDate(0, 0, -s.daysAgo)

	return time.Date(day.Year(), day.Month(), day.Day(), s.startHour, 0, 0, 0, time.UTC)
}

// records are the ride's samples, timed every sampleIntervalSeconds of the
// stage's own predicted elapsed time rather than one per point of its
// geometry: the geometry is spaced by distance, and a slow climb would
// otherwise leave samples far enough apart in time to be read as a recording
// gap, though nothing was ever missing from them.
func (s *rideSpec) records(stages []route.Route, start time.Time) ([]activity.Record, error) {
	if s.routeID == 0 {
		return s.trainerRecords(start), nil
	}
	stage, found := stageFor(stages, s.routeID, s.stageOrder)
	if !found {
		return nil, fmt.Errorf("demo: ride %d follows stage %d/%d, which the library no longer holds",
			s.workoutID, s.routeID, s.stageOrder)
	}
	geometry := stage.Geometry()
	prediction, ok := ridemodel.Predict(geometry, demoCoefficients())
	if !ok {
		return nil, fmt.Errorf("demo: ride %d follows stage %d/%d, which carries no complete profile to time it by",
			s.workoutID, s.routeID, s.stageOrder)
	}

	count := int(math.Ceil(prediction.MovingSeconds/sampleIntervalSeconds)) + 1
	records := make([]activity.Record, count)
	at, distance := 0, 0.0
	for sampleIndex := range records {
		// The last sample always lands exactly on the stage's own end, the way
		// a ride actually finishes where the route does, rather than short of
		// it by however much sampleIntervalSeconds did not divide evenly.
		targetSeconds := float64(sampleIndex) * sampleIntervalSeconds
		if sampleIndex == len(records)-1 {
			targetSeconds = prediction.MovingSeconds
		}
		// The geometry point this moment of riding falls on, distance and time
		// both only ever moving forward: several time steps in a row can land
		// on the same slow point, the way a real recorder would too.
		for at < len(geometry)-1 && prediction.CumulativeSeconds[at] < targetSeconds {
			at++
			distance += measure.HaversineMetres(geometry[at-1].Coordinate(), geometry[at].Coordinate())
		}
		point := &geometry[at]
		fraction := float64(at) / float64(len(geometry)-1)
		elapsed := targetSeconds * riddenSlowerThanPredicted
		record := activity.Record{
			Time:           start.Add(time.Duration(elapsed * float64(time.Second))),
			Latitude:       point.Latitude,
			Longitude:      point.Longitude,
			DistanceMetres: distance,
			HasDistance:    true,
			HasPosition:    true,
		}
		if point.Elevation != nil && at >= s.altitudeFrom {
			record.AltitudeMetres, record.HasAltitude = *point.Elevation, true
		}
		s.fitSensors(&record, effortAt(fraction, gradientAt(geometry, at)), fraction)
		records[sampleIndex] = record
	}

	return records, nil
}

// trainerRecords are a ride with no ground: a virtual distance that accumulates
// from the effort, and no position for a map to draw.
func (s *rideSpec) trainerRecords(start time.Time) []activity.Record {
	count := int(float64(s.trainerMinutes)*60/sampleIntervalSeconds) + 1
	records := make([]activity.Record, count)
	distance := 0.0
	for index := range records {
		fraction := float64(index) / float64(count-1)
		effort := effortAt(fraction, 0)
		if index > 0 {
			// Kilometres per hour into metres per sample.
			distance += (22 + 14*effort) / 3.6 * sampleIntervalSeconds
		}
		record := activity.Record{
			Time:           start.Add(time.Duration(float64(index) * sampleIntervalSeconds * float64(time.Second))),
			DistanceMetres: distance,
			HasDistance:    true,
		}
		s.fitSensors(&record, effort, fraction)
		records[index] = record
	}

	return records
}

// fitSensors hangs on one sample the readings each fitted sensor would have
// taken. A sensor the ride did not carry leaves its field absent, not nought.
func (s *rideSpec) fitSensors(record *activity.Record, effort, fraction float64) {
	if s.carries.has(carriesHeartRate) {
		record.HeartRateBPM, record.HasHeartRate = 104+68*effort, true
	}
	if s.carries.has(carriesCadence) {
		record.CadenceRPM, record.HasCadence = 76+16*math.Sin(6*math.Pi*fraction), true
	}
	if s.carries.has(carriesPower) {
		record.PowerWatts, record.HasPower = 115+205*effort, true
	}
	if s.carries.has(carriesTemperature) {
		record.TemperatureCelsius, record.HasTemperatureCelsius = 12+7*math.Sin(math.Pi*fraction), true
	}
}

// effortAt is how hard the rider is working at one point of the ride: a long
// wave over the whole of it with the ground under that point laid on top.
func effortAt(fraction, gradient float64) float64 {
	effort := 0.45 + 0.2*math.Sin(2*math.Pi*effortCycles*fraction) + 6*gradient

	return min(max(effort, 0), 1)
}

// gradientAt is the rise over the ground between one point and the one before.
func gradientAt(geometry []route.Point, index int) float64 {
	if index == 0 || geometry[index].Elevation == nil || geometry[index-1].Elevation == nil {
		return 0
	}
	span := measure.HaversineMetres(geometry[index-1].Coordinate(), geometry[index].Coordinate())
	if span <= 0 {
		return 0
	}

	return (*geometry[index].Elevation - *geometry[index-1].Elevation) / span
}

// summary is what the account would list the ride as. Raw stands in for the
// provider's own summary document, which nothing in a demo ever decodes.
func (s *rideSpec) summary(records []activity.Record) (activity.Summary, error) {
	raw, err := json.Marshal(map[string]any{"id": s.workoutID, "source": "demo"})
	if err != nil {
		return activity.Summary{}, fmt.Errorf("demo: encoding the summary of ride %d: %w", s.workoutID, err)
	}
	last := &records[len(records)-1]
	moving := last.Time.Sub(records[0].Time).Seconds()

	return activity.Summary{
		Raw:            raw,
		DistanceMetres: last.DistanceMetres,
		MovingSeconds:  moving,
		ElapsedSeconds: moving * (1 + stoppedFraction),
		AscentMetres:   ascentOf(records),
	}, nil
}

// ascentOf is every rise between consecutive samples that carried a height, so
// a warm-up gap costs the climb under it rather than reading as a step.
func ascentOf(records []activity.Record) float64 {
	ascent, previous := 0.0, -1
	for index := range records {
		if !records[index].HasAltitude {
			continue
		}
		if previous >= 0 {
			ascent += max(0, records[index].AltitudeMetres-records[previous].AltitudeMetres)
		}
		previous = index
	}

	return ascent
}

// stageFor finds the library stage a ride followed.
func stageFor(stages []route.Route, routeID int64, stageOrder int) (*route.Route, bool) {
	for index := range stages {
		key := stages[index].Key()
		if key.SourceRouteID() == routeID && key.StageOrder() == stageOrder {
			return &stages[index], true
		}
	}

	return nil, false
}
