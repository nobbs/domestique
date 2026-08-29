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
	// A spent quota with no usable reset still has to be waited out. Wahoo
	// reports seconds_until_reset as 0 on every response that was not itself
	// limited, so treating unknown as "no wait" leaves this permanently unarmed.
	if reset <= 0 {
		reset = defaultRateLimitReset
	}
	c.notBefore = c.now().Add(reset)
}

// RateLimit reports the lowest request quota Wahoo advertised across the windows
// on its most recent response, and when it is next expected to refill. ok is false
// until a request has carried a quota header. resetAt is zero whenever it would
// already be in the past, so a stale reset never reaches a caller.
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
