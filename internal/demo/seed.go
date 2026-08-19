package demo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

// State is what seeding needs from the state database. It is declared here, in
// the package that consumes it, so a test can seed a fake and the demo does not
// drag the SQLite adapter into anything that only wants the fixtures.
type State interface {
	StoreTrustedInventory(ctx context.Context, stages []route.Stage) error
	StoreStageSurface(ctx context.Context, routeID int64, stageOrder int, contentHash string, ranges []byte, matchedMetres float64) error
	EnsureTargets(ctx context.Context, targetIDs []string) error
	AuthorizeTarget(ctx context.Context, targetID, wahooUserID, refreshToken string) error
	UpsertTargetStage(ctx context.Context, targetID string, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error
	RecordSyncRun(ctx context.Context, phase string, startedAt, finishedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error
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
	if storeErr := state.StoreTrustedInventory(ctx, stages); storeErr != nil {
		return fmt.Errorf("demo: storing inventory: %w", storeErr)
	}

	classifications, err := Classifications(stages)
	if err != nil {
		return err
	}
	for _, classification := range classifications {
		if err := state.StoreStageSurface(
			ctx,
			classification.RouteID,
			classification.StageOrder,
			classification.ContentHash,
			classification.Ranges,
			classification.MatchedMetres,
		); err != nil {
			return fmt.Errorf("demo: storing surface for %d/%d: %w",
				classification.RouteID, classification.StageOrder, err)
		}
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

	if err := state.RecordSyncRun(
		ctx, "source", now.Add(-9*time.Minute), now.Add(-8*time.Minute),
		"succeeded", "", len(stages), 2, 1, 0,
	); err != nil {
		return fmt.Errorf("demo: recording source run: %w", err)
	}
	if err := state.RecordSyncRun(
		ctx, "targets", now.Add(-8*time.Minute), now.Add(-7*time.Minute),
		"succeeded", "", len(stages), 2, 1, 0,
	); err != nil {
		return fmt.Errorf("demo: recording targets run: %w", err)
	}
	// Both halves left on, because a demo of a service that synchronises should
	// show the switches the way an operator running it would see them. It still
	// cannot reach a provider: that is the environment's job, not the fixture's.
	if err := state.SetSyncSchedule(ctx, true, true); err != nil {
		return fmt.Errorf("demo: setting schedule: %w", err)
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
			ctx, slot.ID, key.RouteID(), key.StageOrder(), revision,
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
			ctx, slot.ID, orphanRouteID, 1, Revision(orphanRouteID, 1),
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

// wahooRouteID is the identifier a target pretends Wahoo gave a written stage.
func wahooRouteID(routeID int64, stageOrder int) int64 {
	return 900_000_000 + routeID*100 + int64(stageOrder)
}
