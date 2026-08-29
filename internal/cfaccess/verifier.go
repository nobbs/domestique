// Package cfaccess verifies Cloudflare Access JWT assertions, which are the only
// identity a request through the public Cloudflare path carries: Tailscale Serve
// populates no identity header for a tagged device and strips any the client
// supplied. The JOSE work is github.com/coreos/go-oidc's.
package cfaccess

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

const (
	// AssertionHeader carries the signed Access token. The CF_Authorization
	// cookie is not guaranteed to be forwarded; the header is what to verify.
	AssertionHeader = "Cf-Access-Jwt-Assertion"

	// signingAlgorithm is the only algorithm this verifier accepts. Pinning it
	// here makes algorithm confusion impossible: no downgrade to "none" or to an
	// HMAC verified with the public key as its secret.
	signingAlgorithm = jose.RS256

	// certsPath is the JWKS endpoint every Access team domain exposes.
	certsPath = "/cdn-cgi/access/certs"

	// fetchTimeout bounds a single JWKS retrieval.
	fetchTimeout = 5 * time.Second

	// maxCertsBytes caps the JWKS response. The real document is a few
	// kilobytes; this stops a hostile or broken endpoint consuming memory.
	maxCertsBytes = 1 << 20

	// minRefreshInterval rate-limits refetching after an unknown key ID. It counts
	// attempts rather than successes: a limit that only advances on success stops
	// limiting exactly when the endpoint is unhealthy.
	minRefreshInterval = time.Minute

	// clockSkew tolerates drift on not-before and issued-at. Expiry is checked
	// strictly by go-oidc: accepting an expired token is a real weakening.
	clockSkew = 30 * time.Second
)

// errRefreshFloor stands in for a request the floor below refused to send.
var errRefreshFloor = errors.New("cfaccess: key endpoint asked too recently")

// Identity is the verified caller a valid assertion names.
type Identity struct {
	// Email is the address the identity provider asserted. With a single
	// authorised user there is no local user table to look it up in.
	Email string

	// Subject is the stable per-application user identifier.
	Subject string
}

// Options configures a Verifier.
type Options struct {
	// HTTPClient fetches the JWKS. Defaults to a client with a bounded timeout.
	HTTPClient *http.Client

	// Now supplies the current time. Defaults to time.Now.
	Now func() time.Time

	// TeamDomain is the Zero Trust team domain, either "example" or the full
	// "example.cloudflareaccess.com".
	TeamDomain string

	// Audience is the AUD tag of the one Access application fronting this service.
	// A token for another application of the same team is signed by the same key.
	Audience string
}

// Verifier validates Access assertions against the team's published keys.
type Verifier struct {
	verifier *oidc.IDTokenVerifier
	throttle *throttledTransport
	now      func() time.Time
}

// New validates the options and returns a Verifier. It contacts nothing: the
// key set is fetched lazily on the first assertion.
func New(options *Options) (*Verifier, error) {
	if options == nil {
		return nil, errors.New("cfaccess options are required")
	}

	team := strings.TrimSpace(options.TeamDomain)
	if team == "" {
		return nil, errors.New("cfaccess team domain is required")
	}
	team = strings.TrimSuffix(strings.TrimPrefix(team, "https://"), "/")
	if !strings.Contains(team, ".") {
		team += ".cloudflareaccess.com"
	}
	// Userinfo and a port are rejected alongside path, query and fragment: the
	// JWKS URL is built by concatenation, so "user@elsewhere.example" would
	// otherwise fetch this service's signing keys from elsewhere.example.
	if strings.ContainsAny(team, "/?#@:") {
		return nil, errors.New("cfaccess team domain must be a bare host")
	}

	audience := strings.TrimSpace(options.Audience)
	if audience == "" {
		return nil, errors.New("cfaccess audience is required")
	}

	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: fetchTimeout}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	// go-oidc takes the key set's HTTP client from the context it is built with
	// and fetches on that context, so it is Background and the client's timeout
	// is what bounds a fetch.
	keySetClient := *client
	throttle := &throttledTransport{
		base: client.Transport,
		now:  now,
	}
	keySetClient.Transport = throttle
	keySet := oidc.NewRemoteKeySet(
		oidc.ClientContext(context.Background(), &keySetClient),
		"https://"+team+certsPath,
	)

	return &Verifier{
		verifier: oidc.NewVerifier(
			"https://"+team,
			keySet,
			&oidc.Config{
				ClientID:             audience,
				SupportedSigningAlgs: []string{string(signingAlgorithm)},
				Now:                  now,
			},
		),
		throttle: throttle,
		now:      now,
	}, nil
}

// Verify checks the assertion's signature and claims and returns the identity it
// names. Every failure returns the same opaque category; detail stays here.
// go-oidc checks signature, issuer, audience and expiry; not-before and
// issued-at are checked here because it does not look at them.
func (v *Verifier) Verify(ctx context.Context, token string) (Identity, error) {
	if err := requireKeyID(token); err != nil {
		return Identity{}, err
	}

	fetched, refused := v.throttle.counts()
	idToken, err := v.verifier.Verify(ctx, token)
	// One verification is retried: the one the floor refused while the fetch that
	// stamped it was landing the keys it needed. Both halves of the condition
	// matter — without the refusal, a genuinely bad assertion would have its
	// error replaced; without the completed fetch, a bogus key ID costs two.
	if nowFetched, nowRefused := v.throttle.counts(); err != nil &&
		nowRefused != refused && nowFetched != fetched {
		idToken, err = v.verifier.Verify(ctx, token)
	}
	if err != nil {
		return Identity{}, fmt.Errorf("verifying assertion: %w", err)
	}

	var claims struct {
		Email     string `json:"email"`
		NotBefore int64  `json:"nbf"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, fmt.Errorf("decoding assertion claims: %w", err)
	}

	now := v.now()
	if claims.NotBefore != 0 && now.Add(clockSkew).Before(time.Unix(claims.NotBefore, 0)) {
		return Identity{}, errors.New("assertion is not yet valid")
	}
	if !idToken.IssuedAt.IsZero() && now.Add(clockSkew).Before(idToken.IssuedAt) {
		return Identity{}, errors.New("assertion was issued in the future")
	}

	email := strings.TrimSpace(claims.Email)
	if email == "" {
		return Identity{}, errors.New("assertion carries no email claim")
	}

	return Identity{Email: email, Subject: idToken.Subject}, nil
}

// requireKeyID rejects an assertion whose header names no key. Access always
// names one; go-oidc does not insist, and tries every published key instead.
func requireKeyID(token string) error {
	parsed, err := jose.ParseSigned(token, []jose.SignatureAlgorithm{signingAlgorithm})
	if err != nil {
		return fmt.Errorf("parsing assertion: %w", err)
	}
	if len(parsed.Signatures) != 1 {
		return errors.New("assertion does not carry exactly one signature")
	}
	if parsed.Signatures[0].Header.KeyID == "" {
		return errors.New("assertion has no key ID")
	}

	return nil
}

// throttledTransport is the refresh floor and the cap on how much of a reply is
// read. It sits here rather than around the key set because oidc.KeySet is a
// whole verification, which cannot tell an unknown key ID from any other failure.
type throttledTransport struct {
	base        http.RoundTripper
	now         func() time.Time
	lastAttempt time.Time
	// fetched counts round trips that reached a reply and refused counts the ones
	// the floor turned away, so a verification can tell a key set that landed
	// underneath it from one that never came.
	fetched atomic.Uint64
	refused atomic.Uint64
	mu      sync.Mutex
}

// counts reports the fetches that have completed and the ones the floor has
// turned away.
func (t *throttledTransport) counts() (fetched, refused uint64) {
	return t.fetched.Load(), t.refused.Load()
}

func (t *throttledTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.mu.Lock()
	// Stamped before the request rather than after it, so a failing endpoint
	// still closes the window until minRefreshInterval has passed.
	if t.now().Sub(t.lastAttempt) < minRefreshInterval {
		t.mu.Unlock()
		t.refused.Add(1)

		return nil, errRefreshFloor
	}
	t.lastAttempt = t.now()
	t.mu.Unlock()

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	// The deadline is applied here rather than left to the client: the key set is
	// built on Background and a caller may supply a client with no timeout.
	ctx, cancel := context.WithTimeout(request.Context(), fetchTimeout)
	response, err := base.RoundTrip(request.WithContext(ctx))
	if err != nil {
		cancel()

		return nil, fmt.Errorf("fetching access certs: %w", err)
	}
	t.fetched.Add(1)
	response.Body = newLimitedBody(response.Body, cancel)

	return response, nil
}

// newLimitedBody caps the reply and releases the deadline once it is closed.
// Cancelling any earlier would cut off the read the deadline exists to bound.
func newLimitedBody(body io.ReadCloser, cancel context.CancelFunc) io.ReadCloser {
	return struct {
		io.Reader
		io.Closer
	}{
		Reader: io.LimitReader(body, maxCertsBytes),
		Closer: closerFunc(func() error {
			err := body.Close()
			cancel()
			if err != nil {
				return fmt.Errorf("closing access certs response: %w", err)
			}

			return nil
		}),
	}
}

type closerFunc func() error

func (c closerFunc) Close() error { return c() }
