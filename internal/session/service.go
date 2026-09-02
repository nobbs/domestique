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
	"strings"
	"time"
)

const (
	sessionLifetime = 30 * 24 * time.Hour
	renewInterval   = time.Hour
	loginLifetime   = 10 * time.Minute
)

// NotAllowedError reports a subject that authenticated but is not on the
// configured allowlist. Error() deliberately omits the subject, so a logged
// error never carries it; callers that need it for display read the field.
type NotAllowedError struct{ Subject string }

func (e *NotAllowedError) Error() string { return "subject is not allowed to sign in" }

// Identity is the signed-in caller as later requests see it.
type Identity struct{ Subject, Display string }

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
	CreateSession(ctx context.Context, tokenDigest []byte, subject, display string, now, expiresAt time.Time) error
	Session(ctx context.Context, tokenDigest []byte, now time.Time) (subject, display string, renewedAt, expiresAt time.Time, err error)
	RenewSession(ctx context.Context, tokenDigest []byte, now, expiresAt time.Time) error
	DeleteSession(ctx context.Context, tokenDigest []byte) error
}

// Provider is the issuer as this package needs it, in primitives so this
// package never imports the adapter behind it.
type Provider interface {
	AuthorizationURL(ctx context.Context, state, nonce, codeVerifier string) (string, error)
	Exchange(ctx context.Context, code, codeVerifier, nonce string) (subject, email, name string, err error)
}

// Service coordinates browser sign-in and admits later requests.
type Service struct {
	store           Store
	provider        Provider
	allowedSubjects map[string]struct{}
	now             func() time.Time
}

// New creates a session service with explicit consumer dependencies.
func New(store Store, provider Provider, allowedSubjects []string, now func() time.Time) (*Service, error) {
	if store == nil || provider == nil {
		return nil, errors.New("session store and provider are required")
	}
	allowed := make(map[string]struct{}, len(allowedSubjects))
	for _, subject := range allowedSubjects {
		if trimmed := strings.TrimSpace(subject); trimmed != "" {
			allowed[trimmed] = struct{}{}
		}
	}
	if len(allowed) == 0 {
		return nil, errors.New("at least one allowed subject is required")
	}
	if now == nil {
		now = time.Now
	}

	return &Service{store: store, provider: provider, allowedSubjects: allowed, now: now}, nil
}

// Begin mints a sign-in state and returns where to send the browser.
func (s *Service) Begin(ctx context.Context) (Login, error) {
	state, digest, err := randomToken()
	if err != nil {
		return Login{}, fmt.Errorf("minting login state: %w", err)
	}
	nonce, _, err := randomToken()
	if err != nil {
		return Login{}, fmt.Errorf("minting login nonce: %w", err)
	}
	verifier, _, err := randomToken()
	if err != nil {
		return Login{}, fmt.Errorf("minting code verifier: %w", err)
	}

	now := s.now()
	if beginErr := s.store.BeginLogin(ctx, digest, nonce, verifier, now, now.Add(loginLifetime)); beginErr != nil {
		return Login{}, fmt.Errorf("storing login: %w", beginErr)
	}
	url, err := s.provider.AuthorizationURL(ctx, state, nonce, verifier)
	if err != nil {
		return Login{}, fmt.Errorf("building authorization url: %w", err)
	}

	return Login{AuthorizationURL: url, State: state}, nil
}

// Complete validates and consumes a pending sign-in, exchanges the
// authorization code, and creates a browser session for an allowed subject.
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

	subject, email, name, err := s.provider.Exchange(ctx, code, verifier, nonce)
	if err != nil {
		return Completion{}, fmt.Errorf("exchanging authorization code: %w", err)
	}
	if _, ok := s.allowedSubjects[subject]; !ok {
		return Completion{}, &NotAllowedError{Subject: subject}
	}
	display := display(email, name, subject)

	token, tokenDigestBytes, err := randomToken()
	if err != nil {
		return Completion{}, fmt.Errorf("minting session token: %w", err)
	}
	expiresAt := now.Add(sessionLifetime)
	if err := s.store.CreateSession(ctx, tokenDigestBytes, subject, display, now, expiresAt); err != nil {
		return Completion{}, fmt.Errorf("storing session: %w", err)
	}

	return Completion{
		Token:     token,
		Identity:  Identity{Subject: subject, Display: display},
		ExpiresAt: expiresAt,
	}, nil
}

// Verify admits a caller. renewedUntil is zero unless the sliding expiry
// moved, in which case the cookie must be re-issued carrying it.
func (s *Service) Verify(ctx context.Context, token string) (Identity, time.Time, error) {
	digest, err := tokenDigest(token)
	if err != nil {
		return Identity{}, time.Time{}, errors.New("session token is invalid")
	}

	now := s.now()
	subject, display, renewedAt, _, err := s.store.Session(ctx, digest, now)
	if err != nil {
		return Identity{}, time.Time{}, fmt.Errorf("reading session: %w", err)
	}
	if _, ok := s.allowedSubjects[subject]; !ok {
		return Identity{}, time.Time{}, errors.New("subject is no longer allowed")
	}

	var renewedUntil time.Time
	if now.Sub(renewedAt) >= renewInterval {
		renewedUntil = now.Add(sessionLifetime)
		if err := s.store.RenewSession(ctx, digest, now, renewedUntil); err != nil {
			return Identity{}, time.Time{}, fmt.Errorf("renewing session: %w", err)
		}
	}

	return Identity{Subject: subject, Display: display}, renewedUntil, nil
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

// randomToken mints 32 random bytes as a wire value and its storage digest.
func randomToken() (wire string, digest []byte, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", nil, fmt.Errorf("reading randomness: %w", err)
	}
	sum := sha256.Sum256(raw)

	return base64.RawURLEncoding.EncodeToString(raw), sum[:], nil
}

func tokenDigest(wire string) ([]byte, error) {
	raw, err := base64.RawURLEncoding.DecodeString(wire)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("value is invalid")
	}
	sum := sha256.Sum256(raw)

	return sum[:], nil
}
