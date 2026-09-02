package auth0

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testClientID     = "cid"
	testClientSecret = "secret"
	testRedirectURL  = "https://app.example/callback"
	testCode         = "code-1"
	testVerifier     = "verifier-1"
	testKeyID        = "test"
)

// fakeIssuer is a minimal Auth0 tenant: a JWKS endpoint backed by one RSA
// key, and a token endpoint whose response a test configures before calling
// Exchange.
type fakeIssuer struct {
	server       *httptest.Server
	key          *rsa.PrivateKey
	domain       string
	idToken      string
	rawTokenBody []byte
	mu           sync.Mutex
}

func newFakeIssuer(t *testing.T) *fakeIssuer {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err, "generating key")

	issuer := &fakeIssuer{key: key}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/jwks.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write(jwksDocument(&key.PublicKey, testKeyID))
		assert.NoError(t, err)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		// assert/require call FailNow via runtime.Goexit on failure, which the
		// test goroutine never observes from inside a handler goroutine; assert
		// keeps the check without silently swallowing it.
		assert.NoError(t, r.ParseForm(), "parsing token request")
		assert.Equal(t, "authorization_code", r.PostForm.Get("grant_type"))
		assert.Equal(t, testCode, r.PostForm.Get("code"))
		assert.Equal(t, testVerifier, r.PostForm.Get("code_verifier"))
		assert.Equal(t, testRedirectURL, r.PostForm.Get("redirect_uri"))
		assert.Equal(t, testClientID, r.PostForm.Get("client_id"))
		assert.Equal(t, testClientSecret, r.PostForm.Get("client_secret"))

		w.Header().Set("Content-Type", "application/json")

		issuer.mu.Lock()
		raw := issuer.rawTokenBody
		idToken := issuer.idToken
		issuer.mu.Unlock()

		if raw != nil {
			_, err := w.Write(raw)
			assert.NoError(t, err)

			return
		}

		body, err := json.Marshal(map[string]any{
			"access_token": "at-1",
			"id_token":     idToken,
			"token_type":   "Bearer",
			"expires_in":   3600,
		})
		assert.NoError(t, err, "marshalling token response")
		_, err = w.Write(body)
		assert.NoError(t, err)
	})

	issuer.server = httptest.NewTLSServer(mux)
	t.Cleanup(issuer.server.Close)
	issuer.domain = issuer.server.Listener.Addr().String()

	return issuer
}

func (f *fakeIssuer) setIDToken(idToken string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.idToken = idToken
}

func (f *fakeIssuer) setRawTokenBody(body []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rawTokenBody = body
}

// jwksDocument renders a public key as an Auth0-shaped JWKS document.
func jwksDocument(key *rsa.PublicKey, kid string) []byte {
	document := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"use": "sig",
			"alg": "RS256",
			"kid": kid,
			"n":   base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes()),
		}},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}

	return encoded
}

// tokenClaims builds a valid claim set for domain and clientID, evaluated at
// now, then applies overrides (a nil value deletes the claim).
func tokenClaims(domain, clientID, subject string, now time.Time, overrides map[string]any) map[string]any {
	claims := map[string]any{
		"iss":   "https://" + domain + "/",
		"aud":   clientID,
		"sub":   subject,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
		"email": "rider@example.com",
		"name":  "Rider Example",
	}
	for key, value := range overrides {
		if value == nil {
			delete(claims, key)

			continue
		}
		claims[key] = value
	}

	return claims
}

func segment(t *testing.T, value map[string]any) string {
	t.Helper()

	encoded, err := json.Marshal(value)
	require.NoError(t, err, "marshalling jws segment")

	return base64.RawURLEncoding.EncodeToString(encoded)
}

// signRS256 mints a compact JWS signed with key, as Auth0's own ID tokens are.
func signRS256(t *testing.T, key *rsa.PrivateKey, kid string, claims map[string]any) string {
	t.Helper()

	header := map[string]any{"alg": "RS256", "typ": "JWT", "kid": kid}
	signingInput := segment(t, header) + "." + segment(t, claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	require.NoError(t, err, "signing")

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

// hmacSHA256 signs input with secret, used to mint the HS256 alg-confusion
// probe below.
func hmacSHA256(secret, input string) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(input))

	return mac.Sum(nil)
}

func newClient(t *testing.T, issuer *fakeIssuer, httpClient *http.Client, now func() time.Time) *Client {
	t.Helper()

	client, err := New(&Options{
		Domain:       issuer.domain,
		ClientID:     testClientID,
		ClientSecret: []byte(testClientSecret),
		RedirectURL:  testRedirectURL,
		HTTPClient:   httpClient,
		Now:          now,
	})
	require.NoError(t, err, "constructing client")

	return client
}

func TestNewRejectsNilOptions(t *testing.T) {
	_, err := New(nil)
	require.Error(t, err)
}

func TestNewRequiresEachOption(t *testing.T) {
	cases := map[string]Options{
		"blank domain":        {Domain: "", ClientID: testClientID, ClientSecret: []byte(testClientSecret), RedirectURL: testRedirectURL},
		"blank client id":     {Domain: "tenant.example.com", ClientID: "", ClientSecret: []byte(testClientSecret), RedirectURL: testRedirectURL},
		"blank client secret": {Domain: "tenant.example.com", ClientID: testClientID, ClientSecret: nil, RedirectURL: testRedirectURL},
		"blank redirect url":  {Domain: "tenant.example.com", ClientID: testClientID, ClientSecret: []byte(testClientSecret), RedirectURL: ""},
	}
	for name, options := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := New(&options)
			assert.Error(t, err)
		})
	}
}

func TestNewRejectsMalformedURLs(t *testing.T) {
	cases := map[string]Options{
		"domain with scheme":      {Domain: "https://tenant.example.com", RedirectURL: testRedirectURL},
		"domain with path":        {Domain: "tenant.example.com/tenant", RedirectURL: testRedirectURL},
		"domain with userinfo":    {Domain: "user@tenant.example.com", RedirectURL: testRedirectURL},
		"redirect url relative":   {Domain: "tenant.example.com", RedirectURL: "/callback"},
		"redirect url plain http": {Domain: "tenant.example.com", RedirectURL: "http://app.example/callback"},
		"redirect url no path":    {Domain: "tenant.example.com", RedirectURL: "https://app.example"},
		"redirect url with query": {Domain: "tenant.example.com", RedirectURL: "https://app.example/callback?next=/"},
	}
	for name, options := range cases {
		t.Run(name, func(t *testing.T) {
			options.ClientID = testClientID
			options.ClientSecret = []byte(testClientSecret)
			_, err := New(&options)
			assert.Error(t, err)
		})
	}
}

func TestExchangeRequiresParameters(t *testing.T) {
	transport := &countingTransport{base: http.DefaultTransport}
	client, err := New(&Options{
		Domain: "tenant.example.com", ClientID: testClientID, ClientSecret: []byte(testClientSecret), RedirectURL: testRedirectURL,
		HTTPClient: &http.Client{Transport: transport},
	})
	require.NoError(t, err)

	cases := map[string]struct{ code, verifier, nonce string }{
		"empty code":     {"", testVerifier, "nonce-1"},
		"empty verifier": {testCode, "", "nonce-1"},
		"empty nonce":    {testCode, testVerifier, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.Exchange(context.Background(), c.code, c.verifier, c.nonce)
			assert.Error(t, err)
		})
	}
	assert.Zero(t, transport.count.Load(), "an invalid exchange must not contact the provider")
}

func TestReadIdentityRejectsMalformedTokens(t *testing.T) {
	cases := map[string]string{
		"not three parts":          "header.payload",
		"payload not valid base64": "header." + "not-base64!!!" + ".signature",
		"payload not valid json":   "header." + base64.RawURLEncoding.EncodeToString([]byte("not json")) + ".signature",
	}
	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := readIdentity(token)
			assert.Error(t, err)
		})
	}
}

func TestAuthorizationURL(t *testing.T) {
	client, err := New(&Options{
		Domain:       "tenant.example.com",
		ClientID:     testClientID,
		ClientSecret: []byte(testClientSecret),
		RedirectURL:  testRedirectURL,
	})
	require.NoError(t, err)

	verifier := "a-cryptographically-random-verifier"
	challenge := sha256.Sum256([]byte(verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(challenge[:])

	raw, err := client.AuthorizationURL(context.Background(), "state-1", "nonce-1", verifier)
	require.NoError(t, err)

	parsed, err := url.Parse(raw)
	require.NoError(t, err)
	assert.Equal(t, "https", parsed.Scheme)
	assert.Equal(t, "tenant.example.com", parsed.Host)
	assert.Equal(t, "/authorize", parsed.Path)

	query := parsed.Query()
	assert.Equal(t, "code", query.Get("response_type"))
	assert.Equal(t, testClientID, query.Get("client_id"))
	assert.Equal(t, testRedirectURL, query.Get("redirect_uri"))
	assert.Equal(t, "openid profile email", query.Get("scope"))
	assert.Equal(t, "state-1", query.Get("state"))
	assert.Equal(t, "nonce-1", query.Get("nonce"))
	assert.Equal(t, wantChallenge, query.Get("code_challenge"))
	assert.Equal(t, "S256", query.Get("code_challenge_method"))
}

func TestAuthorizationURLRequiresParameters(t *testing.T) {
	client, err := New(&Options{
		Domain: "tenant.example.com", ClientID: testClientID, ClientSecret: []byte(testClientSecret), RedirectURL: testRedirectURL,
	})
	require.NoError(t, err)

	cases := map[string]struct{ state, nonce, verifier string }{
		"empty state":    {"", "nonce", "verifier"},
		"empty nonce":    {"state", "", "verifier"},
		"empty verifier": {"state", "nonce", ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := client.AuthorizationURL(context.Background(), c.state, c.nonce, c.verifier)
			assert.Error(t, err)
		})
	}
}

func TestExchange(t *testing.T) {
	issuer := newFakeIssuer(t)
	now := time.Now()
	issuer.setIDToken(signRS256(t, issuer.key, testKeyID,
		tokenClaims(issuer.domain, testClientID, "user-1", now, map[string]any{"nonce": "nonce-1"})))

	client := newClient(t, issuer, issuer.server.Client(), time.Now)

	identity, err := client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	require.NoError(t, err)
	assert.Equal(t, "user-1", identity.Subject)
	assert.Equal(t, "rider@example.com", identity.Email)
	assert.Equal(t, "Rider Example", identity.Name)
}

// The access and admin claims are read only when a post-login Action actually
// sets them; a token from a tenant with no Action configured carries neither,
// and both must be read as false rather than causing an error.
func TestExchangeReadsTheAccessAndAdminClaims(t *testing.T) {
	for name, overrides := range map[string]struct {
		claims     map[string]any
		wantAccess bool
		wantAdmin  bool
	}{
		"both set": {
			claims: map[string]any{
				"nonce":                             "nonce-1",
				"https://domestique.invalid/access": true,
				"https://domestique.invalid/admin":  true,
			},
			wantAccess: true, wantAdmin: true,
		},
		"access only": {
			claims: map[string]any{
				"nonce":                             "nonce-1",
				"https://domestique.invalid/access": true,
			},
			wantAccess: true, wantAdmin: false,
		},
		"neither claim present": {
			claims:     map[string]any{"nonce": "nonce-1"},
			wantAccess: false, wantAdmin: false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			issuer := newFakeIssuer(t)
			now := time.Now()
			issuer.setIDToken(signRS256(t, issuer.key, testKeyID,
				tokenClaims(issuer.domain, testClientID, "user-1", now, overrides.claims)))

			client := newClient(t, issuer, issuer.server.Client(), time.Now)

			identity, err := client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
			require.NoError(t, err)
			assert.Equal(t, overrides.wantAccess, identity.Access, "Access")
			assert.Equal(t, overrides.wantAdmin, identity.Admin, "Admin")
		})
	}
}

func TestExchangeRefusesNonceMismatch(t *testing.T) {
	issuer := newFakeIssuer(t)
	now := time.Now()
	issuer.setIDToken(signRS256(t, issuer.key, testKeyID,
		tokenClaims(issuer.domain, testClientID, "user-1", now, map[string]any{"nonce": "a-different-nonce"})))

	client := newClient(t, issuer, issuer.server.Client(), time.Now)

	_, err := client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	assert.Error(t, err)
}

func TestExchangeRefusesWrongAudience(t *testing.T) {
	issuer := newFakeIssuer(t)
	now := time.Now()
	issuer.setIDToken(signRS256(t, issuer.key, testKeyID,
		tokenClaims(issuer.domain, "some-other-client", "user-1", now, map[string]any{"nonce": "nonce-1"})))

	client := newClient(t, issuer, issuer.server.Client(), time.Now)

	_, err := client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	assert.Error(t, err)
}

func TestExchangeRefusesExpiredToken(t *testing.T) {
	issuer := newFakeIssuer(t)
	now := time.Now()
	issuer.setIDToken(signRS256(t, issuer.key, testKeyID,
		tokenClaims(issuer.domain, testClientID, "user-1", now, map[string]any{
			"nonce": "nonce-1",
			"iat":   now.Add(-2 * time.Hour).Unix(),
			"exp":   now.Add(-time.Hour).Unix(),
		})))

	client := newClient(t, issuer, issuer.server.Client(), time.Now)

	_, err := client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	assert.Error(t, err)
}

// TestExchangeRefusesAlgConfusion signs the ID token with HS256 using the
// client secret as the HMAC key - the alg-confusion attack an RS256-only
// verifier must reject rather than accept as if it trusted the JWKS.
func TestExchangeRefusesAlgConfusion(t *testing.T) {
	issuer := newFakeIssuer(t)
	now := time.Now()
	claims := tokenClaims(issuer.domain, testClientID, "user-1", now, map[string]any{"nonce": "nonce-1"})
	header := map[string]any{"alg": "HS256", "typ": "JWT"}
	signingInput := segment(t, header) + "." + segment(t, claims)
	mac := hmacSHA256(testClientSecret, signingInput)
	issuer.setIDToken(signingInput + "." + base64.RawURLEncoding.EncodeToString(mac))

	client := newClient(t, issuer, issuer.server.Client(), time.Now)

	_, err := client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	assert.Error(t, err)
}

func TestExchangeRefusesMissingIDToken(t *testing.T) {
	issuer := newFakeIssuer(t)
	issuer.setIDToken("")

	client := newClient(t, issuer, issuer.server.Client(), time.Now)

	_, err := client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	assert.Error(t, err)
}

// countingTransport records how many round trips it forwards, so a test can
// assert that a code path contacted nothing.
type countingTransport struct {
	base  http.RoundTripper
	count atomic.Int64
}

func (c *countingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	c.count.Add(1)

	response, err := c.base.RoundTrip(request)
	if err != nil {
		return nil, fmt.Errorf("counting transport: %w", err)
	}

	return response, nil
}

func TestNewAndAuthorizationURLContactNothing(t *testing.T) {
	transport := &countingTransport{base: http.DefaultTransport}
	client, err := New(&Options{
		Domain: "tenant.example.com", ClientID: testClientID, ClientSecret: []byte(testClientSecret), RedirectURL: testRedirectURL,
		HTTPClient: &http.Client{Transport: transport},
	})
	require.NoError(t, err)

	_, err = client.AuthorizationURL(context.Background(), "state", "nonce", "verifier")
	require.NoError(t, err)

	assert.Zero(t, transport.count.Load())
}

// manualClock is an injectable clock a test can advance to exercise the
// initialisation floor without a wall-clock sleep.
type manualClock struct {
	now time.Time
	mu  sync.Mutex
}

func (c *manualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.now
}

func (c *manualClock) advance(by time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(by)
}

func TestExchangeInitialisationFloor(t *testing.T) {
	// A closed listener refuses connections immediately and deterministically,
	// without depending on an unroutable address behaving consistently across
	// environments.
	var listenConfig net.ListenConfig
	listener, err := listenConfig.Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)
	unreachable := listener.Addr().String()
	require.NoError(t, listener.Close())

	transport := &countingTransport{base: http.DefaultTransport}
	clock := &manualClock{now: time.Now()}
	client, err := New(&Options{
		Domain: unreachable, ClientID: testClientID, ClientSecret: []byte(testClientSecret), RedirectURL: testRedirectURL,
		HTTPClient: &http.Client{Timeout: time.Second, Transport: transport},
		Now:        clock.Now,
	})
	require.NoError(t, err)

	_, err = client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	require.Error(t, err)
	assert.EqualValues(t, 1, transport.count.Load())

	_, err = client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	require.Error(t, err)
	assert.EqualValues(t, 1, transport.count.Load(), "second attempt within the floor must not touch the network")

	clock.advance(initialisationFloor + time.Second)

	_, err = client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	require.Error(t, err)
	assert.EqualValues(t, 2, transport.count.Load(), "advancing past the floor must permit another attempt")
}

// TestExchangeRefusesOversizedTokenResponse pads an otherwise valid token
// response past the cap: without the cap the exchange would succeed, so the
// failure can only come from the truncated body.
func TestExchangeRefusesOversizedTokenResponse(t *testing.T) {
	issuer := newFakeIssuer(t)
	now := time.Now()
	idToken := signRS256(t, issuer.key, testKeyID,
		tokenClaims(issuer.domain, testClientID, "user-1", now, map[string]any{"nonce": "nonce-1"}))
	body, err := json.Marshal(map[string]any{
		"access_token": strings.Repeat("a", maxResponseBytes+1024),
		"id_token":     idToken,
		"token_type":   "Bearer",
		"expires_in":   3600,
	})
	require.NoError(t, err, "marshalling token response")
	issuer.setRawTokenBody(body)

	// The client is handed over unwrapped: bounding a caller-supplied client is
	// this package's guarantee, not the caller's.
	client := newClient(t, issuer, issuer.server.Client(), time.Now)

	_, err = client.Exchange(context.Background(), testCode, testVerifier, "nonce-1")
	assert.ErrorIs(t, err, errResponseTooLarge)
}

func TestBoundedHTTPClientBoundsTheTimeout(t *testing.T) {
	cases := map[string]struct {
		given *http.Client
		want  time.Duration
	}{
		"no client":       {given: nil, want: defaultTimeout},
		"no timeout":      {given: &http.Client{}, want: defaultTimeout},
		"longer timeout":  {given: &http.Client{Timeout: time.Minute}, want: defaultTimeout},
		"shorter timeout": {given: &http.Client{Timeout: time.Second}, want: time.Second},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			bounded := boundedHTTPClient(test.given)
			assert.Equal(t, test.want, bounded.Timeout)
			assert.IsType(t, &boundedTransport{}, bounded.Transport)
			if test.given != nil {
				assert.NotSame(t, test.given, bounded, "the caller's client must not be mutated")
			}
		})
	}
}

func TestBoundedHTTPClientRefusesRedirects(t *testing.T) {
	var followed atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		followed.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)

	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	t.Cleanup(redirector.Close)

	response, err := boundedHTTPClient(nil).Get(redirector.URL) //nolint:noctx // no context to carry in a transport test.
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, response.Body.Close()) })

	assert.Equal(t, http.StatusFound, response.StatusCode, "the redirect itself must be returned")
	assert.False(t, followed.Load(), "a request carrying the client secret must not be redirected")
}

func TestNewClonesTheClientSecret(t *testing.T) {
	secret := []byte("secret")
	client, err := New(&Options{
		Domain: "tenant.example.com", ClientID: testClientID, ClientSecret: secret, RedirectURL: testRedirectURL,
	})
	require.NoError(t, err)

	secret[0] = 'x'
	assert.Equal(t, []byte("secret"), client.clientSecret, "the caller must not be able to change the stored secret")
}

func TestLimitedBodyStopsAtTheCap(t *testing.T) {
	cases := map[string]struct {
		size    int
		wantErr bool
	}{
		"under the cap": {size: maxResponseBytes - 1},
		"at the cap":    {size: maxResponseBytes},
		"over the cap":  {size: maxResponseBytes + 1, wantErr: true},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			body := &limitedBody{
				body:      io.NopCloser(bytes.NewReader(bytes.Repeat([]byte("a"), test.size))),
				remaining: maxResponseBytes,
			}
			defer func() { assert.NoError(t, body.Close()) }()

			read, err := io.ReadAll(body)
			if test.wantErr {
				require.ErrorIs(t, err, errResponseTooLarge)
				assert.Len(t, read, maxResponseBytes, "the bytes within the cap are still handed back")

				return
			}
			require.NoError(t, err)
			assert.Len(t, read, test.size)
		})
	}
}
