package wahoo

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Quota is one reading of Wahoo's advertised request quota. ExpiresAt is when
// the reading stops describing anything, so a restore needs no knowledge of the
// windows behind it; ResetAt and NotBefore are zero when none was observed.
type Quota struct {
	ObservedAt time.Time
	ExpiresAt  time.Time
	ResetAt    time.Time
	NotBefore  time.Time
	Remaining  int
}

// QuotaStore keeps the last observed quota across restarts. It is optional: a
// client without one holds its quota in memory alone.
type QuotaStore interface {
	LoadQuota(ctx context.Context) (Quota, bool, error)
	SaveQuota(ctx context.Context, quota *Quota) error
}

func (c *Client) observeRateLimit(ctx context.Context, response *http.Response) {
	remaining, reset, ok := rateLimit(response.Header)
	if !ok {
		return
	}
	c.rateLimitKnown = true
	c.quotaObservedAt = c.now()
	c.rateLimitRemaining = remaining
	// Reset stays whatever it last usefully was when this response carries an
	// unknown one, rather than jumping to zero: this field is read for display,
	// where a reset that briefly disappears must not read as "already reset".
	if reset > 0 {
		c.rateLimitResetAt = c.now().Add(reset)
	}
	if remaining > 0 {
		c.saveQuota(ctx)

		return
	}
	// A spent quota with no usable reset still has to be waited out. Wahoo
	// reports seconds_until_reset as 0 on every response that was not itself
	// limited, so treating unknown as "no wait" leaves this permanently unarmed.
	if reset <= 0 {
		reset = defaultRateLimitReset
	}
	c.notBefore = c.now().Add(reset)
	c.saveQuota(ctx)
}

// restoreQuota loads the last observed quota, once, on first use: a constructor
// contacts nothing. A row that has expired is discarded rather than honoured,
// which is the whole of what makes a stored reading safe to trust.
func (c *Client) restoreQuota(ctx context.Context) {
	if c.quotaRestored || c.quotaStore == nil {
		return
	}
	c.quotaRestored = true
	// Once only, so the request that happens to come first must not take the
	// restore down with it when it is cancelled; its values still travel.
	loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), quotaStoreTimeout)
	defer cancel()
	quota, found, err := c.quotaStore.LoadQuota(loadCtx)
	if err != nil {
		slog.Warn("wahoo quota state could not be read", "error", err)

		return
	}
	if !found || !quota.ExpiresAt.After(c.now()) {
		return
	}
	c.rateLimitKnown = true
	c.rateLimitRemaining = quota.Remaining
	c.rateLimitResetAt = quota.ResetAt
	c.notBefore = quota.NotBefore
	c.quotaObservedAt = quota.ObservedAt
	c.savedQuota = quota
}

// saveQuota stores an observation that materially changed what the client
// holds, so an ordinary poll does not write per response.
func (c *Client) saveQuota(ctx context.Context) {
	if c.quotaStore == nil {
		return
	}
	quota := Quota{
		ObservedAt: c.quotaObservedAt,
		ExpiresAt:  c.quotaExpiry(),
		ResetAt:    c.rateLimitResetAt,
		NotBefore:  c.notBefore,
		Remaining:  c.rateLimitRemaining,
	}
	if sameQuota(&quota, &c.savedQuota) {
		return
	}
	// The response has arrived; its request being cancelled now must not lose
	// what it said.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), quotaStoreTimeout)
	defer cancel()
	if err := c.quotaStore.SaveQuota(saveCtx, &quota); err != nil {
		slog.Warn("wahoo quota state could not be stored", "error", err)

		return
	}
	c.savedQuota = quota
}

// quotaExpiry is the tightest window this observation could describe: Wahoo's
// shortest published window, extended to whatever later instant it named.
func (c *Client) quotaExpiry() time.Time {
	expiresAt := c.quotaObservedAt.Add(defaultRateLimitReset)
	for _, named := range []time.Time{c.rateLimitResetAt, c.notBefore} {
		if named.After(expiresAt) {
			expiresAt = named
		}
	}

	return expiresAt
}

// quotaExpiryStep is how far the stored expiry may fall behind a fresh
// observation before it is worth a write of its own.
const quotaExpiryStep = time.Minute

// sameQuota compares what a stored row is for: the count, the two instants a
// restore acts on, and an expiry that has not fallen a step behind. A fresh
// observation seconds after the last is not a reason to write again.
func sameQuota(quota, stored *Quota) bool {
	return quota.Remaining == stored.Remaining &&
		quota.ResetAt.Truncate(time.Second).Equal(stored.ResetAt.Truncate(time.Second)) &&
		quota.NotBefore.Truncate(time.Second).Equal(stored.NotBefore.Truncate(time.Second)) &&
		quota.ExpiresAt.Sub(stored.ExpiresAt) < quotaExpiryStep
}

// RateLimit reports the lowest request quota Wahoo advertised across the windows
// on its most recent response, when it is next expected to refill, and when it was
// read. ok is false until a request has carried a quota header, restored readings
// included. resetAt is zero whenever it would already be in the past, so a stale
// reset never reaches a caller.
func (c *Client) RateLimit() (remaining int, resetAt, observedAt time.Time, ok bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	c.restoreQuota(context.Background())
	resetAt = c.rateLimitResetAt
	if !resetAt.IsZero() && !resetAt.After(c.now()) {
		resetAt = time.Time{}
	}

	return c.rateLimitRemaining, resetAt, c.quotaObservedAt, c.rateLimitKnown
}

func rateLimit(header http.Header) (int, time.Duration, bool) {
	remaining, ok := lowestRateLimit(header.Get("X-RateLimit-Remaining"))
	if !ok {
		return 0, 0, false
	}
	// An absent or unreadable reset is not a reason to ignore an exhausted
	// quota. It is reported as zero and the caller supplies its own floor.
	seconds, err := strconv.ParseInt(strings.TrimSpace(header.Get("X-RateLimit-Reset")), 10, 64)
	if err != nil || seconds < 0 {
		seconds = 0
	}

	return remaining, time.Duration(seconds) * time.Second, true
}

func lowestRateLimit(value string) (int, bool) {
	lowest := math.MaxInt
	for item := range strings.SplitSeq(value, ",") {
		remaining, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil || remaining < 0 {
			return 0, false
		}
		lowest = min(lowest, remaining)
	}
	if lowest == math.MaxInt {
		return 0, false
	}

	return lowest, true
}

// quotaPatienceKey marks a context whose requests may wait longer for a spent
// quota to refill. Request-scoped rather than a client setting: the same client
// serves a scheduled run that must not stall and a clear somebody is waiting on.
type quotaPatienceKey struct{}

func withQuotaPatience(ctx context.Context) context.Context {
	return context.WithValue(ctx, quotaPatienceKey{}, true)
}

// waitBudget is how long a request on this context may wait for quota before
// giving up and reporting the limit instead.
func waitBudget(ctx context.Context) time.Duration {
	patient, ok := ctx.Value(quotaPatienceKey{}).(bool)
	if ok && patient {
		return maximumPatientRateLimitWait
	}

	return maximumRateLimitWait
}

func waitFor(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("wahoo: rate limit wait cancelled: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}
