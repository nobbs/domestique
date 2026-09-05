package activity

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"
)

// authorizedState is the target authorization the store reports for a slot whose
// Wahoo account may be read; anything else needs interactive OAuth first.
const authorizedState = "authorized"

// MaxNewPerPoll bounds how many summaries one poll reads. The Wahoo sandbox tier
// allows 250 requests a day across every target and every task sharing it.
const MaxNewPerPoll = 25

// Listing is one recorded activity as the rider's account lists it.
type Listing struct {
	Starts     time.Time
	ID         int64
	TypeID     int
	LocationID int
}

// Summary is one activity's stored totals and the provider's own summary
// document, kept verbatim so a later decoder need not read it again.
type Summary struct {
	Raw            []byte
	DistanceMetres float64
	MovingSeconds  float64
	ElapsedSeconds float64
	AscentMetres   float64
}

// Skip is one activity a poll could not read: how often it has been tried and
// when it was last tried decide when it is offered again.
type Skip struct {
	LastAttempt time.Time
	ID          int64
	Attempts    int
}

const (
	skipRetryBase = 24 * time.Hour
	skipRetryCap  = 28 * 24 * time.Hour
)

// retryDue reports whether a skipped activity's wait has passed. The wait
// doubles with each attempt from a day to a four-week ceiling and never becomes
// permanent: a workout unreadable today may be readable next month, and each
// retry spends one request from a window that fits roughly MaxNewPerPoll.
func retryDue(skip Skip, now time.Time) bool {
	doublings := min(max(skip.Attempts, 1), 6) - 1
	wait := min(skipRetryBase<<doublings, skipRetryCap)

	return !now.Before(skip.LastAttempt.Add(wait))
}

// Stored is one recorded activity as the read model serves it: the listing and
// the summary totals, without the provider's own summary document.
type Stored struct {
	StartedAt      time.Time
	ID             int64
	DistanceMetres float64
	MovingSeconds  float64
	ElapsedSeconds float64
	AscentMetres   float64
	TypeID         int
	LocationID     int
}

// Source is the rider's activity provider, in this package's own vocabulary.
type Source interface {
	RefreshAccessToken(ctx context.Context, refreshToken string) (accessToken, replacementRefreshToken string, err error)
	ListActivities(ctx context.Context, accessToken string) ([]Listing, error)
	ActivitySummary(ctx context.Context, accessToken string, id int64) (Summary, error)
	IsUnauthorized(err error) bool
	// IsUnreadable reports a summary rejection that belongs to that one
	// activity rather than to the connection, the quota or the grant.
	IsUnreadable(err error) bool
}

// Store is the durable state one poll reads and adds to. It never deletes.
type Store interface {
	TargetAuthorization(ctx context.Context, targetID string) (string, error)
	RefreshToken(ctx context.Context, targetID string) (string, error)
	ReplaceRefreshToken(ctx context.Context, targetID, refreshToken string) error
	MarkNeedsReauthorization(ctx context.Context, targetID string) error
	KnownActivityIDs(ctx context.Context, targetID string) ([]int64, error)
	// StoreActivity also forgets any skip recorded for the same activity.
	StoreActivity(ctx context.Context, targetID string, listing Listing, summary Summary, now time.Time) error
	ActivitySkips(ctx context.Context, targetID string) ([]Skip, error)
	// RecordActivitySkip counts one more failed read of an activity. observed is
	// the source's protocol-level error text, never a ride's name or a credential.
	RecordActivitySkip(ctx context.Context, targetID string, id int64, observed string, now time.Time) error
}

// Outcome is what one poll came to. Its zero value is Failed, so a result
// nobody filled in never reads as work that succeeded.
type Outcome int

const (
	// Failed means the poll stopped early; whatever it stored before that stays.
	Failed Outcome = iota
	// Polled means at least one new activity was stored or skipped.
	Polled
	// Unchanged means the account held nothing this service had not stored.
	Unchanged
	// NotReady means the target needs interactive OAuth before it can be read.
	NotReady
)

// Failure is a stable, safe-to-display reason for a failed poll. It never
// carries provider response text.
type Failure string

const (
	// FailureNone means the poll completed without a failure category.
	FailureNone Failure = ""
	// FailureAuthorization means the target needs interactive OAuth again.
	FailureAuthorization Failure = "authorization"
	// FailureUpstream means an activity read did not complete.
	FailureUpstream Failure = "upstream"
	// FailureState means stored state could not be read or updated safely.
	FailureState Failure = "state"
)

// Result is one poll's aggregate, non-sensitive outcome.
type Result struct {
	Failure Failure
	Outcome Outcome
	// Stored counts the activities this poll added, including those a failed
	// poll managed before it stopped.
	Stored int
	// Skipped counts the activities this poll could not read and set aside to
	// try again later.
	Skipped int
}

// Poller reads one target's recorded activities into the store. It adds and
// overwrites; it never removes an activity the account no longer lists.
type Poller struct {
	source Source
	store  Store
	now    func() time.Time
}

// NewPoller builds a poller over its source and store.
func NewPoller(source Source, store Store, now func() time.Time) (*Poller, error) {
	if source == nil || store == nil || now == nil {
		return nil, errors.New("activity: a source, a store and a clock are required")
	}

	return &Poller{source: source, store: store, now: now}, nil
}

// Poll stores a summary for every activity of one target this service has not
// stored yet, oldest first and at most MaxNewPerPoll of them. A read that fails
// part way keeps what it already stored: the next poll continues from there. An
// activity only its own summary rejects is skipped and retried later, so one
// unreadable ride never stops the ones after it.
func (p *Poller) Poll(ctx context.Context, targetID string) Result {
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

	listings, listErr := p.source.ListActivities(ctx, accessToken)
	if listErr != nil {
		return Result{Outcome: Failed, Failure: p.classify(ctx, targetID, listErr)}
	}
	known, knownErr := p.store.KnownActivityIDs(ctx, targetID)
	if knownErr != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	skips, skipsErr := p.store.ActivitySkips(ctx, targetID)
	if skipsErr != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}

	pending := unstored(listings, append(known, deferred(skips, p.now())...))
	if len(pending) == 0 {
		return Result{Outcome: Unchanged}
	}

	var stored, skipped int
	for _, listing := range pending {
		summary, summaryErr := p.source.ActivitySummary(ctx, accessToken, listing.ID)
		if summaryErr != nil && p.source.IsUnreadable(summaryErr) {
			if skipErr := p.store.RecordActivitySkip(ctx, targetID, listing.ID, summaryErr.Error(), p.now()); skipErr != nil {
				return Result{Outcome: Failed, Failure: FailureState, Stored: stored, Skipped: skipped}
			}
			skipped++

			continue
		}
		if summaryErr != nil {
			return Result{Outcome: Failed, Failure: p.classify(ctx, targetID, summaryErr), Stored: stored, Skipped: skipped}
		}
		if storeErr := p.store.StoreActivity(ctx, targetID, listing, summary, p.now()); storeErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Stored: stored, Skipped: skipped}
		}
		stored++
	}
	slog.Info("activities polled", "target", targetID, "stored", stored, "skipped", skipped)

	return Result{Outcome: Polled, Stored: stored, Skipped: skipped}
}

// deferred is the skipped activities whose retry is not yet due.
func deferred(skips []Skip, now time.Time) []int64 {
	var ids []int64
	for _, skip := range skips {
		if !retryDue(skip, now) {
			ids = append(ids, skip.ID)
		}
	}

	return ids
}

// accessToken refreshes the target's credentials, replacing the stored refresh
// token before anything uses the access token it came with.
func (p *Poller) accessToken(ctx context.Context, targetID string) (string, Failure) {
	refreshToken, err := p.store.RefreshToken(ctx, targetID)
	if err != nil {
		return "", FailureState
	}
	accessToken, replacementRefreshToken, refreshErr := p.source.RefreshAccessToken(ctx, refreshToken)
	if refreshErr != nil {
		return "", p.classify(ctx, targetID, refreshErr)
	}
	if replaceErr := p.store.ReplaceRefreshToken(ctx, targetID, replacementRefreshToken); replaceErr != nil {
		return "", FailureState
	}

	return accessToken, FailureNone
}

func (p *Poller) classify(ctx context.Context, targetID string, err error) Failure {
	if !p.source.IsUnauthorized(err) {
		// The source's errors are protocol-level — a status, a rate-limit
		// sentinel, a transport failure — never a ride's name or a credential.
		slog.Warn("activity poll failed", "target", targetID, "error", err)

		return FailureUpstream
	}
	// Logged before the mark, and worded as the rejection rather than the mark,
	// so a store that then refuses the write still leaves the cause on record.
	slog.Error("activity source rejected the target authorization", "target", targetID, "error", err)
	if markErr := p.store.MarkNeedsReauthorization(ctx, targetID); markErr != nil {
		return FailureState
	}

	return FailureAuthorization
}

// unstored is the oldest activities the store has not seen, at most
// MaxNewPerPoll of them. Oldest first so an account with a long history fills in
// chronologically over successive polls rather than restarting each time.
func unstored(listings []Listing, known []int64) []Listing {
	seen := make(map[int64]struct{}, len(known))
	for _, id := range known {
		seen[id] = struct{}{}
	}
	pending := make([]Listing, 0, len(listings))
	for _, listing := range listings {
		if _, ok := seen[listing.ID]; !ok {
			pending = append(pending, listing)
		}
	}
	slices.SortFunc(pending, func(a, b Listing) int {
		if !a.Starts.Equal(b.Starts) {
			return a.Starts.Compare(b.Starts)
		}

		return cmp.Compare(a.ID, b.ID)
	})

	return pending[:min(len(pending), MaxNewPerPoll)]
}
