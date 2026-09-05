package wahoo

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"golang.org/x/oauth2"
)

// classifyTokenError maps x/oauth2's failures onto this package's sentinels.
// Wahoo answers a spent or withdrawn refresh token with 400 rather than 401,
// and sync reads ErrUnauthorized to know a target needs reauthorizing.
func classifyTokenError(err error) error {
	var retrieve *oauth2.RetrieveError
	if errors.As(err, &retrieve) && retrieve.Response != nil {
		switch retrieve.Response.StatusCode {
		case http.StatusBadRequest, http.StatusUnauthorized:
			return fmt.Errorf("wahoo: token request rejected with HTTP %d: %w", retrieve.Response.StatusCode, ErrUnauthorized)
		case http.StatusTooManyRequests:
			return ErrRateLimited
		}
	}
	if errors.Is(err, ErrRateLimited) {
		return ErrRateLimited
	}

	return fmt.Errorf("wahoo: token request failed: %w", err)
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

// RefreshAccessToken obtains a replacement access and refresh token immediately
// before a Wahoo API request.
func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (accessToken, newRefreshToken string, err error) {
	if refreshToken == "" {
		return "", "", errors.New("wahoo: refresh token is required")
	}

	// With no access token to reuse, the source always refreshes.
	token, err := c.oauth.
		TokenSource(c.oauthContext(ctx), &oauth2.Token{RefreshToken: refreshToken}).
		Token()
	if err != nil {
		return "", "", classifyTokenError(err)
	}

	return tokenPair(token)
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
