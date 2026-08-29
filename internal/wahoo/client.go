// Package wahoo speaks Wahoo's OAuth and route APIs.
package wahoo

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const (
	defaultTimeout   = 30 * time.Second
	maximumBodyBytes = 1 << 20
	earthRadiusMetre = 6_371_000.0

	// maximumRoutes bounds a route listing. Wahoo returns the account's routes
	// in one response — verified against a live account, where `page` is
	// accepted and then ignored, every page answering with the same complete
	// set — so there is no paging to walk and no page count to bound. This
	// instead refuses a response too large to be the library it claims to be,
	// because a silently truncated listing would read as "these routes do not
	// exist" and create duplicates of them.
	maximumRoutes = 10_000

	// defaultRateLimitReset is how long to hold off when Wahoo says a quota is
	// spent but not when it refills. It answers 0 for seconds_until_reset on a
	// response that was not itself limited, so a zero there means "unknown"
	// rather than "already reset"; the smallest advertised window is the
	// conservative reading.
	defaultRateLimitReset = 5 * time.Minute

	// maximumRateLimitWait bounds how long one request holds a run open waiting
	// for quota. Beyond it the run ends and reports the limit rather than
	// sleeping: reconciliation records each stage as it succeeds, so the next
	// run resumes from stored state. That makes progress in bounded steps
	// instead of holding a single run open for hours.
	maximumRateLimitWait = 90 * time.Second

	// maximumPatientRateLimitWait is the same bound for an operation somebody
	// is waiting on directly rather than one the schedule started. Clearing a
	// target is the only such operation: it was asked for once, it is finished
	// only when the target is empty, and asking an operator to press the same
	// destructive button five times to get there would be worse than the wait.
	// It exceeds the smallest advertised window so one refill is always
	// reachable.
	maximumPatientRateLimitWait = 6 * time.Minute
)

var (
	// ErrUnauthorized reports a rejected Wahoo authorization or refresh token.
	ErrUnauthorized = errors.New("wahoo authorization was rejected")
	// ErrRateLimited reports a request rejected by Wahoo's advertised rate limit.
	ErrRateLimited = errors.New("wahoo rate limit was reached")
)

// Options configures a Wahoo API client with resolved OAuth credentials.
type Options struct {
	Transport    http.RoundTripper
	APIBaseURL   string
	OAuthBaseURL string
	ClientID     string
	RedirectURL  string
	ClientSecret []byte
	Timeout      time.Duration
}

// Client is a serial Wahoo API client with shared rate-limit handling.
type Client struct {
	rateLimitResetAt   time.Time
	notBefore          time.Time
	client             *http.Client
	apiBaseURL         *url.URL
	oauthBaseURL       *url.URL
	now                func() time.Time
	wait               func(context.Context, time.Duration) error
	oauth              *oauth2.Config
	oauthClient        *http.Client
	rateLimitRemaining int
	mutex              sync.Mutex
	rateLimitKnown     bool
}

// IsUnauthorized reports whether err is a permanent Wahoo authorization
// rejection. Consumers use it to request an interactive reauthorization.
func (c *Client) IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}
