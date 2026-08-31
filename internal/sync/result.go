package sync

import (
	"github.com/nobbs/domestique/internal/route"
)

// Phase is one half of a synchronization. The halves are switched, triggered,
// recorded, and reported separately, because they fail for unrelated reasons and
// an operator has reason to want one without the other.
type Phase string

const (
	// PhaseSource reads the VeloPlanner library into stored state.
	PhaseSource Phase = "source"
	// PhaseTargets reconciles stored state onto the Wahoo targets.
	PhaseTargets Phase = "targets"
)

// Outcome is the terminal result of one attempted synchronization run.
type Outcome string

const (
	// OutcomeNotReady means the run had nothing it could safely do: a target
	// needs OAuth onboarding, or the settings a run needs are not entered yet.
	OutcomeNotReady Outcome = "not_ready"
	// OutcomeSucceeded means every target was reconciled successfully.
	OutcomeSucceeded Outcome = "succeeded"
	// OutcomeFailed means a safe failure stopped one or more target reconciliations.
	OutcomeFailed Outcome = "failed"
	// OutcomeBlocked means a deletion safety gate prevented destination mutation.
	OutcomeBlocked Outcome = "blocked"
	// OutcomeSkipped means another synchronization run was already active.
	OutcomeSkipped Outcome = "skipped"
)

// FailureCategory is a stable, safe-to-display reason for a failed or blocked
// synchronization run. It never contains provider response text.
type FailureCategory string

const (
	// FailureNone means the run completed without a failure category.
	FailureNone FailureCategory = ""
	// FailureState means encrypted state could not be read or updated safely.
	FailureState FailureCategory = "state"
	// FailureSource means the VeloPlanner inventory was not fully valid.
	FailureSource FailureCategory = "source"
	// FailureAuthorization means one Wahoo target needs interactive OAuth again.
	FailureAuthorization FailureCategory = "authorization"
	// FailureDestination means a Wahoo route operation did not complete.
	FailureDestination FailureCategory = "destination"
	// FailureCourse means a FIT course could not be encoded safely.
	FailureCourse FailureCategory = "course"
	// FailureEmptySource means a previously populated library became empty while
	// the explicit empty-source deletion acknowledgement remains disabled.
	FailureEmptySource FailureCategory = "empty_source"
	// FailureDeletionLimit means a target would delete more than the configured
	// maximum owned routes in one automatic run.
	FailureDeletionLimit FailureCategory = "deletion_limit"
)

// TargetResult is one target's own share of a reconciliation. A run is recorded
// once, so its outcome is the worst across the slots, which is the wrong answer
// about one Wahoo account: a run that wrote one and not the other is a partial
// failure, and both halves are worth keeping.
type TargetResult struct {
	// ID is the configured target slot. It is never a Wahoo user identifier.
	ID      string
	Outcome Outcome
	Failure FailureCategory
}

// SourceResult is one configured source's own share of a source-phase read. One
// source's failure must not read as a claim about another, and naming which is
// which is what lets an operator tell them apart.
type SourceResult struct {
	Provider route.Provider
	Outcome  Outcome
	Failure  FailureCategory
	// StageCount is how many stages this source contributed when it was read
	// successfully. It stays zero for a source that failed or was blocked.
	StageCount int
}

// Result contains aggregate, non-sensitive counts for one synchronization run.
type Result struct {
	// Phase names the half of a synchronization this result describes. The
	// counts a phase does not produce stay zero.
	Phase   Phase
	Outcome Outcome
	Failure FailureCategory
	// Targets carries each slot's own outcome, in configured order. Only the
	// target phase produces it, and only for the slots it actually attempted.
	Targets []TargetResult
	// Sources carries each configured source's own outcome, in configured
	// order. Only the source phase produces it.
	Sources []SourceResult
	// SourceStored reports that this pass refreshed the stored inventory, which
	// is what makes reading the ground under it worth doing again.
	SourceStored bool
	SourceStages int
	Created      int
	Updated      int
	Deleted      int
}

// AnySourceStored reports whether any configured source stored a fresh
// inventory this pass; always false for a target or clear result, which leaves Sources empty.
func (r *Result) AnySourceStored() bool {
	for _, source := range r.Sources {
		if source.Outcome == OutcomeSucceeded {
			return true
		}
	}

	return false
}
