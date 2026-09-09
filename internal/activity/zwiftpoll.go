package activity

import (
	"context"
	"errors"
	"log/slog"
	"time"
)

// ZwiftPagesPerPoll bounds how far back into a rider's history one poll reads.
// The listing is newest first and a poll stops at the first page holding
// nothing new, so this only bounds the first read of a long history.
const ZwiftPagesPerPoll = 5

// ZwiftListingPageSize is how many activities one listing request asks for.
const ZwiftListingPageSize = 30

// ZwiftSource is the rider's Zwift account in this package's own vocabulary.
// Signing in yields a reader for the length of one poll; the grant behind it is
// never handed to a store.
type ZwiftSource interface {
	SignIn(ctx context.Context, email, password []byte) (ZwiftReader, error)
	// DownloadActivityFIT reads the file a stored summary names. It takes no
	// reader: the file is a public object the grant plays no part in.
	DownloadActivityFIT(ctx context.Context, summary Summary) ([]byte, error)
	IsUnauthorized(err error) bool
	// IsUnreadable reports a refusal that belongs to one activity or its file
	// rather than to the connection, the quota or the grant.
	IsUnreadable(err error) bool
}

// ZwiftReader reads one signed-in rider's own activities.
type ZwiftReader interface {
	// ListActivities returns one page of the rider's activities, newest first,
	// narrowed to the rides this service records and each carrying its summary,
	// and how many entries the page held before narrowing. A page that held
	// none is the end of the list.
	ListActivities(ctx context.Context, start, limit int) (listings []Listing, held int, err error)
}

// ZwiftStore is the durable state one Zwift poll reads and writes. Unlike the
// Wahoo Store it deletes: an indoor ride Zwift recorded replaces the head
// unit's copy of the same ride.
type ZwiftStore interface {
	TargetOwner(ctx context.Context, targetID string) (string, error)
	// RiderZwiftCredentials are the rider's own Zwift email and password, each
	// empty when it has not been entered.
	RiderZwiftCredentials(ctx context.Context, subject string) (email, password []byte, err error)
	KnownActivityIDs(ctx context.Context, targetID string) ([]int64, error)
	StoreActivity(ctx context.Context, targetID string, listing Listing, summary Summary, now time.Time) error
	// DeleteTrainerCopy removes the Wahoo activities that started within window
	// of at, and reports how many went.
	DeleteTrainerCopy(ctx context.Context, targetID string, at time.Time, window time.Duration) (int, error)
	ActivitiesAwaitingRecords(ctx context.Context, targetID, provider string,
		recordsVersion, limit int) ([]PendingActivity, error)
	StoreActivityRecords(ctx context.Context, targetID string, id int64, fit FIT, recordsVersion int) error
	MarkActivityUnreadable(ctx context.Context, targetID string, id int64) error
}

// ZwiftPoller reads one target owner's own Zwift rides into the store.
type ZwiftPoller struct {
	source ZwiftSource
	store  ZwiftStore
	now    func() time.Time
}

// NewZwiftPoller builds a Zwift poller over its source and store.
func NewZwiftPoller(source ZwiftSource, store ZwiftStore, now func() time.Time) (*ZwiftPoller, error) {
	if source == nil || store == nil || now == nil {
		return nil, errors.New("activity: a source, a store and a clock are required")
	}

	return &ZwiftPoller{source: source, store: store, now: now}, nil
}

// Poll stores every ride of one target's owner that their Zwift account has
// recorded and this service has not, newest first, and fills the samples of
// those stored.
//
// A rider who has entered no Zwift credentials is not ready rather than failed:
// most riders have none, and there is nothing to poll for them. A refused
// grant marks nothing for re-authorization — there is no OAuth to redo, the
// rider re-enters a password.
func (p *ZwiftPoller) Poll(ctx context.Context, targetID string) Result {
	subject, err := p.store.TargetOwner(ctx, targetID)
	if err != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	if subject == "" {
		return Result{Outcome: NotReady}
	}
	email, password, credentialsErr := p.store.RiderZwiftCredentials(ctx, subject)
	if credentialsErr != nil {
		return Result{Outcome: Failed, Failure: FailureState}
	}
	if len(email) == 0 || len(password) == 0 {
		return Result{Outcome: NotReady}
	}
	reader, signInErr := p.source.SignIn(ctx, email, password)
	if signInErr != nil {
		return Result{Outcome: Failed, Failure: p.classify(targetID, signInErr)}
	}

	stored, failure := p.storeNew(ctx, targetID, reader)
	if failure != FailureNone {
		return Result{Outcome: Failed, Failure: failure, Stored: stored}
	}
	records, unreadable, failure := p.fillRecords(ctx, targetID)
	result := Result{Stored: stored, RecordsStored: records, RecordsUnreadable: unreadable}
	if failure != FailureNone {
		result.Outcome, result.Failure = Failed, failure

		return result
	}
	if stored == 0 && records == 0 && unreadable == 0 {
		return Result{Outcome: Unchanged}
	}
	slog.Info("zwift activities polled", "target", targetID,
		"stored", stored, "records", records, "unreadable", unreadable)
	result.Outcome = Polled

	return result
}

// storeNew stores every listed ride the target does not hold, page by page from
// the newest, stopping at the first page that held nothing new.
func (p *ZwiftPoller) storeNew(ctx context.Context, targetID string, reader ZwiftReader) (stored int, failure Failure) {
	ids, err := p.store.KnownActivityIDs(ctx, targetID)
	if err != nil {
		return 0, FailureState
	}
	known := identities(ids, nil)
	for page := range ZwiftPagesPerPoll {
		listings, held, listErr := reader.ListActivities(ctx, page*ZwiftListingPageSize, ZwiftListingPageSize)
		if listErr != nil {
			return stored, p.classify(targetID, listErr)
		}
		if held == 0 {
			return stored, FailureNone
		}
		fresh := 0
		for _, listing := range listings {
			if _, seen := known[listing.ID]; seen || listing.Summary == nil {
				continue
			}
			fresh++
			if storeErr := p.storeOne(ctx, targetID, listing); storeErr != FailureNone {
				return stored, storeErr
			}
			stored++
		}
		if fresh == 0 {
			return stored, FailureNone
		}
	}

	return stored, FailureNone
}

// storeOne records one ride, first removing the head unit's copy of it: the two
// are one ride, and the Zwift file is the one kept for the power it carries.
func (p *ZwiftPoller) storeOne(ctx context.Context, targetID string, listing Listing) Failure {
	removed, err := p.store.DeleteTrainerCopy(ctx, targetID, listing.Starts, trainerCopyWindow)
	if err != nil {
		return FailureState
	}
	if removed > 0 {
		slog.Info("removed a trainer copy of an indoor ride", "target", targetID, "removed", removed)
	}
	if err := p.store.StoreActivity(ctx, targetID, listing, *listing.Summary, p.now()); err != nil {
		return FailureState
	}

	return FailureNone
}

// fillRecords downloads and decodes the FIT file of each stored Zwift ride
// whose samples are absent or predate RecordsVersion, under the same budget the
// Wahoo poll's fill phase spends.
func (p *ZwiftPoller) fillRecords(ctx context.Context, targetID string) (stored, unreadable int, failure Failure) {
	pending, err := p.store.ActivitiesAwaitingRecords(ctx, targetID, ProviderZwift, RecordsVersion, MaxRecordsPerPoll)
	if err != nil {
		return 0, 0, FailureState
	}
	deadline := p.now().Add(RecordsBudgetPerPoll)
	for _, ride := range pending {
		if !p.now().Before(deadline) {
			break
		}
		raw, downloadErr := p.source.DownloadActivityFIT(ctx, ride.Summary)
		if downloadErr != nil && !p.unreadableFile(downloadErr) {
			return stored, unreadable, p.classify(targetID, downloadErr)
		}
		decoded, decodeErr := FIT{}, downloadErr
		if decodeErr == nil {
			decoded, decodeErr = DecodeFIT(raw)
		}
		if decodeErr != nil {
			if markErr := p.store.MarkActivityUnreadable(ctx, targetID, ride.ID); markErr != nil {
				return stored, unreadable, FailureState
			}
			unreadable++

			continue
		}
		if storeErr := p.store.StoreActivityRecords(ctx, targetID, ride.ID, decoded, RecordsVersion); storeErr != nil {
			return stored, unreadable, FailureState
		}
		stored++
	}

	return stored, unreadable, FailureNone
}

// unreadableFile reports a download refusal that belongs to that one file.
func (p *ZwiftPoller) unreadableFile(err error) bool {
	return errors.Is(err, ErrNoActivityFile) || p.source.IsUnreadable(err)
}

// classify names a failure without ever marking the target for
// re-authorization: a Zwift grant is a password the rider re-enters, not an
// authorization this service can send them back to.
func (p *ZwiftPoller) classify(targetID string, err error) Failure {
	if p.source.IsUnauthorized(err) {
		slog.Error("zwift rejected the rider credentials", "target", targetID, "error", err)

		return FailureAuthorization
	}
	slog.Warn("zwift poll failed", "target", targetID, "error", err)

	return FailureUpstream
}
