// Package sync owns safe reconciliation of source stages to Wahoo targets.
package sync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync/atomic"

	"github.com/nobbs/domestique/internal/route"
)

const authorizedState = "authorized"

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

// Target performs serial Wahoo OAuth refresh and route operations.
type Target interface {
	RefreshAccessToken(ctx context.Context, refreshToken string) (accessToken, replacementRefreshToken string, err error)
	RouteByExternalID(ctx context.Context, accessToken, externalID string) (routeID int64, found bool, err error)
	CreateRoute(ctx context.Context, accessToken string, stage *route.Stage, fitData []byte) (routeID int64, err error)
	UpdateRoute(ctx context.Context, routeID int64, accessToken string, stage *route.Stage, fitData []byte) (updatedRouteID int64, err error)
	DeleteRoute(ctx context.Context, routeID int64, accessToken string) error
	IsUnauthorized(err error) bool
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
	ForEachTargetStage(ctx context.Context, targetID string, visit func(routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error) error
	UpsertTargetStage(ctx context.Context, targetID string, routeID int64, stageOrder int, sourceRevision, contentHash string, wahooRouteID int64) error
	DeleteTargetStage(ctx context.Context, targetID string, routeID int64, stageOrder int) error
}

// Service reconciles a complete source inventory to each configured target.
// It has no dependency on HTTP, static configuration, or concrete adapters.
type Service struct {
	state                    State
	source                   Source
	encoder                  Encoder
	target                   Target
	targetIDs                []string
	maxDeletionsPerTarget    int
	allowEmptySourceDeletion bool
	running                  atomic.Bool
}

// New creates a synchronizer with explicit consumer-owned dependencies.
func New(options *Options, state State, source Source, encoder Encoder, target Target) (*Service, error) {
	if options == nil || state == nil || source == nil || encoder == nil || target == nil {
		return nil, errors.New("sync options and dependencies are required")
	}
	if len(options.TargetIDs) != 2 || options.MaxDeletionsPerTarget <= 0 || options.MaxDeletionsPerTarget > 5 {
		return nil, errors.New("sync requires two targets and a deletion limit from one through five")
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
		encoder:                  encoder,
		target:                   target,
		targetIDs:                targetIDs,
		maxDeletionsPerTarget:    options.MaxDeletionsPerTarget,
		allowEmptySourceDeletion: options.AllowEmptySourceDeletion,
	}, nil
}

// Run synchronizes both targets serially. A concurrent trigger performs no
// work and returns OutcomeSkipped without altering durable state.
func (s *Service) Run(ctx context.Context) Result {
	if !s.running.CompareAndSwap(false, true) {
		return Result{Outcome: OutcomeSkipped}
	}
	defer s.running.Store(false)

	for _, targetID := range s.targetIDs {
		authorization, err := s.state.TargetAuthorization(ctx, targetID)
		if err != nil {
			return Result{Outcome: OutcomeFailed, Failure: FailureState}
		}
		if authorization != authorizedState {
			return Result{Outcome: OutcomeNotReady}
		}
	}

	trustedCount, err := s.state.TrustedInventoryCount(ctx)
	if err != nil {
		return Result{Outcome: OutcomeFailed, Failure: FailureState}
	}
	stages, err := s.source.Inventory(ctx)
	if err != nil {
		return Result{Outcome: OutcomeFailed, Failure: FailureSource}
	}
	desired, ordered, err := normalizeInventory(stages)
	if err != nil {
		return Result{Outcome: OutcomeFailed, Failure: FailureSource}
	}
	result := Result{SourceStages: len(ordered)}
	if len(ordered) == 0 && trustedCount > 0 && !s.allowEmptySourceDeletion {
		result.Outcome = OutcomeBlocked
		result.Failure = FailureEmptySource

		return result
	}
	if err := s.state.StoreTrustedInventory(ctx, ordered); err != nil {
		result.Outcome = OutcomeFailed
		result.Failure = FailureState

		return result
	}

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
		if recorded.sourceRevision == stage.Revision() && recorded.contentHash == stage.ContentHash() {
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
		stage.ContentHash(),
		wahooRouteID,
	); err != nil {
		return fmt.Errorf("storing target stage mapping: %w", err)
	}

	return nil
}

func externalID(key stageKey) string {
	return "domestique:veloplanner:" + strconv.FormatInt(key.routeID, 10) + ":stage:" + strconv.Itoa(key.stageOrder)
}
