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
	"sync/atomic"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

const authorizedState = "authorized"

// maxDeletionsPerTarget bounds how many owned routes one reconciliation may
// remove from one target.
//
// It was an operator setting only in name: the configuration file accepted the
// key and then refused every value but five, so the number was never anything
// else. A limit that exists to make a runaway deletion stop is not a dial an
// operator has a use for, and naming it here says so.
const maxDeletionsPerTarget = 5

// Interval is how often a scheduled reconciliation runs.
//
// Like maxDeletionsPerTarget it was a file key that accepted exactly one value.
// An hour is the cadence the whole design is sized for — the rate limits, the
// staleness bound, and the digest window are all expressed against it — so it
// is stated here rather than asked for.
const Interval = time.Hour

const encoderContentVersion = "fit-v4-elevation-profile"

// Options configures safety rules for a synchronizer. It contains no secrets
// and is intentionally independent of the configuration packages.
type Options struct {
	// AllowEmptySourceDeletion reports whether a trusted but empty source may
	// delete the final owned destination routes.
	//
	// It is a function rather than a value because it is the switch an operator
	// turns on for one deliberate run and off again afterwards. It is asked once
	// per source, as that source is read, so turning it off reaches the sources
	// a run has not read yet.
	AllowEmptySourceDeletion func() bool

	// Sources and TargetIDs are functions for the same reason: both are
	// settings an operator edits while the service runs, so a run reads them as
	// it starts rather than inheriting what was configured at composition.
	// Both answer with what has already been validated; an empty answer is a
	// service that has not been configured yet, and a run says so.
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
	running                  atomic.Bool
}

// New creates a synchronizer with explicit consumer-owned dependencies. Every
// dependency is required except the annotator and the predictor: a nil
// annotator leaves stored stages unclassified, a nil predictor leaves them
// carrying no prediction, and either changes nothing else about a run.
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

// RunSource reads every configured source into stored state, one at a time.
//
// It contacts no target and needs no authorisation, so a library refresh keeps
// working while a target waits to be reauthorised. Each source is read and
// stored independently: one source's failure does not stop the others from
// being read, and only a source that was read successfully has its own stored
// stages replaced. A source that failed keeps the stages it was last known to
// have, which is why the empty-source gate is evaluated per source, against
// that source's own prior count — refusing to overwrite a populated source
// with an empty one is what stops the deletion before anything downstream can
// be told to perform it.
//
// A concurrent run performs no work and returns OutcomeSkipped without altering
// durable state.
func (s *Service) RunSource(ctx context.Context) Result {
	if !s.running.CompareAndSwap(false, true) {
		return Result{Phase: PhaseSource, Outcome: OutcomeSkipped}
	}
	defer s.running.Store(false)

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
		// category describes no fault, and reporting it once a source has actually
		// failed would record and alert on the wrong category.
		if result.Failure == FailureNone || (result.Failure == FailureEmptySource && failure != FailureEmptySource) {
			result.Failure = failure
		}
	}
	if result.Outcome == "" {
		result.Outcome = OutcomeSucceeded
	}

	return result
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

// RunTarget reconciles the stored inventory onto exactly one configured
// target, leaving every other target untouched. It shares the same ownership,
// ordering, update-before-delete, and deletion-limit rules RunTargets applies
// to that slot — this is the same reconciliation, scoped to one account rather
// than run over all of them.
//
// It shares RunTargets' mutual exclusion: a target-specific request must not
// race a full synchronization over the same stored state and target-stage
// mappings.
//
// An unconfigured target ID performs no work, the same defensive answer a
// concurrent run gives, because the caller is expected to have already
// refused a slot this service was never given.
func (s *Service) RunTarget(ctx context.Context, targetID string) Result {
	if !slices.Contains(s.targetIDs(), targetID) {
		return Result{Phase: PhaseTargets, Outcome: OutcomeSkipped}
	}
	if !s.running.CompareAndSwap(false, true) {
		return Result{Phase: PhaseTargets, Outcome: OutcomeSkipped}
	}
	defer s.running.Store(false)

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

// ClearTarget deletes every route this service owns from one configured
// target, and forgets its stage mappings, leaving the slot as though it had
// never been written to. The next reconciliation rebuilds it from stored
// source state.
//
// It is the one deletion path the per-run deletion limit does not apply to,
// and that is deliberate. The limit exists so an *unattended* run cannot act
// on a bad inventory; this runs only when an operator asks for it directly,
// having already been told what it will do. For the same reason nothing
// schedules it: it is reachable from a manual trigger alone.
//
// What it may not do is unchanged. It deletes only routes carrying an external
// ID this service issued, so a hand-made route in the same account is as
// untouchable here as anywhere else — the ownership rule is the reason this
// can exist at all, not an exception to it.
//
// It leaves the library alone: source stages, their geometry, and the trusted
// inventory are untouched, because clearing a destination is not forgetting
// what should be on it.
func (s *Service) ClearTarget(ctx context.Context, targetID string) Result {
	if !slices.Contains(s.targetIDs(), targetID) {
		return Result{Phase: PhaseTargets, Outcome: OutcomeSkipped}
	}
	if !s.running.CompareAndSwap(false, true) {
		return Result{Phase: PhaseTargets, Outcome: OutcomeSkipped}
	}
	defer s.running.Store(false)

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

// clearTarget removes the remote routes first and the local record of them
// second. That order is what makes an interrupted clear safe to repeat: a
// mapping still present for an already-deleted route is re-cleared harmlessly,
// where the reverse would strand routes nothing remembers owning.
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

	// The count is real even when this fails: those routes are gone, and
	// repeating the clear continues from what is left rather than starting
	// over. The mappings below are forgotten only on a clean sweep.
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
// uses, so a reader does not have to know the failure categories to tell a
// blocked slot from a broken one.
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
// its moving time, reporting how much of the classification pass could not
// classify. The two run over the same read of the inventory, predicting after
// classifying, so a stage's prediction can read the classification this same
// pass just wrote rather than waiting a full cycle behind it.
//
// Neither is part of either sync phase. Getting routes onto a device is what a
// synchronization is for, so a slow or unavailable tagging endpoint, or a
// coefficient file predicting nothing new, must neither delay that nor be
// reported as a failed sync — which is why the caller runs this after every
// phase it intended to run. Each pass caches what it learns and is bounded to
// this one call, so a failure costs only the stages this pass would have
// filled in; the next one asks again, and until then those stages simply carry
// no surface or no prediction. The failed count is what tells that apart from a
// stage nobody has asked about yet, which looks identical otherwise.
//
// It reads the inventory back from state rather than being handed one, because
// a classification is a set of positions in one stored geometry's coordinate
// array: the stages it must describe are precisely the stored ones.
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

// predictStored runs the ride-model predictor over the same stages
// AnnotateStored just read, and is silent on success for the same reason
// AnnotateStored's own logging is: a routine pass is not worth a log line.
func (s *Service) predictStored(ctx context.Context, stages []route.Route) {
	if s.predictor == nil {
		return
	}
	predicted, failed, err := s.predictor.Predict(ctx, stages)
	logPassOutcome("ride model prediction", predicted, failed, err)
}

// logPassOutcome is the one place either enrichment pass is allowed to be
// heard in the log. Neither pass can fail a run and neither is worth an alert,
// but a stage that fails every pass is invisible otherwise: it looks exactly
// like a stage nobody has asked about. Counts and whether the pass ran to the
// end — no stage names, no geometry, and nothing the endpoint said.
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

	// One listing answers ownership for every stage below, including the ones
	// nothing changed about. It is the same evidence a per-stage lookup gave —
	// what the target actually holds right now, by external ID — gathered once
	// instead of once per stage, so an unchanged library costs a single
	// request rather than one per stage against a quota every target shares.
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
		// FailureDestination alone does not distinguish a rate limit an operator
		// should just wait out from a genuine, unexpected Wahoo failure. The
		// wahoo package's own errors are already protocol-level — an HTTP status,
		// a rate-limit sentinel, a transport failure — never a route name, a
		// stage, or a credential, so logging the message here stays inside the
		// same "aggregate facts only" rule every other log line in this package
		// follows.
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
