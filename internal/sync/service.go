// Package sync owns safe reconciliation of source stages to Wahoo targets.
package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"sync/atomic"

	"github.com/nobbs/domestique/internal/route"
)

const authorizedState = "authorized"

const encoderContentVersion = "fit-v4-elevation-profile"

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
	// OutcomeNotReady means at least one configured target needs OAuth onboarding.
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

// Result contains aggregate, non-sensitive counts for one synchronization run.
type Result struct {
	// Phase names the half of a synchronization this result describes. The
	// counts a phase does not produce stay zero.
	Phase        Phase
	Outcome      Outcome
	Failure      FailureCategory
	SourceStages int
	Created      int
	Updated      int
	Deleted      int
}

// Options configures safety rules for a synchronizer. It contains no secrets
// and is intentionally independent of the static configuration package.
type Options struct {
	TargetIDs                []string
	MaxDeletionsPerTarget    int
	AllowEmptySourceDeletion bool
}

// Source provides a complete, validated VeloPlanner inventory.
type Source interface {
	Inventory(ctx context.Context) ([]route.Stage, error)
}

// Encoder produces a FIT course for one source stage.
type Encoder interface {
	Encode(ctx context.Context, stage route.Stage) ([]byte, error)
}

// Processor derives a device-export stage without changing source identity or
// source-content state.
type Processor interface {
	Process(stage *route.Stage) (route.Stage, error)
}

// Target performs serial Wahoo OAuth refresh and route operations.
type Target interface {
	RefreshAccessToken(ctx context.Context, refreshToken string) (accessToken, replacementRefreshToken string, err error)
	RouteByExternalID(ctx context.Context, accessToken, externalID string) (routeID int64, found bool, err error)
	CreateRoute(ctx context.Context, accessToken string, stage *route.Stage, fitData []byte) (routeID int64, err error)
	UpdateRoute(ctx context.Context, routeID int64, accessToken string, stage *route.Stage, fitData []byte) (updatedRouteID int64, err error)
	DeleteRoute(ctx context.Context, routeID int64, accessToken string) error
	IsUnauthorized(err error) bool
}

// Annotator enriches the stored inventory with the surface classification of the
// ground each stage covers. It is optional, and deliberately narrow: whatever it
// learns it records itself, so synchronization never has to carry it.
//
// It reports counts rather than nothing, because a pass that classified nothing
// and a pass that had nothing to classify look identical from the outside, and
// an operator wondering why a route has no surface deserves the difference.
type Annotator interface {
	Annotate(ctx context.Context, stages []route.Stage) (classified, failed int, err error)
}

// State owns the minimum durable state operations required by synchronization.
// Callback iteration avoids sharing persistence record types with adapters.
type State interface {
	TargetAuthorization(ctx context.Context, targetID string) (string, error)
	RefreshToken(ctx context.Context, targetID string) (string, error)
	ReplaceRefreshToken(ctx context.Context, targetID, refreshToken string) error
	MarkNeedsReauthorization(ctx context.Context, targetID string) error
	TrustedInventoryCount(ctx context.Context) (int, error)
	StoreTrustedInventory(ctx context.Context, stages []route.Stage) error
	TrustedInventory(ctx context.Context) ([]route.Stage, error)
	ForEachTargetStage(ctx context.Context, targetID string, visit func(routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error) error
	UpsertTargetStage(ctx context.Context, targetID string, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error
	DeleteTargetStage(ctx context.Context, targetID string, routeID int64, stageOrder int) error
}

// Service reconciles a complete source inventory to each configured target.
// It has no dependency on HTTP, static configuration, or concrete adapters.
type Service struct {
	state                    State
	source                   Source
	processor                Processor
	encoder                  Encoder
	target                   Target
	annotator                Annotator
	targetIDs                []string
	maxDeletionsPerTarget    int
	allowEmptySourceDeletion bool
	running                  atomic.Bool
}

// New creates a synchronizer with explicit consumer-owned dependencies. Every
// dependency is required except the annotator: a nil annotator leaves stored
// stages unclassified and changes nothing else about a run.
func New(
	options *Options,
	state State,
	source Source,
	processor Processor,
	encoder Encoder,
	target Target,
	annotator Annotator,
) (*Service, error) {
	if options == nil || state == nil || source == nil || processor == nil || encoder == nil || target == nil {
		return nil, errors.New("sync options and dependencies are required")
	}
	if len(options.TargetIDs) < 1 || len(options.TargetIDs) > 2 || options.MaxDeletionsPerTarget <= 0 || options.MaxDeletionsPerTarget > 5 {
		return nil, errors.New("sync requires between one and two targets and a deletion limit from one through five")
	}

	targetIDs := append([]string(nil), options.TargetIDs...)
	seenTargetIDs := make(map[string]struct{}, len(targetIDs))
	for _, targetID := range targetIDs {
		if targetID == "" {
			return nil, errors.New("sync target IDs are required")
		}
		if _, found := seenTargetIDs[targetID]; found {
			return nil, errors.New("sync target IDs must be unique")
		}
		seenTargetIDs[targetID] = struct{}{}
	}

	return &Service{
		state:                    state,
		source:                   source,
		processor:                processor,
		encoder:                  encoder,
		target:                   target,
		annotator:                annotator,
		targetIDs:                targetIDs,
		maxDeletionsPerTarget:    options.MaxDeletionsPerTarget,
		allowEmptySourceDeletion: options.AllowEmptySourceDeletion,
	}, nil
}

// RunSource reads the VeloPlanner library into stored state.
//
// It contacts no target and needs no authorisation, so a library refresh keeps
// working while a target waits to be reauthorised. What it stores is what the
// target phase later reconciles, which is why the empty-source gate lives here:
// refusing to overwrite a populated inventory with an empty one is what stops
// the deletion before anything downstream can be told to perform it.
//
// A concurrent run performs no work and returns OutcomeSkipped without altering
// durable state.
func (s *Service) RunSource(ctx context.Context) Result {
	if !s.running.CompareAndSwap(false, true) {
		return Result{Phase: PhaseSource, Outcome: OutcomeSkipped}
	}
	defer s.running.Store(false)

	trustedCount, err := s.state.TrustedInventoryCount(ctx)
	if err != nil {
		return Result{Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureState}
	}
	stages, err := s.source.Inventory(ctx)
	if err != nil {
		return Result{Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureSource}
	}
	_, ordered, err := normalizeInventory(stages)
	if err != nil {
		return Result{Phase: PhaseSource, Outcome: OutcomeFailed, Failure: FailureSource}
	}
	result := Result{Phase: PhaseSource, SourceStages: len(ordered)}
	if len(ordered) == 0 && trustedCount > 0 && !s.allowEmptySourceDeletion {
		result.Outcome = OutcomeBlocked
		result.Failure = FailureEmptySource

		return result
	}
	exported := s.exportProfiles(ordered)
	if err := s.state.StoreTrustedInventory(ctx, exported); err != nil {
		result.Outcome = OutcomeFailed
		result.Failure = FailureState

		return result
	}
	result.Outcome = OutcomeSucceeded

	return result
}

// RunTargets reconciles the stored inventory onto every configured target.
//
// It reads the library the source phase stored rather than fetching it again,
// so a target that was unreachable or unauthorised catches up from the same
// inventory the last successful read produced, without asking VeloPlanner a
// second time for an answer already on disk.
//
// The stored inventory is authority for what should exist, exactly as a freshly
// fetched one was: an inventory that cannot be read back whole fails the phase
// as a state failure, because a partial library is indistinguishable from a
// library whose missing stages are meant to be deleted.
//
// A concurrent run performs no work and returns OutcomeSkipped without altering
// durable state.
func (s *Service) RunTargets(ctx context.Context) Result {
	if !s.running.CompareAndSwap(false, true) {
		return Result{Phase: PhaseTargets, Outcome: OutcomeSkipped}
	}
	defer s.running.Store(false)

	for _, targetID := range s.targetIDs {
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

	result := Result{Phase: PhaseTargets, SourceStages: len(ordered)}
	for _, targetID := range s.targetIDs {
		counts, failure := s.reconcileTarget(ctx, targetID, desired, ordered)
		result.Created += counts.created
		result.Updated += counts.updated
		result.Deleted += counts.deleted
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

// AnnotateStored classifies the ground under the stored inventory.
//
// It is deliberately not part of either phase. Getting routes onto a device is
// what a synchronization is for, so a slow or unavailable tagging endpoint must
// neither delay that nor be reported as a failed sync — which is why the caller
// runs this after every phase it intended to run, and why nothing here is
// returned. The annotator caches what it learns and is bounded per pass, so a
// failure costs only the stages this pass would have filled in; the next one
// asks again, and until then those stages simply carry no surface.
//
// It reads the inventory back from state rather than being handed one, because
// a classification is a set of positions in one stored geometry's coordinate
// array: the stages it must describe are precisely the stored ones.
func (s *Service) AnnotateStored(ctx context.Context) {
	if s.annotator == nil {
		return
	}
	stages, err := s.state.TrustedInventory(ctx)
	if err != nil {
		slog.Warn("surface classification skipped", "reason", "state")

		return
	}
	classified, failed, err := s.annotator.Annotate(ctx, stages)
	if failed == 0 && err == nil {
		return
	}
	// The one place this pass is allowed to be heard. It cannot fail a run and it
	// is not worth an alert, but a stage that fails every pass is invisible
	// otherwise: it looks exactly like a stage nobody has asked about. Counts and
	// whether the pass ran to the end — no stage names, no geometry, and nothing
	// the endpoint said.
	reason := "stage"
	if err != nil {
		reason = "stopped_early"
	}
	slog.Warn(
		"surface classification incomplete",
		"classified", classified,
		"failed", failed,
		"reason", reason,
	)
}

// exportProfiles returns the inventory carrying the elevation profile that is
// exported to devices, leaving identity, revision, and content hash untouched.
//
// Storing that profile rather than the raw one means any statistic derived from
// stored state describes the same climb a rider will actually see, instead of
// the satellite noise the normalizer exists to remove.
//
// A stage the processor rejects is stored as it arrived rather than failing the
// run here. What is stored is what the targets are sent, so a rejected stage
// reaches a device as the source planned it rather than not at all, and the map
// draws the same line the device carries.
func (s *Service) exportProfiles(ordered []route.Stage) []route.Stage {
	stages := make([]route.Stage, 0, len(ordered))
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

type stageKey struct {
	routeID    int64
	stageOrder int
}

type targetStage struct {
	sourceRevision string
	contentHash    string
	wahooRouteID   int64
}

type counts struct {
	created int
	updated int
	deleted int
}

func normalizeInventory(stages []route.Stage) (map[stageKey]route.Stage, []route.Stage, error) {
	ordered := append([]route.Stage(nil), stages...)
	sort.Slice(ordered, func(left, right int) bool {
		leftKey := ordered[left].Key()
		rightKey := ordered[right].Key()
		if leftKey.RouteID() != rightKey.RouteID() {
			return leftKey.RouteID() < rightKey.RouteID()
		}

		return leftKey.StageOrder() < rightKey.StageOrder()
	})

	desired := make(map[stageKey]route.Stage, len(ordered))
	for _, stage := range ordered {
		key := stage.Key()
		sourceKey := stageKey{routeID: key.RouteID(), stageOrder: key.StageOrder()}
		if _, exists := desired[sourceKey]; exists {
			return nil, nil, errors.New("source inventory contains a duplicate stage")
		}
		desired[sourceKey] = stage
	}

	return desired, ordered, nil
}

// reconcileTarget brings one target in line with the stored inventory.
//
// The stages it is given are the export profiles the source phase derived and
// stored, and they are encoded as they are. Deriving again here would smooth an
// already smoothed profile, and would put a different course on the device from
// the one the stored state describes and the map draws. One derivation, in the
// phase that owns it.
func (s *Service) reconcileTarget(
	ctx context.Context,
	targetID string,
	desired map[stageKey]route.Stage,
	ordered []route.Stage,
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
	if len(deletions) > s.maxDeletionsPerTarget {
		return counts{}, FailureDeletionLimit
	}

	var result counts
	for index := range ordered {
		stage := &ordered[index]
		key := stage.Key()
		sourceKey := stageKey{routeID: key.RouteID(), stageOrder: key.StageOrder()}
		recorded, tracked := mappings[sourceKey]
		wahooRouteID, found, lookupErr := s.target.RouteByExternalID(ctx, accessToken, key.ExternalID())
		if lookupErr != nil {
			return result, s.handleTargetError(ctx, targetID, lookupErr)
		}
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
		wahooRouteID, found, err := s.target.RouteByExternalID(ctx, accessToken, externalID(key))
		if err != nil {
			return result, s.handleTargetError(ctx, targetID, err)
		}
		if found {
			if err := s.target.DeleteRoute(ctx, wahooRouteID, accessToken); err != nil {
				return result, s.handleTargetError(ctx, targetID, err)
			}
			result.deleted++
		}
		if err := s.state.DeleteTargetStage(ctx, targetID, key.routeID, key.stageOrder); err != nil {
			return result, FailureState
		}
	}

	return result, FailureNone
}

func (s *Service) handleTargetError(ctx context.Context, targetID string, err error) FailureCategory {
	if !s.target.IsUnauthorized(err) {
		return FailureDestination
	}
	if markErr := s.state.MarkNeedsReauthorization(ctx, targetID); markErr != nil {
		return FailureState
	}

	return FailureAuthorization
}

func (s *Service) targetStages(ctx context.Context, targetID string) (map[stageKey]targetStage, error) {
	mappings := make(map[stageKey]targetStage)
	err := s.state.ForEachTargetStage(
		ctx,
		targetID,
		func(routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error {
			if routeID <= 0 || stageOrder <= 0 || sourceRevision == "" || contentHash == "" || wahooRouteID <= 0 {
				return errors.New("target stage mapping is invalid")
			}
			key := stageKey{routeID: routeID, stageOrder: stageOrder}
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

func missingStages(mappings map[stageKey]targetStage, desired map[stageKey]route.Stage) []stageKey {
	missing := make([]stageKey, 0)
	for key := range mappings {
		if _, exists := desired[key]; !exists {
			missing = append(missing, key)
		}
	}
	sort.Slice(missing, func(left, right int) bool {
		if missing[left].routeID != missing[right].routeID {
			return missing[left].routeID < missing[right].routeID
		}

		return missing[left].stageOrder < missing[right].stageOrder
	})

	return missing
}

func (s *Service) storeTargetStage(ctx context.Context, targetID string, stage *route.Stage, wahooRouteID int64) error {
	key := stage.Key()
	if err := s.state.UpsertTargetStage(
		ctx,
		targetID,
		key.RouteID(),
		key.StageOrder(),
		stage.Revision(),
		encodedContentHash(stage),
		wahooRouteID,
	); err != nil {
		return fmt.Errorf("storing target stage mapping: %w", err)
	}

	return nil
}

func encodedContentHash(stage *route.Stage) string {
	sum := sha256.Sum256([]byte(encoderContentVersion + "\x00" + stage.ContentHash()))

	return hex.EncodeToString(sum[:])
}

func externalID(key stageKey) string {
	return "domestique:veloplanner:" + strconv.FormatInt(key.routeID, 10) + ":stage:" + strconv.Itoa(key.stageOrder)
}
