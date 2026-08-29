package wahoo

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

func (c *Client) observeRateLimit(response *http.Response) {
	remaining, reset, ok := rateLimit(response.Header)
	if !ok {
		return
	}
	c.rateLimitKnown = true
	c.rateLimitRemaining = remaining
	// Reset stays whatever it last usefully was when this response carries an
	// unknown one, rather than jumping to zero: this field is read for display,
	// where a reset that briefly disappears must not read as "already reset".
	if reset > 0 {
		c.rateLimitResetAt = c.now().Add(reset)
	}
	if remaining > 0 {
		return
	}
	// A spent quota with no usable reset still has to be waited out. Treating
	// an unknown reset as "no need to wait" is what previously left this
	// throttle permanently unarmed — Wahoo reports seconds_until_reset as 0 on
	// every response that was not itself limited, so the guard never passed and
	// a run kept spending until it was refused outright.
	if reset <= 0 {
		reset = defaultRateLimitReset
	}
	c.notBefore = c.now().Add(reset)
}

// RateLimit reports the lowest request quota Wahoo advertised across the
// windows on its most recent response, and when that quota is next expected to
// refill. ok is false until the client has made a request that carried a
// quota header, which is the honest state for a service that has not spoken to
// Wahoo yet.
//
// resetAt is zero whenever it would already be in the past: a response that
// was not itself limited can leave a stale reset in place (see
// observeRateLimit), and this is where that staleness stops rather than
// reaching a caller as a refill time that has already gone by.
func (c *Client) RateLimit() (remaining int, resetAt time.Time, ok bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	resetAt = c.rateLimitResetAt
	if !resetAt.IsZero() && !resetAt.After(c.now()) {
		resetAt = time.Time{}
	}

	return c.rateLimitRemaining, resetAt, c.rateLimitKnown
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
// quota to refill. It is request-scoped rather than a client setting because
// it is a property of what is being asked for, not of who is asking: the same
// client serves both a scheduled run that must not stall and a clear somebody
// is waiting on.
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
