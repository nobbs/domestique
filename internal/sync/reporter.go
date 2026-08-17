package sync

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const failureNotificationSuppression = 6 * time.Hour

// RunState records terminal run data and failure-notification delivery state.
type RunState interface {
	RecordSyncRun(ctx context.Context, startedAt, finishedAt time.Time, outcome, detail string, sourceStages, created, updated, deleted int) error
	LastFailureNotification(ctx context.Context, category string) (sentAt time.Time, found bool, err error)
	RecordFailureNotification(ctx context.Context, category string, sentAt time.Time) error
}

// Notifier delivers already-safe notification text.
type Notifier interface {
	Send(ctx context.Context, title, message string) error
}

// Reporter adds durable run recording and notification policy around a
// synchronization service. It does not expose provider errors or route names.
type Reporter struct {
	runner   Runner
	state    RunState
	notifier Notifier
	now      func() time.Time
}

// Runner is the application service seam consumed by the reporter and
// scheduler.
type Runner interface {
	Run(ctx context.Context) Result
}

// NewReporter creates a reporting runner with explicit dependencies.
func NewReporter(runner Runner, state RunState, notifier Notifier) (*Reporter, error) {
	if runner == nil || state == nil || notifier == nil {
		return nil, errors.New("sync runner, run state, and notifier are required")
	}

	return &Reporter{runner: runner, state: state, notifier: notifier, now: time.Now}, nil
}

// Run records a terminal run and sends the configured safe notifications.
// Notification or state-delivery failures never rewrite the sync outcome.
func (r *Reporter) Run(ctx context.Context) Result {
	startedAt := r.now().UTC()
	result := r.runner.Run(ctx)
	if result.Outcome == OutcomeSkipped {
		return result
	}
	finishedAt := r.now().UTC()
	if err := r.state.RecordSyncRun(
		ctx,
		startedAt,
		finishedAt,
		string(result.Outcome),
		string(result.Failure),
		result.SourceStages,
		result.Created,
		result.Updated,
		result.Deleted,
	); err != nil {
		return result
	}

	switch result.Outcome {
	case OutcomeSucceeded:
		if err := r.notifier.Send(ctx, "Domestique sync", successMessage(result)); err != nil {
			return result
		}
	case OutcomeFailed, OutcomeBlocked:
		r.notifyFailure(ctx, result, finishedAt)
	}

	return result
}

func (r *Reporter) notifyFailure(ctx context.Context, result Result, now time.Time) {
	if result.Failure == FailureNone {
		return
	}
	lastSentAt, found, err := r.state.LastFailureNotification(ctx, string(result.Failure))
	if err != nil || (found && now.Sub(lastSentAt) < failureNotificationSuppression) {
		return
	}
	if err := r.notifier.Send(ctx, "Domestique sync failed", failureMessage(result.Failure)); err != nil {
		return
	}
	if err := r.state.RecordFailureNotification(ctx, string(result.Failure), now); err != nil {
		return
	}
}

func successMessage(result Result) string {
	return fmt.Sprintf(
		"succeeded: source_stages=%d created=%d updated=%d deleted=%d",
		result.SourceStages,
		result.Created,
		result.Updated,
		result.Deleted,
	)
}

func failureMessage(category FailureCategory) string {
	return "failed: " + string(category)
}
