// Package wahoo speaks Wahoo's OAuth, route, and activity APIs.
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

	// maximumRoutes bounds a route listing. Wahoo returns the account's routes in
	// one response — `page` is accepted and ignored — so there is no paging to walk.
	// This refuses a response too large to be the library it claims to be: a
	// truncated listing would read as "these routes do not exist" and duplicate them.
	maximumRoutes = 10_000

	// defaultRateLimitReset is how long to hold off when Wahoo says a quota is
	// spent but not when it refills. It answers 0 on a response that was not itself
	// limited, so zero means unknown rather than already reset.
	defaultRateLimitReset = 5 * time.Minute

	// maximumRateLimitWait bounds how long one request holds a run open waiting for
	// quota. Beyond it the run ends and reports the limit: reconciliation records
	// each stage as it succeeds, so the next run resumes from stored state.
	maximumRateLimitWait = 90 * time.Second

	// maximumPatientRateLimitWait is the same bound for an operation somebody is
	// waiting on directly. Clearing a target is the only one: it is finished only
	// when the target is empty. It exceeds the smallest advertised window, so one
	// refill is always reachable.
	maximumPatientRateLimitWait = 6 * time.Minute
)

var (
	// ErrUnauthorized reports an authorization code or refresh token the Wahoo
	// token endpoint rejected. A rejected data request is never classified so.
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
