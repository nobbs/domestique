// Package auth0 is the Auth0 tenant as a sign-in provider: it builds the
// authorisation URL and turns a returned code into a verified identity.
package auth0

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/auth0/go-auth0/v2/authentication"
	"github.com/auth0/go-auth0/v2/authentication/oauth"
)

// initialisationFloor spaces retries of the SDK's eager JWKS fetch, so a broken
// issuer cannot be re-fetched on every hit of the public callback.
const initialisationFloor = 10 * time.Second

// Options configures a Client.
type Options struct {
	HTTPClient   *http.Client
	Now          func() time.Time
	Domain       string
	ClientID     string
	RedirectURL  string
	ClientSecret []byte
}

// Identity is the verified caller an exchanged ID token names.
type Identity struct {
	Subject string
	Email   string
	Name    string
	// Access is the Action's assertion that this subject may hold a session
	// at all — the replacement for a config-file allowlist.
	Access bool
	// Admin is the Action's assertion that this subject holds cross-subject
	// rights once signed in.
	Admin bool
}

// Client is an Auth0 tenant configured as a sign-in provider.
type Client struct {
	lastAttempt     time.Time
	initialiseError error
	httpClient      *http.Client
	now             func() time.Time
	sdk             *authentication.Authentication
	domain          string
	clientID        string
	redirectURL     string
	clientSecret    []byte
	mu              sync.Mutex
}

// New validates the options and returns a Client. It contacts nothing: the
// Auth0 SDK, which eagerly fetches the tenant's JWKS on construction, is
// built lazily on the first Exchange.
func New(options *Options) (*Client, error) {
	if options == nil {
		return nil, errors.New("auth0: options are required")
	}

	domain := strings.TrimSpace(options.Domain)
	clientID := strings.TrimSpace(options.ClientID)
	redirectURL := strings.TrimSpace(options.RedirectURL)
	if domain == "" || clientID == "" || len(options.ClientSecret) == 0 || redirectURL == "" {
		return nil, errors.New("auth0: domain, client id, client secret, and redirect url are required")
	}
	if err := validateDomain(domain); err != nil {
		return nil, err
	}
	if err := validateRedirectURL(redirectURL); err != nil {
		return nil, err
	}

	httpClient := boundedHTTPClient(options.HTTPClient)
	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Client{
		domain:       domain,
		clientID:     clientID,
		clientSecret: bytes.Clone(options.ClientSecret),
		redirectURL:  redirectURL,
		httpClient:   httpClient,
		now:          now,
	}, nil
}

// validateDomain accepts the tenant as a bare host, optionally with a port.
// The value becomes a URL host and is handed to the SDK, so a scheme, path, or
// userinfo in it would silently address the wrong tenant.
func validateDomain(domain string) error {
	parsed, err := url.Parse("https://" + domain)
	if err != nil || parsed.Host != domain || parsed.User != nil || parsed.Path != "" ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("auth0: domain must be a bare tenant host, without a scheme or path")
	}

	return nil
}

// validateRedirectURL mirrors the Wahoo callback rule: the value is sent to
// the provider and must match the registered callback exactly.
func validateRedirectURL(redirectURL string) error {
	parsed, err := url.Parse(redirectURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.Path == "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("auth0: redirect url must be an absolute https callback url")
	}

	return nil
}

// AuthorizationURL builds the /authorize URL for a PKCE authorization-code
// request. It is pure: no SDK, no network.
func (c *Client) AuthorizationURL(_ context.Context, state, nonce, codeVerifier string) (string, error) {
	if state == "" || nonce == "" || codeVerifier == "" {
		return "", errors.New("auth0: state, nonce, and code verifier are required")
	}

	challenge := sha256.Sum256([]byte(codeVerifier))

	authorizeURL := url.URL{
		Scheme: "https",
		Host:   c.domain,
		Path:   "/authorize",
	}
	query := url.Values{
		"response_type":         {"code"},
		"client_id":             {c.clientID},
		"redirect_uri":          {c.redirectURL},
		"scope":                 {"openid profile email"},
		"state":                 {state},
		"nonce":                 {nonce},
		"code_challenge":        {base64.RawURLEncoding.EncodeToString(challenge[:])},
		"code_challenge_method": {"S256"},
	}
	authorizeURL.RawQuery = query.Encode()

	return authorizeURL.String(), nil
}

// Exchange trades an authorization code for a verified identity.
func (c *Client) Exchange(ctx context.Context, code, codeVerifier, nonce string) (Identity, error) {
	if code == "" || codeVerifier == "" || nonce == "" {
		return Identity{}, errors.New("auth0: code, code verifier, and nonce are required")
	}

	sdk, err := c.authentication()
	if err != nil {
		return Identity{}, err
	}

	tokens, err := sdk.OAuth.LoginWithAuthCodeWithPKCE(
		ctx,
		oauth.LoginWithAuthCodeWithPKCERequest{
			Code:         code,
			CodeVerifier: codeVerifier,
			RedirectURI:  c.redirectURL,
		},
		oauth.IDTokenValidationOptions{Nonce: nonce},
	)
	if err != nil {
		return Identity{}, fmt.Errorf("auth0: exchanging code: %w", err)
	}
	if tokens.IDToken == "" {
		return Identity{}, errors.New("auth0: token response carried no id token")
	}

	identity, err := readIdentity(tokens.IDToken)
	if err != nil {
		return Identity{}, err
	}
	if identity.Subject == "" {
		return Identity{}, errors.New("auth0: id token carried no subject")
	}

	return identity, nil
}

// authentication returns the cached SDK client, building it on first use. Not
// sync.Once: that would cache a transient issuer failure for the process lifetime.
func (c *Client) authentication() (*authentication.Authentication, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sdk != nil {
		return c.sdk, nil
	}
	now := c.now()
	if !c.lastAttempt.IsZero() && now.Sub(c.lastAttempt) < initialisationFloor {
		return nil, fmt.Errorf("auth0: provider unavailable: %w", c.initialiseError)
	}
	c.lastAttempt = now

	// The JWKS refresher the SDK builds outlives every request, so it gets a
	// process-scoped context rather than a detached copy of a request's: a
	// request context would leave its values reachable for the process's life.
	// The initial fetch stays bounded by the HTTP client's timeout.
	sdk, err := authentication.New(
		context.Background(),
		c.domain,
		authentication.WithClientID(c.clientID),
		authentication.WithClientSecret(string(c.clientSecret)),
		authentication.WithIDTokenSigningAlg("RS256"),
		authentication.WithClient(c.httpClient),
		authentication.WithNoRetries(),
	)
	if err != nil {
		c.initialiseError = fmt.Errorf("auth0: initialising provider: %w", err)

		return nil, c.initialiseError
	}
	c.sdk = sdk
	c.initialiseError = nil

	return sdk, nil
}

// readIdentity reads sub, email, name, and the two namespaced claims a
// post-login Action mints from an already-validated ID token's payload. No
// re-verification: LoginWithAuthCodeWithPKCE has already checked the
// signature, issuer, audience, expiry, and nonce. The claim names use
// https://domestique.invalid/ — Auth0 requires custom claims to be namespaced
// URIs, and .invalid is the TLD RFC 2606 reserves so it can never resolve or
// imply a domain this project doesn't own. Either claim is simply absent
// (read as false) on a token from a tenant with no Action configured yet.
func readIdentity(idToken string) (Identity, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return Identity{}, errors.New("auth0: id token is not a compact jws")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Identity{}, errors.New("auth0: id token payload is not valid base64")
	}

	var claims struct {
		Subject string `json:"sub"`
		Email   string `json:"email"`
		Name    string `json:"name"`
		Access  bool   `json:"https://domestique.invalid/access"` //nolint:tagliatelle // an Auth0 namespaced claim, not a field name this project chose
		Admin   bool   `json:"https://domestique.invalid/admin"`  //nolint:tagliatelle // an Auth0 namespaced claim, not a field name this project chose
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Identity{}, errors.New("auth0: id token payload is not valid json")
	}

	return Identity{
		Subject: claims.Subject, Email: claims.Email, Name: claims.Name,
		Access: claims.Access, Admin: claims.Admin,
	}, nil
}
