package komoot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

const (
	defaultTimeout   = 90 * time.Second
	maximumBodyBytes = 16 << 20
	maximumPages     = 1_000
	maximumTours     = 10_000
	tourPageSize     = 50
	userAgent        = "domestique/1.0"
	tourTypePlanned  = "tour_planned"

	// acceptJSON is deliberately not the plain "application/json" it looks
	// like it should be. Verified against a live account: every v007 resource
	// (listing, detail) is served as HAL and answers a strict
	// "Accept: application/json" with 406 HttpMediaTypeNotAcceptable, even
	// though its body is ordinary JSON either way. v006's login is more
	// lenient and would accept either, so the same header is used everywhere
	// rather than carrying two.
	acceptJSON = "application/hal+json, application/json;q=0.9"
)

// ErrAuthentication identifies an unsuccessful Komoot login.
var ErrAuthentication = errors.New("komoot: authentication failed")

// Options configures a Komoot client with resolved credentials.
type Options struct {
	Transport http.RoundTripper
	BaseURL   string
	Email     []byte
	Password  []byte
	Timeout   time.Duration
}

// Client inventories one Komoot account's planned tours. It does not retain a
// session between calls to Inventory, and it issues no HTTP method other than
// GET: the account's session token is not read-scoped, so nothing in this
// package may risk a write against it.
type Client struct {
	transport http.RoundTripper
	baseURL   *url.URL
	email     []byte
	password  []byte
	timeout   time.Duration
}

// New creates a Komoot client without contacting the upstream service.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("komoot: options are required")
	}

	baseURL, err := url.Parse(options.BaseURL)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" || baseURL.Hostname() == "" ||
		baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" ||
		(baseURL.Path != "" && baseURL.Path != "/") {
		return nil, errors.New("komoot: base url must be an absolute https origin")
	}
	if len(options.Email) == 0 {
		return nil, errors.New("komoot: email is required")
	}
	if len(options.Password) == 0 {
		return nil, errors.New("komoot: password is required")
	}

	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("komoot: timeout must be positive")
	}

	transport := options.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	return &Client{
		baseURL:   baseURL,
		email:     append([]byte(nil), options.Email...),
		password:  append([]byte(nil), options.Password...),
		timeout:   timeout,
		transport: transport,
	}, nil
}

// Provider names the upstream this client reads from, for the sync package's
// set of configured sources.
func (c *Client) Provider() route.Provider {
	return route.ProviderKomoot
}

// Inventory logs in with a new session and returns every planned tour as a
// single-stage route.Stage, in stable order.
func (c *Client) Inventory(ctx context.Context) ([]route.Stage, error) {
	session := c.newSession()

	userID, token, err := session.login(ctx)
	if err != nil {
		return nil, err
	}

	summaries, err := session.listTours(ctx, userID, token)
	if err != nil {
		return nil, err
	}

	stages := make([]route.Stage, 0, len(summaries))
	for _, summary := range summaries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("komoot: inventory cancelled: %w", err)
		}
		if summary.ID <= 0 {
			return nil, errors.New("komoot: tour library contained an invalid tour id")
		}

		detail, err := session.tourDetail(ctx, userID, token, summary.ID)
		if err != nil {
			return nil, err
		}
		if detail.ID != summary.ID || detail.ID <= 0 {
			return nil, errors.New("komoot: tour detail identity did not match requested tour")
		}
		if detail.Type != tourTypePlanned {
			return nil, fmt.Errorf("komoot: tour %d was not a planned tour", detail.ID)
		}

		stage, err := convertTour(&detail)
		if err != nil {
			return nil, err
		}
		stages = append(stages, stage)
	}

	return stages, nil
}

type session struct {
	client  *http.Client
	parent  *Client
	baseURL *url.URL
}

func (c *Client) newSession() *session {
	return &session{
		client: &http.Client{
			Timeout:   c.timeout,
			Transport: c.transport,
			// Every request here carries HTTP Basic credentials or the session
			// token. Go's default redirect policy keeps the Authorization header
			// on a same-host redirect, so an https-to-http redirect on the
			// configured origin would otherwise downgrade and leak them. Nothing
			// this package calls ever needs to be redirected.
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("komoot: refusing to follow a redirect")
			},
		},
		parent:  c,
		baseURL: c.baseURL,
	}
}

// login exchanges the account's email and password for a numeric user id and
// a session token, both returned under misleading field names. It is the only
// call that authenticates with the account password; every later call
// authenticates with the returned token instead.
//
// It builds its own request rather than going through newRequest: the email
// is untrusted-shaped data embedded directly in a path segment, and
// newRequest's endpoint strings are otherwise plain constants and validated
// numeric ids, so keeping the one call site that needs path escaping separate
// keeps that escaping in exactly one place.
func (s *session) login(ctx context.Context) (userID, token string, err error) {
	email := string(s.parent.email)
	loginURL := *s.baseURL
	// Path stays decoded; RawPath carries the escaped form so a character that
	// needs escaping — including a literal '/', which PathEscape encodes as
	// %2F — round-trips as one path segment rather than becoming an extra
	// segment or being escaped a second time.
	loginURL.Path = "/v006/account/email/" + email + "/"
	loginURL.RawPath = "/v006/account/email/" + url.PathEscape(email) + "/"
	loginURL.RawQuery = ""

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, loginURL.String(), http.NoBody)
	if err != nil {
		return "", "", fmt.Errorf("creating login request: %w", err)
	}
	request.Header.Set("Accept", acceptJSON)
	request.Header.Set("User-Agent", userAgent)
	request.SetBasicAuth(string(s.parent.email), string(s.parent.password))

	body, status, err := s.doRequest(request)
	if err != nil {
		return "", "", fmt.Errorf("komoot: sending login request: %w", err)
	}
	if status == http.StatusUnauthorized || status == http.StatusNotFound {
		return "", "", ErrAuthentication
	}
	if status != http.StatusOK {
		return "", "", fmt.Errorf("komoot: login returned HTTP %d", status)
	}

	var payload accountResponse
	if unmarshalErr := json.Unmarshal(body, &payload); unmarshalErr != nil {
		return "", "", errors.New("komoot: login response was not valid JSON")
	}
	if payload.Username == "" || payload.Password == "" {
		return "", "", ErrAuthentication
	}
	// username is documented as carrying the numeric user id. It is about to be
	// interpolated into every later request's path, so a non-numeric or zero
	// value is treated the same as any other unusable login response rather
	// than trusted verbatim.
	parsedUserID, err := strconv.ParseUint(payload.Username, 10, 64)
	if err != nil || parsedUserID == 0 {
		return "", "", ErrAuthentication
	}

	return payload.Username, payload.Password, nil
}

func (s *session) listTours(ctx context.Context, userID, token string) ([]tourSummary, error) {
	var tours []tourSummary
	wantTotal, wantPages := -1, -1

	for page := 0; ; page++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("komoot: tour listing cancelled: %w", err)
		}
		if page >= maximumPages {
			return nil, errors.New("komoot: tour library exceeded maximum page count")
		}

		var payload toursResponse
		endpoint := fmt.Sprintf("/v007/users/%s/tours/?type=%s&page=%d&limit=%d", userID, tourTypePlanned, page, tourPageSize)
		if err := s.getJSON(ctx, userID, token, endpoint, &payload); err != nil {
			return nil, fmt.Errorf("komoot: listing tours: %w", err)
		}
		if payload.Page == nil || payload.Page.Number == nil || payload.Page.TotalElements == nil ||
			payload.Page.TotalPages == nil || payload.Page.Size == nil {
			return nil, errors.New("komoot: tour listing response had no page metadata")
		}
		if payload.Embedded == nil || payload.Embedded.Tours == nil {
			return nil, errors.New("komoot: tour listing response had no tours container")
		}
		number, totalElements, totalPages, size :=
			*payload.Page.Number, *payload.Page.TotalElements, *payload.Page.TotalPages, *payload.Page.Size

		if number != page || totalElements < 0 || size < 0 ||
			totalPages < 0 || totalPages > maximumPages ||
			totalElements > maximumTours ||
			(totalPages == 0) != (totalElements == 0) ||
			(wantTotal >= 0 && totalElements != wantTotal) ||
			(wantPages >= 0 && totalPages != wantPages) {
			return nil, errors.New("komoot: invalid tour library pagination")
		}
		wantTotal, wantPages = totalElements, totalPages

		tours = append(tours, payload.Embedded.Tours...)
		if len(tours) > maximumTours {
			return nil, errors.New("komoot: tour library exceeded configured bounds")
		}

		if page+1 >= totalPages {
			break
		}
	}

	if len(tours) != wantTotal {
		return nil, errors.New("komoot: tour library count did not match pagination")
	}

	return tours, nil
}

func (s *session) tourDetail(ctx context.Context, userID, token string, tourID int64) (tourDetail, error) {
	var payload tourDetail
	endpoint := fmt.Sprintf("/v007/tours/%d?_embedded=coordinates", tourID)
	if err := s.getJSON(ctx, userID, token, endpoint, &payload); err != nil {
		return tourDetail{}, fmt.Errorf("komoot: retrieving tour detail: %w", err)
	}

	return payload, nil
}

func (s *session) getJSON(ctx context.Context, userID, token, endpoint string, output any) error {
	request, err := s.newRequest(ctx, endpoint)
	if err != nil {
		return err
	}
	request.SetBasicAuth(userID, token)

	body, status, err := s.doRequest(request)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("upstream returned HTTP %d", status)
	}
	if err := json.Unmarshal(body, output); err != nil {
		return errors.New("upstream response was not valid JSON")
	}

	return nil
}

// newRequest always builds a GET request. This package issues no other HTTP
// method: the account's session token permits PATCH, PUT and DELETE on a tour
// resource, and a bug that sent one would destroy a route in the operator's
// own Komoot library.
func (s *session) newRequest(ctx context.Context, endpoint string) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(endpoint).String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	request.Header.Set("Accept", acceptJSON)
	request.Header.Set("User-Agent", userAgent)

	return request, nil
}

func (s *session) url(endpoint string) *url.URL {
	path, query, _ := strings.Cut(endpoint, "?")
	resolved := *s.baseURL
	resolved.Path = path
	resolved.RawPath = ""
	resolved.RawQuery = query

	return &resolved
}

func (s *session) doRequest(request *http.Request) (body []byte, statusCode int, err error) {
	response, err := s.client.Do(request)
	if err != nil {
		// http.Client wraps a transport, TLS, or timeout failure in a *url.Error
		// whose Error() text includes the request URL — for login, a URL that
		// embeds the account's email. Unwrap to the underlying cause so a
		// caller that logs this failure never has the operator's email to log,
		// while errors.Is against context.Canceled or a deadline still works
		// against the unwrapped cause.
		var urlErr *url.Error
		if errors.As(err, &urlErr) { //nolint:modernize // errors.As is unambiguous to every tool reviewing this code.
			err = urlErr.Err
		}

		return nil, 0, fmt.Errorf("sending request: %w", err)
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	body, err = io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if err != nil {
		// A body read that fails because the caller's context was cancelled or
		// timed out mid-stream is that cancellation, not a corrupt response;
		// callers checking errors.Is against context.Canceled or
		// context.DeadlineExceeded need it preserved rather than replaced with
		// a generic read failure.
		if ctxErr := request.Context().Err(); ctxErr != nil {
			return nil, 0, fmt.Errorf("reading response: %w", ctxErr)
		}

		return nil, 0, errors.New("unable to read upstream response")
	}
	if len(body) > maximumBodyBytes {
		return nil, 0, errors.New("upstream response exceeded size limit")
	}

	return body, response.StatusCode, nil
}
