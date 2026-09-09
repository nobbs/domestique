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

// RequestsPerPoll is the counted requests one poll may make: once the listing
// requests and one request per summary the listing did not carry reach it, the
// poll asks for no more summaries. The Wahoo sandbox tier allows 25 requests
// per five minutes, 100 per hour and 250 per day, shared across every target
// and task; this is the five-minute window, the tightest. A listing whose pages
// alone exceed it is the source's own throttle to pace, not this cap's (#489).
const RequestsPerPoll = 25

// RecordsBudgetPerPoll is the wall-clock after which one poll starts no further
// fill; the fill under way finishes, so the exclusive activities resource is held
// that one ride past it. A row count models rides of 3,000 to 12,000 samples poorly.
const RecordsBudgetPerPoll = 2 * time.Minute

// MaxRecordsPerPoll is the hard ceiling on activities one poll fills, under the
// budget: a clock that never advances still cannot leave one run looping.
const MaxRecordsPerPoll = 200

// RecordsVersion is the record schema fromRecord decodes under. Bump it
// whenever Record gains a field a stored ride's samples must be re-read to
// carry; a poll re-reads every stored ride below this version, at the same
// pace its first download took.
//
// 2: the file's session figures and totals.
// 3: the target power a structured workout prescribed per record.
const RecordsVersion = 3

// The upstreams a recorded activity is read from. A provider is a label on a
// stored ride, never a guard: what a ride is asked about follows its workout
// type, so a Wahoo trainer ride and a Zwift one are treated alike.
const (
	ProviderWahoo = "wahoo"
	ProviderZwift = "zwift"
)

// trainerCopyWindow is how close two starts must be for one to be the head
// unit's copy of an indoor ride Zwift also recorded. One ride, two recorders
// started by hand seconds apart.
const trainerCopyWindow = time.Minute

// Listing is one recorded activity as the rider's account lists it.
type Listing struct {
	Starts time.Time
	// Summary is the summary the account's listing itself carried, or nil. Only
	// a fresh reading of the account carries one; the store keeps listings
	// without it.
	Summary *Summary
	// Provider is which upstream listed this activity. Empty means Wahoo, the
	// only provider that predates the column.
	Provider   string
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
// retry spends one request from a window that fits RequestsPerPoll.
func retryDue(skip Skip, now time.Time) bool {
	doublings := min(max(skip.Attempts, 1), 6) - 1
	wait := min(skipRetryBase<<doublings, skipRetryCap)

	return !now.Before(skip.LastAttempt.Add(wait))
}

// ErrNoActivityFile reports a stored summary that names no readable file. It is
// the activity's own fault, not the provider's, so a poll marks it unreadable.
var ErrNoActivityFile = errors.New("activity: the summary names no file")

// PendingActivity is one stored activity whose FIT records are still absent,
// carrying the provider's summary document so its file can be found again.
type PendingActivity struct {
	Summary Summary
	ID      int64
}

// Stored is one recorded activity as the read model serves it: the listing and
// the summary totals, without the provider's own summary document.
type Stored struct {
	StartedAt time.Time
	// Provider is which upstream recorded this ride.
	Provider string
	// WorkoutName, WorkoutHash and WorkoutCompletion are the structured
	// workout a Zwift ride carried, all zero when HasWorkout is false.
	WorkoutName       string
	ID                int64
	WorkoutHash       int64
	DistanceMetres    float64
	MovingSeconds     float64
	ElapsedSeconds    float64
	AscentMetres      float64
	WorkoutCompletion float64
	TypeID            int
	LocationID        int
	HasWorkout        bool
}

// RecordsState is how far one activity's recorded samples have got: awaiting a
// download, stored, or given up on because the file did not decode.
type RecordsState string

// The three states the store keeps, and the values it keeps them as.
const (
	RecordsPending    RecordsState = "pending"
	RecordsStored     RecordsState = "stored"
	RecordsUnreadable RecordsState = "unreadable"
)

// TrackPoint is one positioned sample of a recorded ride. Altitude is optional
// because a FIT record may carry a position without one.
type TrackPoint struct {
	Time           time.Time
	Latitude       float64
	Longitude      float64
	AltitudeMetres float64
	// EstimatedPowerWatts is the power this service worked out from the track
	// itself, for a bicycle carrying no meter. Never a measurement, and served
	// under its own name so nothing can mistake it for one.
	EstimatedPowerWatts float64
	HasAltitude         bool
	HasEstimatedPower   bool
}

// Source is the rider's activity provider, in this package's own vocabulary.
type Source interface {
	RefreshAccessToken(ctx context.Context, refreshToken string) (accessToken, replacementRefreshToken string, err error)
	// ActivityListingHead returns the account's first page of activities and how
	// many it holds in all, at the cost of one request.
	ActivityListingHead(ctx context.Context, accessToken string) (listings []Listing, total int, err error)
	// ListActivities reads the account's whole list and reports how many
	// requests that cost, so a poll can keep the rest within its window.
	ListActivities(ctx context.Context, accessToken string) (listings []Listing, requests int, err error)
	// Activity reads one activity's listing entry, carrying the summary that
	// entry itself held, at the cost of one request.
	Activity(ctx context.Context, accessToken string, id int64) (Listing, error)
	ActivitySummary(ctx context.Context, accessToken string, id int64) (Summary, error)
	DownloadActivityFIT(ctx context.Context, summary Summary) ([]byte, error)
	IsUnauthorized(err error) bool
	// IsRecordable reports whether a listed activity is one this service
	// records at all. A listing it refuses is still counted as the account's,
	// so a reading stays comparable with the account's own total.
	IsRecordable(listing Listing) bool
	// IsUnreadable reports a summary rejection that belongs to that one
	// activity rather than to the connection, the quota or the grant.
	IsUnreadable(err error) bool
	// IsRejected reports a refusal that belongs to the connection rather than to
	// the activity asked for, and so would meet every request after it.
	IsRejected(err error) bool
}

// Store is the durable state one poll reads and adds to. It never deletes.
type Store interface {
	TargetAuthorization(ctx context.Context, targetID string) (string, error)
	RefreshToken(ctx context.Context, targetID string) (string, error)
	ReplaceRefreshToken(ctx context.Context, targetID, refreshToken string) error
	MarkNeedsReauthorization(ctx context.Context, targetID string) error
	// StoreActivity also forgets any skip recorded for the same activity. It
	// leaves the kept listings alone: those mirror the account, not what is
	// left to read.
	StoreActivity(ctx context.Context, targetID string, listing Listing, summary Summary, now time.Time) error
	ActivitySkips(ctx context.Context, targetID string) ([]Skip, error)
	// RecordActivitySkip counts one more failed read of an activity. observed is
	// the source's protocol-level error text, never a ride's name or a credential.
	RecordActivitySkip(ctx context.Context, targetID string, id int64, observed string, now time.Time) error
	listingStore
	recordStore
}

// listingStore is what a poll reads to know which of the account's activities
// it has yet to store, kept apart so neither half grows past what one reader
// can hold.
type listingStore interface {
	KnownActivityIDs(ctx context.Context, targetID, provider string) ([]int64, error)
	// IndoorRideStarts are the start times of the target's stored Zwift rides,
	// which is what tells a Wahoo listing that is the head unit's copy of one.
	IndoorRideStarts(ctx context.Context, targetID string) ([]time.Time, error)
	// ActivityStored reports one activity's presence without listing the rest.
	ActivityStored(ctx context.Context, targetID string, id int64, provider string) (bool, error)
	// ActivityListings are the activities the account holds, oldest first, as
	// the last full reading of it left them, and when that reading was taken.
	// The order is what a poll fills from; it does not sort them again.
	ActivityListings(ctx context.Context, targetID string) (listings []Listing, readAt time.Time, err error)
	// ReplaceActivityListings makes the kept listings exactly these, read now.
	ReplaceActivityListings(ctx context.Context, targetID string, listings []Listing, now time.Time) error
}

// recordStore is the records phase's half of the store, kept apart so neither
// half grows past what one reader can hold.
type recordStore interface {
	// ActivitiesAwaitingRecords are the stored activities whose samples are still
	// absent, newest first, followed by the stored activities whose samples
	// predate recordsVersion, also newest first, so a fresh ride never waits
	// behind a backfill and a schema re-read never outpaces the first download.
	// Only one provider's rides: each poller downloads from the upstream that
	// recorded them, and a summary belongs to the shape that wrote it.
	ActivitiesAwaitingRecords(ctx context.Context, targetID, provider string,
		recordsVersion, limit int) ([]PendingActivity, error)
	StoreActivityRecords(ctx context.Context, targetID string, id int64, fit FIT, recordsVersion int) error
	MarkActivityUnreadable(ctx context.Context, targetID string, id int64) error
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
	// FailureRejected means the source refused the request itself rather than the
	// activity asked for. Told apart from FailureUpstream because it applies to
	// every request the poll would make next, and from FailureAuthorization
	// because the target's grant is not what refused it.
	FailureRejected Failure = "rejected"
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
	// RecordsStored counts the activities whose FIT samples this poll wrote.
	RecordsStored int
	// RecordsUnreadable counts the activities this poll marked as having no
	// readable FIT file; a mark is a change, so such a poll is not unchanged.
	RecordsUnreadable int
	// Derived counts the rides a derivation settled — worked out, or cleared
	// because there was nothing left to work them out from — including those a
	// failed one managed before it stopped.
	Derived int
	// Matched counts the rides a derivation attributed to a library route or
	// recorded as being on none of them.
	Matched int
	// WeatherRead counts the rides a derivation settled the weather of,
	// including those a failed pass managed before it stopped. A ride with
	// nowhere to ask about is settled without a request being spent on it.
	WeatherRead int
}

// Poller reads one target's recorded activities into the store. It adds and
// overwrites; it never removes an activity the account no longer lists.
type Poller struct {
	source Source
	store  Store
	now    func() time.Time
	// indoorTypes are the workout types a head unit's copy of a trainer ride is
	// recorded under; only such a listing can be the copy of a stored Zwift ride.
	indoorTypes []int
}

// NewPoller builds a poller over its source and store.
func NewPoller(source Source, store Store, indoorTypes []int, now func() time.Time) (*Poller, error) {
	if source == nil || store == nil || now == nil || len(indoorTypes) == 0 {
		return nil, errors.New("activity: a source, a store, the indoor workout types and a clock are required")
	}

	return &Poller{source: source, store: store, indoorTypes: indoorTypes, now: now}, nil
}

// Poll stores a summary for every activity of one target this service has not
// stored yet, oldest first: from the listing where it carried one, and by one
// request each for as many of the rest as RequestsPerPoll leaves after the
// listing requests. A read that fails
// part way keeps what it already stored: the next poll continues from there. An
// activity only its own summary rejects is skipped and retried later, so one
// unreadable ride never stops the ones after it. The account's whole list is
// read only when the account no longer matches the last reading of it the store
// kept, or that reading has aged out; otherwise the poll works from those
// listings.
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

	pending, requested, failure := p.pending(ctx, targetID, accessToken)
	if failure != FailureNone {
		return Result{Outcome: Failed, Failure: failure}
	}
	skips, skipsErr := p.store.ActivitySkips(ctx, targetID)
	if skipsErr != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}

	var stored, skipped int
	// Oldest first, which is the order the kept listings already arrive in, so
	// an account with a long history fills in chronologically over successive
	// polls rather than restarting each time.
	waiting := identities(deferred(skips, p.now()), nil)
	for _, listing := range pending {
		var summary Summary
		var summaryErr error
		if listing.Summary != nil {
			summary = *listing.Summary
		} else {
			// A deferred skip only holds back the request it would cost; a
			// summary the listing now carries is stored, and the skip forgotten.
			if _, ok := waiting[listing.ID]; ok || requested >= RequestsPerPoll {
				continue
			}
			requested++
			summary, summaryErr = p.source.ActivitySummary(ctx, accessToken, listing.ID)
		}
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

	records, unreadable, failure := p.fillRecords(ctx, targetID)
	result := Result{Stored: stored, Skipped: skipped, RecordsStored: records, RecordsUnreadable: unreadable}
	if failure != FailureNone {
		result.Outcome, result.Failure = Failed, failure

		return result
	}
	if stored == 0 && skipped == 0 && records == 0 && unreadable == 0 {
		return Result{Outcome: Unchanged}
	}
	slog.Info("activities polled", "target", targetID,
		"stored", stored, "skipped", skipped, "records", records, "unreadable", unreadable)
	result.Outcome = Polled

	return result
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

// fillRecords downloads and decodes the FIT file of each stored activity whose
// samples are still absent, then of each stored activity whose samples predate
// RecordsVersion, both newest first, until RecordsBudgetPerPoll is spent or
// MaxRecordsPerPoll are done. It reports how many it stored and marked unreadable.
func (p *Poller) fillRecords(ctx context.Context, targetID string) (stored, unreadable int, failure Failure) {
	pending, err := p.store.ActivitiesAwaitingRecords(ctx, targetID, ProviderWahoo, RecordsVersion, MaxRecordsPerPoll)
	if err != nil {
		return 0, 0, FailureState
	}
	deadline := p.now().Add(RecordsBudgetPerPoll)

	for _, pendingActivity := range pending {
		if !p.now().Before(deadline) {
			break
		}
		filled, marked, failure := p.fill(ctx, targetID, pendingActivity)
		stored, unreadable = stored+filled, unreadable+marked
		if failure != FailureNone {
			return stored, unreadable, failure
		}
	}

	return stored, unreadable, FailureNone
}

// fill downloads and decodes one stored activity's FIT file and stores its
// samples, or marks the activity unreadable when the file is its own fault.
func (p *Poller) fill(ctx context.Context, targetID string, pending PendingActivity) (stored, unreadable int, failure Failure) {
	raw, downloadErr := p.source.DownloadActivityFIT(ctx, pending.Summary)
	if downloadErr != nil && !errors.Is(downloadErr, ErrNoActivityFile) {
		// A download that failed for anything but this file's own sake stops the
		// phase; the next poll retries it, and nothing is marked here.
		return 0, 0, p.classify(ctx, targetID, downloadErr)
	}
	decoded, decodeErr := FIT{}, downloadErr
	if decodeErr == nil {
		decoded, decodeErr = DecodeFIT(raw)
	}
	if decodeErr != nil {
		if markErr := p.store.MarkActivityUnreadable(ctx, targetID, pending.ID); markErr != nil {
			return 0, 0, FailureState
		}

		return 0, 1, FailureNone
	}
	if storeErr := p.store.StoreActivityRecords(ctx, targetID, pending.ID, decoded, RecordsVersion); storeErr != nil {
		return 0, 0, FailureState
	}

	return 1, 0, FailureNone
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
	// Checked before the grant: a refusal of the request itself stops the poll
	// without concluding anything about the refresh token, which only the token
	// endpoint judges.
	if p.source.IsRejected(err) {
		slog.Warn("activity source refused the request", "target", targetID, "error", err)

		return FailureRejected
	}
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

// pending is the activities the account holds that are not stored, taken from
// the listings the store kept and re-read from the account only when the
// account no longer agrees with them, and how many requests finding them cost.
// What is compared is the account against its own last reading, not against
// what this service has stored: a ride deleted after it was stored leaves the
// store holding more than the account lists, which would otherwise read as a
// disagreement no poll could settle.
func (p *Poller) pending(
	ctx context.Context, targetID, accessToken string,
) (pending []Listing, requests int, failure Failure) {
	listings, readAt, listingsErr := p.store.ActivityListings(ctx, targetID)
	if listingsErr != nil {
		return nil, 0, FailureState
	}
	// A reading that has to be taken again asks the account nothing first: the
	// first page is what decides, and that decision is already made.
	reread := stale(readAt, p.now())
	if !reread {
		requests++
		head, total, headErr := p.source.ActivityListingHead(ctx, accessToken)
		if headErr != nil {
			return nil, requests, p.classify(ctx, targetID, headErr)
		}
		reread = total != len(listings) || !accountedFor(head, listings)
		if !reread {
			listings = carrying(listings, head)
		}
	}
	if reread {
		fresh, listRequests, listErr := p.source.ListActivities(ctx, accessToken)
		requests += listRequests
		if listErr != nil {
			return nil, requests, p.classify(ctx, targetID, listErr)
		}
		slices.SortFunc(fresh, byStart)
		if replaceErr := p.store.ReplaceActivityListings(ctx, targetID, fresh, p.now()); replaceErr != nil {
			return nil, requests, FailureState
		}
		listings = fresh
	}

	known, knownErr := p.store.KnownActivityIDs(ctx, targetID, ProviderWahoo)
	if knownErr != nil {
		return nil, requests, FailureState
	}
	// Read once per poll, and before any summary request: the copy costs no
	// quota, and it is also what stops it being stored again after a Zwift poll
	// removed it — the kept listings go on mirroring the account, correctly.
	starts, startsErr := p.store.IndoorRideStarts(ctx, targetID)
	if startsErr != nil {
		return nil, requests, FailureState
	}

	return dropTrainerCopies(unstored(p.recordable(listings), known), starts, p.indoorTypes), requests, FailureNone
}

// dropTrainerCopies is the listings that are not the head unit's recording of
// an indoor ride already stored from Zwift.
func dropTrainerCopies(listings []Listing, starts []time.Time, indoorTypes []int) []Listing {
	if len(starts) == 0 {
		return listings
	}
	kept := make([]Listing, 0, len(listings))
	for _, listing := range listings {
		if !slices.Contains(indoorTypes, listing.TypeID) {
			kept = append(kept, listing)

			continue
		}
		// starts is sorted, so only the neighbours either side of the listing's
		// own start can fall inside the window.
		next, _ := slices.BinarySearchFunc(starts, listing.Starts, time.Time.Compare)
		isCopy := next < len(starts) && starts[next].Sub(listing.Starts).Abs() <= trainerCopyWindow ||
			next > 0 && listing.Starts.Sub(starts[next-1]).Abs() <= trainerCopyWindow
		if !isCopy {
			kept = append(kept, listing)
		}
	}

	return kept
}

// carrying is the kept listings with the summaries the account's first page
// carried for them, which the store never kept.
func carrying(listings, head []Listing) []Listing {
	summaries := make(map[int64]*Summary, len(head))
	for _, listing := range head {
		if listing.Summary != nil {
			summaries[listing.ID] = listing.Summary
		}
	}
	for index := range listings {
		if listings[index].Summary == nil {
			listings[index].Summary = summaries[listings[index].ID]
		}
	}

	return listings
}

// recordable is the listings this service records, dropped only here: the
// reading kept above is the account's own, and narrowing it there would leave
// the stored count disagreeing with the account's total on every poll.
func (p *Poller) recordable(listings []Listing) []Listing {
	kept := make([]Listing, 0, len(listings))
	for _, listing := range listings {
		if p.source.IsRecordable(listing) {
			kept = append(kept, listing)
		}
	}

	return kept
}

// MaxReadingAge is how long a reading of the account is worked from before it is
// taken again. A change the account's own count cannot show — one activity added
// and another deleted between two polls — is otherwise unnoticed until that count
// next moves, so a poll reads the whole list again this often whatever it holds.
const MaxReadingAge = 7 * 24 * time.Hour

// stale reports whether the kept reading is due to be taken again. A target
// never read has no reading to work from and is always due.
func stale(readAt, now time.Time) bool {
	return readAt.IsZero() || !now.Before(readAt.Add(MaxReadingAge))
}

// accountedFor reports whether the last reading held every activity on the
// account's first page — the bounded side, which is what it keeps in memory.
func accountedFor(head, listings []Listing) bool {
	unseen := identities(nil, head)
	for _, listing := range listings {
		delete(unseen, listing.ID)
		if len(unseen) == 0 {
			return true
		}
	}

	return len(unseen) == 0
}

// unstored is the listings the store has not stored, in the order they came.
func unstored(listings []Listing, known []int64) []Listing {
	seen := identities(known, nil)
	pending := make([]Listing, 0, len(listings))
	for _, listing := range listings {
		if _, ok := seen[listing.ID]; !ok {
			pending = append(pending, listing)
		}
	}

	return pending
}

func byStart(a, b Listing) int {
	if !a.Starts.Equal(b.Starts) {
		return a.Starts.Compare(b.Starts)
	}

	return cmp.Compare(a.ID, b.ID)
}

func identities(ids []int64, listings []Listing) map[int64]struct{} {
	seen := make(map[int64]struct{}, len(ids)+len(listings))
	for _, id := range ids {
		seen[id] = struct{}{}
	}
	for _, listing := range listings {
		seen[listing.ID] = struct{}{}
	}

	return seen
}
