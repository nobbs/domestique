package veloplanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/nobbs/domestique/internal/route"
)

const (
	defaultTimeout   = 90 * time.Second
	maximumBodyBytes = 16 << 20
	maximumPages     = 1_000
	maximumRoutes    = 10_000
	userAgent        = "domestique/1.0"
	sessionCookie    = "_veloplanner_ex_key"
)

var (
	// ErrAuthentication identifies an unsuccessful VeloPlanner login.
	ErrAuthentication = errors.New("veloplanner: authentication failed")

	inputTagRE  = regexp.MustCompile(`(?i)<input\b[^>]*>`)
	nameAttrRE  = regexp.MustCompile(`(?i)\bname\s*=\s*"([^"]*)"`)
	valueAttrRE = regexp.MustCompile(`(?i)\bvalue\s*=\s*"([^"]*)"`)
	loggedInRE  = regexp.MustCompile(`isUserLoggedIn:\s*(true|false)`)
	userIDRE    = regexp.MustCompile(`userId:\s*(-?\d+)`)
)

// Options configures a VeloPlanner client with resolved credentials.
type Options struct {
	Transport http.RoundTripper
	BaseURL   string
	Email     []byte
	Password  []byte
	Timeout   time.Duration
}

// Client inventories one VeloPlanner account. It does not retain a session
// between calls to Inventory.
type Client struct {
	transport http.RoundTripper
	baseURL   *url.URL
	email     []byte
	password  []byte
	timeout   time.Duration
}

// New creates a VeloPlanner client without contacting the upstream service.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("veloplanner: options are required")
	}

	baseURL, err := url.Parse(options.BaseURL)
	if err != nil || baseURL.Scheme != "https" || baseURL.Host == "" ||
		baseURL.User != nil || baseURL.RawQuery != "" || baseURL.Fragment != "" ||
		(baseURL.Path != "" && baseURL.Path != "/") {
		return nil, errors.New("veloplanner: base url must be an absolute https origin")
	}
	if len(options.Email) == 0 {
		return nil, errors.New("veloplanner: email is required")
	}
	if len(options.Password) == 0 {
		return nil, errors.New("veloplanner: password is required")
	}

	timeout := options.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("veloplanner: timeout must be positive")
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
	return route.ProviderVeloPlanner
}

// Inventory logs in with a new session and returns every non-empty source
// route stage in stable source order.
func (c *Client) Inventory(ctx context.Context) ([]route.Route, error) {
	session, err := c.newSession()
	if err != nil {
		return nil, fmt.Errorf("veloplanner: creating session: %w", err)
	}

	userID, err := session.login(ctx)
	if err != nil {
		return nil, err
	}

	summaries, err := session.listRoutes(ctx, userID)
	if err != nil {
		return nil, err
	}

	stages := make([]route.Route, 0, len(summaries))
	for _, summary := range summaries {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("veloplanner: inventory cancelled: %w", err)
		}
		if summary.ID <= 0 {
			return nil, errors.New("veloplanner: route library contained an invalid route id")
		}

		detail, err := session.routeDetail(ctx, summary.ID)
		if err != nil {
			return nil, err
		}
		if detail.ID != summary.ID || detail.ID <= 0 {
			return nil, fmt.Errorf("veloplanner: route detail identity did not match requested route")
		}

		converted, err := convertRoute(detail)
		if err != nil {
			return nil, err
		}
		stages = append(stages, converted...)
	}

	return stages, nil
}

type session struct {
	client  *http.Client
	parent  *Client
	baseURL *url.URL
}

func (c *Client) newSession() (*session, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	return &session{
		client: &http.Client{
			Jar:       jar,
			Timeout:   c.timeout,
			Transport: c.transport,
		},
		parent:  c,
		baseURL: c.baseURL,
	}, nil
}

func (s *session) login(ctx context.Context) (int, error) {
	body, err := s.get(ctx, "/login", "text/html,application/xhtml+xml")
	if err != nil {
		return 0, fmt.Errorf("veloplanner: fetching login page: %w", err)
	}

	csrfToken, err := extractCSRFToken(string(body))
	if err != nil {
		return 0, fmt.Errorf("%w: login form did not contain a CSRF token", ErrAuthentication)
	}

	form := url.Values{
		"_csrf_token":    {csrfToken},
		"user[email]":    {string(s.parent.email)},
		"user[password]": {string(s.parent.password)},
	}
	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		s.url("/login").String(),
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return 0, fmt.Errorf("veloplanner: creating login request: %w", err)
	}
	request.Header.Set("Accept", "text/html,application/xhtml+xml")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", userAgent)

	body, err = s.do(request)
	if err != nil {
		return 0, fmt.Errorf("veloplanner: reading login response: %w", err)
	}

	userID, authenticated := extractIdentity(string(body))
	if !authenticated || !s.hasSessionCookie() {
		return 0, ErrAuthentication
	}

	return userID, nil
}

func (s *session) listRoutes(ctx context.Context, userID int) ([]routeSummary, error) {
	var routes []routeSummary
	for page := 1; page <= maximumPages; page++ {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("veloplanner: route listing cancelled: %w", err)
		}

		var payload struct {
			Data     []routeSummary `json:"data"`
			Metadata struct {
				Page int `json:"page"`
				//nolint:tagliatelle // VeloPlanner's API uses snake_case.
				TotalPages int `json:"total_pages"`
				//nolint:tagliatelle // VeloPlanner's API uses snake_case.
				TotalCount int `json:"total_count"`
			} `json:"metadata"`
		}

		endpoint := fmt.Sprintf("/api/internal/users/%d/routes?page=%d", userID, page)
		if err := s.getJSON(ctx, endpoint, &payload); err != nil {
			return nil, fmt.Errorf("veloplanner: listing routes: %w", err)
		}
		if payload.Metadata.Page != page || payload.Metadata.TotalCount < 0 ||
			payload.Metadata.TotalPages < page || payload.Metadata.TotalPages > maximumPages ||
			payload.Metadata.TotalCount > maximumRoutes {
			return nil, errors.New("veloplanner: invalid route library pagination")
		}

		routes = append(routes, payload.Data...)
		if len(routes) > maximumRoutes || len(routes) > payload.Metadata.TotalCount {
			return nil, errors.New("veloplanner: route library exceeded configured bounds")
		}
		if page == payload.Metadata.TotalPages {
			if len(routes) != payload.Metadata.TotalCount {
				return nil, errors.New("veloplanner: route library count did not match pagination")
			}
			return routes, nil
		}
	}

	return nil, errors.New("veloplanner: route library exceeded maximum page count")
}

func (s *session) routeDetail(ctx context.Context, routeID int64) (sourceRoute, error) {
	var payload struct {
		Data sourceRoute `json:"data"`
	}
	if err := s.getJSON(ctx, fmt.Sprintf("/api/internal/user_routes/%d", routeID), &payload); err != nil {
		return sourceRoute{}, fmt.Errorf("veloplanner: retrieving route detail: %w", err)
	}

	return payload.Data, nil
}

func (s *session) getJSON(ctx context.Context, endpoint string, output any) error {
	body, err := s.get(ctx, endpoint, "application/json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, output); err != nil {
		return errors.New("upstream response was not valid JSON")
	}

	return nil
}

func (s *session) get(ctx context.Context, endpoint, accept string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, s.url(endpoint).String(), http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	request.Header.Set("Accept", accept)
	request.Header.Set("User-Agent", userAgent)

	return s.do(request)
}

func (s *session) hasSessionCookie() bool {
	for _, cookie := range s.client.Jar.Cookies(s.baseURL) {
		if cookie.Name == sessionCookie && cookie.Value != "" {
			return true
		}
	}

	return false
}

func (s *session) url(endpoint string) *url.URL {
	path, query, _ := strings.Cut(endpoint, "?")
	resolved := *s.baseURL
	resolved.Path = path
	resolved.RawPath = ""
	resolved.RawQuery = query

	return &resolved
}

func (s *session) do(request *http.Request) (body []byte, err error) {
	response, err := s.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	body, err = io.ReadAll(io.LimitReader(response.Body, maximumBodyBytes+1))
	if err != nil {
		return nil, errors.New("unable to read upstream response")
	}
	if len(body) > maximumBodyBytes {
		return nil, errors.New("upstream response exceeded size limit")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upstream returned HTTP %d", response.StatusCode)
	}

	return body, nil
}

func extractCSRFToken(html string) (string, error) {
	for _, tag := range inputTagRE.FindAllString(html, -1) {
		name := nameAttrRE.FindStringSubmatch(tag)
		if len(name) != 2 || name[1] != "_csrf_token" {
			continue
		}
		value := valueAttrRE.FindStringSubmatch(tag)
		if len(value) == 2 && value[1] != "" {
			return value[1], nil
		}
	}

	return "", errors.New("CSRF token was not found")
}

func extractIdentity(html string) (int, bool) {
	flag := loggedInRE.FindStringSubmatch(html)
	if len(flag) != 2 || flag[1] != "true" {
		return 0, false
	}
	userID := userIDRE.FindStringSubmatch(html)
	if len(userID) != 2 {
		return 0, false
	}
	parsed, err := strconv.Atoi(userID[1])
	if err != nil || parsed <= 0 {
		return 0, false
	}

	return parsed, true
}
