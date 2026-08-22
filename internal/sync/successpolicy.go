package sync

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// SuccessPolicy decides what a routine successful run notifies.
//
// It governs routine success alone. A failure, a blocked run, and the first
// success after either are signals an operator is waiting for, and no policy
// here can suppress them.
type SuccessPolicy string

const (
	// SuccessEvery pushes one message per successful run.
	SuccessEvery SuccessPolicy = "every"
	// SuccessQuiet pushes nothing for a routine success, leaving failure and
	// recovery as the only traffic.
	SuccessQuiet SuccessPolicy = "quiet"
	// SuccessDigest replaces routine success pushes with one aggregate message
	// per interval.
	SuccessDigest SuccessPolicy = "digest"
)

// SuccessNotification configures routine-success delivery.
type SuccessNotification struct {
	Policy SuccessPolicy
	// Interval is how much time separates two digests. It is read only by
	// SuccessDigest.
	Interval time.Duration
}

// validate rejects a policy the composition root failed to supply.
//
// The zero value is not quietly treated as SuccessEvery: every value here
// decides how much an operator hears, and a miswired one that silently went
// quiet is the failure this service can least afford to hide.
func (s SuccessNotification) validate() error {
	switch s.Policy {
	case SuccessEvery, SuccessQuiet:
		return nil
	case SuccessDigest:
		if s.Interval <= 0 {
			return errors.New("a success digest needs a positive interval")
		}

		return nil
	default:
		return fmt.Errorf("unknown success notification policy %q", s.Policy)
	}
}

// digest totals the successful runs of one window. It carries counts and
// nothing else: no route, no target identity, no provider text.
type digest struct {
	sourceRuns int
	targetRuns int
	created    int
	updated    int
	deleted    int
}

func (d digest) empty() bool {
	return d.sourceRuns == 0 && d.targetRuns == 0
}

func (d digest) message(since time.Time) string {
	return fmt.Sprintf(
		"since %s: source_runs=%d target_runs=%d created=%d updated=%d deleted=%d",
		since.UTC().Format(time.RFC3339),
		d.sourceRuns,
		d.targetRuns,
		d.created,
		d.updated,
		d.deleted,
	)
}

// notifySuccess delivers what the configured policy allows for one successful
// run.
//
// A recovery is delivered whatever the policy, because the operator is owed the
// end of a failure they were told about. Everything else is routine, and
// routine is what a policy may hold back — a digest reports it later, from the
// record, rather than from here.
func (r *Reporter) notifySuccess(ctx context.Context, result *Result, reference string, recovered bool) {
	if !recovered && r.success.Policy != SuccessEvery {
		return
	}
	if err := r.notifier.Send(ctx, "Domestique sync", successMessage(result, reference)); err != nil {
		return
	}
}

// recovered reports whether this success ends a run of unhealthy passes, which
// is what makes it the recovery signal rather than a routine success.
//
// Anything the phase recorded other than a success counts. A target awaiting
// OAuth records `not_ready` and notifies nothing, so a phase that failed, was
// left needing onboarding, and then came good would otherwise leave the failure
// alert open with no message ever closing it — the one thing the recovery signal
// exists to prevent.
//
// It reads the phase's own previous run, so the answer survives a restart and
// cannot drift from the record an operator reads back. It is asked only when a
// policy might hold the success back; under SuccessEvery the message goes out
// either way and the query would buy nothing.
func (r *Reporter) recovered(ctx context.Context, phase Phase) bool {
	if r.success.Policy == SuccessEvery {
		return false
	}
	outcome, found, err := r.state.LastPhaseOutcome(ctx, string(phase))
	if err != nil || !found {
		// An unreadable history must not silence a success that may be the
		// recovery. Sending one message too many costs an operator a line;
		// withholding the recovery costs them the end of an alert.
		return err != nil
	}

	return outcome != string(OutcomeSucceeded)
}

// notifyDigest sends one aggregate message per interval, covering the
// successful runs recorded since the last one.
//
// It is asked once per pass, after both halves are recorded, rather than as
// each half is notified. A pass whose halves straddled the digest would
// otherwise anchor the next window on its own timestamp and drop the half
// recorded a moment later, which is a run reported in no digest at all.
//
// The first digest of a new configuration sends nothing and starts the clock:
// the alternative is an opening message covering however much history the
// database happens to hold, which is not the period the operator asked for.
func (r *Reporter) notifyDigest(ctx context.Context, now time.Time) {
	since, after, found, err := r.state.LastDigestNotification(ctx)
	if err != nil {
		return
	}
	if found && now.Sub(since) < r.success.Interval {
		return
	}
	totals, latest, err := r.totalSuccessesAfter(ctx, after)
	if err != nil {
		return
	}
	// Two silent cases move the window without a message: the first digest of a
	// new configuration, and a period that nothing succeeded in. Leaving the
	// second where it was would make the next digest claim every quiet period
	// before it as part of its own.
	if !found || totals.empty() {
		r.anchorDigest(ctx, now, latest)

		return
	}
	if err := r.notifier.Send(ctx, "Domestique sync digest", totals.message(since)); err != nil {
		return
	}
	r.anchorDigest(ctx, now, latest)
}

// anchorDigest moves the window past the runs just reported.
//
// An anchor that cannot be written leaves the next pass to place it. That
// repeats a period rather than losing one, which is the right way round for a
// message whose only job is to summarise.
func (r *Reporter) anchorDigest(ctx context.Context, now time.Time, lastRunID int64) {
	if err := r.state.RecordDigestNotification(ctx, now, lastRunID); err != nil {
		slog.Warn("success digest window not anchored", "reason", "state")
	}
}

// totalSuccessesAfter adds up the successful runs of one window and reports the
// last run it saw, which becomes the next window's lower bound.
//
// Bounding by run rather than by time is what keeps a run from falling between
// two windows: recorded times are whole seconds, so a run finishing in the same
// second as a digest cannot be placed on one side of it reliably.
func (r *Reporter) totalSuccessesAfter(ctx context.Context, after int64) (digest, int64, error) {
	var totals digest
	latest := after
	if err := r.state.ForEachSuccessfulRunAfter(ctx, after,
		func(id int64, phase string, created, updated, deleted int) error {
			if phase == string(PhaseSource) {
				totals.sourceRuns++
			} else {
				totals.targetRuns++
			}
			totals.created += created
			totals.updated += updated
			totals.deleted += deleted
			latest = id

			return nil
		}); err != nil {
		return digest{}, after, fmt.Errorf("totalling successful runs: %w", err)
	}

	return totals, latest, nil
}
