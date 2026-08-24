package demo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/ridemodel"
	"github.com/nobbs/domestique/internal/route"
	"github.com/nobbs/domestique/internal/surface"
)

// State is what seeding needs from the state database. It is declared here, in
// the package that consumes it, so a test can seed a fake and the demo does not
// drag the SQLite adapter into anything that only wants the fixtures.
type State interface {
	StoreTrustedInventory(ctx context.Context, provider route.Provider, stages []route.Stage) error
	StoreStageSurface(ctx context.Context, provider route.Provider, routeID int64, stageOrder int, contentHash, indexGeneration string, ranges []byte, matchedMetres float64) error
	StoreStageDuration(ctx context.Context, provider route.Provider, routeID int64, stageOrder int, contentHash, surfaceGeneration, coefficientFingerprint string, movingSeconds *float64, cumulativeSeconds []byte) error
	EnsureTargets(ctx context.Context, targetIDs []string) error
	AuthorizeTarget(ctx context.Context, targetID, wahooUserID, refreshToken string) error
	UpsertTargetStage(ctx context.Context, targetID string, provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error
	RecordSyncRun(ctx context.Context, phase string, startedAt, finishedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) (string, error)
	RecordTargetRun(ctx context.Context, targetID string, finishedAt time.Time, outcome, detail string) error
	SetSyncSchedule(ctx context.Context, source, targets bool) error
}

// SlotState is the state a seeded Wahoo slot is left in. Between them the three
// cover every word the status endpoint can report for a target: an unauthorized
// slot has not been onboarded, a failed one has and its last write did not
// finish, and a current one holds the whole library.
type SlotState string

const (
	// SlotCurrent holds every stored stage at the revision the library holds.
	SlotCurrent SlotState = "current"
	// SlotFailed is onboarded, is missing stages, is carrying one the library has
	// dropped, and its last write failed.
	SlotFailed SlotState = "failed"
	// SlotUnauthorized has never completed its browser onboarding.
	SlotUnauthorized SlotState = "unauthorized"
)

// Slot is one configured Wahoo target and the state to seed it in.
type Slot struct {
	ID    string
	State SlotState
}

// orphanRouteID is a stage a target still carries that the library no longer
// has. It is deliberately not one of the generated route IDs.
const orphanRouteID = 4199

// demoIndexGeneration marks every seeded classification as coming from a map
// build that no real index will ever carry. A demo that is later pointed at
// configured regions therefore reclassifies its fixtures rather than trusting
// them, which is what any stage classified by a retired index should do.
const demoIndexGeneration = "demo"

// demoCoefficientFingerprint marks every seeded prediction as coming from a
// coefficient file that no real deployment loads, on the same terms as
// demoIndexGeneration above.
const demoCoefficientFingerprint = "demo-coefficients"

// demoCoefficients are a physically plausible hybrid profile for a mid-weight
// rider and a fairly upright road position — an unremarkable ride, not this
// package's own claim about any real one. Mass, power, CdA and the route
// coefficients are near the figures #239's real benchmark actually recorded;
// the rest are reasonable stand-ins that Coefficients' own load-time
// validation would accept from a real coefficient file.
//
// A function rather than a package variable, on the same terms as specs() in
// library.go: nothing here may become mutable package-level state, and a
// fresh map on every call is cheap next to the geometry this package already
// rebuilds on every Seed.
func demoCoefficients() ridemodel.Coefficients {
	return ridemodel.Coefficients{
		CalibrationCutoff: "2025-08-01",
		MassKG:            90,
		PowerWatts:        180,
		CdAM2:             0.45,
		SecondsPerKM:      145.0,
		SecondsPerAscentM: 3.2,
		EvaluatedRides:    42,
		BiasPercent:       -1.2,
		MAEPercent:        6.8,
		P90Percent:        14.1,
		CrrBySurface: map[surface.Kind]float64{
			surface.KindAsphalt:   0.012,
			surface.KindPaving:    0.012,
			surface.KindCompacted: 0.012,
			surface.KindGravel:    0.012,
			surface.KindGround:    0.012,
		},
	}
}

// Seed writes the synthetic library, its surfaces, and one state per slot.
//
// The clock is a parameter because everything else here is a function of the
// constants in this package, and a run that stamped itself with time.Now would
// be the one thing in the fixture that could not be asserted on. Pass a fixed
// instant in a test and the wall clock in a development environment, where a
// last run dated two years ago would read as a broken service.
//
// Nothing here contacts a provider: the whole point is a library that exists
// without one.
func Seed(ctx context.Context, state State, slots []Slot, now time.Time) error {
	if len(slots) == 0 {
		return errors.New("demo: at least one target slot is required")
	}

	stages, err := Stages()
	if err != nil {
		return err
	}
	// One call per source, each replacing only that source's own slice of the
	// inventory — the same isolation a real read gives each configured source,
	// so the fixture cannot hide a bug that only shows up once there is more
	// than one.
	for _, group := range stagesByProvider(stages) {
		if storeErr := state.StoreTrustedInventory(ctx, group.provider, group.stages); storeErr != nil {
			return fmt.Errorf("demo: storing inventory for %s: %w", group.provider, storeErr)
		}
	}

	classifications, err := Classifications(stages)
	if err != nil {
		return err
	}
	for _, classification := range classifications {
		if err := state.StoreStageSurface(
			ctx,
			classification.Provider,
			classification.RouteID,
			classification.StageOrder,
			classification.ContentHash,
			demoIndexGeneration,
			classification.Ranges,
			classification.MatchedMetres,
		); err != nil {
			return fmt.Errorf("demo: storing surface for %d/%d: %w",
				classification.RouteID, classification.StageOrder, err)
		}
	}

	if err := seedDurations(ctx, state, stages); err != nil {
		return err
	}

	targetIDs := make([]string, 0, len(slots))
	for _, slot := range slots {
		targetIDs = append(targetIDs, slot.ID)
	}
	if err := state.EnsureTargets(ctx, targetIDs); err != nil {
		return fmt.Errorf("demo: ensuring targets: %w", err)
	}
	for _, slot := range slots {
		if err := seedSlot(ctx, state, slot, stages, now); err != nil {
			return err
		}
	}

	if err := seedRuns(ctx, state, len(stages), now); err != nil {
		return err
	}
	// Both halves left on, because a demo of a service that synchronises should
	// show the switches the way an operator running it would see them. It still
	// cannot reach a provider: that is the environment's job, not the fixture's.
	if err := state.SetSyncSchedule(ctx, true, true); err != nil {
		return fmt.Errorf("demo: setting schedule: %w", err)
	}

	return nil
}

// seedDurations predicts and stores one moving-time series per stage, computed
// by internal/ridemodel.Predict over the stage's own geometry and its surface
// classification — the same forward model and the same per-point surface
// classes a real enrichment pass would use, run here once against fixed
// coefficients instead of a loaded coefficient file.
//
// Predict returns false for a stage with no usable elevation, which this
// package's own elevation-less and partially-elevated stages both are: they
// are stored as nil, nil, exactly what Predict itself would have this package
// cache for a real stage it could not answer, rather than a fabricated time
// that would misrepresent what the model can and cannot do.
func seedDurations(ctx context.Context, state State, stages []route.Stage) error {
	kindsByStage := stageSurfaceKinds(stages)
	for index := range stages {
		stage := &stages[index]
		key := stage.Key()
		result, ok := ridemodel.Predict(stage.Geometry(), kindsByStage[index], demoCoefficients())

		var movingSeconds *float64
		var cumulativeSeconds []byte
		if ok {
			movingSeconds = &result.MovingSeconds
			encoded, encodeErr := json.Marshal(result.CumulativeSeconds)
			if encodeErr != nil {
				return fmt.Errorf("demo: encoding cumulative series for %d/%d: %w",
					key.RouteID(), key.StageOrder(), encodeErr)
			}
			cumulativeSeconds = encoded
		}

		if err := state.StoreStageDuration(
			ctx, key.Provider(), key.RouteID(), key.StageOrder(),
			stage.ContentHash(), demoIndexGeneration, demoCoefficientFingerprint,
			movingSeconds, cumulativeSeconds,
		); err != nil {
			return fmt.Errorf("demo: storing duration for %d/%d: %w", key.RouteID(), key.StageOrder(), err)
		}
	}

	return nil
}

// seedSlot leaves one target in the state its slot asks for.
func seedSlot(ctx context.Context, state State, slot Slot, stages []route.Stage, now time.Time) error {
	if slot.State == SlotUnauthorized {
		// Ensuring the target was the whole of it: an un-onboarded slot has no
		// token, no applied stages, and no run of its own.
		return nil
	}

	if err := state.AuthorizeTarget(
		ctx, slot.ID, "demo-wahoo-"+slot.ID, "demo-refresh-token-"+slot.ID,
	); err != nil {
		return fmt.Errorf("demo: authorizing %s: %w", slot.ID, err)
	}

	for index := range stages {
		stage := &stages[index]
		key := stage.Key()
		revision := Revision(key.RouteID(), key.StageOrder())
		// One stage left a revision behind, so the slot reads as owing work
		// rather than as a wall of identical rows.
		if slot.State == SlotFailed && index == len(stages)-1 {
			revision = EarlierRevision(key.RouteID(), key.StageOrder())
		}
		if err := state.UpsertTargetStage(
			ctx, slot.ID, key.Provider(), key.RouteID(), key.StageOrder(), revision,
			"demo-encoded-"+revision, wahooRouteID(key.RouteID(), key.StageOrder()),
		); err != nil {
			return fmt.Errorf("demo: applying stage to %s: %w", slot.ID, err)
		}
	}

	outcome, detail := "succeeded", ""
	if slot.State == SlotFailed {
		outcome, detail = "failed", "destination"
		// A stage the library has dropped and this slot still holds, so the
		// outstanding count covers a deletion as well as a write.
		if err := state.UpsertTargetStage(
			ctx, slot.ID, route.ProviderVeloPlanner, orphanRouteID, 1, Revision(orphanRouteID, 1),
			"demo-encoded-orphan", wahooRouteID(orphanRouteID, 1),
		); err != nil {
			return fmt.Errorf("demo: applying orphan to %s: %w", slot.ID, err)
		}
	}
	if err := state.RecordTargetRun(
		ctx, slot.ID, now.Add(-7*time.Minute), outcome, detail,
	); err != nil {
		return fmt.Errorf("demo: recording run for %s: %w", slot.ID, err)
	}

	return nil
}

// providerStages is one source's own slice of the synthetic inventory.
type providerStages struct {
	provider route.Provider
	stages   []route.Stage
}

// stagesByProvider splits the library by source, in the order each source's
// first stage appears. StoreTrustedInventory now isolates one source's write
// from another's, so seeding has to call it once per source rather than once
// for stages of more than one.
func stagesByProvider(stages []route.Stage) []providerStages {
	var groups []providerStages
	index := make(map[route.Provider]int, 2)
	for _, stage := range stages {
		provider := stage.Key().Provider()
		if position, seen := index[provider]; seen {
			groups[position].stages = append(groups[position].stages, stage)

			continue
		}
		index[provider] = len(groups)
		groups = append(groups, providerStages{provider: provider, stages: []route.Stage{stage}})
	}

	return groups
}

// wahooRouteID is the identifier a target pretends Wahoo gave a written stage.
func wahooRouteID(routeID int64, stageOrder int) int64 {
	return 900_000_000 + routeID*100 + int64(stageOrder)
}

// seedRuns records the demo's run history in order, so the newest row is the
// most recent run here as it would be on a service that had been running.
//
// The last two runs are the pair a first synchronization leaves behind; the
// rest are there so the history is a history rather than one line, and the
// blocked one is how a deletion gate looks to a reader without a demo that can
// delete anything.
func seedRuns(ctx context.Context, state State, stages int, now time.Time) error {
	pastRuns := []struct {
		phase   string
		outcome string
		failure string
		minutes int
		created int
		updated int
		deleted int
	}{
		{phase: "source", outcome: "succeeded", minutes: 189},
		{phase: "targets", outcome: "succeeded", minutes: 188, updated: 3},
		{phase: "source", outcome: "succeeded", minutes: 129},
		{phase: "targets", outcome: "blocked", failure: "deletion_limit", minutes: 128},
		{phase: "source", outcome: "succeeded", minutes: 69},
		{phase: "targets", outcome: "failed", failure: "destination", minutes: 68},
		{phase: "source", outcome: "succeeded", minutes: 9, created: 2, updated: 1},
		{phase: "targets", outcome: "succeeded", minutes: 8, created: 2, updated: 1},
	}

	for _, run := range pastRuns {
		finishedAt := now.Add(-time.Duration(run.minutes) * time.Minute)
		if _, err := state.RecordSyncRun(
			ctx, run.phase, finishedAt.Add(-time.Minute), finishedAt,
			run.outcome, run.failure, stages, run.created, run.updated, run.deleted,
		); err != nil {
			return fmt.Errorf("demo: recording %s run: %w", run.phase, err)
		}
	}

	return nil
}
