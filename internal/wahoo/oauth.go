package wahoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

// tokenExpiryMargin retires a held access token early, so one handed to a caller
// cannot expire part-way through the run that asked for it.
const tokenExpiryMargin = 2 * time.Minute

// classifyTokenError maps x/oauth2's failures onto this package's sentinels.
// Wahoo answers a spent or withdrawn refresh token with 400 rather than 401,
// and sync reads ErrUnauthorized to know a target needs reauthorizing.
func classifyTokenError(err error) error {
	if retrieve, ok := errors.AsType[*oauth2.RetrieveError](err); ok {
		return classifyRetrieveError(retrieve)
	}
	if errors.Is(err, ErrRateLimited) {
		return ErrRateLimited
	}

	return fmt.Errorf("wahoo: token request failed: %w", err)
}

// classifyRetrieveError reports the status and this package's own category, and
// never wraps the error it was given. x/oauth2's Error quotes the reply — the
// body verbatim, or the strings it parsed out of it — and sync logs a refusal it
// cannot classify, so wrapping it would put the token endpoint's reply in a log.
func classifyRetrieveError(retrieve *oauth2.RetrieveError) error {
	category := tokenErrorCategory(retrieve.Body)
	if retrieve.Response == nil {
		// Said as the absence it is. A status of zero reads as one Wahoo sent.
		return &tokenError{category: category, err: errors.New("wahoo: token request failed without an HTTP response")}
	}
	switch retrieve.Response.StatusCode {
	case http.StatusBadRequest, http.StatusUnauthorized:
		return &tokenError{
			category: category,
			err: fmt.Errorf("wahoo: token request rejected with HTTP %d: %w",
				retrieve.Response.StatusCode, ErrUnauthorized),
		}
	case http.StatusTooManyRequests:
		return &tokenError{category: "rate_limited", err: ErrRateLimited}
	}

	return &tokenError{
		category: category,
		err:      fmt.Errorf("wahoo: token request failed with HTTP %d", retrieve.Response.StatusCode),
	}
}

// tokenError names why the token endpoint refused, without becoming the reason
// itself: it unwraps to the sentinel callers already match on.
type tokenError struct {
	err      error
	category string
}

func (e *tokenError) Error() string    { return e.err.Error() }
func (e *tokenError) Unwrap() error    { return e.err }
func (e *tokenError) Category() string { return e.category }

// tokenErrorCategory maps a refusal onto this package's own vocabulary. Wahoo
// does not keep to the OAuth error codes — its exhausted-quota refusal puts a
// whole English sentence in `error` — so a reply this does not recognise is
// reported as unrecognised rather than quoted onward.
func tokenErrorCategory(body []byte) string {
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "unrecognised"
	}
	switch payload.Error {
	case "invalid_grant", "invalid_client", "invalid_request", "invalid_scope", "unauthorized_client":
		return payload.Error
	}
	// The one refusal an operator cannot act on without being told: Wahoo issues
	// no further token for this account until the existing ones are revoked, and
	// says so only here.
	if strings.Contains(payload.Error, "unrevoked access tokens") {
		return "token_quota_exhausted"
	}

	return "unrecognised"
}

// AuthorizationURL returns a confidential-client Wahoo authorization URL for a
// one-time opaque state value.
func (c *Client) AuthorizationURL(state string) (string, error) {
	if state == "" {
		return "", errors.New("wahoo: oauth state is required")
	}

	return c.oauth.AuthCodeURL(state), nil
}

// ExchangeAuthorizationCode trades a Wahoo authorization code for fresh tokens.
func (c *Client) ExchangeAuthorizationCode(ctx context.Context, code string) (accessToken, refreshToken string, err error) {
	if code == "" {
		return "", "", errors.New("wahoo: authorization code is required")
	}

	token, err := c.oauth.Exchange(c.oauthContext(ctx), code)
	if err != nil {
		return "", "", classifyTokenError(err)
	}

	return tokenPair(token)
}

// RefreshAccessToken returns the access token this client already holds for an
// account, and asks Wahoo for one only when it holds none or the one it holds is
// about to expire.
//
// Reuse is a correctness requirement, not an optimisation. Wahoo caps how many
// unrevoked access tokens may exist for one application and user, and offers no
// way to revoke a single token — only a deauthorization that revokes every token
// the application holds for everyone. A token minted per run and per poll
// therefore fills that cap and locks the account out of authorizing at all.
func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error) {
	if refreshToken == "" {
		return "", "", errors.New("wahoo: refresh token is required")
	}

	// Held across the request, so two callers cannot each mint a token for the
	// same account. This is a different lock from the rate-limit mutex the
	// transport takes, and is only ever acquired before it.
	c.tokenMutex.Lock()
	defer c.tokenMutex.Unlock()

	if held, ok := c.held[refreshToken]; ok {
		if c.now().Add(tokenExpiryMargin).Before(held.expiry) {
			return held.accessToken, refreshToken, nil
		}
		// Dropped before the refresh rather than after a successful one. Nothing
		// will reuse a spent token, and a refresh that keeps failing — a grant
		// the rider withdrew — would otherwise hold it for the process's life.
		delete(c.held, refreshToken)
	}

	token, err := c.oauth.
		TokenSource(c.oauthContext(ctx), &oauth2.Token{RefreshToken: refreshToken}).
		Token()
	if err != nil {
		return "", "", classifyTokenError(err)
	}
	access, rotated, err := tokenPair(token)
	if err != nil {
		return "", "", err
	}
	c.hold(rotated, access, token.Expiry)

	return access, rotated, nil
}

// hold records a freshly obtained token under the refresh token that now reaches
// it — which is a new key whenever Wahoo rotated. A reply without an expiry is
// not held: this client cannot know when it would stop working.
func (c *Client) hold(rotated, accessToken string, expiry time.Time) {
	if expiry.IsZero() {
		return
	}
	if c.held == nil {
		c.held = make(map[string]heldToken, 1)
	}
	c.held[rotated] = heldToken{accessToken: accessToken, expiry: expiry}
}

func (c *Client) oauthContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauth2.HTTPClient, c.oauthClient)
}

// tokenPair enforces that Wahoo returned both halves. x/oauth2 carries the old
// refresh token forward when a reply omits one, which would otherwise read as a
// rotation that happened.
func tokenPair(token *oauth2.Token) (accessToken, refreshToken string, err error) {
	if token == nil || token.AccessToken == "" || token.RefreshToken == "" {
		return "", "", errors.New("wahoo: token response was incomplete")
	}

	return token.AccessToken, token.RefreshToken, nil
}

// AuthenticatedUser returns the stable Wahoo user identity for an access token.
func (c *Client) AuthenticatedUser(ctx context.Context, accessToken string) (string, error) {
	if accessToken == "" {
		return "", errors.New("wahoo: access token is required")
	}

	request, err := c.newRequest(ctx, http.MethodGet, c.endpoint(c.apiBaseURL, "/v1/user"), http.NoBody, accessToken)
	if err != nil {
		return "", err
	}
	var response struct {
		ID int64 `json:"id"`
	}
	if err := c.doJSON(request, &response); err != nil {
		return "", err
	}
	if response.ID <= 0 {
		return "", errors.New("wahoo: user response did not contain an id")
	}

	return strconv.FormatInt(response.ID, 10), nil
}

func parseCallbackURL(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "/oauth/wahoo/callback" {
		return nil, errors.New("must be an absolute https oauth callback url")
	}

	return parsed, nil
}
