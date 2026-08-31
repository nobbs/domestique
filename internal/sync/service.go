// Package sync owns safe reconciliation of source stages to Wahoo targets.
package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

const authorizedState = "authorized"

// maxDeletionsPerTarget bounds how many owned routes one reconciliation may
// remove from one target. Not a setting: it exists to stop a runaway deletion.
const maxDeletionsPerTarget = 5

// Interval is how often a scheduled reconciliation runs. The rate limits, the
// staleness bound and the digest window are all sized against an hour.
const Interval = time.Hour

const encoderContentVersion = "fit-v4-elevation-profile"

// Options configures safety rules for a synchronizer. It contains no secrets
// and is intentionally independent of the configuration packages.
type Options struct {
	// AllowEmptySourceDeletion reports whether a trusted but empty source may delete
	// the final owned destination routes. Asked once per source, as it is read.
	AllowEmptySourceDeletion func() bool

	// Sources and TargetIDs are read as a run starts, not at composition: both are
	// settings an operator edits while the service runs. Empty means unconfigured.
	Sources   func() ([]Source, error)
	TargetIDs func() []string
}

// Service reconciles a complete source inventory to each configured target.
// It has no dependency on HTTP, static configuration, or concrete adapters.
type Service struct {
	state                    State
	sources                  func() ([]Source, error)
	processor                Processor
	encoder                  Encoder
	target                   Target
	annotator                Annotator
	predictor                Predictor
	allowEmptySourceDeletion func() bool
	targetIDs                func() []string
}

// New creates a synchronizer with explicit consumer-owned dependencies. All are
// required except the annotator and predictor; nil leaves stages unenriched.
func New(
	options *Options,
	state State,
	processor Processor,
	encoder Encoder,
	target Target,
	annotator Annotator,
	predictor Predictor,
) (*Service, error) {
	if options == nil || state == nil || processor == nil || encoder == nil || target == nil {
		return nil, errors.New("sync options and dependencies are required")
	}
	if options.Sources == nil || options.TargetIDs == nil {
		return nil, errors.New("sync requires its sources and targets to be readable")
	}
	if options.AllowEmptySourceDeletion == nil {
		return nil, errors.New("sync requires an empty-source deletion gate")
	}

	return &Service{
		state:                    state,
		sources:                  options.Sources,
		processor:                processor,
		encoder:                  encoder,
		target:                   target,
		annotator:                annotator,
		predictor:                predictor,
		targetIDs:                options.TargetIDs,
		allowEmptySourceDeletion: options.AllowEmptySourceDeletion,
	}, nil
}

// RunSource reads every configured source into stored state, one at a time,
// contacting no target. Only a source read successfully has its stages replaced;
// a failed one keeps its last-known stages, and the empty-source gate is
// evaluated per source against that source's own prior count.
func (s *Service) RunSource(ctx context.Context) Result {
	sources, err := s.sources()
	if err != nil || len(sources) == 0 {
		return Result{Phase: PhaseSource, Outcome: OutcomeNotReady}
	}

	result := Result{Phase: PhaseSource, Sources: make([]SourceResult, 0, len(sources))}
	for _, source := range sources {
		provider := source.Provider()
		outcome, failure, stageCount := s.runOneSource(ctx, source, provider)
		result.Sources = append(result.Sources, SourceResult{
			Provider:   provider,
			Outcome:    outcome,
			Failure:    failure,
			StageCount: stageCount,
		})
		result.SourceStages += stageCount
		if failure == FailureNone {
			continue
		}
		// An empty-source block is a guard doing its job, not a fault: it only
		// overrides an outcome no source has failed yet.
		if failure == FailureEmptySource && (result.Outcome == "" || result.Outcome == OutcomeBlocked) {
			result.Outcome = OutcomeBlocked
		} else {
			result.Outcome = OutcomeFailed
		}
		// A real failure always replaces a recorded empty-source block: the guard
		// category describes no fault.
		if result.Failure == FailureNone || (result.Failure == FailureEmptySource && failure != FailureEmptySource) {
			result.Failure = failure
		}
	}
	if result.Outcome == "" {
		result.Outcome = OutcomeSucceeded
	}

	return result
}

// RunSourceProvider reads exactly one configured source library, leaving every
// other source's stored stages untouched. A provider this service is not
// configured for is not ready rather than a fault: absent is not broken.
func (s *Service) RunSourceProvider(ctx context.Context, provider route.Provider) Result {
	sources, err := s.sources()
	if err != nil {
		return Result{Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureState}
	}
	for _, source := range sources {
		if source.Provider() != provider {
			continue
		}
		outcome, failure, stageCount := s.runOneSource(ctx, source, provider)

		return Result{
			Phase:        PhaseSource,
			Outcome:      outcome,
			Failure:      failure,
			SourceStages: stageCount,
			Sources: []SourceResult{{
				Provider: provider, Outcome: outcome, Failure: failure, StageCount: stageCount,
			}},
		}
	}

	return Result{Phase: PhaseSource, Outcome: OutcomeNotReady}
}

// runOneSource reads and stores one configured source's own share of the
// trusted inventory, leaving every other source's stored stages untouched.
func (s *Service) runOneSource(
	ctx context.Context, source Source, provider route.Provider,
) (Outcome, FailureCategory, int) {
	trustedCount, err := s.state.TrustedInventoryCount(ctx, provider)
	if err != nil {
		return OutcomeFailed, FailureState, 0
	}
	stages, err := source.Inventory(ctx)
	if err != nil {
		return OutcomeFailed, FailureSource, 0
	}
	for _, stage := range stages {
		if stage.Key().Provider() != provider {
			return OutcomeFailed, FailureSource, 0
		}
	}
	_, ordered, err := normalizeInventory(stages)
	if err != nil {
		return OutcomeFailed, FailureSource, 0
	}
	if len(ordered) == 0 && trustedCount > 0 && !s.allowEmptySourceDeletion() {
		return OutcomeBlocked, FailureEmptySource, 0
	}
	exported := s.exportProfiles(ordered)
	if err := s.state.StoreTrustedInventory(ctx, provider, exported); err != nil {
		return OutcomeFailed, FailureState, 0
	}

	return OutcomeSucceeded, FailureNone, len(ordered)
}

// RunTargets reconciles the stored inventory onto every configured target,
// reading the library the source phase stored rather than fetching it again. An
// inventory that cannot be read back whole fails the phase as a state failure:
// a partial library is indistinguishable from one meant to shrink.
func (s *Service) RunTargets(ctx context.Context) Result {
	targetIDs := s.targetIDs()
	if len(targetIDs) == 0 {
		return Result{Phase: PhaseTargets, Outcome: OutcomeNotReady}
	}

	for _, targetID := range targetIDs {
		authorization, err := s.state.TargetAuthorization(ctx, targetID)
		if err != nil {
			return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}
		}
		if authorization != authorizedState {
			return Result{Phase: PhaseTargets, Outcome: OutcomeNotReady}
		}
	}

	stored, err := s.state.TrustedInventory(ctx)
	if err != nil {
		return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}
	}
	desired, ordered, err := normalizeInventory(stored)
	if err != nil {
		return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}
	}

	result := Result{
		Phase:        PhaseTargets,
		SourceStages: len(ordered),
		Targets:      make([]TargetResult, 0, len(targetIDs)),
	}
	for _, targetID := range targetIDs {
		applied, failure := s.reconcileTarget(ctx, targetID, desired, ordered)
		result.Created += applied.created
		result.Updated += applied.updated
		result.Deleted += applied.deleted
		result.Targets = append(result.Targets, TargetResult{
			ID:      targetID,
			Outcome: targetOutcome(failure),
			Failure: failure,
		})
		if failure == FailureNone {
			continue
		}
		if failure == FailureDeletionLimit && (result.Outcome == "" || result.Outcome == OutcomeBlocked) {
			result.Outcome = OutcomeBlocked
		} else {
			result.Outcome = OutcomeFailed
		}
		if result.Failure == FailureNone {
			result.Failure = failure
		}
	}
	if result.Outcome == "" {
		result.Outcome = OutcomeSucceeded
	}

	return result
}

// RunTarget reconciles the stored inventory onto exactly one configured target,
// applying every rule RunTargets applies to that slot. An unconfigured target ID
// performs no work.
func (s *Service) RunTarget(ctx context.Context, targetID string) Result {
	if !slices.Contains(s.targetIDs(), targetID) {
		return Result{Phase: PhaseTargets, Outcome: OutcomeSkipped}
	}

	authorization, err := s.state.TargetAuthorization(ctx, targetID)
	if err != nil {
		return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}
	}
	if authorization != authorizedState {
		return Result{Phase: PhaseTargets, Outcome: OutcomeNotReady}
	}

	stored, err := s.state.TrustedInventory(ctx)
	if err != nil {
		return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}
	}
	desired, ordered, err := normalizeInventory(stored)
	if err != nil {
		return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}
	}

	targetCounts, failure := s.reconcileTarget(ctx, targetID, desired, ordered)

	return Result{
		Phase:        PhaseTargets,
		Outcome:      targetOutcome(failure),
		Failure:      failure,
		SourceStages: len(ordered),
		Created:      targetCounts.created,
		Updated:      targetCounts.updated,
		Deleted:      targetCounts.deleted,
		Targets:      []TargetResult{{ID: targetID, Outcome: targetOutcome(failure), Failure: failure}},
	}
}

// ClearTarget deletes every route this service owns from one configured target
// and forgets its stage mappings; the next reconciliation rebuilds it. It is the
// one deletion the per-run limit does not bound, nothing schedules it, and it
// deletes only routes carrying an external ID this service issued.
func (s *Service) ClearTarget(ctx context.Context, targetID string) Result {
	if !slices.Contains(s.targetIDs(), targetID) {
		return Result{Phase: PhaseTargets, Outcome: OutcomeSkipped}
	}

	authorization, err := s.state.TargetAuthorization(ctx, targetID)
	if err != nil {
		return Result{Phase: PhaseTargets, Outcome: OutcomeFailed, Failure: FailureState}
	}
	if authorization != authorizedState {
		return Result{Phase: PhaseTargets, Outcome: OutcomeNotReady}
	}

	deleted, failure := s.clearTarget(ctx, targetID)

	return Result{
		Phase:   PhaseTargets,
		Outcome: targetOutcome(failure),
		Failure: failure,
		Deleted: deleted,
		Targets: []TargetResult{{ID: targetID, Outcome: targetOutcome(failure), Failure: failure}},
	}
}

// clearTarget removes the remote routes first and the local record second, so an
// interrupted clear is safe to repeat rather than stranding unowned routes.
func (s *Service) clearTarget(ctx context.Context, targetID string) (int, FailureCategory) {
	refreshToken, refreshErr := s.state.RefreshToken(ctx, targetID)
	if refreshErr != nil {
		return 0, FailureState
	}
	accessToken, replacementRefreshToken, refreshErr := s.target.RefreshAccessToken(ctx, refreshToken)
	if refreshErr != nil {
		return 0, s.handleTargetError(ctx, targetID, refreshErr)
	}
	if replaceErr := s.state.ReplaceRefreshToken(ctx, targetID, replacementRefreshToken); replaceErr != nil {
		return 0, FailureState
	}

	mappings, mappingsErr := s.targetStages(ctx, targetID)
	if mappingsErr != nil {
		return 0, FailureState
	}

	// The count is real even when this fails: those routes are gone. The mappings
	// below are forgotten only on a clean sweep.
	deleted, deleteErr := s.target.DeleteOwnedRoutes(ctx, accessToken)
	if deleteErr != nil {
		return deleted, s.handleTargetError(ctx, targetID, deleteErr)
	}

	for key := range mappings {
		if err := s.state.DeleteTargetStage(ctx, targetID, key.Provider(), key.SourceRouteID(), key.StageOrder()); err != nil {
			return deleted, FailureState
		}
	}

	return deleted, FailureNone
}

// targetOutcome states one slot's reconciliation in the same vocabulary a run
// uses.
func targetOutcome(failure FailureCategory) Outcome {
	if failure == FailureNone {
		return OutcomeSucceeded
	}
	// A deletion limit is a guard doing its job, not a fault: the slot is intact
	// and waiting for an operator to confirm the removals.
	if failure == FailureDeletionLimit {
		return OutcomeBlocked
	}

	return OutcomeFailed
}

// AnnotateStored classifies the ground under the stored inventory and predicts
// its moving time over one read, predicting after classifying. Neither is part
// of either sync phase and neither can fail a run; the failed count tells a
// stage that keeps failing apart from one nobody has asked about yet.
func (s *Service) AnnotateStored(ctx context.Context) (classified, failed int) {
	if s.annotator == nil && s.predictor == nil {
		return 0, 0
	}
	stages, err := s.state.TrustedInventory(ctx)
	if err != nil {
		slog.Warn("surface classification skipped", "reason", "state")

		return 0, 0
	}

	if s.annotator != nil {
		classified, failed, err = s.annotator.Annotate(ctx, stages)
		logPassOutcome("surface classification", classified, failed, err)
	}
	s.predictStored(ctx, stages)

	return classified, failed
}

// predictStored runs the ride-model predictor over the stages AnnotateStored
// read. Silent on success: a routine pass is not worth a log line.
func (s *Service) predictStored(ctx context.Context, stages []route.Route) {
	if s.predictor == nil {
		return
	}
	predicted, failed, err := s.predictor.Predict(ctx, stages)
	logPassOutcome("ride model prediction", predicted, failed, err)
}

// logPassOutcome is the one place either enrichment pass is heard in the log.
// Counts and completion only: no stage names, no geometry, nothing upstream said.
func logPassOutcome(pass string, completed, failed int, err error) {
	if failed == 0 && err == nil {
		return
	}
	reason := "stage"
	if err != nil {
		reason = "stopped_early"
	}
	slog.Warn(pass+" incomplete", "completed", completed, "failed", failed, "reason", reason)
}

// exportProfiles returns the inventory carrying the elevation profile exported to
// devices, leaving identity, revision, and content hash untouched. A stage the
// processor rejects is stored as it arrived, so it still reaches a device.
func (s *Service) exportProfiles(ordered []route.Route) []route.Route {
	stages := make([]route.Route, 0, len(ordered))
	for index := range ordered {
		processed, err := s.processor.Process(&ordered[index])
		if err != nil {
			stages = append(stages, ordered[index])

			continue
		}
		stages = append(stages, processed)
	}

	return stages
}

// reconcileTarget brings one target in line with the stored inventory. The
// stages are encoded as given; deriving again here would smooth twice.
func (s *Service) reconcileTarget(
	ctx context.Context,
	targetID string,
	desired map[route.Key]route.Route,
	ordered []route.Route,
) (counts, FailureCategory) {
	refreshToken, refreshErr := s.state.RefreshToken(ctx, targetID)
	if refreshErr != nil {
		return counts{}, FailureState
	}
	accessToken, replacementRefreshToken, refreshErr := s.target.RefreshAccessToken(ctx, refreshToken)
	if refreshErr != nil {
		return counts{}, s.handleTargetError(ctx, targetID, refreshErr)
	}
	if replaceErr := s.state.ReplaceRefreshToken(ctx, targetID, replacementRefreshToken); replaceErr != nil {
		return counts{}, FailureState
	}

	mappings, mappingsErr := s.targetStages(ctx, targetID)
	if mappingsErr != nil {
		return counts{}, FailureState
	}
	deletions := missingStages(mappings, desired)
	if len(deletions) > maxDeletionsPerTarget {
		return counts{}, FailureDeletionLimit
	}

	// One listing answers ownership for every stage below, so an unchanged library
	// costs one request rather than one per stage against a shared quota.
	owned, listErr := s.target.ListOwnedRoutes(ctx, accessToken)
	if listErr != nil {
		return counts{}, s.handleTargetError(ctx, targetID, listErr)
	}

	var result counts
	for index := range ordered {
		stage := &ordered[index]
		key := stage.Key()
		recorded, tracked := mappings[key]
		wahooRouteID, found := owned[key.ExternalID()]
		if !found {
			fitData, encodeErr := s.encoder.Encode(ctx, *stage)
			if encodeErr != nil {
				return result, FailureCourse
			}
			createdRouteID, createErr := s.target.CreateRoute(ctx, accessToken, stage, fitData)
			if createErr != nil {
				return result, s.handleTargetError(ctx, targetID, createErr)
			}
			if storeErr := s.storeTargetStage(ctx, targetID, stage, createdRouteID); storeErr != nil {
				return result, FailureState
			}
			result.created++

			continue
		}
		if !tracked {
			if storeErr := s.storeTargetStage(ctx, targetID, stage, wahooRouteID); storeErr != nil {
				return result, FailureState
			}

			continue
		}
		if recorded.sourceRevision == stage.Revision() && recorded.contentHash == encodedContentHash(stage) {
			if recorded.wahooRouteID != wahooRouteID {
				if storeErr := s.storeTargetStage(ctx, targetID, stage, wahooRouteID); storeErr != nil {
					return result, FailureState
				}
			}

			continue
		}

		fitData, encodeErr := s.encoder.Encode(ctx, *stage)
		if encodeErr != nil {
			return result, FailureCourse
		}
		updatedRouteID, updateErr := s.target.UpdateRoute(ctx, wahooRouteID, accessToken, stage, fitData)
		if updateErr != nil {
			return result, s.handleTargetError(ctx, targetID, updateErr)
		}
		if storeErr := s.storeTargetStage(ctx, targetID, stage, updatedRouteID); storeErr != nil {
			return result, FailureState
		}
		result.updated++
	}

	for _, key := range deletions {
		// Ownership is still established by external ID before anything is
		// removed; the listing above is where that answer now comes from.
		wahooRouteID, found := owned[key.ExternalID()]
		if found {
			if err := s.target.DeleteRoute(ctx, wahooRouteID, accessToken); err != nil {
				return result, s.handleTargetError(ctx, targetID, err)
			}
			result.deleted++
		}
		if err := s.state.DeleteTargetStage(ctx, targetID, key.Provider(), key.SourceRouteID(), key.StageOrder()); err != nil {
			return result, FailureState
		}
	}

	return result, FailureNone
}

func (s *Service) handleTargetError(ctx context.Context, targetID string, err error) FailureCategory {
	if !s.target.IsUnauthorized(err) {
		// The wahoo package's errors are protocol-level — a status, a rate-limit
		// sentinel, a transport failure — never a route name or a credential.
		slog.Warn("wahoo target reconciliation failed", "target", targetID, "error", err)

		return FailureDestination
	}
	if markErr := s.state.MarkNeedsReauthorization(ctx, targetID); markErr != nil {
		return FailureState
	}

	return FailureAuthorization
}

func (s *Service) targetStages(ctx context.Context, targetID string) (map[route.Key]targetStage, error) {
	mappings := make(map[route.Key]targetStage)
	err := s.state.ForEachTargetStage(
		ctx,
		targetID,
		func(provider route.Provider, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error {
			if provider == "" || routeID <= 0 || stageOrder <= 0 || sourceRevision == "" || contentHash == "" || wahooRouteID <= 0 {
				return errors.New("target stage mapping is invalid")
			}
			key := route.NewKey(provider, routeID, stageOrder)
			if _, exists := mappings[key]; exists {
				return errors.New("target stage mapping is duplicated")
			}
			mappings[key] = targetStage{
				sourceRevision: sourceRevision,
				contentHash:    contentHash,
				wahooRouteID:   wahooRouteID,
			}

			return nil
		},
	)
	if err != nil {
		return nil, fmt.Errorf("listing target stage mappings: %w", err)
	}

	return mappings, nil
}

func (s *Service) storeTargetStage(ctx context.Context, targetID string, stage *route.Route, wahooRouteID int64) error {
	key := stage.Key()
	if err := s.state.UpsertTargetStage(
		ctx,
		targetID,
		key.Provider(),
		key.SourceRouteID(),
		key.StageOrder(),
		stage.Revision(),
		encodedContentHash(stage),
		wahooRouteID,
	); err != nil {
		return fmt.Errorf("storing target stage mapping: %w", err)
	}

	return nil
}

func encodedContentHash(stage *route.Route) string {
	sum := sha256.Sum256([]byte(encoderContentVersion + "\x00" + stage.ContentHash()))

	return hex.EncodeToString(sum[:])
}
