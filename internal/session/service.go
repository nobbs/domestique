// Package session owns who is signed in: the sign-in flow that creates a
// browser session, and the check every later request is admitted by.
package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

const (
	// sessionLifetime is fixed, not renewed: whether a subject may still sign
	// in is an Auth0-Action-minted claim, checked only at sign-in time, so a
	// revoked subject is reached only by forcing every session back through a
	// real Auth0 round-trip within this bound rather than by a live per-request
	// check.
	sessionLifetime = 24 * time.Hour
	loginLifetime   = 10 * time.Minute
)

// NotAllowedError reports a subject Auth0 authenticated but did not assert
// the sign-in claim for. Error() deliberately omits the subject, so a logged
// error never carries it; callers that need it for display read the field.
type NotAllowedError struct{ Subject string }

func (e *NotAllowedError) Error() string { return "subject is not allowed to sign in" }

// Identity is the signed-in caller as later requests see it.
type Identity struct {
	Subject, Display string
	Admin            bool
}

// Login is a pending browser sign-in: where to send the browser, and the
// state it must return unchanged.
type Login struct{ AuthorizationURL, State string }

// Completion is a finished sign-in: the wire session token and its holder.
type Completion struct {
	ExpiresAt time.Time
	Token     string
	Identity  Identity
}

// Store persists pending sign-ins and completed sessions by digest; raw
// state and session values are never persisted.
type Store interface {
	BeginLogin(ctx context.Context, stateDigest []byte, nonce, codeVerifier string, now, expiresAt time.Time) error
	ConsumeLogin(ctx context.Context, stateDigest []byte, now time.Time) (nonce, codeVerifier string, err error)
	CreateSession(ctx context.Context, tokenDigest []byte, subject, display string, admin bool, now, expiresAt time.Time) error
	Session(ctx context.Context, tokenDigest []byte, now time.Time) (subject, display string, admin bool, expiresAt time.Time, err error)
	DeleteSession(ctx context.Context, tokenDigest []byte) error
}

// ExchangedIdentity is a verified ID token reduced to what this package needs
// from it — a value type of this package's own, not the adapter's, so
// Provider stays in primitives and never imports the adapter behind it.
// Access and Admin are the two namespaced claims an Auth0 Action mints:
// whether the subject may hold a session at all, and whether it holds
// cross-subject rights once it does.
type ExchangedIdentity struct {
	Subject, Email, Name string
	Access, Admin        bool
}

// Provider is the issuer as this package needs it.
type Provider interface {
	AuthorizationURL(ctx context.Context, state, nonce, codeVerifier string) (string, error)
	Exchange(ctx context.Context, code, codeVerifier, nonce string) (ExchangedIdentity, error)
}

// Service coordinates browser sign-in and admits later requests.
type Service struct {
	store    Store
	provider Provider
	now      func() time.Time
}

// New creates a session service with explicit consumer dependencies.
func New(store Store, provider Provider, now func() time.Time) (*Service, error) {
	if store == nil || provider == nil {
		return nil, errors.New("session store and provider are required")
	}
	if now == nil {
		now = time.Now
	}

	return &Service{store: store, provider: provider, now: now}, nil
}

// Begin mints a sign-in state and returns where to send the browser.
func (s *Service) Begin(ctx context.Context) (Login, error) {
	state, digest, err := randomToken()
	if err != nil {
		return Login{}, fmt.Errorf("minting login state: %w", err)
	}
	nonce, err := randomValue()
	if err != nil {
		return Login{}, fmt.Errorf("minting login nonce: %w", err)
	}
	verifier, err := randomValue()
	if err != nil {
		return Login{}, fmt.Errorf("minting code verifier: %w", err)
	}

	// The URL is built first: it is pure, and a failure there would otherwise
	// leave a pending login behind that no browser will ever return for.
	url, err := s.provider.AuthorizationURL(ctx, state, nonce, verifier)
	if err != nil {
		return Login{}, fmt.Errorf("building authorization url: %w", err)
	}

	now := s.now()
	if beginErr := s.store.BeginLogin(ctx, digest, nonce, verifier, now, now.Add(loginLifetime)); beginErr != nil {
		return Login{}, fmt.Errorf("storing login: %w", beginErr)
	}

	return Login{AuthorizationURL: url, State: state}, nil
}

// Complete validates and consumes a pending sign-in, exchanges the
// authorization code, and creates a browser session for a subject Auth0
// asserted the sign-in claim for.
func (s *Service) Complete(ctx context.Context, state, cookieState, code string) (Completion, error) {
	// The digests are compared, not the wire values: they are fixed length, so
	// the comparison cannot leak through an early return on differing lengths,
	// and no slice of attacker-chosen size is allocated.
	digest, err := tokenDigest(state)
	cookieDigest, cookieErr := tokenDigest(cookieState)
	if err != nil || cookieErr != nil || subtle.ConstantTimeCompare(digest, cookieDigest) != 1 {
		return Completion{}, errors.New("login state did not match")
	}

	now := s.now()
	nonce, verifier, err := s.store.ConsumeLogin(ctx, digest, now)
	if err != nil {
		return Completion{}, fmt.Errorf("consuming login: %w", err)
	}

	exchanged, err := s.provider.Exchange(ctx, code, verifier, nonce)
	if err != nil {
		return Completion{}, fmt.Errorf("exchanging authorization code: %w", err)
	}
	if !exchanged.Access {
		return Completion{}, &NotAllowedError{Subject: exchanged.Subject}
	}
	display := display(exchanged.Email, exchanged.Name, exchanged.Subject)

	token, tokenDigestBytes, err := randomToken()
	if err != nil {
		return Completion{}, fmt.Errorf("minting session token: %w", err)
	}
	expiresAt := now.Add(sessionLifetime)
	if err := s.store.CreateSession(ctx, tokenDigestBytes, exchanged.Subject, display, exchanged.Admin, now, expiresAt); err != nil {
		return Completion{}, fmt.Errorf("storing session: %w", err)
	}

	return Completion{
		Token:     token,
		Identity:  Identity{Subject: exchanged.Subject, Display: display, Admin: exchanged.Admin},
		ExpiresAt: expiresAt,
	}, nil
}

// Verify admits a caller by its stored session row. Whether the subject may
// still sign in is checked only at sign-in time now; sessionLifetime is what
// bounds how long a revoked subject's existing session keeps working.
func (s *Service) Verify(ctx context.Context, token string) (Identity, error) {
	digest, err := tokenDigest(token)
	if err != nil {
		return Identity{}, errors.New("session token is invalid")
	}

	subject, display, admin, _, err := s.store.Session(ctx, digest, s.now())
	if err != nil {
		return Identity{}, fmt.Errorf("reading session: %w", err)
	}

	return Identity{Subject: subject, Display: display, Admin: admin}, nil
}

// Revoke ends a browser session.
func (s *Service) Revoke(ctx context.Context, token string) error {
	digest, err := tokenDigest(token)
	if err != nil {
		return errors.New("session token is invalid")
	}
	if err := s.store.DeleteSession(ctx, digest); err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}

	return nil
}

// display picks the first non-empty of email, name, or subject.
func display(email, name, subject string) string {
	if email != "" {
		return email
	}
	if name != "" {
		return name
	}

	return subject
}

// tokenBytes is how much randomness a state or session token carries.
const tokenBytes = 32

// randomRaw mints tokenBytes of randomness.
func randomRaw() ([]byte, error) {
	raw := make([]byte, tokenBytes)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("reading randomness: %w", err)
	}

	return raw, nil
}

// randomValue mints a wire value this service never stores, so it needs no
// digest: the nonce and the PKCE verifier go to the provider and nowhere else.
func randomValue() (string, error) {
	raw, err := randomRaw()
	if err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// randomToken mints a wire value together with the digest it is stored under.
func randomToken() (wire string, digest []byte, err error) {
	raw, err := randomRaw()
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(raw)

	return base64.RawURLEncoding.EncodeToString(raw), sum[:], nil
}

// tokenDigest checks the encoded length before decoding: these values arrive
// from a browser, and nothing should be allocated to the caller's measure.
func tokenDigest(wire string) ([]byte, error) {
	if len(wire) != base64.RawURLEncoding.EncodedLen(tokenBytes) {
		return nil, errors.New("value is invalid")
	}
	raw, err := base64.RawURLEncoding.DecodeString(wire)
	if err != nil || len(raw) != tokenBytes {
		return nil, errors.New("value is invalid")
	}
	sum := sha256.Sum256(raw)

	return sum[:], nil
}
