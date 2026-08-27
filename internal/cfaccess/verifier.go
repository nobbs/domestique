// Package cfaccess verifies Cloudflare Access JWT assertions so the service can
// establish a caller identity on a request path that carries no Tailnet
// identity of its own.
//
// Tailscale Serve populates its identity headers only for traffic from user
// nodes, and never for a tagged device. A request that reaches this service
// through the public Cloudflare path therefore arrives with no
// Tailscale-User-Login at all, and Serve strips any the client tried to supply.
// The signed assertion Cloudflare Access adds is the only identity such a
// request carries, so it is the only thing worth trusting on that path.
//
// The JOSE work is github.com/coreos/go-oidc's, which is what Cloudflare's
// documentation points at for this.
package cfaccess

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	jose "github.com/go-jose/go-jose/v4"
)

const (
	// AssertionHeader carries the signed Access token. Cloudflare also sets a
	// CF_Authorization cookie, but the cookie is not guaranteed to be forwarded,
	// so the header is the documented thing to verify.
	AssertionHeader = "Cf-Access-Jwt-Assertion"

	// signingAlgorithm is the only algorithm this verifier accepts. Pinning it
	// here, rather than reading it from the token, is what makes algorithm
	// confusion impossible: an attacker cannot downgrade a token to "none" or
	// to an HMAC verified with the public key as its secret.
	signingAlgorithm = jose.RS256

	// certsPath is the JWKS endpoint every Access team domain exposes.
	certsPath = "/cdn-cgi/access/certs"

	// fetchTimeout bounds a single JWKS retrieval.
	fetchTimeout = 5 * time.Second

	// maxCertsBytes caps the JWKS response. The real document is a few
	// kilobytes; this stops a hostile or broken endpoint consuming memory.
	maxCertsBytes = 1 << 20

	// minRefreshInterval rate-limits refetching after an unknown key ID, so a
	// stream of bogus tokens cannot turn into a request flood against
	// Cloudflare. It counts attempts rather than successes: a limit that only
	// advances on success stops limiting exactly when the endpoint is
	// unhealthy, which is when the flood would cost the most.
	minRefreshInterval = time.Minute

	// clockSkew tolerates a small amount of clock drift on the not-before and
	// issued-at claims. Expiry is checked strictly by go-oidc: accepting an
	// expired token is a real weakening, whereas rejecting a token issued a few
	// seconds into this host's future is merely inconvenient.
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

	// Audience is the AUD tag of the one Access application that fronts this
	// service. A token minted for a different application of the same team is
	// signed by the same key, so this check is what stops it being accepted.
	Audience string
}

// Verifier validates Access assertions against the team's published keys.
type Verifier struct {
	verifier *oidc.IDTokenVerifier
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
	// otherwise be accepted here and then fetch this service's signing keys
	// from elsewhere.example.
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
	// rather than from the one verifying a token, and fetches on that context
	// too — so it is Background, and the client's timeout is what bounds a fetch.
	keySetClient := *client
	keySetClient.Transport = &throttledTransport{
		base: client.Transport,
		now:  now,
	}
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
		now: now,
	}, nil
}

// Verify checks the assertion's signature and claims and returns the identity
// it names. Every failure returns the same opaque error category to the caller;
// the detail stays here rather than being reflected to a client.
//
// go-oidc checks the signature, issuer, audience and expiry; not-before and
// issued-at are checked here because it does not look at them.
func (v *Verifier) Verify(ctx context.Context, token string) (Identity, error) {
	if err := requireKeyID(token); err != nil {
		return Identity{}, err
	}

	idToken, err := v.verifier.Verify(ctx, token)
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

// throttledTransport is the refresh floor, and the cap on how much of a reply
// is read. It sits here rather than around the key set because oidc.KeySet is a
// whole verification, which cannot tell an unknown key ID from any other failure.
type throttledTransport struct {
	base        http.RoundTripper
	now         func() time.Time
	lastAttempt time.Time
	mu          sync.Mutex
}

func (t *throttledTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.mu.Lock()
	// Stamped before the request rather than after it, so a failing endpoint
	// still closes the window until minRefreshInterval has passed.
	if t.now().Sub(t.lastAttempt) < minRefreshInterval {
		t.mu.Unlock()

		return nil, errRefreshFloor
	}
	t.lastAttempt = t.now()
	t.mu.Unlock()

	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	// The deadline is applied here rather than left to the client, because the
	// key set is built on Background and a caller may supply a client with no
	// timeout of its own; without this such a fetch would never give up.
	ctx, cancel := context.WithTimeout(request.Context(), fetchTimeout)
	response, err := base.RoundTrip(request.WithContext(ctx))
	if err != nil {
		cancel()

		return nil, fmt.Errorf("fetching access certs: %w", err)
	}
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
