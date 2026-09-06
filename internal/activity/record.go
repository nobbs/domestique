package activity

import (
	"context"
	"log/slog"
	"slices"
)

// Record stores one notified activity of one target and fills its samples, in
// at most two requests. The notification is a hint: what is stored is what the
// account answers, so an unrecordable, already recorded, or unheld one changes nothing.
func (p *Poller) Record(ctx context.Context, targetID string, workoutID int64) Result {
	authorization, err := p.store.TargetAuthorization(ctx, targetID)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	if authorization != authorizedState {
		return Result{Outcome: NotReady}
	}

	accessToken, failure := p.accessToken(ctx, targetID)
	if failure != FailureNone {
		return Result{Outcome: Failed, Failure: failure}
	}

	listing, readErr := p.source.Activity(ctx, accessToken, workoutID)
	if readErr != nil {
		return p.recordFailure(ctx, targetID, workoutID, readErr)
	}
	if !p.source.IsRecordable(listing) {
		return Result{Outcome: Unchanged}
	}

	stored, failure := p.recordActivity(ctx, targetID, accessToken, listing)
	if failure != FailureNone {
		return Result{Outcome: Failed, Failure: failure}
	}
	if stored.skipped {
		return Result{Outcome: Polled, Skipped: 1}
	}

	records, unreadable, failure := p.fillOne(ctx, targetID, listing.ID)
	result := Result{Stored: stored.count, RecordsStored: records, RecordsUnreadable: unreadable}
	if failure != FailureNone {
		result.Outcome, result.Failure = Failed, failure

		return result
	}
	if stored.count == 0 && records == 0 && unreadable == 0 {
		return Result{Outcome: Unchanged}
	}
	slog.Info("activity recorded", "target", targetID,
		"stored", stored.count, "records", records, "unreadable", unreadable)
	result.Outcome = Polled

	return result
}

// storeOutcome is what storing one notified activity came to: how many were
// added, and whether the account refused to say anything about it.
type storeOutcome struct {
	count   int
	skipped bool
}

// recordActivity stores the notified activity unless it is stored already,
// asking for its summary only when its listing entry carried none.
func (p *Poller) recordActivity(
	ctx context.Context, targetID, accessToken string, listing Listing,
) (storeOutcome, Failure) {
	known, knownErr := p.store.KnownActivityIDs(ctx, targetID)
	if knownErr != nil {
		return storeOutcome{}, FailureState
	}
	if slices.Contains(known, listing.ID) {
		return storeOutcome{}, FailureNone
	}

	var summary Summary
	if listing.Summary != nil {
		summary = *listing.Summary
	} else {
		read, summaryErr := p.source.ActivitySummary(ctx, accessToken, listing.ID)
		if summaryErr != nil {
			if !p.source.IsUnreadable(summaryErr) {
				return storeOutcome{}, p.classify(ctx, targetID, summaryErr)
			}
			if skipErr := p.store.RecordActivitySkip(
				ctx, targetID, listing.ID, summaryErr.Error(), p.now()); skipErr != nil {
				return storeOutcome{}, FailureState
			}

			return storeOutcome{skipped: true}, FailureNone
		}
		summary = read
	}
	if storeErr := p.store.StoreActivity(ctx, targetID, listing, summary, p.now()); storeErr != nil {
		return storeOutcome{}, FailureState
	}

	return storeOutcome{count: 1}, FailureNone
}

// fillOne fills the samples of the notified activity alone, and only while they
// are still absent: one already filled is nothing to do rather than a re-read.
func (p *Poller) fillOne(ctx context.Context, targetID string, id int64) (stored, unreadable int, failure Failure) {
	pending, err := p.store.ActivitiesAwaitingRecords(ctx, targetID, MaxRecordsPerPoll)
	if err != nil {
		return 0, 0, FailureState
	}
	index := slices.IndexFunc(pending, func(awaiting PendingActivity) bool { return awaiting.ID == id })
	if index < 0 {
		return 0, 0, FailureNone
	}

	return p.fill(ctx, targetID, pending[index])
}

// recordFailure is what a refused reading of the notified activity comes to: an
// activity only its own reading refuses is skipped, so the scheduled poll's own
// retry decides when it is asked for again.
func (p *Poller) recordFailure(ctx context.Context, targetID string, workoutID int64, err error) Result {
	if !p.source.IsUnreadable(err) {
		return Result{Outcome: Failed, Failure: p.classify(ctx, targetID, err)}
	}
	if skipErr := p.store.RecordActivitySkip(ctx, targetID, workoutID, err.Error(), p.now()); skipErr != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}

	return Result{Outcome: Polled, Skipped: 1}
}
