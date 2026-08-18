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
package cfaccess

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
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
	signingAlgorithm = "RS256"

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
	// issued-at claims. Expiry is checked strictly: accepting an expired token
	// is a real weakening, whereas rejecting a token issued a few seconds into
	// this host's future is merely inconvenient.
	clockSkew = 30 * time.Second
)

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
	client *http.Client
	now    func() time.Time

	// keys and lastAttempt are guarded by mu.
	keys        map[string]*rsa.PublicKey
	lastAttempt time.Time

	issuer   string
	audience string
	certsURL string

	mu sync.Mutex
}

// New validates the options and returns a Verifier. It contacts nothing: the
// key set is fetched lazily on the first assertion, under that request's
// context.
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

	return &Verifier{
		issuer:   "https://" + team,
		audience: audience,
		certsURL: "https://" + team + certsPath,
		client:   client,
		now:      now,
		keys:     map[string]*rsa.PublicKey{},
	}, nil
}

// Verify checks the assertion's signature and claims and returns the identity
// it names. Every failure returns the same opaque error category to the caller;
// the detail stays here rather than being reflected to a client.
func (v *Verifier) Verify(ctx context.Context, token string) (Identity, error) {
	header, claims, signingInput, signature, err := split(token)
	if err != nil {
		return Identity{}, err
	}
	if header.Algorithm != signingAlgorithm {
		return Identity{}, fmt.Errorf("unexpected signing algorithm %q", header.Algorithm)
	}
	if header.KeyID == "" {
		return Identity{}, errors.New("assertion has no key ID")
	}

	key, err := v.key(ctx, header.KeyID)
	if err != nil {
		return Identity{}, err
	}

	digest := sha256.Sum256(signingInput)
	if err = rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return Identity{}, fmt.Errorf("assertion signature is invalid: %w", err)
	}
	if claimsErr := v.checkClaims(claims); claimsErr != nil {
		return Identity{}, claimsErr
	}

	email := strings.TrimSpace(claims.Email)
	if email == "" {
		return Identity{}, errors.New("assertion carries no email claim")
	}

	return Identity{Email: email, Subject: claims.Subject}, nil
}

// checkClaims enforces issuer, audience, and the time window.
func (v *Verifier) checkClaims(claims *claimSet) error {
	if claims.Issuer != v.issuer {
		return fmt.Errorf("assertion issuer %q is not this team", claims.Issuer)
	}
	if !claims.Audience.contains(v.audience) {
		return errors.New("assertion audience does not match this application")
	}

	now := v.now()
	if claims.ExpiresAt == 0 {
		return errors.New("assertion has no expiry")
	}
	if now.After(time.Unix(claims.ExpiresAt, 0)) {
		return errors.New("assertion has expired")
	}
	if claims.NotBefore != 0 && now.Add(clockSkew).Before(time.Unix(claims.NotBefore, 0)) {
		return errors.New("assertion is not yet valid")
	}
	if claims.IssuedAt != 0 && now.Add(clockSkew).Before(time.Unix(claims.IssuedAt, 0)) {
		return errors.New("assertion was issued in the future")
	}

	return nil
}

// key returns the public key for a key ID, refreshing the cached set once if
// the ID is unknown. Access rotates its signing key every six weeks and serves
// the previous key for a further seven days, so an unknown ID normally means a
// rotation this process has not seen yet.
func (v *Verifier) key(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	key, ok := v.keys[keyID]
	stale := v.now().Sub(v.lastAttempt) >= minRefreshInterval
	v.mu.Unlock()

	if ok {
		return key, nil
	}
	if !stale {
		return nil, fmt.Errorf("unknown assertion key ID %q", keyID)
	}
	if err := v.refresh(ctx); err != nil {
		return nil, err
	}

	v.mu.Lock()
	key, ok = v.keys[keyID]
	v.mu.Unlock()

	if !ok {
		return nil, fmt.Errorf("unknown assertion key ID %q", keyID)
	}

	return key, nil
}

// refresh replaces the cached key set from the team's JWKS endpoint.
func (v *Verifier) refresh(ctx context.Context) (err error) {
	// Stamped before the request rather than after it, so a failing endpoint
	// still closes the window until minRefreshInterval has passed.
	v.mu.Lock()
	v.lastAttempt = v.now()
	v.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.certsURL, http.NoBody)
	if err != nil {
		return fmt.Errorf("building access certs request: %w", err)
	}

	response, err := v.client.Do(request)
	if err != nil {
		return fmt.Errorf("fetching access certs: %w", err)
	}
	defer func() {
		err = errors.Join(err, response.Body.Close())
	}()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("access certs endpoint returned status %d", response.StatusCode)
	}

	var document struct {
		Keys []jsonWebKey `json:"keys"`
	}
	if err = json.NewDecoder(io.LimitReader(response.Body, maxCertsBytes)).Decode(&document); err != nil {
		return fmt.Errorf("decoding access certs: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for index := range document.Keys {
		webKey := &document.Keys[index]
		if webKey.KeyType != "RSA" || webKey.KeyID == "" {
			continue
		}
		if webKey.Algorithm != "" && webKey.Algorithm != signingAlgorithm {
			continue
		}
		parsed, keyErr := webKey.publicKey()
		if keyErr != nil {
			continue
		}
		keys[webKey.KeyID] = parsed
	}
	if len(keys) == 0 {
		return errors.New("access certs endpoint returned no usable RSA keys")
	}

	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()

	return nil
}

// jsonWebKey is the subset of a JWK this verifier needs.
type jsonWebKey struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Algorithm string `json:"alg"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

// publicKey converts the JWK's base64url big-endian fields into an RSA key.
func (k *jsonWebKey) publicKey() (*rsa.PublicKey, error) {
	modulus, err := base64.RawURLEncoding.DecodeString(k.Modulus)
	if err != nil {
		return nil, fmt.Errorf("decoding key modulus: %w", err)
	}
	exponent, err := base64.RawURLEncoding.DecodeString(k.Exponent)
	if err != nil {
		return nil, fmt.Errorf("decoding key exponent: %w", err)
	}
	if len(modulus) == 0 || len(exponent) == 0 || len(exponent) > 8 {
		return nil, errors.New("key parameters are out of range")
	}

	value := new(big.Int).SetBytes(exponent).Int64()
	if value < 3 || value > 1<<31 {
		return nil, errors.New("key exponent is out of range")
	}

	return &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: int(value)}, nil
}

// jwtHeader is the decoded JOSE header.
type jwtHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}

// claimSet is the subset of the payload this verifier enforces or reads.
type claimSet struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Email     string   `json:"email"`
	Audience  audience `json:"aud"`
	ExpiresAt int64    `json:"exp"`
	NotBefore int64    `json:"nbf"`
	IssuedAt  int64    `json:"iat"`
}

// audience accepts the aud claim in either of its permitted JSON shapes.
type audience []string

// UnmarshalJSON decodes aud from a single string or an array of strings.
func (a *audience) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*a = audience{single}

		return nil
	}

	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return fmt.Errorf("decoding aud claim: %w", err)
	}
	*a = many

	return nil
}

// contains reports whether the wanted tag is present, compared in constant time
// so the check does not leak the expected value through timing.
func (a audience) contains(wanted string) bool {
	found := false
	for _, candidate := range a {
		if len(candidate) == len(wanted) &&
			subtle.ConstantTimeCompare([]byte(candidate), []byte(wanted)) == 1 {
			found = true
		}
	}

	return found
}

// split parses the compact serialisation into its verified-together parts.
func split(token string) (header *jwtHeader, claims *claimSet, signingInput, signature []byte, err error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, nil, nil, errors.New("assertion is not a compact JWT")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("decoding assertion header: %w", err)
	}
	claimBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("decoding assertion claims: %w", err)
	}
	signature, err = base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("decoding assertion signature: %w", err)
	}

	header = &jwtHeader{}
	if err = json.Unmarshal(headerBytes, header); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parsing assertion header: %w", err)
	}
	claims = &claimSet{}
	if err = json.Unmarshal(claimBytes, claims); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("parsing assertion claims: %w", err)
	}

	signingInput = []byte(parts[0] + "." + parts[1])

	return header, claims, signingInput, signature, nil
}
