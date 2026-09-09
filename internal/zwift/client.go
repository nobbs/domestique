// Package zwift speaks Zwift's password grant and activity APIs.
package zwift

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultAuthBaseURL is Zwift's Keycloak realm, reached only for the
	// password and refresh grants.
	DefaultAuthBaseURL = "https://secure.zwift.com"
	// DefaultAPIBaseURL is Zwift's activity API host.
	DefaultAPIBaseURL = "https://us-or-rly101.zwift.com"

	// tokenPath is the grant endpoint every known public Zwift client uses.
	tokenPath = "/auth/realms/zwift/tokens/access/codes" //nolint:gosec // G101: an endpoint path, not a credential

	// TokenPathCanonical is Keycloak's standard token endpoint on the same
	// realm. Documented as a fallback the acceptance test can switch to if
	// tokenPath ever stops answering; never read from runtime configuration.
	TokenPathCanonical = "/auth/realms/zwift/protocol/openid-connect/token" //nolint:gosec // G101: an endpoint path, not a credential

	// clientID is the public mobile client id every known open-source Zwift
	// client authenticates as. Zwift issues no client secret for it.
	clientID = "Zwift_Mobile_Link"

	defaultTimeout   = 30 * time.Second
	maximumBodyBytes = 1 << 20
	// maximumFITBytes bounds a downloaded activity file. An indoor ride's FIT
	// file is small; this refuses a reply too large to be one.
	maximumFITBytes = 32 << 20
)

// ErrUnauthorized reports credentials or a session the token endpoint or an
// API call rejected.
var ErrUnauthorized = errors.New("zwift: authorization was rejected")

// ErrActivityRefused reports one activity or its file that Zwift refuses or no
// longer has, rather than a connection or account problem.
var ErrActivityRefused = errors.New("zwift: activity was refused")

// ErrRejected reports a request Zwift refused outright: rate limited or its
// own failure, rather than this account's or this activity's.
var ErrRejected = errors.New("zwift: request was rejected")

// fitHostPattern matches only a bucket-scoped S3 virtual-hosted URL; anchored
// so neither a trailing nor a spoofed leading label can slip past it.
var fitHostPattern = regexp.MustCompile(`^[a-z0-9.-]+\.s3\.amazonaws\.com$`)

// Options configures a Zwift API client.
type Options struct {
	Transport   http.RoundTripper
	AuthBaseURL string
	APIBaseURL  string
	Timeout     time.Duration
}

// Session is one rider's Zwift grant, held only in memory for the length of
// one poll and never handed to a store.
type Session struct {
	ExpiresAt    time.Time
	AccessToken  string
	RefreshToken string
}

// String redacts a session from every formatting verb and log line.
func (Session) String() string { return "[redacted]" }

// GoString is the same for %#v.
func (Session) GoString() string { return "[redacted]" }

// Client is a Zwift API client.
type Client struct {
	httpClient  *http.Client
	authBaseURL *url.URL
	apiBaseURL  *url.URL
}

// New creates a Zwift client without contacting the API.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("zwift: options are required")
	}
	authBaseURL, err := parseOrigin(cmp.Or(options.AuthBaseURL, DefaultAuthBaseURL))
	if err != nil {
		return nil, fmt.Errorf("zwift: auth base url: %w", err)
	}
	apiBaseURL, err := parseOrigin(cmp.Or(options.APIBaseURL, DefaultAPIBaseURL))
	if err != nil {
		return nil, fmt.Errorf("zwift: api base url: %w", err)
	}
	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("zwift: timeout must be positive")
	}
	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &Client{
		httpClient:  &http.Client{Timeout: timeout, Transport: transport},
		authBaseURL: authBaseURL,
		apiBaseURL:  apiBaseURL,
	}, nil
}

// IsUnauthorized reports whether err is a permanent Zwift authorization
// rejection. Consumers use it to ask the rider to re-enter their credentials.
func (c *Client) IsUnauthorized(err error) bool { return errors.Is(err, ErrUnauthorized) }

// IsUnreadable reports whether err is one activity's or its file's own
// refusal, rather than a connection, account or quota problem.
func (c *Client) IsUnreadable(err error) bool { return errors.Is(err, ErrActivityRefused) }

// IsRejected reports whether err is a refusal that belongs to the connection
// or Zwift's own load, rather than to the account or the activity asked for.
func (c *Client) IsRejected(err error) bool { return errors.Is(err, ErrRejected) }

// Session signs in with a rider's own Zwift email and password.
func (c *Client) Session(ctx context.Context, email, password []byte) (Session, error) {
	if len(email) == 0 || len(password) == 0 {
		return Session{}, errors.New("zwift: email and password are required")
	}

	return c.token(ctx, url.Values{
		"grant_type": {"password"},
		"username":   {string(email)},
		"password":   {string(password)},
		"client_id":  {clientID},
	})
}

// Refresh trades a held refresh token for a fresh session.
func (c *Client) Refresh(ctx context.Context, session Session) (Session, error) {
	if session.RefreshToken == "" {
		return Session{}, errors.New("zwift: refresh token is required")
	}

	return c.token(ctx, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {session.RefreshToken},
		"client_id":     {clientID},
	})
}

func (c *Client) token(ctx context.Context, form url.Values) (Session, error) {
	endpoint := c.endpoint(c.authBaseURL, tokenPath)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return Session{}, fmt.Errorf("zwift: creating token request: %w", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")

	var response struct {
		//nolint:tagliatelle // Zwift's token endpoint uses snake_case.
		AccessToken string `json:"access_token"`
		//nolint:tagliatelle // Zwift's token endpoint uses snake_case.
		RefreshToken string `json:"refresh_token"`
		//nolint:tagliatelle // Zwift's token endpoint uses snake_case.
		ExpiresIn int64 `json:"expires_in"`
	}
	requestedAt := time.Now()
	if err := c.doJSON(request, &response, false); err != nil {
		return Session{}, err
	}
	if response.AccessToken == "" || response.RefreshToken == "" {
		return Session{}, errors.New("zwift: token response was incomplete")
	}

	return Session{
		AccessToken:  response.AccessToken,
		RefreshToken: response.RefreshToken,
		ExpiresAt:    requestedAt.Add(time.Duration(response.ExpiresIn) * time.Second),
	}, nil
}

// PlayerID returns the rider's own player id, the handle every other call in
// this package addresses them by.
func (c *Client) PlayerID(ctx context.Context, session Session) (int64, error) {
	if session.AccessToken == "" {
		return 0, errors.New("zwift: session access token is required")
	}
	request, err := c.newAPIRequest(ctx, http.MethodGet, "/api/profiles/me", nil, session)
	if err != nil {
		return 0, err
	}

	var response struct {
		ID int64 `json:"id"`
	}
	if err := c.doJSON(request, &response, false); err != nil {
		return 0, err
	}
	if response.ID <= 0 {
		return 0, errors.New("zwift: profile response did not contain an id")
	}

	return response.ID, nil
}

// Activities returns one offset page of the rider's own activities, newest
// first.
func (c *Client) Activities(ctx context.Context, session Session, playerID int64, start, limit int) ([]Activity, error) {
	if session.AccessToken == "" || playerID <= 0 {
		return nil, errors.New("zwift: session and player id are required")
	}
	if start < 0 || limit <= 0 {
		return nil, errors.New("zwift: start must not be negative and limit must be positive")
	}

	query := url.Values{"start": {strconv.Itoa(start)}, "limit": {strconv.Itoa(limit)}}
	request, err := c.newAPIRequest(ctx, http.MethodGet, fmt.Sprintf("/api/profiles/%d/activities", playerID), query, session)
	if err != nil {
		return nil, err
	}

	var activities []Activity
	if err := c.doJSON(request, &activities, false); err != nil {
		return nil, err
	}

	return activities, nil
}

// DownloadFIT reads the FIT file a listing entry names, refusing any URL that
// is not a bucket-scoped https://<bucket>.s3.amazonaws.com/<key> object: the
// file is a public S3 object, and following anywhere else would turn a
// listing into SSRF. No Authorization header is sent — the bucket rejects one.
func (c *Client) DownloadFIT(ctx context.Context, fileURL string) (data []byte, err error) {
	parsed, parseErr := url.Parse(fileURL)
	if parseErr != nil || parsed.Scheme != "https" || parsed.User != nil ||
		!fitHostPattern.MatchString(parsed.Host) {
		return nil, errors.New("zwift: fit file url was invalid")
	}

	request, requestErr := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), http.NoBody)
	if requestErr != nil {
		// Not wrapped: a *url.Error message would carry the fit file url.
		return nil, errors.New("zwift: fit file request could not be created")
	}

	response, doErr := c.httpClient.Do(request)
	if doErr != nil {
		if urlErr, ok := errors.AsType[*url.Error](doErr); ok {
			doErr = urlErr.Err
		}

		return nil, fmt.Errorf("zwift: fit file request failed: %w", doErr)
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	if classified := classifyStatus(response.StatusCode, true); classified != nil {
		return nil, classified
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, maximumFITBytes+1))
	if err != nil {
		return nil, errors.New("zwift: fit file could not be read")
	}
	if len(data) == 0 || len(data) > maximumFITBytes {
		return nil, errors.New("zwift: fit file was empty or exceeded size limit")
	}

	return data, nil
}

func (c *Client) newAPIRequest(ctx context.Context, method, path string, query url.Values, session Session) (*http.Request, error) {
	endpoint := c.endpoint(c.apiBaseURL, path)
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("zwift: creating request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+session.AccessToken)

	return request, nil
}

func (c *Client) endpoint(base *url.URL, path string) *url.URL {
	endpoint := *base
	endpoint.Path = path
	endpoint.RawPath = ""
	endpoint.RawQuery = ""

	return &endpoint
}

// doJSON sends request and decodes a JSON reply into output. activityRefusal
// says whether a 404 or 410 here belongs to one activity rather than the
// connection or the account.
func (c *Client) doJSON(request *http.Request, output any, activityRefusal bool) (err error) {
	response, err := c.httpClient.Do(request)
	if err != nil {
		if urlErr, ok := errors.AsType[*url.Error](err); ok {
			err = urlErr.Err
		}

		return fmt.Errorf("zwift: request failed: %w", err)
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	if classified := classifyStatus(response.StatusCode, activityRefusal); classified != nil {
		return classified
	}

	body, readErr := io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if readErr != nil {
		return errors.New("zwift: response could not be read")
	}
	if len(body) > maximumBodyBytes {
		return errors.New("zwift: response exceeded size limit")
	}
	if err := json.Unmarshal(body, output); err != nil {
		return errors.New("zwift: response was not valid json")
	}

	return nil
}

// classifyStatus maps an HTTP status onto this package's sentinels. The error
// text carries only the status and the category name, never the response body.
func classifyStatus(status int, activityRefusal bool) error {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		return fmt.Errorf("%w: HTTP %d", ErrUnauthorized, status)
	case activityRefusal && (status == http.StatusNotFound || status == http.StatusGone):
		return fmt.Errorf("%w: HTTP %d", ErrActivityRefused, status)
	case status == http.StatusTooManyRequests || status >= http.StatusInternalServerError:
		return fmt.Errorf("%w: HTTP %d", ErrRejected, status)
	default:
		return fmt.Errorf("zwift: request returned HTTP %d", status)
	}
}

func parseOrigin(value string) (*url.URL, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return nil, errors.New("must be an absolute https origin")
	}

	return parsed, nil
}
