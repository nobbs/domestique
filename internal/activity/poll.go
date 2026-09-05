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
}

// Store is the durable state one poll reads and adds to. It never deletes.
type Store interface {
	TargetAuthorization(ctx context.Context, targetID string) (string, error)
	RefreshToken(ctx context.Context, targetID string) (string, error)
	ReplaceRefreshToken(ctx context.Context, targetID, refreshToken string) error
	MarkNeedsReauthorization(ctx context.Context, targetID string) error
	KnownActivityIDs(ctx context.Context, targetID string) ([]int64, error)
	StoreActivity(ctx context.Context, targetID string, listing Listing, summary Summary, now time.Time) error
}

// Outcome is what one poll came to. Its zero value is Failed, so a result
// nobody filled in never reads as work that succeeded.
type Outcome int

const (
	// Failed means the poll stopped early; whatever it stored before that stays.
	Failed Outcome = iota
	// Polled means at least one new activity was stored.
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
// part way keeps what it already stored: the next poll continues from there.
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

	pending := unstored(listings, known)
	if len(pending) == 0 {
		return Result{Outcome: Unchanged}
	}

	var stored int
	for _, listing := range pending {
		summary, summaryErr := p.source.ActivitySummary(ctx, accessToken, listing.ID)
		if summaryErr != nil {
			return Result{Outcome: Failed, Failure: p.classify(ctx, targetID, summaryErr), Stored: stored}
		}
		if storeErr := p.store.StoreActivity(ctx, targetID, listing, summary, p.now()); storeErr != nil {
			return Result{Outcome: Failed, Failure: FailureState, Stored: stored}
		}
		stored++
	}
	slog.Info("activities polled", "target", targetID, "stored", stored)

	return Result{Outcome: Polled, Stored: stored}
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
	// Losing the grant is durable and needs an operator, so it is recorded at
	// error level; the wrapped status says which endpoint class refused it.
	slog.Error("activity target marked for reauthorization", "target", targetID, "error", err)
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
