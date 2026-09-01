// Package auth0 is the Auth0 tenant as a sign-in provider: it builds the
// authorisation URL and turns a returned code into a verified identity.
package auth0

import (
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
	ClientSecret string
	RedirectURL  string
}

// Identity is the verified caller an exchanged ID token names.
type Identity struct {
	Subject string
	Email   string
	Name    string
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
	clientSecret    string
	redirectURL     string
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
	clientSecret := strings.TrimSpace(options.ClientSecret)
	redirectURL := strings.TrimSpace(options.RedirectURL)
	if domain == "" || clientID == "" || clientSecret == "" || redirectURL == "" {
		return nil, errors.New("auth0: domain, client id, client secret, and redirect url are required")
	}

	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = defaultHTTPClient()
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Client{
		domain:       domain,
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		httpClient:   httpClient,
		now:          now,
	}, nil
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
	sdk, err := c.authentication(ctx)
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
func (c *Client) authentication(ctx context.Context) (*authentication.Authentication, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sdk != nil {
		return c.sdk, nil
	}
	if !c.lastAttempt.IsZero() && c.now().Sub(c.lastAttempt) < initialisationFloor {
		return nil, fmt.Errorf("auth0: provider unavailable: %w", c.initialiseError)
	}
	c.lastAttempt = c.now()

	// The cache the SDK builds its JWKS refresher on must outlive this
	// request: it is reused by every later Exchange, not just this one.
	sdk, err := authentication.New(
		context.WithoutCancel(ctx),
		c.domain,
		authentication.WithClientID(c.clientID),
		authentication.WithClientSecret(c.clientSecret),
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

// readIdentity reads sub, email, and name from an already-validated ID
// token's payload. No re-verification: LoginWithAuthCodeWithPKCE has already
// checked the signature, issuer, audience, expiry, and nonce.
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
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Identity{}, errors.New("auth0: id token payload is not valid json")
	}

	return Identity{Subject: claims.Subject, Email: claims.Email, Name: claims.Name}, nil
}
