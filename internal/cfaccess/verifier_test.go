package cfaccess_test

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nobbs/domestique/internal/cfaccess"
)

const (
	testTeam     = "example"
	testIssuer   = "https://example.cloudflareaccess.com"
	testAudience = "aud-tag-for-this-application"
	testKeyID    = "key-1"
)

// fixedNow is the instant every test evaluates claims at.
func fixedNow() time.Time {
	return time.Unix(1_700_000_000, 0).UTC()
}

// keySet is a signing key plus the JWKS document that publishes it.
type keySet struct {
	private *rsa.PrivateKey
	keyID   string
}

// newKeySet generates a signing key for one test.
func newKeySet(t *testing.T, keyID string) *keySet {
	t.Helper()

	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}

	return &keySet{private: private, keyID: keyID}
}

// jwks renders the key set as Cloudflare's certs document.
func (k *keySet) jwks() []byte {
	document := map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA",
			"kid": k.keyID,
			"alg": "RS256",
			"use": "sig",
			"n":   base64.RawURLEncoding.EncodeToString(k.private.N.Bytes()),
			"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.private.E)).Bytes()),
		}},
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}

	return encoded
}

// sign produces a compact JWT over the supplied header and claims.
func (k *keySet) sign(t *testing.T, header, claims map[string]any) string {
	t.Helper()

	segment := func(value map[string]any) string {
		encoded, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshalling segment: %v", err)
		}

		return base64.RawURLEncoding.EncodeToString(encoded)
	}

	signingInput := segment(header) + "." + segment(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, k.private, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("signing: %v", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

// validClaims is the claim set a well-formed assertion carries.
func validClaims() map[string]any {
	return map[string]any{
		"iss":   testIssuer,
		"sub":   "subject-1",
		"aud":   []string{testAudience},
		"email": "rider@example.com",
		"exp":   fixedNow().Add(time.Hour).Unix(),
		"iat":   fixedNow().Add(-time.Minute).Unix(),
		"nbf":   fixedNow().Add(-time.Minute).Unix(),
	}
}

// validHeader is the JOSE header a well-formed assertion carries.
func validHeader() map[string]any {
	return map[string]any{"alg": "RS256", "kid": testKeyID, "typ": "JWT"}
}

// clock is a test clock the caller can advance.
type clock struct {
	offset atomic.Int64
}

// now reports the fixed instant plus whatever the test has advanced.
func (c *clock) now() time.Time {
	return fixedNow().Add(time.Duration(c.offset.Load()))
}

// advance moves the clock forward.
func (c *clock) advance(by time.Duration) {
	c.offset.Add(int64(by))
}

// newVerifier wires a verifier to a JWKS server backed by the key set and
// reports how many times the key set was fetched.
func newVerifier(t *testing.T, keys *keySet) (*cfaccess.Verifier, *atomic.Int64) {
	t.Helper()

	verifier, fetches, _ := newVerifierWithClock(t, keys)

	return verifier, fetches
}

// newVerifierWithClock is newVerifier plus control over the verifier's clock.
func newVerifierWithClock(t *testing.T, keys *keySet) (*cfaccess.Verifier, *atomic.Int64, *clock) {
	t.Helper()

	var fetches atomic.Int64
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/cdn-cgi/access/certs" {
			writer.WriteHeader(http.StatusNotFound)

			return
		}
		fetches.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		if _, err := writer.Write(keys.jwks()); err != nil {
			t.Errorf("writing key set: %v", err)
		}
	}))
	t.Cleanup(server.Close)

	client := server.Client()
	// The verifier builds an https URL from the team domain, so redirect that
	// host to the test server rather than weakening certificate checking.
	transport := client.Transport
	client.Transport = rewriteHost{next: transport, target: strings.TrimPrefix(server.URL, "https://")}

	testClock := &clock{}
	verifier, err := cfaccess.New(&cfaccess.Options{
		TeamDomain: testTeam,
		Audience:   testAudience,
		HTTPClient: client,
		Now:        testClock.now,
	})
	if err != nil {
		t.Fatalf("building verifier: %v", err)
	}

	return verifier, &fetches, testClock
}

// rewriteHost sends requests for the team domain to the test server.
type rewriteHost struct {
	next   http.RoundTripper
	target string
}

// RoundTrip rewrites the request host, leaving TLS verification intact against
// the test server's own certificate.
func (r rewriteHost) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL.Host = r.target

	return r.next.RoundTrip(clone) //nolint:wrapcheck // transport shim must return the transport's error unchanged
}

func TestVerifyAcceptsValidAssertion(t *testing.T) {
	t.Parallel()

	keys := newKeySet(t, testKeyID)
	verifier, fetches := newVerifier(t, keys)

	identity, err := verifier.Verify(t.Context(), keys.sign(t, validHeader(), validClaims()))
	if err != nil {
		t.Fatalf("expected acceptance, got %v", err)
	}
	if identity.Email != "rider@example.com" {
		t.Fatalf("expected the email claim, got %q", identity.Email)
	}
	if identity.Subject != "subject-1" {
		t.Fatalf("expected the subject claim, got %q", identity.Subject)
	}

	// A second assertion must reuse the cached key set.
	if _, err = verifier.Verify(t.Context(), keys.sign(t, validHeader(), validClaims())); err != nil {
		t.Fatalf("expected the cached key to verify, got %v", err)
	}
	if got := fetches.Load(); got != 1 {
		t.Fatalf("expected one key fetch, got %d", got)
	}
}

func TestVerifyAcceptsAudienceAsString(t *testing.T) {
	t.Parallel()

	keys := newKeySet(t, testKeyID)
	verifier, _ := newVerifier(t, keys)

	claims := validClaims()
	claims["aud"] = testAudience

	if _, err := verifier.Verify(t.Context(), keys.sign(t, validHeader(), claims)); err != nil {
		t.Fatalf("expected a string aud to be accepted, got %v", err)
	}
}

func TestVerifyRejectsBadAssertions(t *testing.T) {
	t.Parallel()

	cases := map[string]struct {
		header func() map[string]any
		claims func() map[string]any
	}{
		"audience of another application": {
			header: validHeader,
			claims: func() map[string]any {
				claims := validClaims()
				claims["aud"] = []string{"aud-tag-of-a-different-application"}

				return claims
			},
		},
		"issuer of another team": {
			header: validHeader,
			claims: func() map[string]any {
				claims := validClaims()
				claims["iss"] = "https://attacker.cloudflareaccess.com"

				return claims
			},
		},
		"expired": {
			header: validHeader,
			claims: func() map[string]any {
				claims := validClaims()
				claims["exp"] = fixedNow().Add(-time.Second).Unix()

				return claims
			},
		},
		"no expiry": {
			header: validHeader,
			claims: func() map[string]any {
				claims := validClaims()
				delete(claims, "exp")

				return claims
			},
		},
		"not yet valid": {
			header: validHeader,
			claims: func() map[string]any {
				claims := validClaims()
				claims["nbf"] = fixedNow().Add(time.Hour).Unix()

				return claims
			},
		},
		"no email claim": {
			header: validHeader,
			claims: func() map[string]any {
				claims := validClaims()
				delete(claims, "email")

				return claims
			},
		},
		"unknown key id": {
			header: func() map[string]any {
				header := validHeader()
				header["kid"] = "key-that-was-never-published"

				return header
			},
			claims: validClaims,
		},
		"no key id": {
			header: func() map[string]any {
				header := validHeader()
				delete(header, "kid")

				return header
			},
			claims: validClaims,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			keys := newKeySet(t, testKeyID)
			verifier, _ := newVerifier(t, keys)

			if _, err := verifier.Verify(t.Context(), keys.sign(t, testCase.header(), testCase.claims())); err == nil {
				t.Fatal("expected rejection, got acceptance")
			}
		})
	}
}

// TestVerifyRejectsAlgorithmSubstitution covers the attack the pinned algorithm
// exists to prevent: a token that names a weaker algorithm, or none at all,
// must never be verified even though it is otherwise well formed.
func TestVerifyRejectsAlgorithmSubstitution(t *testing.T) {
	t.Parallel()

	for _, algorithm := range []string{"none", "HS256", "RS512", ""} {
		t.Run("alg "+algorithm, func(t *testing.T) {
			t.Parallel()

			keys := newKeySet(t, testKeyID)
			verifier, _ := newVerifier(t, keys)

			header := validHeader()
			header["alg"] = algorithm

			if _, err := verifier.Verify(t.Context(), keys.sign(t, header, validClaims())); err == nil {
				t.Fatalf("expected %q to be rejected", algorithm)
			}
		})
	}
}

// TestVerifyRejectsForeignSignature covers a correctly shaped assertion signed
// by a key the team never published.
func TestVerifyRejectsForeignSignature(t *testing.T) {
	t.Parallel()

	published := newKeySet(t, testKeyID)
	verifier, _ := newVerifier(t, published)

	// Same advertised key ID, different private key.
	forged := newKeySet(t, testKeyID)

	if _, err := verifier.Verify(t.Context(), forged.sign(t, validHeader(), validClaims())); err == nil {
		t.Fatal("expected a foreign signature to be rejected")
	}
}

func TestVerifyRejectsMalformedAssertions(t *testing.T) {
	t.Parallel()

	cases := map[string]string{
		"empty":            "",
		"two segments":     "aaaa.bbbb",
		"not base64url":    "!!!.???.###",
		"payload not json": base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"key-1"}`)) + ".bm90LWpzb24.c2ln",
	}

	for name, token := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			keys := newKeySet(t, testKeyID)
			verifier, _ := newVerifier(t, keys)

			if _, err := verifier.Verify(t.Context(), token); err == nil {
				t.Fatalf("expected %s to be rejected", name)
			}
		})
	}
}

// Access rotates its signing key roughly every six weeks. An assertion under a
// key ID this process has not seen must be accepted once the key set is
// refetched, or the service locks its operator out at every rotation.
func TestVerifyRefetchesAfterKeyRotation(t *testing.T) {
	t.Parallel()

	keys := newKeySet(t, testKeyID)
	verifier, fetches, testClock := newVerifierWithClock(t, keys)

	if _, err := verifier.Verify(t.Context(), keys.sign(t, validHeader(), validClaims())); err != nil {
		t.Fatalf("expected the first assertion to verify, got %v", err)
	}

	// The endpoint now publishes a different key under a new ID.
	rotated := newKeySet(t, "key-2")
	keys.private = rotated.private
	keys.keyID = rotated.keyID

	header := validHeader()
	header["kid"] = "key-2"

	// Past the refresh floor, the unknown key ID triggers one refetch and the
	// assertion verifies against the newly published key.
	testClock.advance(2 * time.Minute)

	if _, err := verifier.Verify(t.Context(), rotated.sign(t, header, validClaims())); err != nil {
		t.Fatalf("expected the rotated key to verify, got %v", err)
	}
	if got := fetches.Load(); got != 2 {
		t.Fatalf("expected exactly one refetch, got %d fetches", got)
	}
}

// A stream of assertions naming key IDs that do not exist must not become a
// request flood against Cloudflare.
func TestVerifyRateLimitsRefetchForUnknownKeyID(t *testing.T) {
	t.Parallel()

	keys := newKeySet(t, testKeyID)
	verifier, fetches, _ := newVerifierWithClock(t, keys)

	if _, err := verifier.Verify(t.Context(), keys.sign(t, validHeader(), validClaims())); err != nil {
		t.Fatalf("expected the first assertion to verify, got %v", err)
	}

	header := validHeader()
	header["kid"] = "key-that-was-never-published"

	for range 5 {
		if _, err := verifier.Verify(t.Context(), keys.sign(t, header, validClaims())); err == nil {
			t.Fatal("expected an unknown key ID to be rejected")
		}
	}
	if got := fetches.Load(); got != 1 {
		t.Fatalf("expected the refresh floor to suppress refetches, got %d fetches", got)
	}
}

func TestNewValidatesOptions(t *testing.T) {
	t.Parallel()

	cases := map[string]*cfaccess.Options{
		"nil options":    nil,
		"no team domain": {Audience: testAudience},
		"no audience":    {TeamDomain: testTeam},
		"team with path": {TeamDomain: "example.cloudflareaccess.com/x", Audience: testAudience},
		"blank team":     {TeamDomain: "   ", Audience: testAudience},
		"blank audience": {TeamDomain: testTeam, Audience: "  "},
	}

	for name, options := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if _, err := cfaccess.New(options); err == nil {
				t.Fatalf("expected %s to be rejected", name)
			}
		})
	}
}

// TestNewAcceptsFullTeamDomain covers the operator writing either form.
func TestNewAcceptsFullTeamDomain(t *testing.T) {
	t.Parallel()

	for _, team := range []string{"example", "example.cloudflareaccess.com"} {
		if _, err := cfaccess.New(&cfaccess.Options{TeamDomain: team, Audience: testAudience}); err != nil {
			t.Fatalf("expected %q to be accepted, got %v", team, err)
		}
	}
}
